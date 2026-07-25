package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
)

type CollaboratorHandler struct {
	collabRepo domain.CollaboratorRepository
}

func NewCollaboratorHandler(repo domain.CollaboratorRepository) *CollaboratorHandler {
	return &CollaboratorHandler{collabRepo: repo}
}

// collaboratorResponse é a identidade mínima do colaborador exposta ao painel.
// Não inclui CPF/celular (dado pessoal desnecessário para os indicadores) nem as
// jornadas (usadas só internamente pelo motor de auditoria).
type collaboratorResponse struct {
	ID         int    `json:"id"`
	SecullumID int    `json:"secullum_id"`
	Name       string `json:"name"`
}

// List — GET /api/v1/tenants/:id/collaborators
// Devolve o espelho local de colaboradores sincronizados do tenant (via fila
// tenant.provisioning) e o total, para o painel de Indicadores mostrar quantos
// funcionários estão sob auditoria.
func (h *CollaboratorHandler) List(c *gin.Context) {
	const op = "CollaboratorHandler.List"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	collaborators, err := h.collabRepo.GetByTenantID(tenantID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]collaboratorResponse, 0, len(collaborators))
	for _, col := range collaborators {
		out = append(out, collaboratorResponse{
			ID:         col.ID,
			SecullumID: col.SecullumID,
			Name:       col.Name,
		})
	}
	c.JSON(http.StatusOK, gin.H{"collaborators": out, "total": len(out)})
}
