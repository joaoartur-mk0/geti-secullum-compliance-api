package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
)

// EquipmentHandler expõe, somente leitura, o espelho local de equipamentos (relógios de
// ponto) sincronizado da Secullum — ver usecase.SynchronizerService.SyncEquipment. Não há
// escrita: o cadastro de equipamentos vive na Secullum, o painel só consulta.
type EquipmentHandler struct {
	equipRepo domain.EquipmentRepository
}

func NewEquipmentHandler(equipRepo domain.EquipmentRepository) *EquipmentHandler {
	return &EquipmentHandler{equipRepo: equipRepo}
}

type equipmentResponse struct {
	ID         int     `json:"id"`
	SecullumID int     `json:"secullum_id"`
	Descricao  string  `json:"descricao"`
	EnderecoIP *string `json:"endereco_ip"`
}

// List — GET /api/v1/tenants/:id/equipamentos
// Devolve os equipamentos (relógios de ponto) sincronizados do tenant.
func (h *EquipmentHandler) List(c *gin.Context) {
	const op = "EquipmentHandler.List"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	equipments, err := h.equipRepo.GetByTenantID(tenantID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]equipmentResponse, 0, len(equipments))
	for _, e := range equipments {
		out = append(out, equipmentResponse{
			ID:         e.ID,
			SecullumID: e.SecullumID,
			Descricao:  e.Descricao,
			EnderecoIP: e.EnderecoIP,
		})
	}
	c.JSON(http.StatusOK, gin.H{"equipamentos": out, "total": len(out)})
}
