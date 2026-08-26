package models

import "time"

// PunchRecord é o enriquecimento (equipamento/motivo) do dia auditado de um colaborador,
// cruzado a partir do endpoint FonteDados da Secullum — ver domain.PunchRecord.
type PunchRecord struct {
	ID             int       `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID       int       `gorm:"not null;uniqueIndex:idx_punch_record_identity,priority:1" json:"tenant_id"`
	CollaboratorID int       `gorm:"not null;uniqueIndex:idx_punch_record_identity,priority:2" json:"collaborator_id"`
	Date           time.Time `gorm:"type:date;not null;uniqueIndex:idx_punch_record_identity,priority:3" json:"date"`

	EquipamentoID *int    `json:"equipamento_id"`
	Motivo        *string `gorm:"type:text" json:"motivo"`
}
