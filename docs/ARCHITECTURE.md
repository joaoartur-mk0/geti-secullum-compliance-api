# Arquitetura

Documento de referência de como o sistema é construído — camadas, fluxo de dados,
integrações externas e por que as coisas são como são. Para o contrato de cada rota, ver
[`ROUTES.md`](./ROUTES.md); para os serviços de domínio, [`SERVICES.md`](./SERVICES.md);
para os handlers HTTP, [`CONTROLLERS.md`](./CONTROLLERS.md).

## Visão geral

```
┌─────────────┐      HTTP       ┌──────────────────────────────────────────────┐
│  Frontend    │ ───────────────▶│  API Go (Gin)                                 │
│  React + TS  │◀─────────────── │  interface/http → usecase → domain            │
└─────────────┘                 │            ▲                    │              │
                                 │            │                    ▼              │
                                 │      infrastructure/database (GORM/Postgres)   │
                                 └───────┬────────────────────────┬───────────────┘
                                         │ AMQP                   │ HTTP
                                         ▼                        ▼
                                  ┌─────────────┐         ┌───────────────┐
                                  │  RabbitMQ    │         │  Secullum API  │
                                  │  (filas)     │         │  Evolution API │
                                  └──────┬──────┘         └───────────────┘
                                         │
                        ┌────────────────┼────────────────┐
                        ▼                ▼                ▼
                AuditConsumer   ProvisioningConsumer  NotificationConsumer
                (audit.trigger)  (tenant.provisioning)  (notifications.whatsapp)
```

O processo Go único (`cmd/api/main.go`) sobe o servidor HTTP **e** três workers em
background (goroutines) consumindo filas do RabbitMQ, mais um agendador que publica nessas
filas sozinho, no horário configurado por tenant.

## Camadas (backend)

O backend segue arquitetura em camadas, de dentro para fora:

```
backend/internal/
├── domain/            regras e tipos de negócio — não importa nada das camadas de fora
├── usecase/            orquestração de casos de uso — importa domain, não importa infra/http
├── infrastructure/     implementações concretas (GORM, cliente Secullum, RabbitMQ, Evolution)
│   ├── database/           models GORM + repositórios (implementam as interfaces de domain)
│   ├── secullum/           cliente HTTP da API do Secullum
│   ├── evolution/          cliente HTTP da Evolution API (WhatsApp)
│   └── messaging/           consumers RabbitMQ + pool de canais de publicação
└── interface/http/     camada HTTP — router, handlers, middleware
    ├── handlers/            um arquivo por recurso, traduz HTTP ↔ domain
    └── middleware/          auth (JWT), CORS, controle de acesso por tenant
```

**Regra de dependência**: `domain` não importa nada de fora. `usecase` importa `domain`,
nunca `infrastructure` nem `interface/http`. `infrastructure` implementa as interfaces que
`domain` declara (ex.: `domain.CollaboratorRepository` é satisfeita por
`infrastructure/database/repositories/collaborator_repository.go`). O `main.go` é o único
lugar onde tudo é instanciado e conectado — é ali que a inversão de dependência acontece
de fato (injeção manual de construtor, sem framework de DI).

### `domain/`

Entidades e regras de negócio puras: `Occurrence` (ocorrência, com sua máquina de estados
`aberta → atualizada/resolvida_automatica/resolvida_manual/tratada`), `Collaborator`,
`Branch`, `Treatment`/`Attachment`, `Tenant`, `User`/`UserTenant`, `Warning`, `Equipment`,
`Audit`, `Notification`. Cada arquivo declara também o `Repository` (interface) que a
infraestrutura precisa implementar — o domínio define o contrato, não a implementação.

### `usecase/`

Serviços que orquestram regra de negócio que não cabe numa única entidade:

- `AuditorService` — o motor de regras (audita um dia contra a CLT).
- `ReconcilerService` — decide, comparando a apuração nova com o estado anterior, se uma
  ocorrência é nova/atualizada/resolvida — a máquina de estados em ação.
- `SynchronizerService` — busca o espelho de colaboradores/jornadas na Secullum.
- `BranchResolverService` — resolve a qual filial uma batida pertence (aparelho ou nº de
  folha).
- `SchedulerService` — dispara a auditoria automática (diária com notificação + horária
  silenciosa) sozinho, publicando na mesma fila que o botão manual do painel usa.
- `TreatmentService` — valida e aplica a tratativa de uma ocorrência (Feature 4).

Detalhe de cada um em [`SERVICES.md`](./SERVICES.md).

### `infrastructure/`

- **`database/`** — models GORM (`models.*`, com tags `gorm:"..."`) e repositórios
  (`repositories.*`) que convertem entre o model de persistência e o tipo de `domain`.
  Migração de schema é `db.AutoMigrate` (sem ferramenta de migration separada) — roda toda
  vez que o processo sobe, em `main.go`.
- **`secullum/`** — cliente HTTP da API SecullumWEB. Cuida de autenticação (login
  username/senha OU token estático), renovação automática de token e rate limit (100
  req/min). Ver [`01_Secullum_API_Info.md`](./01_Secullum_API_Info.md).
- **`evolution/`** — cliente HTTP da Evolution API (gateway de WhatsApp). Usado tanto pelo
  worker de notificações quanto pelos endpoints `/whatsapp/*` (gerência de instância).
