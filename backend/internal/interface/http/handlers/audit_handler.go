package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
)

type EventPublisher interface {
	Publish(ctx context.Context, queue string, body []byte) error
}

type AuditHandler struct {
	publisher      EventPublisher
	userTenantRepo domain.UserTenantRepository
}

func NewAuditHandler(publisher EventPublisher, userTenantRepo domain.UserTenantRepository) *AuditHandler {
	return &AuditHandler{
		publisher:      publisher,
		userTenantRepo: userTenantRepo,
	}
}

// TriggerRequest aceita três formas:
//
//	{"tenant_id":1}                     -> fechamento de D-1 (o comportamento de sempre,
//	                                       usado pela varredura diária automática)
//	{"tenant_id":1,"date":"2026-07-01"} -> auditoria de um dia específico, sob demanda
//	{"tenant_id":1,"start_date":"2026-07-01","end_date":"2026-07-07"}
//	                                     -> auditoria de um período completo (semana, mês
//	                                       ou intervalo customizado), sob demanda
//
// `date` e `start_date`/`end_date` são mutuamente exclusivos. Nenhum dos dois substitui a
// rotina diária automática: servem para o gestor conferir a situação de dias já
// encerrados sem esperar a próxima varredura.
type TriggerRequest struct {
	TenantID  int    `json:"tenant_id" binding:"required"`
	Date      string `json:"date"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// maxRangeDays limita o tamanho de uma auditoria de período — cobre folgadamente "semana
// ou mês completo" (o caso previsto) sem permitir um pedido acidental (ou abusivo) de
// meses seguidos, que multiplicaria o número de dias avaliados e a carga sobre a Secullum
// mesmo com a otimização de uma única chamada por período (ver GetDailyPunchesRange).
const maxRangeDays = 62

// isRange indica se a requisição pede um período (start_date/end_date) em vez de um único
// dia.
func (r TriggerRequest) isRange() bool {
	return r.StartDate != "" || r.EndDate != ""
}

func (h *AuditHandler) TriggerAudit(c *gin.Context) {
	const op = "AuditHandler.TriggerAudit"

	var req TriggerRequest

	// 1. Valida o JSON de entrada
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Payload inválido",
			"details": "O campo tenant_id é obrigatório e deve ser um número inteiro.",
		})
		return
	}

	// 1b. O tenant_id vem no corpo (não na rota), então a checagem de acesso é feita
	// aqui em vez de via middleware — mesma regra de isolamento das demais rotas.
	if err := ensureTenantAccess(c, h.userTenantRepo, op, req.TenantID); err != nil {
		httperr.Respond(c, err)
		return
	}

	// 1b2. A forma do corpo decide o papel mínimo (docs/08 §5.4): "auditar agora" — sem
	// date nem range, o fechamento de D-1 de sempre — é a única forma que a Diretoria
	// pode disparar (ensureTenantAccess acima já garante esse piso). Pedir um dia
	// específico ou um período grava até maxRangeDays relatórios e reconcilia dias de
	// uma vez — está longe de "mostrar o sistema sem risco", então exige Gestor+.
	if req.Date != "" || req.isRange() {
		if err := requireRole(c, h.userTenantRepo, op, req.TenantID, domain.RoleGestor); err != nil {
			httperr.Respond(c, err)
			return
		}
	}

	// 1c. Requisição de período (start_date/end_date) vs. dia único (date, ou nenhum dos
	// dois = fechamento de D-1) seguem caminhos de validação diferentes.
	if req.isRange() {
		h.triggerRange(c, op, req)
		return
	}
	h.triggerSingleDay(c, op, req)
}

// triggerSingleDay é o caminho de sempre: fechamento de D-1 (sem `date`) ou um dia
// específico sob demanda.
func (h *AuditHandler) triggerSingleDay(c *gin.Context, op string, req TriggerRequest) {
	dia, err := req.resolveDate(op, time.Now())
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	// notify: false — auditorias manuais (disparadas por um humano, a qualquer hora) nunca
	// notificam o WhatsApp dos gestores. O único alerta é o da varredura diária automática
	// no horário configurado (aba Avisos), publicado pelo SchedulerService com notify:true.
	eventPayload, err := json.Marshal(map[string]interface{}{
		"tenant_id":    req.TenantID,
		"date":         dia.Format("2006-01-02"),
		"triggered_by": "manual_http_request",
		"notify":       false,
	})
	if err != nil {
		log.Printf("[AuditHandler] Falha ao serializar evento do tenant %d: %v", req.TenantID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha interna ao serializar evento"})
		return
	}

	if err := h.publisher.Publish(c.Request.Context(), "audit.trigger", eventPayload); err != nil {
		log.Printf("[AuditHandler] Falha ao enfileirar auditoria do tenant %d: %v", req.TenantID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enfileirar requisição de auditoria no Broker"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":   "Auditoria enfileirada com sucesso",
		"tenant_id": req.TenantID,
		"date":      dia.Format("2006-01-02"),
		"status":    "processing",
	})
}

// triggerRange é o caminho de auditoria de período completo (semana, mês ou intervalo
// customizado): um único evento carrega start_date/end_date, e é o worker
// (AuditConsumer) quem busca o período inteiro numa só chamada à Secullum e salva um
// relatório por dia — o pedido HTTP não bloqueia esperando os N dias serem processados.
func (h *AuditHandler) triggerRange(c *gin.Context, op string, req TriggerRequest) {
	if req.Date != "" {
		httperr.Respond(c, domain.NewValidation(op, "parâmetros conflitantes", nil).
			WithDetails("date não pode ser combinado com start_date/end_date"))
		return
	}

	start, end, err := req.resolvePeriod(op, time.Now())
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	// notify: false — mesma regra da auditoria manual de dia único: nunca notifica o
	// WhatsApp (ver comentário em triggerSingleDay).
	eventPayload, err := json.Marshal(map[string]interface{}{
		"tenant_id":    req.TenantID,
		"start_date":   start.Format("2006-01-02"),
		"end_date":     end.Format("2006-01-02"),
		"triggered_by": "manual_http_request",
		"notify":       false,
	})
	if err != nil {
		log.Printf("[AuditHandler] Falha ao serializar evento de período do tenant %d: %v", req.TenantID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha interna ao serializar evento"})
		return
	}

	if err := h.publisher.Publish(c.Request.Context(), "audit.trigger", eventPayload); err != nil {
		log.Printf("[AuditHandler] Falha ao enfileirar auditoria de período do tenant %d: %v", req.TenantID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enfileirar requisição de auditoria no Broker"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":    "Auditoria de período enfileirada com sucesso",
		"tenant_id":  req.TenantID,
		"start_date": start.Format("2006-01-02"),
		"end_date":   end.Format("2006-01-02"),
		"status":     "processing",
	})
}

// resolveDate devolve o dia a auditar: o informado no pedido ou, na ausência dele, D-1.
//
// Um dia ainda em curso não pode ser auditado como fechamento (regras como a contagem
// ímpar de batidas dariam falso positivo a cada expediente em andamento), então hoje e
// datas futuras são recusados.
func (r TriggerRequest) resolveDate(op string, now time.Time) (time.Time, error) {
	if r.Date == "" {
		return now.AddDate(0, 0, -1), nil
	}

	day, err := time.Parse("2006-01-02", r.Date)
	if err != nil {
		return time.Time{}, domain.NewValidation(op, "data inválida", err).
			WithDetails("date deve estar no formato YYYY-MM-DD")
	}
	if !day.Before(domain.DayOf(now)) {
		return time.Time{}, domain.NewValidation(op, "data ainda não encerrada", nil).
			WithDetails("só é possível auditar dias já encerrados (anteriores a hoje)")
	}
	return day, nil
}

// resolvePeriod valida e devolve o intervalo [start, end] de uma auditoria de período
// completo. Ambas as datas são obrigatórias, `end` não pode ser anterior a `start`, o
// período inteiro precisa estar encerrado (mesma regra de resolveDate) e não pode
// ultrapassar maxRangeDays.
func (r TriggerRequest) resolvePeriod(op string, now time.Time) (start, end time.Time, err error) {
	if r.StartDate == "" || r.EndDate == "" {
		return time.Time{}, time.Time{}, domain.NewValidation(op, "período incompleto", nil).
			WithDetails("start_date e end_date são obrigatórios juntos")
	}

	start, err = time.Parse("2006-01-02", r.StartDate)
	if err != nil {
		return time.Time{}, time.Time{}, domain.NewValidation(op, "data inválida", err).
			WithDetails("start_date deve estar no formato YYYY-MM-DD")
	}
	end, err = time.Parse("2006-01-02", r.EndDate)
	if err != nil {
		return time.Time{}, time.Time{}, domain.NewValidation(op, "data inválida", err).
			WithDetails("end_date deve estar no formato YYYY-MM-DD")
	}

	if end.Before(start) {
		return time.Time{}, time.Time{}, domain.NewValidation(op, "intervalo de datas inválido", nil).
			WithDetails("end_date não pode ser anterior a start_date")
	}
	if !end.Before(domain.DayOf(now)) {
		return time.Time{}, time.Time{}, domain.NewValidation(op, "período ainda não encerrado", nil).
			WithDetails("só é possível auditar dias já encerrados (end_date anterior a hoje)")
	}
	if days := int(end.Sub(start).Hours()/24) + 1; days > maxRangeDays {
		return time.Time{}, time.Time{}, domain.NewValidation(op, "período muito longo", nil).
			WithDetails(fmt.Sprintf("o intervalo entre start_date e end_date não pode passar de %d dias", maxRangeDays))
	}

	return start, end, nil
}
