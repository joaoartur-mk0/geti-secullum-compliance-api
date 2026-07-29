package usecase

import (
	"errors"
	"fmt"
	"math"
	"time"

	"backend/internal/domain"
)

// defaultStandardHours é a carga diária assumida quando o colaborador ainda não
// tem uma jornada contratual sincronizada localmente.
const defaultStandardHours = 8.0

type AuditorService struct{}

func NewAuditorService() *AuditorService {
	return &AuditorService{}
}

// ProcessRules é a função orquestradora. Recebe as configurações do Inquilino,
// os dados do colaborador, as batidas de hoje e de ontem (necessário para interjornada).
//
// isClosing indica se esta é a avaliação de FECHAMENTO (consolidação noturna do dia
// encerrado) ou uma varredura INTRA-DIA (expediente em andamento). Regras que só fazem
// sentido no fim do dia (ex.: contagem ímpar de batidas) são aplicadas apenas no
// fechamento, evitando falsos positivos durante o expediente.
//
// Cada regra retorna, separadamente, uma possível inconsistência trabalhista e um
// possível erro de dado (ex.: batida em formato inválido). As inconsistências são
// acumuladas para o relatório; os erros de dado NÃO são descartados — são agregados
// via errors.Join e devolvidos ao chamador, que decide como registrá-los. Um erro em
// uma regra não impede a avaliação das demais, garantindo uma auditoria completa e
// rastreável.
func (s *AuditorService) ProcessRules(
	settings *domain.TenantSettings,
	collab *domain.Collaborator,
	todayPunch *domain.DailyPunch,
	yesterdayPunch *domain.DailyPunch,
	currentTime time.Time,
	isClosing bool,
) ([]domain.AuditInconsistency, error) {

	var infractions []domain.AuditInconsistency
	var ruleErrors []error

	if settings == nil {
		return infractions, nil // Se não há configuração, encerra a auditoria
	}

	// Severidades configuráveis por tenant (default CRITICO se não parametrizadas).
	almocoSev := settings.AlmocoSeverity.OrDefault(domain.SeverityCritical)
	interSev := settings.InterjornadaSeverity.OrDefault(domain.SeverityCritical)
	esqSev := settings.EsquecimentoSeverity.OrDefault(domain.SeverityCritical)

	// collect executa uma regra, acumulando a inconsistência e/ou o erro de dado.
	collect := func(ruleName string, inc *domain.AuditInconsistency, err error) {
		if err != nil {
			ruleErrors = append(ruleErrors, fmt.Errorf("regra %q: %w", ruleName, err))
			return
		}
		if inc != nil {
			infractions = append(infractions, *inc)
		}
	}

	// 1. Regra de Batidas Esquecidas / Incompletas
	if settings.Esquecimento {
		inc, err := s.checkMissingPunches(collab, todayPunch, currentTime, esqSev, isClosing)
		collect("batidas esquecidas", inc, err)
	}

	// 2. Regra de Intervalo de Almoço Reduzido (Art. 71 CLT)
	if settings.Almoco {
		inc, err := s.checkLunchBreak(todayPunch, almocoSev)
		collect("almoço reduzido", inc, err)
	}

	// 3. Regra de Interjornada Curta (Art. 66 CLT)
	if settings.Interjornada && yesterdayPunch != nil {
		inc, err := s.checkInterjornada(yesterdayPunch, todayPunch, interSev)
		collect("interjornada", inc, err)
	}

	// 4. Regra de Hora Extra Excedente (Art. 59 CLT) — severidade legal, não configurável.
	if settings.Hextras {
		inc, err := s.checkOvertime(collab, todayPunch)
		collect("hora extra", inc, err)
	}

	// errors.Join devolve nil se ruleErrors estiver vazio.
	return infractions, errors.Join(ruleErrors...)
}

// --- Helpers de tempo ---

// parseClock interpreta um horário "HH:MM".
func parseClock(hhmm string) (time.Time, error) {
	return time.Parse("15:04", hhmm)
}

// durationBetween calcula a duração entre dois horários "HH:MM", tratando a virada
// de meia-noite: se o fim for anterior ao início (ex.: entra 22:00, sai 02:00),
// assume que o fim está no dia seguinte e soma 24h.
func durationBetween(start, end time.Time) time.Duration {
	d := end.Sub(start)
	if d < 0 {
		d += 24 * time.Hour
	}
	return d
}

// formatHoursMinutes converte uma quantidade de horas (float, usada nos cálculos)
// numa representação legível ao usuário final "XhYYmin" (ex.: 19.97 -> "19h58min",
// 1.70 -> "1h42min", 9.0 -> "9h"). Arredonda para o minuto mais próximo somando tudo
// em minutos primeiro, o que também evita o caso de borda "60min" (ex.: 1.999h vira
// "2h", não "1h60min"). Espera valores não-negativos (as regras só formatam durações
// positivas).
func formatHoursMinutes(hours float64) string {
	totalMinutes := int(math.Round(hours * 60))
	h := totalMinutes / 60
	m := totalMinutes % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%02dmin", h, m)
}

