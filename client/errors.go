package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// APIError é um erro devolvido pelo daemon (status HTTP + mensagem dele).
// Os métodos embrulham ele com a ação que falhou; para ler:
//
//	var apiErr *client.APIError
//	if errors.As(err, &apiErr) { ... apiErr.StatusCode ... }
type APIError struct {
	StatusCode int    `json:"-"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("status %d", e.StatusCode)
	}
	return fmt.Sprintf("status %d: %s", e.StatusCode, e.Message)
}

// IsNotFound diz se o erro é "não existe" (404): container, imagem, exec...
func IsNotFound(err error) bool { return hasStatus(err, http.StatusNotFound) }

// IsConflict diz se o erro é de conflito (409): ex.: nome de container já em
// uso, ou apagar algo que está rodando.
func IsConflict(err error) bool { return hasStatus(err, http.StatusConflict) }

func hasStatus(err error, status int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

// newAPIError lê a mensagem de erro do daemon (até 64 KB) e monta o erro
// "erro ao <action>: status N: mensagem".
func newAPIError(resp *http.Response, action string) error {
	apiErr := &APIError{StatusCode: resp.StatusCode}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(apiErr)
	return fmt.Errorf("erro ao %s: %w", action, apiErr)
}

// trimSpace é strings.TrimSpace (atalho usado pelas validações).
func trimSpace(s string) string { return strings.TrimSpace(s) }
