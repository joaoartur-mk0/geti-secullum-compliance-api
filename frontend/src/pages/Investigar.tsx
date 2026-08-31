// Investigar — sinais operacionais que precisam ser apurados antes de virar auditoria.
// Contrato: docs/11_Historico_Ranking_Frontend_Contract.md, §9.4. Spec: SPEC-04_Investigar.md.
//
// A ARMADILHA (por isso este comentário existe aqui, não só na spec): filtrar por
// `severity === 'OPERACIONAL'`, NUNCA por `category === 'ALTERACAO_ESCALA'`. O backend
// deriva a categoria em domain/Occurrence.go: quando o estado é 'atualizada' a categoria
// vira NAO_CONFIRMADA antes de olhar a severidade. Filtrar por categoria descartaria em
// silêncio justo as ocorrências operacionais cujo valor mudou — as que mais precisam de
// conferência.

import { Ban, SearchCheck, Settings2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { BranchResolutionNote, PeriodFilters, usePeriodParams } from '../components/PeriodFilters'
import { Button, EmptyState, ErrorNote, OccurrenceStateBadge, Skeleton, useToast } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { branchLabel } from '../lib/analytics'
import { api, ApiError } from '../lib/api'
import { formatDate } from '../lib/format'
import type { Branch, Occurrence } from '../lib/types'

type Loadable<T> =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: T }

const PAGE_SIZE = 20

// Limiar da frase de leitura (seção 4.3 da spec): 3+ dias seguidos é troca de escala
// provável, 1-2 é exceção pontual. Também define quais grupos entram colapsados.
const STREAK_THRESHOLD = 3
const COLLAPSE_ABOVE_DAYS = 5
const OPEN_BY_DEFAULT_TOP = 3

interface CollaboratorGroup {
  collaboratorId: number
  name: string
  branch: string
  occurrences: Occurrence[] // ordenadas por data crescente
  daysAffected: number
  longestStreak: number
}

export default function Investigar() {
  const { tenant } = useTenant()
  const { preset, period, setParam, setPreset, params } = usePeriodParams('30d')
  const branchId = params.get('branch_id') ?? ''

  const [branches, setBranches] = useState<Branch[]>([])
  const [state, setState] = useState<Loadable<Occurrence[]>>({ phase: 'loading' })
  const [page, setPage] = useState(1)

  useEffect(() => {
    api.listBranches(tenant.id).then(setBranches).catch(() => setBranches([]))
  }, [tenant.id])

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      const { occurrences } = await api.listOccurrences(tenant.id, {
        start_date: period.start,
        end_date: period.end,
        state: ['aberta', 'atualizada'],
        branch_id: branchId ? Number(branchId) : undefined,
      })
      // A API não filtra por severidade (mesmo padrão de Incidentes.tsx) — o recorte é
      // aqui, no cliente, e é por SEVERIDADE, não categoria (ver comentário no topo).
      setState({ phase: 'ready', data: occurrences.filter((o) => o.severity === 'OPERACIONAL') })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar sinais operacionais.',
      })
    }
  }, [tenant.id, period.start, period.end, branchId])

  useEffect(() => {
    void load()
  }, [load])

  // Qualquer troca de filtro volta pra primeira página — evita cair numa página vazia.
  useEffect(() => {
    setPage(1)
  }, [period.start, period.end, branchId])

  const groups = useMemo<CollaboratorGroup[]>(() => {
    if (state.phase !== 'ready') return []
    return groupByCollaborator(state.data)
  }, [state])

  const totalPages = Math.max(1, Math.ceil(groups.length / PAGE_SIZE))
  const pageGroups = groups.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)

  return (
    <div className="animate-rise">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Investigar</h1>
        <p className="mt-1 text-sm text-ink-soft">
          {state.phase === 'ready'
            ? `${groups.length} colaborador${groups.length === 1 ? '' : 'es'} com sinal operacional a apurar.`
            : 'Sinais operacionais agrupados por colaborador, para apurar antes de auditar.'}
        </p>
      </header>

      {/* Bloco de contexto fixo — texto travado pela spec, não é resumo do que a tela faz. */}
      <div className="mt-4 flex items-start gap-2.5 rounded-card border border-line bg-panel px-4 py-3 text-sm text-ink-soft">
        <Settings2 size={17} className="mt-0.5 shrink-0 text-operacional" aria-hidden />
        <p>
          Sinais operacionais não são infração. Costumam indicar troca de escala não
          comunicada ou operação feita de propósito e não avisada ao RH. Apure antes de
          auditar.
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
      {/* A tela É o filtro de severidade — nenhum controle extra de severidade aqui. */}
      <BranchResolutionNote />

      <div className="mt-6">
        {state.phase === 'loading' && (
          <div className="flex flex-col gap-3">
            <Skeleton className="h-32 w-full" />
            <Skeleton className="h-32 w-full" />
            <Skeleton className="h-32 w-full" />
          </div>
        )}

        {state.phase === 'error' && <ErrorNote message={state.message} onRetry={load} />}

        {state.phase === 'ready' && groups.length === 0 && (
          <EmptyState
            icon={<SearchCheck size={32} strokeWidth={1.5} />}
            title="Nenhum sinal operacional no período"
            description="Nenhuma troca de escala ou operação não comunicada ao RH foi detectada nesse recorte."
          />
        )}

        {state.phase === 'ready' && groups.length > 0 && (
          <>
            <ul className="flex flex-col gap-4">
              {pageGroups.map((group, index) => (
                <GroupCard
                  key={group.collaboratorId}
                  group={group}
                  // Grupos com mais de 5 dias afetados colapsam por padrão, exceto os 3
                  // primeiros (já ordenados pelo maior nº de dias) — é onde o olho do
                  // gestor deve ir primeiro sem precisar abrir tudo.
                  defaultOpen={group.daysAffected <= COLLAPSE_ABOVE_DAYS || index < OPEN_BY_DEFAULT_TOP}
                  onChanged={load}
                />
              ))}
            </ul>

            {totalPages > 1 && (
              <div className="mt-4 flex items-center justify-between gap-3">
                <p className="text-xs text-ink-faint">
                  Página {page} de {totalPages}
                </p>
                <div className="flex items-center gap-2">
                  <Button variant="ghost" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
                    Anterior
                  </Button>
                  <Button
                    variant="ghost"
                    disabled={page >= totalPages}
                    onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  >
                    Próxima
                  </Button>
                </div>
              </div>
            )}
          </>
        )}
      </div>

      <p className="mt-8 text-xs text-ink-faint">
        Por enquanto o único desfecho disponível é "Ignorar". Registrar tratativa com
        justificativa e anexo depende do backend.
      </p>
    </div>
  )
}

