// Package evolution implementa o client HTTP da Evolution API, usado pelo worker de
// notificações para entregar alertas de compliance via WhatsApp aos gestores (staffs)
// de cada tenant. Ver docs/00_Automation_Engineering_Documentation.md, seção 5.4.
package evolution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config são as credenciais GLOBAIS da Evolution API (uma instância de WhatsApp
// atende todos os tenants), vindas de variáveis de ambiente.
type Config struct {
	BaseURL  string // Ex.: https://evolution.exemplo.com.br
	APIKey   string // Header "apikey"
	Instance string // Nome da instância conectada ao WhatsApp
}

// Client implementa domain.NotificationService.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type sendTextRequest struct {
	Number      string      `json:"number"`
	Options     options     `json:"options"`
	TextMessage textMessage `json:"textMessage"`
}

type options struct {
	Delay    int    `json:"delay"`
	Presence string `json:"presence"`
}

type textMessage struct {
	Text string `json:"text"`
}

// SendText envia uma mensagem de texto para o número informado através do endpoint
// `/message/sendText/{instance}` da Evolution API, conforme o contrato de integração
// documentado (delay + presence "composing" para simular digitação humana).
func (c *Client) SendText(number string, message string) error {
	if c.cfg.BaseURL == "" || c.cfg.Instance == "" {
		return fmt.Errorf("evolution: configuração ausente (base URL ou instância)")
	}

	body, err := json.Marshal(sendTextRequest{
		Number: number,
		Options: options{
			Delay:    1500,
			Presence: "composing",
		},
		TextMessage: textMessage{Text: message},
	})
	if err != nil {
		return fmt.Errorf("evolution: falha ao serializar payload: %w", err)
	}

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/message/sendText/" + c.cfg.Instance

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("evolution: falha ao montar requisição: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
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
