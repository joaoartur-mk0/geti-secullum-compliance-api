package models

// Equipment é o espelho local de um aparelho (relógio de ponto) da Secullum. Mantido
// espelhado 1:1 pelo sincronizador: um aparelho ausente na última sincronização é
// removido daqui (ver repositories.equipmentRepository.SaveAll).
type Equipment struct {
	ID         int     `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID   int     `gorm:"not null;uniqueIndex:idx_equipment_tenant_secullum" json:"tenant_id"`
	SecullumID int     `gorm:"not null;uniqueIndex:idx_equipment_tenant_secullum" json:"secullum_id"`
	Descricao  string  `gorm:"type:varchar(255)" json:"descricao"`
	EnderecoIP *string `gorm:"type:varchar(45)" json:"endereco_ip"`
}
