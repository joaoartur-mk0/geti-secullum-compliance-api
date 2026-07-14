package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
)

type ReportHandler struct {
	reportRepo domain.ReportRepository
}

func NewReportHandler(repo domain.ReportRepository) *ReportHandler {
	return &ReportHandler{reportRepo: repo}
}

// reportResponse é o item do painel de auditoria (relatório consolidado).
type reportResponse struct {
	ID              int                         `json:"id"`
	TenantID        int                         `json:"tenant_id"`
	Date            string                      `json:"date"`           // dia auditado (YYYY-MM-DD)
	DataGenerated   time.Time                   `json:"data_generated"` // momento da geração
	Total           int                         `json:"total"`          // nº de inconsistências
	Inconsistencies []domain.AuditInconsistency `json:"inconsistencies"`
}

// List — GET /api/v1/tenants/:id/reports
func (h *ReportHandler) List(c *gin.Context) {
	const op = "ReportHandler.List"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	reports, err := h.reportRepo.ListByTenant(tenantID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]reportResponse, 0, len(reports))
	for _, r := range reports {
		out = append(out, reportResponse{
			ID:              r.ID,
			TenantID:        r.TenantID,
			Date:            r.Date.Format("2006-01-02"),
			DataGenerated:   r.DataGenerated,
			Total:           len(r.Inconsistencies),
			Inconsistencies: r.Inconsistencies,
		})
	}
	c.JSON(http.StatusOK, gin.H{"reports": out})
}
