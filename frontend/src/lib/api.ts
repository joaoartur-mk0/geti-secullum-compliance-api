import type {
  AddUserToTenantRequest,
  ApiErrorBody,
  Collaborator,
  CreateTenantRequest,
  HealthResponse,
  LoginRequest,
  LoginResponse,
  RegisterUserRequest,
  Report,
  Settings,
  Staff,
  StaffRequest,
  Tenant,
  User,
  WhatsAppConnectResponse,
  WhatsAppStatus,
} from './types'
import { endSession, getToken } from './session'

// Endereço da API: vem da variável de ambiente de build do Vite (VITE_API_URL).
// É resolvida em BUILD TIME — em produção, defina VITE_API_URL ao buildar o frontend
// (ver frontend/.env.example). No desenvolvimento local, cai no padrão localhost.
const API_URL = (import.meta.env.VITE_API_URL || 'http://localhost:8080').replace(/\/+$/, '')

export class ApiError extends Error {
  code: string
  status: number

  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken()

  let res: Response
  try {
    res = await fetch(`${API_URL}${path}`, {
      ...init,
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...init?.headers,
      },
    })
  } catch {
    throw new ApiError(0, 'OFFLINE', 'Não foi possível falar com a API. Verifique se o backend está no ar.')
  }

  if (!res.ok) {
    let body: ApiErrorBody = {}
    try {
      body = await res.json()
    } catch {
      // corpo não-JSON: mantém mensagem genérica
    }

    // Sessão expirada/inválida: limpa e manda para o login. Não faz isso para a própria
    // chamada de login (que devolve 400, não 401, em caso de credenciais erradas).
    if (res.status === 401) {
      endSession()
      if (location.pathname !== '/login') location.href = '/login'
    }

    throw new ApiError(
      res.status,
      body.error?.code ?? 'INTERNAL',
      body.error?.message ?? `A API respondeu com erro ${res.status}.`,
    )
  }

  return res.json() as Promise<T>
}

export const api = {
  health: () => request<HealthResponse>('/health'),

  login: (body: LoginRequest) =>
    request<LoginResponse>('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  triggerAudit: (tenantId: number) =>
    request<{ message: string; tenant_id: number; status: string }>('/api/v1/audit/trigger', {
      method: 'POST',
      body: JSON.stringify({ tenant_id: tenantId }),
    }),

  listTenants: (includeInactive = false) =>
    request<{ tenants: Tenant[] | null }>(
      `/api/v1/tenants${includeInactive ? '?include_inactive=true' : ''}`,
    ).then((r) => r.tenants ?? []),

  createTenant: (body: CreateTenantRequest) =>
    request<{ message: string; tenant: Tenant }>('/api/v1/tenants', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  getTenant: (id: number) => request<{ tenant: Tenant }>(`/api/v1/tenants/${id}`).then((r) => r.tenant),

  updateTenant: (id: number, body: { name: string; secullum_database_id: number }) =>
    request<{ message: string }>(`/api/v1/tenants/${id}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),

  activateTenant: (id: number) =>
    request<{ message: string }>(`/api/v1/tenants/${id}/activate`, { method: 'PATCH' }),

  deactivateTenant: (id: number) =>
    request<{ message: string }>(`/api/v1/tenants/${id}/deactivate`, { method: 'PATCH' }),

  deleteTenant: (id: number) =>
    request<{ message: string }>(`/api/v1/tenants/${id}`, { method: 'DELETE' }),

  // ---------- Moderação: usuários (só super admin) ----------

  listUsers: () => request<{ users: User[] | null }>('/api/v1/users').then((r) => r.users ?? []),

  registerUser: (body: RegisterUserRequest) =>
    request<{ message: string; user: User }>('/api/v1/auth/register', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  activateUser: (id: number) =>
    request<{ message: string }>(`/api/v1/users/${id}/activate`, { method: 'PATCH' }),

  deactivateUser: (id: number) =>
    request<{ message: string }>(`/api/v1/users/${id}/deactivate`, { method: 'PATCH' }),

  deleteUser: (id: number) => request<{ message: string }>(`/api/v1/users/${id}`, { method: 'DELETE' }),

  // ---------- Moderação: vínculo usuário↔tenant (só super admin gerencia) ----------

  listTenantUsers: (tenantId: number) =>
    request<{ users: User[] | null }>(`/api/v1/tenants/${tenantId}/users`).then((r) => r.users ?? []),

  addUserToTenant: (tenantId: number, body: AddUserToTenantRequest) =>
    request<{ message: string }>(`/api/v1/tenants/${tenantId}/users`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  removeUserFromTenant: (tenantId: number, userId: number) =>
    request<{ message: string }>(`/api/v1/tenants/${tenantId}/users/${userId}`, { method: 'DELETE' }),

  getSettings: (tenantId: number) =>
    request<{ settings: Settings }>(`/api/v1/tenants/${tenantId}/settings`).then((r) => r.settings),

  updateSettings: (tenantId: number, settings: Settings) =>
    request<{ message: string }>(`/api/v1/tenants/${tenantId}/settings`, {
      method: 'PUT',
      body: JSON.stringify(settings),
    }),

  listStaffs: (tenantId: number) =>
    request<{ staffs: Staff[] | null }>(`/api/v1/tenants/${tenantId}/staffs`).then((r) => r.staffs ?? []),

  createStaff: (tenantId: number, body: StaffRequest) =>
    request<{ message: string; staff: Staff }>(`/api/v1/tenants/${tenantId}/staffs`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  updateStaff: (staffId: number, body: StaffRequest) =>
    request<{ message: string }>(`/api/v1/staffs/${staffId}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),

  deleteStaff: (staffId: number) =>
    request<{ message: string }>(`/api/v1/staffs/${staffId}`, { method: 'DELETE' }),

  listReports: (tenantId: number) =>
    request<{ reports: Report[] | null }>(`/api/v1/tenants/${tenantId}/reports`).then(
      (r) => r.reports ?? [],
    ),

  listCollaborators: (tenantId: number) =>
    request<{ collaborators: Collaborator[] | null; total: number }>(
      `/api/v1/tenants/${tenantId}/collaborators`,
    ).then((r) => ({ collaborators: r.collaborators ?? [], total: r.total ?? 0 })),

  getWhatsappStatus: (tenantId: number) =>
    request<WhatsAppStatus>(`/api/v1/tenants/${tenantId}/whatsapp/status`),

  connectWhatsapp: (tenantId: number) =>
    request<WhatsAppConnectResponse>(`/api/v1/tenants/${tenantId}/whatsapp/instance`, {
      method: 'POST',
    }),

  disconnectWhatsapp: (tenantId: number) =>
    request<{ success: boolean }>(`/api/v1/tenants/${tenantId}/whatsapp/instance`, {
      method: 'DELETE',
    }),
}
