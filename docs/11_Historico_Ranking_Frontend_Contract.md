# Histórico, Ranking e Revisão Mensal — contrato de frontend

**Origem:** documento funcional, **rascunho 1**. A versão vigente é
`docs/documento-funcional-compliance.md` (rascunho 2).

**Escopo deste documento:** a fatia das features 1, 2, 3 e 6 que era implementável **sem
backend** — e que já foi entregue (`d462944`). É contrato, não sugestão: os números destas
telas são calculados no cliente e precisam significar sempre a mesma coisa, em qualquer
tela.

> ⚠ **O que mudou depois desta escrita.** Este documento descreve o estado entregue, mas
> quatro afirmações dele deixam de valer quando o backend do ciclo entrar:
>
> | Seção | Dizia | Passa a ser |
> |---|---|---|
> | §6 | "`tratada` não existe" | Passa a existir. Cinco estados de sistema, quatro grupos de exibição. |
> | §9.2 r.5 | "dias trabalhados: o dado não existe" | O dado é derivável. Normalização adiada por decisão, não por falta. |
> | §9.3 r.3/r.4 | "não existe botão de encerrar" | Passa a existir, por tenant + competência. |
> | §11 | setor/função "não estão no payload" | **Errado.** Sempre estiveram. Entram neste ciclo. |
>
> Enquanto o backend não entrega, o que está escrito aqui continua sendo o comportamento
> correto das telas no ar.

O que depende de backend está em `docs/12_Revisao_Mensal_E_Tratativas_Backend_Contract.md`.

---

## 1. Vocabulário travado

| Termo | Significado | Não confundir com |
|---|---|---|
| **Fechamento** | A varredura **diária** de D-1 (scheduler, notificação WhatsApp). Já existe. | — |
| **Revisão mensal** | O ciclo **mensal** por filial: conferir o que falta antes de considerar a competência encerrada. | Fechamento |
| **Competência** | Mês calendário, no formato `YYYY-MM`. | Período (intervalo livre de datas) |
| **Ocorrência** | A inconsistência com identidade estável (`domain.Occurrence`). | Inconsistência de `Report`, que é linha de varredura |
| **Desfecho** | O estado terminal de uma ocorrência. | Categoria (eixo de exibição) |

Nenhuma tela, rótulo, rota ou comentário pode usar "fechamento" para o ciclo mensal.

---

## 2. Severidade — travado, não é decisão de implementação

O sistema tem **três** severidades e continua com três. A tabela de 4 níveis do documento
funcional (Crítica/Alta/Média/Baixa) **não** vale.

| Severidade | Significado | Atribuível ao colaborador? |
|---|---|---|
| `CRITICO` | Infração trabalhista grave. | Sim |
| `ALERTA` | Atenção preventiva. | Sim |
| `OPERACIONAL` | **Sinal de investigação**: provável troca de escala não comunicada, ou operação feita de propósito e não avisada ao RH. Precisa ser apurado **antes** de virar auditoria. | **Não** |

**Consequência dura:** `OPERACIONAL` nunca entra em pontuação punitiva, ranking de
colaborador ou semáforo de risco. Ele tem trilha própria (tela Investigar, seção 9.4) e
aparece como **contagem separada**, nunca somado ao resto e nunca omitido.

### Pesos

```
CRITICO: 10   ALERTA: 3   OPERACIONAL: 0
```

O peso 0 é deliberado e **não** significa "ignore": significa "não pontua". A ocorrência
operacional continua contada, em campo próprio.

---

## 3. Pontuação — duas medidas, nomes distintos, nunca intercambiáveis

Uma única "pontuação" não serve para as duas perguntas que o produto faz.

| Medida | Estados que entram | Pergunta que responde |
|---|---|---|
| **Exposição** | `aberta` + `atualizada` | "Onde eu estou exposto **agora**?" |
| **Período** | `aberta` + `atualizada` + `resolvida_automatica` | "O que **aconteceu** naquele mês?" |

`resolvida_manual` (= ignorada) **nunca** pontua, em nenhuma das duas: ignorar significa
que o apontamento não procedia. Contá-la puniria alguém por um falso positivo.

