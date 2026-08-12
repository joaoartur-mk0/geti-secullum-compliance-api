package repositories

import (
	"errors"
	"time"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type occurrenceRepository struct {
	db *gorm.DB
}

func NewOccurrenceRepository(db *gorm.DB) domain.OccurrenceRepository {
	return &occurrenceRepository{db: db}
}

func toDomainOccurrence(m models.Occurrence) domain.Occurrence {
	return domain.Occurrence{
		ID:               m.ID,
		TenantID:         m.TenantID,
		CollaboratorID:   m.CollaboratorID,
		CollaboratorName: m.CollaboratorName,
		Date:             m.Date,
		Type:             m.Type,
		Severity:         domain.Severity(m.Severity),
		Description:      m.Description,
		Fingerprint:      m.Fingerprint,
		State:            domain.OccurrenceState(m.State),
		FirstSeenAt:      m.FirstSeenAt,
		LastSeenAt:       m.LastSeenAt,
		TimesSeen:        m.TimesSeen,
		ResolvedAt:       m.ResolvedAt,
		IgnoredReason:    m.IgnoredReason,
		IgnoredByUserID:  m.IgnoredByUserID,
	}
}

func toModelOccurrence(o domain.Occurrence) models.Occurrence {
	return models.Occurrence{
		ID:               o.ID,
		TenantID:         o.TenantID,
		CollaboratorID:   o.CollaboratorID,
		CollaboratorName: o.CollaboratorName,
		Date:             domain.DayOf(o.Date),
		Type:             o.Type,
		Severity:         string(o.Severity),
		Description:      o.Description,
		Fingerprint:      o.Fingerprint,
		State:            string(o.State),
		FirstSeenAt:      o.FirstSeenAt,
		LastSeenAt:       o.LastSeenAt,
		TimesSeen:        o.TimesSeen,
		ResolvedAt:       o.ResolvedAt,
		IgnoredReason:    o.IgnoredReason,
		IgnoredByUserID:  o.IgnoredByUserID,
	}
}

func toDomainEvent(m models.OccurrenceEvent) domain.OccurrenceEvent {
	return domain.OccurrenceEvent{
		ID:              m.ID,
		OccurrenceID:    m.OccurrenceID,
		TenantID:        m.TenantID,
		Type:            domain.OccurrenceEventType(m.Type),
		FromState:       domain.OccurrenceState(m.FromState),
		ToState:         domain.OccurrenceState(m.ToState),
		FromDescription: m.FromDescription,
		ToDescription:   m.ToDescription,
		Reason:          m.Reason,
		ActorUserID:     m.ActorUserID,
		CreatedAt:       m.CreatedAt,
	}
}

func (r *occurrenceRepository) ListByTenantAndDate(tenantID int, date time.Time) ([]domain.Occurrence, error) {
	const op = "occurrenceRepository.ListByTenantAndDate"

	var rows []models.Occurrence
	err := r.db.Where("tenant_id = ? AND date = ?", tenantID, domain.DayOf(date)).
		Order("collaborator_id, type").Find(&rows).Error
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao listar ocorrências do dia", err)
	}

	out := make([]domain.Occurrence, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainOccurrence(m))
	}
	return out, nil
}

func (r *occurrenceRepository) List(filter domain.OccurrenceFilter) ([]domain.Occurrence, error) {
	const op = "occurrenceRepository.List"

	q := r.db.Where("tenant_id = ?", filter.TenantID)
	if filter.StartDate != nil {
		q = q.Where("date >= ?", domain.DayOf(*filter.StartDate))
	}
	if filter.EndDate != nil {
		q = q.Where("date <= ?", domain.DayOf(*filter.EndDate))
	}
	if len(filter.States) > 0 {
		states := make([]string, 0, len(filter.States))
		for _, s := range filter.States {
			states = append(states, string(s))
		}
		q = q.Where("state IN ?", states)
	}
	if filter.CollaboratorID != nil {
		q = q.Where("collaborator_id = ?", *filter.CollaboratorID)
	}

	var rows []models.Occurrence
	if err := q.Order("date DESC, collaborator_id, type").Find(&rows).Error; err != nil {
		return nil, domain.NewInternal(op, "falha ao listar ocorrências", err)
	}

	out := make([]domain.Occurrence, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainOccurrence(m))
	}
	return out, nil
}

func (r *occurrenceRepository) GetByID(id int) (*domain.Occurrence, error) {
	const op = "occurrenceRepository.GetByID"

	var m models.Occurrence
	err := r.db.First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "ocorrência não encontrada", err)
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao buscar ocorrência", err)
	}

	occ := toDomainOccurrence(m)
	return &occ, nil
}

// identityColumns são as colunas do índice único da ocorrência, usadas no ON CONFLICT.
var identityColumns = []clause.Column{
	{Name: "tenant_id"}, {Name: "collaborator_id"}, {Name: "date"}, {Name: "type"},
}

