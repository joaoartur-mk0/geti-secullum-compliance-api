package usecase

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"backend/internal/domain"
)

// EventPublisher é redeclarada aqui (mesmo formato de handlers.EventPublisher) para o
// SchedulerService publicar na fila sem o pacote usecase importar handlers — usecase é
// a camada de baixo, handlers já depende dela, não o contrário. *messaging.ChannelPool
// (usado em cmd/api/main.go) já satisfaz esta interface estruturalmente.
type EventPublisher interface {
	Publish(ctx context.Context, queue string, body []byte) error
}

// SchedulerService dispara, sozinho, DOIS tipos de auditoria automática de cada tenant
// ATIVO:
//
//  1. A auditoria diária de fechamento, no horário que o tenant configurou
//     (TenantSettings.Horario, aba Avisos do painel) — no máximo uma vez por dia por
//     tenant. É a ÚNICA que notifica o WhatsApp dos gestores (`notify: true`).
//  2. Uma atualização silenciosa de hora em hora (`notify: false`), reauditando o MÊS
//     ATUAL inteiro até o último dia encerrado (D-1) — não só D-1 — para capturar
//     correções feitas na Secullum a qualquer momento do mês corrente (ex.: RH ajustou
//     uma batida do dia 3, e hoje é dia 15), sem repetir o alerta no WhatsApp a cada
//     hora, que treinaria o gestor a ignorá-lo. O período inteiro é buscado numa única
//     chamada à Secullum (ver `SecullumService.GetDailyPunchesRange`) e depois separado
//     por dia — a mesma otimização que a auditoria de período sob demanda já usa.
//
// Nenhuma delas é um motor de auditoria novo: publicam o MESMO payload que o handler HTTP
// (POST /api/v1/audit/trigger) publica na fila `audit.trigger`, com o campo `notify`
// controlando se o AuditConsumer deve enfileirar o resumo no WhatsApp ao final.
type SchedulerService struct {
	tenantRepo domain.TenantRepository
	publisher  EventPublisher

	mu sync.Mutex
	// lastRun guarda, por tenant, a data (YYYY-MM-DD) do último disparo automático — só
	// em memória. Um reinício do processo exatamente no minuto configurado pode gerar um
	// disparo extra naquele dia (inofensivo: mesmo efeito de reauditar manualmente); um
	// reinício logo após o horário já ter passado deixa o tenant sem a automática até o
	// dia seguinte. Persistir isso resolveria, mas não foi pedido nesta rodada.
	lastRun map[int]string
}

// tickInterval define a granularidade da checagem do horário diário configurado. Precisa
// ser menor que 1 minuto para não arriscar pular o horário (comparado em resolução de
// minuto, "HH:MM").
const tickInterval = 30 * time.Second

// hourlyInterval é a cadência da atualização silenciosa (item 2 acima).
const hourlyInterval = 1 * time.Hour

func NewSchedulerService(tenantRepo domain.TenantRepository, publisher EventPublisher) *SchedulerService {
	return &SchedulerService{
		tenantRepo: tenantRepo,
		publisher:  publisher,
		lastRun:    make(map[int]string),
	}
}

// Start bloqueia escutando os tickers até o contexto ser cancelado. Deve rodar em
// goroutine própria, como os workers de fila em cmd/api/main.go.
func (s *SchedulerService) Start(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	hourlyTicker := time.NewTicker(hourlyInterval)
	defer hourlyTicker.Stop()

	log.Println("[*] Agendador de auditoria automática em execução...")
	for {
		select {
		case <-ctx.Done():
			log.Println("[*] Desligando Agendador de auditoria graciosamente...")
			return
		case now := <-ticker.C:
			s.tick(now)
		case <-hourlyTicker.C:
			s.hourlyTick()
		}
	}
}

// tick verifica, para cada tenant ATIVO, se o horário configurado bate com o instante
// atual e, se sim (e ainda não disparou hoje), enfileira a auditoria.
func (s *SchedulerService) tick(now time.Time) {
	tenants, err := s.tenantRepo.GetActiveTenants()
	if err != nil {
		log.Printf("[Agendador] falha ao listar tenants ativos: %v\n", err)
		return
	}

	today := now.Format("2006-01-02")
	nowHHMM := now.Format("15:04")

	for _, tenant := range tenants {
		if tenant.Settings == nil || tenant.Settings.Horario == "" {
			continue // sem agendamento configurado
		}
		if tenant.Settings.Horario != nowHHMM {
			continue
		}
		if !s.claimForToday(tenant.ID, today) {
			continue // já disparou hoje
		}
		s.trigger(tenant.ID)
	}
}

// claimForToday devolve true (e reserva) na primeira vez que um tenant bate o horário
// num dia; devolve false em qualquer tick seguinte no mesmo dia (o tick roda a cada
// 30s, então o mesmo minuto é observado mais de uma vez).
func (s *SchedulerService) claimForToday(tenantID int, today string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lastRun[tenantID] == today {
		return false
	}
	s.lastRun[tenantID] = today
	return true
}

