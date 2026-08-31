import {
  AlertTriangle,
  Ban,
  CheckCircle2,
  Loader2,
  OctagonAlert,
  PenLine,
  RotateCcw,
  Send,
  Settings2,
  X,
} from 'lucide-react'
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
} from 'react'
import { CATEGORY_CLASSES, CATEGORY_HINT, CATEGORY_LABEL } from '../lib/categories'
import type { OccurrenceCategory, OccurrenceState, Severity, WarningStatus } from '../lib/types'

// ---------- Botões ----------

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'

const buttonStyles: Record<ButtonVariant, string> = {
  primary:
    'bg-brand text-white hover:bg-brand-strong active:bg-brand-strong disabled:bg-brand/45',
  secondary:
    'border border-line bg-bg text-ink hover:border-ink-faint hover:bg-panel disabled:text-ink-faint',
  ghost: 'text-ink-soft hover:bg-panel hover:text-ink disabled:text-ink-faint',
  danger:
    'border border-critico/30 bg-bg text-critico hover:bg-critico-bg disabled:opacity-50',
}

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  busy?: boolean
}

export function Button({ variant = 'primary', busy, disabled, children, className = '', ...rest }: ButtonProps) {
  return (
    <button
      {...rest}
      disabled={disabled || busy}
      className={`inline-flex min-h-11 items-center justify-center gap-2 rounded-field px-4 text-sm font-semibold transition-colors duration-150 disabled:cursor-not-allowed ${buttonStyles[variant]} ${className}`}
    >
      {busy && <Loader2 size={16} className="animate-spin" aria-hidden />}
      {children}
    </button>
  )
}

// ---------- Campos ----------

export function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-sm font-medium text-ink">{label}</span>
      {children}
      {hint && <span className="text-xs text-ink-soft">{hint}</span>}
    </label>
  )
}

export function Input({ className = '', ...rest }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...rest}
      className={`min-h-11 rounded-field border border-line bg-bg px-3 text-sm text-ink transition-colors duration-150 placeholder:text-ink-faint hover:border-ink-faint focus:border-brand ${className}`}
    />
  )
}

export function Select({ className = '', children, ...rest }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    // Um <select> nativo cresce até a largura da opção mais longa (ex.: nome completo de
    // colaborador) e estoura o cartão de filtro no mobile. `max-w-full` sozinho não basta
    // quando o <select> está dentro de um <label> que é item de um pai `flex-wrap`: o
    // label também dimensiona pelo conteúdo, então o 100% se resolve contra uma largura
    // que o próprio conteúdo definiu. A contenção real é o `max-w-full` NO LABEL — este
    // aqui é o complemento, não a solução.
    <select
      {...rest}
      className={`min-h-11 max-w-full rounded-field border border-line bg-bg px-3 text-sm text-ink transition-colors duration-150 hover:border-ink-faint focus:border-brand ${className}`}
    >
      {children}
    </select>
  )
}

export function Toggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean
  onChange: (next: boolean) => void
  label: string
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      onClick={() => onChange(!checked)}
      className={`relative h-7 w-12 shrink-0 rounded-full transition-colors duration-200 ${
        checked ? 'bg-brand' : 'bg-line'
      }`}
    >
      <span
        className={`absolute top-1 h-5 w-5 rounded-full bg-white shadow-sm transition-[left] duration-200 ease-out ${
          checked ? 'left-6' : 'left-1'
        }`}
      />
    </button>
  )
}

// ---------- Severidade ----------

// OPERACIONAL não é "quão grave" — é cadastro/escala a corrigir (ver
// domain.SeverityOperational no backend). Cor e ícone próprios para não competir com
// crítico/alerta na leitura de relance (Design Principle 1 do PRODUCT.md).
export function SeverityBadge({ severity }: { severity: Severity }) {
  if (severity === 'OPERACIONAL') {
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-operacional-bg px-2.5 py-1 text-xs font-semibold text-operacional">
        <Settings2 size={13} aria-hidden />
        Operacional
      </span>
    )
  }
  const critical = severity === 'CRITICO'
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-semibold ${
        critical ? 'bg-critico-bg text-critico' : 'bg-alerta-bg text-alerta'
      }`}
    >
      {critical ? <OctagonAlert size={13} aria-hidden /> : <AlertTriangle size={13} aria-hidden />}
      {critical ? 'Crítico' : 'Alerta'}
    </span>
  )
}

// ---------- Categoria de ocorrência (eixo de exibição — ver lib/categories.ts) ----------

const categoryIcon: Record<OccurrenceCategory, typeof OctagonAlert> = {
  CRITICO: OctagonAlert,
  ALERTA: AlertTriangle,
  ALTERACAO_ESCALA: Settings2,
  NAO_CONFIRMADA: RotateCcw,
}

export function CategoryBadge({ category }: { category: OccurrenceCategory }) {
  const Icon = categoryIcon[category]
  const classes = CATEGORY_CLASSES[category]
  return (
    <span
      title={CATEGORY_HINT[category]}
      className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-semibold ${classes.bg} ${classes.text}`}
    >
      <Icon size={13} aria-hidden />
      {CATEGORY_LABEL[category]}
    </span>
  )
}

