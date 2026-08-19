package usecase

import (
	"errors"
	"fmt"
	"time"

	"backend/internal/domain"
)

// Limiares do intervalo intrajornada (Art. 71 da CLT), em minutos de carga do dia:
//   - jornada > 6h         -> intervalo mínimo de 60 min (caput)
//   - jornada > 4h e <= 6h -> intervalo mínimo de 15 min (§1º)
//   - jornada <= 4h        -> sem intervalo obrigatório
//
// É por isso que o domingo, cuja jornada prevista nesta operação é de 6h, exige apenas
// 15 min: a regra não é "domingo", é o tamanho da jornada. Derivar o limite da carga
// cobre igualmente qualquer turno curto em outro dia da semana.
const (
	jornadaComIntervalo60 = 6 * 60
	jornadaComIntervalo15 = 4 * 60
	intervaloMinimoLongo  = 60
	intervaloMinimoMedio  = 15

	descansoInterjornadaMinimo = 11 * 60
	limiteExtraCritico         = 120
	limiteExtraAlerta          = 60
)

// Tipos de inconsistência emitidos pelo motor.
const (
	TipoBatidaEsquecida = "Batida Esquecida"
	TipoAlmocoReduzido  = "Almoço Reduzido"
	TipoInterjornada    = "Interjornada Curta"
	TipoHoraExtra       = "Hora Extra Excedente"
	TipoAlertaHoraExtra = "Alerta de Hora Extra"
	TipoCargaNaoApurada = "Carga Horária Não Apurada"
	TipoTrabalhoEmFolga = "Trabalho em Dia de Folga/DSR"
)

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
// A carga esperada do dia vem SEMPRE da própria Secullum — da jornada que ela alocou
// para aquela data (DailyPunch.Previstas). Não existe carga padrão assumida: se o
// colaborador trabalhou e a Secullum não informou jornada para o dia, isso vira
// inconsistência no relatório em vez de virar hora extra fantasma.
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

	// 0. Qualificação do dia. Só faz sentido cobrar carga de quem efetivamente
	// trabalhou; e sem carga apurada as regras que dependem dela (hora extra e
	// intervalo) ficam suspensas — o problema é reportado, nunca estimado.
	//
	// Os dois desvios apontados aqui (trabalho em dia de folga e dia sem carga prevista)
	// nascem com severidade OPERACIONAL, não CRÍTICA: na escala mensal variável desta
	// operação, o caso comum é o gestor trocar colaboradores de dia sem atualizar a
	// Secullum. Isso é cadastro desatualizado, não infração da CLT — e, corrigida a
	// escala, a ocorrência deixa de ser apurada e se resolve sozinha na varredura
	// seguinte (ver usecase/reconciler.go). Uma folga que seja realmente folga continua
	// visível ao gestor, apenas sem o alarme de infração.
	worked, temTrabalho, errTrabalho := todayPunch.WorkedMinutes()
	if errTrabalho != nil {
		ruleErrors = append(ruleErrors, fmt.Errorf("regra %q: %w", "jornada trabalhada", errTrabalho))
	}
	expected, temCarga, errCarga := todayPunch.ExpectedMinutes()
	if errCarga != nil {
		ruleErrors = append(ruleErrors, fmt.Errorf("regra %q: %w", "carga prevista", errCarga))
	}
	dadosLegiveis := errTrabalho == nil && errCarga == nil

	switch {
	case !temTrabalho || !dadosLegiveis:
		// Sem jornada trabalhada apurável (ou dado ilegível): nada a comparar.
	case todayPunch.Folga:
		infractions = append(infractions, domain.AuditInconsistency{
			CollaboratorID: collab.ID,
			Type:           TipoTrabalhoEmFolga,
			Severity:       domain.SeverityOperational,
			Description: fmt.Sprintf(
				"O colaborador trabalhou %s em um dia registrado como folga/DSR na Secullum. Confirme se houve troca de escala não registrada; se a folga estiver correta, todo o período é extraordinário.",
				formatMinutes(worked),
			),
		})
	case !temCarga && !todayPunch.Neutro:
		infractions = append(infractions, domain.AuditInconsistency{
			CollaboratorID: collab.ID,
			Type:           TipoCargaNaoApurada,
			Severity:       domain.SeverityOperational,
			Description: fmt.Sprintf(
				"O colaborador registrou %s de trabalho, mas a Secullum não informou jornada prevista para o dia. Sem a carga contratual não é possível apurar hora extra nem intervalo — verifique o horário cadastrado para este colaborador.",
				formatMinutes(worked),
			),
		})
	}

	// Regras de carga só rodam com jornada trabalhada E carga prevista, ambas legíveis.
	// Num dia de folga a jornada inteira já foi apontada acima; reavaliá-la como hora
	// extra duplicaria a mesma infração.
	cargaApurada := temTrabalho && temCarga && dadosLegiveis && !todayPunch.Folga

	// 1. Regra de Batidas Esquecidas / Incompletas
	if settings.Esquecimento {
		inc, err := s.checkMissingPunches(collab, todayPunch, currentTime, esqSev, isClosing)
		collect("batidas esquecidas", inc, err)
	}

	// 2. Regra de Intervalo Intrajornada Reduzido (Art. 71 CLT)
	if settings.Almoco && cargaApurada {
		inc, err := s.checkLunchBreak(collab, todayPunch, expected, almocoSev)
		collect("intervalo intrajornada", inc, err)
	}

	// 3. Regra de Interjornada Curta (Art. 66 CLT)
	if settings.Interjornada && yesterdayPunch != nil {
		inc, err := s.checkInterjornada(collab, yesterdayPunch, todayPunch, interSev)
		collect("interjornada", inc, err)
	}

	// 4. Regra de Hora Extra Excedente (Art. 59 CLT) — severidade legal, não configurável.
	if settings.Hextras && cargaApurada {
		collect("hora extra", s.checkOvertime(collab, worked, expected), nil)
	}

	// errors.Join devolve nil se ruleErrors estiver vazio.
	return infractions, errors.Join(ruleErrors...)
}

