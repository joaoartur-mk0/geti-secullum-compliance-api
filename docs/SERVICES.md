# Services (`usecase/`)

Os serviços em `backend/internal/usecase/` orquestram regra de negócio que atravessa mais
de uma entidade de domínio, ou que precisa coordenar chamadas externas (Secullum,
RabbitMQ). Diferente dos handlers HTTP, não sabem nada de `gin.Context` — recebem tipos de
domínio e devolvem tipos de domínio ou erro. São consumidos tanto pelos handlers quanto
pelos workers RabbitMQ (`infrastructure/messaging/`).

## `AuditorService` (`auditor.go`)

O motor de regras. Audita **um dia** de **um colaborador** contra a legislação
trabalhista e devolve as inconsistências apuradas (`AuditInconsistency`). Regras
implementadas, com os limiares travados no código (ver
`docs/documento-funcional-compliance.md` §4 e §6.2 — parametrização por cliente está
fora do escopo atual):

| Tipo | Regra |
|---|---|
| `Batida Esquecida` | Par de marcação incompleto no dia |
| `Almoço Reduzido` | Intervalo intrajornada abaixo do mínimo (Art. 71 CLT — 60 min se jornada > 6h, 15 min se > 4h e ≤ 6h) |
| `Interjornada Curta` | Menos de 11h entre o fim de uma jornada e o início da próxima |
| `Hora Extra Excedente` | Acima do limite crítico (120 min) |
| `Alerta de Hora Extra` | Acima do limite de alerta (60 min), abaixo do crítico |
| `Carga Horária Não Apurada` | Sem `Memoria*` no dia (a Secullum não resolveu a jornada esperada) |
| `Trabalho em Dia de Folga/DSR` | Marcação num dia sinalizado como folga |

A carga esperada do dia vem dos campos `Memoria*` do endpoint de batidas (a jornada que a
Secullum efetivamente aplicou naquele dia), não da grade semanal do horário — a grade
diverge em escalas, trocas pontuais e feriados.

`TypeRequiresAttachment(tipo string) bool` — exportada, usada pela Feature 4 (tratativa)
para decidir se um anexo é obrigatório antes de aceitar a tratativa. Tabela travada, deve
ser mantida em sincronia com `docs/documento-funcional-compliance.md` §4.

## `ReconcilerService` (`reconciler.go`)

Recebe a lista de inconsistências apuradas num dia e o estado anterior das ocorrências
daquele dia/tenant, e decide o que fazer com cada uma — a máquina de estados em ação:

- Nunca vista antes → `ChangeInsert` (nova ocorrência, estado `aberta`).
- Já existe, mesmo `Fingerprint` (mesmo valor apurado) → `ChangeTouch` (só atualiza
  `last_seen_at`/`times_seen`, sem gerar evento).
- Já existe, `Fingerprint` diferente → `ChangeUpdate` (estado `atualizada`, evento
  registrado).
- Tinha desfecho humano (`tratada` ou `resolvida_manual` — `OccurrenceState.Sticky()`) →
  `ChangeTouch`, nunca reabre sozinha.
- Estava aberta e deixou de ser apurada → `ChangeResolve` (`resolvida_automatica`): a
  batida foi corrigida na origem.

`Fingerprint` resume tipo+severidade+descrição num hash — muda exatamente quando o valor
apurado muda, sem precisar de um campo estruturado por regra.

## `SynchronizerService` (`synchronizer.go`)

Busca o espelho de colaboradores (identidade, jornada, departamento/função/empresa) e
equipamentos na Secullum e persiste localmente via `SaveAll` (upsert em transação única).
Deduplica a busca de jornada (`GetHorario`) por número de horário, não por colaborador —
poucos horários distintos cobrem centenas de funcionários, e cada chamada extra consome o
rate limit da Secullum (100 req/min).

## `BranchResolverService` (`branch_resolver.go`)

Resolve a qual filial uma batida pertence, em duas tentativas: pelo aparelho (`EquipId` da
batida, resolução forte) e, se não achar, pelo número de folha do colaborador (cadastro de
lotação, cobre o caso majoritário — a maioria das batidas reais vem do app/web, sem
`EquipId`). Não encontrar filial não é erro — devolve `BranchUnresolved` e o painel mostra
"Sem filial".

## `SchedulerService` (`scheduler.go`)

Roda em goroutine própria desde a subida do processo (`main.go`). Dispara, sozinho, dois
tipos de auditoria por tenant ativo:

1. **Diária de fechamento** — no horário configurado em `TenantSettings.Horario` (aba
   Avisos do painel), no máximo uma vez por dia por tenant. É a única que notifica o
   WhatsApp dos gestores (`notify: true`).
2. **Atualização horária silenciosa** — de hora em hora, reauditando o mês corrente
   inteiro até D-1 (não só D-1), sem notificar, para capturar correções feitas na
   Secullum a qualquer momento do mês. Uma única chamada à Secullum busca o período
   inteiro (`GetDailyPunchesRange`), depois separado por dia.

Publica o mesmo payload que `POST /audit/trigger` publica na fila `audit.trigger` — não é
um motor de auditoria paralelo, só o gatilho automático.

## `TreatmentService` (`treatment_service.go`)

Feature 4 (tratativa). Valida antes de gravar: ocorrência existe, justificativa não vazia,
anexo obrigatório conforme `AuditorService.TypeRequiresAttachment`, anexo é PDF de verdade
(assinatura `%PDF-`, não só o `Content-Type` declarado pelo cliente) e dentro do limite de
tamanho (`AttachmentMaxSizeBytes`, 5 MB). Sanitiza o nome do arquivo (`sanitizeFileName`)
contra path traversal e quebra de header HTTP no download. A transação em si (gravar
tratativa + anexo(s) + transicionar a ocorrência para `tratada`) é responsabilidade do
`domain.TreatmentRepository`, não deste serviço — `TreatmentService` decide *se* pode
gravar, o repositório grava *atomicamente*.

## Fora de `usecase/`: consumers RabbitMQ

Os workers assíncronos (`infrastructure/messaging/`) usam estes serviços mas não são eles
mesmos serviços de domínio:

- **`AuditConsumer`** — consome `audit.trigger`, busca dados na Secullum, roda
  `AuditorService` + `ReconcilerService`, persiste e publica em
  `notifications.whatsapp` quando `notify: true`.
- **`ProvisioningConsumer`** — consome `tenant.provisioning`, chama `SynchronizerService`.
- **`NotificationConsumer`** — consome `notifications.whatsapp`, envia via cliente da
  Evolution API.

Ver [`ARCHITECTURE.md`](./ARCHITECTURE.md#fluxo-assíncrono-rabbitmq) para o fluxo
completo.
