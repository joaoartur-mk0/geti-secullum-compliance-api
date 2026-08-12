package repositories

import (
	"errors"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/gorm"
)

type branchRepository struct {
	db *gorm.DB
}

func NewBranchRepository(db *gorm.DB) domain.BranchRepository {
	return &branchRepository{db: db}
}

func toDomainBranch(m models.Branch) domain.Branch {
	b := domain.Branch{
		ID:           m.ID,
		TenantID:     m.TenantID,
		Name:         m.Name,
		ManagerName:  m.ManagerName,
		ManagerPhone: m.ManagerPhone,
	}
	for _, d := range m.Devices {
		b.Devices = append(b.Devices, domain.BranchDevice{
			ID: d.ID, BranchID: d.BranchID, TenantID: d.TenantID,
			SecullumEquipID: d.SecullumEquipID, Label: d.Label,
		})
	}
	for _, p := range m.PayrollNumbers {
		b.PayrollNumbers = append(b.PayrollNumbers, domain.BranchPayrollNumber{
			ID: p.ID, BranchID: p.BranchID, TenantID: p.TenantID, Numero: p.Numero,
		})
	}
	return b
}

func (r *branchRepository) Create(branch *domain.Branch) error {
	const op = "branchRepository.Create"

	model := &models.Branch{
		TenantID:     branch.TenantID,
		Name:         branch.Name,
		ManagerName:  branch.ManagerName,
		ManagerPhone: branch.ManagerPhone,
	}
	if err := r.db.Create(model).Error; err != nil {
		return domain.NewInternal(op, "falha ao criar filial", err)
	}
	branch.ID = model.ID
	return nil
}

func (r *branchRepository) Update(branch *domain.Branch) error {
	const op = "branchRepository.Update"

	res := r.db.Model(&models.Branch{}).Where("id = ?", branch.ID).Updates(map[string]interface{}{
		"name":          branch.Name,
		"manager_name":  branch.ManagerName,
		"manager_phone": branch.ManagerPhone,
	})
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao atualizar filial", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.NewNotFound(op, "filial não encontrada", nil)
	}
	return nil
}

// Delete remove a filial junto com seus aparelhos e números de folha, numa transação:
// deixar aparelho órfão apontando para uma filial inexistente quebraria a resolução.
func (r *branchRepository) Delete(id int) error {
	const op = "branchRepository.Delete"

	var deleted int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("branch_id = ?", id).Delete(&models.BranchDevice{}).Error; err != nil {
			return err
		}
		if err := tx.Where("branch_id = ?", id).Delete(&models.BranchPayrollNumber{}).Error; err != nil {
			return err
		}
		res := tx.Delete(&models.Branch{}, id)
		deleted = res.RowsAffected
		return res.Error
	})
	if err != nil {
		return domain.NewInternal(op, "falha ao excluir filial", err)
	}
	if deleted == 0 {
		return domain.NewNotFound(op, "filial não encontrada", nil)
	}
	return nil
}

func (r *branchRepository) GetByID(id int) (*domain.Branch, error) {
	const op = "branchRepository.GetByID"

	var model models.Branch
	err := r.db.Preload("Devices").Preload("PayrollNumbers").First(&model, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "filial não encontrada", err)
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao buscar filial", err)
	}

	branch := toDomainBranch(model)
	return &branch, nil
}

func (r *branchRepository) ListByTenant(tenantID int) ([]domain.Branch, error) {
	const op = "branchRepository.ListByTenant"

	var rows []models.Branch
	err := r.db.Preload("Devices").Preload("PayrollNumbers").
		Where("tenant_id = ?", tenantID).Order("name").Find(&rows).Error
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao listar filiais", err)
	}

	out := make([]domain.Branch, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainBranch(m))
	}
	return out, nil
}

func (r *branchRepository) AddDevice(device *domain.BranchDevice) error {
	const op = "branchRepository.AddDevice"

	model := &models.BranchDevice{
		BranchID:        device.BranchID,
		TenantID:        device.TenantID,
		SecullumEquipID: device.SecullumEquipID,
		Label:           device.Label,
	}
	if err := r.db.Create(model).Error; err != nil {
		// O índice único (tenant, equip) é a regra "aparelho pertence a uma única
		// filial": o conflito aqui é erro do usuário, não falha do sistema.
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return domain.NewConflict(op, "este aparelho já está vinculado a outra filial", err)
		}
		return domain.NewInternal(op, "falha ao vincular aparelho", err)
	}
	device.ID = model.ID
	return nil
}

func (r *branchRepository) RemoveDevice(deviceID int) error {
	const op = "branchRepository.RemoveDevice"

	res := r.db.Delete(&models.BranchDevice{}, deviceID)
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao desvincular aparelho", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.NewNotFound(op, "aparelho não encontrado", nil)
	}
	return nil
}

func (r *branchRepository) AddPayrollNumber(pn *domain.BranchPayrollNumber) error {
	const op = "branchRepository.AddPayrollNumber"

	model := &models.BranchPayrollNumber{
		BranchID: pn.BranchID,
		TenantID: pn.TenantID,
		Numero:   pn.Numero,
	}
	if err := r.db.Create(model).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return domain.NewConflict(op, "este nº de folha já está vinculado a outra filial", err)
		}
		return domain.NewInternal(op, "falha ao vincular nº de folha", err)
	}
	pn.ID = model.ID
	return nil
}

func (r *branchRepository) RemovePayrollNumber(pnID int) error {
	const op = "branchRepository.RemovePayrollNumber"

	res := r.db.Delete(&models.BranchPayrollNumber{}, pnID)
	if res.Error != nil {
		return domain.NewInternal(op, "falha ao desvincular nº de folha", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.NewNotFound(op, "nº de folha não encontrado", nil)
	}
	return nil
}

// FindByDevice e FindByPayrollNumber devolvem (nil, nil) quando não há vínculo: filial
// desconhecida é situação normal (aparelho recém-instalado, colaborador sem lotação
// cadastrada), não erro.
func (r *branchRepository) FindByDevice(tenantID int, secullumEquipID int) (*domain.Branch, error) {
	const op = "branchRepository.FindByDevice"

	var device models.BranchDevice
	err := r.db.Where("tenant_id = ? AND secullum_equip_id = ?", tenantID, secullumEquipID).First(&device).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao resolver filial pelo aparelho", err)
	}
	return r.GetByID(device.BranchID)
}

func (r *branchRepository) FindByPayrollNumber(tenantID int, numero string) (*domain.Branch, error) {
	const op = "branchRepository.FindByPayrollNumber"

	if numero == "" {
		return nil, nil
	}

	var pn models.BranchPayrollNumber
	err := r.db.Where("tenant_id = ? AND numero = ?", tenantID, numero).First(&pn).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao resolver filial pelo nº de folha", err)
	}
	return r.GetByID(pn.BranchID)
}
