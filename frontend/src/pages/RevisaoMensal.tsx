// Revisão mensal — o que falta para considerar a competência encerrada, por filial.
// NÃO é "fechamento": fechamento é a varredura diária de D-1 (scheduler, alerta de
// WhatsApp). Usar o mesmo nome pro ciclo mensal cria ambiguidade permanente entre time e
// cliente — ver docs/11_Historico_Ranking_Frontend_Contract.md, §1 e §9.3.

import { AlertTriangle, CalendarCheck, CheckCircle2, ClipboardList } from 'lucide-react'
import { useCallback, useEffect, useMemo } from 'react'
import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { BranchResolutionNote } from '../components/PeriodFilters'
import { EmptyState, ErrorNote, Select, Skeleton } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { ALL_STATES, UNASSIGNED_BRANCH, type ReviewConditions, reviewConditionsByBranch } from '../lib/analytics'
import { formatDate } from '../lib/format'
import {
  competenciaCoverage,
  competenciaPeriod,
  competenciasBetween,
  currentCompetencia,
  formatCompetencia,
  listDates,
  today,
  type Competencia,
} from '../lib/periods'
import type { Occurrence, Report } from '../lib/types'

type Loadable<T> = { phase: 'loading' } | { phase: 'error'; message: string } | { phase: 'ready'; data: T }

// Depois de 10 datas a lista vira ruído — "e mais N" carrega a mesma informação sem
// empurrar o resto da tela pra baixo.
const DIAS_LISTADOS_MAX = 10

