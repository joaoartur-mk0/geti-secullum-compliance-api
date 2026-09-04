# Tratativas, Filial e Revisão Mensal — contrato de backend

**Origem:** documento funcional do ciclo de evolução (features 1 a 6), cruzado com o
código atual em `41b05f7`.

> ⚠ **Este documento é anterior ao fechamento de escopo.** A fonte de verdade do que entra
> e do que fica fora é `docs/documento-funcional-compliance.md` (rascunho 2). Diferenças
> principais: **isolamento por filial saiu do ciclo** (seções 3.1, 3.2 e 4 abaixo estão
> fora), **filial não muda** (seção 2.2 fora), **revisão mensal encerra por tenant e não
> por filial** (seção 6), **`ciente_sem_acao` foi descartado** (seção 5.1) e a **seção 9
> estava factualmente errada** — já corrigida abaixo.

**Para quem:** João (Go, banco, scheduler, integração Secullum).

**O que este documento é:** a lista do que precisa existir no backend para as features do
novo ciclo, na ordem em que destrava trabalho. O frontend já está entregando a fatia que
não depende de nada disso — ver `docs/11_Historico_Ranking_Frontend_Contract.md`.

---

## 0. Decisões já travadas (não reabrir na implementação)

1. **Severidade continua com três valores**: `CRITICO`, `ALERTA`, `OPERACIONAL`. A tabela
   de quatro níveis do documento funcional foi descartada.
2. **`OPERACIONAL` ganhou semântica explícita**: é sinal de **investigação** — provável
   troca de escala não comunicada ou operação deliberada não avisada ao RH. Precisa ser
   apurado **antes** de virar auditoria. Não é infração e não pontua em ranking.
3. **"Fechamento" continua sendo o diário de D-1.** O ciclo mensal chama-se **revisão
   mensal**. Nenhum identificador novo (tabela, campo, rota, fila, log) pode usar
   "fechamento"/"closing" para o mensal.
4. **Competência** é o mês calendário, `YYYY-MM`.

---

## 1. Contrato de não-regressão — o que o painel já depende

Estas quatro coisas estão em uso hoje. Mudar qualquer uma quebra tela em produção:

| O que | Onde | Por que importa |
|---|---|---|
| `GET /occurrences?state=` aceitar lista separada por vírgula com os **quatro** estados | `buildOccurrenceFilter` | É o que torna o histórico possível sem endpoint novo |
| `resolved_at` e `first_seen_at` na resposta | `occurrenceResponse` | O painel calcula "tempo até o desfecho" com esses dois |
| `filial` enriquecida em cada ocorrência | `enrich` | Base de todo recorte por filial no cliente |
| `severity` com os três valores atuais | — | Pesos de ranking dependem disso |

Se algum desses precisar mudar, avise antes: o ajuste no painel é barato, a descoberta em
produção não.

---

## 2. Prioridade 0 — corrigir o que já existe

### 2.1 `branch_id` é filtrado em memória

`OccurrenceHandler.List` monta a resposta inteira, enriquece com filial e **só então**
descarta o que não bate com `branch_id` (`occurrence_handler.go:107`). Consequências:

- `total` na resposta não corresponde à lista devolvida quando há filtro de filial;
- o custo é o da consulta inteira, sempre;
- não serve como base de isolamento (seção 4) — filtro de leitura não é autorização.

**Correção:** filial precisa ser coluna da ocorrência (2.2) e o filtro precisa ir para o
`WHERE`.

### 2.2 Filial não é persistida na ocorrência — FORA DESTE CICLO

> Filial não muda neste ciclo (nem o cadastro, nem a resolução, nem a persistência). O
> diagnóstico abaixo continua correto e vale para o próximo ciclo — junto com a descoberta
> de que a Secullum **modela** unidade organizacional (`Estrutura`), ao contrário do que
> afirmam os comentários de `domain/Branch.go` e `usecase/branch_resolver.go`. Ver
> `docs/documento-funcional-compliance.md` §8.

Hoje a filial é resolvida **em tempo de consulta** por `BranchResolverService`. Isso
significa que a mesma ocorrência muda de filial se alguém editar o cadastro de nº de folha
depois — e uma revisão mensal por filial calculada sobre resolução dinâmica não é
auditável.

**Correção:** congelar a filial no momento da reconciliação.

```go
// domain/Occurrence.go
type Occurrence struct {
    // ... campos atuais ...
    BranchID     *int                   // filial no momento em que o fato foi apurado
    BranchSource BranchResolutionSource // "aparelho" | "numero_folha" | ""
}
```

