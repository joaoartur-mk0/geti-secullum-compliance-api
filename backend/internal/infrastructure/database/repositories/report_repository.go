package repositories

import (
	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"
	"encoding/json"

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
	// Transforma o array de inconsistências do domínio em um JSON bruto
	reportJSON, err := json.Marshal(report.Inconsistencies)
	if err != nil {
		return err
	}

	model := &models.Report{
		TenantID:      report.TenantID,
		Report:        datatypes.JSON(reportJSON),
		DataGenerated: report.DataGenerated,
		Date:          report.Date,
	}

	return r.db.Create(model).Error
}
