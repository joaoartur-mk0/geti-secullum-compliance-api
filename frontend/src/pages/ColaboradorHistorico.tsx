import {
  ArrowLeft,
  Ban,
  Building2,
  CalendarClock,
  ChevronLeft,
  ChevronRight,
  Clock,
  Fingerprint,
  OctagonAlert,
  PenLine,
  TrendingUp,
  Watch,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import type { ReactNode } from 'react'
import {
  Button,
  CategoryBadge,
  EmptyState,
  ErrorNote,
  Field,
  OccurrenceStateBadge,
  Select,
  SeverityBadge,
  Skeleton,
  useToast,
} from '../components/ui'
import WarningPanel from '../components/WarningPanel'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { formatDate, formatDateTime, formatPhone } from '../lib/format'
import { findLink, MISSING_PAYROLL, setFilial } from '../lib/lotacao'
import { isoDaysAgo, isoStartOfMonth, today } from '../lib/periods'
import type { Period } from '../lib/periods'
import type {
  Branch,
  CollaboratorHistoryEntry,
  CollaboratorPrefill,
  Equipment,
  Occurrence,
  PunchRecord,
} from '../lib/types'

const ALL_STATES = ['aberta', 'atualizada', 'resolvida_automatica', 'resolvida_manual'] as const

interface Data {
  collaborator: CollaboratorHistoryEntry | null
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
      // Histórico completo (não só ativos): a página individual precisa continuar
      // acessível para quem foi desligado — é aqui que o gestor confere a última
      // ocorrência dele antes do desligamento.
      const [{ collaborators }, { occurrences }] = await Promise.all([
        api.listCollaboratorsHistory(tenant.id),
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
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
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
        {/* Os dois números são diferentes e já foram confundidos: o da Secullum identifica
            a pessoa nas telas, o de folha é o que lota numa filial. Nomear cada um evita
            que o de cima seja digitado no campo do de baixo. */}
        <p className="mt-1 text-sm text-ink-soft">
          ID Secullum {secullumId}
          {prefill?.collaborator.numero_folha && ` · folha ${prefill.collaborator.numero_folha}`}
          {collaborator?.demitido && collaborator.demissao && ` · desligado em ${formatDate(collaborator.demissao)}`}
          {collaborator == null && ' · não consta mais entre os sincronizados'}
        </p>
      </header>

      <section aria-label="Resumo de ocorrências" className="mt-6 grid grid-cols-2 gap-3 lg:grid-cols-3">
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

      <PrefillPanel prefill={prefill} onLinked={onChanged} />

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

      <OrigemMarcacoes secullumId={secullumId} occurrences={occurrences} />

      <section aria-label="Advertências" className="mt-8">
        <WarningPanel collaboratorId={secullumId} collaboratorName={name} branchId={prefill?.filial?.id ?? null} />
      </section>
    </>
  )
}

// ---------- Autopreenchimento ----------

function PrefillPanel({ prefill, onLinked }: { prefill: CollaboratorPrefill | null; onLinked: () => void }) {
  if (!prefill) return null
  const { horario_fixo } = prefill

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

      <BranchCard prefill={prefill} onLinked={onLinked} />
    </section>
  )
}

// BranchCard deixa lotar o colaborador direto daqui, em vez de obrigar a anotar o nº de
// folha e ir cadastrá-lo na tela de Filiais.
//
// O vínculo continua sendo gravado como nº de folha (é o que o backend resolve): o número
// usado é o `numero_folha` que o próprio prefill trouxe, nunca um digitado à mão — foi
// justamente a digitação manual que colocou um id da Secullum no lugar da folha.
function BranchCard({ prefill, onLinked }: { prefill: CollaboratorPrefill; onLinked: () => void }) {
  const { tenant } = useTenant()
  const toast = useToast()
  const [branches, setBranches] = useState<Branch[] | null>(null)
  const [saving, setSaving] = useState(false)

  const numeroFolha = prefill.collaborator.numero_folha ?? ''
  const filial = prefill.filial

  useEffect(() => {
    let alive = true
    api
      .listBranches(tenant.id)
      .then((list) => alive && setBranches(list))
      .catch(() => alive && setBranches([]))
    return () => {
      alive = false
    }
  }, [tenant.id])

  // A filial resolvida pelo APARELHO não vem de um vínculo de nº de folha, então o select
  // não pode fingir que ela é a lotação cadastrada. Nesse caso a origem manda, e o que o
  // select mostra é o vínculo real (que pode não existir).
  const linked = branches ? findLink(branches, numeroFolha) : null

  async function change(value: string) {
    if (!branches) return
    setSaving(true)
    try {
      await setFilial(numeroFolha, value ? Number(value) : null, branches)
      toast('success', value ? 'Colaborador lotado na filial.' : 'Colaborador desvinculado da filial.')
      setBranches(await api.listBranches(tenant.id))
      onLinked()
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao alterar a filial.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="rounded-card border border-line bg-bg p-4 shadow-card">
      <p className="mb-2 flex items-center gap-1.5 text-sm font-semibold text-ink-soft">
        <Building2 size={15} aria-hidden />
        Filial
      </p>

      {branches === null ? (
        <Skeleton className="h-9 w-full" />
      ) : branches.length === 0 ? (
        <p className="text-sm text-ink-faint">
          Nenhuma filial cadastrada.{' '}
          <Link to="/filiais" className="font-semibold text-brand underline underline-offset-2">
            Cadastrar filiais
          </Link>
        </p>
      ) : !numeroFolha ? (
        <p className="text-sm text-ink-faint">{MISSING_PAYROLL}</p>
      ) : (
        <Select
          aria-label="Filial do colaborador"
          value={linked ? String(linked.branchId) : ''}
          disabled={saving}
          onChange={(e) => void change(e.target.value)}
        >
          <option value="">Sem filial</option>
          {branches.map((b) => (
            <option key={b.id} value={b.id}>
              {b.name}
            </option>
          ))}
        </Select>
      )}

      {filial && (
        <div className="mt-2 text-sm">
          <p className="text-ink-soft">
            {filial.manager_name || 'Sem gestor cadastrado'}
            {filial.manager_phone && ` · ${formatPhone(filial.manager_phone)}`}
          </p>
          <p className="mt-1 text-xs text-ink-faint">
            {filial.source === 'aparelho'
              ? `Resolvido como ${filial.name}, confirmado pelo aparelho da última batida`
              : 'Resolvido pelo nº de folha'}
          </p>
        </div>
      )}

      {!filial && branches !== null && branches.length > 0 && numeroFolha && !linked && (
        <p className="mt-2 text-xs text-ink-faint">
          Sem lotação — escolha a filial acima para vincular pelo nº de folha {numeroFolha}.
        </p>
      )}
    </div>
  )
}

// ---------- Origem das marcações ----------
//
// SPEC-05: bloco novo, alimentado por api.listPunchRecords, que nunca teve UI. Carrega
// separado do resto da ficha (Loadable próprio) porque uma falha aqui não pode impedir a
// ficha de abrir — mesmo espírito do prefill acima.

// Preset próprio deste bloco: a ficha não tem filtro de período em querystring hoje, e
// criar um mudaria comportamento existente da página (proibido pela spec). "90 dias" não
// existe como preset em lib/periods.ts — compomos a partir de isoDaysAgo, que é exportada.
type OrigemPreset = '30' | '90' | 'mes'

const ORIGEM_PRESET_LABEL: Record<OrigemPreset, string> = {
  '30': '30 dias',
  '90': '90 dias',
  mes: 'Este mês',
}

function origemPeriod(preset: OrigemPreset): Period {
  switch (preset) {
    case '30':
      return { start: isoDaysAgo(30), end: today() }
    case '90':
      return { start: isoDaysAgo(90), end: today() }
    case 'mes':
      return { start: isoStartOfMonth(), end: today() }
  }
}

interface OrigemData {
  records: PunchRecord[]
  equipamentos: Equipment[]
}

type OrigemLoadable =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: OrigemData }

const ORIGEM_PAGE_SIZE = 20

function OrigemMarcacoes({ secullumId, occurrences }: { secullumId: number; occurrences: Occurrence[] }) {
  const { tenant } = useTenant()
  const [preset, setPreset] = useState<OrigemPreset>('30')
  const [state, setState] = useState<OrigemLoadable>({ phase: 'loading' })
  const [page, setPage] = useState(1)

  const period = useMemo(() => origemPeriod(preset), [preset])

  const load = useCallback(() => {
    setState({ phase: 'loading' })
    Promise.all([api.listPunchRecords(tenant.id, secullumId, period.start, period.end), api.listEquipamentos(tenant.id)])
      .then(([records, { equipamentos }]) => setState({ phase: 'ready', data: { records, equipamentos } }))
      .catch((error) => {
        setState({
          phase: 'error',
          message: error instanceof ApiError ? error.message : 'Erro ao carregar a origem das marcações.',
        })
      })
  }, [tenant.id, secullumId, period.start, period.end])

  useEffect(() => {
    load()
  }, [load])

  // Troca de período volta pra primeira página — mesmo motivo do Incidentes.tsx: evita
  // cair numa página vazia depois que a lista muda de tamanho.
  useEffect(() => {
    setPage(1)
  }, [preset])

  // Cruzamento com as ocorrências que a ficha já carregou (SPEC-05 §3.4) — sem consulta nova.
  const occurrenceDates = useMemo(() => new Set(occurrences.map((o) => o.date)), [occurrences])

  return (
    <section aria-label="Origem das marcações" className="mt-8">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-sm font-semibold text-ink-soft">Origem das marcações</h2>
        <Select
          aria-label="Período da origem das marcações"
          value={preset}
          onChange={(e) => setPreset(e.target.value as OrigemPreset)}
          className="min-h-9 py-0 text-xs"
        >
          {(Object.keys(ORIGEM_PRESET_LABEL) as OrigemPreset[]).map((p) => (
            <option key={p} value={p}>
              {ORIGEM_PRESET_LABEL[p]}
            </option>
          ))}
        </Select>
      </div>

      {state.phase === 'loading' && (
        <div className="flex flex-col gap-3">
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-20 w-full" />
            ))}
          </div>
          <Skeleton className="h-40 w-full" />
        </div>
      )}

      {state.phase === 'error' && <ErrorNote message={state.message} onRetry={load} />}

      {state.phase === 'ready' && (
        <OrigemMarcacoesContent data={state.data} occurrenceDates={occurrenceDates} page={page} setPage={setPage} />
      )}
    </section>
  )
}