```sql
ALTER TABLE occurrences ADD COLUMN branch_id INTEGER NULL REFERENCES branches(id);
ALTER TABLE occurrences ADD COLUMN branch_source VARCHAR(20) NOT NULL DEFAULT '';
CREATE INDEX idx_occurrences_tenant_branch_date ON occurrences (tenant_id, branch_id, date);
```

`NULL` é resultado válido e frequente (aparelho não cadastrado, colaborador sem lotação) —
não trate como erro; o painel já tem a linha "Sem filial".

Ocorrências antigas ficam com `branch_id NULL` até uma reauditoria. Vale um script de
backfill que roda o resolver sobre o histórico e grava o resultado, marcando
`branch_source` com o que conseguiu.

### 2.3 A resolução por aparelho está inerte em consulta de período

`enrich` só carrega as batidas quando o filtro é de **dia único**
(`occurrence_handler.go:144`). Em período, `punches` vai vazio e a resolução cai sempre no
nº de folha. Some-se a isso que **nenhuma filial tem `EquipId` cadastrado** hoje, e o
caminho forte de resolução nunca roda.

Com 2.2 resolvido isso deixa de importar para a leitura (a filial vem da coluna), mas o
**momento da gravação** precisa ter as batidas em mãos — o que a reconciliação já tem.

### 2.4 `GET /occurrences` não filtra severidade nem tipo, e não pagina

O painel filtra severidade no cliente sobre a lista inteira (`Incidentes.tsx:84`) e pagina
em memória. Funciona com o volume atual; não funciona com um ano de histórico.

**Correção:** `?severity=`, `?type=` (ambos aceitando lista) e `?limit=`/`?offset=`, com
`total` refletindo o filtro, não a página.

### 2.5 O log de eventos só é consultável por ocorrência

`OccurrenceRepository.ListEvents(occurrenceID)` responde "por que esta ocorrência está
assim". Não responde "o que foi tratado neste mês, por quem" — que é a pergunta central da
feature 1.

**Correção:** `GET /tenants/:id/occurrence-events?start_date=&end_date=&actor_user_id=&type=`,
com o nome do colaborador e o tipo da ocorrência já embutidos (senão o painel faz N+1).

---

## 3. Prioridade 1 — papéis e permissões

`docs/08_Roles_And_Permissions_Contract.md` está especificado até o SQL e **continua com
zero linhas no código** (não há nenhuma ocorrência de `Role` em `backend/internal/`).

Implementar como está escrito lá, com **uma alteração**: `GET /tenants/:id/users` sobe de
**RH** para **Super Admin** (a matriz da seção 5.2 do doc 08 precisa ser corrigida).

> ⚠ **As seções 3.1 e 3.2 abaixo estão FORA deste ciclo.** Perfil consolidador e vínculo
> usuário ↔ filial só fazem sentido com isolamento por filial, que foi descartado
> (seção 4). Ficam registradas para o ciclo em que isolamento voltar.

### 3.1 Perfil consolidador — FORA DESTE CICLO

O documento funcional pede um perfil com **visão agregada de todas as filiais e capacidade
de comparar filiais entre si**. Os quatro papéis do doc 08 são aninhados e nenhum deles
tem recorte de filial, então isso não é um quinto papel: é uma **dimensão ortogonal**.

Proposta: o vínculo do usuário com filiais é que define o recorte, e ausência de vínculo
com filial significa "vê todas".

### 3.2 Vínculo usuário ↔ filial (não previsto no doc 08)

```sql
CREATE TABLE user_branches (
  user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  branch_id INTEGER NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
  tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, branch_id)
);
```

Regra: **sem linha nenhuma = vê todas as filiais do tenant** (consolidador). Com uma ou
mais linhas, o `WHERE branch_id IN (...)` entra em toda consulta de ocorrência, relatório,
histórico, ranking e exportação.

O default aberto é deliberado: fechado por padrão deixaria todo usuário existente sem ver
nada no dia do deploy.

---

## 4. Isolamento de filial — FORA DESTE CICLO

> **Decisão:** não entra. Toda a staff vê todas as filiais. Filial continua sendo recorte
> de leitura, e **nenhuma tela pode sugerir que é fronteira de segurança**.
>
> Motivo: o ciclo já carrega tratativa, anexos, histórico, revisão mensal com encerramento,
> papéis e sincronização de departamento/função/empresa. Isolamento é o único item com
> critério de aceite de segurança — entregue pela metade é pior que não entregue, porque
> cria a crença de que existe.
>
> O restante desta seção fica registrado para o ciclo em que isolamento voltar.

