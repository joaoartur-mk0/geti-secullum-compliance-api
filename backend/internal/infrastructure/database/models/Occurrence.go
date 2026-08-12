package models

import "time"

// Occurrence é o estado atual de uma inconsistência.
//
// O índice único idx_occurrence_identity (tenant + colaborador + data + tipo) é o que
// garante, NO BANCO, que auditar o mesmo dia várias vezes não duplique ocorrências —
// mesmo se dois workers processarem o mesmo tenant simultaneamente.
type Occurrence struct {
	ID       int `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID int `gorm:"not null;index;uniqueIndex:idx_occurrence_identity,priority:1" json:"tenant_id"`

	// CollaboratorID é o id do funcionário na Secullum (mesmo espaço de
	// Collaborator.SecullumID), não a chave primária local — é a identidade estável
	// mesmo que o espelho local seja recriado.
	CollaboratorID   int       `gorm:"not null;index;uniqueIndex:idx_occurrence_identity,priority:2" json:"collaborator_id"`
	Date             time.Time `gorm:"type:date;not null;index;uniqueIndex:idx_occurrence_identity,priority:3" json:"date"`
	Type             string    `gorm:"type:varchar(80);not null;uniqueIndex:idx_occurrence_identity,priority:4" json:"type"`
	CollaboratorName string    `gorm:"type:varchar(255)" json:"collaborator_name"`

	Severity    string `gorm:"type:varchar(20);not null" json:"severity"`
	Description string `gorm:"type:text;not null" json:"description"`
	// Fingerprint do valor apurado: é a comparação dele que distingue "ocorrência
	// repetida" (nada muda) de "ocorrência com valor novo" (vira atualizada).
	Fingerprint string `gorm:"type:char(64);not null" json:"fingerprint"`

	State       string     `gorm:"type:varchar(30);not null;index" json:"state"`
	FirstSeenAt time.Time  `gorm:"type:timestamp;not null" json:"first_seen_at"`
	LastSeenAt  time.Time  `gorm:"type:timestamp;not null" json:"last_seen_at"`
	TimesSeen   int        `gorm:"not null;default:1" json:"times_seen"`
	ResolvedAt  *time.Time `gorm:"type:timestamp" json:"resolved_at"`

	IgnoredReason   string `gorm:"type:text" json:"ignored_reason"`
	IgnoredByUserID *int   `gorm:"index" json:"ignored_by_user_id"`
}

// OccurrenceEvent é o log append-only das transições. Nada aqui é atualizado ou apagado:
// é o rastro que explica por que uma ocorrência está no estado em que está.
type OccurrenceEvent struct {
	ID           int    `gorm:"primaryKey;autoIncrement" json:"id"`
	OccurrenceID int    `gorm:"not null;index" json:"occurrence_id"`
	TenantID     int    `gorm:"not null;index" json:"tenant_id"`
	Type         string `gorm:"type:varchar(30);not null" json:"type"`
	FromState    string `gorm:"type:varchar(30)" json:"from_state"`
	ToState      string `gorm:"type:varchar(30);not null" json:"to_state"`

	FromDescription string `gorm:"type:text" json:"from_description"`
	ToDescription   string `gorm:"type:text" json:"to_description"`

	Reason      string    `gorm:"type:text" json:"reason"`
	ActorUserID *int      `gorm:"index" json:"actor_user_id"`
	CreatedAt   time.Time `gorm:"type:timestamp;not null" json:"created_at"`
}
