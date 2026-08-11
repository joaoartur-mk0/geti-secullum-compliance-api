// Sessão de autenticação real (JWT) — ver docs/05_Auth_Backend_Contract.md no backend.
// O token é obtido em POST /api/v1/auth/login e reenviado pelo api.ts (header
// Authorization: Bearer <token>) em toda chamada subsequente à API.

import type { User } from './types'

const SESSION_KEY = 'scw.session'
const TENANT_KEY = 'scw.tenantId'

export interface Session {
  token: string
  user: User
}

function isSession(value: unknown): value is Session {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<Session>
  return (
    typeof candidate.token === 'string' &&
    typeof candidate.user === 'object' &&
    candidate.user !== null &&
    typeof (candidate.user as Partial<User>).email === 'string'
  )
}

export function getSession(): Session | null {
  try {
    const raw = localStorage.getItem(SESSION_KEY)
    if (!raw) return null

    const parsed: unknown = JSON.parse(raw)
    if (!isSession(parsed)) {
      // Formato antigo (pré-JWT) ou corrompido: descarta em vez de deixar a UI
      // quebrar tentando ler campos que não existem (ex.: session.user.email).
      endSession()
      return null
    }

    return parsed
  } catch {
    return null
  }
}

export function getToken(): string | null {
  return getSession()?.token ?? null
}

// Só o super admin acessa o painel de moderação (/moderacao) — cadastro/exclusão de
// usuários e empresas, ver docs/05_Auth_Backend_Contract.md no backend.
export function isSuperAdmin(): boolean {
  return getSession()?.user.is_super_admin ?? false
}

export function startSession(session: Session) {
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
