package domain

// Collaborator é a entidade que representa o funcionário auditado
type Collaborator struct {
	ID         int
	TenantID   int
	SecullumID int
	Name       string
	Cpf        string
	Celular    string

	// HorarioNumero identifica, na Secullum, qual jornada contratual (Horarios) este
	// colaborador segue. Preenchido pelo client em GetCollaborators; o Synchronizer o
	// usa para buscar (e deduplicar) as jornadas via GetHorario antes de montar Schedules.
	HorarioNumero int

	Schedules []CollaboratorSchedule
}

// CollaboratorSchedule representa a jornada de trabalho contratual prevista para um
// dia específico da semana (a Secullum define a jornada por dia, não um bloco único
// para a semana toda — sábados/domingos costumam ter cargas diferentes).
//
// DiaSemana segue a mesma convenção do time.Weekday do Go e do DiaSemana da Secullum:
// 0 = Domingo, 1 = Segunda, ..., 6 = Sábado.
type CollaboratorSchedule struct {
	DiaSemana    int
	Entrada1     string // Formato esperado "HH:MM"
	Saida1       string
	Entrada2     string
	Saida2       string
	CargaMinutos int // Carga diária contratual em minutos, conforme calculada pela Secullum
}

// CollaboratorRepository é o contrato para salvar e buscar o espelho local de funcionários
type CollaboratorRepository interface {
	Save(collaborator *Collaborator) error
	SaveAll(collaborators []Collaborator) error
	GetByTenantID(tenantID int) ([]Collaborator, error)
	GetBySecullumID(tenantID int, secullumID int) (*Collaborator, error)
}
