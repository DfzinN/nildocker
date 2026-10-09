package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Containers. Cada função faz a sua própria chamada HTTP ao daemon:
//
//	ListContainers    GET  /containers/json?all=true
//	InspectContainer  GET  /containers/{id}/json
//	CreateContainer   POST /containers/create  (via postCreate; se faltar a imagem, PullImage e tenta de novo)
//	StartContainer    POST /containers/{id}/start
//	StopContainer     POST /containers/{id}/stop?t=N
//	RestartContainer  POST /containers/{id}/restart?t=N

// ListContainers lista todos os containers (rodando ou parados).
func (c *Client) ListContainers(ctx context.Context) ([]Container, error) {
	reqURL := fmt.Sprintf("%s/containers/json?all=true", c.baseURL)

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
		return nil, newAPIError(resp, "listar containers")
	}

	var list []Container
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("erro ao ler a lista de containers: %w", err)
	}
	return list, nil
}

// InspectContainer devolve os detalhes de um container (por Id ou nome): estado,
// configuração, rede, volumes. Container que não existe: IsNotFound(err).
func (c *Client) InspectContainer(ctx context.Context, containerID string) (*Inspect, error) {
	id, err := pathID(containerID, "container")
	if err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("%s/containers/%s/json", c.baseURL, id)

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
		return nil, newAPIError(resp, "inspecionar o container "+containerID)
	}

	var out Inspect
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("erro ao ler o inspect: %w", err)
	}
	return &out, nil
}

// CreateContainer cria um container e devolve o Id dele.
//
// name é opcional (passe "" para o Docker gerar um); nome já em uso:
// IsConflict(err). Se a imagem não existir no host, ela é baixada e a criação
// é tentada de novo (pode demorar). Se body.HostConfig.DNS estiver vazio,
// aplica DefaultDNS (Cloudflare + Google).
func (c *Client) CreateContainer(ctx context.Context, name string, body CreateContainer) (CreateResponse, error) {
	if trimSpace(body.Image) == "" {
		return CreateResponse{}, fmt.Errorf("imagem não especificada")
	}
	body.ensureDNS()

	raw, err := json.Marshal(body)
	if err != nil {
		return CreateResponse{}, fmt.Errorf("erro ao serializar o container: %w", err)
	}

	// Tenta criar direto (caso comum: imagem já baixada, uma chamada só). Se a
	// imagem não existir, o Docker responde 404: baixa e tenta de novo.
	out, err := c.postCreate(ctx, trimSpace(name), raw)
	if IsNotFound(err) {
		if err := c.PullImage(ctx, body.Image); err != nil {
			return CreateResponse{}, err
		}
		out, err = c.postCreate(ctx, trimSpace(name), raw)
	}
	return out, err
}

// postCreate faz o POST /containers/create com o corpo já em JSON.
func (c *Client) postCreate(ctx context.Context, name string, raw []byte) (CreateResponse, error) {
	reqURL := fmt.Sprintf("%s/containers/create", c.baseURL)
	if name != "" {
		reqURL = fmt.Sprintf("%s?name=%s", reqURL, url.QueryEscape(name))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(raw))
	if err != nil {
		return CreateResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return CreateResponse{}, fmt.Errorf("docker inacessível em %s: %w", c.host, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusCreated {
		return CreateResponse{}, newAPIError(resp, "criar container") // 404 sem imagem · 409 nome em uso
	}

	var out CreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return CreateResponse{}, fmt.Errorf("erro ao ler a resposta da criação: %w", err)
	}
	return out, nil
}

// StartContainer liga um container (por Id ou nome). Já ligado: não dá erro.
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	id, err := pathID(containerID, "container")
	if err != nil {
		return err
	}
	reqURL := fmt.Sprintf("%s/containers/%s/start", c.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("docker inacessível em %s: %w", c.host, err)
	}
	defer closeBody(resp)

	// 204 = ligou · 304 = já estava ligado
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
		return newAPIError(resp, "ligar o container "+containerID)
	}
	return nil
}

// StopContainer desliga um container. timeout é quanto o Docker espera o
// processo fechar sozinho antes de matar (SIGKILL); 0 usa o padrão do Docker
// (10 s). Já parado: não dá erro.
//
// Usa o client sem timeout: o Docker só responde depois de parar, o que pode
// levar o timeout inteiro; cortar antes daria erro com ele ainda parando.
func (c *Client) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	id, err := pathID(containerID, "container")
	if err != nil {
		return err
	}
	reqURL := fmt.Sprintf("%s/containers/%s/stop", c.baseURL, id)
	if timeout > 0 {
		reqURL = fmt.Sprintf("%s?t=%d", reqURL, int(timeout.Seconds()))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return fmt.Errorf("docker inacessível em %s: %w", c.host, err)
	}
	defer closeBody(resp)

	// 204 = parou · 304 = já estava parado
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
		return newAPIError(resp, "parar o container "+containerID)
	}
	return nil
}

// RestartContainer reinicia um container. timeout: ver StopContainer.
func (c *Client) RestartContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	id, err := pathID(containerID, "container")
	if err != nil {
		return err
	}
	reqURL := fmt.Sprintf("%s/containers/%s/restart", c.baseURL, id)
	if timeout > 0 {
		reqURL = fmt.Sprintf("%s?t=%d", reqURL, int(timeout.Seconds()))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.stream.Do(req) // sem timeout: ver StopContainer
	if err != nil {
		return fmt.Errorf("docker inacessível em %s: %w", c.host, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusNoContent {
		return newAPIError(resp, "reiniciar o container "+containerID)
	}
	return nil
}
