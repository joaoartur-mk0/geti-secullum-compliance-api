package domain

import "time"

// Collaborator é a entidade que representa o funcionário auditado
type Collaborator struct {
	ID         int
	TenantID   int
	SecullumID int
	Name       string
	Cpf        string
	Celular    string

	// NumeroFolha é o número do colaborador na folha de pagamento (string na Secullum,
	// pode ter zeros à esquerda). Usado para resolver a filial quando a batida do dia não
	// identifica o aparelho — ver domain.BranchRepository.FindByPayrollNumber.
	NumeroFolha string

	// HorarioNumero identifica, na Secullum, qual jornada contratual (Horarios) este
	// colaborador segue. Preenchido pelo client em GetCollaborators; o Synchronizer o
	// usa para buscar (e deduplicar) as jornadas via GetHorario antes de montar Schedules.
	HorarioNumero int

	// Admissao/Demissao vêm direto da Secullum. Demitido é derivada de Demissao
	// preenchida — mantida como campo próprio (em vez de checar Demissao != nil toda
	// vez) porque é o que diferencia as rotas /collaborators (só ativos) e
	// /collaborators/history (todos).
	Admissao *time.Time
	Demissao *time.Time
	Demitido bool

	// DepartamentoID/Departamento, FuncaoID/Funcao e EmpresaID/Empresa vêm crus do
	// cadastro da Secullum (endpoint Funcionarios), sem normalização — se o cliente
	// cadastrou "AÇOUGUE" e "AÇOUGUE MATRIZ" como departamentos distintos, os dois
	// chegam aqui como registros distintos. Ver docs/documento-funcional-compliance.md
	// §7.1. Ponteiro no ID porque o colaborador pode não ter nenhum dos três atribuído.
	DepartamentoID *int
	Departamento   string
	FuncaoID       *int
	Funcao         string
	EmpresaID      *int
	Empresa        string
	// EmpresaDocumento é o CNPJ — vem no mesmo objeto Empresa do payload.
	EmpresaDocumento string

	Schedules []CollaboratorSchedule
}

// FilterOption é um item de catálogo (departamento, função ou empresa) para alimentar
// seletores de filtro no painel — ver domain.CollaboratorRepository.ListFilterCatalog.
type FilterOption struct {
	ID        int
	Descricao string
}

// CollaboratorFilterCatalog é a lista de departamentos, funções e empresas que existem
// hoje entre os colaboradores sincronizados de um tenant. Deriva do próprio cadastro de
// colaboradores (DISTINCT) em vez de sincronizar os endpoints de lista da Secullum à
// parte — o dado já chega embutido em cada funcionário, e não há necessidade de listar
// um departamento que nenhum colaborador usa.
type CollaboratorFilterCatalog struct {
	Departamentos []FilterOption
	Funcoes       []FilterOption
	Empresas      []FilterOption
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
	// GetByTenantID devolve só os colaboradores ATIVOS (sem Demissao) — usado pelo
	// painel padrão (GET /collaborators) e pelo motor de auditoria, que não deve avaliar
	// jornada de quem já foi desligado.
	GetByTenantID(tenantID int) ([]Collaborator, error)
	// GetHistoryByTenantID devolve TODOS os colaboradores do tenant, ativos e demitidos
	// — usado por GET /collaborators/history.
	GetHistoryByTenantID(tenantID int) ([]Collaborator, error)
	GetBySecullumID(tenantID int, secullumID int) (*Collaborator, error)
	// ListFilterCatalog devolve os departamentos, funções e empresas distintos entre os
	// colaboradores do tenant — usado pelos seletores de filtro do painel.
	ListFilterCatalog(tenantID int) (CollaboratorFilterCatalog, error)
}
