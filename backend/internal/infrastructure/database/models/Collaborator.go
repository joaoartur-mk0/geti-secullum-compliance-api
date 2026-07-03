package models

type Collaborator struct {
	ID         int    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID   int    `gorm:"not null;index" json:"tenant_id"`
	SecullumID int    `gorm:"not null;index" json:"secullum_id"`
	Cpf        string `gorm:"type:varchar(20)" json:"cpf"`
	Celular    string `gorm:"type:varchar(20)" json:"celular"`

	// Relacionamentos GORM
	Schedules []CollaboratorSchedule `gorm:"foreignKey:CollaboratorID" json:"schedules,omitempty"`
}

type CollaboratorSchedule struct {
	ID             int    `gorm:"primaryKey;autoIncrement" json:"id"`
	CollaboratorID int    `gorm:"not null;index" json:"collaborator_id"`
	Entrada1       string `gorm:"type:time" json:"entrada_1"` // GORM converterá "15:04:05" para TIME no Postgres
	Saida1         string `gorm:"type:time" json:"saida_1"`
	Entrada2       string `gorm:"type:time" json:"entrada_2"`
	Saida2         string `gorm:"type:time" json:"saida_2"`
}