- **`messaging/`** — três consumers RabbitMQ e um pool de canais de publicação
  (`ChannelPool`, porque canais AMQP não são seguros para uso concorrente entre
  goroutines/requisições).

### `interface/http/`

- **`router.go`** — todo o wiring de rotas (ver [`ROUTES.md`](./ROUTES.md) para a lista
  completa) e a injeção de dependências dos handlers.
- **`handlers/`** — um arquivo por recurso (ver [`CONTROLLERS.md`](./CONTROLLERS.md)).
  Convenção: `const op = "Handler.Metodo"` no início de cada método, usado nos erros
  estruturados; erros sempre envelopados via `domain.New{Validation,NotFound,Conflict,
  Forbidden,Internal}` e devolvidos por `httperr.Respond(c, err)`.
- **`middleware/`** — `auth.go` (valida JWT, injeta `user_id`/`is_super_admin` no
  contexto), `tenant_access.go` (`RequireTenantAccess`/`RequireSuperAdmin`, e o padrão
  `ensureTenantAccess` usado quando o tenant só é conhecido depois de carregar o recurso
  pelo id da rota), `cors.go`.

## Fluxo assíncrono (RabbitMQ)

Quatro filas, três consumers:

| Fila | Consumer | Publicada por |
|---|---|---|
| `audit.trigger` | `AuditConsumer` | `POST /audit/trigger` (handler) **e** `SchedulerService` |
| `notifications.whatsapp` | `NotificationConsumer` | `AuditConsumer`, ao final de uma auditoria com `notify: true` |
| `tenant.provisioning` | `ProvisioningConsumer` | fluxo de criação/ativação de tenant |
| `audit.process` | *(declarada, reservada)* | — |

O **`AuditConsumer`** é o coração do sistema: recebe um pedido de auditoria (um tenant, um
dia ou período, com a flag `notify`), busca os dados necessários na Secullum (batidas,
funcionários), roda o `AuditorService` contra cada dia, passa o resultado pelo
`ReconcilerService` (que decide o que é ocorrência nova/atualizada/resolvida), persiste via
`OccurrenceRepository.ApplyChanges` numa única transação, grava o `Report` da execução e —
se `notify: true` — publica um resumo na fila `notifications.whatsapp`.

O **`SchedulerService`** roda em goroutine própria desde a subida do processo e dispara
sozinho dois tipos de auditoria por tenant ativo: a diária de fechamento (no horário
configurado em `TenantSettings`, com notificação) e uma atualização horária silenciosa
(sem notificação, cobrindo o mês corrente inteiro, para capturar correções feitas na
Secullum a qualquer momento). Ambas publicam o mesmo payload que o botão manual do painel.

## Autenticação e acesso

JWT (`golang-jwt/jwt/v5`), token válido por 24h, carregando `user_id`, `email` e
`is_super_admin`. Multi-tenant: usuários se vinculam a tenants via `UserTenant` (N:N).
Papel de acesso (RH/Gestor/Diretoria) é definido por tenant — ver
[`08_Roles_And_Permissions_Contract.md`](./08_Roles_And_Permissions_Contract.md). Duas
formas de checar acesso, dependendo de onde o tenant aparece na rota:

- **Tenant no path** (`/tenants/:id/...`) — middleware `RequireTenantAccess` no grupo de
  rotas (`router.go`).
- **Tenant só conhecido depois de carregar o recurso** (`/occurrences/:id/...`,
  `/treatments/:id/...`, `/staffs/:id`) — o handler carrega o recurso primeiro e chama
  `ensureTenantAccess(c, userTenantRepo, op, recurso.TenantID)`.

## Persistência

PostgreSQL via GORM, sem ferramenta de migration separada — `AutoMigrate` roda a cada
subida do processo (ver lista de models em `cmd/api/main.go`). Trocas de estado que
precisam ser atômicas (ex.: `Treat`, `Undo`, `Ignore`, `ApplyChanges`) usam
`db.Transaction(func(tx *gorm.DB) error {...})`.

## Integrações externas

| Integração | Direção | Propósito |
|---|---|---|
| API Secullum Ponto Web | saída (o backend consome) | fonte de verdade das marcações de ponto, funcionários, horários, equipamentos, filiais/estruturas |
| Evolution API | saída (o backend consome) | envio de alertas via WhatsApp e gerência de instância por tenant |

Credenciais da Secullum e da Evolution API são **globais** (uma só para todos os tenants),
configuradas via variáveis de ambiente — ver `infrastructure/.env.example`.

## Frontend

React + Vite + TypeScript, servido separadamente (container próprio em produção). Consome
a API via `VITE_API_URL` (injetada no build). Sem dependências de gráfico — visualizações
são CSS/Tailwind puro (ver `docs/11_Historico_Ranking_Frontend_Contract.md` §8).

## Deploy

`infrastructure/docker-compose.yml` (base), `docker-compose.local.yml` (dev local, stack
isolada) e `docker-compose.prod.yml` (produção, com labels Traefik para TLS/roteamento).
Três serviços: `app` (backend Go), `db` (Postgres), `rabbitmq`, mais `web` (frontend) nos
composes que o incluem.
