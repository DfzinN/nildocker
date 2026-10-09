package client

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Logs de um container. Cada chamada ao Docker fica numa função própria:
//
//	LogsContainer       histórico: valida → inspeciona → busca os logs (devolve string)
//	StreamLogs          ao vivo:   valida → inspeciona → segue os logs (escreve em w)
//	tipoContainer       diz se o texto recebido é um id ou um nome
//	inspecionarParaLogs GET /containers/{id}/json  (só o que os logs precisam)
//	buscarLogs          GET /containers/{id}/logs              (termina sozinho)
//	seguirLogs          GET /containers/{id}/logs?follow=true  (só termina pelo ctx)
//	lerStream           tira os cabeçalhos de 8 bytes do stream do Docker

// LogsContainer devolve os logs (stdout + stderr) que o container já tem, por
// id ou nome. Para acompanhar ao vivo, use StreamLogs.
func (c *Client) LogsContainer(ctx context.Context, container string) (string, error) {
	// 1. Valida: veio um id ou um nome?
	tipo, err := tipoContainer(container)
	if err != nil {
		return "", err
	}
	container = strings.TrimPrefix(strings.TrimSpace(container), "/")

	// 2. Inspeciona: confirma que existe e descobre se tem TTY.
	tty, err := c.inspecionarParaLogs(ctx, container)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", tipo, container, err)
	}

	// 3. Busca os logs.
	return c.buscarLogs(ctx, container, tty)
}

// StreamLogs acompanha os logs do container ao vivo e escreve cada pedaço em w
// assim que chega (ex.: o http.ResponseWriter do console do painel).
//
// tail é quantas linhas antigas mandar antes das novas (ex.: 200); 0 manda o
// histórico inteiro — evite em sites e bancos, que acumulam milhões de linhas.
//
// Bloqueia até o ctx ser cancelado (devolve nil — ex.: o usuário fechou a
// página, com r.Context()) ou o container parar (nil). Não tem timeout fixo.
func (c *Client) StreamLogs(ctx context.Context, container string, tail int, w io.Writer) error {
	// 1. Valida: veio um id ou um nome?
	tipo, err := tipoContainer(container)
	if err != nil {
		return err
	}
	container = strings.TrimPrefix(strings.TrimSpace(container), "/")

	// 2. Inspeciona: confirma que existe e descobre se tem TTY.
	tty, err := c.inspecionarParaLogs(ctx, container)
	if err != nil {
		return fmt.Errorf("%s %q: %w", tipo, container, err)
	}

	// 3. Segue os logs até o ctx acabar ou o container parar.
	err = c.seguirLogs(ctx, container, tail, tty, w)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil // quem chamou mandou parar: não é erro
	}
	return err
}

// ---------------------------------------------------------------------------
// 1. Validação
// ---------------------------------------------------------------------------

var (
	// Id do Docker: hexadecimal, completo (64) ou curto (a partir de 12).
	idRE = regexp.MustCompile(`^[0-9a-f]{12,64}$`)
	// Nome do Docker: começa com letra ou número; depois letras, números, _ . -
	nomeRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
)

// tipoContainer diz se o texto é um "id" ou um "nome" de container. A barra do
// começo ("/mc", como vem no ListContainers) é aceita.
//
//	tipoContainer("3f2a9c1b7d4e")  → "id"
//	tipoContainer("mc")            → "nome"
//	tipoContainer("")              → erro: obrigatório
//	tipoContainer("../images")     → erro: inválido
func tipoContainer(container string) (string, error) {
	container = strings.TrimPrefix(strings.TrimSpace(container), "/")
	switch {
	case container == "":
		return "", errors.New("id ou nome do container é obrigatório")
	case idRE.MatchString(container):
		return "id", nil // um nome só de hex também cai aqui; o Docker aceita os dois
	case nomeRE.MatchString(container):
		return "nome", nil
	default:
		return "", fmt.Errorf("id ou nome do container inválido: %q", container)
	}
}

// ---------------------------------------------------------------------------
// 2. GET /containers/{id}/json
// ---------------------------------------------------------------------------

