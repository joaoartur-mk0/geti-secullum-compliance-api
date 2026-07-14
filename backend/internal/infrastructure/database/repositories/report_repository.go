package repositories

import (
	"encoding/json"

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

// ListByTenant devolve os relatórios de auditoria de um tenant, dos mais recentes
// para os mais antigos, para alimentar o painel de consulta.
func (r *reportRepository) ListByTenant(tenantID int) ([]domain.Report, error) {
	const op = "reportRepository.ListByTenant"

	var modelsList []models.Report
	err := r.db.Where("tenant_id = ?", tenantID).Order("date DESC, id DESC").Find(&modelsList).Error
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao listar relatórios", err)
	}

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
	return reports, nil
}
