package domain

import "time"

type Severity string

const (
	SeverityAlert    Severity = "ALERTA"
	SeverityCritical Severity = "CRITICO"
	// SeverityOperational marca o que NÃO é infração trabalhista, e sim cadastro
	// desatualizado — tipicamente a escala mensal variável que o gestor trocou sem
	// registrar na Secullum. Exige correção do cadastro, não ação disciplinar, e some
	// sozinha assim que a escala é corrigida.
	SeverityOperational Severity = "OPERACIONAL"
)

func (s Severity) OrDefault(def Severity) Severity {
	if s == "" {
		return def
	}
	return s
}

// Valid indica se o valor é uma das três severidades conhecidas (validação de query
// string em ?severity=).
func (s Severity) Valid() bool {
	switch s {
	case SeverityAlert, SeverityCritical, SeverityOperational:
		return true
	}
	return false
}

// PunchPair é um bloco de trabalho (entrada + saída correspondente). A Secullum
// devolve até 5 pares por dia; um ponteiro nil significa "marcação ausente".
type PunchPair struct {
	Entrada *string // "HH:MM"
	Saida   *string // "HH:MM"

	// FonteDadosIDEntrada/FonteDadosIDSaida são os ids que a Secullum já devolve junto
	// de cada marcação (FonteDadosIdEntradaN/FonteDadosIdSaidaN na resposta de Batidas),
	// usados para cruzar com o endpoint FonteDados e obter EquipamentoId/Motivo — sem
	// precisar combinar FuncionarioId+Data+Hora.
	FonteDadosIDEntrada *int
	FonteDadosIDSaida   *int
}

// DailyPunch é o cartão de um colaborador em um dia.
//
// Marcacoes são as batidas efetivamente registradas. Previstas é a jornada que a
// SecullumWEB alocou para AQUELE dia (campos Memoria* da resposta de batidas) — é a
// única fonte confiável da carga esperada, porque considera escala, horário
// alternativo, feriado e trocas pontuais, coisas que a grade semanal do horário
// (Horarios/Dias) não reflete.
type DailyPunch struct {
	CollaboratorID int
	Date           time.Time
	Marcacoes      []PunchPair
	Previstas      []PunchPair
	Folga          bool // dia de folga/DSR segundo a Secullum
	Neutro         bool // dia neutro: não gera saldo, sem carga a cumprir

	// EquipIDs são os aparelhos (relógios de ponto) em que as marcações do dia foram
	// registradas, sem repetição e na ordem de uso. Vazio quando todas as batidas vieram
	// do app/web ou de inclusão manual — o caso mais comum nos dados reais desta
	// operação, e a razão de a resolução de filial ter um fallback por nº de folha.
	EquipIDs []int
}

// ParseClock interpreta um horário "HH:MM" como minutos desde a meia-noite.
func ParseClock(hhmm string) (int, error) {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}

// MinutesBetween devolve a duração em minutos entre dois horários "HH:MM", tratando a
// virada de meia-noite (fim anterior ao início = dia seguinte).
func MinutesBetween(start, end string) (int, error) {
	s, err := ParseClock(start)
	if err != nil {
		return 0, err
	}
	e, err := ParseClock(end)
	if err != nil {
		return 0, err
	}
	d := e - s
	if d < 0 {
		d += 24 * 60
	}
	return d, nil
}

// sumPairs soma a duração dos pares completos. Devolve ok=false se nenhum par estiver
// completo (nada a somar).
func sumPairs(pairs []PunchPair) (int, bool, error) {
	total, found := 0, false
	for _, p := range pairs {
		if p.Entrada == nil || p.Saida == nil {
			continue
		}
		d, err := MinutesBetween(*p.Entrada, *p.Saida)
		if err != nil {
			return 0, false, err
		}
		total += d
		found = true
	}
	return total, found, nil
}

// PunchCount conta as marcações registradas (entradas + saídas), independentemente de
// formarem pares completos.
func (p DailyPunch) PunchCount() int {
	n := 0
	for _, pair := range p.Marcacoes {
		if pair.Entrada != nil {
			n++
		}
		if pair.Saida != nil {
			n++
		}
	}
	return n
}

// WorkedMinutes soma o tempo efetivamente trabalhado (todos os blocos completos).
func (p DailyPunch) WorkedMinutes() (int, bool, error) { return sumPairs(p.Marcacoes) }

// ExpectedMinutes soma a carga que a Secullum previu para o dia. ok=false significa que
// não há jornada prevista (folga, dia neutro ou cadastro incompleto) — o chamador
// decide como tratar.
func (p DailyPunch) ExpectedMinutes() (int, bool, error) { return sumPairs(p.Previstas) }

// FirstScheduledExit devolve a primeira saída prevista do dia (tipicamente a saída para
// a refeição), usada na varredura intra-dia de batida esquecida.
func (p DailyPunch) FirstScheduledExit() (string, bool) {
	for _, pair := range p.Previstas {
		if pair.Saida != nil {
			return *pair.Saida, true
		}
	}
	return "", false
}

// FirstEntry devolve a primeira batida de entrada do dia.
func (p DailyPunch) FirstEntry() (string, bool) {
	for _, pair := range p.Marcacoes {
		if pair.Entrada != nil {
			return *pair.Entrada, true
		}
	}
	return "", false
}

