import type {
  ApiErrorBody,
  Collaborator,
  CreateTenantRequest,
  HealthResponse,
  Report,
  Settings,
  Staff,
  StaffRequest,
  Tenant,
} from './types'

const API_URL_KEY = 'scw.apiUrl'
const DEFAULT_API_URL = 'http://localhost:8080'

export function getApiUrl(): string {
  return localStorage.getItem(API_URL_KEY)?.replace(/\/+$/, '') || DEFAULT_API_URL
}

export function setApiUrl(url: string) {
  const clean = url.trim().replace(/\/+$/, '')
  if (clean && clean !== DEFAULT_API_URL) localStorage.setItem(API_URL_KEY, clean)
  else localStorage.removeItem(API_URL_KEY)
}

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
  let res: Response
  try {
    res = await fetch(`${getApiUrl()}${path}`, {
      ...init,
      headers: { 'Content-Type': 'application/json', ...init?.headers },
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
}
