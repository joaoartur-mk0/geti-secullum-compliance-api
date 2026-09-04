# Rotas HTTP

Todas as rotas vivem sob `/api/v1`, exceto `/health`. Salvo `POST /auth/login`, **toda**
rota exige `Authorization: Bearer <token>` (middleware `RequireAuth`). A coluna "Acesso"
descreve a checagem além da autenticação básica. Ver
[`08_Roles_And_Permissions_Contract.md`](./08_Roles_And_Permissions_Contract.md) para o
desenho completo dos quatro papéis (Super Admin, RH, Gestor, Diretoria) — o mecanismo
(`domain.Role`, `RequireTenantRole`, `requireRole`) está implementado e aplicado às rotas
de escrita listadas abaixo com papel mínimo explícito; onde a tabela só diz "Vínculo com
tenant" ou "Autenticado", qualquer papel (piso Diretoria) passa.

Convenção de acesso:

- **Público** — sem autenticação.
- **Super Admin** — exige `is_super_admin`.
- **Autenticado** — qualquer usuário com token válido; a checagem de tenant acontece
  dentro do handler (`ensureTenantAccess`), depois de carregar o recurso pelo id da rota.
  Piso Diretoria — qualquer papel serve.
- **Vínculo com tenant** — middleware `RequireTenantAccess`, aplicado a todo o grupo
  `/tenants/:id/...`. Também piso Diretoria.
- **RH+ / Gestor+** — papel mínimo exigido (RH ⊃ Gestor ⊃ Diretoria, aninhados — RH
  satisfaz qualquer exigência de Gestor). Rotas com `:id` de tenant no path usam
  `middleware.RequireTenantRole`; rotas cujo tenant só é conhecido depois de carregar o
  recurso (`:staffId`, `:warningId`, `:branchId`, `:occurrenceId`, `:treatmentId`) usam
  `requireRole` dentro do handler.
- **Próprio ou Super Admin** — o usuário só acessa o próprio recurso, exceto super admin.

## Health check

| Método | Rota | Acesso |
|---|---|---|
| GET | `/health` | Público — status do Postgres e do RabbitMQ |

## Autenticação e usuários

| Método | Rota | Acesso | Handler |
|---|---|---|---|
| POST | `/auth/login` | Público | `userHandler.Login` |
| POST | `/auth/register` | Super Admin | `userHandler.Register` |
| GET | `/users` | Super Admin | `userHandler.List` |
| GET | `/users/:id` | Próprio ou Super Admin | `userHandler.Get` |
| GET | `/users/:id/tenants` | Próprio ou Super Admin | `userHandler.ListTenants` |
| PUT | `/users/:id/email` | Próprio ou Super Admin | `userHandler.UpdateEmail` |
| PUT | `/users/:id/password` | Próprio ou Super Admin | `userHandler.UpdatePassword` |
| PATCH | `/users/:id/activate` | Super Admin | `userHandler.Activate` |
| PATCH | `/users/:id/deactivate` | Super Admin | `userHandler.Deactivate` |
| DELETE | `/users/:id` | Super Admin | `userHandler.Delete` |

> O primeiro super admin é criado via seed (`SEED_ADMIN_EMAIL`/`SEED_ADMIN_PASSWORD`), não
> por esta API — `/auth/register` já exige um super admin autenticado.

## Tenants

| Método | Rota | Acesso | Handler |
|---|---|---|---|
| GET | `/tenants` | Autenticado (filtrado por vínculo no handler) | `tenantHandler.List` |
| POST | `/tenants` | Super Admin | `tenantHandler.Create` |
| PUT | `/tenants/:id` | Super Admin | `tenantHandler.Update` |
| PATCH | `/tenants/:id/activate` | Super Admin | `tenantHandler.Activate` |
| PATCH | `/tenants/:id/deactivate` | Super Admin | `tenantHandler.Deactivate` |
| DELETE | `/tenants/:id` | Super Admin | `tenantHandler.Delete` |
| POST | `/tenants/:id/users` | Super Admin | `tenantHandler.AddUser` |
| DELETE | `/tenants/:id/users/:userId` | Super Admin | `tenantHandler.RemoveUser` |
| PATCH | `/tenants/:id/users/:userId/role` | Super Admin | `tenantHandler.UpdateUserRole` |
| PATCH | `/users/:id/super-admin` | Super Admin | `userHandler.UpdateSuperAdmin` |

`POST /tenants/:id/users` exige `role` (`rh`\|`gestor`\|`diretoria`) no corpo — sem
default, é o que força a escolha consciente na tela de cadastro. `PATCH .../super-admin`
não permite que um super admin rebaixe a si mesmo, e só vale a partir do próximo login do
alvo (o JWT já emitido carrega `is_super_admin` e não há revogação de token hoje).

## Auditoria

| Método | Rota | Acesso | Handler |
|---|---|---|---|
| POST | `/audit/trigger` | Autenticado (`tenant_id` no corpo, checado no handler) | `auditHandler.TriggerAudit` |

