// Lotação = em qual filial cada colaborador está.
//
// No banco esse vínculo é uma linha em `branch_payroll_numbers`: a filial reivindica um
// Nº DE FOLHA, e o backend resolve a filial casando esse número com o `numero_folha` que
// veio da Secullum. O painel, porém, precisa falar em NOMES — ninguém sabe de cor que a
// folha 47802 é o Marcos.
//
// O problema é que `GET /collaborators` devolve só {id, secullum_id, name}: o
// `numero_folha` só existe no `/prefill`, um colaborador por chamada. Este módulo faz
// essa ponte — varre os prefills uma vez, monta os dois índices (por id e por nº de
// folha) e guarda em sessionStorage, para as telas trabalharem com nome de gente.
//
// A varredura é o custo desta abordagem: ~13s para 481 colaboradores com concorrência 8.
// Ela só é paga pela tela de Filiais, que precisa do sentido inverso (número → nome). A
// ficha do colaborador não usa este índice: ela já chama o prefill da própria pessoa e
// tem o `numero_folha` em mãos.

import { api, ApiError } from './api'
import type { Branch } from './types'

// Quantas chamadas de prefill em paralelo. Seis é o teto prático de conexões por host do
// navegador em HTTP/1.1 — subir mais só enfileira do outro lado e ainda arrisca esbarrar
// em rate limit da API.
const CONCURRENCY = 6

const CACHE_PREFIX = 'lotacao-index:'

export interface CollaboratorEntry {
  secullumId: number
  name: string
  /** `numero_folha` da Secullum. Vazio impede o vínculo — ver `MISSING_PAYROLL`. */
  numeroFolha: string
}

export interface LotacaoIndex {
  /** Todos os colaboradores do tenant, na ordem em que a API devolveu. */
  collaborators: CollaboratorEntry[]
  bySecullumId: Map<number, CollaboratorEntry>
  /** O sentido que a tela de Filiais precisa: nº de folha → quem é. */
  byNumeroFolha: Map<string, CollaboratorEntry>
  /** Quando o índice foi montado (ISO). Mostrado ao usuário para ele decidir se atualiza. */
  builtAt: string
}

export const MISSING_PAYROLL =
  'Este colaborador não tem nº de folha na Secullum — sem ele não é possível lotá-lo numa filial.'

// ---------- Construção do índice ----------

interface CachedIndex {
  builtAt: string
  collaborators: CollaboratorEntry[]
}

function cacheKey(tenantId: number) {
  return `${CACHE_PREFIX}${tenantId}`
}

function hydrate(cached: CachedIndex): LotacaoIndex {
  const bySecullumId = new Map<number, CollaboratorEntry>()
  const byNumeroFolha = new Map<string, CollaboratorEntry>()
  for (const entry of cached.collaborators) {
    bySecullumId.set(entry.secullumId, entry)
    // Nº de folha é único por tenant nesta base, mas o backend não garante isso do lado
    // do colaborador (só do lado da filial). Se houver colisão, o primeiro vence e o
    // segundo fica sem nome na tela — melhor do que sobrescrever silenciosamente.
    if (entry.numeroFolha && !byNumeroFolha.has(entry.numeroFolha)) {
      byNumeroFolha.set(entry.numeroFolha, entry)
    }
  }
  return { collaborators: cached.collaborators, bySecullumId, byNumeroFolha, builtAt: cached.builtAt }
}

/** Índice já em cache nesta sessão, ou null. Não faz rede. */
export function readCachedIndex(tenantId: number): LotacaoIndex | null {
  try {
    const raw = sessionStorage.getItem(cacheKey(tenantId))
    if (!raw) return null
    return hydrate(JSON.parse(raw) as CachedIndex)
  } catch {
    // Cache corrompido não é motivo para quebrar a tela: só reconstrói.
    return null
  }
}

export function clearCachedIndex(tenantId: number) {
  try {
    sessionStorage.removeItem(cacheKey(tenantId))
  } catch {
    // sessionStorage indisponível (modo privado antigo): seguir sem cache.
  }
}

/**
 * Monta o índice de lotação do tenant, varrendo o prefill de cada colaborador.
 *
 * `onProgress` recebe (concluídos, total) para a tela mostrar barra — a varredura é longa
 * o bastante para exigir feedback. Um prefill que falha NÃO derruba a varredura: aquele
 * colaborador entra sem `numeroFolha` e a tela o exibe como não-vinculável, que é a
 * verdade operacional.
 */
export async function buildIndex(
  tenantId: number,
  onProgress?: (done: number, total: number) => void,
): Promise<LotacaoIndex> {
  const { collaborators } = await api.listCollaborators(tenantId)

  const entries: CollaboratorEntry[] = new Array(collaborators.length)
  let cursor = 0
  let done = 0

  async function worker() {
    for (;;) {
      const i = cursor++
      if (i >= collaborators.length) return
      const c = collaborators[i]
      let numeroFolha = ''
      try {
        const prefill = await api.getCollaboratorPrefill(tenantId, c.secullum_id)
        numeroFolha = prefill.collaborator.numero_folha ?? ''
      } catch {
        // Mantém o colaborador na lista, sem nº de folha.
      }
      entries[i] = { secullumId: c.secullum_id, name: c.name, numeroFolha }
      onProgress?.(++done, collaborators.length)
    }
  }

  await Promise.all(Array.from({ length: Math.min(CONCURRENCY, collaborators.length) }, worker))

  const cached: CachedIndex = { builtAt: new Date().toISOString(), collaborators: entries }
  try {
    sessionStorage.setItem(cacheKey(tenantId), JSON.stringify(cached))
  } catch {
    // Cota estourada: segue sem cache, só fica mais lento na próxima abertura.
  }
  return hydrate(cached)
}

