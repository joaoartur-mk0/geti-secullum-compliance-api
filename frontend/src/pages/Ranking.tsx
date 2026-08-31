// Ranking — onde a exposição se concentra.
// Contrato: docs/11_Historico_Ranking_Frontend_Contract.md, §2, §3, §7 e §9.2.
// Spec: docs/intern/specs/SPEC-02_Ranking.md.
//
// Regra que define a tela: OPERACIONAL pesa 0 (não é infração trabalhista, é sinal de
// investigação) e por isso NÃO entra na pontuação — mas nunca some: fica em coluna própria,
// ao lado da pontuação, em toda tabela. `resolvida_manual` (ignorada) nunca pontua, em
// nenhuma medida — essa conta já está pronta em lib/severity.ts (accumulate) e
// lib/analytics.ts (rankByCollaborator/rankByBranch/improvement); esta página só monta a
// tela em cima dela, sem reimplementar nada.

import { ChevronLeft, ChevronRight, LandPlot, TrendingDown, Users } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { BranchResolutionNote, PeriodFilters, usePeriodParams } from '../components/PeriodFilters'
import { EmptyState, ErrorNote, Skeleton } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import {
  UNASSIGNED_BRANCH,
  improvement,
  rankByBranch,
  rankByCollaborator,
  type ImprovementRow,
  type RankedGroup,
} from '../lib/analytics'
import { formatDate } from '../lib/format'
import { daysInPeriod, previousPeriod } from '../lib/periods'
import { MEASURE_LABEL, statesForMeasure, type ScoreMeasure } from '../lib/severity'
import type { Branch, Occurrence } from '../lib/types'

type Loadable<T> =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: T }

type Aba = 'colaboradores' | 'filiais' | 'melhora'

const ABAS: { value: Aba; label: string }[] = [
  { value: 'colaboradores', label: 'Colaboradores' },
  { value: 'filiais', label: 'Filiais' },
  { value: 'melhora', label: 'Melhora' },
]

const PAGE_SIZE = 20

// Textos fixos da medida ativa — ver SPEC-02_Ranking.md §3. Não estão em lib/severity.ts
// porque MEASURE_LABEL já serve de rótulo do botão; esta é a frase de apoio ao lado dele.
const MEASURE_EXPLANATION: Record<ScoreMeasure, string> = {
  exposicao: 'Conta o que está pendente agora (em aberto e a reconferir).',
  periodo: 'Conta tudo que aconteceu no intervalo, inclusive o que já foi corrigido na origem.',
}