// ---------- Estado da ocorrência ----------

const stateLabel: Record<OccurrenceState, string> = {
  aberta: 'Aberta',
  atualizada: 'Atualizada',
  resolvida_automatica: 'Resolvida',
  resolvida_manual: 'Ignorada',
}

export function OccurrenceStateBadge({ state }: { state: OccurrenceState }) {
  if (state === 'resolvida_automatica') {
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-ok-bg px-2.5 py-1 text-xs font-semibold text-ok">
        <CheckCircle2 size={13} aria-hidden />
        {stateLabel[state]}
      </span>
    )
  }
  if (state === 'resolvida_manual') {
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-panel px-2.5 py-1 text-xs font-semibold text-ink-soft">
        <Ban size={13} aria-hidden />
        {stateLabel[state]}
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-revisar-bg px-2.5 py-1 text-xs font-semibold text-revisar">
      {state === 'atualizada' ? <RotateCcw size={13} aria-hidden /> : <OctagonAlert size={13} aria-hidden />}
      {stateLabel[state]}
    </span>
  )
}

// ---------- Status de advertência ----------

const warningStatusLabel: Record<WarningStatus, string> = {
  draft: 'Rascunho',
  enviada: 'Enviada',
  assinada: 'Assinada',
}

export function WarningStatusBadge({ status }: { status: WarningStatus }) {
  if (status === 'assinada') {
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-ok-bg px-2.5 py-1 text-xs font-semibold text-ok">
        <CheckCircle2 size={13} aria-hidden />
        {warningStatusLabel[status]}
      </span>
    )
  }
  if (status === 'enviada') {
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-alerta-bg px-2.5 py-1 text-xs font-semibold text-alerta">
        <Send size={13} aria-hidden />
        {warningStatusLabel[status]}
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-panel px-2.5 py-1 text-xs font-semibold text-ink-soft">
      <PenLine size={13} aria-hidden />
      {warningStatusLabel[status]}
    </span>
  )
}

// ---------- Estados ----------

export function Skeleton({ className = '' }: { className?: string }) {
  return <div className={`animate-pulse rounded-field bg-panel ${className}`} aria-hidden />
}

export function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon: ReactNode
  title: string
  description: string
  action?: ReactNode
}) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-card border border-dashed border-line px-6 py-12 text-center">
      <div className="text-ink-faint">{icon}</div>
      <div>
        <p className="font-semibold text-ink">{title}</p>
        <p className="mx-auto mt-1 max-w-md text-sm text-ink-soft">{description}</p>
      </div>
      {action}
    </div>
  )
}

export function ErrorNote({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-card border border-critico/25 bg-critico-bg px-4 py-3 text-sm text-critico">
      <OctagonAlert size={18} className="shrink-0" aria-hidden />
      <span className="flex-1">{message}</span>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="rounded-field px-2 py-1 font-semibold underline underline-offset-2 hover:bg-critico/10"
        >
          Tentar de novo
        </button>
      )}
    </div>
  )
}

// ---------- Toast ----------

interface Toast {
  id: number
  kind: 'success' | 'error'
  message: string
}

const ToastContext = createContext<(kind: Toast['kind'], message: string) => void>(() => {})

export function useToast() {
  return useContext(ToastContext)
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextId = useRef(1)

  const push = useCallback((kind: Toast['kind'], message: string) => {
    const id = nextId.current++
    setToasts((current) => [...current, { id, kind, message }])
  }, [])

  return (
    <ToastContext.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed inset-x-0 bottom-20 z-50 flex flex-col items-center gap-2 px-4 md:bottom-6">
        {toasts.map((toast) => (
          <ToastItem
            key={toast.id}
            toast={toast}
            onDone={() => setToasts((current) => current.filter((t) => t.id !== toast.id))}
          />
        ))}
      </div>
    </ToastContext.Provider>
  )
}

function ToastItem({ toast, onDone }: { toast: Toast; onDone: () => void }) {
  useEffect(() => {
    const timer = setTimeout(onDone, 4200)
    return () => clearTimeout(timer)
  }, [onDone])

  const success = toast.kind === 'success'
  return (
    <div
      role="status"
      className={`pointer-events-auto flex w-full max-w-sm animate-rise items-center gap-2.5 rounded-card px-4 py-3 text-sm font-medium shadow-float ${
        success ? 'bg-side text-side-ink' : 'bg-critico text-white'
      }`}
    >
      {success ? (
        <CheckCircle2 size={17} className="shrink-0 text-brand-ring" aria-hidden />
      ) : (
        <OctagonAlert size={17} className="shrink-0" aria-hidden />
      )}
      <span className="flex-1">{toast.message}</span>
      <button
        type="button"
        onClick={onDone}
        aria-label="Fechar aviso"
        className="rounded p-0.5 opacity-70 hover:opacity-100"
      >
        <X size={15} />
      </button>
    </div>
  )
}
