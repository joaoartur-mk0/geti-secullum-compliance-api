package repositories

import (
	"time"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/gorm"
)

type punchRecordRepository struct {
	db *gorm.DB
}

func NewPunchRecordRepository(db *gorm.DB) domain.PunchRecordRepository {
	return &punchRecordRepository{db: db}
}

// SaveAll faz upsert por (tenant_id, collaborator_id, date) — reauditar o mesmo dia
// atualiza o registro em vez de duplicar, mesma convenção do restante do domínio.
func (r *punchRecordRepository) SaveAll(records []domain.PunchRecord) error {
	const op = "punchRecordRepository.SaveAll"
	if len(records) == 0 {
		return nil
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		for i := range records {
			rec := records[i]
			var existing models.PunchRecord
			res := tx.Where("tenant_id = ? AND collaborator_id = ? AND date = ?", rec.TenantID, rec.CollaboratorID, rec.Date).
				Limit(1).Find(&existing)
			if res.Error != nil {
				return res.Error
			}

			if res.RowsAffected == 0 {
				model := models.PunchRecord{
					TenantID:       rec.TenantID,
					CollaboratorID: rec.CollaboratorID,
					Date:           rec.Date,
					EquipamentoID:  rec.EquipamentoID,
					Motivo:         rec.Motivo,
				}
				if err := tx.Create(&model).Error; err != nil {
					return err
				}
				continue
			}

			if err := tx.Model(&existing).Updates(map[string]interface{}{
				"equipamento_id": rec.EquipamentoID,
				"motivo":         rec.Motivo,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.NewInternal(op, "falha ao salvar registros de origem da marcação", err)
	}
	return nil
}

// GetByCollaborator devolve os registros de um colaborador no período [start, end], do
// dia mais antigo ao mais recente.
func (r *punchRecordRepository) GetByCollaborator(tenantID, collaboratorID int, start, end time.Time) ([]domain.PunchRecord, error) {
	const op = "punchRecordRepository.GetByCollaborator"

	var modelsList []models.PunchRecord
	if err := r.db.
		Where("tenant_id = ? AND collaborator_id = ? AND date >= ? AND date <= ?", tenantID, collaboratorID, start, end).
		Order("date").
		Find(&modelsList).Error; err != nil {
		return nil, domain.NewInternal(op, "falha ao buscar registros de origem da marcação", err)
	}

	records := make([]domain.PunchRecord, 0, len(modelsList))
	for _, m := range modelsList {
		records = append(records, domain.PunchRecord{
			ID:             m.ID,
			TenantID:       m.TenantID,
			CollaboratorID: m.CollaboratorID,
			Date:           m.Date,
			EquipamentoID:  m.EquipamentoID,
			Motivo:         m.Motivo,
		})
	}
	return records, nil
}
