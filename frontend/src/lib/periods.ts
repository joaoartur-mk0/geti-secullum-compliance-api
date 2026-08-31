// Períodos, competências e comparação de intervalos — ver
// docs/11_Historico_Ranking_Frontend_Contract.md, §5.
//
// Toda data aqui é "YYYY-MM-DD" tratada como STRING. Nunca `new Date(iso)` para data
// pura: o construtor interpreta como UTC e o fuso do Brasil devolve o dia anterior — bug
// já visto em relatório de fechamento.

export type PeriodPreset = '7d' | '30d' | 'mes' | 'custom'

export const PRESET_LABEL: Record<PeriodPreset, string> = {
  '7d': '7 dias',
  '30d': '30 dias',
  mes: 'Este mês',
  custom: 'Personalizado',
}

export interface Period {
  start: string // "YYYY-MM-DD"
  end: string // "YYYY-MM-DD"
}

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

function toIso(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

export function today(): string {
  return toIso(new Date())
}

export function isoDaysAgo(n: number): string {
  const d = new Date()
  d.setDate(d.getDate() - n)
  return toIso(d)
}

export function isoStartOfMonth(): string {
  const d = new Date()
  return toIso(new Date(d.getFullYear(), d.getMonth(), 1))
}

/** periodForPreset devolve o intervalo do preset. 'custom' devolve null: quem escolhe as datas é o usuário. */
export function periodForPreset(preset: PeriodPreset): Period | null {
  switch (preset) {
    case '7d':
      return { start: isoDaysAgo(7), end: today() }
    case '30d':
      return { start: isoDaysAgo(30), end: today() }
    case 'mes':
      return { start: isoStartOfMonth(), end: today() }
    default:
      return null
  }
}

/** daysInPeriod conta os dias do intervalo, ambas as pontas inclusive. */
export function daysInPeriod(period: Period): number {
  const [ys, ms, ds] = period.start.split('-').map(Number)
  const [ye, me, de] = period.end.split('-').map(Number)
  const start = new Date(ys, ms - 1, ds)
  const end = new Date(ye, me - 1, de)
  return Math.floor((end.getTime() - start.getTime()) / 86_400_000) + 1
}

// previousPeriod devolve o intervalo IMEDIATAMENTE anterior, de mesma duração — é a base
// do "ranking de melhora". Mesma duração importa: comparar 30 dias com 15 mediria o
// tamanho da janela, não a melhora.
export function previousPeriod(period: Period): Period {
  const len = daysInPeriod(period)
  const [y, m, d] = period.start.split('-').map(Number)
  const end = new Date(y, m - 1, d - 1)
  const start = new Date(y, m - 1, d - len)
  return { start: toIso(start), end: toIso(end) }
}

/** listDates enumera todos os dias do intervalo, inclusive. Usado para achar dias sem varredura. */
export function listDates(period: Period): string[] {
  const out: string[] = []
  const [ys, ms, ds] = period.start.split('-').map(Number)
  const [ye, me, de] = period.end.split('-').map(Number)
  const cursor = new Date(ys, ms - 1, ds)
  const end = new Date(ye, me - 1, de)
  while (cursor <= end) {
    out.push(toIso(cursor))
    cursor.setDate(cursor.getDate() + 1)
  }
  return out
}

// ---------- Competência (mês calendário, "YYYY-MM") ----------
//
// Vocabulário travado: o ciclo mensal chama-se REVISÃO MENSAL. "Fechamento" é a varredura
// diária de D-1 e não pode ser usado aqui.

export type Competencia = string // "YYYY-MM"

export function currentCompetencia(): Competencia {
  const d = new Date()
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}`
}

export function competenciaOf(isoDate: string): Competencia {
  return isoDate.slice(0, 7)
}

/**
 * competenciaPeriod devolve o intervalo da competência LIMITADO A HOJE.
 *
 * O corte em hoje é regra de honestidade: um mês corrente cuja janela fosse até o dia 31
 * contaria como "sem varredura" dias que ainda não aconteceram, e a revisão mensal
 * apareceria eternamente incompleta.
 */
export function competenciaPeriod(competencia: Competencia): Period {
  const [y, m] = competencia.split('-').map(Number)
  const lastDay = new Date(y, m, 0).getDate()
  const end = `${competencia}-${pad(lastDay)}`
  const now = today()
  return { start: `${competencia}-01`, end: end > now ? now : end }
}

/**
 * competenciaCoverage devolve os dias da competência que JÁ PODEM ter varredura.
 *
 * A auditoria só roda sobre dias ENCERRADOS: o fechamento é de D-1 e o backend recusa
 * auditar hoje ou o futuro. Contar hoje como "dia sem varredura" marcaria como buraco de
 * cobertura um dia que ainda nem terminou — e a competência corrente apareceria
 * eternamente incompleta por um motivo que não é falha de ninguém, todo dia, para sempre.
 *
 * Devolve null quando a competência ainda não tem nenhum dia encerrado (dia 1º do mês).
 */
export function competenciaCoverage(competencia: Competencia): Period | null {
  const [y, m] = competencia.split('-').map(Number)
  const lastDay = new Date(y, m, 0).getDate()
  const monthEnd = `${competencia}-${pad(lastDay)}`

  const d = new Date()
  d.setDate(d.getDate() - 1)
  const limit = toIso(d)

  const start = `${competencia}-01`
  const end = monthEnd > limit ? limit : monthEnd
  return end < start ? null : { start, end }
}

export function formatCompetencia(competencia: Competencia): string {
  const [y, m] = competencia.split('-').map(Number)
  const label = new Date(y, m - 1, 1).toLocaleDateString('pt-BR', { month: 'long', year: 'numeric' })
  return label.charAt(0).toUpperCase() + label.slice(1)
}

/** competenciasBetween lista as competências de duas datas, da mais recente para a mais antiga. */
export function competenciasBetween(firstDate: string, lastDate: string): Competencia[] {
  const out: Competencia[] = []
  const [ys, ms] = firstDate.split('-').map(Number)
  const [ye, me] = lastDate.split('-').map(Number)
  const cursor = new Date(ys, ms - 1, 1)
  const end = new Date(ye, me - 1, 1)
  while (cursor <= end) {
    out.push(`${cursor.getFullYear()}-${pad(cursor.getMonth() + 1)}`)
    cursor.setMonth(cursor.getMonth() + 1)
  }
  return out.reverse()
}
