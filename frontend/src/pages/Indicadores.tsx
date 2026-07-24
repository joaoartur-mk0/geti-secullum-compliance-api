import { BarChart3, Clock, Info, RefreshCw, ShieldCheck, TrendingUp, Users } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import type { ReactNode } from 'react'
import { EmptyState, ErrorNote, Skeleton } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { formatDate, formatDateTime } from '../lib/format'
import type { Report, ReportMetrics } from '../lib/types'

type Loadable<T> =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: T }

// A ordem canônica dos tipos de inconsistência, para o gráfico de distribuição
// manter um eixo estável (não reordena a cada varredura).
const TYPE_ORDER = [
  'Batida Esquecida',
  'Almoço Reduzido',
  'Interjornada Curta',
  'Hora Extra Excedente',
  'Alerta de Hora Extra',
]

// derivedFromInconsistencies reconstrói o que é possível SEM o backend de métricas:
// contagens por severidade e por tipo. Horas extras, atrasos e índice de conformidade
// dependem de dados que só a varredura completa por colaborador fornece (metrics).
function derive(report: Report) {
  const items = report.inconsistencies ?? []
  const byType: Record<string, number> = {}
  const collaborators = new Set<number>()
  let critical = 0
  for (const item of items) {
    byType[item.Type] = (byType[item.Type] ?? 0) + 1
    collaborators.add(item.CollaboratorID)
    if (item.Severity === 'CRITICO') critical++
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

  const load = useCallback(async () => {
    setReports({ phase: 'loading' })
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
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-28 w-full" />
            ))}
          </div>
          <Skeleton className="h-64 w-full" />
        </div>
      )}

      {reports.phase === 'error' && <div className="mt-8"><ErrorNote message={reports.message} onRetry={load} /></div>}

      {reports.phase === 'ready' && !latest && (
        <div className="mt-8">
          <EmptyState
            icon={<BarChart3 size={32} strokeWidth={1.5} />}
            title="Nenhuma varredura ainda"
            description='Os indicadores são calculados a partir das varreduras de compliance. Dispare uma auditoria no Painel — o resultado aparece aqui.'
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

      {latest && <Dashboard report={latest} />}
    </div>
  )
}

function Dashboard({ report }: { report: Report }) {
  const view = useMemo(() => {
    const d = derive(report)
    const m: ReportMetrics | null = report.metrics ?? null
    return {
      hasBackendMetrics: m != null,
      complianceRate: m?.compliance_rate ?? null,
      total: m?.total_inconsistencies ?? d.total,
      critical: m?.critical ?? d.critical,
      alert: m?.alert ?? d.alert,
      overtimeHours: m?.overtime_hours_total ?? null,
      lateHours: m?.late_hours_total ?? null,
      collaboratorsAudited: m?.collaborators_audited ?? null,
      affectedCollaborators: d.affectedCollaborators,
      byType: m?.by_type ?? d.byType,
    }
  }, [report])

  const nf = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 })

  return (
    <div className="mt-8 flex flex-col gap-6">
      {!view.hasBackendMetrics && (
        <div className="flex items-start gap-2.5 rounded-card border border-line bg-panel px-4 py-3 text-sm text-ink-soft">
          <Info size={16} className="mt-0.5 shrink-0 text-ink-faint" aria-hidden />
          <p>
            Índice de conformidade, horas extras e atrasos ficam disponíveis quando a varredura
            completa por colaborador entrar em produção. Por ora, exibimos os indicadores derivados
            das inconsistências detectadas.
          </p>
        </div>
      )}

      <section aria-label="Indicadores consolidados" className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        <Kpi
          icon={<ShieldCheck size={17} aria-hidden />}
          label="Conformidade"
          value={view.complianceRate != null ? `${nf.format(view.complianceRate)}%` : '—'}
          hint={view.complianceRate != null ? 'colaboradores sem ocorrência' : 'aguardando varredura completa'}
          tone={
            view.complianceRate == null
              ? 'muted'
              : view.complianceRate >= 90
                ? 'ok'
                : view.complianceRate >= 70
                  ? 'alerta'
                  : 'critico'
          }
        />
        <Kpi
          icon={<TrendingUp size={17} aria-hidden />}
          label="Inconsistências"
          value={String(view.total)}
          hint={
            view.total === 0
              ? 'dia em conformidade'
              : `${view.critical} crítica${view.critical === 1 ? '' : 's'} · ${view.alert} alerta${view.alert === 1 ? '' : 's'}`
          }
          tone={view.total === 0 ? 'ok' : view.critical > 0 ? 'critico' : 'alerta'}
        />
        <Kpi
          icon={<Users size={17} aria-hidden />}
          label={view.collaboratorsAudited != null ? 'Colaboradores' : 'Com ocorrência'}
          value={
            view.collaboratorsAudited != null
              ? String(view.collaboratorsAudited)
              : String(view.affectedCollaborators)
          }
          hint={view.collaboratorsAudited != null ? 'auditados na varredura' : 'colaboradores afetados'}
          tone="neutral"
        />
        <Kpi
          icon={<Clock size={17} aria-hidden />}
          label="Horas extras"
          value={view.overtimeHours != null ? `${nf.format(view.overtimeHours)}h` : '—'}
          hint={view.overtimeHours != null ? 'total do dia' : 'aguardando varredura completa'}
          tone={view.overtimeHours == null ? 'muted' : view.overtimeHours > 0 ? 'alerta' : 'ok'}
        />
        <Kpi
          icon={<Clock size={17} aria-hidden />}
          label="Atrasos"
          value={view.lateHours != null ? `${nf.format(view.lateHours)}h` : '—'}
          hint={view.lateHours != null ? 'total do dia' : 'aguardando varredura completa'}
          tone={view.lateHours == null ? 'muted' : view.lateHours > 0 ? 'alerta' : 'ok'}
        />
      </section>

      <DistributionChart byType={view.byType} total={view.total} />
    </div>
  )
}

