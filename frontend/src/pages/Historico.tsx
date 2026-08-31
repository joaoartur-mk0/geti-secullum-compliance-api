// Histórico de tratamento — o que teve desfecho no período, e como.
// Contrato: docs/11_Historico_Ranking_Frontend_Contract.md, §9.1.
// Implementação: docs/intern/specs/SPEC-01_Historico.md.

import {
  Ban,
  Building2,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Clock,
  History,
  ListFilter,
  OctagonAlert,
  RotateCcw,
  TrendingUp,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState, type Dispatch, type SetStateAction } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { BranchResolutionNote, PeriodFilters, usePeriodParams } from '../components/PeriodFilters'
import { EmptyState, ErrorNote, OccurrenceStateBadge, Select, SeverityBadge, Skeleton } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import {
  ALL_STATES,
  GRANULARITY_LABEL,
  STATE_LABEL,
  UNASSIGNED_BRANCH_LABEL,
  averageResolutionDays,
  countByState,
  groupByTime,
  rankByBranch,
  type Granularity,
  type OutcomeCounts,
  type RankedGroup,
  type TimeBucket,
} from '../lib/analytics'
import { formatDate } from '../lib/format'
import type { Branch, Occurrence, OccurrenceState } from '../lib/types'

type Loadable<T> =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: T }

const PAGE_SIZE = 20
// Mesmo teto de Indicadores.tsx (MAX_CHART_BARS): acima disso a barra fica fina demais
// para ler, e o eixo deixa de comunicar tendência.
const MAX_CHART_BARS = 62
const GRANULARITIES: Granularity[] = ['dia', 'semana', 'mes']

