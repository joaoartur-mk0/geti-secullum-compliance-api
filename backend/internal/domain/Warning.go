package domain

import "time"

// Warning é o registro de uma advertência emitida a um colaborador.
//
// O sistema não gera nem entrega o documento: ele acompanha o ESTADO do processo, para o
// RH saber o que já saiu do rascunho, o que foi entregue e o que voltou assinado. A
// assinatura eletrônica (hash do documento) ficou fora deste momento — quando entrar, o
// campo se apoia no mesmo registro, sem mudar o fluxo.
type Warning struct {
	ID       int
	TenantID int

	// OccurrenceID liga a advertência à ocorrência que a motivou. Opcional: o RH pode
	// advertir por algo que a auditoria não apurou.
	OccurrenceID *int

	CollaboratorID   int // id na Secullum
	CollaboratorName string
	BranchID         *int // filial no momento da emissão (quem responde pelo colaborador)

	Body   string
	Status WarningStatus

	CreatedByUserID *int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	SentAt          *time.Time
	SignedAt        *time.Time
}

// WarningStatus é o estágio da advertência.
type WarningStatus string

const (
	WarningDraft  WarningStatus = "draft"   // escrita, ainda não entregue
	WarningSent   WarningStatus = "enviada" // entregue ao colaborador
	WarningSigned WarningStatus = "assinada"
)

func (s WarningStatus) Valid() bool {
	switch s {
	case WarningDraft, WarningSent, WarningSigned:
		return true
	}
	return false
}

// CanTransitionTo diz se a mudança de status é permitida.
//
// O fluxo é de mão única: draft → enviada → assinada. Não se "desassina" um documento
// entregue nem se pula a entrega — permitir isso deixaria o registro contando uma história
// que não aconteceu, e é justamente esse registro que tem valor numa disputa trabalhista.
func (s WarningStatus) CanTransitionTo(next WarningStatus) bool {
	if s == next {
		return true // reenviar o mesmo status é idempotente, não erro
	}
	switch s {
	case WarningDraft:
		return next == WarningSent
	case WarningSent:
		return next == WarningSigned
	default:
		return false
	}
}

// WarningFilter são os recortes da listagem de advertências.
type WarningFilter struct {
	TenantID       int
	CollaboratorID *int
	Status         *WarningStatus
}

// WarningRepository é o contrato de persistência das advertências.
type WarningRepository interface {
	Create(warning *Warning) error
	Update(warning *Warning) error
	UpdateStatus(id int, status WarningStatus, at time.Time) error
	GetByID(id int) (*Warning, error)
	List(filter WarningFilter) ([]Warning, error)
	Delete(id int) error
}
