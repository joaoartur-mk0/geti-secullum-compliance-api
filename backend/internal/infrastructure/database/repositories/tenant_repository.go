package repositories

import (
	"encoding/json"
	"errors"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/datatypes"
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
	const op = "tenantRepository.GetByID"

	var model models.Tenant
	err := r.db.Preload("Settings").Preload("Staffs").First(&model, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "tenant não encontrado", err)
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao buscar tenant", err)
	}

	return toDomainTenant(&model), nil
}

func (r *tenantRepository) GetActiveTenants() ([]*domain.Tenant, error) {
	const op = "tenantRepository.GetActiveTenants"

	var modelsList []models.Tenant
	err := r.db.Preload("Settings").Preload("Staffs").Where("active = ?", true).Find(&modelsList).Error
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao listar tenants ativos", err)
	}

	return mapTenants(modelsList), nil
}

func (r *tenantRepository) List(includeInactive bool) ([]*domain.Tenant, error) {
	const op = "tenantRepository.List"

	query := r.db.Preload("Settings").Preload("Staffs")
	if !includeInactive {
		query = query.Where("active = ?", true)
	}

	var modelsList []models.Tenant
	if err := query.Order("id").Find(&modelsList).Error; err != nil {
		return nil, domain.NewInternal(op, "falha ao listar tenants", err)
	}

	return mapTenants(modelsList), nil
}

func (r *tenantRepository) Save(tenant *domain.Tenant) error {
	const op = "tenantRepository.Save"

	settings := tenant.Settings
	if settings == nil {
		settings = &domain.TenantSettings{}
	}

	horariosJSON, err := marshalHorarios(settings.Horarios)
	if err != nil {
		return domain.NewInternal(op, "falha ao serializar horários", err)
	}

	tenantModel := &models.Tenant{
		Name:               tenant.Name,
		SecullumDatabaseID: tenant.SecullumDatabaseID,
		Active:             true,
		Settings: &models.TenantSettings{
			Almoco:               settings.Almoco,
			Interjornada:         settings.Interjornada,
			Hextras:              settings.Hextras,
			Esquecimento:         settings.Esquecimento,
			AlmocoSeverity:       string(settings.AlmocoSeverity),
			InterjornadaSeverity: string(settings.InterjornadaSeverity),
			EsquecimentoSeverity: string(settings.EsquecimentoSeverity),
			Horarios:             horariosJSON,
		},
	}

	err = r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(tenantModel).Error; err != nil {
			return err
		}
		for _, st := range tenant.Staffs {
			staffModel := &models.Staff{TenantID: tenantModel.ID, Name: st.Name, Celular: st.Celular}
			if err := tx.Create(staffModel).Error; err != nil {
				return err
			}
		}
		tenant.ID = tenantModel.ID
		return nil
	})
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.NewConflict(op, "já existe um tenant com este secullum_database_id", err)
	}
	if err != nil {
		return domain.NewInternal(op, "falha ao salvar tenant", err)
	}
	return nil
}

func (r *tenantRepository) Update(tenant *domain.Tenant) error {
	const op = "tenantRepository.Update"

	// Atualiza apenas os campos editáveis do tenant.
	res := r.db.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Updates(map[string]interface{}{
		"name":                 tenant.Name,
		"secullum_database_id": tenant.SecullumDatabaseID,
	})
	if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
		return domain.NewConflict(op, "já existe um tenant com este secullum_database_id", res.Error)
	}
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao atualizar tenant", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.NewNotFound(op, "tenant não encontrado", nil)
	}
	return nil
}

func (r *tenantRepository) Activate(id int) error {
	const op = "tenantRepository.Activate"
	return r.setActive(op, id, true)
}

func (r *tenantRepository) Deactivate(id int) error {
	const op = "tenantRepository.Deactivate"
	return r.setActive(op, id, false)
}

func (r *tenantRepository) setActive(op string, id int, active bool) error {
	res := r.db.Model(&models.Tenant{}).Where("id = ?", id).Update("active", active)
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao atualizar estado do tenant", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.NewNotFound(op, "tenant não encontrado", nil)
	}
	return nil
}

