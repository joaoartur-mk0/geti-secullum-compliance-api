import {
  ArrowLeft,
  Ban,
  Building2,
  CalendarClock,
  Clock,
  OctagonAlert,
  TrendingUp,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import type { ReactNode } from 'react'
import {
  Button,
  CategoryBadge,
  ErrorNote,
  Field,
  OccurrenceStateBadge,
  SeverityBadge,
  Skeleton,
  useToast,
} from '../components/ui'
import WarningPanel from '../components/WarningPanel'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { formatDate, formatDateTime } from '../lib/format'
import type { Collaborator, CollaboratorPrefill, Occurrence } from '../lib/types'

const ALL_STATES = ['aberta', 'atualizada', 'resolvida_automatica', 'resolvida_manual'] as const

interface Data {
  collaborator: Collaborator | null
  occurrences: Occurrence[]
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
  // Autopreenchimento é secundário: uma falha aqui não deve impedir a tela de abrir.
  const [prefill, setPrefill] = useState<CollaboratorPrefill | null>(null)

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    setPrefill(null)
    api
      .getCollaboratorPrefill(tenant.id, secullumId)
      .then(setPrefill)
      .catch(() => setPrefill(null))
    try {
      const [{ collaborators }, { occurrences }] = await Promise.all([
        api.listCollaborators(tenant.id),
        api.listOccurrences(tenant.id, { collaborator_id: secullumId, state: [...ALL_STATES] }),
      ])
      const collaborator = collaborators.find((c) => c.secullum_id === secullumId) ?? null
      setState({ phase: 'ready', data: { collaborator, occurrences } })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar o histórico.',
      })
    }
  }, [tenant.id, secullumId])

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
        <Detail secullumId={secullumId} data={state.data} prefill={prefill} onChanged={load} />
      )}
    </div>
  )
}

function Detail({
  secullumId,
  data,
  prefill,
  onChanged,
}: {
  secullumId: number
  data: Data
  prefill: CollaboratorPrefill | null
  onChanged: () => void
}) {
  const { collaborator, occurrences } = data

  const name =
    collaborator?.name || occurrences[0]?.collaborator_name || prefill?.collaborator.name || `Colaborador ${secullumId}`

  const open = useMemo(() => occurrences.filter((o) => o.state === 'aberta' || o.state === 'atualizada'), [occurrences])
  const critical = useMemo(() => open.filter((o) => o.severity === 'CRITICO').length, [open])
  const lastDate = occurrences[0]?.date ?? null

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
          label="Em aberto"
          value={String(open.length)}
          hint={open.length === 0 ? 'nenhuma ação pendente' : 'aberta ou atualizada'}
          tone={open.length === 0 ? 'ok' : 'neutral'}
        />
        <Stat
          icon={<OctagonAlert size={17} aria-hidden />}
          label="Críticas"
          value={String(critical)}
          hint={critical === 0 ? 'nenhuma' : 'exigem atenção'}
          tone={critical > 0 ? 'critico' : 'ok'}
        />
        <Stat
          icon={<CalendarClock size={17} aria-hidden />}
          label="Última"
          value={lastDate ? formatDate(lastDate) : '—'}
          hint={lastDate ? 'ocorrência mais recente' : 'sem ocorrências'}
          tone="neutral"
        />
      </section>

      <PrefillPanel prefill={prefill} />

      <section aria-label="Histórico de ocorrências" className="mt-8">
        <h2 className="mb-3 text-sm font-semibold text-ink-soft">Histórico de ocorrências</h2>

        {occurrences.length === 0 ? (
          <div className="rounded-card border border-dashed border-line px-6 py-12 text-center">
            <p className="font-semibold text-ink">Nenhuma ocorrência registrada</p>
            <p className="mx-auto mt-1 max-w-md text-sm text-ink-soft">
              Este colaborador esteve em conformidade em todas as varreduras do período.
            </p>
          </div>
        ) : (
          <ul className="flex flex-col gap-3">
            {occurrences.map((occ) => (
              <OccurrenceCard key={occ.id} occurrence={occ} onChanged={onChanged} />
            ))}
          </ul>
        )}
      </section>

      <section aria-label="Advertências" className="mt-8">
        <WarningPanel collaboratorId={secullumId} collaboratorName={name} branchId={prefill?.filial?.id ?? null} />
      </section>
    </>
  )
}

// ---------- Autopreenchimento ----------

