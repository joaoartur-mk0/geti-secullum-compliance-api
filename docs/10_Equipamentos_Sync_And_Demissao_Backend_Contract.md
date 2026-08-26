# Equipamentos, sincronização diária fixa, enriquecimento de auditoria e demissão

Este documento cobre quatro mudanças de backend (+ o pouco de frontend que consome cada
uma), implementadas juntas por serem todas parte da mesma rotina de sincronização com a
Secullum. Item por item:

1. Espelho local de equipamentos (relógios de ponto).
2. Enriquecimento da auditoria com equipamento/motivo de cada marcação (FonteDados).
3. Separação de colaboradores ativos vs. histórico completo (demissão).
4. Horário fixo (03:00) para a sincronização diária, com um bug de concorrência corrigido
   nesta mesma rodada.

---

## 1. Equipamentos

**Problema:** a sincronização de colaboradores (funcionários + jornadas) já existia; os
aparelhos de ponto cadastrados na Secullum não tinham nenhum espelho local.

**O que foi construído:**

| Peça | Onde |
|---|---|
| Tabela `equipamentos` (`id`, `secullum_id`, `descricao`, `endereco_ip`, `tenant_id`) | `models.Equipment`, migrada via GORM `AutoMigrate` |
| `domain.EquipmentRepository.SaveAll` | upsert por `(tenant_id, secullum_id)` **+ hard delete** de qualquer linha do tenant fora da lista recebida |
| `SecullumService.GetEquipamentos` | `GET /IntegracaoExterna/Equipamentos` |
| `SynchronizerService.SyncEquipment` | chamado pelo mesmo fluxo que já sincronizava colaboradores (fila `tenant.provisioning`) |
| `GET /api/v1/tenants/:id/equipamentos` | leitura, somente consulta (`EquipmentHandler.List`) |
| `frontend/src/pages/Equipamentos.tsx` | lista com busca, sem CRUD (o cadastro vive na Secullum) |

**Espelhamento 1:1, de propósito.** `SaveAll` apaga qualquer equipamento local cujo
`secullum_id` não veio na resposta mais recente — inclusive **apagar tudo** se a resposta
vier vazia. Isso é o que a operação pediu (equipamento removido lá precisa sumir daqui), mas
é indistinguível, só pelo dado, de uma resposta degradada da Secullum (200 com corpo vazio
por engano). Não há como resolver isso sem violar o espelhamento estrito pedido — o que
existe é visibilidade: `equipmentRepository.SaveAll` registra `[Aviso Sync]` no log sempre
que uma lista vazia zera um cadastro que tinha equipamentos. **Dívida aceita, não
corrigida**: se isso incomodar na operação real, a próxima iteração seria não confiar
cegamente numa lista vazia (ex.: exigir confirmação, ou só apagar se a lista vier vazia N
sincronizações seguidas).

---

## 2. Enriquecimento com FonteDados (equipamento/motivo por dia)

**Problema:** a auditoria sabe que houve uma inconsistência, mas não sabia **onde** (qual
aparelho) nem **por quê** (motivo de uma inclusão manual, por exemplo) a marcação aconteceu.

**Chave de correlação:** a própria resposta de `Batidas` já traz, por marcação,
`FonteDadosIdEntradaN`/`FonteDadosIdSaidaN` — o `Id` exato do registro correspondente no
endpoint `GET /IntegracaoExterna/FonteDados?DataInicio=&DataFim=`. Não é preciso casar por
`FuncionarioId`+`Data`+`Hora`.

**Fluxo (`AuditConsumer`, `consumer.go`):**

```
GetDailyPunchesRange(...)         // já buscava as batidas do período
GetFonteDados(tenant, start, end) // NOVO — mesmo período, uma única chamada
  -> fonteDadosByID: map[int]FonteDadoItem

para cada colaborador, cada dia auditado:
  buildPunchRecord() varre as marcações do dia procurando o primeiro
  FonteDadosIDEntrada/Saida presente em fonteDadosByID
  -> domain.PunchRecord{EquipamentoID, Motivo}
```

Uma falha ao buscar `FonteDados` **não aborta a auditoria** (é enriquecimento, não o dado
principal) — fica só registrada em log, e os relatórios daquele ciclo seguem sem
equipamento/motivo. Um `Id` de fonte de dados sem correspondência no período consultado
também só gera log (`[Aviso Auditoria] ... sem correspondência no período consultado`).

**Persistência:** tabela `punch_records`, uma linha por `(tenant_id, collaborator_id,
date)` — quando o dia tem mais de uma marcação com fontes diferentes, fica a **primeira**
com correspondência encontrada, o suficiente para apontar "de onde veio o registro do dia"
sem multiplicar linhas por marcação.

