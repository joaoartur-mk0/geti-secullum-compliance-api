// Histórico completo de auditorias — todas as execuções, sem deduplicar por dia (isso é
// o Painel, que mostra só a mais recente de cada dia). Aqui dá para ver, e pesquisar por
// data, cada varredura já rodada — inclusive reauditorias do mesmo dia.

import { History, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import ReportRow from '../components/ReportRow'
import { EmptyState, ErrorNote, Input, Skeleton } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { formatDate, yesterday } from '../lib/format'
import type { Report } from '../lib/types'

type Loadable<T> =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: T }

export default function Auditorias() {
  const { tenant } = useTenant()
  const [reports, setReports] = useState<Loadable<Report[]>>({ phase: 'loading' })
  const [filterDate, setFilterDate] = useState('')

  const load = useCallback(async () => {
    setReports({ phase: 'loading' })
    try {
      setReports({ phase: 'ready', data: await api.listReports(tenant.id) })
    } catch (error) {
      setReports({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar auditorias.',
      })
    }
  }, [tenant.id])

  useEffect(() => {
    void load()
  }, [load])

  const filtered = useMemo(() => {
    if (reports.phase !== 'ready') return []
    if (!filterDate) return reports.data
    return reports.data.filter((r) => r.date.slice(0, 10) === filterDate)
  }, [reports, filterDate])

  const total = reports.phase === 'ready' ? reports.data.length : 0

  return (
    <div className="animate-rise">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Auditorias</h1>
          <p className="mt-1 text-sm text-ink-soft">
            {reports.phase === 'ready'
              ? `${total} varredura${total === 1 ? '' : 's'} no histórico — inclui reauditorias do mesmo dia.`
              : 'Histórico completo de varreduras, da mais recente à mais antiga.'}
          </p>
        </div>
        <button
          type="button"
          onClick={load}
          className="flex min-h-11 items-center gap-1.5 rounded-field px-2.5 text-sm font-medium text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink"
        >
          <History size={15} aria-hidden />
          Atualizar
        </button>
      </header>

      <div className="mt-6 flex flex-wrap items-end gap-2">
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink">Filtrar por dia</span>
          <Input
            type="date"
            value={filterDate}
            max={yesterday()}
            onChange={(e) => setFilterDate(e.target.value)}
            aria-label="Filtrar auditorias por data"
          />
        </label>
        {filterDate && (
          <button
            type="button"
            onClick={() => setFilterDate('')}
            className="flex min-h-11 items-center gap-1.5 rounded-field px-2.5 text-sm font-medium text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink"
          >
            <X size={15} aria-hidden />
            Limpar filtro
          </button>
        )}
      </div>

      {reports.phase === 'loading' && (
        <div className="mt-6 flex flex-col gap-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      )}

      {reports.phase === 'error' && (
        <div className="mt-6">
          <ErrorNote message={reports.message} onRetry={load} />
        </div>
      )}

      {reports.phase === 'ready' && reports.data.length === 0 && (
        <div className="mt-6">
          <EmptyState
            icon={<History size={32} strokeWidth={1.5} />}
            title="Nenhuma auditoria ainda"
            description='Dispare uma auditoria no Painel — cada execução aparece aqui, mesmo que o mesmo dia seja auditado mais de uma vez.'
          />
        </div>
      )}

      {reports.phase === 'ready' && reports.data.length > 0 && filtered.length === 0 && (
        <p className="mt-6 text-center text-sm text-ink-soft">
          Nenhuma auditoria encontrada para {formatDate(filterDate)}.
        </p>
      )}

      {filtered.length > 0 && (
        <ul className="mt-4 flex flex-col gap-2">
          {filtered.map((report) => (
            <ReportRow key={report.id} report={report} />
          ))}
        </ul>
      )}
    </div>
  )
}
