package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Imagens. Cada função faz a sua própria chamada HTTP ao daemon:
//
//	ImageExists  GET  /images/{nome}/json
//	PullImage    POST /images/create?fromImage=...&tag=...
//	EnsureImage  ImageExists e, se faltar, PullImage

// ImageExists diz se a imagem (ex.: "nginx:latest") já está no host.
func (c *Client) ImageExists(ctx context.Context, image string) (bool, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return false, fmt.Errorf("imagem não especificada")
	}
	reqURL := fmt.Sprintf("%s/images/%s/json", c.baseURL, url.PathEscape(image))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("docker inacessível em %s: %w", c.host, err)
	}
	defer closeBody(resp)

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, newAPIError(resp, "inspecionar a imagem "+image)
	}
}

// PullImage baixa a imagem (ex.: "nginx", "itzg/minecraft-server:java21") do
// registry. Sem tag usa "latest" — sem isso o Docker baixaria TODAS as tags.
// Bloqueia até o download terminar (sem timeout: use o ctx para limitar).
func (c *Client) PullImage(ctx context.Context, image string) error {
	image = strings.TrimSpace(image)
	if image == "" {
		return fmt.Errorf("imagem não especificada")
	}
	q := url.Values{"fromImage": {image}}
	if !hasTagOrDigest(image) {
		q.Set("tag", "latest")
	}
	reqURL := fmt.Sprintf("%s/images/create?%s", c.baseURL, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.stream.Do(req) // sem timeout: imagens grandes demoram
	if err != nil {
		return fmt.Errorf("docker inacessível em %s: %w", c.host, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return newAPIError(resp, "baixar a imagem "+image)
	}

	// O daemon responde 200 e manda o progresso como uma sequência de objetos
	// JSON; um erro no meio do download vem num desses objetos, no campo "error".
	dec := json.NewDecoder(resp.Body)
	var msg struct {
		Error string `json:"error"`
	}
	for {
		msg.Error = ""
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("erro ao ler progresso do download de %q: %w", image, err)
		}
		if msg.Error != "" {
			return fmt.Errorf("erro ao baixar a imagem %q: %s", image, msg.Error)
		}
	}
}

// EnsureImage baixa a imagem só se ela ainda não existir no host.
func (c *Client) EnsureImage(ctx context.Context, image string) error {
	exists, err := c.ImageExists(ctx, image)
	if err != nil || exists {
		return err
	}
	return c.PullImage(ctx, image)
}

// hasTagOrDigest informa se a referência já traz tag ("nginx:1.25") ou digest
// ("nginx@sha256:..."). O ":" só conta depois da última "/", para não confundir
// com a porta do registry ("localhost:5000/app").
func hasTagOrDigest(image string) bool {
	if strings.Contains(image, "@") {
		return true
	}
	return strings.Contains(image[strings.LastIndex(image, "/")+1:], ":")
}