export default function Ranking() {
  const { tenant } = useTenant()
  const { preset, period, setParam, setPreset, params } = usePeriodParams('30d')

  const branchId = params.get('branch_id') ?? ''
  const aba = (params.get('aba') as Aba | null) ?? 'colaboradores'
  const measureParam = (params.get('medida') as ScoreMeasure | null) ?? 'exposicao'
  // Na aba Melhora a medida não é escolha do usuário: é a única que permite comparar mês
  // contra mês (ver lib/severity.ts). Forçar aqui em vez de gravar na URL preserva a
  // medida que o usuário tinha escolhido ao voltar para as outras abas.
  const measure: ScoreMeasure = aba === 'melhora' ? 'periodo' : measureParam

  // Cálculo de datas, não vale a pena memoizar — e memoizar por period.start/period.end
  // (em vez do objeto period, recriado a cada render por usePeriodParams) só trocaria um
  // warning de deps por outro.
  const anteriorPeriod = previousPeriod(period)

  const [branches, setBranches] = useState<Branch[]>([])
  const [rankState, setRankState] = useState<Loadable<Occurrence[]>>({ phase: 'loading' })
  const [melhoraState, setMelhoraState] = useState<
    Loadable<{ atual: Occurrence[]; anterior: Occurrence[] }>
  >({ phase: 'loading' })
  const [page, setPage] = useState(1)

  useEffect(() => {
    api.listBranches(tenant.id).then(setBranches).catch(() => setBranches([]))
  }, [tenant.id])

  const loadRank = useCallback(async () => {
    setRankState({ phase: 'loading' })
    try {
      const { occurrences } = await api.listOccurrences(tenant.id, {
        start_date: period.start,
        end_date: period.end,
        state: statesForMeasure(measure),
        branch_id: branchId ? Number(branchId) : undefined,
      })
      setRankState({ phase: 'ready', data: occurrences })
    } catch (error) {
      setRankState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar o ranking.',
      })
    }
  }, [tenant.id, period.start, period.end, measure, branchId])

  const loadMelhora = useCallback(async () => {
    setMelhoraState({ phase: 'loading' })
    try {
      // Duas chamadas em paralelo — período selecionado e o imediatamente anterior de
      // mesma duração, ambas na medida Período (SPEC-02_Ranking.md §2).
      const [atualResp, anteriorResp] = await Promise.all([
        api.listOccurrences(tenant.id, {
          start_date: period.start,
          end_date: period.end,
          state: statesForMeasure('periodo'),
          branch_id: branchId ? Number(branchId) : undefined,
        }),
        api.listOccurrences(tenant.id, {
          start_date: anteriorPeriod.start,
          end_date: anteriorPeriod.end,
          state: statesForMeasure('periodo'),
          branch_id: branchId ? Number(branchId) : undefined,
        }),
      ])
      setMelhoraState({
        phase: 'ready',
        data: { atual: atualResp.occurrences, anterior: anteriorResp.occurrences },
      })
    } catch (error) {
      setMelhoraState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar a comparação de períodos.',
      })
    }
  }, [tenant.id, period.start, period.end, anteriorPeriod.start, anteriorPeriod.end, branchId])

  // Cada aba busca só o que precisa: Colaboradores/Filiais reaproveitam a mesma consulta
  // (mudam só a agregação, no cliente); Melhora precisa das duas janelas de período.
  useEffect(() => {
    if (aba !== 'melhora') void loadRank()
  }, [aba, loadRank])

  useEffect(() => {
    if (aba === 'melhora') void loadMelhora()
  }, [aba, loadMelhora])

  // Qualquer troca de aba/medida/filtro volta pra primeira página — evita cair numa página vazia.
  useEffect(() => {
    setPage(1)
  }, [aba, measure, branchId, period.start, period.end])

  const rankRows = useMemo<RankedGroup[]>(() => {
    if (rankState.phase !== 'ready') return []
    return aba === 'filiais' ? rankByBranch(rankState.data, measure) : rankByCollaborator(rankState.data, measure)
  }, [rankState, aba, measure])

  const melhora = useMemo<{ rows: ImprovementRow[]; excluded: number } | null>(() => {
    if (melhoraState.phase !== 'ready') return null
    const atual = rankByCollaborator(melhoraState.data.atual, 'periodo')
    const anterior = rankByCollaborator(melhoraState.data.anterior, 'periodo')
    return improvement(atual, anterior)
  }, [melhoraState])

  return (
    <div className="animate-rise">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Ranking</h1>
        <p className="mt-1 text-sm text-ink-soft">Onde a exposição se concentra.</p>
      </header>

      {/* Notas fixas — sempre visíveis, nunca em tooltip/acordeão (SPEC-02_Ranking.md §5). */}
      <div className="mt-6 flex flex-col gap-1.5 rounded-card border border-line bg-panel p-4 text-sm text-ink-soft">
        <p>Pontuação bruta — não ajustada por dias trabalhados.</p>
        <p>Ocorrências operacionais não pontuam: são sinal para investigar, não infração.</p>
        <p className="font-medium text-ink">Use para investigar causa, não para punir isoladamente.</p>
        <p className="mt-1 text-xs text-ink-faint">
          Pesos — <span className="font-medium text-critico">Crítico 10</span> ·{' '}
          <span className="font-medium text-alerta">Alerta 3</span> ·{' '}
          <span className="font-medium text-operacional">Operacional 0</span>
        </p>
      </div>

      <PeriodFilters
        preset={preset}
        period={period}
        onPreset={setPreset}
        onParam={setParam}
        branches={branches}
        branchId={branchId}
      />

      {/* Medida — sempre visível: um ranking cuja base de cálculo é implícita é um ranking
          que ninguém consegue contestar (SPEC-02_Ranking.md §3). */}
      <section className="mt-4 rounded-card border border-line bg-bg p-4 shadow-card">
        <span className="text-sm font-medium text-ink">Medida</span>
        {aba === 'melhora' ? (
          <p className="mt-1.5 text-sm text-ink-soft">
            Travada em <strong className="text-ink">Período</strong> nesta aba: comparar exposição entre
            períodos mediria só o que ainda não foi resolvido, não a melhora real.
          </p>
        ) : (
          <div className="mt-1.5 flex flex-wrap items-center gap-3">
            <div className="flex flex-wrap items-center gap-1.5">
              {(['exposicao', 'periodo'] as ScoreMeasure[]).map((m) => (
                <button
                  key={m}
                  type="button"
                  onClick={() => setParam('medida', m)}
                  aria-pressed={measure === m}
                  className={`flex min-h-11 items-center rounded-field px-3 text-sm font-medium transition-colors duration-150 ${
                    measure === m
                      ? 'bg-brand text-white'
                      : 'border border-line text-ink-soft hover:border-ink-faint hover:text-ink'
                  }`}
                >
                  {MEASURE_LABEL[m]}
                </button>
              ))}
            </div>
            <p className="text-xs text-ink-soft">{MEASURE_EXPLANATION[measure]}</p>
          </div>
        )}
      </section>

      <div className="mt-6 flex flex-wrap gap-1.5 border-b border-line" role="tablist" aria-label="Ranking">
        {ABAS.map((a) => (
          <button
            key={a.value}
            type="button"
            role="tab"
            aria-selected={aba === a.value}
            onClick={() => setParam('aba', a.value)}
            className={`flex min-h-11 items-center px-4 text-sm font-medium transition-colors duration-150 ${
              aba === a.value
                ? 'border-b-2 border-brand text-brand'
                : 'border-b-2 border-transparent text-ink-soft hover:text-ink'
            }`}
          >
            {a.label}
          </button>
        ))}
      </div>

      <div className="mt-6">
        {aba === 'colaboradores' && (
          <RankSection
            state={rankState}
            rows={rankRows}
            onRetry={loadRank}
            page={page}
            onPageChange={setPage}
            entityLabel="Colaborador"
            linkToCollaborator
            emptyIcon={<Users size={32} strokeWidth={1.5} />}
            emptyTitle="Nenhum colaborador pontuado"
            emptyDescription="Nenhuma ocorrência no período e filtros atuais gerou pontuação ou contagem operacional."
          />
        )}

        {aba === 'filiais' && (
          <>
            <RankSection
              state={rankState}
              rows={rankRows}
              onRetry={loadRank}
              page={page}
              onPageChange={setPage}
              entityLabel="Filial"
              emptyIcon={<LandPlot size={32} strokeWidth={1.5} />}
              emptyTitle="Nenhuma filial pontuada"
              emptyDescription="Nenhuma ocorrência no período e filtros atuais gerou pontuação ou contagem operacional."
            />
            {rankState.phase === 'ready' && rankRows.length > 0 && daysInPeriod(period) > 1 && (
              <BranchResolutionNote />
            )}
          </>
        )}

        {aba === 'melhora' && (
          <>
            <p className="mb-4 text-sm text-ink-soft">
              Comparando{' '}
              <strong className="text-ink">
                {formatDate(anteriorPeriod.start)} – {formatDate(anteriorPeriod.end)}
              </strong>{' '}
              (período anterior) com{' '}
              <strong className="text-ink">
                {formatDate(period.start)} – {formatDate(period.end)}
              </strong>{' '}
              (período atual), na medida Período. Ordenado por maior redução.
            </p>

            {melhoraState.phase === 'loading' && <TableSkeleton />}
            {melhoraState.phase === 'error' && <ErrorNote message={melhoraState.message} onRetry={loadMelhora} />}
            {melhoraState.phase === 'ready' && melhora && melhora.rows.length === 0 && (
              <EmptyState
                icon={<TrendingDown size={32} strokeWidth={1.5} />}
                title="Nenhuma comparação possível"
                description="Ninguém apareceu nos dois períodos, ou não há ocorrências que pontuem na medida Período."
              />
            )}
            {melhoraState.phase === 'ready' && melhora && melhora.rows.length > 0 && (
              <>
                {melhora.excluded > 0 && (
                  <p className="mb-3 text-sm text-ink-soft">{excludedNote(melhora.excluded)}</p>
                )}
                <ImprovementTable rows={melhora.rows} page={page} onPageChange={setPage} />
              </>
            )}
          </>
        )}
      </div>
    </div>
  )
}

