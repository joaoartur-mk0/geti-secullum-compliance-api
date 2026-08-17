// Histórico completo de auditorias — todas as execuções, sem deduplicar por dia, e os
// controles para disparar uma nova varredura (agora ou para um dia específico).

import { Activity, CalendarSearch, History, PlayCircle, RefreshCw, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import ReportRow from '../components/ReportRow'
import { Button, EmptyState, ErrorNote, Input, Skeleton, useToast } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { formatDate, yesterday } from '../lib/format'
import type { HealthResponse, Report } from '../lib/types'

type Loadable<T> =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: T }

export default function Auditorias() {
  const { tenant } = useTenant()
  const toast = useToast()
  const [reports, setReports] = useState<Loadable<Report[]>>({ phase: 'loading' })
  const [health, setHealth] = useState<HealthResponse | null>(null)
  const [filterDate, setFilterDate] = useState('')
  const [triggering, setTriggering] = useState(false)
  const [pickingDate, setPickingDate] = useState(false)
  const [pickedDate, setPickedDate] = useState('')
  const [triggeringDate, setTriggeringDate] = useState(false)

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
    api.health().then(setHealth).catch(() => setHealth(null))
  }, [load])

  async function triggerAudit() {
    setTriggering(true)
    try {
      await api.triggerAudit(tenant.id)
      toast(
        'success',
        'Auditoria enfileirada. O processamento é assíncrono — atualize a lista em instantes.',
      )
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao disparar a auditoria.')
    } finally {
      setTriggering(false)
    }
  }

  // Auditoria de um dia específico: não substitui a varredura diária automática, só
  // permite conferir a situação de um dia passado sob demanda.
  async function triggerAuditForDate() {
    if (!pickedDate) return
    setTriggeringDate(true)
    try {
      const result = await api.triggerAudit(tenant.id, pickedDate)
      toast(
        'success',
        `Auditoria de ${formatDate(result.date)} enfileirada. Atualize a lista em instantes.`,
      )
      setPickingDate(false)
      setPickedDate('')
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao disparar a auditoria.')
    } finally {
      setTriggeringDate(false)
    }
  }

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
        <div className="flex items-center gap-2">
          <HealthChip health={health} />
          <Button
            variant="secondary"
            onClick={() => setPickingDate((v) => !v)}
            aria-expanded={pickingDate}
          >
            <CalendarSearch size={17} aria-hidden />
            Auditar um dia
          </Button>
          <Button onClick={triggerAudit} busy={triggering}>
            <PlayCircle size={17} aria-hidden />
            Auditar agora
          </Button>
        </div>
      </header>

      {pickingDate && (
        <div className="mt-4 flex flex-wrap items-end gap-2 rounded-card border border-brand/30 bg-brand-soft/40 p-4 animate-rise">
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-ink">Auditar um dia específico</span>
            <Input
              type="date"
              value={pickedDate}
              max={yesterday()}
              onChange={(e) => setPickedDate(e.target.value)}
            />
          </label>
          <Button onClick={triggerAuditForDate} busy={triggeringDate} disabled={!pickedDate}>
            Enfileirar
          </Button>
          <Button variant="ghost" onClick={() => setPickingDate(false)}>
            Cancelar
          </Button>
          <p className="w-full text-xs text-ink-soft">
            Não substitui a varredura diária automática — serve para conferir a situação de um dia já
            encerrado sob demanda.
          </p>
        </div>
      )}

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
        <button
          type="button"
          onClick={load}
          className="flex min-h-11 items-center gap-1.5 rounded-field px-2.5 text-sm font-medium text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink"
        >
          <RefreshCw size={15} aria-hidden />
          Atualizar
        </button>
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
            description='As auditorias rodam nos horários configurados em Avisos, ou dispare uma agora com o botão "Auditar agora" acima. O resultado aparece aqui, mesmo que o mesmo dia seja auditado mais de uma vez.'
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

function HealthChip({ health }: { health: HealthResponse | null }) {
  const up = health?.status === 'ok'
  return (
    <span
      title={health ? `banco: ${health.database} · fila: ${health.rabbitmq}` : 'API fora do ar'}
      className={`inline-flex min-h-11 items-center gap-2 rounded-field border px-3 text-sm font-medium ${
        up ? 'border-ok/25 bg-ok-bg text-ok' : 'border-critico/25 bg-critico-bg text-critico'
      }`}
    >
      <Activity size={15} className={up ? 'animate-pulse-dot' : undefined} aria-hidden />
      {up ? 'Serviço ativo' : 'Fora do ar'}
    </span>
  )
}
