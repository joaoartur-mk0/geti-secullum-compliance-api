// Package evolution implementa o client HTTP da Evolution API, usado para (1) entregar
// alertas de compliance via WhatsApp aos gestores (staffs) e (2) gerenciar a instância
// de WhatsApp de cada tenant (criar/conectar/status/desconectar). Ver
// docs/00_Automation_Engineering_Documentation.md, seção 5.4.
package evolution

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"backend/internal/domain"
)

// Config são as credenciais GLOBAIS da Evolution API (mesma base URL e apikey para
// todos os tenants). O nome da instância NÃO fica aqui: é por-tenant e informado em
// cada chamada (ver domain.WhatsAppInstanceName).
type Config struct {
	BaseURL string // Ex.: https://evolution.exemplo.com.br
	APIKey  string // Header "apikey"
}

// Client implementa domain.NotificationService e domain.WhatsAppManager.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		// Criar/conectar instância pode demorar (a Evolution gera o QR sob demanda),
		// por isso um timeout mais folgado que o de um simples envio.
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// do monta e executa uma requisição já com os cabeçalhos de autenticação. body nil =
// requisição sem corpo (GET/DELETE).
func (c *Client) do(method, path string, body any) (*http.Response, error) {
	if strings.TrimSpace(c.cfg.BaseURL) == "" {
		return nil, fmt.Errorf("evolution: base URL não configurada")
	}

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("evolution: falha ao serializar payload: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, strings.TrimRight(c.cfg.BaseURL, "/")+path, reader)
	if err != nil {
		return nil, fmt.Errorf("evolution: falha ao montar requisição: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("apikey", c.cfg.APIKey)

	return c.httpClient.Do(req)
}

// --- Envio de mensagem (domain.NotificationService) ---

// sendTextRequest usa o formato "achatado" da Evolution API v2 (number/text/delay no
// topo), o mesmo já validado em produção no servidor da Geti.
type sendTextRequest struct {
	Number      string `json:"number"`
	Text        string `json:"text"`
	Delay       int    `json:"delay"`
	LinkPreview bool   `json:"linkPreview"`
}

// SendText envia uma mensagem de texto para `number` através da `instance` informada,
// via endpoint `/message/sendText/{instance}`.
func (c *Client) SendText(instance string, number string, message string) error {
	if strings.TrimSpace(instance) == "" {
		return fmt.Errorf("evolution: instância não informada")
	}

	resp, err := c.do(http.MethodPost, "/message/sendText/"+instance, sendTextRequest{
		Number:      number,
		Text:        message,
		Delay:       1200,
		LinkPreview: false,
	})
	if err != nil {
		return fmt.Errorf("evolution: falha na requisição HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("evolution: status %d ao enviar mensagem: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// --- Gerência de instância (domain.WhatsAppManager) ---

type createInstanceRequest struct {
	InstanceName string `json:"instanceName"`
	Token        string `json:"token"`
	QRCode       bool   `json:"qrcode"`
	Integration  string `json:"integration"`
}

// evoInstancePayload cobre, de forma tolerante, as várias formas em que a Evolution
// devolve QR Code e estado da instância entre os endpoints create/connect/state. Campos
// ausentes ficam zerados — nenhum deles é obrigatório em toda resposta.
type evoInstancePayload struct {
	Base64 string `json:"base64"` // /instance/connect: QR no topo
	QRCode struct {
		Base64 string `json:"base64"` // /instance/create: QR aninhado
	} `json:"qrcode"`
	Instance struct {
		State  string `json:"state"`
		Status string `json:"status"`
	} `json:"instance"`
	State  string `json:"state"`
	Status string `json:"status"`
}

// toDomain converte o corpo bruto num domain.WhatsAppInstance, escolhendo o QR e o
// estado de onde quer que a Evolution os tenha colocado.
func (p evoInstancePayload) toDomain() *domain.WhatsAppInstance {
	qr := p.QRCode.Base64
	if qr == "" {
		qr = p.Base64
	}
	state := firstNonEmpty(p.Instance.State, p.Instance.Status, p.State, p.Status)
	return &domain.WhatsAppInstance{
		QRCode:    qr,
		Connected: isConnected(state),
		State:     normalizeState(state),
	}
}

// CreateInstance cria a instância e devolve o QR Code para pareamento. Se ela já
// existir (403/409), cai para ConnectInstance, que traz o QR/estado da instância atual
// — replicando o fluxo idempotente usado no Eliza.
func (c *Client) CreateInstance(instance string) (*domain.WhatsAppInstance, error) {
	resp, err := c.do(http.MethodPost, "/instance/create", createInstanceRequest{
		InstanceName: instance,
		Token:        randomToken(),
		QRCode:       true,
		Integration:  "WHATSAPP-BAILEYS",
	})
	if err != nil {
		return nil, fmt.Errorf("evolution: falha ao criar instância: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	// Já existe: busca o QR/estado da instância existente.
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusConflict {
		return c.ConnectInstance(instance)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("evolution: status %d ao criar instância: %s", resp.StatusCode, string(body))
	}

	var p evoInstancePayload
	_ = json.Unmarshal(body, &p)
	res := p.toDomain()

	// Criada, mas sem QR no corpo e ainda não conectada: pede o QR separadamente.
	if res.QRCode == "" && !res.Connected {
		return c.ConnectInstance(instance)
	}
	return res, nil
}

// ConnectInstance pede o QR Code (ou o estado atual) da instância via
// `/instance/connect/{instance}`.
func (c *Client) ConnectInstance(instance string) (*domain.WhatsAppInstance, error) {
	resp, err := c.do(http.MethodGet, "/instance/connect/"+instance, nil)
	if err != nil {
		return nil, fmt.Errorf("evolution: falha ao conectar instância: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("evolution: status %d ao conectar instância: %s", resp.StatusCode, string(body))
	}

	var p evoInstancePayload
	_ = json.Unmarshal(body, &p)
	return p.toDomain(), nil
}

// ConnectionState consulta o estado da conexão via `/instance/connectionState/{instance}`.
// 404 significa instância inexistente/desconectada (não é erro).
func (c *Client) ConnectionState(instance string) (*domain.WhatsAppInstance, error) {
	resp, err := c.do(http.MethodGet, "/instance/connectionState/"+instance, nil)
	if err != nil {
		return nil, fmt.Errorf("evolution: falha ao consultar estado: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &domain.WhatsAppInstance{Connected: false, State: "close"}, nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("evolution: status %d ao consultar estado: %s", resp.StatusCode, string(body))
	}

	var p evoInstancePayload
	_ = json.Unmarshal(body, &p)
	return p.toDomain(), nil
}

// DeleteInstance remove a instância via `/instance/delete/{instance}`. 404 é tratado
// como sucesso (idempotente: já não existe).
func (c *Client) DeleteInstance(instance string) error {
	resp, err := c.do(http.MethodDelete, "/instance/delete/"+instance, nil)
	if err != nil {
		return fmt.Errorf("evolution: falha ao deletar instância: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("evolution: status %d ao deletar instância: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// --- Helpers ---

// randomToken gera um token aleatório para a criação da instância (campo exigido pela
// Evolution como chave de segurança da instância).
func randomToken() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback improvável: rand.Read raramente falha. Usa um valor fixo para não
		// abortar a criação (o token não é sensível à segurança do nosso lado).
		return "secullumtoken"
	}
	return hex.EncodeToString(b)
}

func normalizeState(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// isConnected classifica os estados que a Evolution usa para "instância pareada".
func isConnected(state string) bool {
	switch normalizeState(state) {
	case "open", "connected", "online":
		return true
	default:
		return false
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
