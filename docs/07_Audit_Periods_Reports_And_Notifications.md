# Auditoria de período, histórico de relatórios e notificações — o que mudou

Este documento registra, num só lugar, tudo que foi implementado nas rodadas descritas em
`roadmap-compliance-update.md`: o fix de domingo, a auditoria de período completo, a
divisão de `GET /reports`, a rotina horária silenciosa, a regra de quando o WhatsApp é
notificado, e as telas novas do frontend que passaram a consumir tudo isso. A referência
formal das rotas continua em `backend/internal/interface/http/swagger/openapi.yaml`
(Swagger UI em `/swagger`); aqui fica o **porquê** de cada mudança e como as peças se
encaixam.

---

## 1. Fix de domingo no intervalo intrajornada

**Onde:** `backend/internal/usecase/auditor.go` (`checkLunchBreak`).

Antes, o intervalo mínimo exigido (Art. 71 CLT) era **sempre** derivado da carga prevista
do dia: >6h exige 60min, >4h e ≤6h exige 15min. Isso presumia que a jornada de domingo é
sempre ~6h — na prática nem sempre é (trabalho extraordinário de duração variável), e um
domingo com mais carga acabava exigindo 60min por engano.

Uma primeira correção trocou isso por um piso fixo de 15min aos domingos — mas testes
reais mostraram que ainda gerava CRÍTICO para pausas curtas (ex.: 14min), o que não é o
comportamento desejado: **domingo não deve ter regra graduada nenhuma**, nem os 60min, nem
um piso de 15min. A regra final é mais simples: só importa se o colaborador fez **alguma**
pausa.

```go
intervalo, ok, err := punch.FirstBreak()
// ...
if punch.Date.Weekday() == time.Sunday {
    if ok && intervalo > 0 {
        return nil, nil // fez alguma pausa: conta como almoço feito, mesmo que curta
    }
    return &domain.AuditInconsistency{
        Type:     TipoAlmocoReduzido,
        Severity: domain.SeverityAlert, // nunca CRÍTICO no domingo
        // ...
    }, nil
}
```

- Domingo com **qualquer** pausa registrada (mesmo 1min) → **não infringe**. Um intervalo
  de 14min, que antes disparava CRÍTICO, agora não gera ocorrência nenhuma.
- Domingo **sem nenhuma** pausa (turno corrido, `FirstBreak` não encontra gap) → infringe,
  mas sempre com severidade **ALERTA** — nunca CRÍTICO, mesmo que o tenant tenha
  `almoco_severity: CRITICO` configurado para o resto da semana. A severidade não é
  configurável para este caso específico.

Testes: `TestProcessRules_DomingoComPausaCurtaNaoInfringe`,
`TestProcessRules_DomingoSemPausaInfringeComoAlerta`.

---

## 2. Auditoria de período completo (semana, mês, intervalo customizado)

**Onde:** `POST /api/v1/audit/trigger`, `AuditHandler` (`audit_handler.go`) e
`AuditConsumer` (`consumer.go`).

Antes só dava para auditar D-1 (automático) ou um dia específico por vez. Agora o mesmo
endpoint aceita um **período**:

```jsonc
// Dia único (como antes)
{ "tenant_id": 1, "date": "2026-07-12" }

// Período completo — NOVO
{ "tenant_id": 1, "start_date": "2026-07-01", "end_date": "2026-07-07" }
```

`date` e `start_date`/`end_date` são **mutuamente exclusivos**. Regras de validação
(`TriggerRequest.resolvePeriod`):

- `start_date` e `end_date` são obrigatórios juntos.
- `end_date` não pode ser anterior a `start_date`.
- o período inteiro precisa estar encerrado (mesma regra do dia único: nada de hoje/futuro).
- no máximo **62 dias** (`maxRangeDays`) — cobre folgadamente uma semana ou mês, sem
  permitir um pedido acidental de meses seguidos.

A resposta (202) reflete a forma da requisição: dia único devolve `date`; período devolve
`start_date`/`end_date`.

### Como o worker processa (sem estourar o rate limit da Secullum)

`AuditConsumer.processMessage` resolve a lista de dias a auditar (`resolveTargetDays`) e
busca as batidas de **todo o período numa única chamada** à Secullum —
`SecullumService.GetDailyPunchesRange(tenant, start, end)`, nova no contrato — em vez de
uma requisição por dia. O `GetDailyPunches(tenant, date)` de sempre agora é só um atalho
para `GetDailyPunchesRange(tenant, date, date)`.

O intervalo buscado inclui o dia **anterior** ao primeiro dia do período, necessário só
para a interjornada do primeiro dia. As batidas voltam indexadas por dia
(`indexPunchesByDay`), e cada dia do período é auditado e persistido separadamente
(`auditDay`) — **um `Report` por dia**, exatamente como se cada dia tivesse sido auditado
individualmente. É isso que permite consultar/filtrar dias específicos depois (seção 3),
mesmo quando a auditoria foi disparada para o período inteiro de uma vez.

