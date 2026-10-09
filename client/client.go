// Package client é um cliente mínimo para a Docker Engine API.
//
// Sem dependências externas: usa só a biblioteca padrão e fala HTTP com o daemon
// indicado pelo DOCKER_HOST (padrão: tcp://localhost:2375 no Windows e
// unix:///var/run/docker.sock no Linux). Ver daemon.go.
//
// Como no SDK oficial do Docker, todo método recebe um context.Context como
// primeiro parâmetro: cancelar o ctx interrompe a chamada (útil com o
// r.Context() de uma requisição HTTP ou com um timeout).
//
// Arquivos:
//
//	client.go     New, Close, Ping e o envio das requisições
//	daemon.go     DOCKER_HOST (tcp/unix) e PortsHost
//	container.go  listar, inspecionar, criar, ligar, parar, reiniciar
//	images.go     ImageExists, PullImage, EnsureImage
//	errors.go     APIError, IsNotFound, IsConflict
//	types.go      tipos da Engine API
package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// apiVersion é a versão da Engine API fixada nas rotas (/v1.44/...). Fixar
// evita que uma atualização do Docker mude campos sem aviso (o SDK oficial
// faz o mesmo).
const apiVersion = "v1.44"

// requestTimeout é o limite das chamadas rápidas (listar, inspecionar, criar).
// Chamadas longas (pull da imagem, stop e restart) não têm limite fixo: quem
// decide é o ctx.
const requestTimeout = 30 * time.Second

// pingTimeout é quanto o New espera o Docker responder.
const pingTimeout = 5 * time.Second

// ErrOffline é devolvido pelo New quando o Docker não responde. Teste com
// errors.Is(err, client.ErrOffline).
var ErrOffline = errors.New("docker offline")

// Client conversa com o daemon Docker. É seguro usar o mesmo Client em várias
// goroutines (ex.: um só para a API inteira).
type Client struct {
	host    string       // DOCKER_HOST usado (ver DaemonHost)
	baseURL string       // ex.: http://localhost:2375/v1.44 (com socket Unix: http://docker/v1.44)
	http    *http.Client // chamadas rápidas, com requestTimeout
	stream  *http.Client // chamadas longas, sem timeout (mesmas conexões do http)
}

// New cria um Client para o Docker do DOCKER_HOST (ou o padrão do sistema) e
// faz um Ping para conferir se ele está online (espera até 5 s).
//
// Dois tipos de erro:
//   - DOCKER_HOST inválido: devolve c = nil (não há como conectar).
//   - Docker não respondeu: devolve ErrOffline JUNTO com um Client pronto; quem
//     chama decide se para o programa ou continua e tenta depois.
//
// Exemplo:
//
//	c, err := client.New()
//	if err != nil {
//		log.Fatal(err) // ou, se errors.Is(err, client.ErrOffline), só avisar e seguir
//	}
//	defer c.Close()
func New() (*Client, error) {
	c, err := newClient(defaultDaemonHost())
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		return c, fmt.Errorf("%w: %w", ErrOffline, err)
	}
	return c, nil
}

// Close fecha as conexões ociosas com o daemon. Não mexe nos containers: eles
// continuam rodando. Chamadas em andamento (ex.: pull) não são cortadas;
// cancele o ctx que você passou para elas.
func (c *Client) Close() error {
	c.http.CloseIdleConnections()
	return nil
}

// Ping verifica se o daemon Docker responde (GET /_ping).
func (c *Client) Ping(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/_ping", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("docker inacessível em %s: %w", c.host, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return newAPIError(resp, "falar com o docker")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Ajudantes pequenos (não fazem chamada HTTP).
// ---------------------------------------------------------------------------

// closeBody lê o resto do corpo (até 64 KB) antes de fechar. Sem isso o Go
// fecha a conexão em vez de reaproveitar, e a próxima chamada abre outra.
func closeBody(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}

// pathID valida e escapa um Id ou nome para usar na URL (impede que algo como
// "../images" mude a rota chamada).
func pathID(id, what string) (string, error) {
	id = trimSpace(id)
	if id == "" {
		return "", fmt.Errorf("informe o Id ou nome do %s", what)
	}
	if id == "." || id == ".." { // PathEscape não escapa pontos
		return "", fmt.Errorf("Id ou nome do %s inválido: %q", what, id)
	}
	return url.PathEscape(id), nil
}
