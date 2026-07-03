package models

import (
	"time"

	"gorm.io/datatypes"
)

type Report struct {
	ID            int            `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID      int            `gorm:"not null;index" json:"tenant_id"`
	Report        datatypes.JSON `gorm:"type:jsonb;not null" json:"report"` // Payload de Inconsistências
	DataGenerated time.Time      `gorm:"type:timestamp;not null" json:"data_generated"`
	Date          time.Time      `gorm:"type:date;not null;index" json:"date"`
}
