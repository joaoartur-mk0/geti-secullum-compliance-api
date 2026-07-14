package secullum

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenManager_StaticToken(t *testing.T) {
	tm := &tokenManager{staticToken: "fixo-123"}
	got, err := tm.get(context.Background())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != "fixo-123" {
		t.Errorf("token = %q, quer 'fixo-123'", got)
	}
}

func TestTokenManager_AutenticaECacheia(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Method != http.MethodPost {
			t.Errorf("método = %s, quer POST", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-abc"})
	}))
	defer srv.Close()

	tm := &tokenManager{
		httpClient: srv.Client(),
		authURL:    srv.URL,
		username:   "elmer",
		password:   "123456",
	}

	// 1ª chamada autentica.
	got, err := tm.get(context.Background())
	if err != nil || got != "tok-abc" {
		t.Fatalf("get() = (%q, %v)", got, err)
	}
	// 2ª chamada deve vir do cache (sem nova autenticação).
	if _, err := tm.get(context.Background()); err != nil {
		t.Fatalf("get() 2: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("autenticou %d vezes, esperava 1 (cache)", n)
	}

	// Forçando expiração, deve renovar.
	tm.expiresAt = time.Now().Add(-time.Minute)
	if _, err := tm.get(context.Background()); err != nil {
		t.Fatalf("get() 3: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("após expiração autenticou %d vezes, esperava 2", n)
	}
}

func TestTokenManager_CredenciaisAusentes(t *testing.T) {
	tm := &tokenManager{httpClient: http.DefaultClient} // sem authURL/user/pass
	if _, err := tm.get(context.Background()); err == nil {
		t.Fatalf("esperava erro por credenciais ausentes")
	}
}
