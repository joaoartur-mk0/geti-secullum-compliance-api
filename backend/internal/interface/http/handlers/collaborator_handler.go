package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
	"backend/internal/usecase"
)

type CollaboratorHandler struct {
	collabRepo      domain.CollaboratorRepository
	tenantRepo      domain.TenantRepository
	branchResolve   *usecase.BranchResolverService
	secullumSvc     domain.SecullumService
	punchRecordRepo domain.PunchRecordRepository
}

func NewCollaboratorHandler(
	repo domain.CollaboratorRepository,
	tenantRepo domain.TenantRepository,
	branchResolve *usecase.BranchResolverService,
	secullumSvc domain.SecullumService,
	punchRecordRepo domain.PunchRecordRepository,
) *CollaboratorHandler {
	return &CollaboratorHandler{
		collabRepo:      repo,
		tenantRepo:      tenantRepo,
		branchResolve:   branchResolve,
		secullumSvc:     secullumSvc,
		punchRecordRepo: punchRecordRepo,
	}
}

// collaboratorResponse é a identidade mínima do colaborador exposta ao painel.
// Não inclui CPF/celular (dado pessoal desnecessário para os indicadores) nem as
// jornadas (usadas só internamente pelo motor de auditoria).
type collaboratorResponse struct {
	ID         int    `json:"id"`
	SecullumID int    `json:"secullum_id"`
	Name       string `json:"name"`
}

// collaboratorHistoryResponse é a versão exposta em /collaborators/history — inclui
// admissão/demissão, o que a lista padrão de ativos não precisa mostrar.
type collaboratorHistoryResponse struct {
	ID         int     `json:"id"`
	SecullumID int     `json:"secullum_id"`
	Name       string  `json:"name"`
	Admissao   *string `json:"admissao"`
	Demissao   *string `json:"demissao"`
	Demitido   bool    `json:"demitido"`
}

// List — GET /api/v1/tenants/:id/collaborators
// Devolve o espelho local de colaboradores ATIVOS (sem Demissao) do tenant sincronizado
// (via fila tenant.provisioning) e o total, para o painel de Indicadores mostrar quantos
// funcionários estão sob auditoria. Funcionários desligados aparecem só em
// GET /collaborators/history.
func (h *CollaboratorHandler) List(c *gin.Context) {
	const op = "CollaboratorHandler.List"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	collaborators, err := h.collabRepo.GetByTenantID(tenantID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]collaboratorResponse, 0, len(collaborators))
	for _, col := range collaborators {
		out = append(out, collaboratorResponse{
			ID:         col.ID,
			SecullumID: col.SecullumID,
			Name:       col.Name,
		})
	}
	c.JSON(http.StatusOK, gin.H{"collaborators": out, "total": len(out)})
}

// History — GET /api/v1/tenants/:id/collaborators/history
// Devolve TODOS os colaboradores já sincronizados do tenant, ativos e demitidos —
// diferente de /collaborators, que só lista os ativos.
func (h *CollaboratorHandler) History(c *gin.Context) {
	const op = "CollaboratorHandler.History"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	collaborators, err := h.collabRepo.GetHistoryByTenantID(tenantID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]collaboratorHistoryResponse, 0, len(collaborators))
	for _, col := range collaborators {
		out = append(out, collaboratorHistoryResponse{
			ID:         col.ID,
			SecullumID: col.SecullumID,
			Name:       col.Name,
			Admissao:   formatDatePtr(col.Admissao),
			Demissao:   formatDatePtr(col.Demissao),
			Demitido:   col.Demitido,
		})
	}
	c.JSON(http.StatusOK, gin.H{"collaborators": out, "total": len(out)})
}

// formatDatePtr formata uma data opcional como "YYYY-MM-DD", ou nil se ausente.
func formatDatePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

