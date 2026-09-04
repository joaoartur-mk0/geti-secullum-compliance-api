package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Occurrence é uma inconsistência com IDENTIDADE ESTÁVEL e ciclo de vida próprio.
//
// Até aqui cada varredura gravava um relatório novo (Report) com a lista inteira de
// inconsistências apuradas: auditar o mesmo dia duas vezes produzia duas cópias da mesma
// infração, e não havia como dizer se uma ocorrência era nova, se tinha piorado ou se já
// havia sido resolvida. A ocorrência resolve isso: a chave de negócio
// (tenant + colaborador + data + tipo) é única, e o que muda entre varreduras é o ESTADO,
// não a quantidade de linhas.
//
// O Report continua sendo gravado, como registro da execução da varredura (é dele que
// saem o histórico e o gráfico de evolução do painel).
type Occurrence struct {
	ID       int
	TenantID int

	// --- Identidade estável (chave de negócio, única no banco) ---
	CollaboratorID int       // id do funcionário na Secullum (mesmo espaço de Collaborator.SecullumID)
	Date           time.Time // dia auditado
	Type           string    // tipo da inconsistência (ver constantes Tipo* do pacote usecase)

	CollaboratorName string

	// --- Valor apurado na última varredura ---
	Severity    Severity
	Description string
	// Fingerprint resume o valor apurado. Duas varreduras com o mesmo fingerprint
	// significam "nada mudou"; fingerprint diferente significa "o valor mudou" e dispara
	// a transição para OccurrenceUpdated.
	Fingerprint string

	// --- Ciclo de vida ---
	State       OccurrenceState
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	TimesSeen   int
	ResolvedAt  *time.Time

	// Preenchidos apenas quando a ocorrência é ignorada manualmente por um usuário.
	IgnoredReason   string
	IgnoredByUserID *int
}

// OccurrenceState é o estado da ocorrência na máquina de estados.
type OccurrenceState string

const (
	// OccurrenceOpen — apurada pela primeira vez e ainda presente.
	OccurrenceOpen OccurrenceState = "aberta"
	// OccurrenceUpdated — continua presente, mas com valor diferente do apurado antes
	// (ex.: o intervalo era de 43min e passou a 20min). Exige nova conferência.
	OccurrenceUpdated OccurrenceState = "atualizada"
	// OccurrenceResolvedAuto — deixou de ser apurada: a batida foi corrigida na Secullum
	// (ou a escala foi ajustada) e o problema não existe mais.
	OccurrenceResolvedAuto OccurrenceState = "resolvida_automatica"
	// OccurrenceResolvedManual — um usuário decidiu ignorar. É PEGAJOSO: varreduras
	// seguintes não reabrem a ocorrência, mesmo que ela continue sendo apurada.
	// EXIBIDA NA TELA COMO "Ignorada" — nunca como "tratada"/"resolvida"/"concluída".
	// Ignorar significa que o apontamento não procedia; tratar (abaixo) significa que
	// havia um problema real e alguém agiu. Confundir os dois apaga a distinção que
	// motivou a Feature 4 — ver docs/documento-funcional-compliance.md §1 e §3.1.
	OccurrenceResolvedManual OccurrenceState = "resolvida_manual"
	// OccurrenceTreated — um usuário registrou uma tratativa (Feature 4): houve ação real
	// sobre o problema, com justificativa e, quando o tipo exige, anexo. Também PEGAJOSO.
	// NUNCA fundida com OccurrenceResolvedAuto: a primeira é trabalho humano registrado
	// aqui, a segunda é o dado corrigido na origem — são as duas metades da dor que
	// originou o ciclo (quanto foi resolvido, e como).
	OccurrenceTreated OccurrenceState = "tratada"
)

// Open indica se a ocorrência ainda demanda ação do gestor.
func (s OccurrenceState) Open() bool {
	return s == OccurrenceOpen || s == OccurrenceUpdated
}

// Sticky indica se a ocorrência já tem desfecho humano e a reconciliação automática NÃO
// deve reabri-la, mesmo que a inconsistência volte a ser apurada. Ignorada e tratada são
// igualmente pegajosas — só resolvida_automatica pode ser revertida por uma nova varredura.
func (s OccurrenceState) Sticky() bool {
	return s == OccurrenceResolvedManual || s == OccurrenceTreated
}

// Valid indica se o valor é um dos estados conhecidos (validação de query string).
func (s OccurrenceState) Valid() bool {
	switch s {
	case OccurrenceOpen, OccurrenceUpdated, OccurrenceResolvedAuto, OccurrenceResolvedManual, OccurrenceTreated:
		return true
	}
	return false
}

// OccurrenceCategory agrupa as ocorrências para exibição no painel. Não é o mesmo eixo
// da severidade: uma ocorrência cuja escala mudou é operacional (o gestor precisa
// corrigir o cadastro, não punir o colaborador), e uma que mudou de valor desde a última
// varredura precisa de reconferência antes de virar ação.
type OccurrenceCategory string

const (
	CategoryCritical       OccurrenceCategory = "CRITICO"
	CategoryAlert          OccurrenceCategory = "ALERTA"
	CategoryScheduleChange OccurrenceCategory = "ALTERACAO_ESCALA"
	CategoryUnconfirmed    OccurrenceCategory = "NAO_CONFIRMADA"
)

