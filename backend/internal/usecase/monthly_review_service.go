package usecase

import (
	"fmt"
	"time"

	"backend/internal/domain"
)

// MonthlyReviewService calcula o painel de revisão mensal (Feature 3): as quatro
// condições automáticas nunca são persistidas — são recalculadas a cada consulta, porque
// mudam a cada varredura e persistir um snapshot desatualizado seria pior que não ter
// nada. As duas condições manuais vêm do domain.MonthlyReviewRepository.
type MonthlyReviewService struct {
	occurrenceRepo    domain.OccurrenceRepository
	reportRepo        domain.ReportRepository
	monthlyReviewRepo domain.MonthlyReviewRepository
	tenantRepo        domain.TenantRepository
}

func NewMonthlyReviewService(
	occurrenceRepo domain.OccurrenceRepository,
	reportRepo domain.ReportRepository,
	monthlyReviewRepo domain.MonthlyReviewRepository,
	tenantRepo domain.TenantRepository,
) *MonthlyReviewService {
	return &MonthlyReviewService{
		occurrenceRepo:    occurrenceRepo,
		reportRepo:        reportRepo,
		monthlyReviewRepo: monthlyReviewRepo,
		tenantRepo:        tenantRepo,
	}
}

// Conditions monta o painel de revisão mensal de um tenant para uma competência
// ("YYYY-MM"). Cria o registro de MonthlyReview (estado `aberta`) se for a primeira
// consulta dessa competência.
func (s *MonthlyReviewService) Conditions(tenantID int, competencia string) (domain.MonthlyReviewConditions, error) {
	const op = "MonthlyReviewService.Conditions"

	diaCorte := 0
	if settings, err := s.tenantRepo.GetSettings(tenantID); err == nil {
		diaCorte = settings.RevisaoMensalDiaCorte
	}
	// Falha ao buscar settings não impede a consulta: cai no padrão de mês calendário
	// (diaCorte=0) em vez de bloquear o painel de revisão mensal por causa disso.

	start, end, err := competenciaRange(competencia, diaCorte)
	if err != nil {
		return domain.MonthlyReviewConditions{}, domain.NewValidation(op, "competência inválida", err).
			WithDetails("competencia deve estar no formato YYYY-MM")
	}

	review, err := s.monthlyReviewRepo.GetOrCreate(tenantID, competencia)
	if err != nil {
		return domain.MonthlyReviewConditions{}, err
	}

	openStates := []domain.OccurrenceState{domain.OccurrenceOpen, domain.OccurrenceUpdated}
	occurrences, _, err := s.occurrenceRepo.List(domain.OccurrenceFilter{
		TenantID:  tenantID,
		StartDate: &start,
		EndDate:   &end,
		States:    openStates,
	})
	if err != nil {
		return domain.MonthlyReviewConditions{}, err
	}

	conditions := domain.MonthlyReviewConditions{
		Competencia:    competencia,
		Status:         review.Status,
		PayrollDone:    review.PayrollDone,
		OffsetsDone:    review.OffsetsDone,
		ClosedAt:       review.ClosedAt,
		ClosedByUserID: review.ClosedByUserID,
	}
	for _, occ := range occurrences {
		conditions.OpenOccurrences++
		if occ.State == domain.OccurrenceUpdated {
			conditions.PendingRecheck++
		}
		// Reusa Category() em vez de checar Severity == OPERACIONAL cru: Category() já
		// resolve a precedência canônica (atualizada vence operacional — o valor mudou e
		// o que o gestor viu da última vez não vale mais). Duplicar a regra aqui divergiria
		// silenciosamente se a precedência de Category() mudasse.
		if occ.Category() == domain.CategoryScheduleChange {
			conditions.OpenOperational++
		}
	}

	// Dias sem varredura: dias da competência ATÉ HOJE (dias futuros do mês corrente não
	// contam — não dá para varrer um dia que ainda não aconteceu) sem nenhum Report.
	today := domain.DayOf(time.Now())
	scanEnd := end
	if today.Before(scanEnd) {
		scanEnd = today
	}
	if !scanEnd.Before(start) {
		reports, err := s.reportRepo.ListLatestByTenant(tenantID, &start, &scanEnd)
		if err != nil {
			return domain.MonthlyReviewConditions{}, err
		}
		scanned := make(map[string]bool, len(reports))
		for _, rep := range reports {
			scanned[rep.Date.Format("2006-01-02")] = true
		}
		for d := start; !d.After(scanEnd); d = d.AddDate(0, 0, 1) {
			if !scanned[d.Format("2006-01-02")] {
				conditions.DaysWithoutScan++
			}
		}
	}

	return conditions, nil
}

