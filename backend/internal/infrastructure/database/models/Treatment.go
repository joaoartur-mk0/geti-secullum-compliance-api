package models

import "time"

type Treatment struct {
	ID             int        `gorm:"primaryKey;autoIncrement" json:"id"`
	OccurrenceID   int        `gorm:"not null;index" json:"occurrence_id"`
	TenantID       int        `gorm:"not null;index" json:"tenant_id"`
	Justification  string     `gorm:"type:text;not null" json:"justification"`
	ActorUserID    int        `gorm:"not null" json:"actor_user_id"`
	CreatedAt      time.Time  `gorm:"not null" json:"created_at"`
	UndoneAt       *time.Time `json:"undone_at"`
	UndoneByUserID *int       `json:"undone_by_user_id"`

	Attachments []Attachment `gorm:"foreignKey:TreatmentID" json:"attachments,omitempty"`
}

// Attachment guarda o PDF direto no banco (bytea) — decisão registrada em
// docs/documento-funcional-compliance.md §7.3. Nunca servido por caminho estático.
type Attachment struct {
	ID          int       `gorm:"primaryKey;autoIncrement" json:"id"`
	TreatmentID int       `gorm:"not null;index" json:"treatment_id"`
	TenantID    int       `gorm:"not null;index" json:"tenant_id"`
	FileName    string    `gorm:"type:varchar(255)" json:"file_name"`
	ContentType string    `gorm:"type:varchar(100)" json:"content_type"`
	SizeBytes   int       `gorm:"not null" json:"size_bytes"`
	Data        []byte    `gorm:"type:bytea;not null" json:"-"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
}

// AttachmentDownload é o log de acesso — obrigatório porque o anexo é dado de saúde.
type AttachmentDownload struct {
	ID           int       `gorm:"primaryKey;autoIncrement" json:"id"`
	AttachmentID int       `gorm:"not null;index" json:"attachment_id"`
	UserID       int       `gorm:"not null" json:"user_id"`
	DownloadedAt time.Time `gorm:"not null" json:"downloaded_at"`
}
