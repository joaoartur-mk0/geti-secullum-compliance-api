import { ChevronRight, RefreshCw, Search, Users } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { EmptyState, ErrorNote, Input, SeverityBadge, Skeleton } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { summarizeByCollaborator, type CollaboratorOccurrenceSummary } from '../lib/occurrences'
import type { Collaborator } from '../lib/types'

type Loadable<T> =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: T }

const EMPTY_SUMMARY: CollaboratorOccurrenceSummary = {
  latestCount: 0,
  latestCritical: 0,
  totalCount: 0,
  reportsWithOccurrence: 0,
}

export default function Colaboradores() {
  const { tenant } = useTenant()
  const [collabs, setCollabs] = useState<Loadable<Collaborator[]>>({ phase: 'loading' })
  const [summaries, setSummaries] = useState<Map<number, CollaboratorOccurrenceSummary>>(new Map())
  const [query, setQuery] = useState('')
  const [onlyWithOccurrence, setOnlyWithOccurrence] = useState(false)

  const load = useCallback(async () => {
    setCollabs({ phase: 'loading' })
    // Ocorrências são derivadas dos relatórios (secundário): se falhar, a lista ainda
    // aparece, só sem o status de ocorrência.
    api
      .listReports(tenant.id)
      .then((reports) => setSummaries(summarizeByCollaborator(reports)))
      .catch(() => setSummaries(new Map()))
    try {
      const { collaborators } = await api.listCollaborators(tenant.id)
      setCollabs({ phase: 'ready', data: collaborators })
    } catch (error) {
      setCollabs({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar colaboradores.',
      })
    }
  }, [tenant.id])

  useEffect(() => {
    void load()
  }, [load])

  const rows = useMemo(() => {
    if (collabs.phase !== 'ready') return []
    const q = query.trim().toLowerCase()
    return collabs.data
      .map((c) => ({ collaborator: c, summary: summaries.get(c.secullum_id) ?? EMPTY_SUMMARY }))
      .filter(({ collaborator, summary }) => {
        if (onlyWithOccurrence && summary.totalCount === 0) return false
        if (!q) return true
        return (
          collaborator.name.toLowerCase().includes(q) || String(collaborator.secullum_id).includes(q)
        )
      })
      .sort((a, b) => a.collaborator.name.localeCompare(b.collaborator.name, 'pt-BR'))
  }, [collabs, summaries, query, onlyWithOccurrence])

  const total = collabs.phase === 'ready' ? collabs.data.length : 0
  const withOccurrence = useMemo(
    () => [...summaries.values()].filter((s) => s.totalCount > 0).length,
    [summaries],
  )

  return (
    <div className="animate-rise">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Colaboradores</h1>
          <p className="mt-1 text-sm text-ink-soft">
            {collabs.phase === 'ready'
              ? `${total} sincronizado${total === 1 ? '' : 's'}${withOccurrence > 0 ? ` · ${withOccurrence} com ocorrência no período` : ''}`
              : 'Funcionários sincronizados da Secullum, sob auditoria.'}
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

      {collabs.phase === 'loading' && (
        <div className="mt-8 flex flex-col gap-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-14 w-full" />
          ))}
        </div>
      )}

      {collabs.phase === 'error' && (
        <div className="mt-8">
          <ErrorNote message={collabs.message} onRetry={load} />
        </div>
      )}

      {collabs.phase === 'ready' && collabs.data.length === 0 && (
        <div className="mt-8">
          <EmptyState
            icon={<Users size={32} strokeWidth={1.5} />}
            title="Nenhum colaborador sincronizado"
            description="A sincronização com a Secullum roda ao cadastrar a empresa. Dispare novamente em Empresa se necessário."
            action={
              <Link
                to="/empresa"
                className="text-sm font-semibold text-brand underline underline-offset-2 hover:text-brand-strong"
              >
                Ir para Empresa
              </Link>
            }
          />
        </div>
      )}

      {collabs.phase === 'ready' && collabs.data.length > 0 && (
        <>
          <div className="mt-6 flex flex-wrap items-center gap-3">
            <div className="relative min-w-56 flex-1">
              <Search
                size={16}
                aria-hidden
                className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-faint"
              />
              <Input
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Buscar por nome ou nº"
                aria-label="Buscar colaborador"
                className="w-full pl-9"
              />
            </div>
            <label className="flex min-h-11 cursor-pointer items-center gap-2 rounded-field border border-line bg-bg px-3 text-sm font-medium text-ink-soft transition-colors duration-150 hover:border-ink-faint">
              <input
                type="checkbox"
                checked={onlyWithOccurrence}
                onChange={(e) => setOnlyWithOccurrence(e.target.checked)}
                className="h-4 w-4 accent-brand"
              />
              Só com ocorrência
            </label>
          </div>

          {rows.length === 0 ? (
            <p className="mt-8 text-center text-sm text-ink-soft">
              Nenhum colaborador corresponde ao filtro.
            </p>
          ) : (
            <ul className="mt-4 flex flex-col gap-2">
              {rows.map(({ collaborator, summary }) => (
                <CollaboratorRow key={collaborator.id} collaborator={collaborator} summary={summary} />
              ))}
            </ul>
          )}
        </>
      )}
    </div>
  )
}

function CollaboratorRow({
  collaborator,
  summary,
}: {
  collaborator: Collaborator
  summary: CollaboratorOccurrenceSummary
}) {
  return (
    <li>
      <Link
        to={`/colaboradores/${collaborator.secullum_id}`}
        className="flex items-center gap-3 rounded-card border border-line bg-bg px-4 py-3 shadow-card transition-colors duration-150 hover:bg-panel"
      >
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium text-ink">{collaborator.name || `Colaborador ${collaborator.secullum_id}`}</p>
          <p className="text-xs text-ink-faint">
            nº {collaborator.secullum_id}
            {summary.totalCount > 0 && (
              <span className="text-ink-soft">
                {' · '}
                {summary.totalCount} no histórico ({summary.reportsWithOccurrence} varredura
                {summary.reportsWithOccurrence === 1 ? '' : 's'})
              </span>
            )}
          </p>
        </div>

        {summary.latestCount > 0 ? (
          <span className="flex items-center gap-2">
            <span className="text-sm tabular-nums text-ink-soft">{summary.latestCount}</span>
            <SeverityBadge severity={summary.latestCritical > 0 ? 'CRITICO' : 'ALERTA'} />
          </span>
        ) : (
          <span className="inline-flex items-center rounded-full bg-ok-bg px-2.5 py-1 text-xs font-semibold text-ok">
            OK
          </span>
        )}

        <ChevronRight size={18} aria-hidden className="shrink-0 text-ink-faint" />
      </Link>
    </li>
  )
}
