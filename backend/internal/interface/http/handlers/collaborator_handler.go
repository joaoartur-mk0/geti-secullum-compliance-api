package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
	"backend/internal/usecase"
)

type CollaboratorHandler struct {
	collabRepo    domain.CollaboratorRepository
	tenantRepo    domain.TenantRepository
	branchResolve *usecase.BranchResolverService
	secullumSvc   domain.SecullumService
}

func NewCollaboratorHandler(
	repo domain.CollaboratorRepository,
	tenantRepo domain.TenantRepository,
	branchResolve *usecase.BranchResolverService,
	secullumSvc domain.SecullumService,
) *CollaboratorHandler {
	return &CollaboratorHandler{
		collabRepo:    repo,
		tenantRepo:    tenantRepo,
		branchResolve: branchResolve,
		secullumSvc:   secullumSvc,
	}
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

// Prefill — GET /api/v1/tenants/:id/collaborators/:secullumId/prefill?date=YYYY-MM-DD
//
// Entrega, num único payload, o que a tela de colaborador (e o formulário de advertência)
// precisam preencher sozinhas: o horário fixo cadastrado na Secullum e a filial em que a
// pessoa está lotada, com o gestor responsável.
//
// O horário sai do espelho local sincronizado — nenhuma chamada externa. A `date` é
// opcional e serve só para tentar a resolução de filial pelo aparelho da batida daquele
// dia; sem ela (ou se a Secullum falhar) a filial vem do nº de folha.
func (h *CollaboratorHandler) Prefill(c *gin.Context) {
	const op = "CollaboratorHandler.Prefill"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	secullumID, err := idParam(c, op, "secullumId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	collab, err := h.collabRepo.GetBySecullumID(tenantID, secullumID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	date, err := optionalDateQuery(c, op, "date")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	// Só vale chamar a Secullum se houver aparelho cadastrado; sem nenhum, a filial viria
	// do nº de folha de qualquer forma.
	var punch *domain.DailyPunch
	if date != nil && h.tenantRepo != nil && h.branchResolve != nil && h.branchResolve.HasDevices(tenantID) {
		if tenant, err := h.tenantRepo.GetByID(tenantID); err == nil {
			if p, ok := punchesForBranchResolution(h.secullumSvc, tenant, *date)[secullumID]; ok {
				punch = &p
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"collaborator": gin.H{
			"id":           collab.ID,
			"secullum_id":  collab.SecullumID,
			"name":         collab.Name,
			"numero_folha": collab.NumeroFolha,
		},
		"horario_fixo": toFixedSchedule(collab.Schedules),
		"filial":       resolveBranchFor(h.branchResolve, tenantID, collab, punch),
	})
}

// optionalDateQuery lê um parâmetro de data "YYYY-MM-DD" da query string. Ausente devolve
// nil (sem filtro); presente e malformado é erro de validação, nunca uma data silenciosa.
func optionalDateQuery(c *gin.Context, op, name string) (*time.Time, error) {
	raw := c.Query(name)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, domain.NewValidation(op, "data inválida", err).
			WithDetails(name + " deve estar no formato YYYY-MM-DD")
	}
	return &parsed, nil
}
