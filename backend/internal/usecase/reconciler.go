package usecase

import (
	"fmt"
	"time"

	"backend/internal/domain"
)

// ReconcilerService compara o que a varredura ACABOU de apurar com o que já estava
// registrado para aquele dia, e move cada ocorrência na máquina de estados.
//
// É o que substitui o antigo "toda auditoria devolve a lista inteira de novo": auditar o
// mesmo dia cinco vezes não cria cinco cópias da mesma infração — cria uma ocorrência que
// foi vista cinco vezes. O que o gestor vê é o que MUDOU.
type ReconcilerService struct {
	repo domain.OccurrenceRepository
}

func NewReconcilerService(repo domain.OccurrenceRepository) *ReconcilerService {
	return &ReconcilerService{repo: repo}
}

// ReconcileResult resume o efeito de uma varredura, para o log e para o resumo enviado
// aos gestores no WhatsApp.
type ReconcileResult struct {
	Created   []domain.Occurrence // apuradas pela primeira vez
	Updated   []domain.Occurrence // continuam existindo, com valor diferente
	Reopened  []domain.Occurrence // tinham se resolvido sozinhas e voltaram
	Resolved  []domain.Occurrence // sumiram da apuração: resolvidas automaticamente
	Unchanged int                 // idênticas à varredura anterior (o caso do sync repetido)
	Ignored   int                 // em resolvida_manual: apuradas, mas silenciadas pelo usuário
}

// Changed indica se a varredura produziu alguma novidade digna de notificação.
func (r ReconcileResult) Changed() bool {
	return len(r.Created) > 0 || len(r.Updated) > 0 || len(r.Reopened) > 0 || len(r.Resolved) > 0
}

// Reconcile aplica a máquina de estados para um tenant em uma data.
//
// `fresh` são as inconsistências recém-apuradas pelo motor de regras para AQUELE dia
// (apenas aquele dia: misturar datas quebraria a resolução automática, já que uma
// ocorrência ausente da lista é interpretada como resolvida).
func (s *ReconcilerService) Reconcile(
	tenantID int,
	date time.Time,
	fresh []domain.AuditInconsistency,
	now time.Time,
) (ReconcileResult, error) {
	day := domain.DayOf(date)

	existing, err := s.repo.ListByTenantAndDate(tenantID, day)
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("carregar ocorrências de %s: %w", day.Format("2006-01-02"), err)
	}

	changes, result := planReconciliation(tenantID, day, existing, fresh, now)

	if len(changes) > 0 {
		if err := s.repo.ApplyChanges(changes); err != nil {
			return ReconcileResult{}, fmt.Errorf("gravar ocorrências de %s: %w", day.Format("2006-01-02"), err)
		}
	}

	return result, nil
}

// occurrenceKey é a identidade estável da ocorrência dentro de um tenant/dia.
type occurrenceKey struct {
	collaboratorID int
	typ            string
}