// Category deriva a categoria de exibição a partir do estado e da severidade.
//
// A ordem importa: "atualizada" ganha de tudo, porque o valor mudou e o que o gestor viu
// da última vez não vale mais. Depois vem a severidade OPERACIONAL (alteração de escala),
// e só então a divisão clássica crítico/alerta.
func (o Occurrence) Category() OccurrenceCategory {
	switch {
	case o.State == OccurrenceUpdated:
		return CategoryUnconfirmed
	case o.Severity == SeverityOperational:
		return CategoryScheduleChange
	case o.Severity == SeverityCritical:
		return CategoryCritical
	default:
		return CategoryAlert
	}
}

// Fingerprint resume o valor apurado de uma inconsistência.
//
// As descrições geradas pelo motor de regras já embutem o número medido ("o intervalo foi
// de apenas 43 minutos", "realizou 2h15min de horas extras"), então o hash de
// tipo+severidade+descrição muda exatamente quando o valor muda — sem precisar
// instrumentar cada regra com um campo de valor estruturado.
func Fingerprint(inc AuditInconsistency) string {
	h := sha256.New()
	h.Write([]byte(inc.Type))
	h.Write([]byte{0})
	h.Write([]byte(inc.Severity))
	h.Write([]byte{0})
	h.Write([]byte(inc.Description))
	return hex.EncodeToString(h.Sum(nil))
}

// DayOf trunca um instante para a data (meia-noite no mesmo fuso). A identidade da
// ocorrência é por DIA — sem isso, duas varreduras do mesmo dia em horários diferentes
// gerariam chaves distintas e a deduplicação não funcionaria.
func DayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// OccurrenceEventType nomeia as transições registradas no log.
type OccurrenceEventType string

const (
	EventCreated         OccurrenceEventType = "criada"
	EventUpdated         OccurrenceEventType = "atualizada"
	EventResolvedAuto    OccurrenceEventType = "resolvida_automatica"
	EventResolvedManual  OccurrenceEventType = "resolvida_manual"
	EventReopened        OccurrenceEventType = "reaberta"
	EventTreated         OccurrenceEventType = "tratada"
	EventTreatmentUndone OccurrenceEventType = "tratativa_desfeita"
)

// OccurrenceEvent é uma linha do log append-only de transições. É o "mantendo os logs" do
// roadmap: o estado atual vive na Occurrence, mas nenhuma mudança é perdida — dá para
// reconstruir por que uma ocorrência está como está.
type OccurrenceEvent struct {
	ID           int
	OccurrenceID int
	TenantID     int
	Type         OccurrenceEventType
	FromState    OccurrenceState
	ToState      OccurrenceState
	// Descrição antes/depois, preenchidas quando o valor apurado mudou.
	FromDescription string
	ToDescription   string
	Reason          string // motivo informado no "ignorar"
	ActorUserID     *int   // usuário que agiu; nil quando a transição foi automática
	CreatedAt       time.Time
}

// OccurrenceFilter são os recortes aceitos na consulta de ocorrências pelo painel.
// Campos zerados significam "sem filtro".
type OccurrenceFilter struct {
	TenantID       int
	StartDate      *time.Time
	EndDate        *time.Time
	States         []OccurrenceState
	CollaboratorID *int
}

// OccurrenceRepository é o contrato de persistência da máquina de estados.
//
// Reconcile faz a comparação inteira de um dia em UMA transação: é o que garante que dois
// workers auditando o mesmo tenant/dia ao mesmo tempo não criem ocorrências duplicadas,
// apoiado no índice único (tenant_id, collaborator_id, date, type).
type OccurrenceRepository interface {
	// ListByTenantAndDate devolve todas as ocorrências de um dia, em qualquer estado.
	ListByTenantAndDate(tenantID int, date time.Time) ([]Occurrence, error)
	// List aplica os filtros do painel, do mais recente para o mais antigo.
	List(filter OccurrenceFilter) ([]Occurrence, error)
	GetByID(id int) (*Occurrence, error)
	// ApplyChanges persiste, atomicamente, o resultado de uma reconciliação.
	ApplyChanges(changes []OccurrenceChange) error
	// Ignore marca a ocorrência como resolvida_manual e registra o evento.
	Ignore(id int, reason string, actorUserID *int) error
	// ListEvents devolve o log de transições de uma ocorrência, do mais antigo ao mais recente.
	ListEvents(occurrenceID int) ([]OccurrenceEvent, error)
}

// OccurrenceChangeKind diz ao repositório o que fazer com a ocorrência reconciliada.
type OccurrenceChangeKind string

const (
	// ChangeInsert — ocorrência nova.
	ChangeInsert OccurrenceChangeKind = "insert"
	// ChangeTouch — mesma ocorrência, mesmo valor: só atualiza last_seen_at/times_seen.
	// É o caminho do sync repetido no mesmo dia, e não gera evento.
	ChangeTouch OccurrenceChangeKind = "touch"
	// ChangeUpdate — mudou de valor e/ou de estado: atualiza a linha e grava o evento.
	ChangeUpdate OccurrenceChangeKind = "update"
	// ChangeResolve — sumiu da apuração: vira resolvida_automatica.
	ChangeResolve OccurrenceChangeKind = "resolve"
)

// OccurrenceChange é a instrução emitida pelo reconciliador para o repositório. Separar
// "decidir" de "gravar" mantém a regra de negócio testável sem banco.
type OccurrenceChange struct {
	Kind       OccurrenceChangeKind
	Occurrence Occurrence
	Event      *OccurrenceEvent // nil em ChangeTouch
}
