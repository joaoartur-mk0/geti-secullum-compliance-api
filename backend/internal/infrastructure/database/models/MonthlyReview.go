package models

import "time"

type MonthlyReview struct {
	ID          int    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID    int    `gorm:"not null;uniqueIndex:idx_monthly_review_tenant_competencia" json:"tenant_id"`
	Competencia string `gorm:"type:char(7);not null;uniqueIndex:idx_monthly_review_tenant_competencia" json:"competencia"`
	Status      string `gorm:"type:varchar(20);not null;default:'aberta'" json:"status"`

	PayrollDone bool `gorm:"not null;default:false" json:"payroll_done"`
	OffsetsDone bool `gorm:"not null;default:false" json:"offsets_done"`

	ClosedAt       *time.Time `json:"closed_at"`
	ClosedByUserID *int       `json:"closed_by_user_id"`

	ReopenedAt       *time.Time `json:"reopened_at"`
	ReopenedByUserID *int       `json:"reopened_by_user_id"`
	ReopenReason     string     `gorm:"type:text" json:"reopen_reason"`
}

type MonthlyReviewEvent struct {
	ID              int       `gorm:"primaryKey;autoIncrement" json:"id"`
	MonthlyReviewID int       `gorm:"not null;index" json:"monthly_review_id"`
	TenantID        int       `gorm:"not null;index" json:"tenant_id"`
	Type            string    `gorm:"type:varchar(20);not null" json:"type"`
	Reason          string    `gorm:"type:text" json:"reason"`
	ActorUserID     int       `gorm:"not null" json:"actor_user_id"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
}
