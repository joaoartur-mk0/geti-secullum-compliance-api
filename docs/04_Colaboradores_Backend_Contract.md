# Aba "Colaboradores" — o que falta no backend

A aba **Colaboradores** (frontend: `frontend/src/pages/Colaboradores.tsx` e
`ColaboradorHistorico.tsx`) já está no ar com o que o backend atual permite. Este
documento registra as duas funcionalidades que **dependem de backend novo** (do João)
para sair do papel.

---

## Já funciona hoje (frontend, sem backend novo)

- **Listagem de todos os sincronizados** via `GET /api/v1/tenants/:id/collaborators`
  (já implementado), com busca por nome/nº e filtro "só com ocorrência".
- **Página de histórico individual** (`/colaboradores/:secullumId`): a linha do tempo de
  ocorrências do colaborador é **derivada dos relatórios** (`GET /reports`), filtrando as
  inconsistências por `CollaboratorID` (= `secullum_id`). Resumo: total, críticas, última.
- A lista "colaboradores com ocorrências" da aba **Indicadores** linka para essa página.

---

## 1. Lista de exceções (tirar colaborador da auditoria e das métricas)

Objetivo: marcar um colaborador para ser **ignorado pela auditoria** (e, por consequência,
não contar nas métricas/indicadores), mesmo que ele gere inconsistências — ex.: cargo de
confiança sem controle de jornada, ou caso já tratado que não deve mais alertar.

Precisa de:

- **Persistência**: um flag por colaborador, ex. `audit_excluded bool` (default false) na
  tabela `collaborators`, OU uma tabela de exceções por tenant. Um motivo/observação
  (`exclusion_reason`) e data ajudam na rastreabilidade (compliance é auditável).
- **Auditor**: o loop do `consumer.go` deve **pular** colaboradores excluídos (ou o
  `AuditorService` recebe a flag e não gera inconsistências para eles). Decidir se o
  excluído some do relatório ou aparece marcado como "ignorado".
- **Endpoints**:
  - `PATCH /api/v1/tenants/:id/collaborators/:secullumId/exclusion` com `{ excluded: bool, reason?: string }`.
  - Expor o estado atual no `GET /collaborators` (campo `audit_excluded` no item) para o
    painel mostrar/alternar.
- **Métricas**: quando o campo `metrics` existir (ver `03_Metrics_Frontend_Contract.md`),
  `collaborators_audited`/`clean_count` devem **desconsiderar** os excluídos.

Frontend previsto: um toggle "Excluir da auditoria" na página individual + selo na lista.

---

## 2. Registro disciplinar (advertências e medidas adotadas)

Objetivo: na página individual, registrar advertências/medidas tomadas com o colaborador e
acompanhar se ele **continua reincidindo** depois delas. Isso é **dado novo** (escrito pelo
gestor), diferente das ocorrências (que são derivadas das varreduras).

Precisa de:

- **Persistência**: tabela nova, ex. `collaborator_measures` — `id`, `tenant_id`,
  `collaborator_secullum_id`, `type` (advertência verbal/escrita, suspensão, orientação…),
  `description`, `applied_by` (gestor), `date`, `created_at`.
- **Endpoints** (CRUD por colaborador):
  - `GET  /api/v1/tenants/:id/collaborators/:secullumId/measures`
  - `POST /api/v1/tenants/:id/collaborators/:secullumId/measures`
  - `PUT`/`DELETE` `.../measures/:measureId` (editar/remover).
- **Correlação**: com as datas das medidas + a linha do tempo de ocorrências (já derivada),
  o painel consegue mostrar "reincidiu N vezes após a última advertência".

Frontend previsto: seção "Histórico disciplinar" na página individual, com formulário de
registro e a lista de medidas intercalada/comparada com as ocorrências.

---

## Observação de identidade

As inconsistências e o endpoint de colaboradores usam o **`secullum_id`** como chave de
negócio (é o que aparece nos relatórios). Os dois recursos acima devem usar a mesma chave
para casar com o que o frontend já consome, evitando depender do `id` local do banco.
