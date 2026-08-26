// Tipos espelhando o contrato do backend (backend/internal/interface/http/swagger/openapi.yaml)
// Ocorrências/filiais/advertências: ver docs/06_Occurrences_Backend_Contract.md

export type Severity = 'ALERTA' | 'CRITICO' | 'OPERACIONAL'

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
  // Horário (HH:MM) em que o agendador dispara sozinho o fechamento automático de D-1,
  // no máximo uma vez por dia. Vazio = sem agendamento (só auditoria manual).
  horario: string
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
// Este endpoint só devolve ATIVOS (sem Demissao) — para o histórico completo (ativos +
// demitidos), ver CollaboratorHistoryEntry / GET /collaborators/history.
export interface Collaborator {
  id: number
  secullum_id: number
  name: string
}

// Item de GET /tenants/:id/collaborators/history — todos os colaboradores já
// sincronizados do tenant, com admissão/demissão. admissao/demissao vêm como "YYYY-MM-DD"
// ou null.
export interface CollaboratorHistoryEntry {
  id: number
  secullum_id: number
  name: string
  admissao: string | null
  demissao: string | null
  demitido: boolean
}

// ---------- Ocorrências (máquina de estados) ----------

export type OccurrenceState = 'aberta' | 'atualizada' | 'resolvida_automatica' | 'resolvida_manual'

// Eixo de EXIBIÇÃO, diferente de Severity (eixo jurídico). ALTERACAO_ESCALA e
// NAO_CONFIRMADA precisam de cor própria: nenhuma das duas é "quão grave", é "que tipo de
// atenção pede" — cadastro a corrigir, ou reconferência porque o valor mudou.
export type OccurrenceCategory = 'CRITICO' | 'ALERTA' | 'ALTERACAO_ESCALA' | 'NAO_CONFIRMADA'

export interface FixedScheduleDay {
  dia_semana: number
  entrada_1: string
  saida_1: string
  entrada_2: string
  saida_2: string
  carga_minutos: number
}

export type BranchResolutionSource = 'aparelho' | 'numero_folha' | ''

export interface BranchSummary {
  id: number
  name: string
  manager_name: string
  manager_phone: string
  source: BranchResolutionSource
}

export interface Occurrence {
  id: number
  tenant_id: number
  collaborator_id: number // id na Secullum
  collaborator_name: string
  date: string
  type: string
  severity: Severity
  category: OccurrenceCategory
  description: string
  state: OccurrenceState
  first_seen_at: string
  last_seen_at: string
  times_seen: number
  resolved_at: string | null
  ignored_reason?: string
  ignored_by_user_id?: number | null
  horario_fixo: FixedScheduleDay[]
  filial: BranchSummary | null
}

export interface OccurrenceEvent {
  id: number
  type: 'criada' | 'atualizada' | 'resolvida_automatica' | 'resolvida_manual' | 'reaberta'
  from_state: string
  to_state: string
  from_description: string
  to_description: string
  reason: string
  actor_user_id: number | null
  created_at: string
}

// ---------- Equipamentos (relógios de ponto, espelho da Secullum) ----------

// Somente leitura: o cadastro vive na Secullum, sincronizado por
// usecase.SynchronizerService.SyncEquipment (ver GET /tenants/:id/equipamentos).
export interface Equipment {
  id: number
  secullum_id: number
  descricao: string
  endereco_ip: string | null
}

// ---------- Filiais ----------

export interface BranchDevice {
  id: number
  branch_id: number
  secullum_equip_id: number
  label: string
}

export interface BranchPayrollNumber {
  id: number
  branch_id: number
  numero: string
}

export interface Branch {
  id: number
  tenant_id: number
  name: string
  manager_name: string
  manager_phone: string
  devices: BranchDevice[]
  payroll_numbers: BranchPayrollNumber[]
}

export interface BranchRequest {
  name: string
  manager_name: string
  manager_phone: string
}

export interface BranchDeviceRequest {
  secullum_equip_id: number
  label: string
}

export interface BranchPayrollNumberRequest {
  numero: string
}

// ---------- Autopreenchimento (colaborador) ----------

export interface CollaboratorPrefill {
  collaborator: { id: number; secullum_id: number; name: string; numero_folha: string }
  horario_fixo: FixedScheduleDay[]
  filial: BranchSummary | null
}

// ---------- Advertências ----------

export type WarningStatus = 'draft' | 'enviada' | 'assinada'

export interface Warning {
  id: number
  tenant_id: number
  occurrence_id: number | null
  collaborator_id: number // id na Secullum
  collaborator_name: string
  branch_id: number | null
  body: string
  status: WarningStatus
  created_by_user_id: number | null
  created_at: string
  updated_at: string
  sent_at: string | null
  signed_at: string | null
}

export interface CreateWarningRequest {
  collaborator_id: number
  occurrence_id?: number | null
  branch_id?: number | null
  body: string
  status?: WarningStatus
}

export interface WarningCounts {
  draft: number
  enviada: number
  assinada: number
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
