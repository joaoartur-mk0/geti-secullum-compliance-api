import { ArrowLeft, CalendarClock, OctagonAlert, TrendingUp } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import type { ReactNode } from 'react'
import { ErrorNote, SeverityBadge, Skeleton } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { collaboratorHistory } from '../lib/occurrences'
import { formatDate, formatDateTime } from '../lib/format'
import type { Collaborator, Report } from '../lib/types'

interface Data {
  collaborators: Collaborator[]
  reports: Report[]
}

type Loadable =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: Data }

export default function ColaboradorHistorico() {
  const { tenant } = useTenant()
  const params = useParams()
  const secullumId = Number(params.secullumId)
  const [state, setState] = useState<Loadable>({ phase: 'loading' })

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      const [{ collaborators }, reports] = await Promise.all([
        api.listCollaborators(tenant.id),
        api.listReports(tenant.id),
      ])
      setState({ phase: 'ready', data: { collaborators, reports } })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar o histórico.',
      })
    }
  }, [tenant.id])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="animate-rise">
      <Link
        to="/colaboradores"
        className="inline-flex items-center gap-1.5 text-sm font-medium text-ink-soft transition-colors duration-150 hover:text-ink"
      >
        <ArrowLeft size={16} aria-hidden />
        Colaboradores
      </Link>

      {state.phase === 'loading' && (
        <div className="mt-4 flex flex-col gap-6">
          <Skeleton className="h-9 w-64" />
          <div className="grid grid-cols-3 gap-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-24 w-full" />
            ))}
          </div>
          <Skeleton className="h-64 w-full" />
        </div>
      )}

      {state.phase === 'error' && (
        <div className="mt-4">
          <ErrorNote message={state.message} onRetry={load} />
        </div>
      )}

      {state.phase === 'ready' && (
        <Detail secullumId={secullumId} data={state.data} />
      )}
    </div>
  )
}

function Detail({ secullumId, data }: { secullumId: number; data: Data }) {
  const { collaborators, reports } = data
  const collaborator = collaborators.find((c) => c.secullum_id === secullumId) ?? null
  const history = useMemo(() => collaboratorHistory(reports, secullumId), [reports, secullumId])

  // Nome: do espelho sincronizado; se o colaborador não estiver mais na lista, cai para
  // o nome gravado na própria inconsistência, e por fim para o número.
  const name =
    collaborator?.name ||
    history.groups[0]?.items[0]?.CollaboratorName ||
    `Colaborador ${secullumId}`

  return (
    <>
      <header className="mt-4">
        <h1 className="text-2xl font-semibold tracking-tight">{name}</h1>
        <p className="mt-1 text-sm text-ink-soft">
          nº {secullumId}
          {collaborator == null && ' · não consta mais entre os sincronizados'}
        </p>
      </header>

      <section aria-label="Resumo de ocorrências" className="mt-6 grid grid-cols-3 gap-3">
        <Stat
          icon={<TrendingUp size={17} aria-hidden />}
          label="Ocorrências"
          value={String(history.total)}
          hint="no histórico"
          tone={history.total === 0 ? 'ok' : 'neutral'}
        />
        <Stat
          icon={<OctagonAlert size={17} aria-hidden />}
          label="Críticas"
          value={String(history.critical)}
          hint={history.critical === 0 ? 'nenhuma' : 'exigem atenção'}
          tone={history.critical > 0 ? 'critico' : 'ok'}
        />
        <Stat
          icon={<CalendarClock size={17} aria-hidden />}
          label="Última"
          value={history.lastDate ? formatDate(history.lastDate) : '—'}
          hint={history.lastDate ? 'ocorrência mais recente' : 'sem ocorrências'}
          tone="neutral"
        />
      </section>

      <section aria-label="Histórico de ocorrências" className="mt-8">
        <h2 className="mb-3 text-sm font-semibold text-ink-soft">Histórico de ocorrências</h2>

        {history.groups.length === 0 ? (
          <div className="rounded-card border border-dashed border-line px-6 py-12 text-center">
            <p className="font-semibold text-ink">Nenhuma ocorrência registrada</p>
            <p className="mx-auto mt-1 max-w-md text-sm text-ink-soft">
              Este colaborador esteve em conformidade em todas as varreduras do período.
            </p>
          </div>
        ) : (
          <ol className="flex flex-col gap-3">
            {history.groups.map((group) => (
              <li key={group.date} className="rounded-card border border-line bg-bg shadow-card">
                <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5 border-b border-line px-4 py-2.5">
                  <p className="font-semibold text-ink">{formatDate(group.date)}</p>
                  <p className="text-xs text-ink-faint">gerado {formatDateTime(group.dataGenerated)}</p>
                </div>
                <ul className="divide-y divide-line px-4">
                  {group.items.map((item, index) => (
                    <li key={index} className="flex flex-wrap items-start gap-x-4 gap-y-1 py-3">
                      <div className="min-w-0 flex-1">
                        <p className="font-medium text-ink">{item.Type}</p>
                        <p className="mt-0.5 text-sm text-ink-soft">{item.Description}</p>
                      </div>
                      <SeverityBadge severity={item.Severity} />
                    </li>
                  ))}
                </ul>
              </li>
            ))}
          </ol>
        )}
      </section>
    </>
  )
}

type Tone = 'neutral' | 'ok' | 'critico'

const toneValue: Record<Tone, string> = {
  neutral: 'text-ink',
  ok: 'text-ok',
  critico: 'text-critico',
}

function Stat({
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
      <span className={`text-xl font-semibold ${toneValue[tone]}`}>{value}</span>
      <span className="text-xs text-ink-soft">{hint}</span>
    </div>
  )
}