// ---------- Agrupamento ----------

function groupByCollaborator(occurrences: Occurrence[]): CollaboratorGroup[] {
  const map = new Map<number, Occurrence[]>()
  for (const occ of occurrences) {
    const list = map.get(occ.collaborator_id)
    if (list) list.push(occ)
    else map.set(occ.collaborator_id, [occ])
  }

  const groups: CollaboratorGroup[] = []
  for (const [collaboratorId, list] of map) {
    const sorted = [...list].sort((a, b) => a.date.localeCompare(b.date))
    const uniqueDates = [...new Set(sorted.map((o) => o.date))]
    groups.push({
      collaboratorId,
      name: sorted[0].collaborator_name,
      branch: branchLabel(sorted[0]),
      occurrences: sorted,
      daysAffected: uniqueDates.length,
      longestStreak: longestConsecutiveStreak(uniqueDates),
    })
  }

  // Maior quantidade de dias afetados primeiro — é a leitura que separa troca de escala
  // (vários dias seguidos) de exceção pontual (um dia isolado). Ver seção 4.3 da spec.
  return groups.sort(
    (a, b) => b.daysAffected - a.daysAffected || a.name.localeCompare(b.name, 'pt-BR'),
  )
}

/** longestConsecutiveStreak espera datas ÚNICAS já ordenadas "YYYY-MM-DD" ascendente. */
function longestConsecutiveStreak(dates: string[]): number {
  if (dates.length === 0) return 0
  let longest = 1
  let current = 1
  for (let i = 1; i < dates.length; i++) {
    current = isNextCalendarDay(dates[i - 1], dates[i]) ? current + 1 : 1
    longest = Math.max(longest, current)
  }
  return longest
}

