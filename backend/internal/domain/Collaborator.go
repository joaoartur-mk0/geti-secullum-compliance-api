package domain

// Collaborator é a entidade que representa o funcionário auditado
type Collaborator struct {
	ID         int
	TenantID   int
	SecullumID int
	Cpf        string
	Celular    string
	Schedules  []CollaboratorSchedule
}

// CollaboratorSchedule representa a jornada de trabalho contratual prevista
type CollaboratorSchedule struct {
	Entrada1 string // Formato esperado "HH:MM"
	Saida1   string
	Entrada2 string
	Saida2   string
}

// CollaboratorRepository é o contrato para salvar e buscar o espelho local de funcionários
type CollaboratorRepository interface {
	Save(collaborator *Collaborator) error
	SaveAll(collaborators []Collaborator) error
	GetByTenantID(tenantID int) ([]Collaborator, error)
	GetBySecullumID(tenantID int, secullumID int) (*Collaborator, error)
}
