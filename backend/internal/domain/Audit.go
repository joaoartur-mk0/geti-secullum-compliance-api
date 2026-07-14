package domain

import "time"

// Severity define o nível de criticidade da infração (Alerta Preventivo vs Crítico)
type Severity string

const (
	SeverityAlert    Severity = "ALERTA"
	SeverityCritical Severity = "CRITICO"
)

// OrDefault retorna a própria severidade se configurada; caso contrário, o default.
// Usado quando o tenant não parametrizou a severidade de uma regra.
func (s Severity) OrDefault(def Severity) Severity {
	if s == "" {
		return def
	}
	return s
}

// DailyPunch representa a marcação extraída limpa da API SecullumWEB
// Diferente da resposta bruta cheia de metadados, o domínio só precisa das horas
type DailyPunch struct {
	CollaboratorID int
	Date           time.Time
	Entrada1       *string // Ponteiros para lidar com valores nulos (esquecimentos)
	Saida1         *string
	Entrada2       *string
	Saida2         *string
}

// AuditInconsistency é o objeto gerado quando uma regra de negócio é violada
type AuditInconsistency struct {
	CollaboratorID   int
	CollaboratorName string
	Type             string // Ex: "Interjornada Curta", "Hora Extra Excedente"
	Severity         Severity
	Description      string
}

// Report é o consolidado que será salvo no banco de dados na madrugada
type Report struct {
	ID              int
	TenantID        int
	Date            time.Time // Data alvo avaliada (D-1)
	DataGenerated   time.Time // Momento da geração
	Inconsistencies []AuditInconsistency
}

// ReportRepository é o contrato para gravar e consultar os relatórios de auditoria.
type ReportRepository interface {
	Save(report *Report) error
	ListByTenant(tenantID int) ([]Report, error)
}

// SecullumService é o contrato (Interface) que a camada de Infraestrutura HTTP vai implementar.
// O motor de auditoria chamará isso para buscar as batidas em tempo real.
type SecullumService interface {
	GetDailyPunches(tenant *Tenant, date time.Time) ([]DailyPunch, error)
	GetCollaborators(tenant *Tenant) ([]Collaborator, error)
}