export default function Historico() {
  const { tenant } = useTenant()
  const [searchParams, setSearchParams] = useSearchParams()
  const { preset, period, setParam, setPreset } = usePeriodParams('30d')

  const branchId = searchParams.get('branch_id') ?? ''
  const stateFilter = (searchParams.get('state') as OccurrenceState | null) ?? ''
  const typeFilter = searchParams.get('type') ?? ''
  const collaboratorFilter = searchParams.get('collaborator_id') ?? ''
  const granularity = (searchParams.get('granularidade') as Granularity | null) ?? 'dia'

  const [branches, setBranches] = useState<Branch[]>([])
  const [state, setState] = useState<Loadable<Occurrence[]>>({ phase: 'loading' })
  const [page, setPage] = useState(1)

  useEffect(() => {
    // Filiais são acessório do filtro: se a chamada falhar, a tela continua sem elas.
    api.listBranches(tenant.id).then(setBranches).catch(() => setBranches([]))
  }, [tenant.id])

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      const { occurrences } = await api.listOccurrences(tenant.id, {
        start_date: period.start,
        end_date: period.end,
        // Sem `state`, a API devolve só aberta+atualizada — a tela inteira perderia o
        // sentido (SPEC-01 §1). Desfecho/tipo/colaborador são recortados no cliente logo
        // abaixo, sobre esta mesma lista.
        state: ALL_STATES,
        branch_id: branchId ? Number(branchId) : undefined,
      })
      setState({ phase: 'ready', data: occurrences })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar o histórico de tratamento.',
      })
    }
  }, [tenant.id, period.start, period.end, branchId])

  useEffect(() => {
    void load()
  }, [load])

  // Qualquer troca de filtro volta pra primeira página — evita cair numa página vazia.
  useEffect(() => {
    setPage(1)
  }, [stateFilter, typeFilter, collaboratorFilter, period.start, period.end, branchId])

  function setSearchParam(key: string, value: string) {
    const next = new URLSearchParams(searchParams)
    if (value) next.set(key, value)
    else next.delete(key)
    setSearchParams(next, { replace: true })
  }

  // Memoizado de propósito: o literal `[]` do ramo "ainda não pronto" seria recriado a
  // cada render e invalidaria todos os useMemo abaixo, refazendo as agregações da lista
  // inteira à toa.
  const rawList = useMemo(() => (state.phase === 'ready' ? state.data : []), [state])

  // Opções dos seletores vêm do resultado CARREGADO (antes do recorte cliente), não do já
  // filtrado — senão a lista de opções encolheria a cada filtro aplicado, escondendo as
  // outras escolhas possíveis.
  const typeOptions = useMemo(
    () => [...new Set(rawList.map((o) => o.type))].sort((a, b) => a.localeCompare(b, 'pt-BR')),
    [rawList],
  )

  const collaboratorOptions = useMemo(() => {
    const map = new Map<number, string>()
    for (const o of rawList) map.set(o.collaborator_id, o.collaborator_name)
    return [...map.entries()]
      .map(([id, name]) => ({ id, name }))
      .sort((a, b) => a.name.localeCompare(b.name, 'pt-BR'))
  }, [rawList])

  // Desfecho, tipo e colaborador filtram no cliente (SPEC-01 §2): a API não filtra por
  // tipo, e recarregar por causa de um <select> piora a experiência.
  const filtered = useMemo(
    () =>
      rawList
        .filter((o) => !stateFilter || o.state === stateFilter)
        .filter((o) => !typeFilter || o.type === typeFilter)
        .filter((o) => !collaboratorFilter || String(o.collaborator_id) === collaboratorFilter),
    [rawList, stateFilter, typeFilter, collaboratorFilter],
  )

  const sortedList = useMemo(
    () => [...filtered].sort((a, b) => b.date.localeCompare(a.date)),
    [filtered],
  )
  const totalPages = Math.max(1, Math.ceil(sortedList.length / PAGE_SIZE))
  const pageRows = sortedList.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)

  const counts = useMemo(() => countByState(filtered), [filtered])
  const resolution = useMemo(() => averageResolutionDays(filtered), [filtered])
  // Medida Período (não Exposição): a pergunta da tela é o que ACONTECEU, não o que está
  // pendente agora — ver contrato §3.
  const branchGroups = useMemo(() => rankByBranch(filtered, 'periodo'), [filtered])
  // Últimos MAX_CHART_BARS baldes: groupByTime devolve em ordem crescente, então o corte
  // fica no início (mais antigo), preservando os mais recentes.
  const buckets = useMemo(
    () => groupByTime(filtered, granularity).slice(-MAX_CHART_BARS),
    [filtered, granularity],
  )

  const hasActiveFilters = Boolean(
    stateFilter || typeFilter || collaboratorFilter || branchId || preset !== '30d',
  )

  return (
    <div className="animate-rise">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Histórico de tratamento</h1>
        <p className="mt-1 text-sm text-ink-soft">O que teve desfecho no período, e como.</p>
      </header>

      <PeriodFilters preset={preset} period={period} onPreset={setPreset} onParam={setParam} branches={branches} branchId={branchId}>
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink">Desfecho</span>
          <Select value={stateFilter} onChange={(e) => setSearchParam('state', e.target.value)} className="min-w-40">
            <option value="">Todos</option>
            {ALL_STATES.map((s) => (
              <option key={s} value={s}>
                {STATE_LABEL[s]}
              </option>
            ))}
          </Select>
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink">Tipo</span>
          <Select value={typeFilter} onChange={(e) => setSearchParam('type', e.target.value)} className="min-w-40">
            <option value="">Todos</option>
            {typeOptions.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </Select>
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink">Colaborador</span>
          <Select
            value={collaboratorFilter}
            onChange={(e) => setSearchParam('collaborator_id', e.target.value)}
            className="min-w-40"
          >
            <option value="">Todos</option>
            {collaboratorOptions.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </Select>
        </label>
        {hasActiveFilters && (
          <button
            type="button"
            onClick={() => setSearchParams({}, { replace: true })}
            className="flex min-h-11 items-center gap-1.5 rounded-field px-2.5 text-sm font-medium text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink"
          >
            Limpar filtros
          </button>
        )}
      </PeriodFilters>

      <div className="mt-6">
        {state.phase === 'loading' && (
          <div className="flex flex-col gap-3">
            <Skeleton className="h-32 w-full" />
            <Skeleton className="h-24 w-full" />
            <Skeleton className="h-48 w-full" />
          </div>
        )}

        {state.phase === 'error' && <ErrorNote message={state.message} onRetry={load} />}

        {state.phase === 'ready' && filtered.length === 0 && (
          <EmptyState
            icon={<History size={32} strokeWidth={1.5} />}
            title="Nenhuma ocorrência com desfecho no período"
            description="Ajuste o período ou os filtros acima para ver outro recorte."
          />
        )}

        {state.phase === 'ready' && filtered.length > 0 && (
          <div className="flex flex-col gap-6">
            <OutcomeSummary counts={counts} total={filtered.length} />
            <ResolutionTime resolution={resolution} />
            <TrendSection
              buckets={buckets}
              granularity={granularity}
              onGranularity={(g) => setSearchParam('granularidade', g)}
            />
            <BranchBreakdown groups={branchGroups} ignored={counts.resolvida_manual} />
            <OccurrenceList
              rows={pageRows}
              total={sortedList.length}
              page={page}
              totalPages={totalPages}
              onPage={setPage}
            />
          </div>
        )}
      </div>
    </div>
  )
}

// ---------- Resumo por desfecho (3.1) ----------

// Mesma paleta de OccurrenceStateBadge (components/ui.tsx) — o cartão é só uma versão
// maior do mesmo sinal, não um significado novo. aberta/atualizada dividem a cor
// "revisar" ali também: as duas ainda pedem ação, só o ícone muda.
const STATE_CARD_ICON: Record<OccurrenceState, typeof OctagonAlert> = {
  aberta: OctagonAlert,
  atualizada: RotateCcw,
  resolvida_automatica: CheckCircle2,
  resolvida_manual: Ban,
}

const STATE_CARD_CLASSES: Record<OccurrenceState, { bg: string; text: string }> = {
  aberta: { bg: 'bg-revisar-bg', text: 'text-revisar' },
  atualizada: { bg: 'bg-revisar-bg', text: 'text-revisar' },
  resolvida_automatica: { bg: 'bg-ok-bg', text: 'text-ok' },
  resolvida_manual: { bg: 'bg-panel', text: 'text-ink-soft' },
}

// Descrições travadas pela SPEC-01 §3.1 — não reformular, é o que evita alguém ler
// "Ignorada" como "resolvida".
const STATE_CARD_DESCRIPTION: Record<OccurrenceState, string> = {
  aberta: 'Sem desfecho.',
  atualizada: 'O valor mudou desde a última varredura.',
  resolvida_automatica: 'O dado foi ajustado na Secullum.',
  resolvida_manual: 'Um usuário registrou que não procedia.',
}

function OutcomeSummary({ counts, total }: { counts: OutcomeCounts; total: number }) {
  return (
    <section aria-label="Resumo por desfecho" className="rounded-card border border-line bg-bg p-5 shadow-card">
      <h2 className="text-sm font-semibold text-ink">Resumo por desfecho</h2>
      <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <div className="rounded-card border border-line p-4">
          <p className="text-xs font-medium uppercase tracking-wide text-ink-faint">Total</p>
          <p className="mt-2 text-2xl font-semibold tabular-nums text-ink">{total}</p>
          <p className="mt-1 text-xs text-ink-soft">Todos os desfechos no período.</p>
        </div>
        {ALL_STATES.map((s) => {
          const Icon = STATE_CARD_ICON[s]
          const classes = STATE_CARD_CLASSES[s]
          return (
            <div key={s} className="rounded-card border border-line p-4">
              <p
                className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold ${classes.bg} ${classes.text}`}
              >
                <Icon size={12} aria-hidden />
                {STATE_LABEL[s]}
              </p>
              <p className="mt-2 text-2xl font-semibold tabular-nums text-ink">{counts[s]}</p>
              <p className="mt-1 text-xs text-ink-soft">{STATE_CARD_DESCRIPTION[s]}</p>
            </div>
          )
        })}
      </div>
      <p className="mt-4 text-xs text-ink-faint">
        O desfecho "tratada", com justificativa e anexo, ainda não existe no sistema.
      </p>
    </section>
  )
}

// ---------- Tempo médio até o desfecho (3.2) ----------

const daysFormatter = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 })

function ResolutionTime({ resolution }: { resolution: { days: number; count: number } }) {
  return (
    <section aria-label="Tempo médio até o desfecho" className="rounded-card border border-line bg-bg p-5 shadow-card">
      <div className="flex items-center gap-2">
        <Clock size={16} className="text-ink-faint" aria-hidden />
        <h2 className="text-sm font-semibold text-ink">Tempo médio até o desfecho</h2>
      </div>
      {resolution.count === 0 ? (
        // Zero ocorrência com desfecho é ausência de dado, não "zero dias" — as duas
        // afirmações são diferentes (contrato §7.2) e não podem se confundir na tela.
        <p className="mt-3 text-sm text-ink-soft">— nenhuma ocorrência com desfecho no período.</p>
      ) : (
        <>
          <p className="mt-3 text-2xl font-semibold tabular-nums text-ink">
            {daysFormatter.format(resolution.days)} <span className="text-sm font-normal text-ink-soft">dias</span>
          </p>
          <p className="mt-1 text-xs text-ink-soft">
            Média sobre {resolution.count} ocorrência{resolution.count === 1 ? '' : 's'} com desfecho.
          </p>
        </>
      )}
    </section>
  )
}

// ---------- Evolução no tempo (3.3) ----------

// Reaproveita a cor "revisar" para os dois estados ainda em aberto (mesma família da
// OccurrenceStateBadge), só com opacidade menor para diferenciar visualmente as duas
// barras empilhadas — evita reusar "alerta"/"crítico" aqui, que já significam severidade
// no restante da tela (a lista em 3.5).
const STATE_BAR_CLASSES: Record<OccurrenceState, string> = {
  aberta: 'bg-revisar',
  atualizada: 'bg-revisar/50',
  resolvida_automatica: 'bg-ok',
  resolvida_manual: 'bg-ink-faint',
}

function bucketLabel(key: string, granularity: Granularity): string {
  if (granularity === 'mes') {
    const [y, m] = key.split('-')
    return `${m}/${y}`
  }
  const [, m, d] = key.split('-')
  const label = `${d}/${m}`
  return granularity === 'semana' ? `sem. ${label}` : label
}

function TrendSection({
  buckets,
  granularity,
  onGranularity,
}: {
  buckets: TimeBucket[]
  granularity: Granularity
  onGranularity: (g: Granularity) => void
}) {
  const max = buckets.reduce((m, b) => Math.max(m, b.occurrences.length), 0)

  return (
    <section aria-label="Evolução no tempo" className="rounded-card border border-line bg-bg p-5 shadow-card">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <TrendingUp size={16} className="text-ink-faint" aria-hidden />
          <h2 className="text-sm font-semibold text-ink">Evolução no tempo</h2>
        </div>
        <div className="flex items-center gap-1.5">
          {GRANULARITIES.map((g) => (
            <button
              key={g}
              type="button"
              onClick={() => onGranularity(g)}
              aria-pressed={granularity === g}
              className={`flex min-h-9 items-center rounded-field px-3 text-xs font-medium transition-colors duration-150 ${
                granularity === g
                  ? 'bg-brand text-white'
                  : 'border border-line text-ink-soft hover:border-ink-faint hover:text-ink'
              }`}
            >
              {GRANULARITY_LABEL[g]}
            </button>
          ))}
        </div>
      </div>

      <div className="mb-3 flex flex-wrap items-center gap-3 text-xs text-ink-soft">
        {ALL_STATES.map((s) => (
          <span key={s} className="inline-flex items-center gap-1.5">
            <span className={`h-2 w-2 rounded-full ${STATE_BAR_CLASSES[s]}`} aria-hidden />
            {STATE_LABEL[s]}
          </span>
        ))}
      </div>

      {/* overflow-x-auto + min-w-8 por coluna: com granularidade "Dia" num período de 30+
          dias, as colunas não podem encolher abaixo do legível — o rótulo de data virava um
          único dígito no mobile. Em vez disso a faixa rola horizontalmente dentro do card
          (mesmo padrão do gráfico de evolução em Indicadores). */}
      <div className="overflow-x-auto">
        <div className="flex h-40 items-stretch gap-1">
          {buckets.map((bucket) => {
            const byState = countByState(bucket.occurrences)
            const total = bucket.occurrences.length
            const heightPct = max > 0 ? (total / max) * 100 : 0
            return (
              <div
                key={bucket.key}
                className="flex min-w-8 flex-1 flex-col items-center gap-1.5"
                title={`${bucketLabel(bucket.key, granularity)} — ${total} ocorrência${total === 1 ? '' : 's'}`}
              >
                <div className="flex w-full flex-1 items-end justify-center">
                  <div
                    className="flex w-full max-w-8 flex-col-reverse overflow-hidden rounded-t-sm transition-[height] duration-500 ease-out"
                    style={{ height: `${Math.max(heightPct, 4)}%` }}
                  >
                    {ALL_STATES.map((s) =>
                      byState[s] > 0 ? (
                        <div key={s} className={STATE_BAR_CLASSES[s]} style={{ height: `${(byState[s] / total) * 100}%` }} />
                      ) : null,
                    )}
                  </div>
                </div>
                <span className="w-full truncate text-center text-[10px] tabular-nums text-ink-faint">
                  {bucketLabel(bucket.key, granularity)}
                </span>
              </div>
            )
          })}
        </div>
      </div>
    </section>
  )
}

// ---------- Quebra por filial (3.4) ----------

function BranchBreakdown({ groups, ignored }: { groups: RankedGroup[]; ignored: number }) {
  const totalAll = groups.reduce((sum, g) => sum + g.breakdown.total, 0)

  return (
    <section aria-label="Quebra por filial" className="rounded-card border border-line bg-bg p-5 shadow-card">
      <div className="mb-1 flex items-center gap-2">
        <Building2 size={16} className="text-ink-faint" aria-hidden />
        <h2 className="text-sm font-semibold text-ink">Quebra por filial</h2>
      </div>
      <p className="mb-4 text-xs text-ink-faint">
        Medida: Período — o que aconteceu no intervalo (em aberto + a reconferir + corrigida na origem).
        Ignoradas não entram aqui, pelo mesmo motivo por que não pontuam em nenhuma medida.
      </p>

      {groups.length === 0 ? (
        <p className="rounded-card border border-dashed border-line px-4 py-6 text-center text-sm text-ink-soft">
          Nenhuma ocorrência conta para a medida Período neste recorte — só há ignoradas, que ficam de fora.
        </p>
      ) : (
        <div className="overflow-x-auto rounded-card border border-line">
          <table className="w-full min-w-[520px] text-left text-sm">
            <thead>
              <tr className="border-b border-line text-xs font-semibold uppercase tracking-wide text-ink-faint">
                <th className="px-4 py-3">Filial</th>
                <th className="px-4 py-3">Total</th>
                <th className="px-4 py-3">Críticas</th>
                <th className="px-4 py-3">Alertas</th>
                <th className="px-4 py-3">Operacionais</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {groups.map((g) => (
                <tr key={g.key}>
                  <td className="px-4 py-3 font-medium text-ink">{g.label}</td>
                  <td className="px-4 py-3 tabular-nums text-ink-soft">{g.breakdown.total}</td>
                  <td className="px-4 py-3 tabular-nums text-ink-soft">{g.breakdown.critical}</td>
                  <td className="px-4 py-3 tabular-nums text-ink-soft">{g.breakdown.alert}</td>
                  <td className="px-4 py-3 tabular-nums text-ink-soft">{g.breakdown.operational}</td>
                </tr>
              ))}
            </tbody>
            <tfoot>
              <tr className="border-t border-line font-semibold text-ink">
                <td className="px-4 py-3">Total geral</td>
                <td className="px-4 py-3 tabular-nums">{totalAll}</td>
                <td className="px-4 py-3" colSpan={3} />
              </tr>
            </tfoot>
          </table>
        </div>
      )}

      {/* Reconciliação explícita: o total desta tabela é MENOR que o total do resumo
          acima, porque as ignoradas ficam de fora da medida Período. Sem esta linha, o
          leitor vê dois totais diferentes na mesma tela e conclui que um deles está
          errado — invariante §10.4 do contrato: a soma das partes tem que bater, e quando
          não bate por regra, a regra aparece na tela. */}
      {ignored > 0 && (
        <p className="mt-3 text-xs text-ink-faint">
          Total geral {totalAll} = {totalAll + ignored} do resumo acima menos {ignored}{' '}
          {ignored === 1 ? 'ignorada' : 'ignoradas'}, que não entram na medida Período.
        </p>
      )}

      <BranchResolutionNote />
    </section>
  )
}

// ---------- Lista (3.5) ----------

function OccurrenceList({
  rows,
  total,
  page,
  totalPages,
  onPage,
}: {
  rows: Occurrence[]
  total: number
  page: number
  totalPages: number
  onPage: Dispatch<SetStateAction<number>>
}) {
  return (
    <section aria-label="Lista de ocorrências" className="rounded-card border border-line bg-bg shadow-card">
      <div className="flex items-center gap-2 border-b border-line p-5">
        <ListFilter size={16} className="text-ink-faint" aria-hidden />
        <h2 className="text-sm font-semibold text-ink">
          Ocorrências no período <span className="font-normal text-ink-faint">({total})</span>
        </h2>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[760px] text-left text-sm">
          <thead>
            <tr className="border-b border-line text-xs font-semibold uppercase tracking-wide text-ink-faint">
              <th className="px-4 py-3">Colaborador</th>
              <th className="px-4 py-3">Data</th>
              <th className="px-4 py-3">Tipo</th>
              <th className="px-4 py-3">Severidade</th>
              <th className="px-4 py-3">Desfecho</th>
              <th className="px-4 py-3">Filial</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {rows.map((occ) => (
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
                  {occ.state === 'resolvida_manual' && occ.ignored_reason && (
                    <p className="mt-1 text-xs text-ink-faint">{occ.ignored_reason}</p>
                  )}
                </td>
                <td className="px-4 py-3 text-ink-soft">{occ.filial?.name ?? UNASSIGNED_BRANCH_LABEL}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-between gap-3 border-t border-line px-5 py-3">
          <p className="text-xs text-ink-faint">
            Página {page} de {totalPages}
          </p>
          <div className="flex items-center gap-1">
            <button
              type="button"
              disabled={page <= 1}
              onClick={() => onPage((p) => Math.max(1, p - 1))}
              aria-label="Página anterior"
              className="flex min-h-11 min-w-11 items-center justify-center rounded-field text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink disabled:cursor-not-allowed disabled:opacity-40"
            >
              <ChevronLeft size={16} aria-hidden />
            </button>
            <button
              type="button"
              disabled={page >= totalPages}
              onClick={() => onPage((p) => Math.min(totalPages, p + 1))}
              aria-label="Próxima página"
              className="flex min-h-11 min-w-11 items-center justify-center rounded-field text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink disabled:cursor-not-allowed disabled:opacity-40"
            >
              <ChevronRight size={16} aria-hidden />
            </button>
          </div>
        </div>
      )}
    </section>
  )
}
