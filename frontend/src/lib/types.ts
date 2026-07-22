// Tipos espelhando o contrato do backend (backend/internal/interface/http/swagger/openapi.yaml)

export type Severity = 'ALERTA' | 'CRITICO'

export interface Tenant {
  id: number
  name: string
  secullum_database_id: number
  active: boolean
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

export interface Report {
  id: number
  tenant_id: number
  date: string
  data_generated: string
  total: number
  inconsistencies: AuditInconsistency[] | null
}

export interface HealthResponse {
  status: string
  database: string
  rabbitmq: string
}

export interface ApiErrorBody {
  error?: {
    code?: 'VALIDATION' | 'NOT_FOUND' | 'CONFLICT' | 'INTERNAL'
    message?: string
    details?: string | null
  }
}