/** Índice do cache, ou recém-construído se ainda não houver. */
export async function ensureIndex(
  tenantId: number,
  onProgress?: (done: number, total: number) => void,
): Promise<LotacaoIndex> {
  return readCachedIndex(tenantId) ?? (await buildIndex(tenantId, onProgress))
}

// ---------- Leitura da lotação ----------

/** O vínculo existente de um nº de folha: em que filial ele está e sob qual id. */
export interface PayrollLink {
  branchId: number
  branchName: string
  payrollNumberId: number
}

/** Procura o nº de folha entre TODAS as filiais — é assim que se descobre a lotação atual. */
export function findLink(branches: Branch[], numeroFolha: string): PayrollLink | null {
  if (!numeroFolha) return null
  for (const branch of branches) {
    for (const pn of branch.payroll_numbers) {
      if (pn.numero === numeroFolha) {
        return { branchId: branch.id, branchName: branch.name, payrollNumberId: pn.id }
      }
    }
  }
  return null
}

/** Colaboradores lotados numa filial, resolvidos para nome via índice. */
export interface BranchMember {
  payrollNumberId: number
  numeroFolha: string
  /** null quando o nº de folha não corresponde a nenhum colaborador sincronizado. */
  collaborator: CollaboratorEntry | null
}

/**
 * Cruza os nº de folha de uma filial com o índice.
 *
 * `collaborator: null` é o caso que motivou este módulo: um número digitado à mão que não
 * pertence a ninguém (foi assim que o `secullum_id` acabou cadastrado no lugar da folha).
 * A tela precisa mostrar isso como órfão, não escondê-lo.
 */
export function branchMembers(branch: Branch, index: LotacaoIndex | null): BranchMember[] {
  return branch.payroll_numbers
    .map((pn) => ({
      payrollNumberId: pn.id,
      numeroFolha: pn.numero,
      collaborator: index?.byNumeroFolha.get(pn.numero) ?? null,
    }))
    .sort((a, b) => {
      // Órfãos por último: são pendência de limpeza, não a lista de gente.
      if (!a.collaborator !== !b.collaborator) return a.collaborator ? -1 : 1
      const an = a.collaborator?.name ?? a.numeroFolha
      const bn = b.collaborator?.name ?? b.numeroFolha
      return an.localeCompare(bn, 'pt-BR')
    })
}

// ---------- Escrita da lotação ----------

/**
 * Move um colaborador de filial (ou o desloca de todas, com `branchId = null`).
 *
 * A ordem importa: o índice único (tenant, numero) faz o POST devolver 409 se o vínculo
 * antigo ainda existir, então remove-se ANTES de criar.
 *
 * Sem transação do lado do servidor, a janela entre as duas chamadas é real: se o POST
 * falhar, a pessoa fica sem filial em vez de ficar na antiga. Por isso o vínculo anterior
 * é restaurado no catch — best-effort, mas cobre o caso comum (rede oscilando).
 *
 * `branches` deve ser a lista recém-carregada: é dela que sai o vínculo atual.
 */
export async function setFilial(
  numeroFolha: string,
  branchId: number | null,
  branches: Branch[],
): Promise<void> {
  if (!numeroFolha) throw new ApiError(422, 'VALIDATION', MISSING_PAYROLL)

  const current = findLink(branches, numeroFolha)
  if (current?.branchId === branchId) return

  if (current) {
    await api.removeBranchPayrollNumber(current.branchId, current.payrollNumberId)
  }
  if (branchId == null) return

  try {
    await api.addBranchPayrollNumber(branchId, { numero: numeroFolha })
  } catch (error) {
    if (current) {
      try {
        await api.addBranchPayrollNumber(current.branchId, { numero: numeroFolha })
      } catch {
        // Restauração falhou também: o erro original é o que interessa reportar, e a
        // tela vai recarregar mostrando o colaborador sem filial — estado visível, não
        // silencioso.
      }
    }
    throw error
  }
}

export interface BulkResult {
  ok: number
  failures: { name: string; message: string }[]
}

/**
 * Lota vários colaboradores de uma vez na mesma filial.
 *
 * Em série de propósito: cada item pode precisar de duas chamadas (remover + criar) e
 * paralelizar escrita sobre o mesmo índice único só multiplica conflito. O volume aqui é
 * de dezenas, não centenas — o tempo não justifica o risco.
 *
 * Falha de um item não interrompe os outros: lotar 40 pessoas e abortar na 12ª por causa
 * de um cadastro estranho seria pior do que reportar as falhas no fim.
 */
export async function setFilialEmLote(
  entries: CollaboratorEntry[],
  branchId: number,
  branches: Branch[],
  onProgress?: (done: number, total: number) => void,
): Promise<BulkResult> {
  const result: BulkResult = { ok: 0, failures: [] }
  let done = 0

  for (const entry of entries) {
    try {
      if (!entry.numeroFolha) throw new ApiError(422, 'VALIDATION', MISSING_PAYROLL)
      await setFilial(entry.numeroFolha, branchId, branches)
      result.ok++
    } catch (error) {
      result.failures.push({
        name: entry.name || `Colaborador ${entry.secullumId}`,
        message: error instanceof ApiError ? error.message : 'Falha inesperada.',
      })
    }
    onProgress?.(++done, entries.length)
  }
  return result
}
