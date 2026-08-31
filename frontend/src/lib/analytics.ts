// Agregações de ocorrências para Histórico, Ranking e Revisão mensal.
//
// Regra de ouro deste arquivo (docs/11_Historico_Ranking_Frontend_Contract.md, §10): a
// soma das partes bate com o total. Toda função aqui devolve grupos que, somados,
// reproduzem a lista de entrada — inclusive as ocorrências sem filial, que ganham grupo
// próprio em vez de sumir.

import { accumulate, emptyBreakdown, isOpen, type ScoreBreakdown, type ScoreMeasure } from './severity'
import type { Occurrence, OccurrenceState } from './types'

// ---------- Filial ----------

// UNASSIGNED_BRANCH é a chave do grupo "Sem filial". Ocorrência com `filial: null` é
// comum (aparelho não cadastrado, colaborador sem lotação) e NÃO pode ser descartada: se
// sumir, a soma das filiais deixa de bater com o total e o painel mente.
export const UNASSIGNED_BRANCH = -1
export const UNASSIGNED_BRANCH_LABEL = 'Sem filial'

export function branchKey(occurrence: Occurrence): number {
  return occurrence.filial?.id ?? UNASSIGNED_BRANCH
}

export function branchLabel(occurrence: Occurrence): string {
  return occurrence.filial?.name ?? UNASSIGNED_BRANCH_LABEL
}

// ---------- Desfechos ----------

// Rótulos dos estados. `resolvida_manual` é "Ignorada", NUNCA "tratada": o desfecho
// "tratada" não existe no backend, e chamar uma coisa pela outra apaga justamente a
// distinção que a feature 4 vai comprar.
export const STATE_LABEL: Record<OccurrenceState, string> = {
  aberta: 'Em aberto',
  atualizada: 'A reconferir',
  resolvida_automatica: 'Corrigida na origem',
  resolvida_manual: 'Ignorada',
}

export const ALL_STATES: OccurrenceState[] = [
  'aberta',
  'atualizada',
  'resolvida_automatica',
  'resolvida_manual',
]

export type OutcomeCounts = Record<OccurrenceState, number>

export function countByState(occurrences: Occurrence[]): OutcomeCounts {
  const counts = {
    aberta: 0,
    atualizada: 0,
    resolvida_automatica: 0,
    resolvida_manual: 0,
  } as OutcomeCounts
  for (const occ of occurrences) counts[occ.state]++
  return counts
}

// ---------- Ranking ----------

export interface RankedGroup {
  key: number | string
  label: string
  breakdown: ScoreBreakdown
}

function rank(
  occurrences: Occurrence[],
  measure: ScoreMeasure,
  keyOf: (o: Occurrence) => number | string,
  labelOf: (o: Occurrence) => string,
): RankedGroup[] {
  const groups = new Map<number | string, RankedGroup>()

  for (const occ of occurrences) {
    const key = keyOf(occ)
    let group = groups.get(key)
    if (!group) {
      group = { key, label: labelOf(occ), breakdown: emptyBreakdown() }
      groups.set(key, group)
    }
    accumulate(group.breakdown, occ, measure)
  }

  // Grupos que ficaram zerados (só tinham ocorrências fora da medida) saem: uma linha com
  // tudo zero não informa nada e empurra as linhas úteis para fora da tela.
  return [...groups.values()]
    .filter((g) => g.breakdown.total > 0)
    .sort(
      (a, b) =>
        b.breakdown.score - a.breakdown.score ||
        b.breakdown.total - a.breakdown.total ||
        a.label.localeCompare(b.label, 'pt-BR'),
    )
}

export function rankByCollaborator(occurrences: Occurrence[], measure: ScoreMeasure): RankedGroup[] {
  return rank(occurrences, measure, (o) => o.collaborator_id, (o) => o.collaborator_name)
}

export function rankByBranch(occurrences: Occurrence[], measure: ScoreMeasure): RankedGroup[] {
  return rank(occurrences, measure, branchKey, branchLabel)
}

// ---------- Melhora entre períodos ----------

export interface ImprovementRow {
  key: number | string
  label: string
  current: number // pontuação no período selecionado
  previous: number // pontuação no período anterior de mesma duração
  delta: number // current - previous (negativo = melhorou)
}

/**
 * improvement cruza dois períodos pela pontuação.
 *
 * Só entra quem aparece nos DOIS períodos: quem só existe em um teria delta igual à
 * própria pontuação, e o ranking passaria a medir entrada e saída de gente em vez de
 * melhora de comportamento. Quem foi excluído é contabilizado em `excluded`, para a tela
 * poder dizer isso em vez de silenciar.
 */
