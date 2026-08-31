// Pesos e medidas de pontuação — ver docs/11_Historico_Ranking_Frontend_Contract.md, §2 e §3.
//
// Este arquivo existe para que "pontuação" signifique a MESMA coisa em Ranking, Revisão
// mensal e qualquer tela futura. Duplicar a regra em duas telas é como o produto ganha
// dois números diferentes para a mesma pergunta.

import type { Occurrence, OccurrenceState, Severity } from './types'

// SEVERITY_WEIGHT pondera a gravidade. OPERACIONAL vale 0 porque NÃO é infração
// trabalhista: é sinal de investigação (provável troca de escala não comunicada, ou
// operação deliberada não avisada ao RH). Pontuar isso puniria o colaborador por um
// cadastro que o gestor não atualizou.
//
// Peso 0 não é "ignore": a ocorrência operacional continua contada, em campo próprio
// (ver ScoreBreakdown.operational).
export const SEVERITY_WEIGHT: Record<Severity, number> = {
  CRITICO: 10,
  ALERTA: 3,
  OPERACIONAL: 0,
}

export const SEVERITY_LABEL: Record<Severity, string> = {
  CRITICO: 'Crítico',
  ALERTA: 'Alerta',
  OPERACIONAL: 'Operacional',
}

// ScoreMeasure escolhe QUE estados entram na conta. As duas medidas respondem perguntas
// diferentes e não são intercambiáveis:
//
//   'exposicao' — o que está pendente AGORA. É o padrão das telas.
//   'periodo'   — o que ACONTECEU no intervalo, incluindo o que já se resolveu na origem.
//                 É a única medida que permite comparar mês contra mês; sem ela, todo mês
//                 encerrado tende a zero e "ranking de melhora" vira ruído.
//
// `resolvida_manual` (= ignorada) não entra em NENHUMA das duas: ignorar significa que o
// apontamento não procedia, e contá-la puniria alguém por um falso positivo.
export type ScoreMeasure = 'exposicao' | 'periodo'

export const MEASURE_LABEL: Record<ScoreMeasure, string> = {
  exposicao: 'Exposição (em aberto agora)',
  periodo: 'Período (tudo que aconteceu)',
}

const EXPOSICAO_STATES: OccurrenceState[] = ['aberta', 'atualizada']
const PERIODO_STATES: OccurrenceState[] = ['aberta', 'atualizada', 'resolvida_automatica']

export function statesForMeasure(measure: ScoreMeasure): OccurrenceState[] {
  return measure === 'exposicao' ? EXPOSICAO_STATES : PERIODO_STATES
}

/** countsForMeasure diz se a ocorrência entra na medida escolhida. */
export function countsForMeasure(occurrence: Occurrence, measure: ScoreMeasure): boolean {
  return statesForMeasure(measure).includes(occurrence.state)
}

/** Estados considerados "em aberto" (ainda demandam ação) — espelha OccurrenceState.Open() do backend. */
export function isOpen(occurrence: Occurrence): boolean {
  return occurrence.state === 'aberta' || occurrence.state === 'atualizada'
}

// ScoreBreakdown é o resultado de qualquer agregação de pontuação. `operational` fica
// FORA de `score` de propósito — some as duas e você acaba de punir um colaborador por
// uma escala desatualizada.
export interface ScoreBreakdown {
  score: number
  critical: number
  alert: number
  operational: number
  total: number
}

export function emptyBreakdown(): ScoreBreakdown {
  return { score: 0, critical: 0, alert: 0, operational: 0, total: 0 }
}

/** accumulate soma uma ocorrência ao breakdown, respeitando a medida. Mutação deliberada: é chamada em laço quente. */
export function accumulate(
  into: ScoreBreakdown,
  occurrence: Occurrence,
  measure: ScoreMeasure,
): ScoreBreakdown {
  if (!countsForMeasure(occurrence, measure)) return into

  into.total++
  into.score += SEVERITY_WEIGHT[occurrence.severity] ?? 0

  if (occurrence.severity === 'CRITICO') into.critical++
  else if (occurrence.severity === 'ALERTA') into.alert++
  else if (occurrence.severity === 'OPERACIONAL') into.operational++

  return into
}
