package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"backend/internal/domain"
	"backend/internal/usecase"

	amqp "github.com/rabbitmq/amqp091-go"
)

type AuditConsumer struct {
	channel     *amqp.Channel
	tenantRepo  domain.TenantRepository
	collabRepo  domain.CollaboratorRepository
	secullumSvc domain.SecullumService
	reportRepo  domain.ReportRepository
	auditorCore *usecase.AuditorService
	publisher   *ChannelPool
}

func NewAuditConsumer(
	ch *amqp.Channel,
	tr domain.TenantRepository,
	cr domain.CollaboratorRepository,
	ss domain.SecullumService,
	rr domain.ReportRepository,
	ac *usecase.AuditorService,
	publisher *ChannelPool,
) *AuditConsumer {
	return &AuditConsumer{
		channel:     ch,
		tenantRepo:  tr,
		collabRepo:  cr,
		secullumSvc: ss,
		reportRepo:  rr,
		auditorCore: ac,
		publisher:   publisher,
	}
}

// Start fica escutando a fila infinitamente em background
func (c *AuditConsumer) Start(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		"audit.trigger", // Nome da fila
		"",              // Consumer tag
		false,           // Auto-Ack (falso = nós confirmamos manualmente quando terminar)
		false,           // Exclusive
		false,           // No-local
		false,           // No-wait
		nil,             // Args
	)
	if err != nil {
		return err
	}

	log.Println("[*] Worker de Auditoria aguardando mensagens...")

	// Loop infinito lendo o canal do Go
	for {
		select {
		case msg := <-msgs:
			c.processMessage(msg)
		case <-ctx.Done():
			log.Println("[*] Desligando Worker de Auditoria graciosamente...")
			return nil
		}
	}
}

// requeueBackoff é a pausa aplicada antes de devolver uma mensagem à fila. O Nack com
// requeue=true recoloca a mensagem no INÍCIO da fila e ela é reentregue imediatamente —
// sem esta pausa, um erro persistente (ex.: credencial inválida na Secullum) vira um
// loop apertado de várias tentativas por segundo, martelando a API externa e o log.
// Var (não const) para os testes poderem zerá-la.
var requeueBackoff = 15 * time.Second

// reject rejeita a mensagem e registra caso o próprio Nack falhe, para que
// nenhuma falha de broker passe despercebida (software auditável).
// requeue=true devolve a mensagem para nova tentativa (com pausa); false a descarta.
func (c *AuditConsumer) reject(msg amqp.Delivery, requeue bool, tenantID int, reason string) {
	log.Printf("[Rejeitando] Tenant %d: %s (requeue=%v)\n", tenantID, reason, requeue)
	if requeue {
		time.Sleep(requeueBackoff)
	}
	if err := msg.Nack(false, requeue); err != nil {
		log.Printf("[Erro] Falha ao rejeitar (Nack) mensagem do Tenant %d: %v\n", tenantID, err)
	}
}

