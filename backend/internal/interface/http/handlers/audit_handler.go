package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
)

type EventPublisher interface {
	Publish(ctx context.Context, queue string, body []byte) error
}

type AuditHandler struct {
	publisher      EventPublisher
	userTenantRepo domain.UserTenantRepository
}

func NewAuditHandler(publisher EventPublisher, userTenantRepo domain.UserTenantRepository) *AuditHandler {
	return &AuditHandler{
		publisher:      publisher,
		userTenantRepo: userTenantRepo,
	}
}

type TriggerRequest struct {
	TenantID int `json:"tenant_id" binding:"required"`
}

func (h *AuditHandler) TriggerAudit(c *gin.Context) {
	const op = "AuditHandler.TriggerAudit"

	var req TriggerRequest

	// 1. Valida o JSON de entrada
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Payload inválido",
			"details": "O campo tenant_id é obrigatório e deve ser um número inteiro.",
		})
		return
	}

	// 1b. O tenant_id vem no corpo (não na rota), então a checagem de acesso é feita
	// aqui em vez de via middleware — mesma regra de isolamento das demais rotas.
	if err := ensureTenantAccess(c, h.userTenantRepo, op, req.TenantID); err != nil {
		httperr.Respond(c, err)
		return
	}

	// 2. Monta o evento (AuditTriggeredEvent) que será enviado para a fila
	eventPayload, err := json.Marshal(map[string]interface{}{
		"tenant_id":    req.TenantID,
		"triggered_by": "manual_http_request", // Pode ser "cron_job" futuramente
	})
	if err != nil {
		log.Printf("[AuditHandler] Falha ao serializar evento do tenant %d: %v", req.TenantID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha interna ao serializar evento"})
		return
	}

	// 3. Publica a mensagem na fila 'audit.trigger' via pool de canais (seguro concorrente).
	//    O contexto da requisição garante que a publicação não bloqueie indefinidamente.
	if err := h.publisher.Publish(c.Request.Context(), "audit.trigger", eventPayload); err != nil {
		log.Printf("[AuditHandler] Falha ao enfileirar auditoria do tenant %d: %v", req.TenantID, err)
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
