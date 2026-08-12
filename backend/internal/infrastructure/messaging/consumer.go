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
	channel        *amqp.Channel
	tenantRepo     domain.TenantRepository
	collabRepo     domain.CollaboratorRepository
	secullumSvc    domain.SecullumService
	reportRepo     domain.ReportRepository
	auditorCore    *usecase.AuditorService
	reconciler     *usecase.ReconcilerService
	publisher      *ChannelPool
	instancePrefix string // prefixo p/ derivar a instância de WhatsApp do tenant
}

func NewAuditConsumer(
	ch *amqp.Channel,
	tr domain.TenantRepository,
	cr domain.CollaboratorRepository,
	ss domain.SecullumService,
	rr domain.ReportRepository,
	ac *usecase.AuditorService,
	rc *usecase.ReconcilerService,
	publisher *ChannelPool,
	instancePrefix string,
) *AuditConsumer {
	return &AuditConsumer{
		channel:        ch,
		tenantRepo:     tr,
		collabRepo:     cr,
		secullumSvc:    ss,
		reportRepo:     rr,
		auditorCore:    ac,
		reconciler:     rc,
		publisher:      publisher,
		instancePrefix: instancePrefix,
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
	// 1. Decodifica o JSON que veio do Handler HTTP.
	//
	// `date` é opcional e serve para auditar um dia específico sob demanda. Sem ele —
	// que é o caso da varredura diária automática — audita-se D-1, como sempre.
	var payload struct {
		TenantID int    `json:"tenant_id"`
		Date     string `json:"date"`
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

	// 4. Determina o dia auditado — o informado no evento (auditoria sob demanda de um dia
	// específico) ou, na ausência dele, D-1: o fechamento diário automático de sempre. E o
	// dia anterior a ele, necessário apenas para a regra de interjornada.
	diaAlvo := resolveTargetDay(payload.Date, time.Now())
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

	// 6. Reconcilia as ocorrências do dia: é aqui que a auditoria deixa de empilhar listas
	// e passa a mover estados. Auditar o mesmo dia de novo não duplica nada — atualiza o
	// que mudou e resolve sozinho o que deixou de ser apurado.
	resumo, err := c.reconcile(payload.TenantID, diaAlvo, inconsistencies)
	if err != nil {
		// Sem reconciliar, o estado do dia fica pela metade: devolve à fila para retry.
		c.reject(msg, true, payload.TenantID, fmt.Sprintf("falha ao reconciliar ocorrências: %v", err))
		return
	}

	// 7. Salva o Relatório consolidado no Banco de Dados (registro da execução da
	// varredura, base do histórico e do gráfico de evolução do painel).
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

	log.Printf("[OK] Auditoria de %s concluída! %d colaborador(es) avaliado(s), %d infração(ões) apurada(s) — %d nova(s), %d atualizada(s), %d reaberta(s), %d resolvida(s), %d inalterada(s).\n",
		diaAlvo.Format("2006-01-02"), len(collaborators), len(inconsistencies),
		len(resumo.Created), len(resumo.Updated), len(resumo.Reopened), len(resumo.Resolved), resumo.Unchanged)

	// 8. Notifica os gestores (staffs) do tenant com o resumo do fechamento noturno,
	// publicando na fila `notifications.whatsapp` (seção 5.4 da especificação). A
	// entrega via Evolution API é responsabilidade do worker de notificações — este
	// worker apenas enfileira, sem esperar a latência da API externa.
	c.notifyStaffs(tenant, diaAlvo, resumo)

	// 9. Confirma (Ack) para o RabbitMQ apagar a mensagem da fila
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
func (c *AuditConsumer) notifyStaffs(tenant *domain.Tenant, dia time.Time, resumo usecase.ReconcileResult) {
	if c.publisher == nil {
		return
	}
	if len(tenant.Staffs) == 0 {
		log.Printf("[Aviso Notificação] Tenant %d: nenhum staff cadastrado, alerta não enviado.\n", tenant.ID)
		return
	}

	message := buildClosingSummaryMessage(tenant.Name, dia, resumo)
	instance := domain.WhatsAppInstanceName(c.instancePrefix, tenant.ID)

	for _, staff := range tenant.Staffs {
		notification := domain.WhatsAppNotification{
			TenantID: tenant.ID,
			StaffID:  staff.ID,
			Instance: instance,
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

// buildClosingSummaryMessage monta o texto enviado aos gestores no fechamento.
//
// A mensagem reporta o que MUDOU nesta varredura, não a lista inteira de novo. Com a
// máquina de estados, reenviar todas as ocorrências a cada auditoria repetiria dezenas de
// linhas já conhecidas e treinaria o gestor a ignorar o alerta; o que ele precisa saber é
// o que apareceu, o que piorou, o que voltou e o que se resolveu.
func buildClosingSummaryMessage(tenantName string, dia time.Time, resumo usecase.ReconcileResult) string {
	header := fmt.Sprintf("🏢 *Empresa:* %s\n📅 *Data:* %s\n", tenantName, dia.Format("02/01/2006"))

	if !resumo.Changed() {
		if resumo.Unchanged == 0 && resumo.Ignored == 0 {
			return "✅ *Fechamento de Jornada*\n\n" + header + "\nNenhuma inconsistência encontrada."
		}
		return fmt.Sprintf(
			"✅ *Fechamento de Jornada*\n\n%s\nNenhuma novidade: %d ocorrência(s) já conhecida(s) seguem em aberto.",
			header, resumo.Unchanged,
		)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "⚠️ *Fechamento de Jornada*\n\n%s", header)
	fmt.Fprintf(&b, "🆕 *Novas:* %d | 🔁 *Atualizadas:* %d | ↩️ *Reabertas:* %d | ✅ *Resolvidas:* %d\n",
		len(resumo.Created), len(resumo.Updated), len(resumo.Reopened), len(resumo.Resolved))

	writeGroup(&b, "🆕 *Novas ocorrências*", resumo.Created)
	writeGroup(&b, "🔁 *Ocorrências que mudaram*", resumo.Updated)
	writeGroup(&b, "↩️ *Voltaram a ocorrer*", resumo.Reopened)

	if len(resumo.Resolved) > 0 {
		fmt.Fprintf(&b, "\n✅ *Resolvidas automaticamente*\n")
		for _, occ := range resumo.Resolved {
			fmt.Fprintf(&b, "• %s — %s\n", occ.CollaboratorName, occ.Type)
		}
	}
	if resumo.Unchanged > 0 {
		fmt.Fprintf(&b, "\n_%d ocorrência(s) já conhecida(s) seguem em aberto, sem mudança._\n", resumo.Unchanged)
	}

	return b.String()
}

// writeGroup imprime um bloco de ocorrências com a descrição completa — são as que exigem
// leitura, ao contrário das resolvidas, que só precisam ser contadas.
func writeGroup(b *strings.Builder, title string, occurrences []domain.Occurrence) {
	if len(occurrences) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s\n", title)
	for _, occ := range occurrences {
		fmt.Fprintf(b, "• %s (%s) — %s: %s\n", occ.CollaboratorName, occ.Severity, occ.Type, occ.Description)
	}
}

// resolveTargetDay devolve o dia a auditar: o informado no evento (auditoria sob demanda)
// ou D-1 quando ausente/ilegível — o fechamento diário automático, que nunca deve deixar
// de rodar por causa de um campo malformado.
func resolveTargetDay(raw string, now time.Time) time.Time {
	if raw == "" {
		return now.AddDate(0, 0, -1)
	}
	day, err := time.Parse("2006-01-02", raw)
	if err != nil {
		log.Printf("[Aviso Auditoria] data %q inválida no evento; auditando D-1: %v", raw, err)
		return now.AddDate(0, 0, -1)
	}
	return day
}

// reconcile aplica a máquina de estados ao dia auditado. O reconciliador é opcional na
// construção do worker (os testes montam o consumer sem ele); sem reconciliador, a
// auditoria segue gravando o relatório como antes.
func (c *AuditConsumer) reconcile(tenantID int, dia time.Time, inconsistencies []domain.AuditInconsistency) (usecase.ReconcileResult, error) {
	if c.reconciler == nil {
		return usecase.ReconcileResult{}, nil
	}
	return c.reconciler.Reconcile(tenantID, dia, inconsistencies, time.Now())
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