---

## 3. `GET /reports` dividido em "atual" e "histórico", com filtro de período

**Onde:** `ReportHandler` (`report_handler.go`), `ReportRepository`
(`report_repository.go`), rotas em `router.go`.

```
GET /api/v1/tenants/:id/reports              # só a MAIS RECENTE de cada dia
GET /api/v1/tenants/:id/reports/history      # TODAS as execuções (inclusive reauditorias)

# ambos aceitam, opcionalmente:
    ?start_date=2026-07-01&end_date=2026-07-31
```

- `GET /reports` → `ReportRepository.ListLatestByTenant` — dedupe em memória (a query já
  vem ordenada `date DESC, id DESC`; a primeira ocorrência de cada dia já é a mais
  recente). É o que o painel usa por padrão.
- `GET /reports/history` (**novo**) → `ReportRepository.ListByTenant` — sem dedupe, para
  quem precisa ver reauditorias do mesmo dia (auditoria/compliance).
- `start_date`/`end_date` (opcionais, sem limite de tamanho) filtram por `Report.Date` nos
  dois endpoints — é o que viabiliza consultar/filtrar por semana ou mês completo em vez
  de só um dia.

> **Antes**, `GET /reports` devolvia o histórico completo sem dedupe (o frontend
> deduplicava no cliente com `dedupeByDay`). Isso não existe mais — o dedupe agora é
> responsabilidade do backend, e o `dedupeByDay` do frontend foi removido por ficar
> redundante (ver seção 5).

---

## 4. Quando o WhatsApp é notificado (fix + rotina horária)

**Onde:** `SchedulerService` (`scheduler.go`), `AuditHandler`, `AuditConsumer`.

### O bug

Toda auditoria — automática **ou manual**, D-1 **ou** um dia qualquer no passado —
disparava o resumo no WhatsApp dos gestores. Auditar um mês inteiro sob demanda mandaria
30 mensagens; conferir um dia antigo por curiosidade também notificava, treinando o gestor
a ignorar o alerta.

### A regra agora

O evento `audit.trigger` ganhou o campo **`notify` (bool)**:

| Origem | `notify` |
|---|---|
| `SchedulerService.trigger` — varredura diária automática, no horário configurado (aba Avisos) | **`true`** — a única que notifica |
| `AuditHandler.TriggerAudit` — qualquer disparo manual (dia único ou período) | `false` |
| `SchedulerService.hourlyTick` — atualização horária silenciosa (ver abaixo) | `false` |

`AuditConsumer` só enfileira o resumo em `notifications.whatsapp` quando `payload.Notify`
é `true`:

```go
if payload.Notify {
    c.notifyStaffs(tenant, diaAlvo, resumo)
}
```

O campo `notify` **não é exposto no corpo de `POST /audit/trigger`** — é decidido
internamente pelo publicador do evento (handler HTTP vs. agendador), não pelo cliente da
API. Um cliente HTTP não tem como pedir notificação.

### Rotina horária de atualização silenciosa

`SchedulerService.Start` agora roda **dois** tickers: o de sempre (30s, checa o horário
diário configurado por tenant) e um novo de **1 hora**
(`hourlyTick`). A cada hora cheia, para **todo tenant ativo**, publica uma auditoria
silenciosa (`notify: false`) do fechamento de D-1 — sem `date`/`start_date`/`end_date`,
igual à varredura automática de sempre.

Isso mantém relatórios e ocorrências atualizados com correções feitas na Secullum depois
do fechamento original (ex.: RH ajustou uma batida às 10h da manhã), sem esperar até o
próximo dia — e sem repetir o alerta no WhatsApp a cada hora.

Diferença importante para `tick` (diário): `hourlyTick` **não usa `claimForToday`** — pode
disparar várias vezes no mesmo dia, de propósito (é o comportamento esperado; só o disparo
diário tem o limite de "uma vez por dia").

Testes: `TestScheduler_TickPublicaNotifyTrue`,
`TestScheduler_HourlyTickDisparaParaTodosOsAtivos`,
`TestScheduler_HourlyTickPublicaNotifyFalse`,
`TestScheduler_HourlyTickNaoTemLimiteDiario`.

---

## 5. Frontend: o que passou a consumir tudo isso

### 5.1 `/reports/history` (`frontend/src/pages/ReportsHistory.tsx`)

Página nova ("Registro de execuções" no menu), substitui a antiga `/configuracoes/logs`. Consome `GET /reports/history` de
verdade (antes era interino, apontando para o `GET /reports` antigo). Tabela com sort por
data, e filtro de período **De/Até** (não mais um único dia) — usa `start_date`/`end_date`
de verdade em vez de filtrar no cliente. Coluna "Disparada por" fica com `—`: o backend
ainda não expõe quem disparou cada execução.

