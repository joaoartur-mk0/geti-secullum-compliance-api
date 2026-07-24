package secullum

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// clientIDPontoWeb é o valor fixo exigido pelo autenticador da Secullum para indicar
// que a autenticação é destinada ao Ponto Web (conforme documentação de integração).
const clientIDPontoWeb = "3"

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

	logOnce sync.Once
}

// get devolve um token válido, renovando-o se necessário.
func (tm *tokenManager) get(ctx context.Context) (string, error) {
	if tm.staticToken != "" {
		tm.logDiagnosticsOnce("static")
		return tm.staticToken, nil
	}
	tm.logDiagnosticsOnce("login")

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

// canRefresh indica se o manager consegue obter um token NOVO (modo login).
// Com token estático não há renovação possível — um 401 é definitivo até o
// operador trocar o token no ambiente.
func (tm *tokenManager) canRefresh() bool {
	return tm.staticToken == ""
}

// invalidate descarta o token em cache, forçando nova autenticação no próximo get.
// Usado quando o servidor rejeita (401) um token que localmente ainda parecia válido —
// a Secullum pode invalidar tokens antes do TTL (ex.: novo login na mesma conta).
func (tm *tokenManager) invalidate() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.token = ""
	tm.expiresAt = time.Time{}
}

// authenticate faz a chamada de autenticação (OAuth2 password grant) e devolve o
// access_token, conforme a documentação de Integração Ponto Web da Secullum:
//
//	POST https://autenticador.secullum.com.br/Token
//	Content-Type: application/x-www-form-urlencoded
//	grant_type=password&username=<email>&password=<senha>&client_id=3
func (tm *tokenManager) authenticate(ctx context.Context) (string, error) {
	if tm.authURL == "" || tm.username == "" || tm.password == "" {
		return "", fmt.Errorf("credenciais globais ausentes (authURL/username/password)")
	}

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("username", tm.username)
	form.Set("password", tm.password)
	form.Set("client_id", clientIDPontoWeb)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tm.authURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := tm.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("status %d na autenticação: %s", resp.StatusCode, string(body))
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

// logDiagnosticsOnce imprime, uma única vez, o estado da autenticação — para depurar
// 401 sem vazar o token (loga só tamanho, se parece JWT e a expiração).
func (tm *tokenManager) logDiagnosticsOnce(mode string) {
	tm.logOnce.Do(func() {
		if mode == "static" {
			t := tm.staticToken
			log.Printf("[Secullum][auth] modo=TOKEN ESTÁTICO | len=%d | pareceJWT=%v", len(t), strings.HasPrefix(t, "eyJ"))
			if exp, ok := jwtExpiry(t); ok {
				log.Printf("[Secullum][auth] token exp=%s | agora=%s | EXPIRADO=%v",
					exp.Format(time.RFC3339), time.Now().Format(time.RFC3339), time.Now().After(exp))
			} else {
				log.Printf("[Secullum][auth] não decodifiquei o claim exp (o valor é um JWT válido?)")
			}
			return
		}
		log.Printf("[Secullum][auth] modo=LOGIN | authURL definido=%v | username definido=%v | password definido=%v",
			tm.authURL != "", tm.username != "", tm.password != "")
	})
}

// jwtExpiry decodifica (SEM validar assinatura) o claim exp de um JWT.
func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}
