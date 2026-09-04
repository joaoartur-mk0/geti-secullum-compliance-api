package domain

import "time"

// MonthlyReview é o registro do encerramento de uma competência (Feature 3 — revisão
// mensal). O encerramento é por TENANT e COMPETÊNCIA, não por filial — filial não muda
// neste ciclo (ver docs/documento-funcional-compliance.md §7.5).
//
// "Fechamento" continua reservado para a varredura diária de D-1 (SchedulerService).
// Nenhum identificador aqui usa esse nome para o ciclo mensal.
type MonthlyReview struct {
	ID       int
	TenantID int
	// Competencia é o mês calendário, formato "YYYY-MM".
	Competencia string
	Status      MonthlyReviewStatus

	// Condições manuais — confirmação de que algo aconteceu FORA deste sistema. O painel
	// as exibe desabilitadas até existir aqui (ver docs/11 §9.3 regra 3).
	PayrollDone bool // folha de pagamento processada
	OffsetsDone bool // compensações lançadas

	ClosedAt       *time.Time
	ClosedByUserID *int

	// ReopenedAt/ReopenedByUserID/ReopenReason só são preenchidos na reabertura mais
	// recente — a trilha completa de reaberturas vive em MonthlyReviewEvent, este campo é
	// só o atalho para "o estado mais recente conta a história mais recente".
	ReopenedAt       *time.Time
	ReopenedByUserID *int
	ReopenReason     string
}

// MonthlyReviewStatus é o estado da competência.
type MonthlyReviewStatus string

const (
	MonthlyReviewOpen   MonthlyReviewStatus = "aberta"
	MonthlyReviewClosed MonthlyReviewStatus = "encerrada"
)

// MonthlyReviewEvent é o log append-only de encerramentos e reaberturas — mesma lógica de
// OccurrenceEvent: o estado atual vive em MonthlyReview, mas nenhuma transição se perde.
type MonthlyReviewEvent struct {
	ID              int
	MonthlyReviewID int
	TenantID        int
	Type            MonthlyReviewEventType
	Reason          string // obrigatório em reabertura; vazio em encerramento
	ActorUserID     int
	CreatedAt       time.Time
}

type MonthlyReviewEventType string

const (
	MonthlyReviewEventClosed MonthlyReviewEventType = "encerrada"
	MonthlyReviewEventReopen MonthlyReviewEventType = "reaberta"
)

// MonthlyReviewConditions são as seis condições do painel de revisão mensal — quatro
// automáticas (calculadas a cada consulta, nunca persistidas) e duas manuais (persistidas
// em MonthlyReview). Ver docs/documento-funcional-compliance.md §7.5.
type MonthlyReviewConditions struct {
	Competencia string
	Status      MonthlyReviewStatus

	OpenOccurrences int // aberta + atualizada
	PendingRecheck  int // atualizada (valor mudou desde a última varredura)
	OpenOperational int // severidade OPERACIONAL, aberta ou atualizada
	DaysWithoutScan int // dias da competência (até hoje) sem nenhum Report

	PayrollDone bool
	OffsetsDone bool

	ClosedAt       *time.Time
	ClosedByUserID *int
}

// Ready indica se as quatro condições automáticas permitem encerrar — as duas manuais
// exigem confirmação explícita à parte (não é aceitável assumir "sim" por omissão).
func (c MonthlyReviewConditions) Ready() bool {
	return c.OpenOccurrences == 0 && c.PendingRecheck == 0 && c.OpenOperational == 0 && c.DaysWithoutScan == 0
}

// MonthlyReviewRepository é o contrato de persistência da revisão mensal.
type MonthlyReviewRepository interface {
	// GetOrCreate devolve o registro da competência, criando-o em estado `aberta` (com as
	// duas condições manuais como false) se ainda não existir.
	GetOrCreate(tenantID int, competencia string) (*MonthlyReview, error)
	// SetManualConditions atualiza payroll_done/offsets_done. Ponteiros nil deixam o valor
	// atual intacto — permite atualizar só uma das duas condições por vez.
	SetManualConditions(tenantID int, competencia string, payrollDone, offsetsDone *bool) (*MonthlyReview, error)
	// Close encerra a competência. Devolve NewConflict se já estiver encerrada.
	Close(tenantID int, competencia string, actorUserID int) (*MonthlyReview, error)
	// Reopen reabre uma competência encerrada, com motivo obrigatório. Devolve
	// NewConflict se a competência já estiver aberta.
	Reopen(tenantID int, competencia string, actorUserID int, reason string) (*MonthlyReview, error)
	// IsClosedAt diz se a competência que contém `date` está encerrada para o tenant —
	// usado para congelar tratativa/ignorar em dias de uma competência encerrada.
	IsClosedAt(tenantID int, date time.Time) (bool, error)
}

// CompetenciaOf devolve a competência ("YYYY-MM") de uma data.
func CompetenciaOf(t time.Time) string {
	return t.Format("2006-01")
}

// MonthlyReviewExport é o relatório consolidado exportável de uma competência encerrada —
// a evidência do ciclo (docs/documento-funcional-compliance.md §7.5 regra 4). Só existe
// para competências já encerradas: é o registro do que estava decidido no momento do
// encerramento, não um recorte de trabalho em progresso.
type MonthlyReviewExport struct {
	Competencia      string
	ClosedAt         *time.Time
	ClosedByUserID   *int
	TotalOccurrences int
	ByState          map[string]int
	BySeverity       map[string]int
	ByType           map[string]int
	Occurrences      []Occurrence
}

// EnsureCompetenciaAberta bloqueia (NewConflict) tratar ou ignorar uma ocorrência cuja
// competência já foi encerrada — a mesma regra é usada por TreatmentService.Treat
// (usecase) e OccurrenceHandler.Ignore (interface/http); vive aqui, em domain, porque as
// duas camadas já importam domain e nenhuma pode importar a outra. `action` entra na
// mensagem ("tratar"/"ignorar") para o motivo do bloqueio ficar específico.
func EnsureCompetenciaAberta(repo MonthlyReviewRepository, op string, tenantID int, date time.Time, action string) error {
	closed, err := repo.IsClosedAt(tenantID, date)
	if err != nil {
		return err
	}
	if closed {
		return NewConflict(op, "competência encerrada", nil).
			WithDetails("esta ocorrência é de um dia cuja competência já foi encerrada — reabra a revisão mensal antes de " + action)
	}
	return nil
}
