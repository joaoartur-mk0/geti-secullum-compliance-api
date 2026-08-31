// Listagem completa de ocorrências, com filtros de período/severidade/filial — o destino
// dos atalhos "X críticas" / "X alertas" / "X operacionais" em Indicadores, mas também
// navegável direto com qualquer combinação de filtros via querystring.
//
// A API (GET /occurrences) já filtra por período (start_date/end_date) e filial — só não
// filtra por severidade, então esse recorte é feito aqui no cliente.

import { ChevronLeft, ChevronRight, ListFilter } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import {
  EmptyState,
  ErrorNote,
  Input,
  OccurrenceStateBadge,
  Select,
  SeverityBadge,
  Skeleton,
} from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { formatDate } from '../lib/format'
import type { Branch, Occurrence, Severity } from '../lib/types'

type Loadable<T> =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: T }

const PAGE_SIZE = 20

export default function Incidentes() {
  const { tenant } = useTenant()
  const [searchParams, setSearchParams] = useSearchParams()

  const severity = (searchParams.get('severity') as Severity | null) ?? ''
  const startDate = searchParams.get('start_date') ?? ''
  const endDate = searchParams.get('end_date') ?? ''
  const branchId = searchParams.get('branch_id') ?? ''

  const [branches, setBranches] = useState<Branch[]>([])
  const [state, setState] = useState<Loadable<Occurrence[]>>({ phase: 'loading' })
  const [page, setPage] = useState(1)

  useEffect(() => {
    api.listBranches(tenant.id).then(setBranches).catch(() => setBranches([]))
  }, [tenant.id])

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      const { occurrences } = await api.listOccurrences(tenant.id, {
        start_date: startDate || undefined,
        end_date: endDate || undefined,
        branch_id: branchId ? Number(branchId) : undefined,
      })
      setState({ phase: 'ready', data: occurrences })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar ocorrências.',
      })
    }
  }, [tenant.id, startDate, endDate, branchId])

  useEffect(() => {
    void load()
  }, [load])

  // Qualquer troca de filtro volta pra primeira página — evita cair numa página vazia.
  useEffect(() => {
    setPage(1)
  }, [severity, startDate, endDate, branchId])

  function setParam(key: string, value: string) {
    const next = new URLSearchParams(searchParams)
    if (value) next.set(key, value)
    else next.delete(key)
    setSearchParams(next, { replace: true })
  }

  const filtered = useMemo(() => {
    if (state.phase !== 'ready') return []
    const rows = severity ? state.data.filter((o) => o.severity === severity) : state.data
    return [...rows].sort((a, b) => b.date.localeCompare(a.date))
  }, [state, severity])

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const pageRows = filtered.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)

  return (
    <div className="animate-rise">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Ocorrências</h1>
        <p className="mt-1 text-sm text-ink-soft">
          {state.phase === 'ready'
            ? `${filtered.length} ocorrência${filtered.length === 1 ? '' : 's'} encontrada${filtered.length === 1 ? '' : 's'}.`
            : 'Todas as ocorrências detectadas, com filtros por período, severidade e filial.'}
        </p>
      </header>

      <section
        aria-label="Filtros"
        className="mt-6 flex flex-wrap items-end gap-3 rounded-card border border-line bg-bg p-4 shadow-card [&>label]:max-w-full"
      >
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink">Severidade</span>
          <Select value={severity} onChange={(e) => setParam('severity', e.target.value)} className="min-w-40">
            <option value="">Todas</option>
            <option value="CRITICO">Crítico</option>
            <option value="ALERTA">Alerta</option>
            <option value="OPERACIONAL">Operacional</option>
          </Select>
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink">De</span>
          <Input
            type="date"
            value={startDate}
            max={endDate || undefined}
            onChange={(e) => setParam('start_date', e.target.value)}
          />
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink">Até</span>
          <Input
            type="date"
            value={endDate}
            min={startDate || undefined}
            onChange={(e) => setParam('end_date', e.target.value)}
          />
        </label>
        {branches.length > 0 && (
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-ink">Filial</span>
            <Select value={branchId} onChange={(e) => setParam('branch_id', e.target.value)} className="min-w-40">
              <option value="">Todas</option>
              {branches.map((b) => (
                <option key={b.id} value={b.id}>
                  {b.name}
                </option>
              ))}
            </Select>
          </label>
        )}
        {(severity || startDate || endDate || branchId) && (
          <button
            type="button"
            onClick={() => setSearchParams({}, { replace: true })}
            className="flex min-h-11 items-center gap-1.5 rounded-field px-2.5 text-sm font-medium text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink"
          >
            Limpar filtros
          </button>
        )}
      </section>

      <div className="mt-6">
        {state.phase === 'loading' && (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        )}

        {state.phase === 'error' && <ErrorNote message={state.message} onRetry={load} />}

        {state.phase === 'ready' && filtered.length === 0 && (
          <EmptyState
            icon={<ListFilter size={32} strokeWidth={1.5} />}
            title="Nenhuma ocorrência encontrada"
            description="Ajuste os filtros acima — período, severidade ou filial — para ver outras ocorrências."
          />
        )}

        {state.phase === 'ready' && filtered.length > 0 && (
          <>
            <div className="overflow-x-auto rounded-card border border-line bg-bg shadow-card">
              <table className="w-full min-w-[640px] text-left text-sm">
                <thead>
                  <tr className="border-b border-line text-xs font-semibold uppercase tracking-wide text-ink-faint">
                    <th className="px-4 py-3">Colaborador</th>
                    <th className="px-4 py-3">Data</th>
                    <th className="px-4 py-3">Tipo</th>
                    <th className="px-4 py-3">Severidade</th>
                    <th className="px-4 py-3">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {pageRows.map((occ) => (
                    <tr key={occ.id} className="transition-colors duration-150 hover:bg-panel">
                      <td className="px-4 py-3">
                        <Link
                          to={`/colaboradores/${occ.collaborator_id}`}
                          className="font-medium text-ink hover:text-brand hover:underline"
                        >
                          {occ.collaborator_name}
                        </Link>
                      </td>
                      <td className="px-4 py-3 text-ink-soft">{formatDate(occ.date)}</td>
                      <td className="px-4 py-3 text-ink-soft">{occ.type}</td>
                      <td className="px-4 py-3">
                        <SeverityBadge severity={occ.severity} />
                      </td>
                      <td className="px-4 py-3">
                        <OccurrenceStateBadge state={occ.state} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {totalPages > 1 && (
              <div className="mt-3 flex items-center justify-between gap-3">
                <p className="text-xs text-ink-faint">
                  Página {page} de {totalPages}
                </p>
                <div className="flex items-center gap-1">
                  <button
                    type="button"
                    disabled={page <= 1}
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    aria-label="Página anterior"
                    className="flex min-h-11 min-w-11 items-center justify-center rounded-field text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    <ChevronLeft size={16} aria-hidden />
                  </button>
                  <button
                    type="button"
                    disabled={page >= totalPages}
                    onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                    aria-label="Próxima página"
                    className="flex min-h-11 min-w-11 items-center justify-center rounded-field text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    <ChevronRight size={16} aria-hidden />
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
