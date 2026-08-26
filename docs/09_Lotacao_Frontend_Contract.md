# Lotação (colaborador ↔ filial) — camada de frontend

O backend de Filiais está descrito na seção 5 de `06_Occurrences_Backend_Contract.md`
(rotas, modelo, conflitos 409) e a resolução da filial na seção 3 do mesmo documento.
**Nada disso mudou nesta etapa** — nenhuma rota nova, nenhum campo novo, nenhuma migration.

Este documento cobre o que foi construído por cima: como o painel passou a lotar
colaboradores em filiais sem que ninguém precise digitar um número.

---

## 1. O problema

O vínculo colaborador↔filial não é uma coluna: é uma linha em `branch_payroll_numbers`,
em que a filial reivindica um **nº de folha**, e o backend casa esse número com
`collaborators.numero_folha` (ver `usecase/branch_resolver.go`).

Isso funciona no banco e falhava na interface, por um motivo simples:

> **`secullum_id` ≠ `numero_folha`**, e a UI chamava o primeiro de "nº".

A ficha do colaborador escrevia `nº 442` — que é o `secullum_id`. O campo da tela de
Filiais se chamava "Nº de folha". Cadastrar `442` ali era o movimento óbvio, e produzia um
vínculo que nunca resolvia: o `numero_folha` daquele colaborador era `47802`. O número
verdadeiro não aparecia em lugar nenhum do painel — `GET /collaborators` nem o devolve.

O sintoma era mudo: nenhum erro, nenhum 409, só um card "Filial" permanentemente vazio.

### Por que não bastava corrigir o rótulo

Nos dados reais da Tia Teca (481 colaboradores), o `numero_folha` está **sempre preenchido
e é sempre único** — serve como chave técnica. Mas o cliente usa o campo de forma
inconsistente:

| Tamanho | Quantos | Exemplo |
|---|---|---|
| 1–6 dígitos | 323 | `537`, `66`, `76997` |
| 11 dígitos (PIS) | 90 | `16128285059` |
| 14 dígitos | 14 | — |
| formatado | alguns | `75.791.336-95` |

Ou seja: mesmo com o rótulo certo, **digitar nº de folha à mão é um caminho ruim** — e
inviável para lotar centenas de pessoas. A conclusão de projeto é que o número existe para
a máquina casar, não para o humano transcrever.

---

## 2. Por que a solução é 100% frontend

Decisão de 2026-08-22: resolver **sem tocar no backend nem em migration**, para não
colidir com o contrato de papéis e permissões (`08`), que estava em aberto.

O que o front alcança hoje:

| Endpoint | Entrega |
|---|---|
| `GET /tenants/:id/collaborators` | `id`, `secullum_id`, `name` — **sem** nº de folha |
| `GET /tenants/:id/collaborators/:secullumId/prefill` | `numero_folha` ✅ + filial resolvida — **um colaborador por chamada** |
| `GET /tenants/:id/branches` | filiais **com** `payroll_numbers[{id, numero}]` ✅ |
| `POST /branches/:id/payroll-numbers` | cria vínculo ✅ |
| `DELETE /branches/:id/payroll-numbers/:pnId` | remove vínculo ✅ |

Tudo que faltava era o cruzamento `numero_folha ↔ nome`. O `prefill` entrega isso — só que
um por vez. Daí o módulo abaixo.

> **Alternativa registrada, não adotada:** `NumeroPis` e `NumeroIdentificador` aparecem em
> `data/get_batidas.json`, mas o parser de batidas (`secullumBatidaResponse`) lê apenas
> `FuncionarioId`, `Data` e os marcadores — **descarta o objeto `Funcionario` aninhado**.
> Nenhum dos dois é persistido nem exposto. Não são utilizáveis sem mexer no backend.

---

## 3. `frontend/src/lib/lotacao.ts`

Módulo único, consumido pelas duas telas. Traduz nº de folha em nome de gente.

### Índice

```ts
buildIndex(tenantId, onProgress?) : Promise<LotacaoIndex>
ensureIndex(tenantId, onProgress?) : Promise<LotacaoIndex>   // cache, ou constrói
readCachedIndex(tenantId) : LotacaoIndex | null              // não faz rede
clearCachedIndex(tenantId)
```

`buildIndex` varre o `prefill` de cada colaborador com **concorrência 6** (teto prático de
conexões por host do navegador) e monta:

- `bySecullumId: Map<number, CollaboratorEntry>`
- `byNumeroFolha: Map<string, CollaboratorEntry>` — o sentido que a tela de Filiais precisa
- `collaborators: CollaboratorEntry[]`

