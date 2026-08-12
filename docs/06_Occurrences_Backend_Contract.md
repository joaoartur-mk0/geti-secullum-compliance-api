# Ocorrências, Filiais e Advertências — contrato para o frontend

Este documento descreve o backend que acabou de entrar e que a próxima rodada de frontend
vai consumir (itens da seção **Frontend** de `roadmap-tecnico.md`). A referência formal
das rotas está em `backend/internal/interface/http/swagger/openapi.yaml` (Swagger UI em
`/swagger`); aqui fica o **porquê** de cada coisa e o que muda para o painel.

---

## O que mudou no modelo mental

Até agora, cada varredura gravava um **relatório novo** com a lista inteira de
inconsistências. Rodar a auditoria duas vezes no mesmo dia produzia duas cópias da mesma
infração, e não havia como dizer se algo era novo, se tinha piorado ou se já estava
resolvido.

Agora existe a **ocorrência**: uma inconsistência com identidade estável
(`tenant + colaborador + data + tipo`) e um **estado** que evolui a cada varredura. O que
muda entre auditorias é o estado, não a quantidade de linhas.

**`GET /reports` continua existindo e funcionando igual** — é o registro da execução de
cada varredura, e é dele que saem o gráfico de evolução e o histórico atual do
colaborador. Nada no painel quebrou. A migração para ocorrências pode ser feita tela a
tela, sem pressa.

### Os quatro estados

| Estado | Significa | Como se chega |
|---|---|---|
| `aberta` | apurada pela primeira vez, ainda presente | primeira varredura que a detecta |
| `atualizada` | continua existindo, mas **o valor mudou** desde a última varredura | ex.: o intervalo era de 43min e passou a 20min |
| `resolvida_automatica` | deixou de ser apurada — a batida ou a escala foi corrigida na Secullum | some da apuração |
| `resolvida_manual` | um usuário decidiu ignorar | `PATCH /occurrences/:id/ignore` |

Duas regras que importam para a UI:

- **Sync repetido não duplica e não vira novidade.** Auditar o mesmo dia cinco vezes deixa
  uma ocorrência com `times_seen: 5`, no mesmo estado, sem nenhum evento novo no log.
- **Ignorar é pegajoso.** A varredura seguinte continua apurando a inconsistência, mas não
  reabre a ocorrência. Ignorar vale para aquele dia — no dia seguinte, a mesma infração é
  uma ocorrência nova (a identidade inclui a data).

Toda transição fica registrada em `GET /occurrences/:id/events` (log append-only, com
descrição antes/depois, motivo e autor).

---

## 1. Listagem de ocorrências

```
GET /api/v1/tenants/:id/occurrences
    ?date=2026-07-12                 # dia único
    ?start_date=…&end_date=…         # intervalo
    ?state=aberta,atualizada         # padrão: aberta,atualizada
    ?collaborator_id=9               # id na Secullum
    ?branch_id=3
```

**Sem `state`, vêm só `aberta` e `atualizada`** — o que ainda pede ação. Devolver tudo por
omissão traria de volta o ruído que a máquina de estados veio eliminar; para telas de
histórico, peça os estados explicitamente.

Cada item já vem com `horario_fixo` e `filial` preenchidos (ver §3) — não é preciso uma
segunda chamada por linha.

---

## 2. Categorias e cores (UI/UX 2 do roadmap)

Cada ocorrência traz um campo **`category`**, que é o eixo de exibição — diferente de
`severity`, que é o eixo jurídico:

| `category` | Quando | Sugestão de leitura |
|---|---|---|
| `CRITICO` | severidade CRITICO | infração grave, ação imediata |
| `ALERTA` | severidade ALERTA | atenção preventiva |
| `ALTERACAO_ESCALA` | severidade `OPERACIONAL` | **cor própria** — não é infração, é cadastro desatualizado |
| `NAO_CONFIRMADA` | estado `atualizada` | **cor própria** — o valor mudou; o que o gestor viu antes não vale mais |

> **Mudança de comportamento a conhecer:** `Trabalho em Dia de Folga/DSR` e
> `Carga Horária Não Apurada` **deixaram de nascer CRÍTICOS** e passaram a nascer com a
> nova severidade `OPERACIONAL`. Na escala mensal variável desta operação, o caso comum é
> o gestor trocar colaboradores de dia sem atualizar a Secullum: é cadastro desatualizado,
> não infração da CLT. Corrigida a escala, a ocorrência some da apuração e vira
> `resolvida_automatica` sozinha — sem ninguém clicar em nada.

