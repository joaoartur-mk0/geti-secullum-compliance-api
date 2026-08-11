# Visão Geral do Backend — Como Funciona

Este documento explica o que já foi desenvolvido no backend e como cada peça funciona.
É o mapa para quem chega no projeto (ou volta depois de um tempo). Para o contrato dos
endpoints, veja o **Swagger** (seção [Documentação da API](#8-documentação-da-api-swagger)).

> Complementa o `00_Automation_Engineering_Documentation.md` (especificação funcional)
> e o `01_Secullum_API_Info.md` (detalhes da API externa).

---

## 1. O que o sistema faz

Microsserviço que **audita a jornada de trabalho** dos colaboradores diariamente:

1. Busca as batidas de ponto na **API SecullumWEB**.
2. Aplica **regras de compliance trabalhista** (CLT) para achar inconsistências.
3. Gera **relatórios** de infrações e (futuramente) dispara **alertas via WhatsApp**.

É **multi-tenant**: cada cliente (tenant) tem suas configurações de regras e seus gestores.

---

## 2. Stack

- **Go** + **Gin** (HTTP) — API e workers.
- **PostgreSQL** (GORM) — persistência.
- **RabbitMQ** — processamento assíncrono (a auditoria não trava a API).
- **Docker Compose** — sobe tudo junto (`infrastructure/`).

---

## 3. Arquitetura (Clean Architecture)

O código é isolado em camadas; as de dentro não conhecem as de fora.

```
cmd/api/main.go            → composição: conecta DB, broker, injeta dependências, sobe tudo
internal/
├─ domain/                 → entidades puras + interfaces (contratos) + erros de negócio
├─ usecase/                → regras de negócio (o "cérebro": auditor)
├─ infrastructure/         → implementações técnicas
│  ├─ database/            → models GORM + repositórios
│  ├─ messaging/           → RabbitMQ (channel pool + consumer)
│  └─ secullum/            → client HTTP da API Secullum
└─ interface/http/         → camada web (handlers, router, erros HTTP, swagger)
```

**Regra de ouro:** o `domain` define *interfaces* (ex.: `TenantRepository`), e a
`infrastructure` as *implementa*. Assim o `usecase` chama o banco sem saber que é Postgres.

---

## 4. Domínio e erros

### Entidades (`internal/domain`)
- `Tenant` — o cliente. Tem `Active` (desativação), `Settings` e `Staffs`.
- `TenantSettings` — flags de regras (almoço, interjornada, hextras, esquecimento),
  **severidades configuráveis** por regra e `Horarios` de varredura.
- `Staff` — o gestor que recebe alertas.
- `Collaborator` / `CollaboratorSchedule` — funcionário auditado e sua jornada contratual.
- `DailyPunch` — as batidas de um dia (entrada1/saída1/entrada2/saída2).
- `AuditInconsistency` / `Report` — a infração encontrada e o consolidado salvo.

### Erros estruturados (`internal/domain/errors.go`)
Todo erro relevante é um `AppError` com:
- `Kind` — `VALIDATION` / `NOT_FOUND` / `CONFLICT` / `INTERNAL` (define o status HTTP).
- `Op` — a origem (ex.: `tenantRepository.Update`), formando um rastro para depuração.
- `Message` — mensagem segura para exibir na interface.
- `Details` / `Err` — informação extra e o erro subjacente (preservado para log).

O helper `interface/http/httperr.Respond` traduz isso em resposta JSON padronizada e
loga o contexto completo. **Nenhum erro é engolido em silêncio.**

---

## 5. O motor de auditoria (`usecase/auditor.go`)

Função central: `ProcessRules(settings, collab, todayPunch, yesterdayPunch, now, isClosing)`.

- Recebe as configurações do tenant e as batidas, e devolve `([]AuditInconsistency, error)`.
- Cada regra devolve, **separadamente**, uma possível infração e um possível erro de dado.
  Os erros são agregados com `errors.Join` e devolvidos — infrações válidas não se perdem
  por causa de um dado ruim.

### Regras implementadas
| Regra | Critério | Severidade |
|-------|----------|------------|
| Interjornada (Art. 66) | descanso entre jornadas < 11h | configurável (default CRÍTICO) |
| Almoço (Art. 71) | intervalo < 60min | configurável (default CRÍTICO) |
| Batidas esquecidas (5.3) | contagem ímpar **no fechamento**; ou 1 batida + 30min do almoço (intra-dia) | configurável |
| Hora extra (Art. 59) | > 1h → Alerta; > 2h → Crítico | **legal, não configurável** |

Pontos importantes de correção já aplicados:
- **Virada de meia-noite:** o helper `durationBetween` soma 24h quando o fim é menor que
  o início, então turnos noturnos não geram duração negativa.
- **Carga contratual:** a hora extra usa a jornada real do colaborador (com fallback de 8h).
- **`isClosing`:** a contagem ímpar só vale no fechamento noturno, evitando falso positivo
  durante o expediente.

Este arquivo é puro (sem I/O) e tem cobertura de testes em `auditor_test.go`.

---

## 6. Integração com a Secullum (`infrastructure/secullum`)

Credenciais **globais** (uma só para todos os tenants), vindas de env. O que identifica
o cliente é o `SecullumDatabaseID`, enviado como header por requisição.

- **`auth.go`** — `tokenManager`: autentica por credenciais, cacheia o token (~55min,
  margem sob o 1h da Secullum) e renova sozinho. Refresh serializado por mutex.
- **`ratelimit.go`** — token bucket (stdlib): respeita o limite de **100 req/min**.
- **`schedule.go`** — parseia a jornada, que a API devolve como texto
  (`"Horário 08:00/12:00/14:00/18:00"`), em `CollaboratorSchedule`.
- **`client.go`** — `GetDailyPunches` e `GetCollaborators`. Todo request passa por
  `do()`, que aplica **rate limit → token → headers**.

Testado em `auth_test.go`, `ratelimit_test.go`, `schedule_test.go`.

---

## 7. Mensageria e ciclo de vida da auditoria (`infrastructure/messaging`)

- **`channel_pool.go`** — pool de canais AMQP. Canais do RabbitMQ **não são seguros para
  uso concorrente**; cada publicação HTTP empresta um canal exclusivo do pool. Também é
  usado pelo worker de auditoria para publicar os alertas de notificação.
- **`consumer.go`** — worker que escuta a fila `audit.trigger`, busca as batidas, roda o
  motor de regras, salva o relatório, publica o resumo em `notifications.whatsapp` para
  cada staff do tenant e confirma (Ack) a mensagem. Erros transitórios (rede/DB) voltam
  para a fila (requeue); permanentes (payload inválido) são descartados.
- **`notification_consumer.go`** — worker que escuta a fila `notifications.whatsapp` e
  entrega cada mensagem via **Evolution API** (`infrastructure/evolution`). Erros
  transitórios (rede, instância desconectada) voltam para a fila; payload inválido é
  descartado. Roda totalmente desacoplado do motor de auditoria — só se conectam pela fila.

### Fluxo de uma auditoria
```
POST /api/v1/audit/trigger
  → publica {tenant_id} na fila audit.trigger  (retorna 202 na hora)
      → worker de auditoria consome
          → busca tenant + batidas (Secullum)
              → ProcessRules (motor)
                  → salva Report no Postgres
                      → publica 1 mensagem por staff em notifications.whatsapp
                          → worker de notificações consome
                              → Evolution API (POST /message/sendText/{instance})
                                  → WhatsApp do gestor
```

---

## 8. Documentação da API (Swagger)

Com a aplicação rodando:

- **Swagger UI:** http://localhost:8080/swagger
- **Spec OpenAPI:** http://localhost:8080/openapi.yaml

A spec (`internal/interface/http/swagger/openapi.yaml`) é embutida no binário via
`go:embed` — não precisa de arquivo em disco nem de geração externa.

### Endpoints
Todas as rotas de `/api/v1`, exceto `/auth/login`, exigem `Authorization: Bearer
<token>`. As marcadas como **super admin** também exigem `User.IsSuperAdmin = true`; as
de um tenant específico (`:id`) exigem vínculo do usuário com aquele tenant (ou super
admin) — ver [`05_Auth_Backend_Contract.md`](./05_Auth_Backend_Contract.md) para o
contrato completo de autenticação, papéis e isolamento entre tenants.

| Método | Rota | Auth extra | Descrição |
|--------|------|------------|-----------|
| GET | `/health` | pública | Saúde de DB + broker |
| POST | `/api/v1/auth/login` | pública | Login |
| POST | `/api/v1/auth/register` | super admin | Cadastra usuário |
| GET/DELETE | `/api/v1/users`, `/api/v1/users/:id` | super admin* | Gestão de usuários (*`GET /users/:id` também vale para o próprio usuário) |
| GET | `/api/v1/users/:id/tenants` | próprio usuário ou super admin | Tenants do usuário |
| POST | `/api/v1/audit/trigger` | vínculo com o `tenant_id` do corpo | Enfileira auditoria |
| GET | `/api/v1/tenants` | filtrado por vínculo | Lista tenants (`?include_inactive=true`) |
| POST | `/api/v1/tenants` | super admin | Cadastra tenant |
| GET | `/api/v1/tenants/:id` | vínculo com o tenant | Busca tenant |
| PUT | `/api/v1/tenants/:id` | super admin | Atualiza tenant |
| PATCH | `/api/v1/tenants/:id/deactivate` | super admin | Desativa tenant |
| POST/DELETE | `/api/v1/tenants/:id/users` | super admin | Vincula/desvincula usuário ao tenant |
| GET | `/api/v1/tenants/:id/users` | vínculo com o tenant | Lista usuários do tenant |
| GET | `/api/v1/tenants/:id/settings` | vínculo com o tenant | Busca configurações |
| PUT | `/api/v1/tenants/:id/settings` | vínculo com o tenant | Atualiza configurações |
| GET | `/api/v1/tenants/:id/staffs` | vínculo com o tenant | Lista responsáveis |
| POST | `/api/v1/tenants/:id/staffs` | vínculo com o tenant | Cadastra responsável |
| PUT | `/api/v1/staffs/:staffId` | vínculo com o tenant do staff | Atualiza responsável |
| DELETE | `/api/v1/staffs/:staffId` | vínculo com o tenant do staff | Exclui responsável |
| GET | `/api/v1/tenants/:id/reports` | vínculo com o tenant | Lista relatórios (painel) |

---

## 9. Configuração (variáveis de ambiente)

| Variável | Uso | Default |
|----------|-----|---------|
| `DATABASE_URL` | DSN do Postgres | dsn local |
| `RABBITMQ_URL` | URL do RabbitMQ | `amqp://guest:guest@localhost:5672/` |
| `PORT` | Porta HTTP | `8080` |
| `GIN_MODE` | `release` em produção | — |
| `SECULLUM_API_URL` | Base da API Secullum | `https://api.secullum.com.br` |
| `SECULLUM_AUTH_URL` | Endpoint de autenticação | — |
| `SECULLUM_USERNAME` / `SECULLUM_PASSWORD` | Credenciais globais | — |
| `SECULLUM_API_TOKEN` | Token estático (opcional, testes) | — |
| `EVOLUTION_API_URL` | Base da Evolution API | — |
| `EVOLUTION_API_KEY` | Chave de API (header `apikey`) | — |
| `EVOLUTION_INSTANCE` | Nome da instância conectada ao WhatsApp | — |

---

## 10. Como rodar e testar

```bash
# Subir tudo (Postgres + RabbitMQ + app)
cd infrastructure
docker compose up -d

# Saúde
curl localhost:8080/health

# Login (super admin criado via seed — ver seção 05_Auth_Backend_Contract.md)
TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@empresa.com","password":"minhasenha123"}' | jq -r .token)

# Cadastrar um tenant (exige super admin)
curl -X POST localhost:8080/api/v1/tenants \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Empresa Teste","secullum_database_id":123,"staff_name":"Fulano","staff_contact":"5531999999999"}'

# Testes unitários (não precisam de infra)
cd backend && go test ./...
```

> Ao mudar models, recrie o volume do Postgres (`docker compose down -v`) — o `AutoMigrate`
> adiciona colunas novas, mas não altera defaults nem preenche linhas antigas.

---

## 11. O que ainda falta (roadmap)

- **Agendador (cron)** para as varreduras intra-dia e o fechamento noturno (hoje a
  auditoria só é disparada manualmente via `POST /api/v1/audit/trigger`).
- **Alertas preventivos intra-dia**: a fila `notifications.whatsapp` e o worker que a
  consome já existem e são usados no fechamento noturno; falta o gatilho intra-dia
  (seção 5.3) que dispara uma auditoria parcial sem gravar `Report`.
- **Criptografia** de credenciais sensíveis em repouso.

> Autenticação (JWT), papel de super admin e isolamento de dados por tenant já estão
> implementados — ver [`05_Auth_Backend_Contract.md`](./05_Auth_Backend_Contract.md).
