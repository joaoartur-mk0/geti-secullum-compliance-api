package repositories

import (
	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/gorm"
)

type tenantRepository struct {
	db *gorm.DB
}

// NewTenantRepository injeta a dependência do GORM
func NewTenantRepository(db *gorm.DB) domain.TenantRepository {
	return &tenantRepository{db: db}
}

func (r *tenantRepository) GetByID(id int) (*domain.Tenant, error) {
	var model models.Tenant

	// Preload carrega as configurações e os gestores (Staffs) vinculados
	err := r.db.Preload("Settings").Preload("Staffs").First(&model, id).Error
	if err != nil {
		return nil, err
	}

	// Conversão de Model (Infra) para Entity (Domain)
	domainTenant := &domain.Tenant{
		ID:                 model.ID,
		SecullumDatabaseID: model.SecullumDatabaseID,
		Name:               model.Name,
		SecullumToken:      model.SecullumToken, // Lembre-se: Descriptografar aqui se usar AES
		EvolutionAPIUrl:    model.EvolutionAPIUrl,
		EvolutionAPIKey:    model.EvolutionAPIKey,
	}

	if model.Settings != nil {
		domainTenant.Settings = &domain.TenantSettings{
			Almoco:       model.Settings.Almoco,
			Interjornada: model.Settings.Interjornada,
			Hextras:      model.Settings.Hextras,
			Esquecimento: model.Settings.Esquecimento,
			// Aqui você faria o unmarshal do datatypes.JSON de Horarios
		}
	}

	for _, staff := range model.Staffs {
		domainTenant.Staffs = append(domainTenant.Staffs, domain.Staff{
			ID:      staff.ID,
			Name:    staff.Name,
			Celular: staff.Celular,
		})
	}

	return domainTenant, nil
}