export function improvement(
  current: RankedGroup[],
  previous: RankedGroup[],
): { rows: ImprovementRow[]; excluded: number } {
  const prev = new Map(previous.map((g) => [g.key, g]))
  const rows: ImprovementRow[] = []
  let excluded = 0

  for (const group of current) {
    const before = prev.get(group.key)
    if (!before) {
      excluded++
      continue
    }
    rows.push({
      key: group.key,
      label: group.label,
      current: group.breakdown.score,
      previous: before.breakdown.score,
      delta: group.breakdown.score - before.breakdown.score,
    })
  }

  excluded += previous.filter((g) => !current.some((c) => c.key === g.key)).length
  rows.sort((a, b) => a.delta - b.delta || a.label.localeCompare(b.label, 'pt-BR'))
  return { rows, excluded }
}

// ---------- Tempo até o desfecho ----------

/**
 * averageResolutionDays devolve a média, em dias, entre a primeira detecção
 * (`first_seen_at`) e o desfecho (`resolved_at`).
 *
 * Só entram ocorrências com desfecho. `count` acompanha a média de propósito: "3,2 dias"
 * sobre duas ocorrências não é a mesma informação que sobre duzentas, e a tela precisa
 * poder dizer isso.
 */
export function averageResolutionDays(occurrences: Occurrence[]): { days: number; count: number } {
  let sum = 0
  let count = 0

  for (const occ of occurrences) {
    if (!occ.resolved_at) continue
    const from = new Date(occ.first_seen_at).getTime()
    const to = new Date(occ.resolved_at).getTime()
    if (Number.isNaN(from) || Number.isNaN(to) || to < from) continue
    sum += to - from
    count++
  }

  return { days: count ? sum / count / 86_400_000 : 0, count }
}

// ---------- Agrupamento temporal ----------

export type Granularity = 'dia' | 'semana' | 'mes'

export const GRANULARITY_LABEL: Record<Granularity, string> = {
  dia: 'Dia',
  semana: 'Semana',
  mes: 'Mês',
}

/** bucketOf devolve a chave temporal ("YYYY-MM-DD" da segunda-feira, para semana). */
export function bucketOf(isoDate: string, granularity: Granularity): string {
  const date = isoDate.slice(0, 10)
  if (granularity === 'dia') return date
  if (granularity === 'mes') return date.slice(0, 7)

  const [y, m, d] = date.split('-').map(Number)
  const ref = new Date(y, m - 1, d)
  // getDay(): 0 = domingo. Recuar até a segunda-feira mantém a semana brasileira.
  const offset = (ref.getDay() + 6) % 7
  ref.setDate(ref.getDate() - offset)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${ref.getFullYear()}-${pad(ref.getMonth() + 1)}-${pad(ref.getDate())}`
}

export interface TimeBucket {
  key: string
  occurrences: Occurrence[]
}

/** groupByTime agrupa por dia/semana/mês, da chave mais antiga para a mais recente. */
export function groupByTime(occurrences: Occurrence[], granularity: Granularity): TimeBucket[] {
  const map = new Map<string, Occurrence[]>()
  for (const occ of occurrences) {
    const key = bucketOf(occ.date, granularity)
    const list = map.get(key)
    if (list) list.push(occ)
    else map.set(key, [occ])
  }
  return [...map.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([key, list]) => ({ key, occurrences: list }))
}

// ---------- Revisão mensal ----------

// ReviewConditions são as condições da revisão mensal VERIFICÁVEIS hoje. As manuais
// (folha processada, compensações lançadas) dependem de persistência que não existe — ver
// docs/12_Revisao_Mensal_E_Tratativas_Backend_Contract.md, §6.
export interface ReviewConditions {
  branchKey: number
  branchLabel: string
  open: number // aberta + atualizada
  toRecheck: number // atualizada (o valor mudou desde a última leitura)
  operationalOpen: number // operacional em aberto: escala a confirmar antes de auditar
  total: number
}

export function reviewConditionsByBranch(occurrences: Occurrence[]): ReviewConditions[] {
  const map = new Map<number, ReviewConditions>()

  for (const occ of occurrences) {
    const key = branchKey(occ)
    let row = map.get(key)
    if (!row) {
      row = {
        branchKey: key,
        branchLabel: branchLabel(occ),
        open: 0,
        toRecheck: 0,
        operationalOpen: 0,
        total: 0,
      }
      map.set(key, row)
    }
    row.total++
    if (isOpen(occ)) {
      row.open++
      if (occ.severity === 'OPERACIONAL') row.operationalOpen++
    }
    if (occ.state === 'atualizada') row.toRecheck++
  }

  return [...map.values()].sort(
    (a, b) => b.open - a.open || a.branchLabel.localeCompare(b.branchLabel, 'pt-BR'),
  )
}