// LastExit devolve a última batida de saída do dia (qualquer que seja o bloco), base
// correta para a interjornada — o expediente pode encerrar no 1º, 2º ou 5º bloco.
func (p DailyPunch) LastExit() (string, bool) {
	last, found := "", false
	for _, pair := range p.Marcacoes {
		if pair.Saida != nil {
			last, found = *pair.Saida, true
		}
	}
	return last, found
}

// FirstBreak devolve a duração, em minutos, do primeiro intervalo entre dois blocos
// trabalhados (saída de um bloco até a entrada do seguinte).
func (p DailyPunch) FirstBreak() (int, bool, error) {
	for i := 0; i+1 < len(p.Marcacoes); i++ {
		saida, entrada := p.Marcacoes[i].Saida, p.Marcacoes[i+1].Entrada
		if saida == nil || entrada == nil {
			continue
		}
		d, err := MinutesBetween(*saida, *entrada)
		if err != nil {
			return 0, false, err
		}
		return d, true, nil
	}
	return 0, false, nil
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

// ReportRepository persiste e consulta as varreduras de auditoria (uma execução por
// chamada de Save — reauditar o mesmo dia grava um novo registro, nunca sobrescreve).
//
// start/end (ambos opcionais, nil = sem limite) filtram por Report.Date, para consultar
// e filtrar por período completo (semana, mês, intervalo customizado).
type ReportRepository interface {
	Save(report *Report) error
	// ListByTenant devolve o HISTÓRICO completo (todas as execuções, inclusive
	// reauditorias do mesmo dia), da mais recente para a mais antiga.
	ListByTenant(tenantID int, start, end *time.Time) ([]Report, error)
	// ListLatestByTenant devolve só a execução MAIS RECENTE de cada dia — o estado
	// atual da auditoria, sem o ruído de reauditorias.
	ListLatestByTenant(tenantID int, start, end *time.Time) ([]Report, error)
}

// PunchRecord enriquece o dia auditado de um colaborador com a origem da marcação —
// equipamento e motivo (inclusão manual, abono, etc.) — apurados cruzando o Id que a
// própria resposta de batidas traz (PunchPair.FonteDadosIDEntrada/FonteDadosIDSaida) com
// o retorno de GetFonteDados no mesmo período (ver usecase.AuditorService/AuditConsumer).
//
// Uma linha por (tenant, colaborador, data): quando o dia tem mais de uma marcação com
// fontes diferentes, EquipamentoID/Motivo refletem a PRIMEIRA marcação do dia com
// correspondência encontrada — suficiente para apontar "de onde veio o registro do dia"
// sem multiplicar linhas por marcação.
type PunchRecord struct {
	ID             int
	TenantID       int
	CollaboratorID int // id do funcionário na Secullum
	Date           time.Time

	EquipamentoID *int
	Motivo        *string
}

// PunchRecordRepository persiste e consulta o enriquecimento de origem da marcação por
// dia auditado.
type PunchRecordRepository interface {
	// SaveAll faz upsert por (tenant_id, collaborator_id, date).
	SaveAll(records []PunchRecord) error
	// GetByCollaborator devolve os registros de um colaborador no período [start, end]
	// (ambos inclusive), do dia mais antigo ao mais recente — consumido pelo endpoint que
	// expõe equipamento/motivo ao painel (sem isto, o enriquecimento seria gravado e nunca
	// lido por ninguém).
	GetByCollaborator(tenantID, collaboratorID int, start, end time.Time) ([]PunchRecord, error)
}

type SecullumService interface {
	GetDailyPunches(tenant *Tenant, date time.Time) ([]DailyPunch, error)
	// GetDailyPunchesRange busca as batidas de TODO um período (start a end, inclusive)
	// numa única chamada à Secullum — usada pela auditoria de período completo (semana,
	// mês) para não fazer uma requisição por dia e estourar o rate limiting da API.
	GetDailyPunchesRange(tenant *Tenant, start, end time.Time) ([]DailyPunch, error)
	GetCollaborators(tenant *Tenant) ([]Collaborator, error)
	// GetHorario busca a jornada contratual (por dia da semana) associada ao número de
	// horário do funcionário na Secullum (Funcionario.Horario.Numero).
	GetHorario(tenant *Tenant, numero int) ([]CollaboratorSchedule, error)
	// GetEquipamentos busca os aparelhos (relógios de ponto) cadastrados na Secullum
	// para o tenant.
	GetEquipamentos(tenant *Tenant) ([]Equipment, error)
	// GetFonteDados busca, para um período completo, a origem de cada marcação
	// registrada (aparelho e motivo de inclusão manual) — usado para enriquecer a
	// auditoria com EquipamentoId/Motivo cruzando pelo id retornado nas próprias
	// batidas (PunchPair.FonteDadosIDEntrada/FonteDadosIDSaida).
	GetFonteDados(tenant *Tenant, start, end time.Time) ([]FonteDadoItem, error)
}

// FonteDadoItem é um registro de origem de marcação da Secullum (endpoint FonteDados).
// O Id é a chave de correlação com PunchPair.FonteDadosIDEntrada/FonteDadosIDSaida.
type FonteDadoItem struct {
	ID            int
	EquipamentoID *int
	Motivo        *string
}
