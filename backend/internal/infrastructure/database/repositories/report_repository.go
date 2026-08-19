package repositories

import (
	"encoding/json"
	"time"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type reportRepository struct {
	db *gorm.DB
}

func NewReportRepository(db *gorm.DB) domain.ReportRepository {
	return &reportRepository{db: db}
}

func (r *reportRepository) Save(report *domain.Report) error {
	const op = "reportRepository.Save"

	// Transforma o array de inconsistências do domínio em um JSON bruto
	reportJSON, err := json.Marshal(report.Inconsistencies)
	if err != nil {
		return domain.NewInternal(op, "falha ao serializar inconsistências", err)
	}

	model := &models.Report{
		TenantID:      report.TenantID,
		Report:        datatypes.JSON(reportJSON),
		DataGenerated: report.DataGenerated,
		Date:          report.Date,
	}

	if err := r.db.Create(model).Error; err != nil {
		return domain.NewInternal(op, "falha ao salvar relatório", err)
	}
	report.ID = model.ID
	return nil
}

// ListByTenant devolve o histórico completo de relatórios de auditoria de um tenant
// (inclusive reauditorias do mesmo dia), dos mais recentes para os mais antigos.
// start/end (opcionais) filtram por Report.Date, para consultar por período.
func (r *reportRepository) ListByTenant(tenantID int, start, end *time.Time) ([]domain.Report, error) {
	const op = "reportRepository.ListByTenant"

	query := r.db.Where("tenant_id = ?", tenantID)
	if start != nil {
		query = query.Where("date >= ?", *start)
	}
	if end != nil {
		query = query.Where("date <= ?", *end)
	}

	var modelsList []models.Report
	if err := query.Order("date DESC, id DESC").Find(&modelsList).Error; err != nil {
		return nil, domain.NewInternal(op, "falha ao listar relatórios", err)
	}

	return toDomainReports(modelsList), nil
}

// ListLatestByTenant devolve só a execução mais recente de cada dia — o painel padrão de
// auditoria, sem o ruído de reauditorias do mesmo dia (ver ListByTenant para o histórico
// completo). A dedupe é feita em memória, aproveitando que a consulta já vem ordenada
// (date DESC, id DESC): a primeira ocorrência de cada dia já é a mais recente.
func (r *reportRepository) ListLatestByTenant(tenantID int, start, end *time.Time) ([]domain.Report, error) {
	all, err := r.ListByTenant(tenantID, start, end)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(all))
	latest := make([]domain.Report, 0, len(all))
	for _, report := range all {
		day := report.Date.Format("2006-01-02")
		if seen[day] {
			continue
		}
		seen[day] = true
		latest = append(latest, report)
	}
	return latest, nil
}

// toDomainReports mapeia o modelo persistido para o domínio, desserializando o payload
// de inconsistências.
func toDomainReports(modelsList []models.Report) []domain.Report {
	reports := make([]domain.Report, 0, len(modelsList))
	for _, m := range modelsList {
		var inconsistencies []domain.AuditInconsistency
		if len(m.Report) > 0 {
			// Ignora erro de unmarshal: um relatório com payload corrompido ainda
			// aparece na lista (com inconsistências vazias), sem derrubar a consulta.
			_ = json.Unmarshal(m.Report, &inconsistencies)
		}

		reports = append(reports, domain.Report{
			ID:              m.ID,
			TenantID:        m.TenantID,
			Date:            m.Date,
			DataGenerated:   m.DataGenerated,
			Inconsistencies: inconsistencies,
		})
	}
	return reports
}
