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

// CollaboratorSchedule é a grade semanal cadastrada no horário do colaborador, mantida
// como dado de referência (telas de cadastro).
//
// ATENÇÃO: NÃO é a fonte da carga usada na auditoria. A grade não reflete o que a
// Secullum efetivamente aplica no dia — diverge em escalas, horário alternativo,
// feriados e trocas pontuais. A carga esperada de cada data vem de DailyPunch.Previstas
// (campos Memoria* da resposta de batidas).
//
// A convenção de DiaSemana da Secullum neste campo NÃO está confirmada (a grade de
// exemplo tem 6 dias de 440min e um único dia zerado no índice 6, o que sugere
// 0=Segunda ... 6=Domingo, e não a convenção do time.Weekday do Go). Confirme antes de
// usar este campo para qualquer decisão por dia da semana.
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