func (c *AuditConsumer) processMessage(msg amqp.Delivery) {
	// 1. Decodifica o JSON que veio do Handler HTTP
	var payload struct {
		TenantID int `json:"tenant_id"`
	}
	if err := json.Unmarshal(msg.Body, &payload); err != nil {
		// Payload malformado nunca ficará válido em nova tentativa: descarta (requeue=false).
		c.reject(msg, false, 0, fmt.Sprintf("payload inválido na fila: %v", err))
		return
	}

	log.Printf("-> Iniciando auditoria para o Tenant %d...\n", payload.TenantID)

	// 2. Busca as configurações do Tenant no Banco
	tenant, err := c.tenantRepo.GetByID(payload.TenantID)
	if err != nil || tenant == nil {
		// Tenant inexistente não se resolve com retry: descarta (requeue=false).
		c.reject(msg, false, payload.TenantID, fmt.Sprintf("tenant não encontrado: %v", err))
		return
	}

	// Auditoria auditável: se nenhuma regra está habilitada nas configurações, o
	// relatório sairá inevitavelmente com 0 inconsistências. Isso é uma escolha do
	// tenant (flags nascem desligadas no cadastro), mas NUNCA deve passar em silêncio —
	// senão parece que o motor avaliou e não achou nada.
	if s := tenant.Settings; s == nil || (!s.Almoco && !s.Interjornada && !s.Hextras && !s.Esquecimento) {
		log.Printf("[Aviso Auditoria] Tenant %d: NENHUMA regra habilitada nas configurações — o relatório sairá sem inconsistências. Habilite as regras (PUT /api/v1/tenants/%d/settings ou pela interface) e dispare novamente.\n",
			payload.TenantID, payload.TenantID)
	}

	// 3. Busca o espelho local de colaboradores (sincronizado via fila
	// tenant.provisioning). Sem colaboradores sincronizados não há o que auditar —
	// isso não é uma falha transitória, então confirmamos a mensagem e apenas avisamos.
	collaborators, err := c.collabRepo.GetByTenantID(tenant.ID)
	if err != nil {
		c.reject(msg, true, payload.TenantID, fmt.Sprintf("falha ao carregar colaboradores: %v", err))
		return
	}
	if len(collaborators) == 0 {
		log.Printf("[Aviso Auditoria] Tenant %d: nenhum colaborador sincronizado. Execute a sincronização (fila tenant.provisioning ou POST /tenants/%d/sync) antes de auditar.\n", payload.TenantID, payload.TenantID)
		if err := msg.Ack(false); err != nil {
			log.Printf("[Erro] Tenant %d: falha ao confirmar (Ack) mensagem sem colaboradores: %v\n", payload.TenantID, err)
		}
		return
	}

	// 4. Determina o dia de fechamento (D-1, o dia anterior já encerrado) e o dia
	// anterior a ele (D-2), necessário apenas para a regra de interjornada.
	diaAlvo := time.Now().AddDate(0, 0, -1)
	diaAnterior := diaAlvo.AddDate(0, 0, -1)

	punchesAlvo, err := c.secullumSvc.GetDailyPunches(tenant, diaAlvo)
	if err != nil {
		// Falha externa (rede/token/rate-limit) é transitória: devolve à fila para retry.
		c.reject(msg, true, payload.TenantID, fmt.Sprintf("falha ao buscar batidas de %s: %v", diaAlvo.Format("2006-01-02"), err))
		return
	}

	// As batidas de D-2 alimentam apenas a regra de interjornada. Um erro aqui não
	// invalida a auditoria de D-1, mas NÃO é silenciado: registramos e seguimos sem elas.
	punchesAnterior, err := c.secullumSvc.GetDailyPunches(tenant, diaAnterior)
	if err != nil {
		log.Printf("[Aviso Secullum] Tenant %d: falha ao buscar batidas de %s (interjornada será ignorada): %v\n",
			payload.TenantID, diaAnterior.Format("2006-01-02"), err)
		punchesAnterior = nil
	}

	// Indexa as batidas por colaborador (o CollaboratorID do DailyPunch é o id do
	// funcionário na Secullum, o mesmo espaço de SecullumID do nosso colaborador local).
	mapaAlvo := indexPunchesByCollaborator(punchesAlvo)
	mapaAnterior := indexPunchesByCollaborator(punchesAnterior)

	// 5. Audita CADA colaborador sincronizado do tenant (não mais um único mockado).
	var inconsistencies []domain.AuditInconsistency
	for i := range collaborators {
		collab := collaborators[i]

		punchAlvo, ok := mapaAlvo[collab.SecullumID]
		if !ok {
			// Sem nenhum registro de batida no dia (ex.: dia de folga/abono já filtrado
			// pelo client, ou o funcionário não bateu ponto algum): sintetiza uma batida
			// vazia (não-nil) para as regras avaliarem "ausência total" sem risco de
			// nil pointer, já que o motor de regras espera um *DailyPunch sempre válido
			// para o dia alvo.
			punchAlvo = domain.DailyPunch{CollaboratorID: collab.SecullumID, Date: diaAlvo}
		}
		punchAnteriorPtr := (*domain.DailyPunch)(nil)
		if p, ok := mapaAnterior[collab.SecullumID]; ok {
			punchAnteriorPtr = &p
		}

		collabInconsistencies, err := c.auditorCore.ProcessRules(
			tenant.Settings,
			&collab,
			&punchAlvo,
			punchAnteriorPtr,
			time.Now(),
			true, // isClosing: esta é a consolidação noturna do dia encerrado (D-1)
		)
		if err != nil {
			// Erros de qualidade de dado (ex.: batida em formato inválido) não abortam a
			// auditoria dos demais colaboradores nem são descartados: ficam registrados.
			log.Printf("[Aviso Auditoria] Tenant %d, colaborador %d (%s): regras retornaram erros de dado: %v\n",
				payload.TenantID, collab.SecullumID, collab.Name, err)
		}

		// Uniformiza a identificação da infração pelo id da Secullum (a chave de
		// negócio visível externamente), já que as regras internas nem sempre
		// preenchem CollaboratorID/CollaboratorName de forma consistente entre si.
		for j := range collabInconsistencies {
			collabInconsistencies[j].CollaboratorID = collab.SecullumID
			collabInconsistencies[j].CollaboratorName = collab.Name
		}
		inconsistencies = append(inconsistencies, collabInconsistencies...)
	}

	// 6. Salva o Relatório consolidado no Banco de Dados (data alvo = D-1).
	report := &domain.Report{
		TenantID:        tenant.ID,
		Date:            diaAlvo,
		DataGenerated:   time.Now(),
		Inconsistencies: inconsistencies,
	}
	if err := c.reportRepo.Save(report); err != nil {
		// Persistência falhou: não podemos perder a auditoria. Devolve à fila (requeue=true).
		c.reject(msg, true, payload.TenantID, fmt.Sprintf("falha ao salvar relatório: %v", err))
		return
	}

	log.Printf("[OK] Auditoria concluída! %d colaborador(es) avaliado(s), %d infração(ões) encontrada(s).\n",
		len(collaborators), len(inconsistencies))

	// 7. Notifica os gestores (staffs) do tenant com o resumo do fechamento noturno,
	// publicando na fila `notifications.whatsapp` (seção 5.4 da especificação). A
	// entrega via Evolution API é responsabilidade do worker de notificações — este
	// worker apenas enfileira, sem esperar a latência da API externa.
	c.notifyStaffs(tenant, diaAlvo, inconsistencies)

	// 8. Confirma (Ack) para o RabbitMQ apagar a mensagem da fila
	if err := msg.Ack(false); err != nil {
		// O relatório já foi salvo; o Ack falhou. A mensagem poderá ser reentregue e
		// gerar um relatório duplicado — registrado aqui para rastreabilidade.
		log.Printf("[Erro] Tenant %d: relatório salvo, mas falha ao confirmar (Ack) mensagem: %v\n", payload.TenantID, err)
	}
}

