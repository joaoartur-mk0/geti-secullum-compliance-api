# Controllers (handlers HTTP)

Cada arquivo em `backend/internal/interface/http/handlers/` é um controller — traduz
requisição HTTP em chamada de domínio/usecase e devolve JSON. Convenções comuns a todos:

- `const op = "NomeDoHandler.Metodo"` no início de cada método, usado para nomear a
  operação nos erros estruturados (`domain.NewValidation(op, ...)` etc.).
- Erros sempre devolvidos via `httperr.Respond(c, err)` — nunca `c.JSON` direto de erro.
- Quando o tenant não vem no path (`/tenants/:id/...`), o handler carrega o recurso
  primeiro (pelo id da rota) e só então confere acesso via
  `ensureTenantAccess(c, userTenantRepo, op, recurso.TenantID)` — ver `common.go`.
- `idParam`, `actorUserID`, `bindJSON` (também em `common.go`) são os helpers
  compartilhados de parse de rota/corpo e identidade do usuário autenticado.

Para o mapa completo de rota → handler → acesso, ver [`ROUTES.md`](./ROUTES.md).

## `user_handler.go` — `UserHandler`

Autenticação e ciclo de vida de usuário. `Login` é a única rota pública do sistema.
`Register` exige super admin (o primeiro usuário vem do seed em `main.go`, não desta
rota). `Get`/`ListTenants`/`UpdateEmail`/`UpdatePassword` são "próprio ou super admin".
`Activate`/`Deactivate`/`Delete`/`List` são exclusivos de super admin.

## `tenant_handler.go` — `TenantHandler`

CRUD de tenant (cliente) e o vínculo usuário↔tenant. `List` filtra por vínculo dentro do
handler (não é uma rota exclusiva de super admin, mas cada usuário só vê os tenants aos
quais está vinculado). `Sync` dispara a sincronização de colaboradores/equipamentos sob
demanda. `AddUser`/`RemoveUser`/`ListUsers` gerenciam quem tem acesso a qual tenant.

## `audit_handler.go` — `AuditHandler`

`TriggerAudit` — publica um pedido de auditoria na fila `audit.trigger` (consumida por
`AuditConsumer`, ver `SERVICES.md`). Aceita um dia específico, um período, ou nenhum (D-1
por padrão), mais a flag `notify`. O `tenant_id` vem no corpo, não no path — o acesso é
checado dentro do handler.

## `occurrence_handler.go` — `OccurrenceHandler`

`List` — consulta filtrada de ocorrências (`?start_date=&end_date=&state=&severity=`
etc.), a tela principal do painel. `Ignore` — marca uma ocorrência como
`resolvida_manual` (desfecho "ignorada", com motivo opcional); bloqueia se a ocorrência já
tiver tratativa registrada (estado `tratada`), forçando desfazer a tratativa primeiro.
`Events` — log de transições de uma ocorrência (trilha completa de estado).

## `treatment_handler.go` — `TreatmentHandler`

Feature 4 (tratativa). `Treat` — recebe `multipart/form-data` (`justification` +
`attachment` opcional/obrigatório conforme o tipo), valida via
`usecase.TreatmentService` e grava. `Treatments` — lista as tratativas de uma ocorrência
(inclusive as desfeitas). `Undo` — desfaz uma tratativa, devolvendo a ocorrência para
`aberta` sem apagar o registro original. `DownloadAttachment` — único caminho de acesso ao
conteúdo de um anexo; autenticado, confere tenant, registra o download.

## `collaborator_handler.go` — `CollaboratorHandler`

`List`/`History` — espelho local de colaboradores (só ativos / todos), com filtros
opcionais `departamento_id`/`funcao_id`/`empresa_id`. `Filters` — catálogo de
departamentos/funções/empresas distintos do tenant, para alimentar seletores de filtro.
`Prefill` — autopreenchimento da ficha de colaborador (horário fixo + filial resolvida).
`PunchRecords` — de qual equipamento veio a marcação de cada dia, cruzado com a
`FonteDados` da Secullum.

## `branch_handler.go` — `BranchHandler`

CRUD de filial (`Branch`), mais os dois mecanismos de resolução: aparelhos vinculados
(`AddDevice`/`RemoveDevice`) e números de folha vinculados
(`AddPayrollNumber`/`RemovePayrollNumber`) — ver `usecase.BranchResolverService`.

## `warning_handler.go` — `WarningHandler`

CRUD de advertência disciplinar vinculada a um colaborador, mais `UpdateStatus` (fluxo de
aprovação/emissão da advertência).

## `staff_handler.go` — `StaffHandler`

CRUD de responsável (staff) — quem recebe os alertas de WhatsApp por filial/tenant.

## `report_handler.go` — `ReportHandler`

`List` — a auditoria mais recente de cada dia no período (visão de consulta padrão).
`History` — todas as execuções, inclusive reauditorias do mesmo dia (registro de
execuções, não some nada).

## `settings_handler.go` — `SettingsHandler`

`Get`/`Update` de `TenantSettings`: liga/desliga por regra, severidade configurável por
regra, horário da auditoria diária automática.

## `equipment_handler.go` — `EquipmentHandler`

`List` — equipamentos (relógios de ponto) sincronizados do tenant, somente leitura (a
escrita acontece via sincronização, não manualmente).

## `whatsapp_handler.go` — `WhatsAppHandler`

`Status`/`Connect`/`Disconnect` — gerência da instância de WhatsApp do tenant na
Evolution API (QR code, status de conexão).

## `common.go` — helpers compartilhados

Não é um controller, mas sustenta todos: `idParam`, `bindJSON`, `actorUserID`,
`ensureTenantAccess`. Ver comentários no próprio arquivo para o contrato de cada um.
