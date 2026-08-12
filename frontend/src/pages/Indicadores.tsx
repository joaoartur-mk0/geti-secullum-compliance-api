import {
  BarChart3,
  CalendarDays,
  ChevronRight,
  Clock,
  FileWarning,
  Layers,
  OctagonAlert,
  RefreshCw,
  ShieldCheck,
  TrendingUp,
  Users,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import type { ReactNode } from 'react'
import {
  CategoryBadge,
  EmptyState,
  ErrorNote,
  Select,
  SeverityBadge,
  Skeleton,
  WarningStatusBadge,
} from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { CATEGORY_ORDER } from '../lib/categories'
import { formatDate, formatDateTime } from '../lib/format'
import type {
  AuditInconsistency,
  Branch,
  Occurrence,
  OccurrenceCategory,
  Report,
  ReportMetrics,
  WarningCounts,
} from '../lib/types'

type Loadable<T> =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: T }

// Ordem canônica dos tipos de inconsistência, para o gráfico manter um eixo estável
// (não reordena a cada varredura). Tipos novos ainda não previstos entram ao final.
const TYPE_ORDER = [
  'Batida Esquecida',
  'Almoço Reduzido',
  'Interjornada Curta',
  'Hora Extra Excedente',
  'Alerta de Hora Extra',
]

// Nº máximo de varreduras exibidas no gráfico de tendência.
const TREND_LIMIT = 12

const nf = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 })

// shortDate transforma "2026-07-23" em "23/07" para os rótulos do eixo de tendência.
function shortDate(isoDate: string): string {
  const [, m, d] = isoDate.slice(0, 10).split('-')
  return d && m ? `${d}/${m}` : isoDate
}

interface DerivedMetrics {
  total: number
  critical: number
  alert: number
  byType: Record<string, { critical: number; alert: number }>
  affectedCollaborators: number
}

// derive reconstrói o que é possível apenas a partir da lista de inconsistências —
// contagens por severidade e por tipo, e colaboradores afetados. É o que alimenta a
// aba com os dados que o backend já entrega hoje.
function derive(report: Report): DerivedMetrics {
  const items = report.inconsistencies ?? []
  const byType: Record<string, { critical: number; alert: number }> = {}
  const collaborators = new Set<number>()
  let critical = 0
  for (const item of items) {
    const bucket = (byType[item.Type] ??= { critical: 0, alert: 0 })
    if (item.Severity === 'CRITICO') {
      bucket.critical++
      critical++
    } else {
      bucket.alert++
    }
    collaborators.add(item.CollaboratorID)
  }
  return {
    total: items.length,
    critical,
    alert: items.length - critical,
    byType,
    affectedCollaborators: collaborators.size,
  }
}

export default function Indicadores() {
  const { tenant } = useTenant()
  const [reports, setReports] = useState<Loadable<Report[]>>({ phase: 'loading' })
  // Total de colaboradores sincronizados (GET /collaborators). Secundário: uma falha aqui
  // não derruba os indicadores — só oculta o painel de equipe. null = desconhecido/erro.
  const [syncedTotal, setSyncedTotal] = useState<number | null>(null)

  const load = useCallback(async () => {
    setReports({ phase: 'loading' })
    setSyncedTotal(null)
    // Colaboradores em paralelo, sem bloquear nem derrubar os relatórios.
    api
      .listCollaborators(tenant.id)
      .then((r) => setSyncedTotal(r.total))
      .catch(() => setSyncedTotal(null))
    try {
      setReports({ phase: 'ready', data: await api.listReports(tenant.id) })
    } catch (error) {
      setReports({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar indicadores.',
      })
    }
  }, [tenant.id])

  useEffect(() => {
    void load()
  }, [load])

  const latest = reports.phase === 'ready' && reports.data.length > 0 ? reports.data[0] : null

  return (
    <div className="animate-rise">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Indicadores</h1>
          <p className="mt-1 text-sm text-ink-soft">
            {latest ? (
              <>
                Varredura de <strong className="font-semibold text-ink">{formatDate(latest.date)}</strong>
                {' · '}gerada {formatDateTime(latest.data_generated)}
              </>
            ) : (
              'Visão gerencial das varreduras de compliance da sua equipe.'
            )}
          </p>
        </div>
        <button
          type="button"
          onClick={load}
          className="flex min-h-11 items-center gap-1.5 rounded-field px-2.5 text-sm font-medium text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink"
        >
          <RefreshCw size={15} aria-hidden />
          Atualizar
        </button>
      </header>

      {reports.phase === 'loading' && (
        <div className="mt-8 flex flex-col gap-6">
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-28 w-full" />
            ))}
          </div>
          <Skeleton className="h-64 w-full" />
          <Skeleton className="h-56 w-full" />
        </div>
      )}

      {reports.phase === 'error' && (
        <div className="mt-8">
          <ErrorNote message={reports.message} onRetry={load} />
        </div>
      )}

      {reports.phase === 'ready' && !latest && (
        <div className="mt-8">
          <EmptyState
            icon={<BarChart3 size={32} strokeWidth={1.5} />}
            title="Nenhuma varredura ainda"
            description="Os indicadores são calculados a partir das varreduras de compliance. Dispare uma auditoria no Painel — o resultado aparece aqui."
            action={
              <Link
                to="/"
                className="text-sm font-semibold text-brand underline underline-offset-2 hover:text-brand-strong"
              >
                Ir para o Painel
              </Link>
            }
          />
        </div>
      )}

      {reports.phase === 'ready' && latest && (
        <Dashboard reports={reports.data} latest={latest} syncedTotal={syncedTotal} />
      )}
    </div>
  )
}

