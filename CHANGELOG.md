# Changelog

Todas as mudanças notáveis deste projeto são documentadas neste arquivo.

O formato segue [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/), e o projeto
adota [Versionamento Semântico](https://semver.org/lang/pt-BR/).

## [Unreleased]

### Added
- Sincronização de departamento, função e empresa do colaborador a partir do cadastro da
  Secullum (campos sempre existiram no payload, não eram lidos) — filtros novos em
  `GET /collaborators` e catálogo em `GET /collaborators/filters`.
- Tratativa de ocorrência (Feature 4 do ciclo de evolução): novo estado `tratada` na
  máquina de estados, entidade `Treatment` com justificativa e anexo, endpoints
  `POST /occurrences/:id/treat`, `GET /occurrences/:id/treatments`,
  `POST /treatments/:id/undo`.
- Upload e download de anexo (PDF) vinculado a uma tratativa, armazenado no banco local,
  com validação de assinatura de arquivo e log de acesso por download
  (`GET /attachments/:id/download`).
- Filtros de severidade, tipo e paginação server-side em `GET /occurrences`, com `total`
  refletindo o filtro completo (Feature 1 — histórico).
- Endpoint agregado `GET /tenants/:id/occurrence-events` — "o que foi tratado neste
  período, por quem", com nome do colaborador e tipo já embutidos (sem N+1).
- Revisão mensal (Feature 3): encerramento e reabertura por tenant + competência (não por
  filial), com as seis condições do painel (quatro automáticas recalculadas a cada
  consulta, duas manuais persistidas), congelamento de tratativa/ignorar em competência
  encerrada, data de corte configurável por tenant
  (`TenantSettings.revisao_mensal_dia_corte`) e relatório consolidado exportável
  (`GET /tenants/:id/monthly-reviews/export`) para competências já encerradas.
- Papéis e permissões (Feature 6): `domain.Role` com três níveis aninhados
  (RH ⊃ Gestor ⊃ Diretoria), persistidos em `user_tenants.role`; papel obrigatório ao
  vincular usuário a um tenant; troca de papel (`PATCH /tenants/:id/users/:userId/role`);
  promoção/rebaixamento de super admin (`PATCH /users/:id/super-admin`); papel exposto no
  login e em `GET /tenants`; papel mínimo aplicado às rotas de escrita do sistema
  (configurações, filiais, advertências, ocorrências, tratativas, revisão mensal), com a
  exceção da Diretoria em `POST /audit/trigger` (só o fechamento de D-1 sem parâmetro).

### Changed
- `OccurrenceRepository.Ignore` passa a rejeitar (conflito) uma ocorrência que já tem
  tratativa registrada, em vez de sobrescrever silenciosamente o desfecho.
- `GET /tenants/:id/users` exige Super Admin (antes, qualquer vínculo com o tenant).

### Fixed
- Reconciliação de ocorrências (`reconciler.go`) trata `tratada` e `resolvida_manual`
  igualmente como desfechos pegajosos, via `OccurrenceState.Sticky()`.
- Filtros de departamento/função/empresa em `GET /occurrences` passam a ser resolvidos
  antes da paginação (via `collaborator_id IN (...)` no SQL), evitando que uma página
  pedida com `limit=20` devolvesse menos itens do que o `total` anunciava.
- Erros de infraestrutura na checagem de papel (`GetRole`) não são mais mascarados como
  403 "sem permissão" — só a ausência de vínculo (não encontrado) vira 403; qualquer outra
  falha aparece como 500, como já acontecia em `RequireTenantAccess`.

## [0.1.0] - 2026-08-31

Primeiro ciclo de desenvolvimento do painel de compliance — do protótipo inicial até um
sistema com auditoria diária automática, notificação via WhatsApp, papéis de acesso e
telas de histórico/ranking/revisão mensal.

### Added
- Estrutura inicial do backend em Go (models, migrations, controllers) e docker-compose
  do projeto.
- Motor de auditoria de jornada contra regras da CLT: interjornada, intervalo de almoço,
  horas extras, batida esquecida, trabalho em folga/DSR.
- Cliente de integração com a API do Secullum Ponto Web (funcionários, batidas, horários).
- Interface administrativa (Vite + React + TypeScript) substituindo o painel de testes
  inicial.
- Telas de Empresa, prévia de multiempresa, Indicadores (métricas de compliance) e
  Colaboradores (listagem + histórico individual).
- Módulo de WhatsApp completo (integração com Evolution API) para envio de avisos, com
  tela de conexão (QR + status) e gerência de instância por tenant.
- Autenticação (JWT) ponta a ponta, vínculo de usuários a tenants (N:N) com isolamento de
  dados entre tenants, e painel de moderação (super admin) para gestão de usuários e
  tenants.
- Máquina de estados de ocorrências (aberta, atualizada, resolvida automática, resolvida
  manual), cadastro de filiais, advertências e tela de auditoria por dia.
- Agendamento da auditoria diária automática no horário configurado.
- Auditoria de período (intervalo arbitrário de datas), histórico de relatórios de
  execução e notificação silenciosa (fora do horário configurado).
- Contrato de papéis e permissões (4 níveis: Super Admin, RH, Gestor, Diretoria).
- Lotação de colaborador em filial pela ficha e em massa, sem digitar número de folha.
- Sincronização mensal automática de colaboradores.
- Sincronização de equipamentos (relógios de ponto), enriquecimento de auditoria por
  `FonteDados` (equipamento/motivo da marcação) e controle de demissão, com tela de
  Equipamentos, botão de ressincronizar e histórico de colaboradores desligados.
- Telas de Histórico de tratamento, Ranking (colaboradores/filiais/melhora), Revisão
  mensal e Investigar (sinais operacionais).

### Changed
- Painel unificado e depois desfeito: Indicadores e Auditorias foram temporariamente
  mesclados numa só página e, na sequência, separados de novo em Histórico de varreduras
  e Logs do sistema.
- Bottom nav mobile substituída por menu hambúrguer + drawer.
- Auditoria horária silenciosa passa a cobrir o mês corrente inteiro, não só D-1.
- Telas de histórico renomeadas para descrever a pergunta que cada uma responde
  ("Situação por dia" vs. "Registro de execuções"), em vez do formato que as duas tinham
  em comum.
- Seletores de período passam a usar data local em vez de UTC.

### Fixed
- Motor de auditoria corrigido para ler a carga horária do colaborador corretamente (via
  campos `Memoria*` do endpoint de batidas, não mais o endpoint de horários) — evitava que
  jornadas integrais fossem contabilizadas como hora extra.
- Formatação de horas nas descrições de inconsistência (`XhYYmin`).
- Exclusão de usuário passa a remover a linha correspondente do banco.
- Domingo não gera mais "Almoço Reduzido" crítico por pausa curta.
- Cards de severidade em Indicadores linkavam para a lista de incidentes sem filtro de
  data — o número do card não batia com o que aparecia na lista aberta.
- Falha de build por dependência desnecessária do `apk` no Docker.
- Responsividade em mobile e tablet do painel.

### Infrastructure
- `VITE_API_URL` configurável por ambiente, injetada no build de produção do frontend.
- Docker Compose de produção com serviço de frontend.
