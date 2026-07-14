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
  uso concorrente**; cada publicação HTTP empresta um canal exclusivo do pool.
- **`consumer.go`** — worker que escuta a fila `audit.trigger`, busca as batidas, roda o
  motor de regras, salva o relatório e confirma (Ack) a mensagem. Erros transitórios
  (rede/DB) voltam para a fila (requeue); permanentes (payload inválido) são descartados.

### Fluxo de uma auditoria
```
POST /api/v1/audit/trigger
  → publica {tenant_id} na fila audit.trigger  (retorna 202 na hora)
      → worker consome
          → busca tenant + batidas (Secullum)
              → ProcessRules (motor)
                  → salva Report no Postgres
```

---

## 8. Documentação da API (Swagger)

Com a aplicação rodando:

- **Swagger UI:** http://localhost:8080/swagger
- **Spec OpenAPI:** http://localhost:8080/openapi.yaml

A spec (`internal/interface/http/swagger/openapi.yaml`) é embutida no binário via
`go:embed` — não precisa de arquivo em disco nem de geração externa.

### Endpoints
| Método | Rota | Descrição |
|--------|------|-----------|
| GET | `/health` | Saúde de DB + broker |
| POST | `/api/v1/audit/trigger` | Enfileira auditoria |
| GET | `/api/v1/tenants` | Lista tenants (`?include_inactive=true`) |
| POST | `/api/v1/tenants` | Cadastra tenant |
| GET | `/api/v1/tenants/:id` | Busca tenant |
| PUT | `/api/v1/tenants/:id` | Atualiza tenant |
| PATCH | `/api/v1/tenants/:id/deactivate` | Desativa tenant |
| GET | `/api/v1/tenants/:id/settings` | Busca configurações |
| PUT | `/api/v1/tenants/:id/settings` | Atualiza configurações |
| GET | `/api/v1/tenants/:id/staffs` | Lista responsáveis |
| POST | `/api/v1/tenants/:id/staffs` | Cadastra responsável |
| PUT | `/api/v1/staffs/:staffId` | Atualiza responsável |
| DELETE | `/api/v1/staffs/:staffId` | Exclui responsável |
| GET | `/api/v1/tenants/:id/reports` | Lista relatórios (painel) |

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

---

## 10. Como rodar e testar

```bash
# Subir tudo (Postgres + RabbitMQ + app)
cd infrastructure
docker compose up -d

# Saúde
curl localhost:8080/health

# Cadastrar um tenant
curl -X POST localhost:8080/api/v1/tenants \
  -H 'Content-Type: application/json' \
  -d '{"name":"Empresa Teste","secullum_database_id":123,"staff_name":"Fulano","staff_contact":"5531999999999"}'

# Testes unitários (não precisam de infra)
cd backend && go test ./...
```

> Ao mudar models, recrie o volume do Postgres (`docker compose down -v`) — o `AutoMigrate`
> adiciona colunas novas, mas não altera defaults nem preenche linhas antigas.

---

## 11. O que ainda falta (roadmap)

- **Sincronização de colaboradores** (fila `tenant.provisioning` + job diário): hoje o
  worker usa um colaborador mockado; falta persistir os funcionários reais.
- **Agendador (cron)** para as varreduras intra-dia e o fechamento noturno.
- **Notificações WhatsApp** (fila `notifications.whatsapp` → Evolution API).
- **Autenticação/middleware (JWT)** para proteger os endpoints administrativos.
- **Criptografia** de credenciais sensíveis em repouso.