function Dashboard({
  reports,
  latest,
  syncedTotal,
}: {
  reports: Report[]
  latest: Report
  syncedTotal: number | null
}) {
  const d = useMemo(() => derive(latest), [latest])
  const m: ReportMetrics | null = latest.metrics ?? null

  // Série cronológica (o backend devolve do mais recente ao mais antigo) para a tendência.
  const series = useMemo(
    () =>
      reports
        .slice(0, TREND_LIMIT)
        .map((r) => {
          const dm = derive(r)
          return { date: r.date, total: dm.total, critical: dm.critical, alert: dm.alert }
        })
        .reverse(),
    [reports],
  )

  return (
    <div className="mt-8 flex flex-col gap-6">
      <CollaboratorsSummary syncedTotal={syncedTotal} withAlerts={d.affectedCollaborators} />
      <KpiRow derived={d} metrics={m} reportCount={reports.length} />
      <OccurrencesByCategory />
      <WarningsSummary />
      <DistributionChart byType={d.byType} total={d.total} />
      <TrendChart series={series} />
      <TopCollaborators inconsistencies={latest.inconsistencies ?? []} />
    </div>
  )
}

// ---------- Ocorrências em aberto por categoria (UI/UX 1 e 4) ----------
//
// Diferente do restante do painel (derivado de `reports`), esta seção consome
// GET /occurrences diretamente: é o que carrega a categoria (CRITICO/ALERTA/
// ALTERACAO_ESCALA/NAO_CONFIRMADA) e o filtro por filial, que a lista de inconsistências
// por relatório não tem.

type OccLoadable =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; occurrences: Occurrence[] }

