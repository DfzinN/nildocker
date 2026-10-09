package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

// Endereço do daemon Docker (DOCKER_HOST), no mesmo formato do SDK oficial:
//
//	tcp://localhost:2375          TCP sem TLS (Docker Desktop com "Expose daemon")
//	http://10.0.0.10:2375         igual ao tcp://
//	https://10.0.0.10:2376        HTTPS (sem certificado de cliente por enquanto)
//	unix:///var/run/docker.sock   socket Unix (padrão e mais seguro no Linux)
const (
	defaultHostUnix    = "unix:///var/run/docker.sock"
	defaultHostWindows = "tcp://localhost:2375" // npipe ainda não é suportado
	dockerEnvFile      = "/.dockerenv"          // existe quando o programa roda dentro de um container
)

// defaultDaemonHost devolve o DOCKER_HOST do ambiente ou o padrão do sistema.
func defaultDaemonHost() string {
	if h := strings.TrimSpace(os.Getenv("DOCKER_HOST")); h != "" {
		return h
	}
	if runtime.GOOS == "windows" {
		return defaultHostWindows
	}
	return defaultHostUnix
}

// newClient lê daemonHost e monta o Client: a base das rotas e uma conexão
// (Transport) própria, reaproveitada por todas as chamadas. Com socket Unix a
// base é só um nome fixo: a conexão vai direto para o arquivo do socket.
func newClient(daemonHost string) (*Client, error) {
	u, err := url.Parse(daemonHost)
	if err != nil {
		return nil, fmt.Errorf("DOCKER_HOST inválido %q: %w", daemonHost, err)
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{
		// Todas as chamadas vão para o mesmo daemon: guarda mais conexões
		// abertas para ele (o padrão do Go é só 2), evitando reconectar.
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		// Sem Proxy: o Docker é local ou da rede interna; um HTTP_PROXY do
		// sistema não pode desviar as chamadas.
	}

	var base string
	switch u.Scheme {
	case "tcp", "http", "https":
		if u.Host == "" {
			return nil, fmt.Errorf("DOCKER_HOST %q sem endereço (ex.: tcp://localhost:2375)", daemonHost)
		}
		scheme := "http"
		if u.Scheme == "https" {
			scheme = "https"
		}
		base = scheme + "://" + u.Host
		tr.DialContext = dialer.DialContext

	case "unix":
		socket := u.Path
		if socket == "" {
			return nil, fmt.Errorf("DOCKER_HOST %q sem caminho do socket (ex.: unix:///var/run/docker.sock)", daemonHost)
		}
		base = "http://docker" // o host é ignorado: quem decide é o DialContext
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socket)
		}

	case "npipe":
		return nil, fmt.Errorf("DOCKER_HOST %q: npipe ainda não é suportado; no Windows use tcp://localhost:2375", daemonHost)

	default:
		return nil, fmt.Errorf("DOCKER_HOST %q: esquema %q não suportado (use tcp, http, https ou unix)", daemonHost, u.Scheme)
	}

	return &Client{
		host:    daemonHost,
		baseURL: fmt.Sprintf("%s/%s", base, apiVersion),
		http:    &http.Client{Transport: tr, Timeout: requestTimeout},
		stream:  &http.Client{Transport: tr}, // mesmo Transport: mesmas conexões
	}, nil
}

// DaemonHost devolve o endereço do Docker usado por este Client
// (ex.: "tcp://localhost:2375" ou "unix:///var/run/docker.sock").
func (c *Client) DaemonHost() string {
	return c.host
}

// PortsHost devolve o endereço (sem porta) onde as portas publicadas dos
// containers podem ser acessadas — ex.: para montar "host:25565" de um servidor.
//
//   - tcp/http/https: o host do DOCKER_HOST (ex.: "10.0.0.10").
//   - unix/npipe: "localhost"; se este programa roda dentro de um container,
//     o gateway da rede bridge (o "localhost" do container não é a máquina).
func (c *Client) PortsHost(ctx context.Context) (string, error) {
	return c.daemonHostLocked(ctx)
}

// daemonHostLocked descobre o host a partir do DOCKER_HOST. Baseado no
// testcontainers-go (lá "Locked" quer dizer que o mutex já está travado; aqui
// não há mutex, o nome foi mantido).
func (c *Client) daemonHostLocked(ctx context.Context) (string, error) {
	daemonURL, err := url.Parse(c.DaemonHost())
	if err != nil {
		return "", err
	}

	switch daemonURL.Scheme {
	case "http", "https", "tcp":
		return daemonURL.Hostname(), nil
	case "unix", "npipe":
		if inAContainer(dockerEnvFile) {
			if ip, err := c.getGatewayIP(ctx, "bridge"); err == nil {
				return ip.String(), nil
			}
		}
		return "localhost", nil
	default:
		return "", errors.New("não foi possível descobrir o host pelo DOCKER_HOST")
	}
}

// inAContainer diz se este programa está rodando dentro de um container Docker.
func inAContainer(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// getGatewayIP devolve o gateway da rede (ex.: "bridge" → 172.17.0.1), que é
// como um container enxerga a máquina onde o Docker roda.
// GET /networks/{nome}
func (c *Client) getGatewayIP(ctx context.Context, network string) (net.IP, error) {
	id, err := pathID(network, "rede")
	if err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("%s/networks/%s", c.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker inacessível em %s: %w", c.host, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, newAPIError(resp, "inspecionar a rede "+network)
	}

	var rede struct {
		IPAM struct {
			Config []struct {
				Gateway string `json:"Gateway"`
			} `json:"Config"`
		} `json:"IPAM"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rede); err != nil {
		return nil, fmt.Errorf("erro ao ler a rede %q: %w", network, err)
	}
	for _, cfg := range rede.IPAM.Config {
		if ip := net.ParseIP(cfg.Gateway); ip != nil {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("rede %q sem gateway", network)
}
