package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
)

// WhatsAppHandler expõe a gerência da instância de WhatsApp de um tenant (status,
// conectar/QR, desconectar), traduzindo as chamadas HTTP do painel para o
// domain.WhatsAppManager (implementado pelo client da Evolution). O nome da instância
// é derivado do tenant (prefixo + id), nunca informado pelo cliente.
type WhatsAppHandler struct {
	manager domain.WhatsAppManager
	prefix  string
}

func NewWhatsAppHandler(manager domain.WhatsAppManager, prefix string) *WhatsAppHandler {
	return &WhatsAppHandler{manager: manager, prefix: prefix}
}

// Status — GET /api/v1/tenants/:id/whatsapp/status
func (h *WhatsAppHandler) Status(c *gin.Context) {
	const op = "WhatsAppHandler.Status"

	id, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	res, err := h.manager.ConnectionState(domain.WhatsAppInstanceName(h.prefix, id))
	if err != nil {
		httperr.Respond(c, domain.NewInternal(op, "falha ao consultar a Evolution API", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"connected": res.Connected, "state": res.State})
}

// Connect — POST /api/v1/tenants/:id/whatsapp/instance
// Cria a instância (ou conecta, se já existir) e devolve o QR Code para pareamento.
func (h *WhatsAppHandler) Connect(c *gin.Context) {
	const op = "WhatsAppHandler.Connect"

	id, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	res, err := h.manager.CreateInstance(domain.WhatsAppInstanceName(h.prefix, id))
	if err != nil {
		httperr.Respond(c, domain.NewInternal(op, "falha ao criar/conectar a instância", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"qrcode": res.QRCode, "connected": res.Connected, "state": res.State})
}

// Disconnect — DELETE /api/v1/tenants/:id/whatsapp/instance
func (h *WhatsAppHandler) Disconnect(c *gin.Context) {
	const op = "WhatsAppHandler.Disconnect"

	id, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	if err := h.manager.DeleteInstance(domain.WhatsAppInstanceName(h.prefix, id)); err != nil {
		httperr.Respond(c, domain.NewInternal(op, "falha ao desconectar a instância", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