// scheduledTimeOn projeta um horário contratual "HH:MM" na data de `day`,
// para comparar com o horário atual na varredura intra-dia.
func scheduledTimeOn(day time.Time, hhmm string) (time.Time, error) {
	t, err := parseClock(hhmm)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, day.Location()), nil
}

// scheduleForDate encontra, dentre as jornadas sincronizadas do colaborador, a que
// corresponde ao dia da semana de `date`. A Secullum define a jornada por dia (0 a 6,
// mesma convenção do time.Weekday do Go), então sábados/domingos/feriados podem ter
// cargas diferentes dos dias úteis.
func scheduleForDate(collab *domain.Collaborator, date time.Time) (domain.CollaboratorSchedule, bool) {
	if collab == nil {
		return domain.CollaboratorSchedule{}, false
	}
	weekday := int(date.Weekday())
	for _, s := range collab.Schedules {
		if s.DiaSemana == weekday {
			return s, true
		}
	}
	return domain.CollaboratorSchedule{}, false
}

// expectedDailyHours calcula a carga contratual do dia da semana de `date`.
//
// Prioriza CargaMinutos (o valor já calculado pela Secullum, mais confiável que
// recomputar a partir dos horários — considera tolerâncias e outras regras do
// horário). Se ausente, tenta somar os dois blocos Entrada/Saída. Se a jornada não foi
// sincronizada para este dia da semana, devolve ok=false (o chamador aplica um
// fallback). Um dia sincronizado como folga contratual (Carga=0, sem horários) devolve
// corretamente 0h esperadas — qualquer trabalho nesse dia conta inteiro como extra.
func expectedDailyHours(collab *domain.Collaborator, date time.Time) (float64, bool) {
	sch, ok := scheduleForDate(collab, date)
	if !ok {
		return 0, false
	}
	if sch.CargaMinutos > 0 {
		return float64(sch.CargaMinutos) / 60.0, true
	}
	if sch.Entrada1 != "" && sch.Saida1 != "" && sch.Entrada2 != "" && sch.Saida2 != "" {
		e1, err1 := parseClock(sch.Entrada1)
		s1, err2 := parseClock(sch.Saida1)
		e2, err3 := parseClock(sch.Entrada2)
		s2, err4 := parseClock(sch.Saida2)
		if err1 == nil && err2 == nil && err3 == nil && err4 == nil {
			total := durationBetween(e1, s1) + durationBetween(e2, s2)
			return total.Hours(), true
		}
	}
	// Dia sincronizado sem carga e sem horários: folga contratual (0h esperadas).
	return 0, true
}

// --- Métodos Privados com a Matemática das Regras Trabalhistas ---

// checkMissingPunches implementa a regra 5.3 da especificação em dois modos:
//   - Fechamento (isClosing): qualquer contagem ÍMPAR de batidas no dia encerrado
//     indica batida esquecida (5.3.4).
//   - Intra-dia (!isClosing): se o colaborador tem só 1 batida (entrada) e já passou
//     30min do horário previsto de saída para o almoço, é batida esquecida (5.3.3).
//     Requer jornada contratual sincronizada; sem ela, a regra intra-dia é suspensa.
func (s *AuditorService) checkMissingPunches(collab *domain.Collaborator, punch *domain.DailyPunch, now time.Time, severity domain.Severity, isClosing bool) (*domain.AuditInconsistency, error) {
	punchesCount := 0
	if punch.Entrada1 != nil {
		punchesCount++
	}
	if punch.Saida1 != nil {
		punchesCount++
	}
	if punch.Entrada2 != nil {
		punchesCount++
	}
	if punch.Saida2 != nil {
		punchesCount++
	}

	// Fechamento: contagem ímpar no dia já encerrado = batida esquecida.
	if isClosing {
		if punchesCount%2 != 0 {
			return &domain.AuditInconsistency{
				CollaboratorID: collab.ID,
				Type:           "Batida Esquecida",
				Severity:       severity,
				Description:    fmt.Sprintf("O colaborador encerrou o dia com número ímpar de marcações (%d batida(s)). Provável esquecimento.", punchesCount),
			}, nil
		}
		return nil, nil
	}

	// Intra-dia: precisa da jornada contratual do dia (de hoje) para saber o horário
	// previsto do almoço.
	sch, ok := scheduleForDate(collab, now)
	if !ok || sch.Saida1 == "" {
		return nil, nil // sem jornada sincronizada para hoje, a validação fica suspensa
	}

	lunchOut, err := scheduledTimeOn(now, sch.Saida1)
	if err != nil {
		return nil, err
	}

	// Só 1 batida (entrada) e já passou 30min do horário previsto de saída p/ almoço.
	if punchesCount == 1 && punch.Entrada1 != nil && now.After(lunchOut.Add(30*time.Minute)) {
		return &domain.AuditInconsistency{
			CollaboratorID: collab.ID,
			Type:           "Batida Esquecida",
			Severity:       severity,
			Description:    "O colaborador possui apenas a batida de entrada e já ultrapassou em 30min o horário previsto de saída para o almoço. Provável esquecimento.",
		}, nil
	}
	return nil, nil
}

