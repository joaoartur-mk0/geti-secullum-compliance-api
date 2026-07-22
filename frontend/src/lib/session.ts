// Sessão provisória para a prévia: sem backend de auth ainda.
// Quando o backend ganhar autenticação real (multi-tenant), este módulo troca
// o localStorage por tokens emitidos pela API.

const SESSION_KEY = 'scw.session'
const TENANT_KEY = 'scw.tenantId'

export interface Session {
  email: string
  loggedAt: string
}

export function getSession(): Session | null {
  try {
    const raw = localStorage.getItem(SESSION_KEY)
    return raw ? (JSON.parse(raw) as Session) : null
  } catch {
    return null
  }
}

export function startSession(email: string) {
  const session: Session = { email, loggedAt: new Date().toISOString() }
  localStorage.setItem(SESSION_KEY, JSON.stringify(session))
}

export function endSession() {
  localStorage.removeItem(SESSION_KEY)
  localStorage.removeItem(TENANT_KEY)
}

export function getSavedTenantId(): number | null {
  const raw = localStorage.getItem(TENANT_KEY)
  const id = raw ? Number(raw) : NaN
  return Number.isFinite(id) ? id : null
}

export function saveTenantId(id: number) {
  localStorage.setItem(TENANT_KEY, String(id))
}