// excludedNote formata a nota obrigatória de quem ficou de fora da comparação (SPEC-02_Ranking.md §4.3).
function excludedNote(n: number): string {
  if (n === 1) return '1 colaborador ficou de fora por não aparecer nos dois períodos.'
  return `${n} colaboradores ficaram de fora por não aparecerem nos dois períodos.`
}

// ---------- Tabela de ranking (Colaboradores / Filiais — mesmas colunas) ----------

function RankSection({
  state,
  rows,
  onRetry,
  page,
  onPageChange,
  entityLabel,
  linkToCollaborator,
  emptyIcon,
  emptyTitle,
  emptyDescription,
}: {
  state: Loadable<Occurrence[]>
  rows: RankedGroup[]
  onRetry: () => void
  page: number
  onPageChange: (p: number) => void
  entityLabel: string
  linkToCollaborator?: boolean
  emptyIcon: React.ReactNode
  emptyTitle: string
  emptyDescription: string
}) {
  if (state.phase === 'loading') return <TableSkeleton />
  if (state.phase === 'error') return <ErrorNote message={state.message} onRetry={onRetry} />
  if (rows.length === 0) {
    return <EmptyState icon={emptyIcon} title={emptyTitle} description={emptyDescription} />
  }

  return (
    <RankTable rows={rows} page={page} onPageChange={onPageChange} entityLabel={entityLabel} linkToCollaborator={linkToCollaborator} />
  )
}

