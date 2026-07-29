package evolution

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendText_SucessoEnviaPayloadV2(t *testing.T) {
	var recebida sendTextRequest
	var apiKeyRecebida, caminhoRecebido string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caminhoRecebido = r.URL.Path
		apiKeyRecebida = r.Header.Get("apikey")
		_ = json.NewDecoder(r.Body).Decode(&recebida)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "chave-secreta"})

	if err := client.SendText("tenant-3", "5531999999999", "Alerta de teste"); err != nil {
		t.Fatalf("esperava sucesso, veio erro: %v", err)
	}

	if !strings.HasSuffix(caminhoRecebido, "/message/sendText/tenant-3") {
		t.Errorf("caminho inesperado: %s", caminhoRecebido)
	}
	if apiKeyRecebida != "chave-secreta" {
		t.Errorf("apikey inesperada: %s", apiKeyRecebida)
	}
	// Formato v2 achatado: number/text no topo (sem options/textMessage aninhados).
	if recebida.Number != "5531999999999" || recebida.Text != "Alerta de teste" {
		t.Errorf("payload inesperado: %+v", recebida)
	}
}

func TestSendText_StatusErroDevolveErro(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid apikey"}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "errada"})

	if err := client.SendText("tenant-3", "5531999999999", "Alerta"); err == nil {
		t.Fatal("esperava erro por status HTTP não-2xx")
	}
}

func TestSendText_InstanciaAusenteDevolveErro(t *testing.T) {
	client := NewClient(Config{BaseURL: "http://exemplo", APIKey: "k"})
	if err := client.SendText("", "5531999999999", "Alerta"); err == nil {
		t.Fatal("esperava erro por instância ausente")
	}
}

func TestSendText_BaseURLAusenteDevolveErro(t *testing.T) {
	client := NewClient(Config{})
	if err := client.SendText("tenant-3", "5531999999999", "Alerta"); err == nil {
		t.Fatal("esperava erro por base URL ausente")
	}
}

func TestCreateInstance_DevolveQRCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/instance/create" {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"qrcode":{"base64":"data:image/png;base64,QRDATA"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "k"})
	res, err := client.CreateInstance("tenant-3")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res.QRCode != "data:image/png;base64,QRDATA" {
		t.Errorf("qrcode inesperado: %q", res.QRCode)
	}
	if res.Connected {
		t.Error("não deveria estar conectada com QR pendente")
	}
}

func TestCreateInstance_JaExisteCaiParaConnect(t *testing.T) {
	var connectChamado bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/instance/create":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"instance already exists"}`))
		case strings.HasPrefix(r.URL.Path, "/instance/connect/"):
			connectChamado = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"base64":"data:image/png;base64,QR2"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "k"})
	res, err := client.CreateInstance("tenant-3")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !connectChamado {
		t.Error("esperava fallback para /instance/connect quando a instância já existe")
	}
	if res.QRCode != "data:image/png;base64,QR2" {
		t.Errorf("qrcode do connect inesperado: %q", res.QRCode)
	}
}

func TestConnectionState_Conectado(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"instance":{"state":"open"}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "k"})
	res, err := client.ConnectionState("tenant-3")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !res.Connected || res.State != "open" {
		t.Errorf("esperava conectado/open, veio %+v", res)
	}
}

func TestConnectionState_404EhDesconectadoSemErro(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "k"})
	res, err := client.ConnectionState("tenant-3")
	if err != nil {
		t.Fatalf("404 não deveria ser erro: %v", err)
	}
	if res.Connected {
		t.Error("instância inexistente (404) deveria vir desconectada")
	}
}

func TestDeleteInstance_404EhIdempotente(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "k"})
	if err := client.DeleteInstance("tenant-3"); err != nil {
		t.Fatalf("deletar instância inexistente deveria ser sucesso (idempotente): %v", err)
	}
}

func TestIsConnected(t *testing.T) {
	for _, s := range []string{"open", "OPEN", " connected ", "online"} {
		if !isConnected(s) {
			t.Errorf("%q deveria contar como conectado", s)
		}
	}
	for _, s := range []string{"close", "connecting", ""} {
		if isConnected(s) {
			t.Errorf("%q não deveria contar como conectado", s)
		}
	}
}