// --- Helpers ---

// formatMinutes converte uma quantidade de minutos numa representação legível ao
// usuário final "XhYYmin" (ex.: 1198 -> "19h58min", 102 -> "1h42min", 540 -> "9h").
// Espera valores não-negativos (as regras só formatam durações positivas).
func formatMinutes(minutes int) string {
	h := minutes / 60
	m := minutes % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%02dmin", h, m)
}

// requiredBreakMinutes devolve o intervalo mínimo exigido pelo Art. 71 da CLT para uma
// jornada de `cargaMinutos`. Zero significa "sem intervalo obrigatório".
func requiredBreakMinutes(cargaMinutos int) int {
	switch {
	case cargaMinutos > jornadaComIntervalo60:
		return intervaloMinimoLongo
	case cargaMinutos > jornadaComIntervalo15:
		return intervaloMinimoMedio
	default:
		return 0
	}
}

// scheduledTimeOn projeta um horário previsto "HH:MM" na data de `day`, para comparar
// com o horário atual na varredura intra-dia.
func scheduledTimeOn(day time.Time, hhmm string) (time.Time, error) {
	minutes, err := domain.ParseClock(hhmm)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(day.Year(), day.Month(), day.Day(), minutes/60, minutes%60, 0, 0, day.Location()), nil
}

// --- Métodos Privados com a Matemática das Regras Trabalhistas ---

// checkMissingPunches implementa a regra 5.3 da especificação em dois modos:
//   - Fechamento (isClosing): qualquer contagem ÍMPAR de batidas no dia encerrado
//     indica batida esquecida (5.3.4).
//   - Intra-dia (!isClosing): se o colaborador tem só 1 batida (entrada) e já passou
//     30min do horário previsto da primeira saída, é batida esquecida (5.3.3).
//     Requer jornada prevista para o dia; sem ela, a regra intra-dia é suspensa.
func (s *AuditorService) checkMissingPunches(collab *domain.Collaborator, punch *domain.DailyPunch, now time.Time, severity domain.Severity, isClosing bool) (*domain.AuditInconsistency, error) {
	punchesCount := punch.PunchCount()

	// Fechamento: contagem ímpar no dia já encerrado = batida esquecida.
	if isClosing {
		if punchesCount%2 != 0 {
			return &domain.AuditInconsistency{
				CollaboratorID: collab.ID,
				Type:           TipoBatidaEsquecida,
				Severity:       severity,
				Description:    fmt.Sprintf("O colaborador encerrou o dia com número ímpar de marcações (%d batida(s)). Provável esquecimento.", punchesCount),
			}, nil
		}
		return nil, nil
	}

	// Intra-dia: precisa da jornada prevista de hoje para saber o horário da 1ª saída.
	saidaPrevista, ok := punch.FirstScheduledExit()
	if !ok {
		return nil, nil // sem jornada prevista para hoje, a validação fica suspensa
	}

	saida, err := scheduledTimeOn(now, saidaPrevista)
	if err != nil {
		return nil, err
	}

	// Só 1 batida (entrada) e já passou 30min do horário previsto da 1ª saída.
	if punchesCount == 1 && now.After(saida.Add(30*time.Minute)) {
		return &domain.AuditInconsistency{
			CollaboratorID: collab.ID,
			Type:           TipoBatidaEsquecida,
			Severity:       severity,
			Description:    "O colaborador possui apenas a batida de entrada e já ultrapassou em 30min o horário previsto da primeira saída. Provável esquecimento.",
		}, nil
	}
	return nil, nil
}

