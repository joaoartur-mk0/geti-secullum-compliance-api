# Aba "Indicadores" — Contrato de Métricas com o Backend

Este documento é o **pedido de backend** para a nova aba **Indicadores** do painel
(feita no frontend em `frontend/src/pages/Indicadores.tsx`). É a dashboard operacional
inspirada no painel manual do Elmer, mas alimentada automaticamente por cada varredura
do Compliance — sem importar PDF.

> Escopo desta fase: **apenas métricas operacionais** (horas, contagens, percentuais).
> Métricas financeiras do painel do Elmer (custo de HE em R$, adicional noturno, DSR)
> ficam **fora**, pois exigem salário/valor-hora por colaborador — dado que hoje não
> existe no domínio nem vem da API de batidas.

---

## 0. Endpoint de colaboradores (JÁ IMPLEMENTADO)

A aba mostra um painel **Colaboradores** (Sincronizados / Corretos / Com alertas). Para isso
foi adicionado um endpoint simples que expõe o espelho local de colaboradores (que o worker
`tenant.provisioning` já persiste, mas que nenhuma rota expunha):

```
GET /api/v1/tenants/:id/collaborators
→ { "collaborators": [{ "id", "secullum_id", "name" }], "total": N }
```

Implementado em `internal/interface/http/handlers/collaborator_handler.go` + rota no `router.go`,
reusando `CollaboratorRepository.GetByTenantID`. Não toca na lógica de auditoria/sync. O frontend
usa `total` como "Sincronizados"; "Com alertas" = colaboradores distintos com inconsistência na
última varredura; "Corretos" = total − com alertas. Quando o campo `metrics` (seção 2) existir com
`collaborators_audited`/`clean_count`, esse painel pode migrar para números baseados na auditoria.

> Nota: **não foi compilado localmente** (sem toolchain Go na máquina do Sergio; build é via
> Docker). Rodar `docker compose ... up --build` para validar.

---

## 1. Situação atual (o frontend já está no ar sem depender do back)

A aba **já funciona hoje** consumindo o endpoint que já existe:

```
GET /api/v1/tenants/:id/reports
```

Da lista de `inconsistencies` (de todos os relatórios), o frontend **deriva** —
**sem depender de nada novo do backend**:

- Total de inconsistências da última varredura, split **crítico / alerta**.
- **Distribuição por tipo**, com barras empilhadas por severidade (crít/alerta).
- **Evolução por varredura** (gráfico de tendência ao longo dos relatórios do período).
- **Top colaboradores** com mais ocorrências (usa o `CollaboratorName` já preenchido).
- Nº de **colaboradores afetados** (IDs distintos) e **nº de varreduras** no período.

Quando o backend enviar o objeto `metrics`, a aba passa a **liderar** com os indicadores
consolidados (**Índice de Conformidade**, **Horas Extras**, **Atrasos**, colaboradores
auditados) — que dependem de dados que só a varredura completa por colaborador produz.
Até lá, esses três não aparecem (a aba fica completa só com o que é derivado, sem
placeholders).

---

## 2. O que o backend precisa adicionar

Adicionar um objeto **`metrics`** a cada relatório retornado pelo endpoint acima.
Assim que o campo aparecer, a aba passa a exibir os cards ricos **automaticamente**
(o frontend já trata `report.metrics` como opcional — nada mais precisa mudar no front).

### 2.1 Formato JSON esperado (dentro de cada item de `reports`)

```jsonc
{
  "id": 12,
  "tenant_id": 1,
  "date": "2026-07-23",
  "data_generated": "2026-07-24T02:10:00Z",
  "total": 7,
  "inconsistencies": [ /* ...como hoje... */ ],

  "metrics": {
    "collaborators_audited": 42,          // nº de colaboradores avaliados na varredura
    "clean_count": 35,                    // colaboradores sem NENHUMA inconsistência
    "compliance_rate": 83.3,              // 0–100 = clean_count / collaborators_audited * 100
    "total_inconsistencies": 7,
    "critical": 5,                        // inconsistências de severidade CRITICO
    "alert": 2,                           // inconsistências de severidade ALERTA
    "by_type": {                          // contagem por tipo (alimenta o gráfico)
      "Batida Esquecida": 2,
      "Almoço Reduzido": 1,
      "Interjornada Curta": 1,
      "Hora Extra Excedente": 1,
      "Alerta de Hora Extra": 2
    },
    "overtime_hours_total": 6.5,          // soma das horas extras do dia (horas)
    "late_hours_total": 2.3               // soma dos atrasos do dia (horas)
  }
}
```