## Responsáveis (staff)

| Método | Rota | Acesso | Handler |
|---|---|---|---|
| PUT | `/staffs/:staffId` | RH+ | `staffHandler.Update` |
| DELETE | `/staffs/:staffId` | RH+ | `staffHandler.Delete` |

## Ocorrências e tratativa

| Método | Rota | Acesso | Handler |
|---|---|---|---|
| PATCH | `/occurrences/:occurrenceId/ignore` | Gestor+ | `occurrenceHandler.Ignore` |
| GET | `/occurrences/:occurrenceId/events` | Autenticado (tenant do handler) | `occurrenceHandler.Events` |
| POST | `/occurrences/:occurrenceId/treat` | Gestor+ | `treatmentHandler.Treat` |
| GET | `/occurrences/:occurrenceId/treatments` | Autenticado (tenant do handler) | `treatmentHandler.Treatments` |
| POST | `/treatments/:treatmentId/undo` | Gestor+ | `treatmentHandler.Undo` |
| GET | `/attachments/:attachmentId/download` | Autenticado (tenant do handler) | `treatmentHandler.DownloadAttachment` |

`POST /occurrences/:occurrenceId/treat` é `multipart/form-data`: campo `justification`
(obrigatório) e `attachment` (arquivo PDF, obrigatório para os tipos marcados na
taxonomia — ver `docs/documento-funcional-compliance.md` §4).

## Filiais

| Método | Rota | Acesso | Handler |
|---|---|---|---|
| GET | `/branches/:branchId` | Autenticado (tenant do handler) | `branchHandler.Get` |
| PUT | `/branches/:branchId` | RH+ | `branchHandler.Update` |
| DELETE | `/branches/:branchId` | RH+ | `branchHandler.Delete` |
| POST | `/branches/:branchId/devices` | RH+ | `branchHandler.AddDevice` |
| DELETE | `/branches/:branchId/devices/:deviceId` | RH+ | `branchHandler.RemoveDevice` |
| POST | `/branches/:branchId/payroll-numbers` | RH+ | `branchHandler.AddPayrollNumber` |
| DELETE | `/branches/:branchId/payroll-numbers/:payrollNumberId` | RH+ | `branchHandler.RemovePayrollNumber` |

## Advertências

| Método | Rota | Acesso | Handler |
|---|---|---|---|
| GET | `/warnings/:warningId` | Autenticado (tenant do handler) | `warningHandler.Get` |
| PUT | `/warnings/:warningId` | Gestor+ | `warningHandler.Update` |
| PATCH | `/warnings/:warningId/status` | Gestor+ | `warningHandler.UpdateStatus` |
| DELETE | `/warnings/:warningId` | Gestor+ | `warningHandler.Delete` |

## Recursos aninhados sob `/tenants/:id`

Todas as rotas abaixo passam por `RequireTenantAccess` no mínimo (piso Diretoria) — é este
middleware que garante o isolamento básico entre tenants hoje (por vínculo de usuário, não
por filial — ver `docs/documento-funcional-compliance.md` §6.2 sobre o que fica fora do
ciclo atual). Onde a coluna "Papel mínimo" diz mais que "Diretoria", a rota também passa
por `RequireTenantRole`.

| Método | Rota | Papel mínimo | Handler |
|---|---|---|---|
| GET | `/tenants/:id` | Diretoria | `tenantHandler.Get` |
| POST | `/tenants/:id/sync` | Gestor | `tenantHandler.Sync` |
| GET | `/tenants/:id/settings` | Diretoria | `settingsHandler.Get` |
| PUT | `/tenants/:id/settings` | RH | `settingsHandler.Update` |
| GET | `/tenants/:id/staffs` | Diretoria | `staffHandler.List` |
| POST | `/tenants/:id/staffs` | RH | `staffHandler.Create` |
| GET | `/tenants/:id/reports` | Diretoria | `reportHandler.List` |
| GET | `/tenants/:id/reports/history` | Diretoria | `reportHandler.History` |
| GET | `/tenants/:id/occurrences` | Diretoria | `occurrenceHandler.List` |
| GET | `/tenants/:id/occurrence-events` | Diretoria | `occurrenceHandler.TenantEvents` |
| GET | `/tenants/:id/monthly-reviews` | Diretoria | `monthlyReviewHandler.Get` |
| PATCH | `/tenants/:id/monthly-reviews` | RH | `monthlyReviewHandler.UpdateManualConditions` |
| POST | `/tenants/:id/monthly-reviews/close` | RH | `monthlyReviewHandler.Close` |
| POST | `/tenants/:id/monthly-reviews/reopen` | Diretoria* | `monthlyReviewHandler.Reopen` |
| GET | `/tenants/:id/monthly-reviews/export` | Diretoria | `monthlyReviewHandler.Export` |
| GET | `/tenants/:id/branches` | Diretoria | `branchHandler.List` |
| POST | `/tenants/:id/branches` | RH | `branchHandler.Create` |
| GET | `/tenants/:id/equipamentos` | Diretoria | `equipmentHandler.List` |
| GET | `/tenants/:id/warnings` | Diretoria | `warningHandler.List` |
| POST | `/tenants/:id/warnings` | Gestor | `warningHandler.Create` |
| GET | `/tenants/:id/collaborators` | Diretoria | `collaboratorHandler.List` |
| GET | `/tenants/:id/collaborators/history` | Diretoria | `collaboratorHandler.History` |
| GET | `/tenants/:id/collaborators/filters` | Diretoria | `collaboratorHandler.Filters` |
| GET | `/tenants/:id/collaborators/:secullumId/prefill` | Diretoria | `collaboratorHandler.Prefill` |
| GET | `/tenants/:id/collaborators/:secullumId/punch-records` | Diretoria | `collaboratorHandler.PunchRecords` |
| GET | `/tenants/:id/whatsapp/status` | Diretoria | `whatsappHandler.Status` |
| POST | `/tenants/:id/whatsapp/instance` | RH | `whatsappHandler.Connect` |
| DELETE | `/tenants/:id/whatsapp/instance` | RH | `whatsappHandler.Disconnect` |
| GET | `/tenants/:id/users` | **Super Admin** | `tenantHandler.ListUsers` |

