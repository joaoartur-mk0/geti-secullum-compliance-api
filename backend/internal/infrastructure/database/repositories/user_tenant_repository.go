package repositories

import (
	"errors"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/gorm"
)

type userTenantRepository struct {
	db *gorm.DB
}

// NewUserTenantRepository injeta a dependência do GORM para o vínculo N:N entre
// usuários e tenants.
func NewUserTenantRepository(db *gorm.DB) domain.UserTenantRepository {
	return &userTenantRepository{db: db}
}

func (r *userTenantRepository) AddUserToTenant(userID uint, tenantID int) error {
	const op = "userTenantRepository.AddUserToTenant"

	link := models.UserTenant{UserID: userID, TenantID: tenantID}
	err := r.db.Create(&link).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.NewConflict(op, "usuário já tem acesso a este tenant", err)
	}
	if err != nil {
		return domain.NewInternal(op, "falha ao vincular usuário ao tenant", err)
	}
	return nil
}

func (r *userTenantRepository) RemoveUserFromTenant(userID uint, tenantID int) error {
	const op = "userTenantRepository.RemoveUserFromTenant"

	res := r.db.Where("user_id = ? AND tenant_id = ?", userID, tenantID).Delete(&models.UserTenant{})
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao remover vínculo entre usuário e tenant", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.NewNotFound(op, "vínculo não encontrado", nil)
	}
	return nil
}

func (r *userTenantRepository) HasAccess(userID uint, tenantID int) (bool, error) {
	const op = "userTenantRepository.HasAccess"

	var count int64
	err := r.db.Model(&models.UserTenant{}).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Count(&count).Error
	if err != nil {
		return false, domain.NewInternal(op, "falha ao verificar acesso ao tenant", err)
	}
	return count > 0, nil
}

func (r *userTenantRepository) ListTenantsForUser(userID uint) ([]*domain.Tenant, error) {
	const op = "userTenantRepository.ListTenantsForUser"

	var tenantModels []models.Tenant
	err := r.db.
		Joins("JOIN user_tenants ON user_tenants.tenant_id = tenants.id").
		Where("user_tenants.user_id = ?", userID).
		Order("tenants.id").
		Find(&tenantModels).Error
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao listar tenants do usuário", err)
	}

	return mapTenants(tenantModels), nil
}

func (r *userTenantRepository) ListUsersForTenant(tenantID int) ([]domain.User, error) {
	const op = "userTenantRepository.ListUsersForTenant"

	var userModels []models.User
	err := r.db.
		Joins("JOIN user_tenants ON user_tenants.user_id = users.id").
		Where("user_tenants.tenant_id = ?", tenantID).
		Order("users.id").
		Find(&userModels).Error
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao listar usuários do tenant", err)
	}

	return mapUsers(userModels), nil
}
