import { Activity, CalendarSearch, ChevronDown, ClipboardList, PlayCircle, RefreshCw } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Button, EmptyState, ErrorNote, Input, SeverityBadge, Skeleton, useToast } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { formatDate, formatDateTime } from '../lib/format'
import type { HealthResponse, Report } from '../lib/types'

// Só datas já encerradas (antes de hoje) podem ser auditadas sob demanda — o backend
// recusa hoje/futuro (ver TriggerRequest.resolveDate no handler de auditoria).
function yesterday(): string {
  const d = new Date()
  d.setDate(d.getDate() - 1)
  return d.toISOString().slice(0, 10)
}

type Loadable<T> =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: T }

export default function Painel() {
  const { tenant } = useTenant()
  const toast = useToast()
  const [reports, setReports] = useState<Loadable<Report[]>>({ phase: 'loading' })
  const [health, setHealth] = useState<HealthResponse | null>(null)
  const [triggering, setTriggering] = useState(false)
  const [pickingDate, setPickingDate] = useState(false)
  const [pickedDate, setPickedDate] = useState('')
  const [triggeringDate, setTriggeringDate] = useState(false)

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
          <h1 className="text-2xl font-semibold tracking-tight">Painel</h1>
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

      <section className="mt-8" aria-label="Relatórios de auditoria">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-ink-soft">Relatórios recentes</h2>
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

function ReportRow({ report }: { report: Report }) {
  const [open, setOpen] = useState(false)
  const inconsistencies = report.inconsistencies ?? []
  const criticos = inconsistencies.filter((i) => i.Severity === 'CRITICO').length
  const alertas = inconsistencies.length - criticos
  const clean = report.total === 0

  return (
    <li className="rounded-card border border-line bg-bg shadow-card">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex w-full flex-wrap items-center gap-x-4 gap-y-1 rounded-card px-4 py-3.5 text-left transition-colors duration-150 hover:bg-panel"
      >
        <div className="min-w-32">
          <p className="font-semibold">{formatDate(report.date)}</p>
          <p className="text-xs text-ink-faint">gerado {formatDateTime(report.data_generated)}</p>
        </div>
        <div className="flex flex-1 flex-wrap items-center gap-2 text-sm">
          {clean ? (
            <span className="inline-flex items-center gap-1 rounded-full bg-ok-bg px-2.5 py-1 text-xs font-semibold text-ok">
              Sem inconsistências
            </span>
          ) : (
            <>
              {criticos > 0 && (
                <span className="inline-flex items-center rounded-full bg-critico-bg px-2.5 py-1 text-xs font-semibold text-critico">
                  {criticos} crítica{criticos > 1 ? 's' : ''}
                </span>
              )}
              {alertas > 0 && (
                <span className="inline-flex items-center rounded-full bg-alerta-bg px-2.5 py-1 text-xs font-semibold text-alerta">
                  {alertas} alerta{alertas > 1 ? 's' : ''}
                </span>
              )}
            </>
          )}
        </div>
        <ChevronDown
          size={18}
          aria-hidden
          className={`shrink-0 text-ink-faint transition-transform duration-200 ${open ? 'rotate-180' : ''}`}
        />
      </button>

      {open && (
        <div className="border-t border-line px-4 py-3">
          {inconsistencies.length === 0 ? (
            <p className="py-2 text-sm text-ink-soft">
              Todas as batidas do dia estavam em conformidade com as regras ativas.
            </p>
          ) : (
            <ul className="divide-y divide-line">
              {inconsistencies.map((item, index) => (
                <li key={index} className="flex flex-wrap items-start gap-x-4 gap-y-1 py-3">
                  <div className="min-w-0 flex-1">
                    <p className="font-medium">{item.CollaboratorName}</p>
                    <p className="mt-0.5 text-sm text-ink-soft">
                      {item.Type} — {item.Description}
                    </p>
                  </div>
                  <SeverityBadge severity={item.Severity} />
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </li>
  )
}