Critério de aceite do documento funcional, literal: *"um gestor da filial A não consegue,
por nenhum caminho (filtro, URL, exportação, ranking), ver dado da filial B."*

Isso não é filtro; é `WHERE` aplicado no repositório, derivado do vínculo de 3.2, em
**todas** as leituras. Dois pontos de atenção:

1. **`filial: null` vaza.** Uma ocorrência sem filial resolvida não pertence a ninguém — e
   se ela aparecer para todo mundo, o isolamento tem um buraco do tamanho da taxa de não
   resolução, que hoje é alta. **Decisão necessária:** ocorrência sem filial aparece só
   para o consolidador (recomendado) ou para todos?
2. **Relatórios (`Report`) não têm filial.** A tela "Situação por dia" e o gráfico de
   evolução consomem `Report`, que é por tenant/dia, sem recorte de unidade. Ou o relatório
   ganha quebra por filial, ou essas telas ficam indisponíveis para gestor de filial.

---

## 5. Prioridade 2 — tratativa (feature 4)

### 5.1 O estado que falta

Hoje `resolvida_manual` significa **ignorada** (o usuário decidiu que o apontamento não
procedia), e o campo se chama `IgnoredReason`. O documento funcional quer **dois**
desfechos distintos: tratada e ignorada.

**Não renomeie `resolvida_manual`.** Linhas gravadas com esse valor são ignoradas de fato;
reinterpretá-las como "tratadas" corrompe o histórico que já existe. Adicione:

```go
const (
    OccurrenceTreated OccurrenceState = "tratada" // houve ação sobre o problema
)
```

> **Decidido:** `ciente_sem_acao` foi **descartado**. Fica só `tratada`, totalizando cinco
> estados de sistema: `aberta`, `atualizada`, `resolvida_automatica`, `resolvida_manual`
> (exibida como "Ignorada") e `tratada`.
>
> Ponto de atenção que continua valendo: **`resolvida_automatica` nunca pode ser fundida
> com `tratada`**. Ela é a metade "resolvidas" da dor que originou o ciclo — se as duas
> virarem a mesma coisa, o histórico deixa de distinguir trabalho humano de correção na
> origem, e a feature perde o propósito.

Como `resolvida_manual` e `tratada` são ambos manuais, `OccurrenceState.Open()` precisa
continuar devolvendo `false` para todos eles, e o `ChangeResolve` da reconciliação precisa
tratar os quatro estados terminais como pegajosos — hoje só `resolvida_manual` é.

### 5.2 A tratativa em si

```go
type Treatment struct {
    ID           int
    OccurrenceID int
    TenantID     int
    Outcome      OccurrenceState // tratada | resolvida_manual | ciente_sem_acao
    Justification string
    Attachments  []Attachment
    BatchID      *string // preenchido quando veio de ação em lote
    ActorUserID  int
    CreatedAt    time.Time
    UndoneAt     *time.Time // desfazer não apaga: marca
    UndoneByUserID *int
}
```

Regras que vêm do documento funcional e não podem ser simplificadas:

1. Tratativa desfeita **nunca** apaga o registro anterior — grava evento novo.
2. Lote gera **N registros individuais rastreáveis**, com `batch_id` comum. Um lote de 40
   itens é 40 linhas, não uma.
3. Lote só aceita itens do **mesmo tipo** de inconsistência.
4. Anexo é obrigatório para os tipos assim marcados na taxonomia (5.4).
5. Alteração da origem depois da tratativa **sinaliza, não reverte** — ver 5.3.

### 5.3 Sinalizar mudança na origem após tratativa

Metade disso já existe: `resolvida_manual` é pegajoso, e a reconciliação não reabre. O que
falta é o **sinal**. Sugestão de campo derivado, sem estado novo:

```go
// A ocorrência tem desfecho, mas o fingerprint apurado mudou depois dele.
SourceChangedAfterTreatment bool
```

Comparar o `fingerprint` da última varredura com o vigente no momento da tratativa. Isso
exige guardar o fingerprint tratado no `Treatment`.

### 5.4 Taxonomia — o que existe e o que o documento inventou

O motor emite **7** tipos (`usecase/auditor.go:32`):

`Batida Esquecida` · `Almoço Reduzido` · `Interjornada Curta` · `Hora Extra Excedente` ·
`Alerta de Hora Extra` · `Carga Horária Não Apurada` · `Trabalho em Dia de Folga/DSR`

A tabela do documento funcional lista 11, dos quais **6 não existem** (banco de horas fora
do limite, atraso/saída antecipada, marcação fora da escala, batidas duplicadas, ausência
total de marcação, cadastro inconsistente) e **2 existentes não aparecem** (`Alerta de Hora
Extra`, `Carga Horária Não Apurada`).

