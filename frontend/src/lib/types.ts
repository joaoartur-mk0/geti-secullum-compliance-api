// Tipos espelhando o contrato do backend (backend/internal/interface/http/swagger/openapi.yaml)

export type Severity = 'ALERTA' | 'CRITICO'

export interface Tenant {
  id: number
  name: string
  secullum_database_id: number
  active: boolean
}

export interface AddUserToTenantRequest {
  user_id: number
}

export interface CreateTenantRequest {
  name: string
  secullum_database_id: number
  staff_name: string
  staff_contact: string
}

export interface Staff {
  id: number
  tenant_id: number
  name: string
  celular: string
}

export interface StaffRequest {
  name: string
  celular: string
}

export interface Settings {
  almoco: boolean
  interjornada: boolean
  hextras: boolean
  esquecimento: boolean
  almoco_severity: Severity
  interjornada_severity: Severity
  esquecimento_severity: Severity
  horarios: string[]
}

// Campos capitalizados: o backend serializa as structs Go sem json tags neste objeto
export interface AuditInconsistency {
  CollaboratorID: number
  CollaboratorName: string
  Type: string
  Severity: Severity
  Description: string
}

// ReportMetrics: indicadores operacionais consolidados de uma varredura.
// AINDA NÃO EXISTE NO BACKEND — ver docs/03_Metrics_Frontend_Contract.md. A aba Indicadores
// já consome este objeto quando ele estiver presente no relatório; enquanto não
// estiver, a aba deriva o que consegue da lista de `inconsistencies`.
export interface ReportMetrics {
  collaborators_audited: number // nº de colaboradores avaliados na varredura
  clean_count: number // colaboradores sem nenhuma inconsistência
  compliance_rate: number // 0–100 = clean_count / collaborators_audited * 100
  total_inconsistencies: number
  critical: number // inconsistências de severidade CRITICO
  alert: number // inconsistências de severidade ALERTA
  by_type: Record<string, number> // { "Batida Esquecida": 3, "Almoço Reduzido": 1, ... }
  overtime_hours_total: number // soma das horas extras do dia (horas)
  late_hours_total: number // soma dos atrasos do dia (horas)
}

export interface Report {
  id: number
  tenant_id: number
  date: string
  data_generated: string
  total: number
  inconsistencies: AuditInconsistency[] | null
  metrics?: ReportMetrics | null // preenchido pelo backend quando a varredura completa entrar
}

// Colaborador sincronizado (espelho local do tenant, via GET /tenants/:id/collaborators).
export interface Collaborator {
  id: number
  secullum_id: number
  name: string
}

// Estado da instância de WhatsApp do tenant na Evolution API.
export interface WhatsAppStatus {
  connected: boolean
  state: string
}

// Resposta de conexão: qrcode (data URI base64) presente quando há QR a escanear.
export interface WhatsAppConnectResponse {
  qrcode?: string
  connected: boolean
  state: string
}

export interface HealthResponse {
  status: string
  database: string
  rabbitmq: string
}

export interface ApiErrorBody {
  error?: {
    code?: 'VALIDATION' | 'NOT_FOUND' | 'CONFLICT' | 'FORBIDDEN' | 'INTERNAL' | 'UNAUTHORIZED'
    message?: string
    details?: string | null
  }
}

// Autenticação — ver docs/05_Auth_Backend_Contract.md
export interface User {
  id: number
  name: string
  email: string
  is_super_admin: boolean
  active: boolean
}

export interface LoginRequest {
  email: string
  password: string
}

export interface LoginResponse {
  token: string
  user: User
  tenant_ids: number[]
}

export interface RegisterUserRequest {
  name: string
  email: string
  password: string
}
