package repositories

import (
	"errors"
	"time"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/gorm"
)

type warningRepository struct {
	db *gorm.DB
}

func NewWarningRepository(db *gorm.DB) domain.WarningRepository {
	return &warningRepository{db: db}
}

func toDomainWarning(m models.Warning) domain.Warning {
	return domain.Warning{
		ID:               m.ID,
		TenantID:         m.TenantID,
		OccurrenceID:     m.OccurrenceID,
		CollaboratorID:   m.CollaboratorID,
		CollaboratorName: m.CollaboratorName,
		BranchID:         m.BranchID,
		Body:             m.Body,
		Status:           domain.WarningStatus(m.Status),
		CreatedByUserID:  m.CreatedByUserID,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
		SentAt:           m.SentAt,
		SignedAt:         m.SignedAt,
	}
}

func (r *warningRepository) Create(warning *domain.Warning) error {
	const op = "warningRepository.Create"

	now := time.Now()
	model := &models.Warning{
		TenantID:         warning.TenantID,
		OccurrenceID:     warning.OccurrenceID,
		CollaboratorID:   warning.CollaboratorID,
		CollaboratorName: warning.CollaboratorName,
		BranchID:         warning.BranchID,
		Body:             warning.Body,
		Status:           string(warning.Status),
		CreatedByUserID:  warning.CreatedByUserID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := r.db.Create(model).Error; err != nil {
		return domain.NewInternal(op, "falha ao criar advertência", err)
	}

	warning.ID = model.ID
	warning.CreatedAt = now
	warning.UpdatedAt = now
	return nil
}

// Update altera apenas o conteúdo editável. O status tem caminho próprio (UpdateStatus),
// porque é ele que carrega as regras de transição e os carimbos de data.
func (r *warningRepository) Update(warning *domain.Warning) error {
	const op = "warningRepository.Update"

	res := r.db.Model(&models.Warning{}).Where("id = ?", warning.ID).Updates(map[string]interface{}{
		"body":       warning.Body,
		"branch_id":  warning.BranchID,
		"updated_at": time.Now(),
	})
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao atualizar advertência", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.NewNotFound(op, "advertência não encontrada", nil)
	}
	return nil
}

// UpdateStatus grava o novo status junto com o carimbo do momento em que a etapa
// aconteceu — é esse par (o quê + quando) que dá valor ao registro.
func (r *warningRepository) UpdateStatus(id int, status domain.WarningStatus, at time.Time) error {
	const op = "warningRepository.UpdateStatus"

	updates := map[string]interface{}{
		"status":     string(status),
		"updated_at": at,
	}
	switch status {
	case domain.WarningSent:
		updates["sent_at"] = at
	case domain.WarningSigned:
		updates["signed_at"] = at
	}

	res := r.db.Model(&models.Warning{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao atualizar o status da advertência", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.NewNotFound(op, "advertência não encontrada", nil)
	}
	return nil
}

func (r *warningRepository) GetByID(id int) (*domain.Warning, error) {
	const op = "warningRepository.GetByID"

	var model models.Warning
	err := r.db.First(&model, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "advertência não encontrada", err)
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao buscar advertência", err)
	}

	warning := toDomainWarning(model)
	return &warning, nil
}

func (r *warningRepository) List(filter domain.WarningFilter) ([]domain.Warning, error) {
	const op = "warningRepository.List"

	q := r.db.Where("tenant_id = ?", filter.TenantID)
	if filter.CollaboratorID != nil {
		q = q.Where("collaborator_id = ?", *filter.CollaboratorID)
	}
	if filter.Status != nil {
		q = q.Where("status = ?", string(*filter.Status))
	}

	var rows []models.Warning
	if err := q.Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, domain.NewInternal(op, "falha ao listar advertências", err)
	}

	out := make([]domain.Warning, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainWarning(m))
	}
	return out, nil
}

func (r *warningRepository) Delete(id int) error {
	const op = "warningRepository.Delete"

	res := r.db.Delete(&models.Warning{}, id)
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao excluir advertência", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.NewNotFound(op, "advertência não encontrada", nil)
	}
	return nil
}
