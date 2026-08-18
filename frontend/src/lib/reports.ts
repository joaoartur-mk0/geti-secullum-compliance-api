import type { Report } from './types'

// dedupeByDay mantém, para cada dia auditado, só a varredura MAIS RECENTE — é o que
// Histórico de varreduras usa para mostrar "1 auditoria por dia, sempre a mais atual", em vez
// do log inteiro (ver Logs/Histórico do sistema, em Configurações, para o histórico completo).
//
// Espera `reports` na ordem que o backend já devolve (mais recente para o mais antigo,
// `ORDER BY date DESC, id DESC`): nessa ordem, a primeira ocorrência de cada dia JÁ é a
// mais recente, então um Set de dias vistos basta — não precisa comparar datas/ids.
export function dedupeByDay(reports: Report[]): Report[] {
  const seen = new Set<string>()
  const out: Report[] = []
  for (const r of reports) {
    const day = r.date.slice(0, 10)
    if (seen.has(day)) continue
    seen.add(day)
    out.push(r)
  }
  return out
}
