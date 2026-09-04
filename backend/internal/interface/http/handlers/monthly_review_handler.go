package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
	"backend/internal/usecase"
)

// MonthlyReviewHandler expõe a Feature 3 (revisão mensal): o painel de "o que falta para
// considerar esta competência encerrada" e o ato explícito de encerrar/reabrir — ver
// docs/documento-funcional-compliance.md §7.5.
type MonthlyReviewHandler struct {
	service           *usecase.MonthlyReviewService
	monthlyReviewRepo domain.MonthlyReviewRepository
}

func NewMonthlyReviewHandler(service *usecase.MonthlyReviewService, monthlyReviewRepo domain.MonthlyReviewRepository) *MonthlyReviewHandler {
	return &MonthlyReviewHandler{service: service, monthlyReviewRepo: monthlyReviewRepo}
}

type monthlyReviewResponse struct {
	Competencia     string  `json:"competencia"`
	Status          string  `json:"status"`
	OpenOccurrences int     `json:"open_occurrences"`
	PendingRecheck  int     `json:"pending_recheck"`
	OpenOperational int     `json:"open_operational"`
	DaysWithoutScan int     `json:"days_without_scan"`
	PayrollDone     bool    `json:"payroll_done"`
	OffsetsDone     bool    `json:"offsets_done"`
	Ready           bool    `json:"ready"`
	ClosedAt        *string `json:"closed_at"`
	ClosedByUserID  *int    `json:"closed_by_user_id"`
}

func toMonthlyReviewResponse(c domain.MonthlyReviewConditions) monthlyReviewResponse {
	out := monthlyReviewResponse{
		Competencia:     c.Competencia,
		Status:          string(c.Status),
		OpenOccurrences: c.OpenOccurrences,
		PendingRecheck:  c.PendingRecheck,
		OpenOperational: c.OpenOperational,
		DaysWithoutScan: c.DaysWithoutScan,
		PayrollDone:     c.PayrollDone,
		OffsetsDone:     c.OffsetsDone,
		Ready:           c.Ready(),
		ClosedByUserID:  c.ClosedByUserID,
	}
	if c.ClosedAt != nil {
		s := formatTimestamp(*c.ClosedAt)
		out.ClosedAt = &s
	}
	return out
}

func competenciaQuery(c *gin.Context, op string) (string, error) {
	competencia := c.Query("competencia")
	if competencia == "" {
		return "", domain.NewValidation(op, "competência obrigatória", nil).
			WithDetails("informe ?competencia=YYYY-MM")
	}
	return competencia, nil
}

// Get — GET /api/v1/tenants/:id/monthly-reviews?competencia=YYYY-MM
// Devolve as seis condições — quatro automáticas, recalculadas agora, e duas manuais.
func (h *MonthlyReviewHandler) Get(c *gin.Context) {
	const op = "MonthlyReviewHandler.Get"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	competencia, err := competenciaQuery(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	conditions, err := h.service.Conditions(tenantID, competencia)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, toMonthlyReviewResponse(conditions))
}

// UpdateManualConditions — PATCH /api/v1/tenants/:id/monthly-reviews?competencia=YYYY-MM
// Corpo: {"payroll_done": bool, "offsets_done": bool} — ambos opcionais, só o(s)
// informado(s) muda(m). Bloqueado (conflito) se a competência já estiver encerrada.
func (h *MonthlyReviewHandler) UpdateManualConditions(c *gin.Context) {
	const op = "MonthlyReviewHandler.UpdateManualConditions"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	competencia, err := competenciaQuery(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req struct {
		PayrollDone *bool `json:"payroll_done"`
		OffsetsDone *bool `json:"offsets_done"`
	}
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	review, err := h.monthlyReviewRepo.SetManualConditions(tenantID, competencia, req.PayrollDone, req.OffsetsDone)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"competencia":  review.Competencia,
		"payroll_done": review.PayrollDone,
		"offsets_done": review.OffsetsDone,
	})
}

// Close — POST /api/v1/tenants/:id/monthly-reviews/close?competencia=YYYY-MM
// O ato explícito de encerrar. Bloqueia com o detalhe exato do que falta — nunca um erro
// genérico — quando qualquer uma das seis condições não estiver satisfeita.
func (h *MonthlyReviewHandler) Close(c *gin.Context) {
	const op = "MonthlyReviewHandler.Close"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	competencia, err := competenciaQuery(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	uid := actorUserID(c)
	if uid == nil {
		httperr.Respond(c, domain.NewForbidden(op, "usuário não identificado", nil))
		return
	}

	review, err := h.service.Close(tenantID, competencia, *uid)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "competência encerrada com sucesso",
		"competencia": review.Competencia,
		"status":      string(review.Status),
	})
}

// Export — GET /api/v1/tenants/:id/monthly-reviews/export?competencia=YYYY-MM
//
// O relatório consolidado exportável — a evidência do ciclo (§7.5 regra 4). Só existe
// para competência já ENCERRADA; devolve 409 se a competência ainda estiver aberta.
func (h *MonthlyReviewHandler) Export(c *gin.Context) {
	const op = "MonthlyReviewHandler.Export"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	competencia, err := competenciaQuery(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	export, err := h.service.Export(tenantID, competencia)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	occurrences := make([]gin.H, 0, len(export.Occurrences))
	for _, occ := range export.Occurrences {
		occurrences = append(occurrences, gin.H{
			"id":                occ.ID,
			"collaborator_id":   occ.CollaboratorID,
			"collaborator_name": occ.CollaboratorName,
			"date":              occ.Date.Format("2006-01-02"),
			"type":              occ.Type,
			"severity":          string(occ.Severity),
			"state":             string(occ.State),
			"description":       occ.Description,
		})
	}

	var closedAt *string
	if export.ClosedAt != nil {
		s := formatTimestamp(*export.ClosedAt)
		closedAt = &s
	}

	c.JSON(http.StatusOK, gin.H{
		"competencia":       export.Competencia,
		"closed_at":         closedAt,
		"closed_by_user_id": export.ClosedByUserID,
		"total_occurrences": export.TotalOccurrences,
		"by_state":          export.ByState,
		"by_severity":       export.BySeverity,
		"by_type":           export.ByType,
		"occurrences":       occurrences,
	})
}

// Reopen — POST /api/v1/tenants/:id/monthly-reviews/reopen?competencia=YYYY-MM
// Corpo: {"reason": "..."} — motivo obrigatório, grava evento na trilha, nunca sobrescreve
// o registro de encerramento original.
//
// Restrição de PAPEL a quem pode reabrir ainda não está implementada — é ponto em aberto
// #2 de docs/documento-funcional-compliance.md, que depende da Feature 6 (papéis) definir
// qual nível mínimo. Por ora, qualquer usuário com vínculo no tenant pode reabrir.
func (h *MonthlyReviewHandler) Reopen(c *gin.Context) {
	const op = "MonthlyReviewHandler.Reopen"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	competencia, err := competenciaQuery(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	uid := actorUserID(c)
	if uid == nil {
		httperr.Respond(c, domain.NewForbidden(op, "usuário não identificado", nil))
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		httperr.Respond(c, domain.NewValidation(op, "motivo obrigatório", nil).
			WithDetails("reabrir uma competência encerrada exige motivo registrado"))
		return
	}

	review, err := h.monthlyReviewRepo.Reopen(tenantID, competencia, *uid, reason)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "competência reaberta com sucesso",
		"competencia": review.Competencia,
		"status":      string(review.Status),
	})
}