// trigger publica o pedido de auditoria diária de fechamento — o mesmo payload que
// handlers.AuditHandler.TriggerAudit publica para uma auditoria manual sem `date`
// (fechamento de D-1), mas com `notify: true`: esta é a ÚNICA auditoria automática que
// deve resultar em alerta no WhatsApp dos gestores.
func (s *SchedulerService) trigger(tenantID int) {
	s.publish(tenantID, "cron_job", true, "auditoria automática enfileirada (horário configurado)")
}

// hourlyTick reaudita silenciosamente o MÊS ATUAL (do dia 1 até o último dia encerrado)
// de CADA tenant ativo, de hora em hora — mantém os dados (relatórios/ocorrências)
// atualizados com correções feitas na Secullum a qualquer momento do mês corrente, sem
// depender do próximo disparo diário. Não usa claimForToday: roda em toda hora cheia, o
// dia inteiro, independente do horário configurado (que continua sendo o único disparo
// que notifica).
//
// Publica um único evento de PERÍODO por tenant (start_date/end_date) — é o
// AuditConsumer quem busca o mês inteiro numa única chamada à Secullum e separa por dia
// ao salvar (a mesma otimização da auditoria de período sob demanda), evitando 1
// requisição por dia e o risco de rate limiting.
func (s *SchedulerService) hourlyTick() {
	tenants, err := s.tenantRepo.GetActiveTenants()
	if err != nil {
		log.Printf("[Agendador] falha ao listar tenants ativos para atualização horária: %v\n", err)
		return
	}
	start, end := monthToDateRange(time.Now())
	for _, tenant := range tenants {
		s.publishRange(tenant.ID, start, end, "hourly_refresh", "atualização horária silenciosa do mês corrente enfileirada")
	}
}

// monthToDateRange devolve [dia 1, último dia encerrado] do mês corrente — o período que
// a atualização horária audita. Ex.: hoje é dia 15 => [dia 1, dia 14] do mesmo mês.
//
// Usa o mês do último dia ENCERRADO (ontem), não o de hoje: no dia 1 de um mês, "ontem" é
// o último dia do mês anterior, e é esse mês que ainda não teve nenhum dia encerrado
// dentro do mês corrente — sem este ajuste, o intervalo ficaria vazio/inválido
// (start=dia 1 de hoje > end=ontem) bem no dia em que a virada de mês mais precisa ser
// conferida.
func monthToDateRange(now time.Time) (time.Time, time.Time) {
	end := domain.DayOf(now).AddDate(0, 0, -1) // último dia encerrado (D-1)
	start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location())
	return start, end
}

// publish monta e envia o payload de auditoria automática de UM DIA (D-1), com `notify`
// controlando se o AuditConsumer deve enfileirar o resumo no WhatsApp ao final (ver
// consumer.go). Usado só pela auditoria diária de fechamento (`trigger`) — a atualização
// horária usa `publishRange`.
func (s *SchedulerService) publish(tenantID int, triggeredBy string, notify bool, logMsg string) {
	payload, err := json.Marshal(map[string]interface{}{
		"tenant_id":    tenantID,
		"triggered_by": triggeredBy,
		"notify":       notify,
	})
	if err != nil {
		log.Printf("[Agendador] Tenant %d: falha ao serializar evento: %v\n", tenantID, err)
		return
	}

	if err := s.publisher.Publish(context.Background(), "audit.trigger", payload); err != nil {
		log.Printf("[Agendador] Tenant %d: falha ao enfileirar auditoria automática: %v\n", tenantID, err)
		return
	}

	log.Printf("[Agendador] Tenant %d: %s.\n", tenantID, logMsg)
}

// publishRange monta e envia o payload de auditoria automática de um PERÍODO
// (start_date/end_date) — sempre com `notify: false`: nenhuma atualização automática de
// período notifica o WhatsApp, só a diária de fechamento (`publish`/`trigger`).
func (s *SchedulerService) publishRange(tenantID int, start, end time.Time, triggeredBy, logMsg string) {
	payload, err := json.Marshal(map[string]interface{}{
		"tenant_id":    tenantID,
		"start_date":   start.Format("2006-01-02"),
		"end_date":     end.Format("2006-01-02"),
		"triggered_by": triggeredBy,
		"notify":       false,
	})
	if err != nil {
		log.Printf("[Agendador] Tenant %d: falha ao serializar evento de período: %v\n", tenantID, err)
		return
	}

	if err := s.publisher.Publish(context.Background(), "audit.trigger", payload); err != nil {
		log.Printf("[Agendador] Tenant %d: falha ao enfileirar auditoria automática de período: %v\n", tenantID, err)
		return
	}

	log.Printf("[Agendador] Tenant %d: %s (%s a %s).\n", tenantID, logMsg, start.Format("2006-01-02"), end.Format("2006-01-02"))
}