// ApplyChanges grava o resultado de uma reconciliação em UMA transação: ou o dia inteiro
// é atualizado, ou nada é. Sem isso, uma falha no meio deixaria parte das ocorrências
// resolvidas e parte não, e a próxima varredura interpretaria o estado pela metade.
func (r *occurrenceRepository) ApplyChanges(changes []domain.OccurrenceChange) error {
	const op = "occurrenceRepository.ApplyChanges"

	err := r.db.Transaction(func(tx *gorm.DB) error {
		for _, ch := range changes {
			model := toModelOccurrence(ch.Occurrence)

			switch ch.Kind {
			case domain.ChangeInsert:
				// ON CONFLICT protege contra a corrida entre dois workers auditando o
				// mesmo tenant/dia: o segundo vira atualização em vez de estourar a
				// chave única e derrubar a varredura inteira.
				res := tx.Clauses(clause.OnConflict{
					Columns: identityColumns,
					DoUpdates: clause.AssignmentColumns([]string{
						"collaborator_name", "severity", "description", "fingerprint",
						"state", "last_seen_at",
					}),
				}).Create(&model)
				if res.Error != nil {
					return res.Error
				}

			case domain.ChangeTouch:
				// Só o "ainda está aqui": não mexe em estado nem em valor.
				res := tx.Model(&models.Occurrence{}).Where("id = ?", model.ID).Updates(map[string]interface{}{
					"last_seen_at": model.LastSeenAt,
					"times_seen":   model.TimesSeen,
				})
				if res.Error != nil {
					return res.Error
				}

			case domain.ChangeUpdate:
				res := tx.Model(&models.Occurrence{}).Where("id = ?", model.ID).Updates(map[string]interface{}{
					"collaborator_name": model.CollaboratorName,
					"severity":          model.Severity,
					"description":       model.Description,
					"fingerprint":       model.Fingerprint,
					"state":             model.State,
					"last_seen_at":      model.LastSeenAt,
					"times_seen":        model.TimesSeen,
					"resolved_at":       model.ResolvedAt,
				})
				if res.Error != nil {
					return res.Error
				}

			case domain.ChangeResolve:
				res := tx.Model(&models.Occurrence{}).Where("id = ?", model.ID).Updates(map[string]interface{}{
					"state":       model.State,
					"resolved_at": model.ResolvedAt,
				})
				if res.Error != nil {
					return res.Error
				}
			}

			if ch.Event == nil {
				continue
			}

			event := models.OccurrenceEvent{
				// Numa inserção o id só existe depois do Create (RETURNING id).
				OccurrenceID:    model.ID,
				TenantID:        ch.Event.TenantID,
				Type:            string(ch.Event.Type),
				FromState:       string(ch.Event.FromState),
				ToState:         string(ch.Event.ToState),
				FromDescription: ch.Event.FromDescription,
				ToDescription:   ch.Event.ToDescription,
				Reason:          ch.Event.Reason,
				ActorUserID:     ch.Event.ActorUserID,
				CreatedAt:       ch.Event.CreatedAt,
			}
			if err := tx.Create(&event).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.NewInternal(op, "falha ao gravar ocorrências da varredura", err)
	}
	return nil
}

// Ignore marca a ocorrência como resolvida manualmente. Ignorar duas vezes não é erro
// (é idempotente), mas só a primeira gera evento — o log registra decisões, não cliques.
func (r *occurrenceRepository) Ignore(id int, reason string, actorUserID *int) error {
	const op = "occurrenceRepository.Ignore"

	err := r.db.Transaction(func(tx *gorm.DB) error {
		var current models.Occurrence
		if err := tx.First(&current, id).Error; err != nil {
			return err
		}
		if current.State == string(domain.OccurrenceResolvedManual) {
			return nil
		}

		now := time.Now()
		updates := map[string]interface{}{
			"state":              string(domain.OccurrenceResolvedManual),
			"resolved_at":        now,
			"ignored_reason":     reason,
			"ignored_by_user_id": actorUserID,
		}
		if err := tx.Model(&models.Occurrence{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}

		return tx.Create(&models.OccurrenceEvent{
			OccurrenceID: id,
			TenantID:     current.TenantID,
			Type:         string(domain.EventResolvedManual),
			FromState:    current.State,
			ToState:      string(domain.OccurrenceResolvedManual),
			Reason:       reason,
			ActorUserID:  actorUserID,
			CreatedAt:    now,
		}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.NewNotFound(op, "ocorrência não encontrada", err)
	}
	if err != nil {
		return domain.NewInternal(op, "falha ao ignorar ocorrência", err)
	}
	return nil
}

func (r *occurrenceRepository) ListEvents(occurrenceID int) ([]domain.OccurrenceEvent, error) {
	const op = "occurrenceRepository.ListEvents"

	var rows []models.OccurrenceEvent
	err := r.db.Where("occurrence_id = ?", occurrenceID).Order("created_at, id").Find(&rows).Error
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao listar o histórico da ocorrência", err)
	}

	out := make([]domain.OccurrenceEvent, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainEvent(m))
	}
	return out, nil
}
