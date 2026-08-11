package models

// UserTenant é a tabela de junção do vínculo N:N entre User e Tenant. A chave
// primária composta (user_id, tenant_id) já garante que o mesmo vínculo não se
// duplique.
type UserTenant struct {
	UserID   uint `gorm:"primaryKey;autoIncrement:false"`
	TenantID int  `gorm:"primaryKey;autoIncrement:false"`
}

func (UserTenant) TableName() string {
	return "user_tenants"
}
