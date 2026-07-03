package domain

// Tenant representa a entidade pura de negócio do Inquilino
type Tenant struct {
	ID                 int
	SecullumDatabaseID int
	Name               string
	SecullumToken      string
	EvolutionAPIUrl    string
	EvolutionAPIKey    string
	Settings           *TenantSettings
	Staffs             []Staff
}

// TenantSettings contém as flags (regras habilitadas) e os horários de varredura
type TenantSettings struct {
	Almoco       bool
	Interjornada bool
	Hextras      bool
	Esquecimento bool
	Horarios     []string // Ex: ["12:00", "14:00", "18:30"]
}

// Staff representa o gestor que receberá o alerta
type Staff struct {
	ID      int
	Name    string
	Celular string
}

// TenantRepository é o contrato que a camada de Infraestrutura (GORM) deverá implementar.
// A nossa regra de negócio chamará estes métodos sem saber se é Postgres ou MySQL.
type TenantRepository interface {
	GetByID(id int) (*Tenant, error)
	GetActiveTenants() ([]*Tenant, error)
	Save(tenant *Tenant) error
}
