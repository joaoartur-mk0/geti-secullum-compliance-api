package domain

import "time"

// Treatment é o registro de uma tratativa aplicada a uma ocorrência — Feature 4 do ciclo
// de evolução (ver docs/documento-funcional-compliance.md §7.2). Nunca é apagado: desfazer
// grava um evento novo e marca UndoneAt/UndoneByUserID na própria linha, mas o registro
// original permanece — é o que torna a trilha auditável.
type Treatment struct {
	ID             int
	OccurrenceID   int
	TenantID       int
	Justification  string
	Attachments    []Attachment
	ActorUserID    int
	CreatedAt      time.Time
	UndoneAt       *time.Time
	UndoneByUserID *int
}

// Attachment é o anexo (PDF) vinculado a uma tratativa — atestado médico, acordo de
// compensação. Guardado no banco local, nunca servido por URL estática (ver
// docs/documento-funcional-compliance.md §7.3): é dado sensível de saúde, LGPD art. 11.
type Attachment struct {
	ID          int
	TreatmentID int
	TenantID    int
	FileName    string
	ContentType string
	SizeBytes   int
	Data        []byte
	CreatedAt   time.Time
}

// AttachmentDownload registra quem baixou um anexo. Obrigatório pela mesma razão do
// armazenamento restrito: sem log de acesso, um atestado médico circula sem rastro.
type AttachmentDownload struct {
	ID           int
	AttachmentID int
	UserID       int
	DownloadedAt time.Time
}

// TreatmentRepository é o contrato de persistência de tratativas e anexos. O tenant não
// entra como parâmetro — mesmo padrão de OccurrenceRepository.Ignore: a rota só conhece o
// id do recurso, o tenant é resolvido pelo handler ao carregar o registro antes de agir
// (ver ensureTenantAccess em occurrence_handler.go).
type TreatmentRepository interface {
	// Treat grava a tratativa e o(s) anexo(s) e transiciona a ocorrência para `tratada`
	// numa única transação. Devolve NewConflict se a ocorrência já tiver desfecho —
	// tratativa não sobrescreve desfecho existente, o usuário precisa desfazer primeiro.
	Treat(occurrenceID int, justification string, attachments []Attachment, actorUserID int) (*Treatment, error)
	// GetByID carrega uma tratativa — usado pelo handler para descobrir o tenant antes de
	// autorizar Undo (mesmo padrão de OccurrenceRepository.GetByID + ensureTenantAccess).
	GetByID(treatmentID int) (*Treatment, error)
	// Undo desfaz uma tratativa: marca UndoneAt/UndoneByUserID e devolve a ocorrência
	// para `aberta`. Não apaga o registro original — grava OccurrenceEvent novo.
	Undo(treatmentID, actorUserID int) error
	ListByOccurrence(occurrenceID int) ([]Treatment, error)
	GetAttachment(attachmentID int) (*Attachment, error)
	RecordDownload(attachmentID, userID int) error
}
