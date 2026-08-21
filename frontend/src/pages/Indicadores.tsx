import {
  AlertTriangle,
  BarChart3,
  CalendarDays,
  ChevronDown,
  ChevronRight,
  Clock,
  FileWarning,
  Layers,
  OctagonAlert,
  RefreshCw,
  Settings2,
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
  Input,
  Select,
  SeverityBadge,
  Skeleton,
  WarningStatusBadge,
} from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { CATEGORY_ORDER } from '../lib/categories'
import { formatDate, formatDateTime, yesterday } from '../lib/format'
import type {
  AuditInconsistency,
  Branch,
  Occurrence,
  OccurrenceCategory,
  Report,
  ReportMetrics,
  Severity,
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

// Nº máximo de varreduras exibidas no gráfico de tendência — proteção para quando nenhum
// filtro de período está ativo (histórico inteiro); os presets de período (7d/30d/mês)
// já ficam bem abaixo disso e aparecem por completo.
const MAX_CHART_BARS = 62

// isoDaysAgo devolve a data de N dias atrás no formato "YYYY-MM-DD", para os presets de
// período (7 dias, 30 dias).
function isoDaysAgo(n: number): string {
  const d = new Date()
  d.setDate(d.getDate() - n)
  return d.toISOString().slice(0, 10)
}

// isoStartOfMonth devolve o primeiro dia do mês corrente, para o preset "Este mês".
function isoStartOfMonth(): string {
  const d = new Date()
  return new Date(d.getFullYear(), d.getMonth(), 1).toISOString().slice(0, 10)
}

const nf = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 })

// shortDate transforma "2026-07-23" em "23/07" para os rótulos do eixo de tendência.
function shortDate(isoDate: string): string {
  const [, m, d] = isoDate.slice(0, 10).split('-')
  return d && m ? `${d}/${m}` : isoDate
}