Cada tipo inexistente é **regra nova a implementar**, não classificação de algo já
detectado. Isso precisa voltar para a conversa de escopo antes de virar tarefa.

O que a taxonomia precisa virar em código, para os 7 tipos que existem:

```go
type TypePolicy struct {
    Type            string
    RequiresAttachment bool
    AutoResolves    bool // some sozinho quando a origem é corrigida
}
```

### 5.5 Anexos

Não existe **nada** de upload no repositório (nenhum `multipart`, storage ou bucket).

**Decidido:**

1. Formato aceito: **PDF**, apenas.
2. Armazenamento: **banco local** (não S3/MinIO, não disco no VPS).
3. Teto de tamanho por arquivo — valor a definir na implementação.
4. Download **só** por rota autenticada que confere o tenant. Nunca caminho estático, nunca
   URL adivinhável.
5. Registrar quem baixou.

O item 4 não é negociável: anexo de tratativa é atestado médico e acordo de compensação —
dado sensível de saúde (LGPD art. 11).

---

## 6. Prioridade 3 — revisão mensal (feature 3)

> **Alterado:** o recorte é por **tenant e competência**, não por filial. Como o isolamento
> por filial saiu do escopo (seção 4), encerrar por filial encerraria um recorte que o
> sistema não isola nem garante. Revisitar quando filial voltar a ter fronteira real.

```sql
CREATE TABLE monthly_reviews (
  id           SERIAL PRIMARY KEY,
  tenant_id    INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  competencia  CHAR(7) NOT NULL,               -- "YYYY-MM"
  status       VARCHAR(20) NOT NULL,           -- aberta | encerrada
  payroll_done BOOLEAN NOT NULL DEFAULT FALSE, -- confirmação manual
  offsets_done BOOLEAN NOT NULL DEFAULT FALSE, -- confirmação manual
  closed_at    TIMESTAMP NULL,
  closed_by_user_id INTEGER NULL REFERENCES users(id),
  UNIQUE (tenant_id, competencia)
);
```

Regras:

1. Encerramento é **por tenant e por competência**.
2. Competência encerrada **congela**: nenhuma tratativa nova é aceita naquele intervalo.
3. Reabertura é possível, restrita, **com motivo registrado** — grava evento, não
   sobrescreve. *(Em aberto: qual papel mínimo pode reabrir.)*
4. Data de corte configurável **no nosso sistema**, por tenant. Não herdada da Secullum:
   embora `Empresa` traga `FechamentoPonto`/`DiaFechamentoPonto`, o controle de
   inconsistência é feito aqui e a data que rege o ciclo é a nossa.
5. O encerramento gera relatório consolidado exportável — é a evidência do ciclo.

**Condições automáticas do painel** (o frontend já calcula as quatro hoje, e vai passar a
consumir daqui quando existir): ocorrências em aberto, ocorrências a reconferir,
operacionais em aberto, e **dias da competência sem varredura**. As duas manuais (folha
processada, compensações lançadas) são as colunas booleanas acima.

Sugestão de endpoint único, para o painel não recalcular:
`GET /tenants/:id/monthly-reviews?competencia=YYYY-MM` devolvendo uma linha por filial com
as seis condições e o status.

---

## 7. Prioridade 4 — agregações

O painel calcula ranking e histórico no cliente, sobre a lista completa do período. Isso é
aceitável no volume atual e **deixa de ser** com isolamento por filial (o cliente não pode
receber o que não pode ver) ou com um ano de histórico.

Quando chegar aqui: `GET /tenants/:id/ranking?start_date=&end_date=&group_by=collaborator|branch`,
com os pesos aplicados no servidor. Os pesos travados são `CRITICO: 10`, `ALERTA: 3`,
`OPERACIONAL: 0` — e as duas medidas ("exposição" e "período") estão definidas na seção 3
do contrato de frontend. Não invente uma terceira.

---

## 8. Parametrização de regras

Ponto em aberto #2 do documento funcional, respondido: hoje é **parcialmente**
parametrizável. `TenantSettings` já tem liga/desliga por regra e **severidade configurável
por regra**. Os **limiares** são fixos no código (60 min de almoço, piso de 15 min aos
domingos, interjornada, limites do Art. 59).

Se o cliente quiser limiar próprio, é feature nova: campos em `tenant_settings` mais
validação de faixa. Ponto de atenção: limiar abaixo do mínimo legal transforma o produto
de compliance em ferramenta de burlar compliance. Sugiro piso legal travado no código, com
o tenant só podendo ser **mais** rigoroso.

