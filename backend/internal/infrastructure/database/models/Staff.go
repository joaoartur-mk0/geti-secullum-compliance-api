package models

type Staff struct {
	ID       int    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID int    `gorm:"not null;index" json:"tenant_id"`
	Name     string `gorm:"type:varchar(255);not null" json:"name"`
	Celular  string `gorm:"type:varchar(20);not null" json:"celular"`
}