// checkLunchBreak compara o primeiro intervalo efetivamente registrado com o mínimo
// legal correspondente à carga prevista do dia (Art. 71 CLT).
//
// Domingo é exceção: o piso exigido é sempre 15min, independente da carga prevista do
// dia — a escala de domingo costuma ser trabalho extraordinário com duração variável, e a
// regra graduada (60min acima de 6h) não reflete esse cenário. A regra graduada
// simplesmente não roda no domingo: só o piso de 15min é cobrado (nem mais, nem menos).
func (s *AuditorService) checkLunchBreak(collab *domain.Collaborator, punch *domain.DailyPunch, cargaMinutos int, severity domain.Severity) (*domain.AuditInconsistency, error) {
	minimo := requiredBreakMinutes(cargaMinutos)
	if punch.Date.Weekday() == time.Sunday {
		minimo = intervaloMinimoMedio
	}
	if minimo == 0 {
		return nil, nil // jornada de até 4h: intervalo não é obrigatório
	}

	intervalo, ok, err := punch.FirstBreak()
	if err != nil {
		return nil, err
	}
	if !ok {
		// Jornada registrada em bloco único: não há intervalo a avaliar. A ausência de
		// intervalo numa jornada longa aparece como marcação faltante na regra 5.3.
		return nil, nil
	}

	if intervalo < minimo {
		return &domain.AuditInconsistency{
			CollaboratorID: collab.ID,
			Type:           TipoAlmocoReduzido,
			Severity:       severity,
			Description: fmt.Sprintf(
				"O intervalo foi de apenas %d minutos, inferior ao mínimo de %d minutos exigido para a jornada de %s prevista no dia.",
				intervalo, minimo, formatMinutes(cargaMinutos),
			),
		}, nil
	}
	return nil, nil
}

// checkInterjornada usa a ÚLTIMA saída de ontem e a PRIMEIRA entrada de hoje — o
// expediente pode encerrar em qualquer um dos blocos, não só no segundo.
func (s *AuditorService) checkInterjornada(collab *domain.Collaborator, yesterday *domain.DailyPunch, today *domain.DailyPunch, severity domain.Severity) (*domain.AuditInconsistency, error) {
	saidaOntem, okSaida := yesterday.LastExit()
	entradaHoje, okEntrada := today.FirstEntry()
	if !okSaida || !okEntrada {
		return nil, nil
	}

	// A interjornada cruza a meia-noite por definição; MinutesBetween soma 24h.
	delta, err := domain.MinutesBetween(saidaOntem, entradaHoje)
	if err != nil {
		return nil, err
	}

	if delta < descansoInterjornadaMinimo {
		return &domain.AuditInconsistency{
			CollaboratorID: collab.ID,
			Type:           TipoInterjornada,
			Severity:       severity,
			Description:    fmt.Sprintf("O descanso entre jornadas foi de apenas %s. O mínimo exigido são 11 horas.", formatMinutes(delta)),
		}, nil
	}
	return nil, nil
}

// checkOvertime compara o trabalhado com a carga que a Secullum previu para o dia.
// Ambos já vêm apurados de ProcessRules — aqui só se classifica o excedente.
func (s *AuditorService) checkOvertime(collab *domain.Collaborator, workedMinutes, expectedMinutes int) *domain.AuditInconsistency {
	overtime := workedMinutes - expectedMinutes
	if overtime <= 0 {
		return nil
	}

	// Critério (Art. 59 CLT): > 1h e <= 2h => Alerta; > 2h => Crítico.
	if overtime > limiteExtraCritico {
		return &domain.AuditInconsistency{
			CollaboratorID: collab.ID,
			Type:           TipoHoraExtra,
			Severity:       domain.SeverityCritical,
			Description: fmt.Sprintf(
				"Limite legal estourado. O colaborador realizou %s de horas extras (trabalhou %s para uma carga prevista de %s).",
				formatMinutes(overtime), formatMinutes(workedMinutes), formatMinutes(expectedMinutes),
			),
		}
	}
	if overtime > limiteExtraAlerta {
		return &domain.AuditInconsistency{
			CollaboratorID: collab.ID,
			Type:           TipoAlertaHoraExtra,
			Severity:       domain.SeverityAlert,
			Description: fmt.Sprintf(
				"Atenção preventiva. O colaborador já realizou %s de horas extras (trabalhou %s para uma carga prevista de %s).",
				formatMinutes(overtime), formatMinutes(workedMinutes), formatMinutes(expectedMinutes),
			),
		}
	}
	return nil
}