// inspecionarParaLogs confirma que o container existe e diz se ele tem TTY
// (sem TTY o Docker manda os logs com cabeçalhos; com TTY manda texto puro).
func (c *Client) inspecionarParaLogs(ctx context.Context, container string) (bool, error) {
	reqURL := fmt.Sprintf("%s/containers/%s/json", c.baseURL, url.PathEscape(container))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("docker inacessível em %s: %w", c.host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, newAPIError(resp, "inspecionar o container") // 404 → IsNotFound(err)
	}

	var insp struct {
		Config struct {
			Tty bool `json:"Tty"`
		} `json:"Config"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&insp); err != nil {
		return false, fmt.Errorf("erro ao ler o inspect: %w", err)
	}
	return insp.Config.Tty, nil
}

// ---------------------------------------------------------------------------
// 3. GET /containers/{id}/logs
// ---------------------------------------------------------------------------

// buscarLogs baixa os logs (stdout + stderr) e devolve como texto. Sem
// follow: o Docker manda o que já existe e termina a resposta.
func (c *Client) buscarLogs(ctx context.Context, container string, tty bool) (string, error) {
	q := url.Values{"stdout": {"true"}, "stderr": {"true"}}
	reqURL := fmt.Sprintf("%s/containers/%s/logs?%s", c.baseURL, url.PathEscape(container), q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("docker inacessível em %s: %w", c.host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", newAPIError(resp, "ler logs do container "+container)
	}

	var out strings.Builder
	if tty {
		_, err = io.Copy(&out, resp.Body) // com TTY: texto puro
	} else {
		err = lerStream(&out, resp.Body) // sem TTY: tira os cabeçalhos
	}
	if err != nil {
		return "", fmt.Errorf("erro ao ler logs: %w", err)
	}
	return out.String(), nil
}

// ---------------------------------------------------------------------------
// 4. GET /containers/{id}/logs?follow=true
// ---------------------------------------------------------------------------

// seguirLogs abre os logs com follow=true: o Docker manda o histórico (tail
// linhas) e depois continua mandando cada linha nova, sem terminar a resposta.
// Usa c.stream (sem timeout): quem encerra é o ctx.
func (c *Client) seguirLogs(ctx context.Context, container string, tail int, tty bool, w io.Writer) error {
	q := url.Values{"stdout": {"true"}, "stderr": {"true"}, "follow": {"true"}}
	if tail > 0 {
		q.Set("tail", strconv.Itoa(tail))
	} else {
		q.Set("tail", "all")
	}
	reqURL := fmt.Sprintf("%s/containers/%s/logs?%s", c.baseURL, url.PathEscape(container), q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return fmt.Errorf("docker inacessível em %s: %w", c.host, err)
	}
	defer resp.Body.Close() // não drena: o stream só acaba pelo ctx

	if resp.StatusCode != http.StatusOK {
		return newAPIError(resp, "seguir logs do container "+container)
	}

	if tty {
		_, err = io.Copy(w, resp.Body) // com TTY: texto puro
	} else {
		err = lerStream(w, resp.Body) // sem TTY: tira os cabeçalhos
	}
	return err
}

// lerStream copia o stream multiplexado do Docker para w. Cada pedaço vem com
// um cabeçalho de 8 bytes: [tipo, 0, 0, 0, tamanho em 4 bytes].
//
// Reaproveita o mesmo buffer em todos os pedaços (o io.CopyN criaria 32 KB
// novos a cada linha) — faz diferença em sites e bancos, que logam muito.
func lerStream(w io.Writer, r io.Reader) error {
	br := bufio.NewReaderSize(r, 32<<10) // menos leituras pequenas na conexão
	buf := make([]byte, 32<<10)
	pedaco := &io.LimitedReader{R: br}
	var cabecalho [8]byte
	for {
		if _, err := io.ReadFull(br, cabecalho[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil // acabou
			}
			return err
		}
		pedaco.N = int64(binary.BigEndian.Uint32(cabecalho[4:]))
		if _, err := io.CopyBuffer(w, pedaco, buf); err != nil {
			return err
		}
	}
}
