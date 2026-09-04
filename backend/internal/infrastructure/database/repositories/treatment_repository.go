package repositories

import (
	"errors"
	"time"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/gorm"
)

type treatmentRepository struct {
	db *gorm.DB
}

func NewTreatmentRepository(db *gorm.DB) domain.TreatmentRepository {
	return &treatmentRepository{db: db}
}

func toDomainTreatment(m models.Treatment) domain.Treatment {
	t := domain.Treatment{
		ID:             m.ID,
		OccurrenceID:   m.OccurrenceID,
		TenantID:       m.TenantID,
		Justification:  m.Justification,
		ActorUserID:    m.ActorUserID,
		CreatedAt:      m.CreatedAt,
		UndoneAt:       m.UndoneAt,
		UndoneByUserID: m.UndoneByUserID,
	}
	for _, a := range m.Attachments {
		t.Attachments = append(t.Attachments, toDomainAttachment(a))
	}
	return t
}

func toDomainAttachment(m models.Attachment) domain.Attachment {
	return domain.Attachment{
		ID:          m.ID,
		TreatmentID: m.TreatmentID,
		TenantID:    m.TenantID,
		FileName:    m.FileName,
		ContentType: m.ContentType,
		SizeBytes:   m.SizeBytes,
		Data:        m.Data,
		CreatedAt:   m.CreatedAt,
	}
}

// Treat grava a tratativa, seus anexos e transiciona a ocorrência para `tratada`, tudo em
// uma transação — mesmo formato de occurrenceRepository.Ignore. Bloqueia (NewConflict) se
// a ocorrência já tiver um desfecho humano (tratada ou ignorada): tratativa não sobrescreve
// desfecho existente, o usuário precisa desfazer primeiro.
func (r *treatmentRepository) Treat(occurrenceID int, justification string, attachments []domain.Attachment, actorUserID int) (*domain.Treatment, error) {
	const op = "treatmentRepository.Treat"

	var result models.Treatment
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var current models.Occurrence
		if err := tx.First(&current, occurrenceID).Error; err != nil {
			return err
		}
		if domain.OccurrenceState(current.State).Sticky() {
			return domain.NewConflict(op, "ocorrência já tem desfecho", nil).
				WithDetails("desfaça a tratativa/ignorar atual antes de registrar uma nova")
		}

		now := time.Now()
		treatment := models.Treatment{
			OccurrenceID:  occurrenceID,
			TenantID:      current.TenantID,
			Justification: justification,
			ActorUserID:   actorUserID,
			CreatedAt:     now,
		}
		for i := range attachments {
			treatment.Attachments = append(treatment.Attachments, models.Attachment{
				TenantID:    current.TenantID,
				FileName:    attachments[i].FileName,
				ContentType: attachments[i].ContentType,
				SizeBytes:   attachments[i].SizeBytes,
				Data:        attachments[i].Data,
				CreatedAt:   now,
			})
		}
		if err := tx.Create(&treatment).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{
			"state":       string(domain.OccurrenceTreated),
			"resolved_at": now,
		}
		if err := tx.Model(&models.Occurrence{}).Where("id = ?", occurrenceID).Updates(updates).Error; err != nil {
			return err
		}

		if err := tx.Create(&models.OccurrenceEvent{
			OccurrenceID: occurrenceID,
			TenantID:     current.TenantID,
			Type:         string(domain.EventTreated),
			FromState:    current.State,
			ToState:      string(domain.OccurrenceTreated),
			Reason:       justification,
			ActorUserID:  &actorUserID,
			CreatedAt:    now,
		}).Error; err != nil {
			return err
		}

		result = treatment
		return nil
	})

	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "ocorrência não encontrada", err)
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao registrar tratativa", err)
	}

	treatment := toDomainTreatment(result)
	return &treatment, nil
}

func (r *treatmentRepository) GetByID(treatmentID int) (*domain.Treatment, error) {
	const op = "treatmentRepository.GetByID"

	var m models.Treatment
	err := r.db.Preload("Attachments").First(&m, treatmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "tratativa não encontrada", err)
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao buscar tratativa", err)
	}

	treatment := toDomainTreatment(m)
	return &treatment, nil
}

// Undo marca a tratativa como desfeita e devolve a ocorrência para `aberta`. O registro
// original da tratativa permanece — só ganha UndoneAt/UndoneByUserID — e um evento novo
// documenta a reversão na trilha.
func (r *treatmentRepository) Undo(treatmentID, actorUserID int) error {
	const op = "treatmentRepository.Undo"

	err := r.db.Transaction(func(tx *gorm.DB) error {
		var treatment models.Treatment
		if err := tx.First(&treatment, treatmentID).Error; err != nil {
			return err
		}
		if treatment.UndoneAt != nil {
			return nil // já desfeita: idempotente
		}

		var occurrence models.Occurrence
		if err := tx.First(&occurrence, treatment.OccurrenceID).Error; err != nil {
			return err
		}

		now := time.Now()
		if err := tx.Model(&treatment).Updates(map[string]interface{}{
			"undone_at":         now,
			"undone_by_user_id": actorUserID,
		}).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.Occurrence{}).Where("id = ?", treatment.OccurrenceID).Updates(map[string]interface{}{
			"state":       string(domain.OccurrenceOpen),
			"resolved_at": nil,
		}).Error; err != nil {
			return err
		}

		return tx.Create(&models.OccurrenceEvent{
			OccurrenceID: treatment.OccurrenceID,
			TenantID:     occurrence.TenantID,
			Type:         string(domain.EventTreatmentUndone),
			FromState:    occurrence.State,
			ToState:      string(domain.OccurrenceOpen),
			ActorUserID:  &actorUserID,
			CreatedAt:    now,
		}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.NewNotFound(op, "tratativa não encontrada", err)
	}
	if err != nil {
		return domain.NewInternal(op, "falha ao desfazer tratativa", err)
	}
	return nil
}

func (r *treatmentRepository) ListByOccurrence(occurrenceID int) ([]domain.Treatment, error) {
	const op = "treatmentRepository.ListByOccurrence"

	var rows []models.Treatment
	if err := r.db.Preload("Attachments").
		Where("occurrence_id = ?", occurrenceID).
		Order("created_at").Find(&rows).Error; err != nil {
		return nil, domain.NewInternal(op, "falha ao listar tratativas", err)
	}

	out := make([]domain.Treatment, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainTreatment(row))
	}
	return out, nil
}

func (r *treatmentRepository) GetAttachment(attachmentID int) (*domain.Attachment, error) {
	const op = "treatmentRepository.GetAttachment"

	var m models.Attachment
	err := r.db.First(&m, attachmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "anexo não encontrado", err)
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao buscar anexo", err)
	}

	attachment := toDomainAttachment(m)
	return &attachment, nil
}

func (r *treatmentRepository) RecordDownload(attachmentID, userID int) error {
	const op = "treatmentRepository.RecordDownload"

	if err := r.db.Create(&models.AttachmentDownload{
		AttachmentID: attachmentID,
		UserID:       userID,
		DownloadedAt: time.Now(),
	}).Error; err != nil {
		return domain.NewInternal(op, "falha ao registrar download do anexo", err)
	}
	return nil
}