`resolvida_automatica` entra só em Período porque é o único jeito de medir volume real de
trabalho e de comparar mês contra mês — se ela saísse, todo mês encerrado tenderia a zero
e "ranking de melhora" viraria ruído.

**Padrão das telas:** Exposição. Período é uma troca explícita do usuário, rotulada.

---

## 4. Recorte por filial

A filial vem enriquecida em cada ocorrência (`occurrence.filial`), resolvida pelo backend.
Três regras:

1. **`filial: null` é uma linha de primeira classe**, rotulada **"Sem filial"**. Nenhuma
   tela pode descartar essas ocorrências silenciosamente — elas somem da conta e a soma
   das filiais deixa de bater com o total. Se somem, o painel mente.
2. **Em consulta de período (mais de um dia), a filial é resolvida só pelo nº de folha.**
   O backend só carrega as batidas do dia quando o filtro é de dia único, então a
   resolução por aparelho (a forte) não roda em período. Toda tela que agrega por filial
   em período exibe a nota: *"Filial resolvida pelo cadastro de lotação (nº de folha)."*
3. **Filtro de filial não é isolamento.** Hoje `branch_id` é conveniência de leitura, e o
   backend filtra em memória depois de montar a resposta. Nenhuma tela pode sugerir que a
   filial esconde dado de quem não deveria ver — isolamento é o item 3 do contrato de
   backend.

---

## 5. Período e competência

- Presets obrigatórios em toda tela com recorte temporal: **7 dias · 30 dias · Este mês ·
  Personalizado**. A tela Revisão mensal troca isso por um seletor de **competência**.
- Todo filtro vive na **querystring** (`start_date`, `end_date`, `branch_id`, `severity`,
  `type`, `state`, `competencia`), nunca só em estado de componente. Regra do documento
  funcional: filtros combináveis e persistentes ao navegar. Link colado no chat tem que
  abrir a mesma tela.
- Datas em `YYYY-MM-DD`. Exibição via `lib/format.ts`. Nunca `new Date(iso)` para data
  pura — o fuso desloca o dia.

---

## 6. Desfechos que existem hoje

O documento funcional prevê `TRATADA`, `IGNORADA` e talvez `CIENTE_SEM_AÇÃO`. **Nada
disso existe.** O que existe:

| Estado no backend | Como a tela chama | Significado |
|---|---|---|
| `aberta` | Em aberto | Detectada, sem desfecho |
| `atualizada` | A reconferir | O valor mudou desde a última varredura |
| `resolvida_automatica` | Corrigida na origem | O dado foi ajustado na Secullum |
| `resolvida_manual` | Ignorada | Um usuário decidiu que não procedia, com motivo |

**Proibido** rotular `resolvida_manual` como "tratada", "resolvida" ou "concluída". São
coisas diferentes, e a distinção é justamente o que a feature 4 vai comprar. Enquanto
"tratada" não existir, a tela diz que não existe.

---

## 7. Honestidade do dado — regras de exibição

1. **Vazio não é erro.** Período sem dado usa `EmptyState`, nunca `ErrorNote`.
2. **Zero não é sucesso.** "0 ocorrências em aberto" em um mês que não foi varrido inteiro
   é mentira. Toda agregação mensal exibe **quantos dias do período têm varredura** e
   quantos não têm.
3. **Sem meta, sem semáforo.** Não existe limite aceitável cadastrado (ponto em aberto #5
   do documento funcional). Nenhuma tela pinta verde/amarelo/vermelho por número absoluto
   inventado. Cor é só severidade, que já tem tokens no design system.
4. **Estimativa é rotulada.** Qualquer número derivado no cliente que o backend poderia
   contradizer leva nota de origem.

---

## 8. Restrições técnicas

- **Zero dependências novas.** Sem biblioteca de gráfico. Barras em CSS/Tailwind, como
  `DistributionChart` e `TrendChart` em `Indicadores.tsx`.
- **Zero alteração em `lib/api.ts`, `lib/types.ts`, `App.tsx`, `layouts/AppShell.tsx`,
  `components/ui.tsx`** nesta rodada. A fundação compartilhada já está escrita; quem
  precisar de algo que não existe lá **para e pede**, não edita.
