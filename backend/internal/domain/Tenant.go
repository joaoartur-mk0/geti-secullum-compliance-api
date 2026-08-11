package domain

// Tenant representa a entidade pura de negócio do Inquilino.
// As credenciais da Secullum/Evolution são GLOBAIS (configuradas por env), não por
// tenant; o que identifica o cliente na Secullum é o SecullumDatabaseID.
type Tenant struct {
	ID                 int
	SecullumDatabaseID int
	Name               string
	Active             bool // false = tenant desativado (não auditado nem listado por padrão)
	Settings           *TenantSettings
	Staffs             []Staff
}

// TenantSettings contém as flags (regras habilitadas), as severidades configuráveis
// por regra e os horários de varredura.
type TenantSettings struct {
	Almoco       bool
	Interjornada bool
	Hextras      bool
	Esquecimento bool

	// Severidade configurável por tenant para cada regra (default CRITICO se vazio).
	// A hora extra não entra aqui: segue os limiares legais (Art. 59 CLT).
	AlmocoSeverity       Severity
	InterjornadaSeverity Severity
	EsquecimentoSeverity Severity

	Horarios []string // Ex: ["12:00", "14:00", "18:30"]
}

// Staff representa o gestor que receberá o alerta
type Staff struct {
	ID       int
	TenantID int
	Name     string
	Celular  string
}

// TenantRepository é o contrato que a camada de Infraestrutura (GORM) deverá implementar.
// A nossa regra de negócio chamará estes métodos sem saber se é Postgres ou MySQL.
type TenantRepository interface {
	GetByID(id int) (*Tenant, error)
	GetActiveTenants() ([]*Tenant, error)
	List(includeInactive bool) ([]*Tenant, error)
	Save(tenant *Tenant) error
	Update(tenant *Tenant) error
	Activate(id int) error
	Deactivate(id int) error
	// Delete apaga o tenant e tudo que depende dele (settings, staffs, colaboradores,
	// jornadas, relatórios, vínculos com usuários) — operação irreversível, perde o
	// histórico de auditoria. Prefira Deactivate quando o objetivo é só suspender o
	// acesso mantendo o histórico.
	Delete(id int) error

	GetSettings(tenantID int) (*TenantSettings, error)
	UpdateSettings(tenantID int, settings *TenantSettings) error
}

// StaffRepository é o contrato para o CRUD de responsáveis (gestores) de um tenant.
type StaffRepository interface {
	Create(staff *Staff) error
	Update(staff *Staff) error
	Delete(id int) error
	GetByID(id int) (*Staff, error)
	ListByTenant(tenantID int) ([]Staff, error)
}
