package repositories

import (
	"log"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/gorm"
)

type equipmentRepository struct {
	db *gorm.DB
}

func NewEquipmentRepository(db *gorm.DB) domain.EquipmentRepository {
	return &equipmentRepository{db: db}
}

// SaveAll espelha, numa única transação, a lista de equipamentos vinda da Secullum para
// o tenant: upsert por (tenant_id, secullum_id) e hard delete de qualquer equipamento do
// tenant fora da lista — mantendo o banco local sempre 1:1 com a Secullum.
func (r *equipmentRepository) SaveAll(tenantID int, equipments []domain.Equipment) error {
	const op = "equipmentRepository.SaveAll"

	err := r.db.Transaction(func(tx *gorm.DB) error {
		secullumIDs := make([]int, 0, len(equipments))
		for i := range equipments {
			secullumIDs = append(secullumIDs, equipments[i].SecullumID)

			var existing models.Equipment
			res := tx.Where("tenant_id = ? AND secullum_id = ?", tenantID, equipments[i].SecullumID).
				Limit(1).Find(&existing)
			if res.Error != nil {
				return res.Error
			}

			if res.RowsAffected == 0 {
				model := models.Equipment{
					TenantID:   tenantID,
					SecullumID: equipments[i].SecullumID,
					Descricao:  equipments[i].Descricao,
					EnderecoIP: equipments[i].EnderecoIP,
				}
				if err := tx.Create(&model).Error; err != nil {
					return err
				}
				continue
			}

			if err := tx.Model(&existing).Updates(map[string]interface{}{
				"descricao":   equipments[i].Descricao,
				"endereco_ip": equipments[i].EnderecoIP,
			}).Error; err != nil {
				return err
			}
		}

		// Espelhamento 1:1: remove qualquer equipamento do tenant que não veio na lista
		// desta sincronização. Uma lista vazia remove tudo — é o esperado se a Secullum
		// não tem mais aparelho algum cadastrado para o tenant.
		query := tx.Where("tenant_id = ?", tenantID)
		if len(secullumIDs) > 0 {
			query = query.Where("secullum_id NOT IN ?", secullumIDs)
		}
		res := query.Delete(&models.Equipment{})
		if res.Error != nil {
			return res.Error
		}
		// Uma lista vazia que apaga TODO o cadastro anterior é o caso mais caro de um
		// espelhamento estrito: legítimo (tenant zerou os aparelhos na Secullum), mas
		// indistinguível, só pelo dado, de uma resposta degradada da Secullum (200 com
		// corpo vazio por engano). Não há como diferenciar os dois casos aqui sem violar o
		// espelhamento 1:1 pedido — mas o log torna a perda visível para investigação
		// manual, em vez de sumir silenciosamente.
		if len(equipments) == 0 && res.RowsAffected > 0 {
			log.Printf("[Aviso Sync] Tenant %d: Secullum devolveu 0 equipamentos — %d equipamento(s) local(is) removido(s) para manter o espelhamento 1:1.",
				tenantID, res.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return domain.NewInternal(op, "falha ao sincronizar equipamentos", err)
	}
	return nil
}

func (r *equipmentRepository) GetByTenantID(tenantID int) ([]domain.Equipment, error) {
	const op = "equipmentRepository.GetByTenantID"

	var modelsList []models.Equipment
	if err := r.db.Where("tenant_id = ?", tenantID).Order("id").Find(&modelsList).Error; err != nil {
		return nil, domain.NewInternal(op, "falha ao listar equipamentos", err)
	}

	equipments := make([]domain.Equipment, 0, len(modelsList))
	for _, m := range modelsList {
		equipments = append(equipments, domain.Equipment{
			ID:         m.ID,
			TenantID:   m.TenantID,
			SecullumID: m.SecullumID,
			Descricao:  m.Descricao,
			EnderecoIP: m.EnderecoIP,
		})
	}
	return equipments, nil
}
