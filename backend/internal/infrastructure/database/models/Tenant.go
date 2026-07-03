package models

import (
	"gorm.io/datatypes"
)

type Tenant struct {
	ID                 int    `gorm:"primaryKey;autoIncrement" json:"id"`
	SecullumDatabaseID int    `gorm:"not null" json:"secullum_database_id"`
	Name               string `gorm:"type:varchar(255);not null" json:"name"`

	// Credenciais ocultas necessárias para a automação (conforme documentação técnica)
	SecullumToken   string `gorm:"type:text" json:"-"`
	EvolutionAPIUrl string `gorm:"type:varchar(255)" json:"-"`
	EvolutionAPIKey string `gorm:"type:varchar(255)" json:"-"`

	// Relacionamentos GORM
	Settings      *TenantSettings `gorm:"foreignKey:TenantID" json:"settings,omitempty"`
	Collaborators []Collaborator  `gorm:"foreignKey:TenantID" json:"-"`
	Staffs        []Staff         `gorm:"foreignKey:TenantID" json:"-"`
	Reports       []Report        `gorm:"foreignKey:TenantID" json:"-"`
}

type TenantSettings struct {
	ID           int  `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID     int  `gorm:"not null;uniqueIndex" json:"tenant_id"`
	Almoco       bool `gorm:"default:true" json:"almoco"`
	Interjornada bool `gorm:"default:true" json:"interjornada"`
	Hextras      bool `gorm:"default:true" json:"hextras"`
	Esquecimento bool `gorm:"default:true" json:"esquecimento"`
	// Usando datatypes.JSON do GORM para gerenciar o array de horários de varredura
	Horarios datatypes.JSON `gorm:"type:jsonb" json:"horarios"`
}
