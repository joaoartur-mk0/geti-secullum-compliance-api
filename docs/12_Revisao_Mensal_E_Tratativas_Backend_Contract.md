# Tratativas, Filial e Revisão Mensal — contrato de backend

**Origem:** documento funcional do ciclo de evolução (features 1 a 6), cruzado com o
código atual em `41b05f7`.

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

### 2.2 Filial não é persistida na ocorrência

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

Implementar como está escrito lá. Duas adições que o novo ciclo trouxe:

### 3.1 Perfil consolidador

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

## 4. Prioridade 1 — isolamento de filial de verdade

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
    OccurrenceTreated OccurrenceState = "tratada"        // houve ação sobre o problema
    OccurrenceAware   OccurrenceState = "ciente_sem_acao" // risco reconhecido e aceito
)
```

`ciente_sem_acao` está marcado como "a confirmar" no documento funcional. Recomendo criar
junto: o campo é barato agora e impossível de recuperar depois — sem ele, o usuário marca
hora extra pequena como "tratada" e o histórico perde a diferença entre trabalho feito e
risco aceito.

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
Decisão de infraestrutura necessária antes de estimar: disco no VPS, S3/MinIO, ou base de
dados. Restrições mínimas: tipo permitido, tamanho máximo, e o arquivo **não** pode ser
servido por URL adivinhável — anexo de tratativa é atestado médico, e isso é dado de saúde.

---

## 6. Prioridade 3 — revisão mensal (feature 3)

```sql
CREATE TABLE monthly_reviews (
  id           SERIAL PRIMARY KEY,
  tenant_id    INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  branch_id    INTEGER NULL REFERENCES branches(id),
  competencia  CHAR(7) NOT NULL,               -- "YYYY-MM"
  status       VARCHAR(20) NOT NULL,           -- aberta | encerrada
  payroll_done BOOLEAN NOT NULL DEFAULT FALSE, -- confirmação manual
  offsets_done BOOLEAN NOT NULL DEFAULT FALSE, -- confirmação manual
  closed_at    TIMESTAMP NULL,
  closed_by_user_id INTEGER NULL REFERENCES users(id),
  UNIQUE (tenant_id, branch_id, competencia)
);
```

Regras:

1. Encerramento é **por filial e por competência**, nunca global. `branch_id NULL` é a
   linha das ocorrências sem filial e também precisa poder ser encerrada.
2. Competência encerrada **congela**: nenhuma tratativa nova é aceita naquele intervalo.
3. Reabertura é possível, restrita, **com motivo registrado** — grava evento, não
   sobrescreve.
4. Data de corte configurável por tenant (antecipar/postergar o corte operacional).
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

## 9. Pendências de payload da Secullum — bloqueiam features inteiras

O documento funcional pede filtros e agregações por **setor** e **função** em quatro
features diferentes, e normalização de ranking por **dias trabalhados**. Nada disso existe
no cliente Secullum atual (`secullum/client.go`), no espelho local (`domain.Collaborator`)
ou na documentação de API que temos (`docs/01_Secullum_API_Info.md`).

**Ação pedida:** confirmar, no payload real da Secullum, se existem:

| Campo | Usado por | Se não existir |
|---|---|---|
| Setor / departamento do funcionário | Features 1, 2, 6 (filtros e quebras) | Vira cadastro local no painel, ou some do escopo |
| Função / cargo | Idem | Idem |
| Dias trabalhados no período | Feature 6 (normalização do ranking) | Ranking fica bruto, e precisa dizer isso na tela |

Enquanto não houver resposta, tudo que depende desses campos está **fora de escopo** e
declarado como tal na seção 11 do contrato de frontend. O documento funcional também cita
"ACOUGUE" vs "AÇOUGUE" como problema de qualidade de dado — vale notar que esse ruído vem
de um relatório que **este sistema ainda não lê**; ele só passa a existir quando setor
entrar.

---

## 10. Ordem sugerida

1. **2.2 + 2.4** — filial persistida e filtros server-side. Barato, destrava o resto e já
   melhora o que está no ar.
2. **3 + 4** — papéis e isolamento. É a fundação da feature 5 e o único item com critério
   de aceite de segurança.
3. **5** — tratativa. É a fundação de tudo que o documento chama de features 1, 3 e 6.
4. **2.5 + 6** — eventos agregados e revisão mensal.
5. **7 + 8** — agregações no servidor e limiares.

A seção 9 corre em paralelo e não depende de código: é uma consulta ao payload.