- **Sem paginação server-side.** `GET /occurrences` devolve a lista inteira do período.
  Paginação é client-side, 20 por página, padrão de `Incidentes.tsx`.
- Tudo em português do Brasil, incluindo comentários. Comentário explica **por quê**, não
  o quê — padrão do repositório.
- `npm run build` (tsc -b + vite) e `npm run lint` (oxlint) limpos.

---

## 9. As telas

Cada tela responde **uma** pergunta. Tela que responde duas é duas telas.

### 9.1 `/historico` — Histórico de tratamento

**Pergunta:** o que teve desfecho no período, e como.

**Fonte:** `api.listOccurrences(tenantId, { start_date, end_date, state: [os quatro] })`.

**Regras:**
1. Quatro números no topo, por desfecho (seção 6), mais o total.
2. **Tempo médio até o desfecho**: média de `resolved_at − first_seen_at`, só para
   ocorrências com `resolved_at != null`, exibida em dias com uma casa decimal. É a
   pergunta "quanto tempo entre detecção e desfecho" do documento funcional, e é
   calculável hoje.
3. Granularidade **dia · semana · mês**, agrupando por `date`. "Ano" fica fora: não há
   volume de dado que justifique o eixo, e um gráfico anual com dois pontos desqualifica
   a tela.
4. Filtros: período, filial, tipo, desfecho, colaborador. **Setor e função ficam fora** —
   o dado não existe (seção 11).
5. Toda linha leva à ficha do colaborador (`/colaboradores/:secullumId`).

**Aceite:** um período de 30 dias mostra os quatro desfechos com soma igual ao total.

A quebra por filial usa a medida **Período**, que exclui as ignoradas — então o total dela
é legitimamente **menor** que o total do resumo. Nesse caso a invariante §10.4 se cumpre
mostrando a conta na tela: *"Total geral X = Y do resumo menos Z ignoradas"*. Dois totais
diferentes sem explicação são um bug; com a diferença explicitada, são duas perguntas
diferentes.

### 9.2 `/ranking` — Ranking

**Pergunta:** onde a exposição se concentra.

**Fonte:** a mesma consulta de ocorrências do período.

**Regras:**
1. Três abas: **Colaboradores · Filiais · Melhora**.
2. Pontuação pelos pesos da seção 2, na medida da seção 3, com a medida (Exposição /
   Período) visível e trocável — nunca implícita.
3. Cada linha mostra: pontuação, quantidade de críticas, de alertas e a **contagem
   operacional em coluna própria**, fora da pontuação.
4. **Melhora** compara o período selecionado com o período **imediatamente anterior de
   mesma duração**, usando a medida Período. Ordena por maior redução. Quem não aparece
   nos dois períodos fica de fora, com nota dizendo quantos foram excluídos por isso.
5. **Sem normalização por dias trabalhados.** O dado não existe (seção 11). A tela declara
   isso em nota fixa: *"Pontuação bruta — não ajustada por dias trabalhados."* Sem essa
   nota, o ranking mede presença e ninguém percebe.
6. O ranking individual é dado sensível. Cabeçalho traz a nota: *"Use para investigar
   causa, não para punir isoladamente."*

**Aceite:** um colaborador com 3 operacionais e nenhuma outra ocorrência tem pontuação 0 e
aparece com 3 na coluna operacional.

### 9.3 `/revisao-mensal` — Revisão mensal

**Pergunta:** o que falta para considerar esta competência encerrada, nesta filial.

**Fonte:** ocorrências da competência + `api.listReports` do mesmo intervalo.

**Regras:**
1. Recorte por **competência** (`YYYY-MM`) e **filial**, uma linha por filial mais a linha
   "Sem filial".
2. Quatro condições **verificáveis hoje**, cada uma com contagem e link para a lista
   filtrada:
   - Ocorrências em aberto (`aberta` + `atualizada`);
   - A reconferir (`atualizada`) — valor mudou depois da última leitura;
   - Operacionais em aberto — escala a confirmar antes de auditar;
   - **Dias sem varredura** na competência (dias do mês até hoje sem `Report`).
