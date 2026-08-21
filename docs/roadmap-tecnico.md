# Roadmap Update — Secullum Compliance

**Objetivo:**
Implementar features, refactores and fixes, respeitando a arquitetura atual do código, mantendo as boas práticas de cleancode e segurança. Separando a execução do frontend da execução do backend.

**Backend:**
- [x] Fix domingo: `checkLunchBreak` (`backend/internal/usecase/auditor.go`) agora força o piso de 15min sempre que `punch.Date` cai num domingo, ignorando a regra graduada (60min acima de 6h). Testes: `TestProcessRules_DomingoIntervaloMinimoFlat15min` e `TestProcessRules_DomingoIntervaloAbaixoDe15MinInfringe`.
- [x] Feature: `POST /api/v1/audit/trigger` aceita `start_date`/`end_date` (mutuamente exclusivo com `date`), até 62 dias, validado em `TriggerRequest.resolvePeriod` (`audit_handler.go`). O `AuditConsumer` (`consumer.go`) busca o período inteiro numa ÚNICA chamada à Secullum (`SecullumService.GetDailyPunchesRange`, nova) e salva um `Report` por dia (`auditDay`), preservando a consulta por dia específico.
- [x] Feature: `GET /reports` e `GET /reports/history` aceitam `?start_date=&end_date=` para consultar/filtrar por período completo (semana, mês, intervalo customizado).
- [x] Refactor: `GET /reports` agora devolve só a auditoria mais recente de cada dia (`ReportRepository.ListLatestByTenant`); `GET /reports/history` (novo) devolve o histórico completo, inclusive reauditorias do mesmo dia (`ReportRepository.ListByTenant`). Rota registrada em `router.go`; spec atualizada em `openapi.yaml`.
- [x] Feature: `SchedulerService` (`backend/internal/usecase/scheduler.go`) agora roda um segundo ticker, `hourlyTick` (1h), que reaudita silenciosamente o fechamento de D-1 de cada tenant ativo — mantém relatórios/ocorrências atualizados com correções feitas na Secullum depois do fechamento original, sem depender do próximo disparo diário. Não usa `claimForToday` (pode disparar várias vezes no mesmo dia, ao contrário do disparo diário). Testes: `TestScheduler_HourlyTickDisparaParaTodosOsAtivos`, `TestScheduler_HourlyTickPublicaNotifyFalse`, `TestScheduler_HourlyTickNaoTemLimiteDiario`.
- [x] Fix: o evento `audit.trigger` ganhou o campo `notify` (bool). Só a varredura diária automática do horário configurado (`SchedulerService.trigger`) publica `notify:true`; toda auditoria manual (`AuditHandler.TriggerAudit`, dia único ou período) e a atualização horária (`hourlyTick`) publicam `notify:false`. O `AuditConsumer` (`consumer.go`) só enfileira o resumo em `notifications.whatsapp` quando `payload.Notify` é true. Teste: `TestScheduler_TickPublicaNotifyTrue`. Spec (`openapi.yaml`) documentada.

**Frontend:**
- [x] Criar página de auditoria (rota `/reports/history`)
  - Implementado em `frontend/src/pages/ReportsHistory.tsx`, consumindo `GET /tenants/:id/reports` (lista completa já existente) — troca fácil para `GET /reports/history` quando o endpoint dedicado sair. Coluna "Disparada por" fica com `—` até o backend expor esse dado.
  - Exibe data, hora e qtd de ocorrências detectadas
  - Tabela com sort por data (clique no cabeçalho da coluna)

- [x] Ajustar página de varreduras (`/auditorias`)
  - Já mostrava apenas a varredura mais recente por dia
  - Link "Ver histórico completo" agora redireciona pra `/reports/history`

- [x] Cards de indicadores como atalhos
  - Implementado em `frontend/src/pages/Indicadores.tsx` (`IncidentShortcuts`) → `frontend/src/pages/Incidentes.tsx` (rota `/incidents`)
  - Severidade na querystring usa os valores do enum do backend (`CRITICO`/`ALERTA`/`OPERACIONAL`), não os rótulos acentuados do texto original

- [x] Listagens de ocorrências (melhoria visual)
  - Nova página `/incidents`, com filtros de período (`start_date`/`end_date`, já suportados por `GET /occurrences`), severidade (filtrada no cliente — a API ainda não filtra por severidade) e filial
  - Tabela com colunas: Colaborador, Data, Tipo, Severidade, Status
  - Paginação client-side (20 por página)

- [x] Fix: `frontend/src/pages/Indicadores.tsx` ganhou um filtro de período (`PeriodFilterBar`) com presets "7 dias"/"30 dias"/"Este mês"/"Tudo" e campos De/Até personalizados, que controla `GET /reports?start_date&end_date` (padrão: últimos 30 dias, em vez do histórico inteiro). O seletor de dia único continua para inspecionar um dia específico dentro do período carregado; o gráfico de evolução mostra o período inteiro (antes limitado às últimas 12 varreduras fixas).