// notifyStaffs publica, para cada gestor (staff) cadastrado do tenant, um resumo da
// auditoria de fechamento na fila `notifications.whatsapp`. Sem staffs cadastrados,
// apenas registra o aviso — não é um erro (o tenant pode ainda não ter configurado
// um responsável), então a auditoria não é reprocessada por isso.
func (c *AuditConsumer) notifyStaffs(tenant *domain.Tenant, dia time.Time, inconsistencies []domain.AuditInconsistency) {
	if c.publisher == nil {
		return
	}
	if len(tenant.Staffs) == 0 {
		log.Printf("[Aviso Notificação] Tenant %d: nenhum staff cadastrado, alerta não enviado.\n", tenant.ID)
		return
	}

	message := buildClosingSummaryMessage(tenant.Name, dia, inconsistencies)

	for _, staff := range tenant.Staffs {
		notification := domain.WhatsAppNotification{
			TenantID: tenant.ID,
			StaffID:  staff.ID,
			Number:   staff.Celular,
			Message:  message,
		}
		body, err := json.Marshal(notification)
		if err != nil {
			log.Printf("[Erro Notificação] Tenant %d, staff %d: falha ao serializar alerta: %v\n", tenant.ID, staff.ID, err)
			continue
		}
		if err := c.publisher.Publish(context.Background(), "notifications.whatsapp", body); err != nil {
			log.Printf("[Erro Notificação] Tenant %d, staff %d: falha ao publicar alerta: %v\n", tenant.ID, staff.ID, err)
		}
	}
}

// buildClosingSummaryMessage monta o texto do resumo consolidado noturno enviado
// aos gestores, no mesmo tom do exemplo documentado na especificação (seção 5.4).
func buildClosingSummaryMessage(tenantName string, dia time.Time, inconsistencies []domain.AuditInconsistency) string {
	if len(inconsistencies) == 0 {
		return fmt.Sprintf(
			"✅ *Fechamento de Jornada*\n\n🏢 *Empresa:* %s\n📅 *Data:* %s\n\nNenhuma inconsistência encontrada.",
			tenantName, dia.Format("02/01/2006"),
		)
	}

	var criticas, alertas int
	for _, inc := range inconsistencies {
		if inc.Severity == domain.SeverityCritical {
			criticas++
		} else {
			alertas++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "⚠️ *Fechamento de Jornada*\n\n🏢 *Empresa:* %s\n📅 *Data:* %s\n", tenantName, dia.Format("02/01/2006"))
	fmt.Fprintf(&b, "🔴 *Críticas:* %d | 🟡 *Alertas:* %d\n\n*Ocorrências:*\n", criticas, alertas)
	for _, inc := range inconsistencies {
		fmt.Fprintf(&b, "• %s (%s) — %s: %s\n", inc.CollaboratorName, inc.Severity, inc.Type, inc.Description)
	}
	return b.String()
}

// indexPunchesByCollaborator agrupa as batidas por id de colaborador (Secullum) para
// consulta O(1) durante o loop de auditoria.
func indexPunchesByCollaborator(punches []domain.DailyPunch) map[int]domain.DailyPunch {
	m := make(map[int]domain.DailyPunch, len(punches))
	for _, p := range punches {
		m[p.CollaboratorID] = p
	}
	return m
}
