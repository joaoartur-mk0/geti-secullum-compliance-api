package domain

import "time"

type Severity string

const (
	SeverityAlert    Severity = "ALERTA"
	SeverityCritical Severity = "CRITICO"
)

func (s Severity) OrDefault(def Severity) Severity {
	if s == "" {
		return def
	}
	return s
}

type DailyPunch struct {
	CollaboratorID int
	Date           time.Time
	Entrada1       *string
	Saida1         *string
	Entrada2       *string
	Saida2         *string
}

type AuditInconsistency struct {
	CollaboratorID   int
	CollaboratorName string
	Type             string
	Severity         Severity
	Description      string
}

type Report struct {
	ID              int
	TenantID        int
	Date            time.Time // Data alvo avaliada (D-1)
	DataGenerated   time.Time // Momento da geração
	Inconsistencies []AuditInconsistency
}

type ReportRepository interface {
	Save(report *Report) error
	ListByTenant(tenantID int) ([]Report, error)
}

type SecullumService interface {
	GetDailyPunches(tenant *Tenant, date time.Time) ([]DailyPunch, error)
	GetCollaborators(tenant *Tenant) ([]Collaborator, error)
}