// Prefill — GET /api/v1/tenants/:id/collaborators/:secullumId/prefill?date=YYYY-MM-DD
//
// Entrega, num único payload, o que a tela de colaborador (e o formulário de advertência)
// precisam preencher sozinhas: o horário fixo cadastrado na Secullum e a filial em que a
// pessoa está lotada, com o gestor responsável.
//
// O horário sai do espelho local sincronizado — nenhuma chamada externa. A `date` é
// opcional e serve só para tentar a resolução de filial pelo aparelho da batida daquele
// dia; sem ela (ou se a Secullum falhar) a filial vem do nº de folha.
func (h *CollaboratorHandler) Prefill(c *gin.Context) {
	const op = "CollaboratorHandler.Prefill"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	secullumID, err := idParam(c, op, "secullumId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	collab, err := h.collabRepo.GetBySecullumID(tenantID, secullumID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	date, err := optionalDateQuery(c, op, "date")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	// Só vale chamar a Secullum se houver aparelho cadastrado; sem nenhum, a filial viria
	// do nº de folha de qualquer forma.
	var punch *domain.DailyPunch
	if date != nil && h.tenantRepo != nil && h.branchResolve != nil && h.branchResolve.HasDevices(tenantID) {
		if tenant, err := h.tenantRepo.GetByID(tenantID); err == nil {
			if p, ok := punchesForBranchResolution(h.secullumSvc, tenant, *date)[secullumID]; ok {
				punch = &p
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"collaborator": gin.H{
			"id":           collab.ID,
			"secullum_id":  collab.SecullumID,
			"name":         collab.Name,
			"numero_folha": collab.NumeroFolha,
		},
		"horario_fixo": toFixedSchedule(collab.Schedules),
		"filial":       resolveBranchFor(h.branchResolve, tenantID, collab, punch),
	})
}

// punchRecordResponse expõe o enriquecimento de origem da marcação (equipamento/motivo)
// de um dia — ver domain.PunchRecord. Sem isto, os dados que o AuditConsumer cruza da
// FonteDados da Secullum são gravados e nunca ficam visíveis a ninguém.
type punchRecordResponse struct {
	Date          string  `json:"date"`
	EquipamentoID *int    `json:"equipamento_id"`
	Motivo        *string `json:"motivo"`
}

// PunchRecords — GET /api/v1/tenants/:id/collaborators/:secullumId/punch-records?start_date=&end_date=
//
// Devolve, para o período informado, o equipamento e o motivo apurados de cada dia com
// correspondência encontrada na FonteDados da Secullum (ver AuditConsumer.buildPunchRecord).
// Um dia sem entrada na resposta significa que a auditoria daquele dia não encontrou
// correspondência — não que o colaborador não trabalhou.
func (h *CollaboratorHandler) PunchRecords(c *gin.Context) {
	const op = "CollaboratorHandler.PunchRecords"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	secullumID, err := idParam(c, op, "secullumId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	start, end, err := requiredDateRangeQuery(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	records, err := h.punchRecordRepo.GetByCollaborator(tenantID, secullumID, start, end)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]punchRecordResponse, 0, len(records))
	for _, rec := range records {
		out = append(out, punchRecordResponse{
			Date:          rec.Date.Format("2006-01-02"),
			EquipamentoID: rec.EquipamentoID,
			Motivo:        rec.Motivo,
		})
	}
	c.JSON(http.StatusOK, gin.H{"punch_records": out, "total": len(out)})
}

// requiredDateRangeQuery lê start_date/end_date (ambos obrigatórios juntos, "YYYY-MM-DD")
// e valida que end não é anterior a start — mesma exigência de audit_handler.resolvePeriod,
// mas sem a regra de "período encerrado" (aqui é só consulta, não dispara auditoria).
func requiredDateRangeQuery(c *gin.Context, op string) (start, end time.Time, err error) {
	rawStart, rawEnd := c.Query("start_date"), c.Query("end_date")
	if rawStart == "" || rawEnd == "" {
		return time.Time{}, time.Time{}, domain.NewValidation(op, "período incompleto", nil).
			WithDetails("start_date e end_date são obrigatórios juntos")
	}

	start, err = time.Parse("2006-01-02", rawStart)
	if err != nil {
		return time.Time{}, time.Time{}, domain.NewValidation(op, "start_date inválida", err).
			WithDetails("start_date deve estar no formato YYYY-MM-DD")
	}
	end, err = time.Parse("2006-01-02", rawEnd)
	if err != nil {
		return time.Time{}, time.Time{}, domain.NewValidation(op, "end_date inválida", err).
			WithDetails("end_date deve estar no formato YYYY-MM-DD")
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, domain.NewValidation(op, "período inválido", nil).
			WithDetails("end_date não pode ser anterior a start_date")
	}
	return start, end, nil
}

// optionalDateQuery lê um parâmetro de data "YYYY-MM-DD" da query string. Ausente devolve
// nil (sem filtro); presente e malformado é erro de validação, nunca uma data silenciosa.
func optionalDateQuery(c *gin.Context, op, name string) (*time.Time, error) {
	raw := c.Query(name)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, domain.NewValidation(op, "data inválida", err).
			WithDetails(name + " deve estar no formato YYYY-MM-DD")
	}
	return &parsed, nil
}
