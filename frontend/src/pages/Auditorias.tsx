import { Activity, CalendarRange, CalendarSearch, ClipboardList, PlayCircle, RefreshCw } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
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
  const [triggering, setTriggering] = useState(false)
  const [pickingDate, setPickingDate] = useState(false)
  const [pickedDate, setPickedDate] = useState('')
  const [triggeringDate, setTriggeringDate] = useState(false)
  const [pickingRange, setPickingRange] = useState(false)
  const [rangeStart, setRangeStart] = useState('')
  const [rangeEnd, setRangeEnd] = useState('')
  const [triggeringRange, setTriggeringRange] = useState(false)

  // GET /reports já devolve só a mais recente de cada dia (dedupe é feito no backend) —
  // o painel não precisa reprocessar isso.
  const loadReports = useCallback(async () => {
    setReports({ phase: 'loading' })
    try {
      setReports({ phase: 'ready', data: await api.listReports(tenant.id) })
    } catch (error) {
      setReports({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar relatórios.',
      })
    }
  }, [tenant.id])

  useEffect(() => {
    void loadReports()
    api.health().then(setHealth).catch(() => setHealth(null))
  }, [loadReports])

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

  // Auditoria de um período completo (semana, mês ou intervalo customizado, até 62 dias):
  // o backend salva um relatório por dia — não substitui a varredura diária automática,
  // serve para conferir/recompor um trecho de dias já encerrados de uma vez.
  async function triggerAuditForRange() {
    if (!rangeStart || !rangeEnd) return
    setTriggeringRange(true)
    try {
      const result = await api.triggerAuditRange(tenant.id, rangeStart, rangeEnd)
      toast(
        'success',
        `Auditoria de ${formatDate(result.start_date)} a ${formatDate(result.end_date)} enfileirada. Atualize a lista em instantes.`,
      )
      setPickingRange(false)
      setRangeStart('')
      setRangeEnd('')
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao disparar a auditoria de período.')
    } finally {
      setTriggeringRange(false)
    }
  }

  const summary = useMemo(() => {
    if (reports.phase !== 'ready' || reports.data.length === 0) return null
    const latest = reports.data[0]
    const criticos = (latest.inconsistencies ?? []).filter((i) => i.Severity === 'CRITICO').length
    return { latest, criticos }
  }, [reports])

  return (
    <div className="animate-rise">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Situação por dia</h1>
          <p className="mt-1 text-sm text-ink-soft">
            {summary ? (
              <>
                Última auditoria em <strong className="font-semibold text-ink">{formatDate(summary.latest.date)}</strong>
                {' · '}
                {summary.latest.total === 0
                  ? 'nenhuma inconsistência'
                  : `${summary.latest.total} inconsistência${summary.latest.total > 1 ? 's' : ''}${
                      summary.criticos > 0 ? ` (${summary.criticos} crítica${summary.criticos > 1 ? 's' : ''})` : ''
                    }`}
              </>
            ) : (
              'Acompanhe as auditorias diárias de jornada da sua equipe.'
            )}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <HealthChip health={health} />
          <Button
            variant="secondary"
            onClick={() => {
              setPickingDate((v) => !v)
              setPickingRange(false)
            }}
            aria-expanded={pickingDate}
          >
            <CalendarSearch size={17} aria-hidden />
            Auditar um dia
          </Button>
          <Button
            variant="secondary"
            onClick={() => {
              setPickingRange((v) => !v)
              setPickingDate(false)
            }}
            aria-expanded={pickingRange}
          >
            <CalendarRange size={17} aria-hidden />
            Auditar um período
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

      {pickingRange && (
        <div className="mt-4 flex flex-wrap items-end gap-2 rounded-card border border-brand/30 bg-brand-soft/40 p-4 animate-rise">
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-ink">De</span>
            <Input
              type="date"
              value={rangeStart}
              max={rangeEnd || yesterday()}
              onChange={(e) => setRangeStart(e.target.value)}
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-ink">Até</span>
            <Input
              type="date"
              value={rangeEnd}
              min={rangeStart || undefined}
              max={yesterday()}
              onChange={(e) => setRangeEnd(e.target.value)}
            />
          </label>
          <Button onClick={triggerAuditForRange} busy={triggeringRange} disabled={!rangeStart || !rangeEnd}>
            Enfileirar
          </Button>
          <Button variant="ghost" onClick={() => setPickingRange(false)}>
            Cancelar
          </Button>
          <p className="w-full text-xs text-ink-soft">
            Audita cada dia do período separadamente (semana, mês ou intervalo customizado, até 62
            dias) — não substitui a varredura diária automática.
          </p>
        </div>
      )}

      <section className="mt-8" aria-label="Dias auditados">
        <div className="mb-3 flex items-center justify-between">
          <div>
            <h2 className="text-sm font-semibold text-ink-soft">Dias auditados</h2>
            <p className="text-xs text-ink-faint">
              Uma linha por dia, sempre a varredura mais recente.{' '}
              <Link to="/reports/history" className="font-medium text-brand underline underline-offset-2 hover:text-brand-strong">
                Ver registro de execuções
              </Link>
            </p>
          </div>
          <button
            type="button"
            onClick={loadReports}
            className="flex items-center gap-1.5 rounded-field px-2 py-1 text-sm font-medium text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink"
          >
            <RefreshCw size={14} aria-hidden />
            Atualizar
          </button>
        </div>

        {reports.phase === 'loading' && (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </div>
        )}

        {reports.phase === 'error' && <ErrorNote message={reports.message} onRetry={loadReports} />}

        {reports.phase === 'ready' && reports.data.length === 0 && (
          <EmptyState
            icon={<ClipboardList size={32} strokeWidth={1.5} />}
            title="Nenhum relatório ainda"
            description='As auditorias rodam nos horários configurados em Avisos, ou dispare uma agora com o botão "Auditar agora". O resultado aparece aqui.'
            action={
              <Link to="/avisos" className="text-sm font-semibold text-brand underline underline-offset-2 hover:text-brand-strong">
                Configurar horários de varredura
              </Link>
            }
          />
        )}

        {reports.phase === 'ready' && reports.data.length > 0 && (
          <ul className="flex flex-col gap-2">
            {reports.data.map((report) => (
              <ReportRow key={report.id} report={report} />
            ))}
          </ul>
        )}
      </section>
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