**Importante sobre nomes de campos:** usar **`snake_case`** (como `date`, `data_generated`,
`total` já usam no `reportResponse`). Os nomes acima são o contrato — o tipo TypeScript
correspondente está em `frontend/src/lib/types.ts` (`interface ReportMetrics`). As chaves
de `by_type` devem ser exatamente os `Type` das inconsistências já emitidas pelo auditor
(`"Batida Esquecida"`, `"Almoço Reduzido"`, `"Interjornada Curta"`, `"Hora Extra Excedente"`,
`"Alerta de Hora Extra"`), para casarem com a ordem canônica do gráfico.

> Alternativa: um endpoint dedicado `GET /api/v1/tenants/:id/metrics` com só o consolidado
> mais recente. **Não é necessário** — embutir `metrics` no relatório é suficiente e o
> frontend já está preparado para isso. Fica como opção caso queira evitar recalcular no read.

---

## 3. Onde isso encaixa no código (sugestão de implementação)

A ideia é o "cérebro" (`AuditorService`) passar a **reter os números que já calcula**
(hoje ele calcula horas trabalhadas/extras dentro de `checkOvertime`, mas só guarda a
infração em texto). Nada muda no comportamento das regras — só passamos a agregar métricas.

1. **`internal/domain/Audit.go`**
   - `CollaboratorDailyMetrics` — por colaborador/dia: `WorkedHours`, `OvertimeHours`,
     `LateHours`, `Presente bool`, `MissingPunches bool`, flags de almoço curto / interjornada.
   - `ReportMetrics` — consolidado do dia (os campos da seção 2.1).
   - `Report` ganha o campo `Metrics ReportMetrics`.

2. **`internal/usecase/auditor.go`**
   - Extrair o cálculo de horas (hoje dentro de `checkOvertime`) para um helper reutilizável.
   - `ProcessRules(...)` passa a retornar também um `CollaboratorDailyMetrics`.
     - `LateHours`  = max(0, cargaContratual − horasTrabalhadas) no dia completo.
     - `OvertimeHours` = max(0, horasTrabalhadas − cargaContratual) (já é a base do `checkOvertime`).
     - `Presente` = teve ao menos uma batida válida no dia (ou não foi abono/folga).

3. **`internal/usecase/aggregator.go` (novo)**
   - Acumula os `CollaboratorDailyMetrics` de todos os colaboradores em um `ReportMetrics`
     (`compliance_rate = clean_count / collaborators_audited * 100`, somatórios, `by_type`).

4. **`internal/infrastructure/messaging/consumer.go`**
   - Hoje está **mockado** (`collab := &domain.Collaborator{ID: 1}` e só a 1ª batida).
     Quando o loop real por colaborador entrar (o fix da importação de batidas), acumular
     as métricas via aggregator e gravar em `report.Metrics`.

5. **Persistência**
   - `internal/infrastructure/database/models/Report.go`: coluna nova `metrics jsonb`
     (AutoMigrate cria; **retrocompatível** — relatórios antigos ficam sem `metrics`, e o
     frontend cai no modo derivado para esses).
   - `internal/infrastructure/database/repositories/report_repository.go`: `Save`/`ListByTenant`
     passam a serializar/desserializar o novo campo.

6. **API**
   - `internal/interface/http/handlers/report_handler.go`: incluir `Metrics` no `reportResponse`.
   - Atualizar `internal/interface/http/swagger/openapi.yaml`.

---

## 4. Fora de escopo (registrado para o futuro)

- **Valores em R$** (custo de HE, adicional noturno, DSR): exigem cadastro de salário/valor-hora
  por colaborador. Quando existir, viram novos campos em `metrics` e novos cards na aba.
- **Tabela diária por colaborador**, banco de horas, pré-fechamento mensal e módulo disciplinar
  do painel do Elmer: entregas posteriores.

---

## 5. Como testar o casamento front ↔ back

O frontend já foi validado nos dois modos (derivado e com `metrics`). Para conferir que o
back está devolvendo no formato certo, basta o endpoint de relatórios trazer o `metrics`
como na seção 2.1 — a aba Indicadores acende os cards de Conformidade / Horas Extras /
Atrasos e o aviso amarelo some.