// Delete apaga o tenant e tudo que depende dele numa transação: jornadas dos
// colaboradores, colaboradores, relatórios, staffs, configurações e vínculos com
// usuários. Operação irreversível — perde o histórico de auditoria do tenant.
func (r *tenantRepository) Delete(id int) error {
	const op = "tenantRepository.Delete"

	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Os dependentes têm FK para tenants(id) — precisam sair antes do tenant.
		if err := tx.Exec(
			`DELETE FROM collaborator_schedules WHERE collaborator_id IN (SELECT id FROM collaborators WHERE tenant_id = ?)`,
			id,
		).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ?", id).Delete(&models.Collaborator{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ?", id).Delete(&models.Report{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ?", id).Delete(&models.Staff{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ?", id).Delete(&models.TenantSettings{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ?", id).Delete(&models.UserTenant{}).Error; err != nil {
			return err
		}

		res := tx.Where("id = ?", id).Delete(&models.Tenant{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.NewNotFound(op, "tenant não encontrado", err)
	}
	if err != nil {
		return domain.NewInternal(op, "falha ao apagar tenant", err)
	}
	return nil
}

func (r *tenantRepository) GetSettings(tenantID int) (*domain.TenantSettings, error) {
	const op = "tenantRepository.GetSettings"

	var model models.TenantSettings
	err := r.db.Where("tenant_id = ?", tenantID).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "configurações não encontradas para o tenant", err)
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao buscar configurações", err)
	}

	return toDomainSettings(&model), nil
}

func (r *tenantRepository) UpdateSettings(tenantID int, settings *domain.TenantSettings) error {
	const op = "tenantRepository.UpdateSettings"

	horariosJSON, err := marshalHorarios(settings.Horarios)
	if err != nil {
		return domain.NewInternal(op, "falha ao serializar horários", err)
	}

	res := r.db.Model(&models.TenantSettings{}).Where("tenant_id = ?", tenantID).Updates(map[string]interface{}{
		"almoco":                settings.Almoco,
		"interjornada":          settings.Interjornada,
		"hextras":               settings.Hextras,
		"esquecimento":          settings.Esquecimento,
		"almoco_severity":       string(settings.AlmocoSeverity),
		"interjornada_severity": string(settings.InterjornadaSeverity),
		"esquecimento_severity": string(settings.EsquecimentoSeverity),
		"horarios":              horariosJSON,
	})
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao atualizar configurações", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.NewNotFound(op, "configurações não encontradas para o tenant", nil)
	}
	return nil
}

// --- Conversões e helpers ---

func mapTenants(modelsList []models.Tenant) []*domain.Tenant {
	tenants := make([]*domain.Tenant, 0, len(modelsList))
	for i := range modelsList {
		tenants = append(tenants, toDomainTenant(&modelsList[i]))
	}
	return tenants
}

func marshalHorarios(horarios []string) (datatypes.JSON, error) {
	if horarios == nil {
		horarios = []string{}
	}
	raw, err := json.Marshal(horarios)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(raw), nil
}

func toDomainSettings(m *models.TenantSettings) *domain.TenantSettings {
	var horarios []string
	if len(m.Horarios) > 0 {
		_ = json.Unmarshal(m.Horarios, &horarios)
	}
	return &domain.TenantSettings{
		Almoco:               m.Almoco,
		Interjornada:         m.Interjornada,
		Hextras:              m.Hextras,
		Esquecimento:         m.Esquecimento,
		AlmocoSeverity:       domain.Severity(m.AlmocoSeverity),
		InterjornadaSeverity: domain.Severity(m.InterjornadaSeverity),
		EsquecimentoSeverity: domain.Severity(m.EsquecimentoSeverity),
		Horarios:             horarios,
	}
}

// toDomainTenant converte o model (infra) para a entidade de domínio, incluindo
// settings (com severidades e horários) e staffs.
func toDomainTenant(model *models.Tenant) *domain.Tenant {
	domainTenant := &domain.Tenant{
		ID:                 model.ID,
		SecullumDatabaseID: model.SecullumDatabaseID,
		Name:               model.Name,
		Active:             model.Active,
	}

	if model.Settings != nil {
		domainTenant.Settings = toDomainSettings(model.Settings)
	}

	for _, staff := range model.Staffs {
		domainTenant.Staffs = append(domainTenant.Staffs, domain.Staff{
			ID:       staff.ID,
			TenantID: staff.TenantID,
			Name:     staff.Name,
			Celular:  staff.Celular,
		})
	}

	return domainTenant
}