// scrollToSection leva o usuário até a seção que explica o número de um card de KPI, em
// vez de ele ter que procurar rolando a página. O destaque temporário confirma que
// chegou no lugar certo — a seção pode já estar visível (nada rola) ou vários scrolls
// abaixo.
function scrollToSection(id: string) {
  const el = document.getElementById(id)
  if (!el) return
  el.scrollIntoView({ behavior: 'smooth', block: 'start' })
  el.classList.add('ring-2', 'ring-brand', 'ring-offset-2')
  window.setTimeout(() => {
    el.classList.remove('ring-2', 'ring-brand', 'ring-offset-2')
  }, 1600)
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

// Presets do filtro de período — "Tudo" não manda start_date/end_date (histórico
// inteiro, sujeito só ao teto de exibição do gráfico, ver MAX_CHART_BARS).
type PeriodPreset = '7d' | '30d' | 'month' | 'custom' | 'all'

export default function Indicadores() {
  const { tenant } = useTenant()
  const [reports, setReports] = useState<Loadable<Report[]>>({ phase: 'loading' })
  // Total de colaboradores sincronizados (GET /collaborators). Secundário: uma falha aqui
  // não derruba os indicadores — só oculta o painel de equipe. null = desconhecido/erro.
  const [syncedTotal, setSyncedTotal] = useState<number | null>(null)
  // Dia selecionado para inspeção (YYYY-MM-DD). null = padrão = a varredura mais recente
  // (D-1) dentro do período filtrado. Reseta a cada troca de tenant/período — ver `load`.
  const [selectedDate, setSelectedDate] = useState<string | null>(null)
  // Período consultado — controla o que é pedido a GET /reports (?start_date&end_date).
  // Padrão: últimos 30 dias, para o painel já abrir mostrando uma janela útil em vez do
  // histórico inteiro.
  const [preset, setPreset] = useState<PeriodPreset>('30d')
  const [periodStart, setPeriodStart] = useState(() => isoDaysAgo(30))
  const [periodEnd, setPeriodEnd] = useState(() => yesterday())

  function applyPreset(next: PeriodPreset) {
    setPreset(next)
    switch (next) {
      case '7d':
        setPeriodStart(isoDaysAgo(7))
        setPeriodEnd(yesterday())
        break
      case '30d':
        setPeriodStart(isoDaysAgo(30))
        setPeriodEnd(yesterday())
        break
      case 'month':
        setPeriodStart(isoStartOfMonth())
        setPeriodEnd(yesterday())
        break
      case 'all':
        setPeriodStart('')
        setPeriodEnd('')
        break
      case 'custom':
        break // datas ficam com o que o usuário já escolheu nos campos De/Até
    }
  }

  const load = useCallback(async () => {
    setReports({ phase: 'loading' })
    setSyncedTotal(null)
    setSelectedDate(null)
    // Colaboradores em paralelo, sem bloquear nem derrubar os relatórios.
    api
      .listCollaborators(tenant.id)
      .then((r) => setSyncedTotal(r.total))
      .catch(() => setSyncedTotal(null))
    try {
      setReports({
        phase: 'ready',
        data: await api.listReports(tenant.id, {
          start_date: periodStart || undefined,
          end_date: periodEnd || undefined,
        }),
      })
    } catch (error) {
      setReports({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar indicadores.',
      })
    }
  }, [tenant.id, periodStart, periodEnd])

  useEffect(() => {
    void load()
  }, [load])

  const latest = reports.phase === 'ready' && reports.data.length > 0 ? reports.data[0] : null
  const latestDay = latest?.date.slice(0, 10) ?? ''
  // O dia efetivamente em exibição: o escolhido, ou o mais recente por padrão.
  const effectiveDay = selectedDate ?? latestDay

  // IMPORTANTE: o usuário pode escolher QUALQUER dia, não só um que já tenha varredura —
  // é isso que corrige o seletor "preso" às datas existentes. Sem correspondência,
  // `selectedReport` fica null e a tela mostra um estado vazio explícito em vez de cair
  // silenciosamente na varredura mais recente (o que escondia o fato de o dia não ter
  // sido auditado).
  const selectedReport =
    reports.phase === 'ready' && effectiveDay
      ? (reports.data.find((r) => r.date.slice(0, 10) === effectiveDay) ?? null)
      : null

  function selectDate(date: string) {
    const day = date.slice(0, 10)
    setSelectedDate(day === latestDay ? null : day)
  }

  return (
    <div className="animate-rise">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Indicadores</h1>
          <p className="mt-1 text-sm text-ink-soft">
            {reports.phase !== 'ready' || reports.data.length === 0 ? (
              'Visão gerencial das varreduras de compliance da sua equipe.'
            ) : selectedReport ? (
              <>
                Varredura de <strong className="font-semibold text-ink">{formatDate(selectedReport.date)}</strong>
                {' · '}gerada {formatDateTime(selectedReport.data_generated)}
                {selectedDate && selectedReport.id !== latest?.id && (
                  <span className="ml-1 text-ink-faint">(consultando o histórico)</span>
                )}
              </>
            ) : (
              <>
                Nenhuma varredura registrada para{' '}
                <strong className="font-semibold text-ink">{formatDate(effectiveDay)}</strong>.
              </>
            )}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {reports.phase === 'ready' && reports.data.length > 0 && (
            <Input
              type="date"
              aria-label="Selecionar dia auditado"
              value={effectiveDay}
              max={yesterday()}
              onChange={(e) => selectDate(e.target.value)}
            />
          )}
          <button
            type="button"
            onClick={load}
            className="flex min-h-11 items-center gap-1.5 rounded-field px-2.5 text-sm font-medium text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink"
          >
            <RefreshCw size={15} aria-hidden />
            Atualizar
          </button>
        </div>
      </header>

      <PeriodFilterBar
        preset={preset}
        periodStart={periodStart}
        periodEnd={periodEnd}
        onPreset={applyPreset}
        onCustomStart={(v) => {
          setPreset('custom')
          setPeriodStart(v)
        }}
        onCustomEnd={(v) => {
          setPreset('custom')
          setPeriodEnd(v)
        }}
      />

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

      {reports.phase === 'ready' && reports.data.length === 0 && preset !== 'all' && (
        <div className="mt-8">
          <EmptyState
            icon={<BarChart3 size={32} strokeWidth={1.5} />}
            title="Nenhuma varredura no período selecionado"
            description="Tente um período maior — botão 'Tudo' acima — ou dispare uma auditoria em Situação por dia."
            action={
              <button
                type="button"
                onClick={() => applyPreset('all')}
                className="text-sm font-semibold text-brand underline underline-offset-2 hover:text-brand-strong"
              >
                Ver todo o histórico
              </button>
            }
          />
        </div>
      )}

      {reports.phase === 'ready' && reports.data.length === 0 && preset === 'all' && (
        <div className="mt-8">
          <EmptyState
            icon={<BarChart3 size={32} strokeWidth={1.5} />}
            title="Nenhuma varredura ainda"
            description="Os indicadores são calculados a partir das varreduras de compliance. Dispare uma auditoria em Situação por dia — o resultado aparece aqui."
            action={
              <Link
                to="/auditorias"
                className="text-sm font-semibold text-brand underline underline-offset-2 hover:text-brand-strong"
              >
                Ir para Situação por dia
              </Link>
            }
          />
        </div>
      )}

      {reports.phase === 'ready' && reports.data.length > 0 && (
        <Dashboard
          reports={reports.data}
          selectedReport={selectedReport}
          selectedDay={effectiveDay}
          syncedTotal={syncedTotal}
          onSelectDate={selectDate}
        />
      )}
    </div>
  )
}

// ---------- Filtro de período (dia único, semana, mês ou intervalo customizado) ----------
//
// Controla o que é pedido a GET /reports (?start_date&end_date) — não é um filtro de
// exibição sobre uma lista já carregada. "Tudo" limpa os dois parâmetros (histórico
// inteiro, sujeito ao teto de exibição do gráfico de tendência).

const PERIOD_PRESETS: { key: PeriodPreset; label: string }[] = [
  { key: '7d', label: '7 dias' },
  { key: '30d', label: '30 dias' },
  { key: 'month', label: 'Este mês' },
  { key: 'all', label: 'Tudo' },
]

function PeriodFilterBar({
  preset,
  periodStart,
  periodEnd,
  onPreset,
  onCustomStart,
  onCustomEnd,
}: {
  preset: PeriodPreset
  periodStart: string
  periodEnd: string
  onPreset: (preset: PeriodPreset) => void
  onCustomStart: (value: string) => void
  onCustomEnd: (value: string) => void
}) {
  return (
    <div
      className="mt-4 flex flex-wrap items-end gap-2 rounded-card border border-line bg-bg p-3 shadow-card"
      aria-label="Filtro de período"
    >
      <div className="flex flex-wrap items-center gap-1.5">
        {PERIOD_PRESETS.map((p) => (
          <button
            key={p.key}
            type="button"
            onClick={() => onPreset(p.key)}
            aria-pressed={preset === p.key}
            className={`flex min-h-9 items-center rounded-field px-3 text-sm font-medium transition-colors duration-150 ${
              preset === p.key
                ? 'bg-brand text-white'
                : 'border border-line text-ink-soft hover:border-ink-faint hover:text-ink'
            }`}
          >
            {p.label}
          </button>
        ))}
      </div>
      <span className="text-xs text-ink-faint">ou personalizado:</span>
      <label className="flex flex-col gap-1">
        <span className="text-xs font-medium text-ink-soft">De</span>
        <Input
          type="date"
          value={periodStart}
          max={periodEnd || yesterday()}
          onChange={(e) => onCustomStart(e.target.value)}
          aria-label="Início do período"
          className="min-h-9"
        />
      </label>
      <label className="flex flex-col gap-1">
        <span className="text-xs font-medium text-ink-soft">Até</span>
        <Input
          type="date"
          value={periodEnd}
          min={periodStart || undefined}
          max={yesterday()}
          onChange={(e) => onCustomEnd(e.target.value)}
          aria-label="Fim do período"
          className="min-h-9"
        />
      </label>
    </div>
  )
}

function Dashboard({
  reports,
  selectedReport,
  selectedDay,
  syncedTotal,
  onSelectDate,
}: {
  reports: Report[]
  selectedReport: Report | null
  selectedDay: string
  syncedTotal: number | null
  onSelectDate: (date: string) => void
}) {
  const d = useMemo(() => (selectedReport ? derive(selectedReport) : null), [selectedReport])
  const m: ReportMetrics | null = selectedReport?.metrics ?? null

  // Série cronológica (o backend devolve do mais recente ao mais antigo) para a tendência.
  // Não depende do dia selecionado — mostra a evolução do período filtrado (ver
  // PeriodFilterBar). `reports` já vem do backend com uma linha por dia (a mais recente)
  // — GET /reports deduplica.
  const series = useMemo(
    () =>
      reports
        .slice(0, MAX_CHART_BARS)
        .map((r) => {
          const dm = derive(r)
          return { date: r.date, total: dm.total, critical: dm.critical, alert: dm.alert }
        })
        .reverse(),
    [reports],
  )

  return (
    <div className="mt-8 flex flex-col gap-6">
      {d ? (
        <>
          {/* Leitura do dia selecionado: números-resumo primeiro, depois quem está por trás
              deles e o detalhamento que explica cada número acima. */}
          <KpiRow derived={d} metrics={m} reportCount={reports.length} />
          <IncidentShortcuts
            inconsistencies={selectedReport?.inconsistencies ?? []}
            date={selectedReport?.date ?? ''}
          />
          <CollaboratorsSummary syncedTotal={syncedTotal} withAlerts={d.affectedCollaborators} />
          <DistributionChart
            byType={d.byType}
            total={d.total}
            inconsistencies={selectedReport?.inconsistencies ?? []}
          />
          <TopCollaborators inconsistencies={selectedReport?.inconsistencies ?? []} />
        </>
      ) : (
        <NoReportForDay day={selectedDay} />
      )}

      <TrendChart series={series} selectedDate={selectedReport?.date ?? ''} onSelect={onSelectDate} />

      <LiveStateDivider />

      {/* Estas duas seções são o estado ATUAL (independem do dia selecionado acima). */}
      <OccurrencesByCategory />
      <WarningsSummary />
    </div>
  )
}

// ---------- Divisor: sinaliza a troca de contexto para o estado ATUAL do sistema ----------

function LiveStateDivider() {
  return (
    <div className="flex items-center gap-3" role="separator">
      <span className="shrink-0 text-xs font-semibold uppercase tracking-wide text-ink-faint">
        Situação atual
      </span>
      <span className="h-px flex-1 bg-line" aria-hidden />
      <span className="shrink-0 text-xs text-ink-faint">independe do dia selecionado acima</span>
    </div>
  )
}

// ---------- Estado vazio: dia escolhido sem varredura ----------

function NoReportForDay({ day }: { day: string }) {
  return (
    <section className="rounded-card border border-dashed border-line px-6 py-10 text-center">
      <p className="font-semibold text-ink">Nenhuma varredura para {formatDate(day)}</p>
      <p className="mx-auto mt-1 max-w-md text-sm text-ink-soft">
        Esse dia ainda não foi auditado. Dispare uma auditoria específica em Situação por dia, ou
        escolha outro dia acima ou no gráfico de evolução mais abaixo.
      </p>
      <Link
        to="/auditorias"
        className="mt-3 inline-block text-sm font-semibold text-brand underline underline-offset-2 hover:text-brand-strong"
      >
        Ir para Situação por dia
      </Link>
    </section>
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
          <div>
            <h2 className="text-sm font-semibold text-ink">Em aberto por categoria</h2>
            <p className="text-xs text-ink-faint">Ocorrências abertas neste momento</p>
          </div>
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
          targetId="colaboradores-ocorrencias"
        />
        <Kpi
          icon={<TrendingUp size={17} aria-hidden />}
          label="Inconsistências"
          value={String(metrics.total_inconsistencies)}
          hint={`${metrics.critical} crítica${metrics.critical === 1 ? '' : 's'} · ${metrics.alert} alerta${metrics.alert === 1 ? '' : 's'}`}
          tone={metrics.total_inconsistencies === 0 ? 'ok' : metrics.critical > 0 ? 'critico' : 'alerta'}
          targetId="distribuicao-tipo"
        />
        <Kpi
          icon={<Users size={17} aria-hidden />}
          label="Colaboradores"
          value={String(metrics.collaborators_audited)}
          hint="auditados na varredura"
          tone="neutral"
          targetId="colaboradores-resumo"
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
        targetId="distribuicao-tipo"
      />
      <Kpi
        icon={<OctagonAlert size={17} aria-hidden />}
        label="Críticas"
        value={String(derived.critical)}
        hint={derived.critical === 0 ? 'nenhuma ação urgente' : 'exigem ação imediata'}
        tone={derived.critical > 0 ? 'critico' : 'ok'}
        targetId="colaboradores-ocorrencias"
      />
      <Kpi
        icon={<CalendarDays size={17} aria-hidden />}
        label="Varreduras"
        value={String(reportCount)}
        hint="no histórico do período"
        tone="neutral"
        targetId="evolucao-varreduras"
      />
    </section>
  )
}

// ---------- Atalhos: cards de severidade → listagem completa em /incidents ----------
//
// Diferente dos KPIs acima (que rolam até a seção explicativa na própria página), estes
// cards levam pra listagem filtrável de ocorrências (/incidents), pré-filtrada pela
// severidade clicada E pelo dia que está sendo inspecionado aqui — senão o número do card
// (do dia selecionado) não bate com a lista que abre (o histórico inteiro).

function IncidentShortcuts({
  inconsistencies,
  date,
}: {
  inconsistencies: AuditInconsistency[]
  date: string
}) {
  const counts = useMemo(() => {
    const c = { CRITICO: 0, ALERTA: 0, OPERACIONAL: 0 }
    for (const item of inconsistencies) c[item.Severity]++
    return c
  }, [inconsistencies])

  // O card conta as inconsistências de UM dia (o do relatório selecionado), então o link
  // leva o mesmo dia nas duas pontas do período. Sem dia (nenhuma varredura no período),
  // cai na lista sem filtro de data.
  const incidentLink = useCallback(
    (severity: Severity) => {
      const params = new URLSearchParams({ severity })
      if (date) {
        params.set('start_date', date)
        params.set('end_date', date)
      }
      return `/incidents?${params.toString()}`
    },
    [date],
  )

  return (
    <section aria-label="Atalhos de ocorrências por severidade" className="grid grid-cols-3 gap-3">
      <ShortcutCard
        icon={<OctagonAlert size={17} aria-hidden />}
        label="Críticas"
        value={counts.CRITICO}
        tone="critico"
        to={incidentLink('CRITICO')}
      />
      <ShortcutCard
        icon={<AlertTriangle size={17} aria-hidden />}
        label="Alertas"
        value={counts.ALERTA}
        tone="alerta"
        to={incidentLink('ALERTA')}
      />
      <ShortcutCard
        icon={<Settings2 size={17} aria-hidden />}
        label="Operacionais"
        value={counts.OPERACIONAL}
        tone="neutral"
        to={incidentLink('OPERACIONAL')}
      />
    </section>
  )
}

function ShortcutCard({
  icon,
  label,
  value,
  tone,
  to,
}: {
  icon: ReactNode
  label: string
  value: number
  tone: Tone
  to: string
}) {
  return (
    <Link
      to={to}
      className="flex flex-col gap-2 rounded-card border border-line bg-bg p-4 text-left shadow-card transition-colors duration-150 hover:border-ink-faint"
    >
      <div className="flex items-center gap-1.5 text-ink-faint">
        {icon}
        <span className="text-xs font-semibold uppercase tracking-wide">{label}</span>
        <ChevronRight size={14} className="ml-auto shrink-0" aria-hidden />
      </div>
      <span className={`text-2xl font-semibold tabular-nums ${toneValue[tone]}`}>{value}</span>
      <span className="text-xs text-ink-soft">ver ocorrências</span>
    </Link>
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
    <section
      id="colaboradores-resumo"
      aria-label="Colaboradores sob auditoria"
      className="scroll-mt-20 rounded-card"
    >
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
            targetId="colaboradores-ocorrencias"
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
  targetId,
}: {
  icon: ReactNode
  label: string
  value: string
  hint: string
  tone: Tone
  // Quando informado, o card vira um atalho: clicar rola até a seção que detalha esse
  // número, em vez do usuário ter que procurá-la rolando a página manualmente.
  targetId?: string
}) {
  const content = (
    <>
      <div className="flex items-center gap-1.5 text-ink-faint">
        {icon}
        <span className="text-xs font-semibold uppercase tracking-wide">{label}</span>
        {targetId && <ChevronDown size={14} className="ml-auto shrink-0" aria-hidden />}
      </div>
      <span className={`text-2xl font-semibold tabular-nums ${toneValue[tone]}`}>{value}</span>
      <span className="text-xs text-ink-soft">{hint}</span>
    </>
  )

  if (!targetId) {
    return <div className="flex flex-col gap-2 rounded-card border border-line bg-bg p-4 shadow-card">{content}</div>
  }

  return (
    <button
      type="button"
      onClick={() => scrollToSection(targetId)}
      className="flex flex-col gap-2 rounded-card border border-line bg-bg p-4 text-left shadow-card transition-colors duration-150 hover:border-ink-faint"
    >
      {content}
    </button>
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
  inconsistencies,
}: {
  byType: Record<string, { critical: number; alert: number }>
  total: number
  inconsistencies: AuditInconsistency[]
}) {
  const [expanded, setExpanded] = useState<string | null>(null)
  const known = TYPE_ORDER.filter((t) => byType[t] != null)
  const extras = Object.keys(byType).filter((t) => !TYPE_ORDER.includes(t))
  const rows = [...known, ...extras].map((type) => {
    const b = byType[type] ?? { critical: 0, alert: 0 }
    return { type, critical: b.critical, alert: b.alert, count: b.critical + b.alert }
  })
  const max = rows.reduce((m, r) => Math.max(m, r.count), 0)

  return (
    <section
      id="distribuicao-tipo"
      aria-label="Distribuição de inconsistências por tipo"
      className="scroll-mt-20 rounded-card border border-line bg-bg p-5 shadow-card"
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
            const isExpanded = expanded === type
            const offenders = inconsistencies.filter((i) => i.Type === type)
            return (
              <li key={type} className="flex flex-col gap-1.5">
                <button
                  type="button"
                  onClick={() => setExpanded(isExpanded ? null : type)}
                  aria-expanded={isExpanded}
                  className="flex items-baseline justify-between gap-3 rounded-field text-left text-sm transition-colors duration-150 hover:text-brand"
                >
                  <span className="flex items-center gap-1 font-medium text-ink">
                    {type}
                    <ChevronDown
                      size={14}
                      aria-hidden
                      className={`shrink-0 text-ink-faint transition-transform duration-200 ${isExpanded ? 'rotate-180' : ''}`}
                    />
                  </span>
                  <span className="shrink-0 tabular-nums text-ink-soft">
                    {count} <span className="text-ink-faint">({pct}%)</span>
                  </span>
                </button>
                <div className="h-2.5 w-full overflow-hidden rounded-full bg-panel" aria-hidden>
                  <div className="flex h-full transition-[width] duration-500 ease-out" style={{ width: `${widthPct}%` }}>
                    {critical > 0 && <div className="h-full bg-critico" style={{ width: `${critShare}%` }} />}
                    {alert > 0 && <div className="h-full bg-alerta" style={{ width: `${100 - critShare}%` }} />}
                  </div>
                </div>
                {isExpanded && (
                  <ul className="mt-1 divide-y divide-line rounded-card border border-line">
                    {offenders.map((item, index) => (
                      <li key={index}>
                        <Link
                          to={`/colaboradores/${item.CollaboratorID}`}
                          className="flex items-center gap-3 px-3 py-2.5 transition-colors duration-150 hover:bg-panel"
                        >
                          <span className="min-w-0 flex-1 truncate text-sm font-medium text-ink">
                            {item.CollaboratorName}
                          </span>
                          <SeverityBadge severity={item.Severity} />
                          <ChevronRight size={16} aria-hidden className="shrink-0 text-ink-faint" />
                        </Link>
                      </li>
                    ))}
                  </ul>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

// ---------- Gráfico: tendência ao longo das varreduras ----------

function TrendChart({
  series,
  selectedDate,
  onSelect,
}: {
  series: { date: string; total: number; critical: number; alert: number }[]
  selectedDate: string
  onSelect: (date: string) => void
}) {
  const max = series.reduce((m, s) => Math.max(m, s.total), 0)

  return (
    <section
      id="evolucao-varreduras"
      aria-label="Tendência de inconsistências por varredura"
      className="scroll-mt-20 rounded-card border border-line bg-bg p-5 shadow-card"
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
            const isSelected = s.date === selectedDate
            return (
              <button
                key={s.date}
                type="button"
                onClick={() => onSelect(s.date)}
                aria-pressed={isSelected}
                aria-label={`Ver indicadores de ${formatDate(s.date)}`}
                title={`${formatDate(s.date)} — ${s.total} inconsistência(s): ${s.critical} crítica(s), ${s.alert} alerta(s)`}
                className="flex min-w-0 flex-1 flex-col items-center gap-1.5 rounded-field transition-colors duration-150 hover:bg-panel"
              >
                <div className="flex w-full flex-1 items-end justify-center">
                  {s.total === 0 ? (
                    <div className={`h-0.5 w-full rounded-full ${isSelected ? 'bg-brand' : 'bg-line'}`} aria-hidden />
                  ) : (
                    <div
                      className={`flex w-full max-w-8 flex-col overflow-hidden rounded-t-sm transition-[height] duration-500 ease-out ${
                        isSelected ? 'ring-2 ring-brand ring-offset-1' : ''
                      }`}
                      style={{ height: `${Math.max(heightPct, 4)}%` }}
                    >
                      {s.critical > 0 && <div className="w-full bg-critico" style={{ height: `${critShare}%` }} />}
                      {s.alert > 0 && <div className="w-full bg-alerta" style={{ height: `${100 - critShare}%` }} />}
                    </div>
                  )}
                </div>
                <span
                  className={`w-full truncate text-center text-[10px] tabular-nums ${
                    isSelected ? 'font-semibold text-brand' : 'text-ink-faint'
                  }`}
                >
                  {shortDate(s.date)}
                </span>
              </button>
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
      id="colaboradores-ocorrencias"
      aria-label="Colaboradores com mais ocorrências"
      className="scroll-mt-20 rounded-card border border-line bg-bg p-5 shadow-card"
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