export default function RevisaoMensal() {
  const { tenant } = useTenant()
  const [searchParams, setSearchParams] = useSearchParams()

  const competencia: Competencia = searchParams.get('competencia') ?? currentCompetencia()
  const period = useMemo(() => competenciaPeriod(competencia), [competencia])

  function setCompetencia(next: string) {
    const nextParams = new URLSearchParams(searchParams)
    nextParams.set('competencia', next)
    setSearchParams(nextParams, { replace: true })
  }

  // ---------- Quais competências oferecer ----------
  //
  // Regra do contrato (§9.3.5): só a partir do primeiro Report do tenant. Sem relatório
  // nenhum, a tela nem oferece seletor — mostrar competência sem nunca ter varredura
  // inventaria uma revisão para um mês que não foi auditado.
  const [availability, setAvailability] = useState<Loadable<Competencia[]>>({ phase: 'loading' })

  const loadAvailability = useCallback(async () => {
    setAvailability({ phase: 'loading' })
    try {
      const reports = await api.listReports(tenant.id)
      if (reports.length === 0) {
        setAvailability({ phase: 'ready', data: [] })
        return
      }
      const earliest = reports.reduce((min, r) => (r.date < min ? r.date : min), reports[0].date)
      setAvailability({ phase: 'ready', data: competenciasBetween(earliest, today()) })
    } catch (error) {
      setAvailability({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar as competências disponíveis.',
      })
    }
  }, [tenant.id])

  useEffect(() => {
    void loadAvailability()
  }, [loadAvailability])

  // ---------- Dado da competência selecionada ----------
  const [state, setState] = useState<Loadable<{ occurrences: Occurrence[]; reports: Report[] }>>({
    phase: 'loading',
  })

  const competenciasDisponiveis = availability.phase === 'ready' ? availability.data : []
  const hasAvailableCompetencia = competenciasDisponiveis.length > 0

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      const [{ occurrences }, reports] = await Promise.all([
        api.listOccurrences(tenant.id, { start_date: period.start, end_date: period.end, state: ALL_STATES }),
        api.listReports(tenant.id, { start_date: period.start, end_date: period.end }),
      ])
      setState({ phase: 'ready', data: { occurrences, reports } })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar a revisão mensal.',
      })
    }
  }, [tenant.id, period.start, period.end])

  useEffect(() => {
    // Só busca quando já se sabe que existe ao menos uma competência disponível — evita
    // pedir dado de um tenant que nunca varreu nada.
    if (hasAvailableCompetencia) void load()
  }, [hasAvailableCompetencia, load])

  const rows = state.phase === 'ready' ? reviewConditionsByBranch(state.data.occurrences) : []

  // Dias sem varredura: condição do TENANT, não da filial — Report não tem filial. Por
  // isso aparece uma vez, acima da tabela (contrato §9.3, item 4.2), nunca por linha.
  // Cobertura conta só os dias ENCERRADOS: a auditoria é de D-1, então hoje nunca tem
  // varredura e apontá-lo como buraco faria a competência corrente parecer incompleta
  // todo dia, para sempre — descoberto ao rodar contra a base de produção.
  const cobertura = useMemo(() => competenciaCoverage(competencia), [competencia])
  const diasEsperados = useMemo(() => (cobertura ? listDates(cobertura) : []), [cobertura])
  const faltando = useMemo(() => {
    if (state.phase !== 'ready') return []
    const varridos = new Set(state.data.reports.map((r) => r.date.slice(0, 10)))
    return diasEsperados.filter((d) => !varridos.has(d))
  }, [state, diasEsperados])

  // Link para /incidents com o filtro equivalente ao número clicado.
  //
  // "Sem filial" fica sem link de propósito: o backend só casa branch_id com filial
  // resolvida (item.Filial != nil), então branch_id=-1 nunca bate com nada e devolveria
  // uma lista vazia — um link que mente é pior que um número sem link (regra dura do
  // SPEC-00: "número que não leva a lugar nenhum obriga o usuário a refazer o filtro na
  // mão", mas um link que leva a um lugar ERRADO é pior ainda).
  function incidentesLink(row: ReviewConditions, onlyOperational: boolean): string | null {
    if (row.branchKey === UNASSIGNED_BRANCH) return null
    const params = new URLSearchParams({
      start_date: period.start,
      end_date: period.end,
      branch_id: String(row.branchKey),
    })
    if (onlyOperational) params.set('severity', 'OPERACIONAL')
    return `/incidents?${params.toString()}`
  }

  return (
    <div className="animate-rise">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Revisão mensal</h1>
        <p className="mt-1 text-sm text-ink-soft">
          O que falta para considerar {formatCompetencia(competencia).toLowerCase()} encerrada, filial a filial.
        </p>
      </header>

      {availability.phase === 'loading' && (
        <div className="mt-6 flex flex-col gap-2">
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      )}

      {availability.phase === 'error' && (
        <div className="mt-6">
          <ErrorNote message={availability.message} onRetry={loadAvailability} />
        </div>
      )}

      {availability.phase === 'ready' && availability.data.length === 0 && (
        <div className="mt-6">
          <EmptyState
            icon={<CalendarCheck size={32} strokeWidth={1.5} />}
            title="Ainda sem varredura"
            description="Este tenant ainda não tem nenhum relatório de varredura registrado. A revisão mensal só é oferecida a partir da primeira varredura."
          />
        </div>
      )}

      {hasAvailableCompetencia && (
        <>
          <section
            aria-label="Competência"
            className="mt-6 flex flex-wrap items-end gap-3 rounded-card border border-line bg-bg p-4 shadow-card"
          >
            <label className="flex flex-col gap-1.5">
              <span className="text-sm font-medium text-ink">Competência</span>
              <Select
                value={competencia}
                onChange={(e) => setCompetencia(e.target.value)}
                className="min-w-48"
              >
                {competenciasDisponiveis.map((c) => (
                  <option key={c} value={c}>
                    {formatCompetencia(c)}
                  </option>
                ))}
              </Select>
            </label>
          </section>

          {state.phase === 'loading' && (
            <div className="mt-6 flex flex-col gap-2">
              <Skeleton className="h-16 w-full" />
              <Skeleton className="h-40 w-full" />
            </div>
          )}

          {state.phase === 'error' && (
            <div className="mt-6">
              <ErrorNote message={state.message} onRetry={load} />
            </div>
          )}

          {state.phase === 'ready' && (
            <>
              {/* Regra dura do contrato (§7.2 e §9.3): enquanto houver dia sem varredura, a
                  competência NUNCA aparece como pronta — nem com todas as contagens
                  zeradas embaixo. Este bloco fica sempre visível, acima da tabela. */}
              <section className="mt-6 rounded-card border border-line bg-bg p-4 shadow-card">
                <div className="flex items-start gap-3">
                  {cobertura === null || faltando.length > 0 ? (
                    <AlertTriangle size={18} className="mt-0.5 shrink-0 text-alerta" aria-hidden />
                  ) : (
                    <CheckCircle2 size={18} className="mt-0.5 shrink-0 text-ok" aria-hidden />
                  )}
                  <div>
                    {/* Competência que ainda não tem nenhum dia encerrado (dia 1º do mês):
                        "0 de 0 dias" com ícone de sucesso leria como "tudo pronto". */}
                    <p className="text-sm font-medium text-ink">
                      {cobertura === null
                        ? 'Nenhum dia desta competência foi encerrado ainda — a auditoria só roda sobre dias já concluídos.'
                        : `${diasEsperados.length - faltando.length} de ${diasEsperados.length} dias encerrados da competência têm varredura.`}
                    </p>
                    {faltando.length > 0 && (
                      <p className="mt-1 text-sm text-ink-soft">
                        Sem varredura em: {faltando.slice(0, DIAS_LISTADOS_MAX).map(formatDate).join(', ')}
                        {faltando.length > DIAS_LISTADOS_MAX
                          ? ` e mais ${faltando.length - DIAS_LISTADOS_MAX}.`
                          : '.'}
                      </p>
                    )}
                  </div>
                </div>
              </section>

              <section className="mt-6">
                <h2 className="text-lg font-semibold text-ink">Condições por filial</h2>
                <BranchResolutionNote />

                {rows.length === 0 ? (
                  <div className="mt-3">
                    <EmptyState
                      icon={<ClipboardList size={32} strokeWidth={1.5} />}
                      title="Nenhuma ocorrência nesta competência"
                      description="Não há ocorrências registradas no trecho da competência já varrido."
                    />
                  </div>
                ) : (
                  <div className="mt-3 overflow-x-auto rounded-card border border-line bg-bg shadow-card">
                    <table className="w-full min-w-[720px] text-left text-sm">
                      <thead>
                        <tr className="border-b border-line text-xs font-semibold uppercase tracking-wide text-ink-faint">
                          <th className="px-4 py-3">Filial</th>
                          <th className="px-4 py-3">Em aberto</th>
                          <th className="px-4 py-3">A reconferir</th>
                          <th className="px-4 py-3">Operacionais em aberto</th>
                          <th className="px-4 py-3">Situação</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-line">
                        {rows.map((row) => {
                          const pendente = row.open > 0 || row.toRecheck > 0 || row.operationalOpen > 0
                          return (
                            <tr key={row.branchKey} className="transition-colors duration-150 hover:bg-panel">
                              <td className="px-4 py-3 font-medium text-ink">{row.branchLabel}</td>
                              <td className="px-4 py-3">
                                <CountCell value={row.open} href={incidentesLink(row, false)} />
                              </td>
                              <td className="px-4 py-3">
                                <CountCell value={row.toRecheck} href={incidentesLink(row, false)} />
                              </td>
                              <td className="px-4 py-3">
                                <CountCell value={row.operationalOpen} href={incidentesLink(row, true)} />
                              </td>
                              <td className="px-4 py-3 text-ink">{pendente ? 'Pendente' : 'Sem pendência'}</td>
                            </tr>
                          )
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
              </section>

              <section className="mt-6">
                <h2 className="text-lg font-semibold text-ink">Condições manuais</h2>
                <p className="mt-1 text-sm text-ink-soft">
                  Previstas no documento funcional, ainda sem persistência no backend.
                </p>
                <div className="mt-3 flex flex-col gap-2">
                  <ManualCondition label="Folha de pagamento processada" />
                  <ManualCondition label="Compensações realizadas" />
                </div>
              </section>

              <footer className="mt-8 rounded-card border border-dashed border-line px-4 py-3 text-sm text-ink-soft">
                Encerrar a competência ainda não está disponível: esta tela mostra o que falta, o encerramento
                formal depende do backend.
              </footer>
            </>
          )}
        </>
      )}
    </div>
  )
}

// CountCell decide entre número puro, link e link com filtro extra — nunca mostra um
// número clicável que não leva a lugar nenhum (ou pior, a um lugar errado).
function CountCell({ value, href }: { value: number; href: string | null }) {
  if (value === 0) return <span className="text-ink-soft">0</span>
  if (!href) {
    return (
      <span className="text-ink" title="Sem filtro equivalente em Ocorrências para 'Sem filial'.">
        {value}
      </span>
    )
  }
  return (
    <Link to={href} className="font-medium text-brand hover:underline">
      {value}
    </Link>
  )
}

// ManualCondition é DIV, não <button> nem <input type="checkbox">, de propósito: nada
// aqui responde a clique ou toque. Um controle que parece salvar e não salva é pior que
// nenhum controle — o usuário marca, fecha a aba, e acredita que registrou (contrato
// §9.3, regra 3).
function ManualCondition({ label }: { label: string }) {
  return (
    <div
      aria-disabled="true"
      className="flex items-center justify-between gap-4 rounded-field border border-line bg-panel px-4 py-3 opacity-60"
    >
      <div>
        <p className="text-sm font-medium text-ink">{label}</p>
        <p className="text-xs text-ink-faint">Aguardando backend</p>
      </div>
      <div className="relative h-7 w-12 shrink-0 rounded-full bg-line" aria-hidden>
        <span className="absolute left-1 top-1 h-5 w-5 rounded-full bg-white shadow-sm" />
      </div>
    </div>
  )
}