**Custo: ~30s para 481 colaboradores** no navegador. Cacheado em `sessionStorage` por
tenant e invalidado ao trocar de empresa. Um `prefill` que falha não derruba a varredura:
aquele colaborador entra sem `numeroFolha` e a UI o mostra como não-vinculável, que é a
verdade operacional.

O índice é carregado **sob demanda**, ao expandir uma filial — quem abre `/filiais` só para
corrigir um telefone não paga nada.

### Escrita

```ts
setFilial(numeroFolha, branchId | null, branches) : Promise<void>
setFilialEmLote(entries, branchId, branches, onProgress?) : Promise<BulkResult>
```

`setFilial` remove o vínculo anterior **antes** de criar o novo — na ordem inversa, o
índice único `(tenant, numero)` faria o `POST` devolver 409. Se o `POST` falhar, o vínculo
antigo é restaurado (best-effort).

`setFilialEmLote` roda **em série de propósito**: cada item pode custar duas chamadas sobre
o mesmo índice único, e paralelizar só multiplicaria conflito. Falha de um item não
interrompe os outros — devolve `{ ok, failures[] }`.

### Leitura auxiliar

```ts
findLink(branches, numeroFolha) : PayrollLink | null   // em que filial esse número está
branchMembers(branch, index)    : BranchMember[]       // quem está lotado numa filial
```

`branchMembers` devolve `collaborator: null` quando o número não corresponde a ninguém.
**Isso é intencional e precisa continuar visível na UI**: é exatamente o estado que o bug
original produziu, e esconder o órfão foi o que permitiu que ele passasse despercebido.

---

## 4. Os dois pontos de entrada

Ambos gravam o mesmo vínculo — a escolha é de conveniência, não de semântica.

**Ficha do colaborador** (`pages/ColaboradorHistorico.tsx`) — o card "Filial" é um
`<Select>` com as filiais + "Sem filial". **Não usa o índice**: a página já chama o
`prefill` da própria pessoa, então o `numero_folha` já está em mãos. Custo zero.

**Tela de filiais** (`pages/Filiais.tsx`) — o card expandido lista **nomes**, e o botão
"Adicionar colaboradores" abre um seletor com busca por nome, checkbox e a lotação atual de
cada um ("hoje em São Jacinto"), porque marcar alguém já lotado é uma **mudança** de
filial, não uma adição.

O campo de nº de folha avulso continua existindo, recolhido em `<details>` como
"avançado", com aviso explícito de que o número pedido não é o ID da ficha. Ele serve a
quem já tem a lista do RH em mãos.

---

## 5. Regra de nomenclatura (não regredir)

Os dois números têm nome próprio na interface, em toda tela que os exibe:

```
ID Secullum 442 · folha 47802
```

**Nunca voltar a chamar `secullum_id` de "nº"** — foi essa abreviação, ao lado de um campo
"Nº de folha", que produziu o bug. Aplicado em `ColaboradorHistorico.tsx` (cabeçalho) e
`Colaboradores.tsx` (linha da lista e placeholder de busca).

---

## 6. Limitações aceitas

- **Sem atomicidade na troca de filial.** São duas chamadas HTTP; se o `DELETE` passar e o
  `POST` falhar, a pessoa fica sem filial em vez de ficar na antiga. Há restauração
  best-effort, mas a janela existe. Um endpoint transacional no backend a fecharia.
- **Os ~30s da varredura.** Só na tela de Filiais, só na primeira abertura por sessão.
  **O conserto é de uma linha:** `GET /collaborators` devolver `numero_folha`. Enquanto não
  houver, o `prefill` em massa é o preço.
- **`numero_folha` vazio impede o vínculo.** Nos 481 atuais nenhum está vazio; a UI trata o
  caso com mensagem em vez de gravar string vazia (que casaria com qualquer outro).

---

## 7. Fora de escopo — dívidas conhecidas

1. **O gestor da filial não recebe nada.** `manager_name`/`manager_phone` são gravados, mas
   o `notification_consumer` envia apenas para `StaffID`. O telefone do gestor não é lido
   por nenhum código fora do CRUD — a promessa da tela ("é o que liga uma ocorrência a quem
   deve resolvê-la") ainda não existe no caminho de envio.
2. **Nenhum aparelho (`EquipId`) cadastrado.** O caminho de resolução mais forte está
   inerte, e por decisão de 2026-08-22 **fica assim por enquanto** — não interfere na
   operação, já que as batidas desta operação vêm majoritariamente do app/web, sem
   `EquipId`.
