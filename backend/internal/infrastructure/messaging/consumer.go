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
	channel         *amqp.Channel
	tenantRepo      domain.TenantRepository
	collabRepo      domain.CollaboratorRepository
	secullumSvc     domain.SecullumService
	reportRepo      domain.ReportRepository
	punchRecordRepo domain.PunchRecordRepository
	auditorCore     *usecase.AuditorService
	reconciler      *usecase.ReconcilerService
	publisher       *ChannelPool
	instancePrefix  string // prefixo p/ derivar a instância de WhatsApp do tenant
}

func NewAuditConsumer(
	ch *amqp.Channel,
	tr domain.TenantRepository,
	cr domain.CollaboratorRepository,
	ss domain.SecullumService,
	rr domain.ReportRepository,
	prr domain.PunchRecordRepository,
	ac *usecase.AuditorService,
	rc *usecase.ReconcilerService,
	publisher *ChannelPool,
	instancePrefix string,
) *AuditConsumer {
	return &AuditConsumer{
		channel:         ch,
		tenantRepo:      tr,
		collabRepo:      cr,
		secullumSvc:     ss,
		reportRepo:      rr,
		punchRecordRepo: prr,
		auditorCore:     ac,
		reconciler:      rc,
		publisher:       publisher,
		instancePrefix:  instancePrefix,
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
	// 1. Decodifica o JSON que veio do Handler HTTP ou do SchedulerService.
	//
	// `date` é opcional e serve para auditar um dia específico sob demanda. `start_date`/
	// `end_date` auditam um período completo (semana, mês) de uma vez, salvando um
	// relatório por dia. Sem nenhum dos três — o caso da varredura diária automática —
	// audita-se D-1, como sempre.
	//
	// `notify` controla se o resumo desta auditoria é enfileirado no WhatsApp dos
	// gestores. É `true` SOMENTE na varredura diária automática do horário configurado
	// (aba Avisos); auditorias manuais (handler HTTP) e a atualização horária silenciosa
	// (SchedulerService.hourlyTick) sempre publicam `false` — nunca notificam.
	var payload struct {
		TenantID  int    `json:"tenant_id"`
		Date      string `json:"date"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Notify    bool   `json:"notify"`
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

	// 4. Determina os dias a auditar — o período informado (start_date/end_date), o dia
	// específico informado (date), ou, na ausência de todos, D-1: o fechamento diário
	// automático de sempre. `dias` vem do mais antigo ao mais recente.
	dias := resolveTargetDays(payload.Date, payload.StartDate, payload.EndDate, time.Now())

	// 5. Busca as batidas de TODO o período numa ÚNICA chamada à Secullum — inclusive o
	// dia anterior ao primeiro dia auditado, necessário só para a interjornada dele. Isto
	// é o que evita 1 requisição por dia numa auditoria de período completo (semana/mês)
	// e o consequente risco de rate limiting.
	rangeStart := dias[0].AddDate(0, 0, -1)
	rangeEnd := dias[len(dias)-1]
	punches, err := c.secullumSvc.GetDailyPunchesRange(tenant, rangeStart, rangeEnd)
	if err != nil {
		// Falha externa (rede/token/rate-limit) é transitória: devolve à fila para retry.
		c.reject(msg, true, payload.TenantID, fmt.Sprintf("falha ao buscar batidas de %s a %s: %v",
			rangeStart.Format("2006-01-02"), rangeEnd.Format("2006-01-02"), err))
		return
	}
	punchesByDay := indexPunchesByDay(punches)

	// 5b. Busca, no MESMO período, a origem de cada marcação (FonteDados) para
	// enriquecer a auditoria com equipamento/motivo — cruzando pelo Id que a própria
	// resposta de batidas já traz (PunchPair.FonteDadosIDEntrada/FonteDadosIDSaida).
	// Falha aqui NÃO aborta a auditoria (é um enriquecimento, não o dado principal):
	// fica registrada e os relatórios seguem sem equipamento/motivo neste ciclo.
	fonteDadosByID := c.fetchFonteDadosByID(tenant, rangeStart, rangeEnd)

	// 6. Audita cada dia do período separadamente, salvando um relatório por dia — é
	// isso que permite consultar/filtrar dias específicos depois, mesmo quando a
	// auditoria foi disparada para um período inteiro de uma vez.
	for _, diaAlvo := range dias {
		resumo, inconsistencies, err := c.auditDay(tenant, collaborators, diaAlvo, punchesByDay, fonteDadosByID)
		if err != nil {
			// Sem reconciliar/salvar, o estado do dia fica pela metade: devolve à fila
			// para retry. Dias já processados com sucesso antes deste no mesmo período
			// serão reprocessados também — inofensivo para a reconciliação (idempotente),
			// mas pode duplicar o relatório já salvo desses dias (mesmo risco que já
			// existia numa falha de Ack de dia único).
			c.reject(msg, true, payload.TenantID, err.Error())
			return
		}

		log.Printf("[OK] Auditoria de %s concluída! %d colaborador(es) avaliado(s), %d infração(ões) apurada(s) — %d nova(s), %d atualizada(s), %d reaberta(s), %d resolvida(s), %d inalterada(s).\n",
			diaAlvo.Format("2006-01-02"), len(collaborators), len(inconsistencies),
			len(resumo.Created), len(resumo.Updated), len(resumo.Reopened), len(resumo.Resolved), resumo.Unchanged)

		// Notifica os gestores (staffs) do tenant com o resumo do fechamento deste dia —
		// SOMENTE quando `notify` veio true no evento (a varredura diária automática do
		// horário configurado). Auditorias manuais e a atualização horária silenciosa
		// atualizam os dados sem repetir o alerta no WhatsApp. A publicação em
		// `notifications.whatsapp` (seção 5.4 da especificação) só enfileira — a entrega
		// via Evolution API é responsabilidade do worker de notificações.
		if payload.Notify {
			c.notifyStaffs(tenant, diaAlvo, resumo)
		}
	}

	// 7. Confirma (Ack) para o RabbitMQ apagar a mensagem da fila
	if err := msg.Ack(false); err != nil {
		// Os relatórios já foram salvos; o Ack falhou. A mensagem poderá ser reentregue e
		// gerar relatórios duplicados — registrado aqui para rastreabilidade.
		log.Printf("[Erro] Tenant %d: relatório(s) salvo(s), mas falha ao confirmar (Ack) mensagem: %v\n", payload.TenantID, err)
	}
}

// auditDay audita todos os colaboradores para UM dia e persiste o resultado (reconciliação
// de ocorrências + relatório). `punchesByDay` já contém as batidas de todo o período
// (ver processMessage) — este método só faz a leitura do dia alvo e do dia anterior a ele.
func (c *AuditConsumer) auditDay(
	tenant *domain.Tenant,
	collaborators []domain.Collaborator,
	diaAlvo time.Time,
	punchesByDay map[string]map[int]domain.DailyPunch,
	fonteDadosByID map[int]domain.FonteDadoItem,
) (usecase.ReconcileResult, []domain.AuditInconsistency, error) {
	mapaAlvo := punchesByDay[diaAlvo.Format("2006-01-02")]
	mapaAnterior := punchesByDay[diaAlvo.AddDate(0, 0, -1).Format("2006-01-02")]

	// Audita CADA colaborador sincronizado do tenant.
	var inconsistencies []domain.AuditInconsistency
	var punchRecords []domain.PunchRecord
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
		} else if rec, found := buildPunchRecord(tenant.ID, collab.SecullumID, diaAlvo, punchAlvo, fonteDadosByID); found {
			punchRecords = append(punchRecords, rec)
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
			true, // isClosing: esta é a consolidação do dia encerrado
		)
		if err != nil {
			// Erros de qualidade de dado (ex.: batida em formato inválido) não abortam a
			// auditoria dos demais colaboradores nem são descartados: ficam registrados.
			log.Printf("[Aviso Auditoria] Tenant %d, colaborador %d (%s), dia %s: regras retornaram erros de dado: %v\n",
				tenant.ID, collab.SecullumID, collab.Name, diaAlvo.Format("2006-01-02"), err)
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

	// Reconcilia as ocorrências do dia: é aqui que a auditoria deixa de empilhar listas
	// e passa a mover estados. Auditar o mesmo dia de novo não duplica nada — atualiza o
	// que mudou e resolve sozinho o que deixou de ser apurado.
	resumo, err := c.reconcile(tenant.ID, diaAlvo, inconsistencies)
	if err != nil {
		return usecase.ReconcileResult{}, nil, fmt.Errorf("falha ao reconciliar ocorrências de %s: %w", diaAlvo.Format("2006-01-02"), err)
	}

	// Salva o Relatório consolidado no Banco de Dados (registro da execução da varredura,
	// base do histórico e do gráfico de evolução do painel).
	report := &domain.Report{
		TenantID:        tenant.ID,
		Date:            diaAlvo,
		DataGenerated:   time.Now(),
		Inconsistencies: inconsistencies,
	}
	if err := c.reportRepo.Save(report); err != nil {
		return usecase.ReconcileResult{}, nil, fmt.Errorf("falha ao salvar relatório de %s: %w", diaAlvo.Format("2006-01-02"), err)
	}

	// Persiste o enriquecimento de equipamento/motivo apurado para o dia. É informação
	// auxiliar (não a auditoria em si): uma falha aqui é registrada, mas não derruba o
	// dia já reconciliado e salvo acima.
	if c.punchRecordRepo != nil && len(punchRecords) > 0 {
		if err := c.punchRecordRepo.SaveAll(punchRecords); err != nil {
			log.Printf("[Aviso Auditoria] Tenant %d, dia %s: falha ao salvar equipamento/motivo das marcações: %v\n",
				tenant.ID, diaAlvo.Format("2006-01-02"), err)
		}
	}

	return resumo, inconsistencies, nil
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

// resolveTargetDays devolve os dias a auditar, do mais antigo ao mais recente.
//
// Com startDate/endDate (auditoria de período completo) devolve o intervalo inteiro;
// caso contrário, um único dia — ver resolveTargetDay. Um período malformado (datas
// inválidas ou end antes de start) nunca deveria chegar aqui — o handler HTTP já valida
// isso antes de publicar o evento — mas, se chegar, cai no mesmo fallback seguro que uma
// `date` malformada: audita D-1 em vez de travar o worker.
func resolveTargetDays(date, startDate, endDate string, now time.Time) []time.Time {
	if startDate != "" && endDate != "" {
		start, errStart := time.Parse("2006-01-02", startDate)
		end, errEnd := time.Parse("2006-01-02", endDate)
		if errStart == nil && errEnd == nil && !end.Before(start) {
			days := make([]time.Time, 0, int(end.Sub(start).Hours()/24)+1)
			for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
				days = append(days, d)
			}
			return days
		}
		log.Printf("[Aviso Auditoria] período %q..%q inválido no evento; auditando D-1: start=%v end=%v", startDate, endDate, errStart, errEnd)
	}
	return []time.Time{resolveTargetDay(date, now)}
}

// indexPunchesByDay agrupa as batidas por dia (YYYY-MM-DD) e, dentro de cada dia, por
// colaborador — o formato que a auditoria de período usa para varrer vários dias a partir
// de uma única resposta da Secullum, sem repetir a busca dia a dia.
func indexPunchesByDay(punches []domain.DailyPunch) map[string]map[int]domain.DailyPunch {
	byDay := make(map[string][]domain.DailyPunch)
	for _, p := range punches {
		day := p.Date.Format("2006-01-02")
		byDay[day] = append(byDay[day], p)
	}
	out := make(map[string]map[int]domain.DailyPunch, len(byDay))
	for day, dayPunches := range byDay {
		out[day] = indexPunchesByCollaborator(dayPunches)
	}
	return out
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

// fetchFonteDadosByID busca a origem das marcações do período (FonteDados) e indexa por
// Id — a chave usada para cruzar com PunchPair.FonteDadosIDEntrada/FonteDadosIDSaida. Uma
// falha na chamada é só registrada: o enriquecimento é auxiliar, não deve reprocessar (ou
// bloquear) a auditoria inteira, cujo dado principal já foi obtido em GetDailyPunchesRange.
func (c *AuditConsumer) fetchFonteDadosByID(tenant *domain.Tenant, start, end time.Time) map[int]domain.FonteDadoItem {
	items, err := c.secullumSvc.GetFonteDados(tenant, start, end)
	if err != nil {
		log.Printf("[Aviso Auditoria] Tenant %d: falha ao buscar FonteDados de %s a %s (auditoria segue sem equipamento/motivo): %v\n",
			tenant.ID, start.Format("2006-01-02"), end.Format("2006-01-02"), err)
		return nil
	}

	byID := make(map[int]domain.FonteDadoItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	return byID
}

// buildPunchRecord procura, entre as marcações do dia, a primeira com correspondência em
// FonteDados (pelo Id que a própria batida já traz) e devolve o registro de enriquecimento
// do dia. found=false quando não há Id de fonte de dados no dia, ou nenhum bate com o
// período consultado — caso registrado (não é erro: cadastro incompleto/marcação sem
// origem rastreada), mas não impede a auditoria de seguir.
func buildPunchRecord(tenantID, collaboratorID int, date time.Time, punch domain.DailyPunch, fonteDadosByID map[int]domain.FonteDadoItem) (domain.PunchRecord, bool) {
	if len(fonteDadosByID) == 0 {
		return domain.PunchRecord{}, false
	}

	for _, pair := range punch.Marcacoes {
		for _, fonteID := range []*int{pair.FonteDadosIDEntrada, pair.FonteDadosIDSaida} {
			if fonteID == nil {
				continue
			}
			item, ok := fonteDadosByID[*fonteID]
			if !ok {
				log.Printf("[Aviso Auditoria] Tenant %d, colaborador %d, dia %s: FonteDados id %d sem correspondência no período consultado.\n",
					tenantID, collaboratorID, date.Format("2006-01-02"), *fonteID)
				continue
			}
			return domain.PunchRecord{
				TenantID:       tenantID,
				CollaboratorID: collaboratorID,
				Date:           date,
				EquipamentoID:  item.EquipamentoID,
				Motivo:         item.Motivo,
			}, true
		}
	}
	return domain.PunchRecord{}, false
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
