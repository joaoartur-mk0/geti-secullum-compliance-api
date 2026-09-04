package models

// UserTenant é a tabela de junção do vínculo N:N entre User e Tenant. A chave
// primária composta (user_id, tenant_id) já garante que o mesmo vínculo não se
// duplique.
type UserTenant struct {
	UserID   uint `gorm:"primaryKey;autoIncrement:false"`
	TenantID int  `gorm:"primaryKey;autoIncrement:false"`
	// Role — ver domain.Role. DEFAULT 'rh' é deliberado: todo vínculo que já existia
	// antes desta coluna existir preserva o acesso total que tinha (ninguém perde acesso
	// no dia do deploy). Vínculos NOVOS exigem role explícito no corpo da requisição —
	// o default da coluna nunca é usado pelo caminho de escrita da API, só protege quem
	// já estava lá.
	Role string `gorm:"type:varchar(20);not null;default:'rh';check:role IN ('rh','gestor','diretoria')"`
}

func (UserTenant) TableName() string {
	return "user_tenants"
}