**Consumo — adicionado depois do code review desta rodada.** A primeira versão gravava
`punch_records` e não expunha em lugar nenhum (achado do `/code-review`: "enriquecimento
write-only"). Corrigido com:

```
GET /api/v1/tenants/:id/collaborators/:secullumId/punch-records?start_date=&end_date=
```

`start_date`/`end_date` são obrigatórios juntos (mesma validação de
`audit_handler.resolvePeriod`, sem a exigência de "período já encerrado" — aqui é só
consulta). Resposta:

```json
{
  "punch_records": [
    { "date": "2026-08-24", "equipamento_id": 6, "motivo": null }
  ],
  "total": 1
}
```

Um dia ausente na resposta significa que a auditoria daquele dia não encontrou
correspondência — **não** que o colaborador não trabalhou. Não há UI consumindo este
endpoint ainda (fora do escopo desta rodada); ele existe para o dado parar de ser
write-only, não para virar tela.

---

## 3. Colaboradores ativos vs. histórico (demissão)

**Problema:** a sincronização de funcionários já recebia `Admissao`/`Demissao` da Secullum
e simplesmente ignorava `Demissao`. Um colaborador desligado continuava aparecendo como
qualquer outro.

**O que mudou:**

- `Collaborator` ganhou `Admissao *time.Time`, `Demissao *time.Time`, `Demitido bool` —
  `Demitido` é derivado de `Demissao` preenchida a cada sincronização (inclusive
  "reabilita" um colaborador se a Secullum limpar a `Demissao` por engano).
- `GET /tenants/:id/collaborators` passou a filtrar `demitido = false` — **só ativos**. É
  o que o motor de auditoria também usa (não faz sentido auditar jornada de quem já saiu).
- `GET /tenants/:id/collaborators/history` (novo) devolve todos, ativos e demitidos, com
  `admissao`/`demissao`/`demitido` no payload.

**Frontend:** `Colaboradores.tsx` ganhou o checkbox "Incluir desligados", que troca a fonte
de `listCollaborators` para `listCollaboratorsHistory` e mostra um selo "Desligado" com a
data. `ColaboradorHistorico.tsx` (a ficha individual) passou a usar sempre
`listCollaboratorsHistory`, para que a página de alguém desligado continue abrindo
corretamente — antes, usar só a lista de ativos fazia a ficha achar "não consta mais entre
os sincronizados", texto pensado para outro cenário (colaborador removido da Secullum, não
simplesmente desligado).

---

## 4. Horário fixo de sincronização (03:00) — e um bug corrigido

**Decisão:** todos os tenants ativos sincronizam colaboradores + equipamentos no mesmo
horário fixo (`03:00`, hora local do servidor), em vez de horários variados. Concorrência
limitada a 5 tenants simultâneos (`syncConcurrencyLimit`), cada tenant isolado — falha num
não impede os demais — com log de início/sucesso/erro por tenant e por etapa
(`syncTenantDaily`).

**Bug encontrado pelo `/code-review` e confirmado com `/diagnosing-bugs`:** a primeira
versão chamava `runDailySync` **sincronamente** dentro de `tick()`, que roda no único loop
`select` de `SchedulerService.Start()` (compartilhado com o ticker de 30s e o de auditoria
horária). Como `runDailySync` faz chamadas HTTP reais à Secullum e pode levar minutos com
muitos tenants, isso bloqueava a mesma chamada de `tick()` antes de ela chegar no loop de
disparo por `Horario` (a auditoria de fechamento configurada por tenant) — e o próximo tick
do agendador ficava pendente sem processar, porque **tickers do Go descartam ticks
perdidos, não enfileiram**. Na prática: um tenant configurado para fechar às 03:00 podia ter
o alerta atrasado, e a atualização horária silenciosa podia simplesmente pular uma hora.

Reproduzido com um teste que mede quanto tempo `tick()` leva para retornar com uma
sincronização lenta simulada (`TestScheduler_SincronizacaoDiariaNaoBloqueiaOTick`,
`scheduler_test.go`) — vermelho antes da correção (bloqueava pelos 200ms simulados),
verde depois. **Correção:** `runDailySync` agora roda em goroutine própria, contabilizada
num `sync.WaitGroup` (`dailySyncWG`) que só os testes usam para esperar deterministicamente
o fim da sincronização (`waitForDailySync()` — nunca chamado em produção).

**Ressincronização sob demanda.** `POST /api/v1/tenants/:id/sync` já existia (criada junto
com a sincronização de colaboradores, antes desta rodada) e publica na mesma fila
`tenant.provisioning` — por isso já sincronizava equipamentos assim que o worker passou a
tratar os dois. O botão "Ressincronizar" em `Colaboradores.tsx`/`Equipamentos.tsx` só
chama esse endpoint existente; como é assíncrono (fila), o retorno é um toast pedindo para
atualizar a lista em instantes, não um recarregamento automático.

---

## Decisões registradas (para não serem revisitadas sem motivo)

- **Sem migration framework.** O projeto usa só `gorm.AutoMigrate` (`cmd/api/main.go`) para
  toda tabela nova ou coluna nova — inclusive `equipamentos`, `punch_records` e as colunas
  de demissão. Um script de migração dedicado quebraria essa convenção existente; não foi
  criado um só para esta rodada.
- **Sem UI para `punch-records`.** O endpoint existe para o dado ser consultável (ver seção
  2), não porque uma tela foi pedida. Se/quando o painel precisar mostrar equipamento/motivo
  ao usuário final, é aí que entra a tela — este documento não assume que ela vem junto.
- **Espelhamento de equipamentos aceita apagar tudo numa resposta vazia** (ver seção 1) —
  fiel ao que foi pedido, com log de aviso em vez de proteção automática.