O tipo `Severity` do frontend (`frontend/src/lib/types.ts`) precisa ganhar `'OPERACIONAL'`.

---

## 3. Autopreenchimento: horário fixo e filial

```
GET /api/v1/tenants/:id/collaborators/:secullumId/prefill?date=2026-07-12
```

```jsonc
{
  "collaborator": { "id": 4, "secullum_id": 9, "name": "…", "numero_folha": "9" },
  "horario_fixo": [ { "dia_semana": 0, "entrada_1": "08:00", "…": "…", "carga_minutos": 480 } ],
  "filial": {
    "id": 3, "name": "Filial Centro",
    "manager_name": "…", "manager_phone": "5531999999999",
    "source": "aparelho"          // ou "numero_folha"
  }
}
```

- `horario_fixo` sai do espelho já sincronizado — sem chamada externa, é rápido.
- `filial` pode vir **`null`**: aparelho ainda não cadastrado e colaborador sem lotação são
  situações normais. Trate como campo a preencher, não como erro.
- **`source` importa na UI.** `aparelho` significa que a batida daquele dia veio de um
  relógio daquela filial (forte: a pessoa esteve lá). `numero_folha` é o cadastro de
  lotação. Vale sinalizar visualmente antes de emitir uma advertência em nome da filial.
- O `date` é opcional e serve só para tentar a resolução pelo aparelho. Nos dados reais
  desta operação a maioria das batidas vem do app/web e chega **sem** `EquipId`, então o
  caminho normal acaba sendo o nº de folha.

---

## 4. Ignorar ocorrência

```
PATCH /api/v1/occurrences/:occurrenceId/ignore
{ "reason": "Abonado pelo RH — atestado entregue" }   // corpo opcional
```

Devolve `{ "message": …, "state": "resolvida_manual" }`. O motivo e o usuário autor (do
JWT) ficam no log de eventos — é uma decisão auditável, não um "delete".

---

## 5. Filiais

```
GET  /api/v1/tenants/:id/branches      POST /api/v1/tenants/:id/branches
GET  /api/v1/branches/:branchId        PUT  …        DELETE …
POST /api/v1/branches/:branchId/devices                 DELETE …/devices/:deviceId
POST /api/v1/branches/:branchId/payroll-numbers         DELETE …/payroll-numbers/:id
```

Modelo: **filial 1—N aparelhos**, **filial 1—1 gestor** (`manager_name`/`manager_phone`,
inline na filial), e uma lista de **nº de folha** lotados nela.

Dois conflitos que a UI precisa tratar (ambos devolvem **409**, com mensagem pronta para
exibir):

- vincular um aparelho que já pertence a outra filial;
- lotar um nº de folha que já pertence a outra filial.

---

## 6. Advertências

```
POST  /api/v1/tenants/:id/warnings          GET /api/v1/tenants/:id/warnings?collaborator_id=&status=
GET   /api/v1/warnings/:warningId           PUT /api/v1/warnings/:warningId
PATCH /api/v1/warnings/:warningId/status    DELETE /api/v1/warnings/:warningId
```

O fluxo é **de mão única**: `draft → enviada → assinada`. Repetir o status atual é
idempotente; qualquer outro salto (pular a entrega, "desassinar") devolve **400**. Sem hash
de documento por enquanto, conforme o roadmap.

Editar o texto e excluir só valem em `draft` — depois de entregue, o texto é o que o
colaborador recebeu, e é isso que dá valor ao registro (**409** se tentar).

Para o indicador de **enviadas x confirmadas** (UI/UX 3), o `GET` de listagem já devolve
`counts: { draft, enviada, assinada }` — não é preciso recontar no cliente (a lista pode
vir filtrada).

---

## 7. Auditoria de um dia específico

```
POST /api/v1/audit/trigger
{ "tenant_id": 1, "date": "2026-07-12" }   // date é opcional
```

A **varredura diária automática continua exatamente como era**. O `date` é um extra: deixa
o gestor conferir a situação de um dia passado sem esperar a próxima varredura. Só datas
já encerradas (anteriores a hoje) são aceitas — auditar o dia corrente como fechamento
daria falso positivo em toda jornada em andamento. Omitido, audita D-1.

A resposta (202) devolve a data efetivamente enfileirada.

---

## 8. Notificação de WhatsApp

O resumo enviado aos gestores passou a reportar **o que mudou** (novas, atualizadas,
reabertas, resolvidas) em vez de reimprimir a lista inteira a cada varredura. Uma
auditoria sem novidade manda um aviso curto de "nenhuma novidade" em vez de repetir
dezenas de linhas já conhecidas.