`GET /tenants/:id/users` é a única rota deste grupo que exige Super Admin em vez de papel
— subiu de RH (proposta original do `docs/08`) por decisão registrada em
`docs/documento-funcional-compliance.md` §7.6.

\* `POST /tenants/:id/monthly-reviews/reopen` **não tem papel mínimo aplicado** — é ponto
em aberto #2 do documento funcional (qual perfil pode reabrir uma competência encerrada
ainda não foi decidido). Hoje, qualquer papel com vínculo no tenant pode reabrir.

Query strings notáveis:

- `GET /tenants/:id/collaborators?departamento_id=&funcao_id=&empresa_id=` — filtros
  opcionais e combináveis.
- `GET /tenants/:id/reports`, `/reports/history`, `/tenants/:id/occurrences` —
  `?start_date=&end_date=` (formato `YYYY-MM-DD`).
- `GET /tenants/:id/occurrences?severity=&type=&state=&collaborator_id=&branch_id=&departamento_id=&funcao_id=&empresa_id=&limit=&offset=`
  — todos opcionais e combináveis (`severity`/`type`/`state` aceitam lista separada por
  vírgula). `limit`/`offset` paginam no servidor; `total` reflete o filtro inteiro,
  incluindo `departamento_id`/`funcao_id`/`empresa_id` (resolvidos para `collaborator_id
  IN (...)` antes de paginar). **Exceção conhecida: `branch_id`.** Filial não é campo do
  colaborador — é resolvida por aparelho/nº de folha, dado que só existe depois de
  carregar a página. `branch_id` continua filtrando em memória **depois** da paginação:
  uma página pedida com `limit=20&branch_id=X` pode devolver menos de 20 itens mesmo
  havendo mais correspondências, e `total` não desconta esse filtro. Corrige quando a
  filial for persistida na ocorrência (`docs/12` §2.2, adiado junto do isolamento por
  filial — `docs/documento-funcional-compliance.md` §6.2).
- `GET /tenants/:id/occurrence-events?start_date=&end_date=&actor_user_id=&type=` — a
  consulta central do histórico de tratamento (Feature 1): todos os eventos do tenant no
  período, com nome do colaborador e tipo da ocorrência já embutidos.
- `GET|PATCH /tenants/:id/monthly-reviews?competencia=YYYY-MM`,
  `POST /tenants/:id/monthly-reviews/close?competencia=YYYY-MM`,
  `POST .../reopen?competencia=YYYY-MM` — `competencia` é obrigatória nas quatro. `Close`
  bloqueia (409) com o detalhe exato do que falta se qualquer uma das seis condições não
  estiver satisfeita. `Reopen` exige `{"reason": "..."}` no corpo. Tratar/ignorar uma
  ocorrência cuja competência já foi encerrada também devolve 409 — ver
  `docs/documento-funcional-compliance.md` §7.5.
- `GET /tenants/:id/monthly-reviews/export?competencia=YYYY-MM` — o relatório consolidado
  exportável (evidência do ciclo). Só existe para competência já **encerrada**; devolve
  409 se ainda estiver aberta. Devolve todas as ocorrências do período (qualquer estado,
  não só as pendentes) e contagens por tipo/severidade/desfecho.
- A competência da revisão mensal respeita `TenantSettings.revisao_mensal_dia_corte`
  (`GET/PUT /tenants/:id/settings`) — 0 (padrão) usa mês calendário; um dia 1-28 desloca a
  competência para o corte de folha configurado (ex.: corte 25 faz "2026-09" ir de
  26/08 a 25/09).
- `GET /tenants/:id/collaborators/:secullumId/punch-records` — `start_date`/`end_date`
  obrigatórios juntos.