function PrefillPanel({ prefill }: { prefill: CollaboratorPrefill | null }) {
  if (!prefill) return null
  const { horario_fixo, filial } = prefill
  if (horario_fixo.length === 0 && !filial) return null

  return (
    <section aria-label="Autopreenchimento" className="mt-6 grid gap-3 sm:grid-cols-2">
      <div className="rounded-card border border-line bg-bg p-4 shadow-card">
        <p className="mb-2 flex items-center gap-1.5 text-sm font-semibold text-ink-soft">
          <Clock size={15} aria-hidden />
          Horário fixo (Secullum)
        </p>
        {horario_fixo.length === 0 ? (
          <p className="text-sm text-ink-faint">Nenhum horário cadastrado.</p>
        ) : (
          <ul className="flex flex-col gap-1 text-sm text-ink-soft">
            {horario_fixo.map((d, i) => (
              <li key={i} className="flex items-center justify-between gap-2 tabular-nums">
                <span>Dia {d.dia_semana}</span>
                <span>
                  {d.entrada_1}–{d.saida_1}
                  {d.entrada_2 && ` · ${d.entrada_2}–${d.saida_2}`}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="rounded-card border border-line bg-bg p-4 shadow-card">
        <p className="mb-2 flex items-center gap-1.5 text-sm font-semibold text-ink-soft">
          <Building2 size={15} aria-hidden />
          Filial
        </p>
        {!filial ? (
          <p className="text-sm text-ink-faint">
            Não foi possível resolver a filial — vincule o aparelho ou o nº de folha em Filiais.
          </p>
        ) : (
          <div className="text-sm">
            <p className="font-medium text-ink">{filial.name}</p>
            <p className="text-ink-soft">
              {filial.manager_name || 'Sem gestor cadastrado'}
              {filial.manager_phone && ` · ${filial.manager_phone}`}
            </p>
            <p className="mt-1 text-xs text-ink-faint">
              {filial.source === 'aparelho'
                ? 'Confirmado pelo aparelho da última batida'
                : filial.source === 'numero_folha'
                  ? 'Inferido pelo nº de folha'
                  : ''}
            </p>
          </div>
        )}
      </div>
    </section>
  )
}

// ---------- Ocorrência individual ----------

function OccurrenceCard({ occurrence, onChanged }: { occurrence: Occurrence; onChanged: () => void }) {
  const toast = useToast()
  const [confirming, setConfirming] = useState(false)
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)

  const canIgnore = occurrence.state === 'aberta' || occurrence.state === 'atualizada'

  async function ignore() {
    setBusy(true)
    try {
      await api.ignoreOccurrence(occurrence.id, reason.trim() || undefined)
      toast('success', 'Ocorrência ignorada.')
      onChanged()
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao ignorar a ocorrência.')
    } finally {
      setBusy(false)
      setConfirming(false)
    }
  }

  return (
    <li className="rounded-card border border-line bg-bg shadow-card">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5 border-b border-line px-4 py-2.5">
        <p className="font-semibold text-ink">{formatDate(occurrence.date)}</p>
        <p className="text-xs text-ink-faint">
          visto {occurrence.times_seen}x · atualizado {formatDateTime(occurrence.last_seen_at)}
        </p>
      </div>
      <div className="flex flex-wrap items-start gap-x-4 gap-y-2 px-4 py-3">
        <div className="min-w-0 flex-1">
          <p className="font-medium text-ink">{occurrence.type}</p>
          <p className="mt-0.5 text-sm text-ink-soft">{occurrence.description}</p>
          {occurrence.state === 'resolvida_manual' && occurrence.ignored_reason && (
            <p className="mt-1 text-xs text-ink-faint">Motivo: {occurrence.ignored_reason}</p>
          )}
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <CategoryBadge category={occurrence.category} />
          <SeverityBadge severity={occurrence.severity} />
          <OccurrenceStateBadge state={occurrence.state} />
        </div>
      </div>

      {canIgnore && (
        <div className="border-t border-line px-4 py-3">
          {confirming ? (
            <div className="flex flex-wrap items-end gap-2">
              <Field label="Motivo (opcional)">
                <input
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  placeholder="Ex.: abonado pelo RH"
                  className="min-h-9 w-56 rounded-field border border-line bg-bg px-3 text-sm text-ink placeholder:text-ink-faint focus:border-brand"
                />
              </Field>
              <Button variant="secondary" busy={busy} onClick={ignore}>
                <Ban size={15} aria-hidden />
                Confirmar
              </Button>
              <Button variant="ghost" onClick={() => setConfirming(false)}>
                Cancelar
              </Button>
            </div>
          ) : (
            <Button variant="ghost" onClick={() => setConfirming(true)}>
              <Ban size={15} aria-hidden />
              Ignorar ocorrência
            </Button>
          )}
        </div>
      )}
    </li>
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