### 5.2 `/auditorias` (Situação por dia)

- Ganhou um segundo picker, **"Auditar um período"** (De/Até, até 62 dias), ao lado do
  já existente "Auditar um dia" — chama `api.triggerAuditRange`.
- O link "Ver histórico completo" aponta para `/reports/history`.
- Parou de deduplicar no cliente (`dedupeByDay` removido — `GET /reports` já vem
  deduplicado do backend).

### 5.3 `/incidents` (`frontend/src/pages/Incidentes.tsx`) e atalhos em Indicadores

Página nova, destino dos cards "Críticas"/"Alertas"/"Operacionais" em Indicadores
(`IncidentShortcuts`) — `?severity=CRITICO|ALERTA|OPERACIONAL` na querystring (valores do
enum do backend, não os rótulos acentuados). Tabela (Colaborador, Data, Tipo, Severidade,
Status) com filtro de período, severidade (filtrada no cliente — `GET /occurrences` ainda
não filtra por severidade) e filial, e paginação client-side (20/página).

### 5.4 Indicadores consulta período, não só um dia

`Indicadores.tsx` ganhou um filtro de período (`PeriodFilterBar`): presets **7 dias / 30
dias / Este mês / Tudo**, mais campos De/Até personalizados. Controla o que é pedido a
`GET /reports` (`?start_date&end_date`) — não é mais um recorte sobre uma lista carregada
por inteiro. Padrão: últimos 30 dias (antes carregava o histórico inteiro sem filtro).

O seletor de **dia único** continua existindo, para inspecionar o detalhe de um dia
específico dentro do período carregado. O gráfico de evolução (**Evolução por
varredura**) agora mostra o período inteiro filtrado, em vez de ficar travado nas últimas
12 varreduras (`TREND_LIMIT` virou `MAX_CHART_BARS`, um teto de segurança só para quando
nenhum filtro está ativo — preset "Tudo").

---

## 6. Referência rápida — o que mudou em cada arquivo

| Arquivo | Mudança |
|---|---|
| `backend/internal/usecase/auditor.go` | Domingo só infringe intervalo sem NENHUMA pausa, sempre como ALERTA, em `checkLunchBreak` |
| `backend/internal/interface/http/handlers/audit_handler.go` | `TriggerRequest` aceita `start_date`/`end_date`; `resolvePeriod`; `notify:false` em todo evento manual |
| `backend/internal/infrastructure/messaging/consumer.go` | `resolveTargetDays`, `indexPunchesByDay`, `auditDay` (1 relatório por dia do período); `notifyStaffs` condicionado a `payload.Notify` |
| `backend/internal/infrastructure/secullum/client.go` | `GetDailyPunchesRange` (nova); `GetDailyPunches` delega para ela |
| `backend/internal/domain/Audit.go` | `SecullumService.GetDailyPunchesRange`; `ReportRepository.ListByTenant`/`ListLatestByTenant` com filtro de período |
| `backend/internal/infrastructure/database/repositories/report_repository.go` | `ListLatestByTenant` (dedupe em memória) além de `ListByTenant` (histórico), ambos com `start`/`end` opcionais |
| `backend/internal/interface/http/handlers/report_handler.go` | `List` (latest) e `History` (novo handler), `reportDateRange` compartilhado |
| `backend/internal/interface/http/router.go` | Rota nova `GET /tenants/:id/reports/history` |
| `backend/internal/usecase/scheduler.go` | Segundo ticker (`hourlyTick`, 1h); `notify:true` só no disparo diário; `publish` compartilhado |
| `backend/internal/interface/http/swagger/openapi.yaml` | `TriggerRequest` (`start_date`/`end_date`), `/audit/trigger` (nota sobre `notify`), `/reports` e `/reports/history` |
| `frontend/src/lib/api.ts` | `triggerAuditRange`; `listReports`/`listReportHistory` com filtro `start_date`/`end_date` |
| `frontend/src/pages/ReportsHistory.tsx` | Nova página (substitui `LogsHistorico.tsx`), consome `/reports/history`, filtro De/Até |
| `frontend/src/pages/Incidentes.tsx` | Nova página `/incidents`: filtros + tabela + paginação |
| `frontend/src/pages/Auditorias.tsx` | Picker de período; sem dedupe client-side |
| `frontend/src/pages/Indicadores.tsx` | `PeriodFilterBar`, `IncidentShortcuts`, gráfico sem teto fixo de 12 |
| `frontend/src/lib/reports.ts` | Removido (`dedupeByDay` ficou sem uso — dedupe é do backend) |
