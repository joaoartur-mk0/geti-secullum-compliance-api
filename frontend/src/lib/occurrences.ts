// Agregações de ocorrências (inconsistências) por colaborador, derivadas da lista de
// relatórios que o backend já entrega. As inconsistências são chaveadas por
// CollaboratorID = SecullumID (ver consumer.go), o mesmo `secullum_id` do colaborador.

import type { Report } from './types'

export interface CollaboratorOccurrenceSummary {
  latestCount: number // ocorrências na varredura mais recente
  latestCritical: number // quantas dessas são críticas
  totalCount: number // ocorrências somadas em todo o histórico
  reportsWithOccurrence: number // nº de varreduras em que apareceu
}

// summarizeByCollaborator indexa, por SecullumID, o status da última varredura e o
// acumulado do histórico. `reports` deve vir do mais recente para o mais antigo
// (ordem que o backend já devolve).
export function summarizeByCollaborator(
  reports: Report[],
): Map<number, CollaboratorOccurrenceSummary> {
  const map = new Map<number, CollaboratorOccurrenceSummary>()
  const latest = reports.length > 0 ? reports[0] : null

  const ensure = (id: number) => {
    let s = map.get(id)
    if (!s) {
      s = { latestCount: 0, latestCritical: 0, totalCount: 0, reportsWithOccurrence: 0 }
      map.set(id, s)
    }
    return s
  }

  for (const report of reports) {
    const seen = new Set<number>()
    for (const inc of report.inconsistencies ?? []) {
      const s = ensure(inc.CollaboratorID)
      s.totalCount++
      if (!seen.has(inc.CollaboratorID)) {
        seen.add(inc.CollaboratorID)
        s.reportsWithOccurrence++
      }
      if (report === latest) {
        s.latestCount++
        if (inc.Severity === 'CRITICO') s.latestCritical++
      }
    }
  }
  return map
}
