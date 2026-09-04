# Documento Funcional — Ciclo de Evolução

**Produto:** camada de compliance sobre o Secullum (ERP de RH / ponto eletrônico)

**Natureza deste documento:** base funcional, agora **cruzada com o código real**. Não
descreve arquitetura nem schema — isso está em `docs/12_Revisao_Mensal_E_Tratativas_Backend_Contract.md`
(backend) e `docs/11_Historico_Ranking_Frontend_Contract.md` (frontend).

**Status:** rascunho 2. O rascunho 1 foi escrito sem contexto de código, a partir do pedido
dos usuários. Esta versão mantém as ideias e corrige tudo que não correspondia à realidade.

**Baseline de código:** `a025ea3`.

---

## 0. O que mudou do rascunho 1 para cá

O rascunho 1 fez sete afirmações que o código e o payload real da Secullum contradizem.
Estão listadas aqui porque algumas viraram escopo e outras viraram lixo — e quem ler o
documento antigo precisa saber quais.

| # | O rascunho 1 dizia | Realidade |
|---|---|---|
| 1 | 11 tipos de inconsistência | O motor emite **7** (`usecase/auditor.go`). 6 dos listados não existem; 2 existentes não foram listados. |
| 2 | 4 severidades (Crítica/Alta/Média/Baixa) | Existem **3**: `CRITICO`, `ALERTA`, `OPERACIONAL`. |
| 3 | Filial como sub-tenant com isolamento | **Fora deste ciclo.** Filial continua sendo filtro de leitura, não fronteira. |
| 4 | "Fechamento mensal" | "Fechamento" já é o nome da varredura **diária** de D-1. O ciclo mensal chama-se **revisão mensal**. |
| 5 | Setor/função como filtros transversais planos | O cadastro real embute a unidade no nome do departamento (`AÇOUGUE`, `AÇOUGUE MATRIZ`, `AÇOUGUE SC`, `AÇOUGUE SJ`). Entram crus, sem normalização. |
| 6 | "ACOUGUE" vs "AÇOUGUE" como problema de acento | Os acentos estão corretos no payload. O ruído real é sufixo de unidade e grafia divergente (`PREVENÇÃO E PERDA` vs `PREVENÇÃO DE PERDA MATRIZ`), além de linhas-lixo (`(Não informado)`, `10/06/2014`, `Departamento`). |
| 7 | Sem hierarquia organizacional | A Secullum modela `Estrutura` como **árvore** (`EstruturaPaiId`). Irrelevante neste ciclo porque filial ficou de fora, mas a afirmação é falsa. |

---

## 1. A dor

**Falta de visibilidade das inconsistências resolvidas e não resolvidas.**

Hoje o produto é diagnóstico: aponta problemas, e eles desaparecem da lista quando o dado
de origem é corrigido no Secullum. Não há registro de que existiram, de quem cuidou, nem
de como terminaram.

Isso define a prioridade do ciclo e a régua de qualquer decisão em aberto: se uma escolha
não melhora a visibilidade do que foi resolvido e do que continua pendente, ela não é
prioridade deste ciclo.

**Consequência direta:** o produto passa a ter **estado próprio** — tratativas,
justificativas, anexos e encerramento de competência. É a mudança estrutural da qual as
demais features dependem.

---

## 2. Glossário

| Termo | Definição |
|---|---|
| **Ocorrência** | A inconsistência com identidade estável e ciclo de vida (`domain.Occurrence`). É o que recebe desfecho. |
| **Inconsistência** | O apontamento bruto dentro de um `Report` de varredura. Não confundir com Ocorrência. |
| **Tipo** | A regra violada. São 7, fixos, listados na §4. |
| **Severidade** | `CRITICO`, `ALERTA` ou `OPERACIONAL`. Três valores, não quatro. |
| **Desfecho** | Estado terminal de uma ocorrência. |
| **Tratativa** | Ação humana registrada que dá desfecho a uma ocorrência, com justificativa e, quando exigido, anexo. |
| **Fechamento** | A varredura **diária** de D-1. Já existe. Nunca usar para o ciclo mensal. |
| **Revisão mensal** | O ciclo mensal: conferir o que falta e encerrar a competência. |
| **Competência** | Mês calendário, `YYYY-MM`. |
| **Empresa** | Pessoa jurídica (CNPJ) à qual o colaborador está vinculado, conforme a Secullum. |
| **Departamento** | Setor do colaborador, conforme cadastrado na Secullum. Atributo, não nível de agregação. |
| **Função** | Cargo do colaborador, conforme a Secullum. Atributo. |
| **Filial** | Unidade física do cadastro **local** (`domain.Branch`). Existe para saber quem cobrar (gestor + WhatsApp). Continua como está — ver §8. |