function RankTable({
  rows,
  page,
  onPageChange,
  entityLabel,
  linkToCollaborator,
}: {
  rows: RankedGroup[]
  page: number
  onPageChange: (p: number) => void
  entityLabel: string
  linkToCollaborator?: boolean
}) {
  // Barra proporcional relativa ao maior valor da LISTA INTEIRA, não só da página — senão
  // a barra muda de escala a cada página e deixa de comparar o que promete comparar.
  const maxScore = rows.reduce((m, r) => Math.max(m, r.breakdown.score), 0)
  const totalPages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE))
  const pageRows = rows.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)

  return (
    <>
      <div className="overflow-x-auto rounded-card border border-line bg-bg shadow-card">
        <table className="w-full min-w-[680px] text-left text-sm">
          <thead>
            <tr className="border-b border-line text-xs font-semibold uppercase tracking-wide text-ink-faint">
              <th className="px-4 py-3">#</th>
              <th className="px-4 py-3">{entityLabel}</th>
              <th className="px-4 py-3">Pontuação</th>
              <th className="px-4 py-3">Críticas</th>
              <th className="px-4 py-3">Alertas</th>
              <th className="px-4 py-3" title="Não entra na pontuação: é sinal de investigação, não infração trabalhista.">
                Operacionais
              </th>
              <th className="px-4 py-3">Total</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {pageRows.map((row, index) => {
              const rank = (page - 1) * PAGE_SIZE + index + 1
              const isUnassigned = row.key === UNASSIGNED_BRANCH
              const widthPct = maxScore > 0 ? (row.breakdown.score / maxScore) * 100 : 0
              return (
                <tr key={row.key} className="transition-colors duration-150 hover:bg-panel">
                  <td className="px-4 py-3 text-ink-faint">{rank}</td>
                  <td
                    className="px-4 py-3 font-medium text-ink"
                    title={isUnassigned ? 'Aparelho não cadastrado ou colaborador sem lotação.' : undefined}
                  >
                    {linkToCollaborator ? (
                      <Link to={`/colaboradores/${row.key}`} className="hover:text-brand hover:underline">
                        {row.label}
                      </Link>
                    ) : (
                      row.label
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <span className="w-8 shrink-0 text-right tabular-nums font-semibold text-ink">
                        {row.breakdown.score}
                      </span>
                      <div className="h-2 w-24 overflow-hidden rounded-full bg-panel" aria-hidden>
                        <div
                          className="h-full rounded-full bg-brand transition-[width] duration-300 ease-out"
                          style={{ width: `${widthPct}%` }}
                        />
                      </div>
                    </div>
                  </td>
                  <td className="px-4 py-3 tabular-nums text-critico">{row.breakdown.critical}</td>
                  <td className="px-4 py-3 tabular-nums text-alerta">{row.breakdown.alert}</td>
                  <td className="px-4 py-3 tabular-nums text-operacional">{row.breakdown.operational}</td>
                  <td className="px-4 py-3 tabular-nums text-ink-soft">{row.breakdown.total}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      {totalPages > 1 && <Pagination page={page} totalPages={totalPages} onChange={onPageChange} />}
    </>
  )
}

// ---------- Tabela de melhora ----------

function ImprovementTable({
  rows,
  page,
  onPageChange,
}: {
  rows: ImprovementRow[]
  page: number
  onPageChange: (p: number) => void
}) {
  const totalPages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE))
  const pageRows = rows.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)

  return (
    <>
      <div className="overflow-x-auto rounded-card border border-line bg-bg shadow-card">
        <table className="w-full min-w-[560px] text-left text-sm">
          <thead>
            <tr className="border-b border-line text-xs font-semibold uppercase tracking-wide text-ink-faint">
              <th className="px-4 py-3">#</th>
              <th className="px-4 py-3">Colaborador</th>
              <th className="px-4 py-3">Período anterior</th>
              <th className="px-4 py-3">Período atual</th>
              <th className="px-4 py-3">Variação</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {pageRows.map((row, index) => {
              const rank = (page - 1) * PAGE_SIZE + index + 1
              // delta negativo = melhora (menos pontuação). Sem verde de "meta batida" —
              // não existe meta cadastrada (contrato §7.3). Só ink-soft (neutro/melhora) e
              // critico (piorou), os mesmos tokens de severidade já usados no resto da tela.
              const deltaClass = row.delta > 0 ? 'text-critico' : 'text-ink-soft'
              return (
                <tr key={row.key} className="transition-colors duration-150 hover:bg-panel">
                  <td className="px-4 py-3 text-ink-faint">{rank}</td>
                  <td className="px-4 py-3 font-medium text-ink">
                    <Link to={`/colaboradores/${row.key}`} className="hover:text-brand hover:underline">
                      {row.label}
                    </Link>
                  </td>
                  <td className="px-4 py-3 tabular-nums text-ink-soft">{row.previous}</td>
                  <td className="px-4 py-3 tabular-nums text-ink-soft">{row.current}</td>
                  <td className={`px-4 py-3 tabular-nums font-semibold ${deltaClass}`}>
                    {row.delta > 0 ? `+${row.delta}` : row.delta}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      {totalPages > 1 && <Pagination page={page} totalPages={totalPages} onChange={onPageChange} />}
    </>
  )
}

// ---------- Compartilhados ----------

function TableSkeleton() {
  return (
    <div className="flex flex-col gap-2">
      <Skeleton className="h-12 w-full" />
      <Skeleton className="h-12 w-full" />
      <Skeleton className="h-12 w-full" />
    </div>
  )
}

function Pagination({
  page,
  totalPages,
  onChange,
}: {
  page: number
  totalPages: number
  onChange: (p: number) => void
}) {
  return (
    <div className="mt-3 flex items-center justify-between gap-3">
      <p className="text-xs text-ink-faint">
        Página {page} de {totalPages}
      </p>
      <div className="flex items-center gap-1">
        <button
          type="button"
          disabled={page <= 1}
          onClick={() => onChange(Math.max(1, page - 1))}
          aria-label="Página anterior"
          className="flex min-h-11 min-w-11 items-center justify-center rounded-field text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink disabled:cursor-not-allowed disabled:opacity-40"
        >
          <ChevronLeft size={16} aria-hidden />
        </button>
        <button
          type="button"
          disabled={page >= totalPages}
          onClick={() => onChange(Math.min(totalPages, page + 1))}
          aria-label="Próxima página"
          className="flex min-h-11 min-w-11 items-center justify-center rounded-field text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink disabled:cursor-not-allowed disabled:opacity-40"
        >
          <ChevronRight size={16} aria-hidden />
        </button>
      </div>
    </div>
  )
}
