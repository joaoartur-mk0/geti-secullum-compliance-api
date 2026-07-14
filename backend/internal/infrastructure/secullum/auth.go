package secullum

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// tokenTTL é a validade assumida do token. A Secullum expira em 1h; usamos uma margem
// de segurança para renovar antes do vencimento e evitar 401 por corrida de tempo.
const tokenTTL = 55 * time.Minute

// tokenManager cuida do ciclo de vida do token GLOBAL de acesso à Secullum:
// autentica por credenciais, guarda o token em cache e o renova quando expira.
//
// O refresh é serializado por um mutex: enquanto uma renovação acontece, outras
// requisições aguardam e reaproveitam o mesmo token novo (evita "thundering herd"
// de autenticações simultâneas). Como a renovação ocorre ~1x/hora, o custo é irrelevante.
type tokenManager struct {
	httpClient *http.Client
	authURL    string
	username   string
	password   string

	// staticToken, se preenchido, é usado como token fixo (sem autenticação/refresh).
	// Útil para ambientes de teste que já possuem um token válido.
	staticToken string

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// get devolve um token válido, renovando-o se necessário.
func (tm *tokenManager) get(ctx context.Context) (string, error) {
	if tm.staticToken != "" {
		return tm.staticToken, nil
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Token em cache ainda válido.
	if tm.token != "" && time.Now().Before(tm.expiresAt) {
		return tm.token, nil
	}

	token, err := tm.authenticate(ctx)
	if err != nil {
		return "", fmt.Errorf("secullum auth: %w", err)
	}

	tm.token = token
	tm.expiresAt = time.Now().Add(tokenTTL)
	return token, nil
}

// authenticate faz a chamada de autenticação e devolve o access_token.
func (tm *tokenManager) authenticate(ctx context.Context) (string, error) {
	if tm.authURL == "" || tm.username == "" || tm.password == "" {
		return "", fmt.Errorf("credenciais globais ausentes (authURL/username/password)")
	}

	body, err := json.Marshal(map[string]string{
		"username": tm.username,
		"password": tm.password,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tm.authURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tm.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d na autenticação", resp.StatusCode)
	}

	var authResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", err
	}
	if authResp.AccessToken == "" {
		return "", fmt.Errorf("resposta de autenticação sem access_token")
	}

	return authResp.AccessToken, nil
}