---

## 3. Ciclo de vida da ocorrência

### 3.1 Estados

Quatro estados existem hoje em `domain/Occurrence.go`. Um é novo.

| Estado (sistema) | Rótulo na tela | Significado | Situação |
|---|---|---|---|
| `aberta` | Em aberto | Detectada, sem desfecho | existe |
| `atualizada` | A reconferir | O valor mudou desde a última varredura | existe |
| `resolvida_automatica` | Corrigida na origem | O dado foi ajustado no Secullum | existe |
| `resolvida_manual` | Ignorada | Um usuário decidiu que o apontamento não procedia | existe |
| `tratada` | Tratado | Houve ação humana sobre o problema, registrada aqui | **novo** |

**Confirmado em 02/09/2026:** `tratada` é o quinto estado, novo, dedicado à Feature 4.

**Regras:**

1. `resolvida_manual` **não** é "tratada". Ignorar significa que o apontamento não
   procedia; tratar significa que houve ação sobre um problema real. Confundir os dois
   destrói exatamente a visibilidade que motivou o ciclo (§1).
2. **`resolvida_automatica` nunca é fundida com `tratada`.** Ela é a metade "resolvidas" da
   dor. Se virar Tratado, o histórico deixa de distinguir trabalho humano de correção na
   origem.
3. `CIENTE_SEM_AÇÃO` **não existe**. Foi avaliado e descartado.
4. Os cinco estados terminais e não-terminais convivem: `OccurrenceState.Open()` continua
   devolvendo `false` para `resolvida_manual`, `resolvida_automatica` e `tratada`.
5. A reconciliação trata os desfechos como **pegajosos** — não reabre o que já teve
   desfecho humano.

### 3.2 Exibição

A tela agrupa em quatro colunas, não cinco:

```
Aberto            = aberta + atualizada   (com "a reconferir" marcado dentro do grupo)
Tratado           = tratada
Ignorado          = resolvida_manual
Corrigido na origem = resolvida_automatica
```

A quarta coluna é obrigatória em toda visão de histórico e dashboard. Omiti-la responde
metade da pergunta da §1.

### 3.3 Alteração da origem depois da tratativa

Se o dado mudar no Secullum depois de a ocorrência ser tratada, o sistema **sinaliza e não
reverte**. A decisão fica com o usuário. Tratativa desfeita gera registro novo na trilha —
nunca apaga o anterior.

### 3.4 Rastreabilidade

Todo desfecho registra: quem, quando, qual desfecho, justificativa e anexos vinculados.

---

## 4. Tipos de inconsistência

São **sete**, emitidos por `usecase/auditor.go`. Esta é a lista real e completa:

| Tipo | Anexo na tratativa |
|---|---|
| Batida Esquecida | Não |
| Almoço Reduzido | Sim |
| Interjornada Curta | Sim |
| Hora Extra Excedente | Sim |
| Alerta de Hora Extra | Opcional |
| Carga Horária Não Apurada | Não |
| Trabalho em Dia de Folga/DSR | Sim |

A severidade de cada tipo é **configurável por tenant** (`TenantSettings`), assim como o
liga/desliga da regra. Os **limiares** (60 min de almoço, piso de 15 min aos domingos,
interjornada, limites do Art. 59) são fixos no código e **continuam fixos neste ciclo** —
ninguém pediu parametrização.

Os seis tipos listados no rascunho 1 que não existem (banco de horas, atraso/saída
antecipada, marcação fora da escala, batidas duplicadas, ausência total de marcação,
cadastro inconsistente) seriam **regras novas a implementar**, não classificação de algo
já detectado. Estão fora deste ciclo.

---

## 5. Severidade e pontuação

Três severidades, com pesos travados:

| Severidade | Peso | Atribuível ao colaborador? |
|---|---|---|
| `CRITICO` | 10 | Sim |
| `ALERTA` | 3 | Sim |
| `OPERACIONAL` | 0 | **Não** |

`OPERACIONAL` é **sinal de investigação** — provável troca de escala não comunicada ou
operação deliberada não avisada ao RH. Precisa ser apurado antes de virar auditoria. Peso 0
não significa "ignore": significa "não pontua". Continua contado, em campo próprio, e tem
tela própria (`/investigar`).

Duas medidas, nunca intercambiáveis:

| Medida | Estados que entram | Pergunta |
|---|---|---|
| **Exposição** | `aberta` + `atualizada` | Onde estou exposto agora? |
| **Período** | `aberta` + `atualizada` + `resolvida_automatica` + `tratada` | O que aconteceu naquele mês? |

`resolvida_manual` (ignorada) nunca pontua em nenhuma das duas — contá-la puniria alguém
por um falso positivo.

---

## 6. Escopo do ciclo

### 6.1 Dentro

| # | Item | Situação hoje |
|---|---|---|
| A | Sincronizar **departamento, função e empresa** do colaborador | Não existe. Dados já vêm no payload. |
| B | **Tratativa individual** (Feature 4) | Não existe. Fundação do ciclo. |
| C | **Anexos** em PDF | Não existe nada de upload. |
| D | **Histórico de tratamento** (Feature 1) | Tela existe (`Historico.tsx`), sobre os 4 estados atuais. |
| E | **Revisão mensal com encerramento** (Feature 3) | Tela existe e só diagnostica. Falta o ato de encerrar. |
| F | **Papéis e permissões** (4 níveis) | Especificado em `docs/08`. Zero linhas no código. |
| G | **Dashboards e ranking** (Features 2 e 6) | Telas existem. Ganham os campos novos e o estado `tratada`. |

### 6.2 Fora — e por quê

| Item | Motivo |
|---|---|
| **Isolamento por filial** (Feature 5) | Decisão explícita. Toda a staff vê todas as filiais. |
| **Redesenho de filial / Estrutura da Secullum** | Adiado. Ver §8. |
| **Tratativa em lote** | Segunda etapa: primeiro o individual, depois o lote. |
| **Metas e semáforo executivo** | Sem meta cadastrada não há semáforo. Não pedido. |
| **Limiares de regra parametrizáveis** | Não pedido. |
| **Normalização de ranking por dias trabalhados** | Próximo ciclo. Dado é derivável, mas muda a fórmula de pontuação. |
| **Carga retroativa de histórico** | Começa vazio. A reconciliação diária popula em 2–3 meses. |
| **Ranking/dashboard quebrados por empresa** | Empresa entra como campo e filtro, não como visão agregada própria. |
| **Normalização de departamento** | Entra cru, como a Secullum entrega. De-para vira feature própria se alguém reclamar. |

---

## 7. Features

### 7.1 Sincronizar departamento, função e empresa

**Objetivo:** dar ao painel os filtros que o rascunho 1 pedia em quatro features
diferentes, e que estavam declarados como bloqueados por falta de dado.

**Correção importante:** eles **não** estavam bloqueados. Os dados já chegam no payload de
funcionários (`Departamento`, `DepartamentoId`, `Funcao`, `FuncaoId`, `Empresa`,
`EmpresaId`) e simplesmente não eram lidos. É trabalho de mapeamento, não de negociação
com terceiro.

**Regras:**

1. Gravar no cadastro do colaborador: **departamento**, **função** e **empresa**.
2. Manter, por tenant, a **lista de departamentos, funções e empresas existentes** — é o
   que alimenta os seletores de filtro.
3. Departamentos e funções são **globais no tenant**, sem divisão por unidade. Um
   colaborador aponta para um departamento; o departamento não pertence a filial nenhuma.
4. **Sem normalização.** Se o cadastro do cliente tem `AÇOUGUE` e `AÇOUGUE MATRIZ` como
   registros distintos, o filtro mostra os dois. O painel reflete a fonte; não a corrige.
5. Empresa é campo e filtro. Não gera visão agregada própria neste ciclo.

