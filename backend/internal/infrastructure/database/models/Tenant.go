package models

import (
	"gorm.io/datatypes"
)

type Tenant struct {
	ID                 int             `gorm:"primaryKey;autoIncrement" json:"id"`
	SecullumDatabaseID int             `gorm:"not null;uniqueIndex" json:"secullum_database_id"`
	Name               string          `gorm:"type:varchar(255);not null" json:"name"`
	Active             bool            `gorm:"not null;default:true" json:"active"`
	Settings           *TenantSettings `gorm:"foreignKey:TenantID" json:"settings,omitempty"`
	Collaborators      []Collaborator  `gorm:"foreignKey:TenantID" json:"-"`
	Staffs             []Staff         `gorm:"foreignKey:TenantID" json:"-"`
	Reports            []Report        `gorm:"foreignKey:TenantID" json:"-"`
}

type TenantSettings struct {
	ID           int  `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID     int  `gorm:"not null;uniqueIndex" json:"tenant_id"`
	Almoco       bool `gorm:"default:false" json:"almoco"`
	Interjornada bool `gorm:"default:false" json:"interjornada"`
	Hextras      bool `gorm:"default:false" json:"hextras"`
	Esquecimento bool `gorm:"default:false" json:"esquecimento"`

	// Severidade configurável por regra (ALERTA | CRITICO). Default CRITICO.
	AlmocoSeverity       string `gorm:"type:varchar(10);default:'CRITICO'" json:"almoco_severity"`
	InterjornadaSeverity string `gorm:"type:varchar(10);default:'CRITICO'" json:"interjornada_severity"`
	EsquecimentoSeverity string `gorm:"type:varchar(10);default:'CRITICO'" json:"esquecimento_severity"`

	Horarios datatypes.JSON `gorm:"type:jsonb" json:"horarios"`
}
