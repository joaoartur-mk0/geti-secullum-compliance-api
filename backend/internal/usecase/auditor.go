package usecase

import (
	"fmt"
	"time"

	"backend/internal/domain"
)

type AuditorService struct{}

func NewAuditorService() *AuditorService {
	return &AuditorService{}
}

// ProcessRules é a função orquestradora. Recebe as configurações do Inquilino,
// os dados do colaborador, as batidas de hoje e de ontem (necessário para interjornada).
func (s *AuditorService) ProcessRules(
	settings *domain.TenantSettings,
	collab *domain.Collaborator,
	todayPunch *domain.DailyPunch,
	yesterdayPunch *domain.DailyPunch,
	currentTime time.Time,
) []domain.AuditInconsistency {

	var infractions []domain.AuditInconsistency

	if settings == nil {
		return infractions // Se não há configuração, encerra a auditoria
	}

	// 1. Regra de Batidas Esquecidas / Incompletas
	if settings.Esquecimento {
		if err := s.checkMissingPunches(collab, todayPunch, currentTime); err != nil {
			infractions = append(infractions, *err)
		}
	}

	// 2. Regra de Intervalo de Almoço Reduzido (Art. 71 CLT)
	if settings.Almoco {
		if err := s.checkLunchBreak(todayPunch); err != nil {
			infractions = append(infractions, *err)
		}
	}

	// 3. Regra de Interjornada Curta (Art. 66 CLT)
	if settings.Interjornada && yesterdayPunch != nil {
		if err := s.checkInterjornada(yesterdayPunch, todayPunch); err != nil {
			infractions = append(infractions, *err)
		}
	}

	// 4. Regra de Hora Extra Excedente
	if settings.Hextras {
		if err := s.checkOvertime(collab, todayPunch); err != nil {
			infractions = append(infractions, *err)
		}
	}

	return infractions
}

// --- Métodos Privados com a Matemática das Regras Trabalhistas ---

func (s *AuditorService) checkMissingPunches(collab *domain.Collaborator, punch *domain.DailyPunch, now time.Time) *domain.AuditInconsistency {
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

	// Se o número de batidas for ímpar e já passou das 18h (ou fim de expediente)
	// Trata-se de uma batida esquecida.
	if punchesCount%2 != 0 {
		return &domain.AuditInconsistency{
			CollaboratorID: collab.ID,
			Type:           "Batida Esquecida",
			Severity:       domain.SeverityCritical,
			Description:    fmt.Sprintf("O colaborador possui um número ímpar de marcações (%d batida(s)). Provável esquecimento.", punchesCount),
		}
	}
	return nil
}

func (s *AuditorService) checkLunchBreak(punch *domain.DailyPunch) *domain.AuditInconsistency {
	if punch.Saida1 == nil || punch.Entrada2 == nil {
		return nil // Se não bateu a saída ou a volta do almoço, não avalia intervalo ainda
	}

	saida, _ := time.Parse("15:04", *punch.Saida1)
	volta, _ := time.Parse("15:04", *punch.Entrada2)

	deltaMinutos := volta.Sub(saida).Minutes()

	// Critério: Período de intervalo para refeição e descanso menor do que 60 minutos[cite: 85].
	if deltaMinutos < 60 {
		return &domain.AuditInconsistency{
			CollaboratorID: punch.CollaboratorID,
			Type:           "Almoço Reduzido",
			Severity:       domain.SeverityCritical,
			Description:    fmt.Sprintf("O intervalo de almoço foi de apenas %.0f minutos, inferior ao limite legal de 60 minutos.", deltaMinutos),
		}
	}
	return nil
}

func (s *AuditorService) checkInterjornada(yesterday *domain.DailyPunch, today *domain.DailyPunch) *domain.AuditInconsistency {
	if yesterday.Saida2 == nil || today.Entrada1 == nil {
		return nil
	}

	saidaOntem, _ := time.Parse("15:04", *yesterday.Saida2)
	entradaHoje, _ := time.Parse("15:04", *today.Entrada1)

	// Ajusta as datas fictícias para calcular o delta corretamente entre dias diferentes
	dataSaida := time.Date(2020, 1, 1, saidaOntem.Hour(), saidaOntem.Minute(), 0, 0, time.UTC)
	dataEntrada := time.Date(2020, 1, 2, entradaHoje.Hour(), entradaHoje.Minute(), 0, 0, time.UTC)

	deltaHoras := dataEntrada.Sub(dataSaida).Hours()

	// Critério: Tempo de descanso inferior a 11 horas[cite: 83].
	if deltaHoras < 11 {
		return &domain.AuditInconsistency{
			CollaboratorID: today.CollaboratorID,
			Type:           "Interjornada Curta",
			Severity:       domain.SeverityCritical,
			Description:    fmt.Sprintf("O descanso entre jornadas foi de apenas %.2f horas. O mínimo exigido são 11 horas.", deltaHoras),
		}
	}
	return nil
}

func (s *AuditorService) checkOvertime(collab *domain.Collaborator, punch *domain.DailyPunch) *domain.AuditInconsistency {
	if punch.Entrada1 == nil || punch.Saida1 == nil || punch.Entrada2 == nil || punch.Saida2 == nil {
		return nil // Jornada incompleta, não calcula hora extra exata ainda
	}

	e1, _ := time.Parse("15:04", *punch.Entrada1)
	s1, _ := time.Parse("15:04", *punch.Saida1)
	e2, _ := time.Parse("15:04", *punch.Entrada2)
	s2, _ := time.Parse("15:04", *punch.Saida2)

	workedHours := s1.Sub(e1).Hours() + s2.Sub(e2).Hours()

	// Simplificação: Assumindo jornada padrão de 8 horas para o exemplo
	standardHours := 8.0
	overtime := workedHours - standardHours

	if overtime > 0 {
		// Critério: Ultrapasse o limite de alerta de 1 hora, ou o limite crítico de 2 horas[cite: 90].
		if overtime > 2.0 {
			return &domain.AuditInconsistency{
				CollaboratorID: collab.ID,
				Type:           "Hora Extra Excedente",
				Severity:       domain.SeverityCritical,
				Description:    fmt.Sprintf("Limite legal estourado. O colaborador realizou %.2f horas extras.", overtime),
			}
		} else if overtime > 1.0 {
			return &domain.AuditInconsistency{
				CollaboratorID: collab.ID,
				Type:           "Alerta de Hora Extra",
				Severity:       domain.SeverityAlert,
				Description:    fmt.Sprintf("Atenção preventiva. O colaborador já realizou %.2f horas extras.", overtime),
			}
		}
	}
	return nil
}