function OrigemMarcacoesContent({
  data,
  occurrenceDates,
  page,
  setPage,
}: {
  data: OrigemData
  occurrenceDates: Set<string>
  page: number
  setPage: (updater: (p: number) => number) => void
}) {
  const { records, equipamentos } = data

  const equipmentBySecullumId = useMemo(() => new Map(equipamentos.map((e) => [e.secullum_id, e])), [equipamentos])

  function equipmentLabel(id: number | null): string {
    if (id == null) return '—'
    // Espelho pode estar desatualizado em relação à Secullum — nunca deixar em branco
    // (SPEC-05 §2): o número cru ainda identifica o relógio, mesmo sem descrição.
    const eq = equipmentBySecullumId.get(id)
    return eq ? eq.descricao : `Equipamento #${id}`
  }

  const sorted = useMemo(() => [...records].sort((a, b) => b.date.localeCompare(a.date)), [records])

  const total = sorted.length
  const fromClock = useMemo(() => sorted.filter((r) => r.equipamento_id != null).length, [sorted])
  const manual = useMemo(() => sorted.filter((r) => r.motivo != null).length, [sorted])

  const totalPages = Math.max(1, Math.ceil(total / ORIGEM_PAGE_SIZE))
  const pageRows = sorted.slice((page - 1) * ORIGEM_PAGE_SIZE, page * ORIGEM_PAGE_SIZE)

  if (total === 0) {
    return (
      <EmptyState
        icon={<Fingerprint size={32} strokeWidth={1.5} />}
        title="Nenhuma origem apurada no período"
        description="Isso significa que a auditoria não encontrou correspondência na Secullum — não que houve falta."
      />
    )
  }

  return (
    <>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
        <Stat
          icon={<Fingerprint size={17} aria-hidden />}
          label="Dias com origem apurada"
          value={String(total)}
          hint="registros no período"
          tone="neutral"
        />
        <Stat
          icon={<Watch size={17} aria-hidden />}
          label="De relógio"
          value={String(fromClock)}
          hint="batida no equipamento"
          tone="neutral"
        />
        <Stat
          icon={<PenLine size={17} aria-hidden />}
          label="Manual / abono"
          value={String(manual)}
          hint="inserida à mão"
          tone="neutral"
        />
      </div>

      <div className="mt-4 overflow-x-auto rounded-card border border-line bg-bg shadow-card">
        <table className="w-full min-w-[480px] text-left text-sm">
          <thead>
            <tr className="border-b border-line text-xs font-semibold uppercase tracking-wide text-ink-faint">
              <th className="px-4 py-3">Data</th>
              <th className="px-4 py-3">Equipamento</th>
              <th className="px-4 py-3">Motivo</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {pageRows.map((r) => {
              // A marca só faz sentido quando a origem foi manual: batida de relógio num
              // dia com ocorrência não tem a mesma explicação (SPEC-05 §3.4).
              const withOccurrence = r.motivo != null && occurrenceDates.has(r.date)
              return (
                <tr key={r.date} className="transition-colors duration-150 hover:bg-panel">
                  <td className="px-4 py-3 text-ink-soft">
                    <div className="flex flex-wrap items-center gap-2">
                      <span>{formatDate(r.date)}</span>
                      {withOccurrence && (
                        <span className="inline-flex items-center rounded-field bg-panel px-2 py-0.5 text-xs font-medium text-ink-soft">
                          dia com ocorrência
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-ink-soft">{equipmentLabel(r.equipamento_id)}</td>
                  <td className="px-4 py-3 text-ink-soft">{r.motivo ?? '—'}</td>
                </tr>
              )
            })}
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