**Fontes e decisão de implementação:** os campos já vêm embutidos em cada funcionário
(`Departamento`, `Funcao`, `Empresa`). Os endpoints de lista (`/Departamentos`, `/Funcoes`,
`/Empresas`) existem, mas **não são usados** — o catálogo por tenant (regra 2) é derivado
via `DISTINCT` sobre o próprio cadastro de colaboradores já sincronizado, não sincronizado
à parte. Motivo: evita uma quarta chamada à Secullum por sincronização, evita listar um
departamento que nenhum colaborador usa, e atende ao critério de aceite ("existem no banco
do tenant") de forma mais direta que espelhar a lista externa.

**Aceite:** o filtro de departamento oferece exatamente os departamentos que existem no
banco do tenant, e filtrar por um deles devolve só os colaboradores daquele departamento.

---

### 7.2 Tratativa — Feature 4

**Fundação. Vem primeiro depois da 7.1.**

**Problema:** inconsistências como hora extra não se resolvem editando o cartão de ponto.
Resolvem-se fora do sistema, por compensação ou ocorrência formal. Hoje ficam eternamente
na lista, e não há como registrar que foram cuidadas.

**Regras:**

1. Toda ocorrência pode receber desfecho: **tratada** ou **ignorada**.
2. Ao tratar, o usuário informa **justificativa**. Anexo é obrigatório nos tipos marcados
   na §4.
3. Ocorrência com desfecho sai da fila de pendências e entra no histórico.
4. A tratativa fica no histórico do colaborador, consultável fora do contexto do mês.
5. Tratativa pode ser **desfeita**, gerando registro novo na trilha — nunca apagando o
   anterior.
6. Alteração posterior da origem **sinaliza, não reverte** (§3.3).
7. **Individual nesta etapa.** Lote vem depois, e quando vier: só itens do mesmo tipo,
   seleção explícita, sem "selecionar todos" cego, e N registros individuais rastreáveis
   com identificador de lote comum.

**Aceite:**

- Uma hora extra tratada com anexo sai da fila e aparece no histórico com autor, data,
  justificativa e documento.
- Tentar tratar um tipo com anexo obrigatório sem anexar é bloqueado.
- Desfazer uma tratativa deixa os dois registros visíveis na trilha.

---

### 7.3 Anexos

**Regras:**

1. Formato aceito: **PDF**, apenas.
2. Armazenamento: **banco local**.
3. Teto de tamanho por arquivo, definido na implementação.
4. Download **só** por rota autenticada que confere o tenant. Nunca caminho estático,
   nunca URL adivinhável.
5. Registrar quem baixou.

**Por que a restrição de acesso não é negociável:** anexo de tratativa é atestado médico e
acordo de compensação. Dado de saúde é dado sensível (LGPD art. 11), e servir por URL
previsível transforma um recurso de compliance em vazamento.

---

### 7.4 Histórico de tratamento — Feature 1

**Objetivo:** responder a pergunta da §1 — o que foi resolvido, o que não foi, e como.

**Regras:**

1. Granularidade: **dia · semana · mês**. Ano fica fora: não há volume que justifique o
   eixo.
2. Filtros: período, **departamento**, **função**, **empresa**, filial, tipo, desfecho,
   responsável pela tratativa, colaborador.
3. O histórico inclui **`resolvida_automatica`**, não só o tratado manualmente. Sem isso o
   gestor não mede o volume real do período — e é metade da dor original.
4. Cada registro abre o detalhe: dado original, justificativa, anexos e trilha.
5. **Tempo médio até o desfecho**: média de `resolved_at − first_seen_at`, só para
   ocorrências com desfecho.
6. Toda linha leva à ficha do colaborador.

**Perguntas que a tela responde:** quanto foi resolvido no período e por qual desfecho;
quanto tempo entre detecção e desfecho; onde a pendência se acumula; como o volume evolui.

**Aceite:** um período de 30 dias mostra os quatro grupos de desfecho com soma igual ao
total exibido.

---

### 7.5 Revisão mensal — Feature 3

**Nome:** revisão mensal. Nunca "fechamento" — esse nome já é da varredura diária de D-1.

**Decidido:** o ciclo é o mês calendário. O encerramento é **ato explícito**, porque as
condições incluem coisas que acontecem fora deste software (folha processada, compensações
lançadas) e o sistema não tem como saber sozinho que estão prontas.

**Recorte:** por **tenant e competência**.

> Nota de consequência: o rascunho 1 e `docs/12` §6 previam encerramento **por filial**.
> Como filial saiu do escopo deste ciclo (§6.2), encerrar por filial encerraria um recorte
> que o sistema não isola nem garante. O encerramento é por competência do tenant inteiro.
> Quando filial voltar a ter fronteira real, isso se revisita.

**Painel de revisão** — seis condições:

| Condição | Verificação |
|---|---|
| Ocorrências em aberto (`aberta` + `atualizada`) | Automática |
| A reconferir (`atualizada`) | Automática |
| Operacionais em aberto | Automática |
| Dias da competência sem varredura | Automática |
| Folha de pagamento processada | Confirmação manual |
| Compensações realizadas | Confirmação manual |

**Regras:**

1. Encerramento por competência, com **data de corte configurável no nosso sistema** — não
   herdada da Secullum. O controle de inconsistência é feito aqui, e a data que rege o
   ciclo é a nossa.
2. Após encerrada, a competência **congela**: não aceita tratativa nova naquele intervalo.
3. **Reabertura** é possível, restrita a perfil autorizado, **com motivo registrado** —
   grava evento, não sobrescreve.
4. A competência encerrada gera **relatório consolidado exportável**, que é a evidência do
   ciclo.
5. As duas condições manuais deixam de ser caixas decorativas e passam a persistir.

**Aceite:**

- Com pendências em aberto, o encerramento sinaliza o bloqueio e indica exatamente o que
  falta.
- Uma competência com dias não varridos nunca aparece como pronta, mesmo com zero
  ocorrências em aberto.
- Tratativa em competência encerrada é recusada.

---

### 7.6 Papéis e permissões

Quatro papéis aninhados: **Diretoria ⊂ Gestor ⊂ RH ⊂ Super Admin**. A especificação
completa está em `docs/08_Roles_And_Permissions_Contract.md` e vale como escrita, com três
confirmações e uma alteração:

**Confirmado:**

1. Gestor e Diretoria **não veem** Filiais, Gestores, Avisos e WhatsApp — nem em leitura.
   Esconder, não desabilitar.
2. Diretoria dispara **apenas** o "Auditar agora" (D-1). Dia específico e período exigem
   Gestor.
3. Gestor **pode** sincronizar colaboradores.

**Alterado em relação ao `docs/08`:**

4. `GET /tenants/:id/users` — quem tem acesso ao cliente — sobe de **RH** para **Super
   Admin**. `docs/08` §5.2 precisa ser corrigido.

**Não entra:** o vínculo usuário ↔ filial e o perfil consolidador propostos em `docs/12`
§3.1/§3.2. Eles só fazem sentido com isolamento, que está fora.

---

### 7.7 Dashboards e ranking — Features 2 e 6

Ambas as telas já existem. Neste ciclo elas **ganham conteúdo**, não redesenho.

**O que muda:**

1. O estado `tratada` entra nas contagens e na medida Período.
2. "Corrigido na origem" aparece como grupo próprio, sempre.
3. Filtros de **departamento**, **função** e **empresa** entram.
4. Ranking mantém as três abas (Colaboradores · Filiais · Melhora) e os pesos da §5.

**O que continua fora:**

- **Sem semáforo.** Não existe meta cadastrada; nenhuma tela pinta verde/amarelo/vermelho
  por número absoluto inventado. Cor é só severidade.
- **Sem normalização por dias trabalhados.** A nota da tela muda de "o dado não existe"
  para *"pontuação bruta — normalização por dias trabalhados prevista para o próximo
  ciclo"*, que é honesto e passou a ser verdade.
- Ranking individual segue com a nota: *"use para investigar causa, não para punir
  isoladamente."*

---

## 8. Filial — o que fica pendente e por quê

Filial **não muda neste ciclo**. Continua sendo o cadastro local (`domain.Branch`) com
resolução por aparelho e por nº de folha. Mas duas descobertas precisam ficar registradas,
porque contradizem comentários do próprio código e vão reger o próximo ciclo:

**1. A Secullum modela unidade organizacional.** `domain/Branch.go` afirma *"a Secullum não
modela isso — para ela existe só a empresa"*, e `usecase/branch_resolver.go` repete
*"a Secullum não tem o conceito de filial"*. **As duas afirmações são falsas.** O payload de
funcionários traz `Estrutura`, `EstruturaId` e `EstruturaPaiId`. O `BranchResolverService`
inteiro — de-para de aparelho, de-para de nº de folha, a tela de Lotação — existe para
inferir um dado que a fonte devolve pronto.

**2. Mas `Estrutura` não significa "local físico".** É uma **árvore genérica** que cada
cliente preenche como quiser. Numa base ela contém as quatro lojas (`MATRIZ`,
`SÃO CRISTOVÃO`, `SÃO JACINTO`, `VILA BARREIROS`); noutra base, um organograma
(`Diretoria` → `Supervisor de sistema fiscal`, `Supervisor certificado digital`…). Amarrar
`Filial := Estrutura` como equivalência dura quebraria em qualquer tenant que use o campo
de outro jeito — e quebraria **pintando dado errado, sem dar erro**.

Além disso `EstruturaId` é **anulável**: no payload capturado, 4 de 9 colaboradores estão
sem estrutura. A linha "Sem filial" continua sendo de primeira classe em qualquer agregação.

**Encaminhamento sugerido para o próximo ciclo:** sincronizar `Estrutura` como dado bruto do
colaborador (custa nada, já vem no payload), permitir que a `Branch` local se vincule a uma
estrutura, e criar uma terceira fonte de resolução à frente das duas atuais. A `Branch`
local sobrevive de qualquer forma pelo que só ela tem: `ManagerName` e `ManagerPhone`, que é
quem recebe o WhatsApp e não vem da Secullum.

**Ação de higiene:** os comentários falsos em `Branch.go` e `branch_resolver.go` devem ser
corrigidos, mesmo sem mudança de comportamento. Comentário errado sobre a fonte de dados é
o tipo de coisa que faz a próxima pessoa reconstruir a inferência de novo.

---

## 9. Decisões consolidadas

| # | Decisão |
|---|---|
| 1 | A dor é visibilidade do que foi resolvido e do que não foi. É a régua do ciclo. |
| 2 | Cinco estados de sistema; quatro grupos de exibição. `resolvida_automatica` nunca funde com `tratada`. |
| 3 | `CIENTE_SEM_AÇÃO` não existe. |
| 4 | Sete tipos de inconsistência. Nenhum tipo novo neste ciclo. |
| 5 | Três severidades, pesos 10 / 3 / 0. `OPERACIONAL` não pontua e não é omitido. |
| 6 | Limiares de regra continuam fixos no código. |
| 7 | Isolamento por filial está fora. Toda a staff vê todas as filiais. |
| 8 | Filial não muda neste ciclo. As descobertas sobre `Estrutura` ficam registradas na §8. |
| 9 | Departamento, função e empresa são gravados no colaborador e listados por tenant, crus. |
| 10 | Tratativa individual agora; lote depois. |
| 11 | Anexos em PDF, no banco local, servidos só por rota autenticada. |
| 12 | Revisão mensal encerra por **tenant + competência**, com data de corte configurada aqui. |
| 13 | Competência encerrada congela; reabertura é restrita e exige motivo registrado. |
| 14 | Quatro papéis conforme `docs/08`, com `GET /tenants/:id/users` promovido a Super Admin. |
| 15 | Sem metas, sem semáforo. |
| 16 | Histórico começa vazio; sem carga retroativa. |

---

## 10. Pontos em aberto

| # | Ponto | Impacto |
|---|---|---|
| 1 | Teto de tamanho do anexo PDF | Baixo — decidir na implementação |
| 2 | Qual perfil pode reabrir competência encerrada | Médio — trava a §7.5 regra 3 |

---

## 11. Ordem de entrega

1. **7.1 — sincronizar departamento, função e empresa.** Barato, sem dependência, e
   destrava filtro em três telas de uma vez.
2. **7.2 + 7.3 — tratativa individual e anexos.** A fundação. Tudo abaixo consome o que
   ela produz.
3. **7.4 — histórico.** É a resposta direta à dor da §1.
4. **7.5 — revisão mensal com encerramento.**
5. **7.7 — dashboards e ranking** ganham os campos e o estado novo.

**7.6 (papéis) corre em paralelo** — não depende de nenhum item acima e nenhum item acima
depende dele.
