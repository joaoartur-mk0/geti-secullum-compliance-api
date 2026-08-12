// Agregações de ocorrências por colaborador, para a listagem em Colaboradores.
//
// Deriva de GET /occurrences (estado atual da máquina de estados), não de `reports`. Isso
// importa: um relatório antigo (de uma auditoria de um dia passado, disparada depois do
// fechamento mais recente) não é "o mais recente" na ordenação por data — se o status
// fosse calculado a partir do relatório mais recente por data, uma infração ainda em
// aberto encontrada num dia mais antigo nunca apareceria, e o colaborador ficaria
// incorretamente marcado como OK. Ocorrências não têm esse problema: o estado
// (aberta/atualizada/resolvida) é a verdade atual, independente de quando a varredura que
// a encontrou rodou.

import type { Occurrence } from './types'

export interface CollaboratorOccurrenceSummary {
  openCount: number // ocorrências abertas ou atualizadas AGORA (aberta + atualizada)
  openCritical: number // quantas dessas são críticas
  totalCount: number // ocorrências distintas já registradas (qualquer estado)
}

// summarizeOccurrencesByCollaborator indexa por id do colaborador na Secullum. Espera a
// lista com TODOS os estados (aberta/atualizada/resolvida_automatica/resolvida_manual)
// para o `totalCount` refletir o histórico completo, não só o que está em aberto.
export function summarizeOccurrencesByCollaborator(
  occurrences: Occurrence[],
): Map<number, CollaboratorOccurrenceSummary> {
  const map = new Map<number, CollaboratorOccurrenceSummary>()

  const ensure = (id: number) => {
    let s = map.get(id)
    if (!s) {
      s = { openCount: 0, openCritical: 0, totalCount: 0 }
      map.set(id, s)
    }
    return s
  }

  for (const occ of occurrences) {
    const s = ensure(occ.collaborator_id)
    s.totalCount++
    if (occ.state === 'aberta' || occ.state === 'atualizada') {
      s.openCount++
      if (occ.severity === 'CRITICO') s.openCritical++
    }
  }
  return map
}
