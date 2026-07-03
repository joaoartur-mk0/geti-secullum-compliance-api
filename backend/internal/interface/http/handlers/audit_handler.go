package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	amqp "github.com/rabbitmq/amqp091-go"
)

// AuditHandler estrutura que guarda as dependências do controlador
type AuditHandler struct {
	rabbitChannel *amqp.Channel
}

// NewAuditHandler é o construtor do nosso handler
func NewAuditHandler(ch *amqp.Channel) *AuditHandler {
	return &AuditHandler{
		rabbitChannel: ch,
	}
}

// TriggerRequest mapeia o JSON esperado no corpo da requisição
type TriggerRequest struct {
	TenantID int `json:"tenant_id" binding:"required"`
}

// TriggerAudit corresponde ao endpoint POST /api/v1/audit/trigger
func (h *AuditHandler) TriggerAudit(c *gin.Context) {
	var req TriggerRequest

	// 1. Valida o JSON de entrada
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Payload inválido",
			"details": "O campo tenant_id é obrigatório e deve ser um número inteiro.",
		})
		return
	}

	// 2. Monta o evento (AuditTriggeredEvent) que será enviado para a fila
	eventPayload, err := json.Marshal(map[string]interface{}{
		"tenant_id":    req.TenantID,
		"triggered_by": "manual_http_request", // Pode ser "cron_job" futuramente
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha interna ao serializar evento"})
		return
	}

	// 3. Publica a mensagem na fila 'audit.trigger' do RabbitMQ
	err = h.rabbitChannel.Publish(
		"",              // exchange (padrão)
		"audit.trigger", // routing key (nome da fila que criamos no main.go)
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        eventPayload,
		},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enfileirar requisição de auditoria no Broker"})
		return
	}

	// 4. Retorna HTTP 202 (Accepted) - Indica que a requisição foi aceita para processamento assíncrono
	c.JSON(http.StatusAccepted, gin.H{
		"message":   "Auditoria enfileirada com sucesso",
		"tenant_id": req.TenantID,
		"status":    "processing",
	})
}