type Tone = 'neutral' | 'ok' | 'alerta' | 'critico' | 'muted'

const toneValue: Record<Tone, string> = {
  neutral: 'text-ink',
  ok: 'text-ok',
  alerta: 'text-alerta',
  critico: 'text-critico',
  muted: 'text-ink-faint',
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

function DistributionChart({ byType, total }: { byType: Record<string, number>; total: number }) {
  // Mantém a ordem canônica e acrescenta ao final quaisquer tipos novos ainda não previstos.
  const known = TYPE_ORDER.filter((t) => byType[t] != null)
  const extras = Object.keys(byType).filter((t) => !TYPE_ORDER.includes(t))
  const rows = [...known, ...extras].map((type) => ({ type, count: byType[type] ?? 0 }))
  const max = rows.reduce((m, r) => Math.max(m, r.count), 0)

  return (
    <section
      aria-label="Distribuição de inconsistências por tipo"
      className="rounded-card border border-line bg-bg p-5 shadow-card"
    >
      <div className="mb-4 flex items-center gap-2">
        <BarChart3 size={16} className="text-ink-faint" aria-hidden />
        <h2 className="text-sm font-semibold text-ink">Inconsistências por tipo</h2>
      </div>

      {total === 0 || rows.length === 0 ? (
        <p className="py-6 text-center text-sm text-ink-soft">
          Nenhuma inconsistência nesta varredura — todas as batidas em conformidade.
        </p>
      ) : (
        <ul className="flex flex-col gap-3">
          {rows.map(({ type, count }) => {
            const pct = total > 0 ? Math.round((count / total) * 100) : 0
            const width = max > 0 ? (count / max) * 100 : 0
            return (
              <li key={type} className="flex flex-col gap-1.5">
                <div className="flex items-baseline justify-between gap-3 text-sm">
                  <span className="font-medium text-ink">{type}</span>
                  <span className="shrink-0 tabular-nums text-ink-soft">
                    {count} <span className="text-ink-faint">({pct}%)</span>
                  </span>
                </div>
                <div className="h-2 w-full overflow-hidden rounded-full bg-panel" aria-hidden>
                  <div
                    className="h-full rounded-full bg-brand transition-[width] duration-500 ease-out"
                    style={{ width: `${width}%` }}
                  />
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
