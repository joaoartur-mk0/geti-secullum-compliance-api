package repositories

import (
	"errors"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/gorm"
)

type staffRepository struct {
	db *gorm.DB
}

func NewStaffRepository(db *gorm.DB) domain.StaffRepository {
	return &staffRepository{db: db}
}

func (r *staffRepository) Create(staff *domain.Staff) error {
	const op = "staffRepository.Create"

	model := &models.Staff{
		TenantID: staff.TenantID,
		Name:     staff.Name,
		Celular:  staff.Celular,
	}
	if err := r.db.Create(model).Error; err != nil {
		return domain.NewInternal(op, "falha ao criar responsável", err)
	}
	staff.ID = model.ID
	return nil
}

func (r *staffRepository) Update(staff *domain.Staff) error {
	const op = "staffRepository.Update"

	res := r.db.Model(&models.Staff{}).Where("id = ?", staff.ID).Updates(map[string]interface{}{
		"name":    staff.Name,
		"celular": staff.Celular,
	})
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao atualizar responsável", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.NewNotFound(op, "responsável não encontrado", nil)
	}
	return nil
}

func (r *staffRepository) Delete(id int) error {
	const op = "staffRepository.Delete"

	res := r.db.Delete(&models.Staff{}, id)
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao excluir responsável", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.NewNotFound(op, "responsável não encontrado", nil)
	}
	return nil
}

func (r *staffRepository) GetByID(id int) (*domain.Staff, error) {
	const op = "staffRepository.GetByID"

	var model models.Staff
	err := r.db.First(&model, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "responsável não encontrado", err)
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao buscar responsável", err)
	}

	return &domain.Staff{ID: model.ID, TenantID: model.TenantID, Name: model.Name, Celular: model.Celular}, nil
}

func (r *staffRepository) ListByTenant(tenantID int) ([]domain.Staff, error) {
	const op = "staffRepository.ListByTenant"

	var modelsList []models.Staff
	if err := r.db.Where("tenant_id = ?", tenantID).Order("id").Find(&modelsList).Error; err != nil {
		return nil, domain.NewInternal(op, "falha ao listar responsáveis", err)
	}

	staffs := make([]domain.Staff, 0, len(modelsList))
	for _, m := range modelsList {
		staffs = append(staffs, domain.Staff{ID: m.ID, TenantID: m.TenantID, Name: m.Name, Celular: m.Celular})
	}
	return staffs, nil
}
