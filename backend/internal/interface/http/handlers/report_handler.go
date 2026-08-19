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
//
// Devolve só a auditoria MAIS RECENTE de cada dia — o estado atual do painel, sem o
// ruído de reauditorias do mesmo dia (isso fica em GET .../reports/history). Aceita os
// mesmos filtros de período de ?start_date=&end_date=, para consultar semanas/meses
// completos.
func (h *ReportHandler) List(c *gin.Context) {
	const op = "ReportHandler.List"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	start, end, err := reportDateRange(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	reports, err := h.reportRepo.ListLatestByTenant(tenantID, start, end)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"reports": toReportResponses(reports)})
}

// History — GET /api/v1/tenants/:id/reports/history
//
// Devolve o histórico COMPLETO de execuções (inclusive reauditorias do mesmo dia), da
// mais recente para a mais antiga. Mesmos filtros de período que List.
func (h *ReportHandler) History(c *gin.Context) {
	const op = "ReportHandler.History"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	start, end, err := reportDateRange(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	reports, err := h.reportRepo.ListByTenant(tenantID, start, end)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"reports": toReportResponses(reports)})
}

// reportDateRange traduz ?start_date=&end_date= em limites opcionais de Report.Date, para
// consultar e filtrar por período completo (semana, mês, intervalo customizado).
func reportDateRange(c *gin.Context, op string) (start, end *time.Time, err error) {
	if start, err = optionalDateQuery(c, op, "start_date"); err != nil {
		return nil, nil, err
	}
	if end, err = optionalDateQuery(c, op, "end_date"); err != nil {
		return nil, nil, err
	}
	if start != nil && end != nil && end.Before(*start) {
		return nil, nil, domain.NewValidation(op, "intervalo de datas inválido", nil).
			WithDetails("end_date não pode ser anterior a start_date")
	}
	return start, end, nil
}

func toReportResponses(reports []domain.Report) []reportResponse {
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
	return out
}
