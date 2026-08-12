package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
	"backend/internal/usecase"
)

type OccurrenceHandler struct {
	occurrenceRepo domain.OccurrenceRepository
	collabRepo     domain.CollaboratorRepository
	tenantRepo     domain.TenantRepository
	userTenantRepo domain.UserTenantRepository
	branchResolve  *usecase.BranchResolverService
	secullumSvc    domain.SecullumService
}

func NewOccurrenceHandler(
	occurrenceRepo domain.OccurrenceRepository,
	collabRepo domain.CollaboratorRepository,
	tenantRepo domain.TenantRepository,
	userTenantRepo domain.UserTenantRepository,
	branchResolve *usecase.BranchResolverService,
	secullumSvc domain.SecullumService,
) *OccurrenceHandler {
	return &OccurrenceHandler{
		occurrenceRepo: occurrenceRepo,
		collabRepo:     collabRepo,
		tenantRepo:     tenantRepo,
		userTenantRepo: userTenantRepo,
		branchResolve:  branchResolve,
		secullumSvc:    secullumSvc,
	}
}

type occurrenceResponse struct {
	ID               int    `json:"id"`
	TenantID         int    `json:"tenant_id"`
	CollaboratorID   int    `json:"collaborator_id"` // id na Secullum
	CollaboratorName string `json:"collaborator_name"`
	Date             string `json:"date"`
	Type             string `json:"type"`
	Severity         string `json:"severity"`
	Category         string `json:"category"`
	Description      string `json:"description"`

	State       string  `json:"state"`
	FirstSeenAt string  `json:"first_seen_at"`
	LastSeenAt  string  `json:"last_seen_at"`
	TimesSeen   int     `json:"times_seen"`
	ResolvedAt  *string `json:"resolved_at"`

	IgnoredReason   string `json:"ignored_reason,omitempty"`
	IgnoredByUserID *int   `json:"ignored_by_user_id,omitempty"`

	// Preenchidos para o autopreenchimento da tela de colaborador/advertência.
	HorarioFixo []fixedScheduleResponse `json:"horario_fixo"`
	Filial      *branchSummaryResponse  `json:"filial"`
}

func formatTimestamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// List — GET /api/v1/tenants/:id/occurrences
//
// Filtros: ?date= (dia único), ?start_date=&end_date= (intervalo), ?state= (lista
// separada por vírgula), ?collaborator_id=, ?branch_id=.
//
// Sem filtro de estado o padrão é o que INTERESSA ao gestor: aberta + atualizada. Devolver
// tudo por omissão traria de volta o ruído que a máquina de estados veio eliminar — quem
// quiser o histórico completo pede state=... explicitamente.
func (h *OccurrenceHandler) List(c *gin.Context) {
	const op = "OccurrenceHandler.List"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	filter, err := buildOccurrenceFilter(c, op, tenantID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	occurrences, err := h.occurrenceRepo.List(filter)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	branchID, err := optionalIntQuery(c, op, "branch_id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	items := h.enrich(c, tenantID, occurrences, filter)

	if branchID != nil {
		filtered := make([]occurrenceResponse, 0, len(items))
		for _, item := range items {
			if item.Filial != nil && item.Filial.ID == *branchID {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	c.JSON(http.StatusOK, gin.H{"occurrences": items, "total": len(items)})
}

// enrich acrescenta horário fixo e filial a cada ocorrência.
//
// Tudo é resolvido em lote: os colaboradores do tenant e as filiais são carregados UMA vez
// e indexados em memória. As batidas (que resolvem a filial pelo aparelho) só são buscadas
// quando a consulta é de um dia único — num intervalo longo seria uma chamada à Secullum
// por dia, e o ganho não paga o custo: nesse caso a filial vem do nº de folha.
func (h *OccurrenceHandler) enrich(
	c *gin.Context,
	tenantID int,
	occurrences []domain.Occurrence,
	filter domain.OccurrenceFilter,
) []occurrenceResponse {

	collaborators, err := h.collabRepo.GetByTenantID(tenantID)
	if err != nil {
		collaborators = nil // enriquecimento é acessório: a lista de ocorrências sai mesmo assim
	}

	schedulesBySecullumID := make(map[int][]domain.CollaboratorSchedule, len(collaborators))
	for _, col := range collaborators {
		schedulesBySecullumID[col.SecullumID] = col.Schedules
	}

	var punches map[int]domain.DailyPunch
	if singleDay := singleDayOf(filter); singleDay != nil && h.tenantRepo != nil &&
		h.branchResolve != nil && h.branchResolve.HasDevices(tenantID) {
		if tenant, err := h.tenantRepo.GetByID(tenantID); err == nil {
			punches = punchesForBranchResolution(h.secullumSvc, tenant, *singleDay)
		}
	}

	branches := map[int]usecase.BranchResolution{}
	if h.branchResolve != nil {
		if resolved, err := h.branchResolve.ResolveMany(tenantID, collaborators, punches); err == nil {
			branches = resolved
		}
	}

	out := make([]occurrenceResponse, 0, len(occurrences))
	for _, occ := range occurrences {
		item := occurrenceResponse{
			ID:               occ.ID,
			TenantID:         occ.TenantID,
			CollaboratorID:   occ.CollaboratorID,
			CollaboratorName: occ.CollaboratorName,
			Date:             occ.Date.Format("2006-01-02"),
			Type:             occ.Type,
			Severity:         string(occ.Severity),
			Category:         string(occ.Category()),
			Description:      occ.Description,
			State:            string(occ.State),
			FirstSeenAt:      formatTimestamp(occ.FirstSeenAt),
			LastSeenAt:       formatTimestamp(occ.LastSeenAt),
			TimesSeen:        occ.TimesSeen,
			IgnoredReason:    occ.IgnoredReason,
			IgnoredByUserID:  occ.IgnoredByUserID,
			HorarioFixo:      toFixedSchedule(schedulesBySecullumID[occ.CollaboratorID]),
		}
		if occ.ResolvedAt != nil {
			resolved := formatTimestamp(*occ.ResolvedAt)
			item.ResolvedAt = &resolved
		}
		if res, ok := branches[occ.CollaboratorID]; ok {
			item.Filial = toBranchSummary(res.Branch, res.Source)
		}
		out = append(out, item)
	}
	return out
}

// Ignore — PATCH /api/v1/occurrences/:occurrenceId/ignore
//
// Marca a ocorrência como resolvida_manual. A decisão é DEFINITIVA para aquele dia: as
// varreduras seguintes continuam apurando a inconsistência, mas não a trazem de volta ao
// radar do gestor (ver usecase/reconciler.go). O motivo e o autor ficam registrados no log
// de eventos — ignorar é uma decisão auditável, não um "delete".
func (h *OccurrenceHandler) Ignore(c *gin.Context) {
	const op = "OccurrenceHandler.Ignore"

	id, err := idParam(c, op, "occurrenceId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	occurrence, err := h.occurrenceRepo.GetByID(id)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	// A rota usa o id da ocorrência, então o tenant só é conhecido depois de carregá-la.
	if err := ensureTenantAccess(c, h.userTenantRepo, op, occurrence.TenantID); err != nil {
		httperr.Respond(c, err)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	// Corpo é opcional: ignorar sem justificativa é permitido.
	_ = c.ShouldBindJSON(&req)

	if err := h.occurrenceRepo.Ignore(id, strings.TrimSpace(req.Reason), actorUserID(c)); err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "ocorrência ignorada com sucesso",
		"state":   string(domain.OccurrenceResolvedManual),
	})
}

// Events — GET /api/v1/occurrences/:occurrenceId/events
// Devolve o log de transições, do mais antigo ao mais recente.
func (h *OccurrenceHandler) Events(c *gin.Context) {
	const op = "OccurrenceHandler.Events"

	id, err := idParam(c, op, "occurrenceId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	occurrence, err := h.occurrenceRepo.GetByID(id)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	if err := ensureTenantAccess(c, h.userTenantRepo, op, occurrence.TenantID); err != nil {
		httperr.Respond(c, err)
		return
	}

	events, err := h.occurrenceRepo.ListEvents(id)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]gin.H, 0, len(events))
	for _, e := range events {
		out = append(out, gin.H{
			"id":               e.ID,
			"type":             string(e.Type),
			"from_state":       string(e.FromState),
			"to_state":         string(e.ToState),
			"from_description": e.FromDescription,
			"to_description":   e.ToDescription,
			"reason":           e.Reason,
			"actor_user_id":    e.ActorUserID,
			"created_at":       formatTimestamp(e.CreatedAt),
		})
	}
	c.JSON(http.StatusOK, gin.H{"events": out})
}

// --- Helpers de query string ---

// buildOccurrenceFilter traduz a query string em domain.OccurrenceFilter.
func buildOccurrenceFilter(c *gin.Context, op string, tenantID int) (domain.OccurrenceFilter, error) {
	filter := domain.OccurrenceFilter{TenantID: tenantID}

	date, err := optionalDateQuery(c, op, "date")
	if err != nil {
		return filter, err
	}
	if date != nil {
		filter.StartDate, filter.EndDate = date, date
	} else {
		if filter.StartDate, err = optionalDateQuery(c, op, "start_date"); err != nil {
			return filter, err
		}
		if filter.EndDate, err = optionalDateQuery(c, op, "end_date"); err != nil {
			return filter, err
		}
	}
	if filter.StartDate != nil && filter.EndDate != nil && filter.EndDate.Before(*filter.StartDate) {
		return filter, domain.NewValidation(op, "intervalo de datas inválido", nil).
			WithDetails("end_date não pode ser anterior a start_date")
	}

	if raw := c.Query("state"); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			state := domain.OccurrenceState(strings.TrimSpace(part))
			if !state.Valid() {
				return filter, domain.NewValidation(op, "estado inválido", nil).
					WithDetails("state aceita: aberta, atualizada, resolvida_automatica, resolvida_manual")
			}
			filter.States = append(filter.States, state)
		}
	} else {
		// Padrão: só o que ainda pede ação do gestor.
		filter.States = []domain.OccurrenceState{domain.OccurrenceOpen, domain.OccurrenceUpdated}
	}

	if filter.CollaboratorID, err = optionalIntQuery(c, op, "collaborator_id"); err != nil {
		return filter, err
	}

	return filter, nil
}

func optionalIntQuery(c *gin.Context, op, name string) (*int, error) {
	raw := c.Query(name)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil, domain.NewValidation(op, "parâmetro de consulta inválido", err).
			WithDetails(name + " deve ser um número inteiro")
	}
	return &value, nil
}

// singleDayOf devolve a data quando o filtro cobre exatamente um dia — a única situação
// em que vale buscar as batidas na Secullum para resolver a filial pelo aparelho.
func singleDayOf(filter domain.OccurrenceFilter) *time.Time {
	if filter.StartDate == nil || filter.EndDate == nil {
		return nil
	}
	if !domain.DayOf(*filter.StartDate).Equal(domain.DayOf(*filter.EndDate)) {
		return nil
	}
	return filter.StartDate
}