3. As condições manuais do documento funcional (folha processada, compensações lançadas)
   aparecem **desabilitadas**, com a nota *"aguardando backend"*. **Proibido** renderizar
   caixa de seleção que parece salvar e não salva.
4. **Não existe botão de encerrar.** A tela diagnostica; o ato explícito é backend.
5. Competência só é oferecida a partir do primeiro `Report` do tenant.

**Aceite:** uma competência com dias não varridos nunca aparece como pronta, mesmo com
zero ocorrências em aberto.

### 9.4 `/investigar` — Sinais operacionais

**Pergunta:** o que precisa ser apurado antes de virar auditoria.

**Fonte:** ocorrências com `severity === 'OPERACIONAL'` e estado `aberta` ou `atualizada`.

**Regras:**
1. Filtra por **severidade**, não por categoria: `category` vira `NAO_CONFIRMADA` quando o
   estado é `atualizada`, e essas ocorrências operacionais sumiriam da tela.
2. Texto de contexto fixo: provável troca de escala não comunicada ou operação
   deliberada não avisada ao RH — **apurar antes de auditar**.
3. Agrupamento por colaborador, com a contagem de dias afetados: três dias seguidos do
   mesmo colaborador é troca de escala, um dia isolado é exceção.
4. Ação disponível: `api.ignoreOccurrence(id, motivo)` — o único desfecho humano que
   existe. Rotulada **"Ignorar"**, com motivo **obrigatório na tela** (o backend aceita
   vazio; aqui não). Um sinal descartado sem motivo é um sinal perdido.
5. Nada de "marcar como investigado" ou "confirmar escala": esse desfecho não existe.

**Aceite:** uma ocorrência operacional em estado `atualizada` aparece na lista.

### 9.5 Ficha do colaborador — origem da marcação

`pages/ColaboradorHistorico.tsx` passa a consumir
`api.listPunchRecords(tenantId, secullumId, start, end)`, que existe no backend desde
26/08 e **nunca teve UI**. Mostra, por dia, de qual equipamento veio a marcação e o motivo
(inclusão manual, abono). Dia sem registro mostra "—", não some.

É o insumo da discussão de qualidade do dado de origem: marcação manual recorrente explica
ocorrência operacional melhor que qualquer gráfico.

---

## 10. Invariantes

Valem para as cinco entregas. Quebrar qualquer uma invalida a entrega.

1. `OPERACIONAL` não pontua e não é omitido.
2. `resolvida_manual` não é "tratada" e não pontua.
3. `filial: null` vira "Sem filial", nunca é descartada.
4. A soma das partes bate com o total exibido, em toda tela.
5. Filtro vive na URL.
6. Nenhuma cor de semáforo por meta inventada.
7. Nenhum controle que parece persistir e não persiste.
8. Nenhuma dependência nova; nenhum arquivo compartilhado editado.

---

## 11. Fora de escopo — pendências declaradas

| Pendência | Motivo |
|---|---|
| **Setor (departamento) e função** | ~~Não estão no payload da Secullum.~~ **Correção: sempre estiveram** (`Departamento`, `Funcao`, e endpoints de lista). Não eram lidos pelo cliente. Entram no ciclo do backend — ver `docs/12` §9. |
| **Empresa (CNPJ)** | Idem. Entra como campo e filtro, sem visão agregada própria. |
| **Dias trabalhados** | Derivável de `DailyPunch`. Normalização do ranking adiada **por decisão**, não por falta de dado. |
| **Desfecho "tratada"**, justificativa, anexo | Feature 4, backend. Entra no ciclo. |
| **Tratativa em lote** | Segunda etapa, depois do individual. |
| **Encerrar / reabrir competência** | Backend. Entra no ciclo, por **tenant + competência**. |
| **Isolamento por filial** | **Descartado deste ciclo.** Toda a staff vê todas as filiais. Nenhuma tela pode sugerir que filial é fronteira de segurança. |
| **Semáforo executivo** | Depende de metas cadastradas. Continua fora. |
| **Carga retroativa** | Continua fora. O histórico começa vazio. |