// Close encerra a competência — falha (conflito) se as quatro condições automáticas não
// estiverem zeradas. As duas manuais são checadas aqui também: o documento funcional é
// literal que o encerramento não pode assumir "sim" por omissão.
func (s *MonthlyReviewService) Close(tenantID int, competencia string, actorUserID int) (*domain.MonthlyReview, error) {
	const op = "MonthlyReviewService.Close"

	conditions, err := s.Conditions(tenantID, competencia)
	if err != nil {
		return nil, err
	}
	if !conditions.Ready() {
		return nil, domain.NewConflict(op, "há pendências que impedem o encerramento", nil).
			WithDetails(fmt.Sprintf(
				"em aberto: %d, a reconferir: %d, operacionais em aberto: %d, dias sem varredura: %d",
				conditions.OpenOccurrences, conditions.PendingRecheck, conditions.OpenOperational, conditions.DaysWithoutScan,
			))
	}
	if !conditions.PayrollDone || !conditions.OffsetsDone {
		return nil, domain.NewConflict(op, "confirmações manuais pendentes", nil).
			WithDetails("folha de pagamento e compensações precisam ser confirmadas antes do encerramento")
	}

	return s.monthlyReviewRepo.Close(tenantID, competencia, actorUserID)
}

// Export monta o relatório consolidado de uma competência ENCERRADA — a evidência do
// ciclo. Bloqueia (conflito) se a competência ainda estiver aberta: exportar um "meio
// termo" que pode mudar amanhã não serve como evidência de nada.
func (s *MonthlyReviewService) Export(tenantID int, competencia string) (domain.MonthlyReviewExport, error) {
	const op = "MonthlyReviewService.Export"

	review, err := s.monthlyReviewRepo.GetOrCreate(tenantID, competencia)
	if err != nil {
		return domain.MonthlyReviewExport{}, err
	}
	if review.Status != domain.MonthlyReviewClosed {
		return domain.MonthlyReviewExport{}, domain.NewConflict(op, "competência ainda não foi encerrada", nil).
			WithDetails("o relatório consolidado só existe depois do encerramento — é a evidência do que foi decidido, não um rascunho")
	}

	diaCorte := 0
	if settings, err := s.tenantRepo.GetSettings(tenantID); err == nil {
		diaCorte = settings.RevisaoMensalDiaCorte
	}
	start, end, err := competenciaRange(competencia, diaCorte)
	if err != nil {
		return domain.MonthlyReviewExport{}, domain.NewValidation(op, "competência inválida", err)
	}

	// Sem filtro de State: a evidência do ciclo inclui TODOS os desfechos (tratada,
	// ignorada, resolvida na origem), não só o que ficou pendente.
	occurrences, _, err := s.occurrenceRepo.List(domain.OccurrenceFilter{TenantID: tenantID, StartDate: &start, EndDate: &end})
	if err != nil {
		return domain.MonthlyReviewExport{}, err
	}

	export := domain.MonthlyReviewExport{
		Competencia:      competencia,
		ClosedAt:         review.ClosedAt,
		ClosedByUserID:   review.ClosedByUserID,
		TotalOccurrences: len(occurrences),
		ByState:          map[string]int{},
		BySeverity:       map[string]int{},
		ByType:           map[string]int{},
		Occurrences:      occurrences,
	}
	for _, occ := range occurrences {
		export.ByState[string(occ.State)]++
		export.BySeverity[string(occ.Severity)]++
		export.ByType[occ.Type]++
	}
	return export, nil
}

// competenciaRange traduz "YYYY-MM" no intervalo [start, end] que a competência cobre.
//
// diaCorte == 0 é o padrão: mês calendário (dia 1 ao último dia do mês nomeado).
// diaCorte == D > 0 desloca a competência para o corte de folha configurado — "2026-09"
// com corte 25 vai de 26/08/2026 a 25/09/2026: o mês NOMEADO é o do fim do intervalo, não
// do início, seguindo a convenção comum de folha de pagamento ("competência de setembro"
// fecha em 25/09). Ver domain.TenantSettings.RevisaoMensalDiaCorte.
func competenciaRange(competencia string, diaCorte int) (start, end time.Time, err error) {
	month, err := time.Parse("2006-01", competencia)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if diaCorte <= 0 {
		return month, month.AddDate(0, 1, -1), nil
	}
	end = time.Date(month.Year(), month.Month(), diaCorte, 0, 0, 0, 0, month.Location())
	start = end.AddDate(0, -1, 0).AddDate(0, 0, 1)
	return start, end, nil
}