---

## 9. Campos da Secullum — CORRIGIDO: nunca estiveram bloqueados

> A versão anterior desta seção afirmava que setor, função e dias trabalhados não existiam
> no payload da Secullum e tratava isso como bloqueio externo. **Estava errado.** O payload
> capturado em `docs/intern/Secullum_API_Responses/response_funcionarios_all_colaborator.json`
> — versionado neste repositório desde antes deste ciclo — contém todos eles.

O que o payload de funcionários realmente traz, e que `secullumFuncionarioResponse` em
`secullum/client.go` **não lê** hoje:

| Campo no payload | Conteúdo | Situação |
|---|---|---|
| `Departamento` / `DepartamentoId` | Objeto `{Id, Descricao, Nfolha}` | Não mapeado |
| `Funcao` / `FuncaoId` | Objeto `{Id, Descricao}` | Não mapeado |
| `Empresa` / `EmpresaId` | Objeto completo, com `Documento` (CNPJ) | Não mapeado |
| `Estrutura` / `EstruturaId` / `EstruturaPaiId` | Árvore genérica — ver seção 9.1 | Não mapeado |

Existem também endpoints de lista para alimentar seletores de filtro:
`/IntegracaoExterna/Departamentos`, `/IntegracaoExterna/Funcoes`,
`/IntegracaoExterna/Empresas` e `/IntegracaoExterna/Estruturas`.

**Dias trabalhados** não é campo direto, mas é **derivável**: `GetDailyPunchesRange` já
traz um registro por colaborador por dia (`domain.DailyPunch`, com `Marcacoes` e `Folga`).
Contar os dias com marcação resolve. Ainda assim, a normalização do ranking ficou para o
próximo ciclo — não por falta de dado, mas porque muda a fórmula de pontuação.

Portanto: setor, função e empresa são **trabalho de mapeamento**, não negociação com
terceiro, e entram neste ciclo. Detalhe funcional em `docs/documento-funcional-compliance.md`
§7.1.

### 9.1 `Estrutura` não é "filial" — cuidado

`Estrutura` é uma **árvore configurável pelo cliente** (`EstruturaPaiId`), sem semântica
garantida pela API. Numa base ela contém as unidades físicas (`MATRIZ`, `SÃO CRISTOVÃO`,
`SÃO JACINTO`, `VILA BARREIROS`); noutra, um organograma (`Diretoria` → `Supervisor de
sistema fiscal`, …). `EstruturaId` também é **anulável** — 4 de 9 colaboradores do payload
capturado estão sem estrutura.

Consequência: **não** tratar `Filial := Estrutura` como equivalência. Isso está fora deste
ciclo e o encaminhamento está em `docs/documento-funcional-compliance.md` §8.

### 9.2 Qualidade do cadastro — o ruído real

O documento funcional citava "ACOUGUE" vs "AÇOUGUE" como problema de acento. No cadastro
real os acentos estão corretos. Os problemas de verdade são outros três:

1. **Sufixo de unidade no nome do setor:** `AÇOUGUE`, `AÇOUGUE MATRIZ`, `AÇOUGUE SC`,
   `AÇOUGUE SJ` são quatro registros distintos.
2. **Grafia divergente:** `PREVENÇÃO E PERDA` vs `PREVENÇÃO DE PERDA MATRIZ`.
3. **Linhas-lixo:** `(Não informado)`, `10/06/2014` (uma data cadastrada como
   departamento), `Departamento` (placeholder literal).

**Decisão:** sincronizar cru, sem normalizar. O painel reflete a fonte; não a corrige. Um
de-para local vira feature própria se e quando o cliente reclamar.

---

## 10. Ordem sugerida — ATUALIZADA

Com filial e isolamento fora, a ordem passa a ser:

1. **9** — mapear departamento, função e empresa no cliente Secullum e no espelho local.
   Barato, sem dependência, destrava filtro em três telas de uma vez.
2. **5** — tratativa (individual) e anexos. A fundação: tudo abaixo consome o que ela
   produz. **Lote fica para uma segunda etapa.**
3. **2.4 + 2.5** — filtros server-side (severidade, tipo, paginação) e consulta agregada de
   eventos. É o que sustenta o histórico com volume real.
4. **6** — revisão mensal com encerramento por tenant.
5. **7** — agregações no servidor, se e quando o volume exigir.

**3 (papéis) corre em paralelo** — sem 3.1 e 3.2, que saíram junto com o isolamento.

**Fora:** 2.2 e 2.3 (filial), 4 (isolamento), 8 (limiares parametrizáveis).
