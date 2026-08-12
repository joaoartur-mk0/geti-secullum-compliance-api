package models

import "time"

// Warning é o registro do processo de advertência. Sem hash de documento por ora — o
// que se acompanha aqui é o estado (rascunho → enviada → assinada) e quando cada etapa
// aconteceu.
type Warning struct {
	ID       int `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID int `gorm:"not null;index" json:"tenant_id"`

	OccurrenceID *int `gorm:"index" json:"occurrence_id"`

	CollaboratorID   int    `gorm:"not null;index" json:"collaborator_id"`
	CollaboratorName string `gorm:"type:varchar(255)" json:"collaborator_name"`
	BranchID         *int   `gorm:"index" json:"branch_id"`

	Body   string `gorm:"type:text" json:"body"`
	Status string `gorm:"type:varchar(20);not null;index" json:"status"`

	CreatedByUserID *int       `gorm:"index" json:"created_by_user_id"`
	CreatedAt       time.Time  `gorm:"type:timestamp;not null" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"type:timestamp;not null" json:"updated_at"`
	SentAt          *time.Time `gorm:"type:timestamp" json:"sent_at"`
	SignedAt        *time.Time `gorm:"type:timestamp" json:"signed_at"`
}