function OccurrencesByCategory() {
  const { tenant } = useTenant()
  const [branches, setBranches] = useState<Branch[]>([])
  const [branchId, setBranchId] = useState<number | null>(null)
  const [state, setState] = useState<OccLoadable>({ phase: 'loading' })
  const [expanded, setExpanded] = useState<OccurrenceCategory | null>(null)

  useEffect(() => {
    api.listBranches(tenant.id).then(setBranches).catch(() => setBranches([]))
  }, [tenant.id])

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      const { occurrences } = await api.listOccurrences(tenant.id, {
        branch_id: branchId ?? undefined,
      })
      setState({ phase: 'ready', occurrences })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar ocorrências.',
      })
    }
  }, [tenant.id, branchId])

  useEffect(() => {
    void load()
  }, [load])

  const byCategory = useMemo(() => {
    const map = new Map<OccurrenceCategory, Occurrence[]>()
    if (state.phase === 'ready') {
      for (const occ of state.occurrences) {
        const list = map.get(occ.category) ?? []
        list.push(occ)
        map.set(occ.category, list)
      }
    }
    return map
  }, [state])

  const total = state.phase === 'ready' ? state.occurrences.length : 0

  return (
    <section
      aria-label="Ocorrências em aberto por categoria"
      className="rounded-card border border-line bg-bg p-5 shadow-card"
    >
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Layers size={16} className="text-ink-faint" aria-hidden />
          <h2 className="text-sm font-semibold text-ink">Em aberto por categoria</h2>
        </div>
        {branches.length > 0 && (
          <Select
            aria-label="Filtrar por filial"
            value={branchId ?? ''}
            onChange={(e) => setBranchId(e.target.value ? Number(e.target.value) : null)}
            className="min-w-40"
          >
            <option value="">Todas as filiais</option>
            {branches.map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </Select>
        )}
      </div>

      {state.phase === 'loading' && <Skeleton className="h-24 w-full" />}
      {state.phase === 'error' && <ErrorNote message={state.message} onRetry={load} />}

      {state.phase === 'ready' && total === 0 && (
        <p className="py-6 text-center text-sm text-ink-soft">
          {branchId ? 'Nenhuma ocorrência em aberto nesta filial.' : 'Nenhuma ocorrência em aberto — tudo em conformidade.'}
        </p>
      )}

      {state.phase === 'ready' && total > 0 && (
        <div className="flex flex-col gap-2">
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            {CATEGORY_ORDER.map((category) => {
              const items = byCategory.get(category) ?? []
              return (
                <button
                  key={category}
                  type="button"
                  disabled={items.length === 0}
                  onClick={() => setExpanded((c) => (c === category ? null : category))}
                  className={`flex flex-col gap-1 rounded-card border p-3 text-left transition-colors duration-150 disabled:cursor-default disabled:opacity-50 ${
                    expanded === category ? 'border-brand' : 'border-line hover:border-ink-faint'
                  }`}
                >
                  <CategoryBadge category={category} />
                  <span className="text-xl font-semibold tabular-nums text-ink">{items.length}</span>
                </button>
              )
            })}
          </div>

          {expanded && (byCategory.get(expanded)?.length ?? 0) > 0 && (
            <ul className="mt-2 divide-y divide-line rounded-card border border-line">
              {byCategory.get(expanded)!.map((occ) => (
                <li key={occ.id}>
                  <Link
                    to={`/colaboradores/${occ.collaborator_id}`}
                    className="flex items-center gap-3 px-3 py-2.5 transition-colors duration-150 hover:bg-panel"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-ink">{occ.collaborator_name}</p>
                      <p className="truncate text-xs text-ink-soft">{occ.type}</p>
                    </div>
                    <ChevronRight size={16} aria-hidden className="shrink-0 text-ink-faint" />
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </section>
  )
}

// ---------- Advertências enviadas x assinadas (UI/UX 3) ----------

function WarningsSummary() {
  const { tenant } = useTenant()
  const [counts, setCounts] = useState<WarningCounts | null>(null)

  useEffect(() => {
    api
      .listWarnings(tenant.id)
      .then((r) => setCounts(r.counts))
      .catch(() => setCounts(null))
  }, [tenant.id])

  if (!counts) return null
  const total = counts.draft + counts.enviada + counts.assinada
  if (total === 0) return null

  return (
    <section aria-label="Advertências" className="rounded-card border border-line bg-bg p-5 shadow-card">
      <div className="mb-4 flex items-center gap-2">
        <FileWarning size={16} className="text-ink-faint" aria-hidden />
        <h2 className="text-sm font-semibold text-ink">Advertências</h2>
      </div>
      <div className="flex flex-wrap items-center gap-4">
        <WarningCount label="Rascunho" value={counts.draft} status="draft" />
        <WarningCount label="Enviadas" value={counts.enviada} status="enviada" />
        <WarningCount label="Assinadas" value={counts.assinada} status="assinada" />
      </div>
    </section>
  )
}

function WarningCount({
  label,
  value,
  status,
}: {
  label: string
  value: number
  status: 'draft' | 'enviada' | 'assinada'
}) {
  return (
    <div className="flex items-center gap-2">
      <WarningStatusBadge status={status} />
      <span className="text-sm text-ink-soft">
        <span className="font-semibold tabular-nums text-ink">{value}</span> {label.toLowerCase()}
      </span>
    </div>
  )
}

// ---------- KPIs ----------

function KpiRow({
  derived,
  metrics,
  reportCount,
}: {
  derived: DerivedMetrics
  metrics: ReportMetrics | null
  reportCount: number
}) {
  // Modo rico: o backend já envia métricas consolidadas (conformidade, horas). Nesse
  // caso lideramos com elas. Modo derivado (hoje): mostramos o que sai das inconsistências.
  if (metrics) {
    return (
      <section aria-label="Indicadores consolidados" className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        <Kpi
          icon={<ShieldCheck size={17} aria-hidden />}
          label="Conformidade"
          value={`${nf.format(metrics.compliance_rate)}%`}
          hint="colaboradores sem ocorrência"
          tone={metrics.compliance_rate >= 90 ? 'ok' : metrics.compliance_rate >= 70 ? 'alerta' : 'critico'}
        />
        <Kpi
          icon={<TrendingUp size={17} aria-hidden />}
          label="Inconsistências"
          value={String(metrics.total_inconsistencies)}
          hint={`${metrics.critical} crítica${metrics.critical === 1 ? '' : 's'} · ${metrics.alert} alerta${metrics.alert === 1 ? '' : 's'}`}
          tone={metrics.total_inconsistencies === 0 ? 'ok' : metrics.critical > 0 ? 'critico' : 'alerta'}
        />
        <Kpi
          icon={<Users size={17} aria-hidden />}
          label="Colaboradores"
          value={String(metrics.collaborators_audited)}
          hint="auditados na varredura"
          tone="neutral"
        />
        <Kpi
          icon={<Clock size={17} aria-hidden />}
          label="Horas extras"
          value={`${nf.format(metrics.overtime_hours_total)}h`}
          hint="total do dia"
          tone={metrics.overtime_hours_total > 0 ? 'alerta' : 'ok'}
        />
        <Kpi
          icon={<Clock size={17} aria-hidden />}
          label="Atrasos"
          value={`${nf.format(metrics.late_hours_total)}h`}
          hint="total do dia"
          tone={metrics.late_hours_total > 0 ? 'alerta' : 'ok'}
        />
      </section>
    )
  }

  return (
    <section aria-label="Indicadores da última varredura" className="grid grid-cols-2 gap-3 lg:grid-cols-3">
      <Kpi
        icon={<TrendingUp size={17} aria-hidden />}
        label="Inconsistências"
        value={String(derived.total)}
        hint={
          derived.total === 0
            ? 'dia em conformidade'
            : `${derived.alert} alerta${derived.alert === 1 ? '' : 's'} + ${derived.critical} crítica${derived.critical === 1 ? '' : 's'}`
        }
        tone={derived.total === 0 ? 'ok' : derived.critical > 0 ? 'critico' : 'alerta'}
      />
      <Kpi
        icon={<OctagonAlert size={17} aria-hidden />}
        label="Críticas"
        value={String(derived.critical)}
        hint={derived.critical === 0 ? 'nenhuma ação urgente' : 'exigem ação imediata'}
        tone={derived.critical > 0 ? 'critico' : 'ok'}
      />
      <Kpi
        icon={<CalendarDays size={17} aria-hidden />}
        label="Varreduras"
        value={String(reportCount)}
        hint="no histórico do período"
        tone="neutral"
      />
    </section>
  )
}

// ---------- Resumo de colaboradores (sincronizados / corretos / com alertas) ----------

function CollaboratorsSummary({
  syncedTotal,
  withAlerts,
}: {
  syncedTotal: number | null
  withAlerts: number
}) {
  // "Corretos" = sincronizados sem ocorrência na última varredura. Clamp em 0 para o caso
  // (raro) de o relatório citar um colaborador que já saiu do espelho sincronizado.
  const correct = syncedTotal != null ? Math.max(0, syncedTotal - withAlerts) : null

  return (
    <section aria-label="Colaboradores sob auditoria">
      <div className="mb-3 flex items-center gap-2">
        <Users size={16} className="text-ink-faint" aria-hidden />
        <h2 className="text-sm font-semibold text-ink">Colaboradores</h2>
      </div>

      {syncedTotal == null ? (
        <div className="rounded-card border border-line bg-bg p-4 text-sm text-ink-soft shadow-card">
          Não foi possível carregar os colaboradores sincronizados. Verifique a sincronização em
          Empresa e tente novamente.
        </div>
      ) : syncedTotal === 0 ? (
        <div className="rounded-card border border-line bg-bg p-4 text-sm text-ink-soft shadow-card">
          Nenhum colaborador sincronizado ainda. A sincronização com a Secullum roda ao cadastrar a
          empresa; dispare novamente em Empresa se necessário.
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
          <Kpi
            icon={<Users size={17} aria-hidden />}
            label="Sincronizados"
            value={String(syncedTotal)}
            hint="funcionários sob auditoria"
            tone="neutral"
          />
          <Kpi
            icon={<ShieldCheck size={17} aria-hidden />}
            label="Corretos"
            value={String(correct)}
            hint="sem ocorrência na varredura"
            tone={correct === syncedTotal ? 'ok' : 'neutral'}
          />
          <Kpi
            icon={<OctagonAlert size={17} aria-hidden />}
            label="Com alertas"
            value={String(withAlerts)}
            hint="com ao menos uma ocorrência"
            tone={withAlerts > 0 ? 'alerta' : 'ok'}
          />
        </div>
      )}
    </section>
  )
}

type Tone = 'neutral' | 'ok' | 'alerta' | 'critico'

const toneValue: Record<Tone, string> = {
  neutral: 'text-ink',
  ok: 'text-ok',
  alerta: 'text-alerta',
  critico: 'text-critico',
}

function Kpi({
  icon,
  label,
  value,
  hint,
  tone,
}: {
  icon: ReactNode
  label: string
  value: string
  hint: string
  tone: Tone
}) {
  return (
    <div className="flex flex-col gap-2 rounded-card border border-line bg-bg p-4 shadow-card">
      <div className="flex items-center gap-1.5 text-ink-faint">
        {icon}
        <span className="text-xs font-semibold uppercase tracking-wide">{label}</span>
      </div>
      <span className={`text-2xl font-semibold tabular-nums ${toneValue[tone]}`}>{value}</span>
      <span className="text-xs text-ink-soft">{hint}</span>
    </div>
  )
}

// ---------- Legenda de severidade ----------

function SeverityLegend() {
  return (
    <div className="flex items-center gap-4 text-xs text-ink-soft">
      <span className="flex items-center gap-1.5">
        <span className="h-2.5 w-2.5 rounded-sm bg-critico" aria-hidden />
        Crítico
      </span>
      <span className="flex items-center gap-1.5">
        <span className="h-2.5 w-2.5 rounded-sm bg-alerta" aria-hidden />
        Alerta
      </span>
    </div>
  )
}

// ---------- Gráfico: distribuição por tipo (empilhado por severidade) ----------

function DistributionChart({
  byType,
  total,
}: {
  byType: Record<string, { critical: number; alert: number }>
  total: number
}) {
  const known = TYPE_ORDER.filter((t) => byType[t] != null)
  const extras = Object.keys(byType).filter((t) => !TYPE_ORDER.includes(t))
  const rows = [...known, ...extras].map((type) => {
    const b = byType[type] ?? { critical: 0, alert: 0 }
    return { type, critical: b.critical, alert: b.alert, count: b.critical + b.alert }
  })
  const max = rows.reduce((m, r) => Math.max(m, r.count), 0)

  return (
    <section
      aria-label="Distribuição de inconsistências por tipo"
      className="rounded-card border border-line bg-bg p-5 shadow-card"
    >
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <BarChart3 size={16} className="text-ink-faint" aria-hidden />
          <h2 className="text-sm font-semibold text-ink">Inconsistências por tipo</h2>
        </div>
        {total > 0 && <SeverityLegend />}
      </div>

      {total === 0 || rows.length === 0 ? (
        <p className="py-6 text-center text-sm text-ink-soft">
          Nenhuma inconsistência nesta varredura — todas as batidas em conformidade.
        </p>
      ) : (
        <ul className="flex flex-col gap-3">
          {rows.map(({ type, critical, alert, count }) => {
            const pct = total > 0 ? Math.round((count / total) * 100) : 0
            const widthPct = max > 0 ? (count / max) * 100 : 0
            const critShare = count > 0 ? (critical / count) * 100 : 0
            return (
              <li key={type} className="flex flex-col gap-1.5">
                <div className="flex items-baseline justify-between gap-3 text-sm">
                  <span className="font-medium text-ink">{type}</span>
                  <span className="shrink-0 tabular-nums text-ink-soft">
                    {count} <span className="text-ink-faint">({pct}%)</span>
                  </span>
                </div>
                <div className="h-2.5 w-full overflow-hidden rounded-full bg-panel" aria-hidden>
                  <div className="flex h-full transition-[width] duration-500 ease-out" style={{ width: `${widthPct}%` }}>
                    {critical > 0 && <div className="h-full bg-critico" style={{ width: `${critShare}%` }} />}
                    {alert > 0 && <div className="h-full bg-alerta" style={{ width: `${100 - critShare}%` }} />}
                  </div>
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

// ---------- Gráfico: tendência ao longo das varreduras ----------

function TrendChart({ series }: { series: { date: string; total: number; critical: number; alert: number }[] }) {
  const max = series.reduce((m, s) => Math.max(m, s.total), 0)

  return (
    <section
      aria-label="Tendência de inconsistências por varredura"
      className="rounded-card border border-line bg-bg p-5 shadow-card"
    >
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <TrendingUp size={16} className="text-ink-faint" aria-hidden />
          <h2 className="text-sm font-semibold text-ink">Evolução por varredura</h2>
        </div>
        {series.length > 1 && max > 0 && <SeverityLegend />}
      </div>

      {series.length < 2 ? (
        <p className="py-6 text-center text-sm text-ink-soft">
          A tendência aparece a partir de duas varreduras. Continue auditando para acompanhar a evolução.
        </p>
      ) : (
        <div className="flex h-40 items-stretch gap-1.5">
          {series.map((s) => {
            const heightPct = max > 0 ? (s.total / max) * 100 : 0
            const critShare = s.total > 0 ? (s.critical / s.total) * 100 : 0
            return (
              <div
                key={s.date}
                className="flex min-w-0 flex-1 flex-col items-center gap-1.5"
                title={`${formatDate(s.date)} — ${s.total} inconsistência(s): ${s.critical} crítica(s), ${s.alert} alerta(s)`}
              >
                <div className="flex w-full flex-1 items-end justify-center">
                  {s.total === 0 ? (
                    <div className="h-0.5 w-full rounded-full bg-line" aria-hidden />
                  ) : (
                    <div
                      className="flex w-full max-w-8 flex-col overflow-hidden rounded-t-sm transition-[height] duration-500 ease-out"
                      style={{ height: `${Math.max(heightPct, 4)}%` }}
                    >
                      {s.critical > 0 && <div className="w-full bg-critico" style={{ height: `${critShare}%` }} />}
                      {s.alert > 0 && <div className="w-full bg-alerta" style={{ height: `${100 - critShare}%` }} />}
                    </div>
                  )}
                </div>
                <span className="w-full truncate text-center text-[10px] tabular-nums text-ink-faint">
                  {shortDate(s.date)}
                </span>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}

// ---------- Top colaboradores com ocorrências ----------

function TopCollaborators({ inconsistencies }: { inconsistencies: AuditInconsistency[] }) {
  const ranked = useMemo(() => {
    const byCollab = new Map<number, { id: number; name: string; count: number; critical: number }>()
    for (const item of inconsistencies) {
      const entry = byCollab.get(item.CollaboratorID) ?? {
        id: item.CollaboratorID,
        name: item.CollaboratorName || `Colaborador ${item.CollaboratorID}`,
        count: 0,
        critical: 0,
      }
      entry.count++
      if (item.Severity === 'CRITICO') entry.critical++
      byCollab.set(item.CollaboratorID, entry)
    }
    return [...byCollab.values()]
      .sort((a, b) => b.critical - a.critical || b.count - a.count)
      .slice(0, 5)
  }, [inconsistencies])

  return (
    <section
      aria-label="Colaboradores com mais ocorrências"
      className="rounded-card border border-line bg-bg p-5 shadow-card"
    >
      <div className="mb-4 flex items-center gap-2">
        <Users size={16} className="text-ink-faint" aria-hidden />
        <h2 className="text-sm font-semibold text-ink">Colaboradores com ocorrências</h2>
      </div>

      {ranked.length === 0 ? (
        <p className="py-6 text-center text-sm text-ink-soft">
          Nenhum colaborador com ocorrência nesta varredura.
        </p>
      ) : (
        <ul className="-my-1 flex flex-col">
          {ranked.map((c) => (
            <li key={c.id}>
              <Link
                to={`/colaboradores/${c.id}`}
                className="-mx-2 flex items-center gap-3 rounded-field px-2 py-2.5 transition-colors duration-150 hover:bg-panel"
              >
                <span className="min-w-0 flex-1 truncate font-medium text-ink">{c.name}</span>
                <span className="shrink-0 text-sm tabular-nums text-ink-soft">
                  {c.count} ocorrência{c.count === 1 ? '' : 's'}
                </span>
                <SeverityBadge severity={c.critical > 0 ? 'CRITICO' : 'ALERTA'} />
                <ChevronRight size={16} aria-hidden className="shrink-0 text-ink-faint" />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