// Comparação por componentes de data (dia/mês/ano), nunca `new Date(iso)` direto — o
// construtor de string ISO lê como UTC e o fuso do Brasil desloca o dia (bug já visto em
// relatório de fechamento, ver lib/periods.ts).
function isNextCalendarDay(a: string, b: string): boolean {
  const [ay, am, ad] = a.split('-').map(Number)
  const [by, bm, bd] = b.split('-').map(Number)
  const dayA = new Date(ay, am - 1, ad).getTime()
  const dayB = new Date(by, bm - 1, bd).getTime()
  return dayB - dayA === 86_400_000
}

// ---------- Cartão do colaborador ----------

function GroupCard({
  group,
  defaultOpen,
  onChanged,
}: {
  group: CollaboratorGroup
  defaultOpen: boolean
  onChanged: () => void
}) {
  const phrase =
    group.longestStreak >= STREAK_THRESHOLD
      ? 'Padrão de vários dias seguidos — provável troca de escala.'
      : 'Dias isolados — provável exceção pontual.'

  return (
    <li className="rounded-card border border-line bg-bg shadow-card">
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-line px-4 py-3">
        <div>
          <Link
            to={`/colaboradores/${group.collaboratorId}`}
            className="font-semibold text-ink hover:text-brand hover:underline"
          >
            {group.name}
          </Link>
          <p className="mt-0.5 text-sm text-ink-soft">{group.branch}</p>
        </div>
        <div className="flex flex-col items-end gap-0.5 text-right">
          <p className="text-sm text-ink">
            <span className="font-semibold">{group.daysAffected}</span> dia
            {group.daysAffected === 1 ? '' : 's'} afetado{group.daysAffected === 1 ? '' : 's'} · sequência de{' '}
            <span className="font-semibold">{group.longestStreak}</span>
          </p>
          <p className="text-xs text-ink-faint">{phrase}</p>
        </div>
      </div>

      <details open={defaultOpen}>
        <summary className="cursor-pointer select-none px-4 py-2.5 text-sm font-medium text-ink-soft transition-colors duration-150 hover:text-ink">
          {group.occurrences.length} ocorrência{group.occurrences.length === 1 ? '' : 's'}
        </summary>
        <ul className="divide-y divide-line border-t border-line">
          {group.occurrences.map((occ) => (
            <DayRow key={occ.id} occurrence={occ} onChanged={onChanged} />
          ))}
        </ul>
      </details>
    </li>
  )
}

// ---------- Linha de dia + ação de ignorar ----------

function DayRow({ occurrence, onChanged }: { occurrence: Occurrence; onChanged: () => void }) {
  const toast = useToast()
  const [confirming, setConfirming] = useState(false)
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)

  // Motivo obrigatório AQUI, mesmo o backend aceitando vazio (§4.4 da spec): um sinal
  // operacional descartado sem motivo é um sinal perdido — o estado resolvida_manual é
  // pegajoso, a próxima varredura não reabre.
  const reasonEmpty = reason.trim() === ''

  async function ignore() {
    if (reasonEmpty) return
    setBusy(true)
    try {
      await api.ignoreOccurrence(occurrence.id, reason.trim())
      toast('success', 'Ocorrência ignorada')
      onChanged()
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao ignorar a ocorrência.')
    } finally {
      setBusy(false)
      setConfirming(false)
      setReason('')
    }
  }

  return (
    <li className="flex flex-wrap items-start justify-between gap-3 px-4 py-3">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="font-medium text-ink">{formatDate(occurrence.date)}</p>
          <OccurrenceStateBadge state={occurrence.state} />
        </div>
        <p className="mt-0.5 text-sm text-ink-soft">{occurrence.type}</p>
        <p className="mt-0.5 text-sm text-ink-faint">{occurrence.description}</p>
      </div>

      <div className="flex shrink-0 flex-col items-end gap-2">
        {confirming ? (
          <div className="flex flex-wrap items-end justify-end gap-2">
            <input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Motivo (obrigatório)"
              aria-label="Motivo para ignorar"
              className="min-h-9 w-56 rounded-field border border-line bg-bg px-3 text-sm text-ink placeholder:text-ink-faint focus:border-brand"
            />
            <Button variant="secondary" busy={busy} disabled={reasonEmpty} onClick={ignore}>
              <Ban size={15} aria-hidden />
              Confirmar
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                setConfirming(false)
                setReason('')
              }}
            >
              Cancelar
            </Button>
          </div>
        ) : (
          <Button variant="ghost" onClick={() => setConfirming(true)}>
            <Ban size={15} aria-hidden />
            Ignorar
          </Button>
        )}
      </div>
    </li>
  )
}