// planReconciliation decide, sem tocar no banco, o que fazer com cada ocorrência. Separar
// a decisão da gravação é o que permite testar a máquina de estados inteira (incluindo a
// bateria de syncs repetidos) sem subir Postgres.
func planReconciliation(
	tenantID int,
	day time.Time,
	existing []domain.Occurrence,
	fresh []domain.AuditInconsistency,
	now time.Time,
) ([]domain.OccurrenceChange, ReconcileResult) {

	var changes []domain.OccurrenceChange
	var result ReconcileResult

	byKey := make(map[occurrenceKey]domain.Occurrence, len(existing))
	for _, occ := range existing {
		byKey[occurrenceKey{occ.CollaboratorID, occ.Type}] = occ
	}

	// Marca quais das existentes reapareceram nesta varredura; as que sobrarem são as
	// que se resolveram sozinhas.
	seen := make(map[occurrenceKey]bool, len(fresh))

	for _, inc := range fresh {
		key := occurrenceKey{inc.CollaboratorID, inc.Type}

		// Duas inconsistências do MESMO tipo para o MESMO colaborador no mesmo dia não
		// têm identidade separável — a chave de negócio é (colaborador, data, tipo).
		// Mantemos a primeira; a segunda seria uma duplicata por definição.
		if seen[key] {
			continue
		}
		seen[key] = true

		fingerprint := domain.Fingerprint(inc)

		prev, found := byKey[key]
		if !found {
			occ := domain.Occurrence{
				TenantID:         tenantID,
				CollaboratorID:   inc.CollaboratorID,
				CollaboratorName: inc.CollaboratorName,
				Date:             day,
				Type:             inc.Type,
				Severity:         inc.Severity,
				Description:      inc.Description,
				Fingerprint:      fingerprint,
				State:            domain.OccurrenceOpen,
				FirstSeenAt:      now,
				LastSeenAt:       now,
				TimesSeen:        1,
			}
			changes = append(changes, domain.OccurrenceChange{
				Kind:       domain.ChangeInsert,
				Occurrence: occ,
				Event: &domain.OccurrenceEvent{
					TenantID:      tenantID,
					Type:          domain.EventCreated,
					ToState:       domain.OccurrenceOpen,
					ToDescription: inc.Description,
					CreatedAt:     now,
				},
			})
			result.Created = append(result.Created, occ)
			continue
		}

		// Ignorada manualmente: o usuário já decidiu. Continuar apurando não a ressuscita
		// — só registramos que ela ainda está lá (last_seen_at), sem evento e sem alarde.
		if prev.State == domain.OccurrenceResolvedManual {
			touched := prev
			touched.LastSeenAt = now
			touched.TimesSeen = prev.TimesSeen + 1
			changes = append(changes, domain.OccurrenceChange{Kind: domain.ChangeTouch, Occurrence: touched})
			result.Ignored++
			continue
		}

		sameValue := prev.Fingerprint == fingerprint

		// Reapareceu depois de ter se resolvido sozinha: volta ao radar como "atualizada",
		// mesmo que o valor seja idêntico ao de antes — o gestor precisa saber que voltou.
		if prev.State == domain.OccurrenceResolvedAuto {
			occ := prev
			occ.CollaboratorName = inc.CollaboratorName
			occ.Severity = inc.Severity
			occ.Description = inc.Description
			occ.Fingerprint = fingerprint
			occ.State = domain.OccurrenceUpdated
			occ.LastSeenAt = now
			occ.TimesSeen = prev.TimesSeen + 1
			occ.ResolvedAt = nil

			changes = append(changes, domain.OccurrenceChange{
				Kind:       domain.ChangeUpdate,
				Occurrence: occ,
				Event: &domain.OccurrenceEvent{
					OccurrenceID:    prev.ID,
					TenantID:        tenantID,
					Type:            domain.EventReopened,
					FromState:       prev.State,
					ToState:         occ.State,
					FromDescription: prev.Description,
					ToDescription:   inc.Description,
					CreatedAt:       now,
				},
			})
			result.Reopened = append(result.Reopened, occ)
			continue
		}

		// Mesmo problema, mesmo valor: nada mudou. Este é o caminho do sync repetido no
		// mesmo dia — não duplica, não muda de estado e não gera evento.
		if sameValue {
			touched := prev
			touched.LastSeenAt = now
			touched.TimesSeen = prev.TimesSeen + 1
			changes = append(changes, domain.OccurrenceChange{Kind: domain.ChangeTouch, Occurrence: touched})
			result.Unchanged++
			continue
		}

		// Mesmo problema, valor diferente (ex.: o intervalo caiu de 43min para 20min):
		// atualiza e volta a exigir conferência.
		occ := prev
		occ.CollaboratorName = inc.CollaboratorName
		occ.Severity = inc.Severity
		occ.Description = inc.Description
		occ.Fingerprint = fingerprint
		occ.State = domain.OccurrenceUpdated
		occ.LastSeenAt = now
		occ.TimesSeen = prev.TimesSeen + 1

		changes = append(changes, domain.OccurrenceChange{
			Kind:       domain.ChangeUpdate,
			Occurrence: occ,
			Event: &domain.OccurrenceEvent{
				OccurrenceID:    prev.ID,
				TenantID:        tenantID,
				Type:            domain.EventUpdated,
				FromState:       prev.State,
				ToState:         occ.State,
				FromDescription: prev.Description,
				ToDescription:   inc.Description,
				CreatedAt:       now,
			},
		})
		result.Updated = append(result.Updated, occ)
	}

	// O que estava aberto e não foi apurado agora deixou de existir: a batida foi
	// corrigida na Secullum, ou a escala foi ajustada. Resolve sozinha, com registro.
	for _, prev := range existing {
		if seen[occurrenceKey{prev.CollaboratorID, prev.Type}] {
			continue
		}
		if !prev.State.Open() {
			continue // já resolvida (auto ou manualmente): nada a fazer
		}

		resolvedAt := now
		occ := prev
		occ.State = domain.OccurrenceResolvedAuto
		occ.ResolvedAt = &resolvedAt

		changes = append(changes, domain.OccurrenceChange{
			Kind:       domain.ChangeResolve,
			Occurrence: occ,
			Event: &domain.OccurrenceEvent{
				OccurrenceID: prev.ID,
				TenantID:     tenantID,
				Type:         domain.EventResolvedAuto,
				FromState:    prev.State,
				ToState:      occ.State,
				CreatedAt:    now,
			},
		})
		result.Resolved = append(result.Resolved, occ)
	}

	return changes, result
}
