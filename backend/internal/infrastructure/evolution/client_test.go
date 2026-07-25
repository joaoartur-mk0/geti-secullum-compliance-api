package evolution

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendText_SucessoEnviaPayloadCorreto(t *testing.T) {
	var recebida sendTextRequest
	var apiKeyRecebida, caminhoRecebido string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caminhoRecebido = r.URL.Path
		apiKeyRecebida = r.Header.Get("apikey")
		_ = json.NewDecoder(r.Body).Decode(&recebida)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "chave-secreta", Instance: "minha-instancia"})

	if err := client.SendText("5531999999999", "Alerta de teste"); err != nil {
		t.Fatalf("esperava sucesso, veio erro: %v", err)
	}

	if !strings.HasSuffix(caminhoRecebido, "/message/sendText/minha-instancia") {
		t.Errorf("caminho inesperado: %s", caminhoRecebido)
	}
	if apiKeyRecebida != "chave-secreta" {
		t.Errorf("apikey inesperada: %s", apiKeyRecebida)
	}
	if recebida.Number != "5531999999999" || recebida.TextMessage.Text != "Alerta de teste" {
		t.Errorf("payload inesperado: %+v", recebida)
	}
	if recebida.Options.Presence != "composing" {
		t.Errorf("esperava presence=composing, veio %q", recebida.Options.Presence)
	}
}

func TestSendText_StatusErroDevolveErro(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid apikey"}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "errada", Instance: "minha-instancia"})

	if err := client.SendText("5531999999999", "Alerta"); err == nil {
		t.Fatal("esperava erro por status HTTP não-2xx")
	}
}

func TestSendText_ConfiguracaoAusenteDevolveErro(t *testing.T) {
	client := NewClient(Config{})
	if err := client.SendText("5531999999999", "Alerta"); err == nil {
		t.Fatal("esperava erro por configuração ausente")
	}
}
