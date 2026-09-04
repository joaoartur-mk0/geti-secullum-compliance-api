package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
)

type WarningHandler struct {
	warningRepo    domain.WarningRepository
	collabRepo     domain.CollaboratorRepository
	userTenantRepo domain.UserTenantRepository
}

func NewWarningHandler(
	warningRepo domain.WarningRepository,
	collabRepo domain.CollaboratorRepository,
	userTenantRepo domain.UserTenantRepository,
) *WarningHandler {
	return &WarningHandler{
		warningRepo:    warningRepo,
		collabRepo:     collabRepo,
		userTenantRepo: userTenantRepo,
	}
}

type CreateWarningRequest struct {
	CollaboratorID int    `json:"collaborator_id" binding:"required"` // id na Secullum
	OccurrenceID   *int   `json:"occurrence_id"`
	BranchID       *int   `json:"branch_id"`
	Body           string `json:"body"`
	// Status é opcional: sem ele a advertência nasce como rascunho, que é o fluxo normal
	// (escrever primeiro, entregar depois).
	Status string `json:"status"`
}

type UpdateWarningRequest struct {
	Body     string `json:"body"`
	BranchID *int   `json:"branch_id"`
}

type UpdateWarningStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

type warningResponse struct {
	ID               int     `json:"id"`
	TenantID         int     `json:"tenant_id"`
	OccurrenceID     *int    `json:"occurrence_id"`
	CollaboratorID   int     `json:"collaborator_id"`
	CollaboratorName string  `json:"collaborator_name"`
	BranchID         *int    `json:"branch_id"`
	Body             string  `json:"body"`
	Status           string  `json:"status"`
	CreatedByUserID  *int    `json:"created_by_user_id"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	SentAt           *string `json:"sent_at"`
	SignedAt         *string `json:"signed_at"`
}

func toWarningResponse(w domain.Warning) warningResponse {
	out := warningResponse{
		ID:               w.ID,
		TenantID:         w.TenantID,
		OccurrenceID:     w.OccurrenceID,
		CollaboratorID:   w.CollaboratorID,
		CollaboratorName: w.CollaboratorName,
		BranchID:         w.BranchID,
		Body:             w.Body,
		Status:           string(w.Status),
		CreatedByUserID:  w.CreatedByUserID,
		CreatedAt:        formatTimestamp(w.CreatedAt),
		UpdatedAt:        formatTimestamp(w.UpdatedAt),
	}
	if w.SentAt != nil {
		sent := formatTimestamp(*w.SentAt)
		out.SentAt = &sent
	}
	if w.SignedAt != nil {
		signed := formatTimestamp(*w.SignedAt)
		out.SignedAt = &signed
	}
	return out
}

// loadWarning carrega a advertência e confere o papel mínimo no tenant dono dela (a rota
// usa o id próprio, então o tenant só é conhecido depois de ler o registro). Get passa
// domain.RoleDiretoria (piso de leitura); as escritas (Update, UpdateStatus, Delete)
// passam domain.RoleGestor (docs/08 §5.3).
func (h *WarningHandler) loadWarning(c *gin.Context, op string, min domain.Role) (*domain.Warning, error) {
	id, err := idParam(c, op, "warningId")
	if err != nil {
		return nil, err
	}
	warning, err := h.warningRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if err := requireRole(c, h.userTenantRepo, op, warning.TenantID, min); err != nil {
		return nil, err
	}
	return warning, nil
}

// Create — POST /api/v1/tenants/:id/warnings
func (h *WarningHandler) Create(c *gin.Context) {
	const op = "WarningHandler.Create"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req CreateWarningRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	status := domain.WarningDraft
	if req.Status != "" {
		status = domain.WarningStatus(req.Status)
		if !status.Valid() {
			httperr.Respond(c, invalidWarningStatus(op))
			return
		}
	}

	// O nome vem do espelho local, não do corpo da requisição: uma advertência que cite
	// um nome digitado à mão pode divergir do cadastro justamente quando importa.
	name := ""
	if collab, err := h.collabRepo.GetBySecullumID(tenantID, req.CollaboratorID); err == nil {
		name = collab.Name
	}

	warning := &domain.Warning{
		TenantID:         tenantID,
		OccurrenceID:     req.OccurrenceID,
		CollaboratorID:   req.CollaboratorID,
		CollaboratorName: name,
		BranchID:         req.BranchID,
		Body:             req.Body,
		Status:           status,
		CreatedByUserID:  actorUserID(c),
	}
	if err := h.warningRepo.Create(warning); err != nil {
		httperr.Respond(c, err)
		return
	}

	// Uma advertência criada já como "enviada"/"assinada" precisa do carimbo da etapa.
	if status != domain.WarningDraft {
		now := time.Now()
		if err := h.warningRepo.UpdateStatus(warning.ID, status, now); err != nil {
			httperr.Respond(c, err)
			return
		}
		if status == domain.WarningSent {
			warning.SentAt = &now
		} else {
			warning.SignedAt = &now
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "advertência registrada com sucesso",
		"warning": toWarningResponse(*warning),
	})
}

// List — GET /api/v1/tenants/:id/warnings?collaborator_id=&status=
func (h *WarningHandler) List(c *gin.Context) {
	const op = "WarningHandler.List"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	filter := domain.WarningFilter{TenantID: tenantID}
	if filter.CollaboratorID, err = optionalIntQuery(c, op, "collaborator_id"); err != nil {
		httperr.Respond(c, err)
		return
	}
	if raw := c.Query("status"); raw != "" {
		status := domain.WarningStatus(raw)
		if !status.Valid() {
			httperr.Respond(c, invalidWarningStatus(op))
			return
		}
		filter.Status = &status
	}

	warnings, err := h.warningRepo.List(filter)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]warningResponse, 0, len(warnings))
	// counts alimenta o indicador de "enviadas x confirmadas" do painel sem obrigar o
	// frontend a recontar a lista (que pode vir filtrada).
	counts := map[string]int{
		string(domain.WarningDraft):  0,
		string(domain.WarningSent):   0,
		string(domain.WarningSigned): 0,
	}
	for _, w := range warnings {
		out = append(out, toWarningResponse(w))
		counts[string(w.Status)]++
	}

	c.JSON(http.StatusOK, gin.H{"warnings": out, "total": len(out), "counts": counts})
}

// Get — GET /api/v1/warnings/:warningId
func (h *WarningHandler) Get(c *gin.Context) {
	const op = "WarningHandler.Get"

	warning, err := h.loadWarning(c, op, domain.RoleDiretoria)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"warning": toWarningResponse(*warning)})
}

// Update — PUT /api/v1/warnings/:warningId
//
// Só o rascunho é editável: depois de entregue, o texto da advertência é o que o
// colaborador recebeu, e reescrevê-lo destruiria o valor do registro.
func (h *WarningHandler) Update(c *gin.Context) {
	const op = "WarningHandler.Update"

	warning, err := h.loadWarning(c, op, domain.RoleGestor)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	if warning.Status != domain.WarningDraft {
		httperr.Respond(c, domain.NewConflict(op, "advertência já enviada não pode ser editada", nil).
			WithDetails("apenas advertências em rascunho (draft) podem ter o texto alterado"))
		return
	}

	var req UpdateWarningRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	warning.Body = req.Body
	warning.BranchID = req.BranchID
	if err := h.warningRepo.Update(warning); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "advertência atualizada com sucesso"})
}

// UpdateStatus — PATCH /api/v1/warnings/:warningId/status
func (h *WarningHandler) UpdateStatus(c *gin.Context) {
	const op = "WarningHandler.UpdateStatus"

	warning, err := h.loadWarning(c, op, domain.RoleGestor)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req UpdateWarningStatusRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	next := domain.WarningStatus(req.Status)
	if !next.Valid() {
		httperr.Respond(c, invalidWarningStatus(op))
		return
	}
	if !warning.Status.CanTransitionTo(next) {
		httperr.Respond(c, domain.NewValidation(op, "transição de status não permitida", nil).
			WithDetails("de "+string(warning.Status)+" só é possível seguir o fluxo draft → enviada → assinada"))
		return
	}

	if err := h.warningRepo.UpdateStatus(warning.ID, next, time.Now()); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "status da advertência atualizado", "status": string(next)})
}

// Delete — DELETE /api/v1/warnings/:warningId
func (h *WarningHandler) Delete(c *gin.Context) {
	const op = "WarningHandler.Delete"

	warning, err := h.loadWarning(c, op, domain.RoleGestor)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	if warning.Status != domain.WarningDraft {
		httperr.Respond(c, domain.NewConflict(op, "advertência já enviada não pode ser excluída", nil).
			WithDetails("apenas advertências em rascunho (draft) podem ser excluídas"))
		return
	}
	if err := h.warningRepo.Delete(warning.ID); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "advertência excluída com sucesso"})
}

func invalidWarningStatus(op string) error {
	return domain.NewValidation(op, "status inválido", nil).
		WithDetails("status aceita: draft, enviada, assinada")
}
