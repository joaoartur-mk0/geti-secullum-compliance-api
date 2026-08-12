package repositories

import (
	"errors"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/gorm"
)

type collaboratorRepository struct {
	db *gorm.DB
}

func NewCollaboratorRepository(db *gorm.DB) domain.CollaboratorRepository {
	return &collaboratorRepository{db: db}
}

func (r *collaboratorRepository) Save(collaborator *domain.Collaborator) error {
	const op = "collaboratorRepository.Save"
	return r.upsert(r.db, collaborator, op)
}

// SaveAll faz o upsert de todos os colaboradores em uma única transação: se o processo
// falhar no meio (ex.: erro de dado num colaborador), nenhum fica parcialmente salvo.
// Cada colaborador é identificado por (tenant_id, secullum_id) — o mesmo funcionário
// sincronizado de novo atualiza o registro existente em vez de duplicar.
func (r *collaboratorRepository) SaveAll(collaborators []domain.Collaborator) error {
	const op = "collaboratorRepository.SaveAll"

	err := r.db.Transaction(func(tx *gorm.DB) error {
		for i := range collaborators {
			if err := r.upsert(tx, &collaborators[i], op); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		var appErr *domain.AppError
		if errors.As(err, &appErr) {
			return err
		}
		return domain.NewInternal(op, "falha ao sincronizar colaboradores", err)
	}
	return nil
}

// upsert encontra o colaborador por (tenant_id, secullum_id); se existir, atualiza os
// dados e substitui as jornadas (schedules); se não, cria um novo registro.
//
// Usa Limit(1).Find em vez de First: numa primeira sincronização TODOS os colaboradores
// são novos, e o First loga "record not found" como erro para cada um — centenas de
// linhas de ruído para um caso perfeitamente esperado. O Find com RowsAffected tem a
// mesma semântica, sem poluir o log.
func (r *collaboratorRepository) upsert(tx *gorm.DB, collaborator *domain.Collaborator, op string) error {
	var existing models.Collaborator
	res := tx.Where("tenant_id = ? AND secullum_id = ?", collaborator.TenantID, collaborator.SecullumID).
		Limit(1).Find(&existing)
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao buscar colaborador existente", res.Error)
	}

	schedules := toModelSchedules(collaborator.Schedules)

	if res.RowsAffected == 0 {
		model := &models.Collaborator{
			TenantID:    collaborator.TenantID,
			SecullumID:  collaborator.SecullumID,
			Name:        collaborator.Name,
			Cpf:         collaborator.Cpf,
			Celular:     collaborator.Celular,
			NumeroFolha: collaborator.NumeroFolha,
			Schedules:   schedules,
		}
		if err := tx.Create(model).Error; err != nil {
			return domain.NewInternal(op, "falha ao criar colaborador", err)
		}
		collaborator.ID = model.ID
		return nil
	}

	// Atualiza os dados de identidade.
	if err := tx.Model(&existing).Updates(map[string]interface{}{
		"name":         collaborator.Name,
		"cpf":          collaborator.Cpf,
		"celular":      collaborator.Celular,
		"numero_folha": collaborator.NumeroFolha,
	}).Error; err != nil {
		return domain.NewInternal(op, "falha ao atualizar colaborador", err)
	}

	// Substitui as jornadas (a fonte de verdade é sempre a última sincronização).
	if err := tx.Where("collaborator_id = ?", existing.ID).Delete(&models.CollaboratorSchedule{}).Error; err != nil {
		return domain.NewInternal(op, "falha ao limpar jornadas antigas", err)
	}
	for i := range schedules {
		schedules[i].CollaboratorID = existing.ID
		if err := tx.Create(&schedules[i]).Error; err != nil {
			return domain.NewInternal(op, "falha ao salvar jornada", err)
		}
	}

	collaborator.ID = existing.ID
	return nil
}

func (r *collaboratorRepository) GetByTenantID(tenantID int) ([]domain.Collaborator, error) {
	const op = "collaboratorRepository.GetByTenantID"

	var modelsList []models.Collaborator
	if err := r.db.Preload("Schedules").Where("tenant_id = ?", tenantID).Order("id").Find(&modelsList).Error; err != nil {
		return nil, domain.NewInternal(op, "falha ao listar colaboradores", err)
	}

	collaborators := make([]domain.Collaborator, 0, len(modelsList))
	for _, m := range modelsList {
		collaborators = append(collaborators, toDomainCollaborator(&m))
	}
	return collaborators, nil
}

func (r *collaboratorRepository) GetBySecullumID(tenantID int, secullumID int) (*domain.Collaborator, error) {
	const op = "collaboratorRepository.GetBySecullumID"

	var model models.Collaborator
	err := r.db.Preload("Schedules").Where("tenant_id = ? AND secullum_id = ?", tenantID, secullumID).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "colaborador não encontrado", err)
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao buscar colaborador", err)
	}

	collab := toDomainCollaborator(&model)
	return &collab, nil
}

func toModelSchedules(schedules []domain.CollaboratorSchedule) []models.CollaboratorSchedule {
	out := make([]models.CollaboratorSchedule, 0, len(schedules))
	for _, s := range schedules {
		out = append(out, models.CollaboratorSchedule{
			DiaSemana:    s.DiaSemana,
			Entrada1:     s.Entrada1,
			Saida1:       s.Saida1,
			Entrada2:     s.Entrada2,
			Saida2:       s.Saida2,
			CargaMinutos: s.CargaMinutos,
		})
	}
	return out
}

func toDomainCollaborator(m *models.Collaborator) domain.Collaborator {
	c := domain.Collaborator{
		ID:          m.ID,
		TenantID:    m.TenantID,
		SecullumID:  m.SecullumID,
		Name:        m.Name,
		Cpf:         m.Cpf,
		Celular:     m.Celular,
		NumeroFolha: m.NumeroFolha,
	}
	for _, s := range m.Schedules {
		c.Schedules = append(c.Schedules, domain.CollaboratorSchedule{
			DiaSemana:    s.DiaSemana,
			Entrada1:     s.Entrada1,
			Saida1:       s.Saida1,
			Entrada2:     s.Entrada2,
			Saida2:       s.Saida2,
			CargaMinutos: s.CargaMinutos,
		})
	}
	return c
}