func (s *AuditorService) checkLunchBreak(punch *domain.DailyPunch, severity domain.Severity) (*domain.AuditInconsistency, error) {
	if punch.Saida1 == nil || punch.Entrada2 == nil {
		return nil, nil // Se não bateu a saída ou a volta do almoço, não avalia intervalo ainda
	}

	saida, err := parseClock(*punch.Saida1)
	if err != nil {
		return nil, err
	}
	volta, err := parseClock(*punch.Entrada2)
	if err != nil {
		return nil, err
	}

	deltaMinutos := durationBetween(saida, volta).Minutes()

	// Critério: Período de intervalo para refeição e descanso menor do que 60 minutos.
	if deltaMinutos < 60 {
		return &domain.AuditInconsistency{
			CollaboratorID: punch.CollaboratorID,
			Type:           "Almoço Reduzido",
			Severity:       severity,
			Description:    fmt.Sprintf("O intervalo de almoço foi de apenas %.0f minutos, inferior ao limite legal de 60 minutos.", deltaMinutos),
		}, nil
	}
	return nil, nil
}

func (s *AuditorService) checkInterjornada(yesterday *domain.DailyPunch, today *domain.DailyPunch, severity domain.Severity) (*domain.AuditInconsistency, error) {
	if yesterday.Saida2 == nil || today.Entrada1 == nil {
		return nil, nil
	}

	saidaOntem, err := parseClock(*yesterday.Saida2)
	if err != nil {
		return nil, err
	}
	entradaHoje, err := parseClock(*today.Entrada1)
	if err != nil {
		return nil, err
	}

	// A interjornada cruza a meia-noite por definição; durationBetween soma 24h.
	deltaHoras := durationBetween(saidaOntem, entradaHoje).Hours()

	// Critério: Tempo de descanso inferior a 11 horas.
	if deltaHoras < 11 {
		return &domain.AuditInconsistency{
			CollaboratorID: today.CollaboratorID,
			Type:           "Interjornada Curta",
			Severity:       severity,
			Description:    fmt.Sprintf("O descanso entre jornadas foi de apenas %s. O mínimo exigido são 11 horas.", formatHoursMinutes(deltaHoras)),
		}, nil
	}
	return nil, nil
}

func (s *AuditorService) checkOvertime(collab *domain.Collaborator, punch *domain.DailyPunch) (*domain.AuditInconsistency, error) {
	if punch.Entrada1 == nil || punch.Saida1 == nil || punch.Entrada2 == nil || punch.Saida2 == nil {
		return nil, nil // Jornada incompleta, não calcula hora extra exata ainda
	}

	e1, err := parseClock(*punch.Entrada1)
	if err != nil {
		return nil, err
	}
	s1, err := parseClock(*punch.Saida1)
	if err != nil {
		return nil, err
	}
	e2, err := parseClock(*punch.Entrada2)
	if err != nil {
		return nil, err
	}
	s2, err := parseClock(*punch.Saida2)
	if err != nil {
		return nil, err
	}

	// Tempo trabalhado líquido (dois blocos), tratando virada de meia-noite.
	workedHours := (durationBetween(e1, s1) + durationBetween(e2, s2)).Hours()

	// Usa a carga contratual do dia da semana avaliado; se não sincronizada, cai no
	// padrão de 8h.
	standardHours := defaultStandardHours
	if h, ok := expectedDailyHours(collab, punch.Date); ok {
		standardHours = h
	}

	overtime := workedHours - standardHours
	if overtime <= 0 {
		return nil, nil
	}

	// Critério (Art. 59 CLT): > 1h e <= 2h => Alerta; > 2h => Crítico.
	if overtime > 2.0 {
		return &domain.AuditInconsistency{
			CollaboratorID: collab.ID,
			Type:           "Hora Extra Excedente",
			Severity:       domain.SeverityCritical,
			Description:    fmt.Sprintf("Limite legal estourado. O colaborador realizou %s de horas extras.", formatHoursMinutes(overtime)),
		}, nil
	}
	if overtime > 1.0 {
		return &domain.AuditInconsistency{
			CollaboratorID: collab.ID,
			Type:           "Alerta de Hora Extra",
			Severity:       domain.SeverityAlert,
			Description:    fmt.Sprintf("Atenção preventiva. O colaborador já realizou %s de horas extras.", formatHoursMinutes(overtime)),
		}, nil
	}
	return nil, nil
}
