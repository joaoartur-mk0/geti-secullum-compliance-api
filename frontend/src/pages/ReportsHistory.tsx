// Histórico completo de auditorias — todas as execuções, sem deduplicar por dia (isso é
// Histórico de varreduras, em /auditorias, que mostra só a mais recente). Aqui dá para ver
// cada varredura já rodada, inclusive reauditorias do mesmo dia.
//
// Consome GET /tenants/:id/reports/history, com filtro de período (start_date/end_date) —
// dá para consultar semanas/meses completos, além de um único dia (De = Até).

import { ChevronDown, ChevronUp, History, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { EmptyState, ErrorNote, Input, SeverityBadge, Skeleton } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { formatDate, formatDateTime, yesterday } from '../lib/format'
import type { Report } from '../lib/types'

type Loadable<T> =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: T }

type SortDir = 'asc' | 'desc'

export default function ReportsHistory() {
  const { tenant } = useTenant()
  const [reports, setReports] = useState<Loadable<Report[]>>({ phase: 'loading' })
  const [startDate, setStartDate] = useState('')
  const [endDate, setEndDate] = useState('')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const [expandedId, setExpandedId] = useState<number | null>(null)

  const load = useCallback(async () => {
    setReports({ phase: 'loading' })
    try {
      setReports({
        phase: 'ready',
        data: await api.listReportHistory(tenant.id, {
          start_date: startDate || undefined,
          end_date: endDate || undefined,
        }),
      })
    } catch (error) {
      setReports({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar auditorias.',
      })
    }
  }, [tenant.id, startDate, endDate])

  useEffect(() => {
    void load()
  }, [load])

  const rows = useMemo(() => {
    if (reports.phase !== 'ready') return []
    const sorted = [...reports.data].sort((a, b) => a.date.localeCompare(b.date))
    return sortDir === 'asc' ? sorted : sorted.reverse()
  }, [reports, sortDir])

  const total = reports.phase === 'ready' ? reports.data.length : 0
  const filtering = startDate !== '' || endDate !== ''

  return (
    <div className="animate-rise">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Histórico de auditorias</h1>
          <p className="mt-1 text-sm text-ink-soft">
            {reports.phase === 'ready'
              ? `${total} varredura${total === 1 ? '' : 's'} no histórico — inclui reauditorias do mesmo dia.`
              : 'Histórico completo de varreduras, com data, hora, ocorrências detectadas e quem disparou.'}
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
          <span className="text-sm font-medium text-ink">De</span>
          <Input
            type="date"
            value={startDate}
            max={endDate || yesterday()}
            onChange={(e) => setStartDate(e.target.value)}
            aria-label="Início do período"
          />
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink">Até</span>
          <Input
            type="date"
            value={endDate}
            min={startDate || undefined}
            max={yesterday()}
            onChange={(e) => setEndDate(e.target.value)}
            aria-label="Fim do período"
          />
        </label>
        {filtering && (
          <button
            type="button"
            onClick={() => {
              setStartDate('')
              setEndDate('')
            }}
            className="flex min-h-11 items-center gap-1.5 rounded-field px-2.5 text-sm font-medium text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink"
          >
            <X size={15} aria-hidden />
            Limpar filtro
          </button>
        )}
      </div>

      <div className="mt-6">
        {reports.phase === 'loading' && (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        )}

        {reports.phase === 'error' && <ErrorNote message={reports.message} onRetry={load} />}

        {reports.phase === 'ready' && reports.data.length === 0 && !filtering && (
          <EmptyState
            icon={<History size={32} strokeWidth={1.5} />}
            title="Nenhuma auditoria ainda"
            description='Dispare uma auditoria em Histórico de varreduras — cada execução aparece aqui, mesmo que o mesmo dia seja auditado mais de uma vez.'
          />
        )}

        {reports.phase === 'ready' && reports.data.length === 0 && filtering && (
          <p className="text-center text-sm text-ink-soft">
            Nenhuma auditoria encontrada para o período selecionado.
          </p>
        )}

        {rows.length > 0 && (
          <div className="overflow-x-auto rounded-card border border-line bg-bg shadow-card">
            <table className="w-full min-w-[560px] text-left text-sm">
              <thead>
                <tr className="border-b border-line text-xs font-semibold uppercase tracking-wide text-ink-faint">
                  <th className="px-4 py-3">
                    <button
                      type="button"
                      onClick={() => setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))}
                      className="flex items-center gap-1 hover:text-ink"
                    >
                      Data
                      {sortDir === 'asc' ? (
                        <ChevronUp size={13} aria-hidden />
                      ) : (
                        <ChevronDown size={13} aria-hidden />
                      )}
                    </button>
                  </th>
                  <th className="px-4 py-3">Hora</th>
                  <th className="px-4 py-3">Ocorrências</th>
                  <th className="px-4 py-3">Disparada por</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {rows.map((report) => (
                  <ReportHistoryRow
                    key={report.id}
                    report={report}
                    expanded={expandedId === report.id}
                    onToggle={() => setExpandedId((id) => (id === report.id ? null : report.id))}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

function ReportHistoryRow({
  report,
  expanded,
  onToggle,
}: {
  report: Report
  expanded: boolean
  onToggle: () => void
}) {
  const inconsistencies = report.inconsistencies ?? []
  const criticos = inconsistencies.filter((i) => i.Severity === 'CRITICO').length

  return (
    <>
      <tr className="cursor-pointer transition-colors duration-150 hover:bg-panel" onClick={onToggle}>
        <td className="px-4 py-3 font-medium text-ink">{formatDate(report.date)}</td>
        <td className="px-4 py-3 text-ink-soft">{formatDateTime(report.data_generated).split(' ').pop()}</td>
        <td className="px-4 py-3">
          {report.total === 0 ? (
            <span className="inline-flex items-center rounded-full bg-ok-bg px-2.5 py-1 text-xs font-semibold text-ok">
              Sem inconsistências
            </span>
          ) : (
            <span className="text-ink-soft">
              {report.total} {criticos > 0 && <span className="text-critico">({criticos} crítica{criticos > 1 ? 's' : ''})</span>}
            </span>
          )}
        </td>
        <td className="px-4 py-3 text-ink-faint">—</td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={4} className="border-t border-line bg-panel/40 px-4 py-3">
            {inconsistencies.length === 0 ? (
              <p className="text-sm text-ink-soft">
                Todas as batidas do dia estavam em conformidade com as regras ativas.
              </p>
            ) : (
              <ul className="divide-y divide-line">
                {inconsistencies.map((item, index) => (
                  <li key={index} className="flex flex-wrap items-start gap-x-4 gap-y-1 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="font-medium text-ink">{item.CollaboratorName}</p>
                      <p className="mt-0.5 text-sm text-ink-soft">
                        {item.Type} — {item.Description}
                      </p>
                    </div>
                    <SeverityBadge severity={item.Severity} />
                  </li>
                ))}
              </ul>
            )}
          </td>
        </tr>
      )}
    </>
  )
}
