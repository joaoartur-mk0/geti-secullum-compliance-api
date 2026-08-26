import {
  BarChart3,
  Bell,
  Building2,
  Contact,
  Fingerprint,
  History,
  LandPlot,
  LogOut,
  Menu,
  MessageCircle,
  ScrollText,
  ShieldAlert,
  ShieldCheck,
  Users,
  X,
} from 'lucide-react'
import { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { NavLink, Navigate, Outlet, useNavigate } from 'react-router-dom'
import { Button, EmptyState, ErrorNote, Field, Input, Skeleton } from '../components/ui'
import { api, ApiError } from '../lib/api'
import { endSession, getSavedTenantId, getSession, isSuperAdmin, saveTenantId } from '../lib/session'
import { isValidPhone, normalizePhone } from '../lib/format'
import type { Tenant } from '../lib/types'

// ---------- Contexto de tenant ----------

interface TenantContextValue {
  tenant: Tenant
  tenants: Tenant[]
  reloadTenant: () => void
  switchTenant: (id: number) => void
}

const TenantContext = createContext<TenantContextValue | null>(null)

export function useTenant(): TenantContextValue {
  const value = useContext(TenantContext)
  if (!value) throw new Error('useTenant fora do AppShell')
  return value
}

// ---------- Shell ----------

const navItems = [
  { to: '/indicadores', label: 'Indicadores', icon: BarChart3, end: false, superAdminOnly: false },
  { to: '/auditorias', label: 'Situação por dia', icon: History, end: false, superAdminOnly: false },
  { to: '/colaboradores', label: 'Colaboradores', icon: Contact, end: false, superAdminOnly: false },
  { to: '/filiais', label: 'Filiais', icon: LandPlot, end: false, superAdminOnly: false },
  { to: '/equipamentos', label: 'Equipamentos', icon: Fingerprint, end: false, superAdminOnly: false },
  { to: '/gestores', label: 'Gestores', icon: Users, end: false, superAdminOnly: false },
  { to: '/avisos', label: 'Avisos', icon: Bell, end: false, superAdminOnly: false },
  { to: '/whatsapp', label: 'WhatsApp', icon: MessageCircle, end: false, superAdminOnly: false },
  { to: '/empresa', label: 'Empresa', icon: Building2, end: false, superAdminOnly: false },
  { to: '/moderacao', label: 'Moderação', icon: ShieldAlert, end: false, superAdminOnly: true },
]

// Itens de configurações: menos evidentes que a navegação principal, ficam num bloco
// separado abaixo dela.
const settingsNavItems = [
  { to: '/reports/history', label: 'Registro de execuções', icon: ScrollText, end: false },
]

type TenantState =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'empty' }
  | { phase: 'ready'; tenant: Tenant; tenants: Tenant[] }

export default function AppShell() {
  const session = getSession()
  const navigate = useNavigate()
  const admin = isSuperAdmin()
  const items = navItems.filter((item) => !item.superAdminOnly || admin)
  const [state, setState] = useState<TenantState>({ phase: 'loading' })
  const [mobileNavOpen, setMobileNavOpen] = useState(false)

  const loadTenant = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      const tenants = await api.listTenants()
      if (tenants.length === 0) {
        setState({ phase: 'empty' })
        return
      }
      const savedId = getSavedTenantId()
      const tenant = tenants.find((t) => t.id === savedId) ?? tenants[0]
      saveTenantId(tenant.id)
      setState({ phase: 'ready', tenant, tenants })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro inesperado ao carregar a empresa.',
      })
    }
  }, [])

  const switchTenant = useCallback((id: number) => {
    setState((current) => {
      if (current.phase !== 'ready') return current
      const next = current.tenants.find((t) => t.id === id)
      if (!next) return current
      saveTenantId(next.id)
      return { ...current, tenant: next }
    })
  }, [])

  useEffect(() => {
    if (session) void loadTenant()
  }, [session != null, loadTenant]) // eslint-disable-line react-hooks/exhaustive-deps

  // Trava o scroll de fundo e fecha no Esc enquanto o menu mobile está aberto.
  useEffect(() => {
    if (!mobileNavOpen) return
    document.body.style.overflow = 'hidden'
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setMobileNavOpen(false)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => {
      document.body.style.overflow = ''
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [mobileNavOpen])

  if (!session) return <Navigate to="/login" replace />

  function logout() {
    endSession()
    navigate('/login', { replace: true })
  }

  return (
    <div className="min-h-dvh bg-bg md:flex">
      {/* Backdrop do menu mobile */}
      {mobileNavOpen && (
        <div
          className="fixed inset-0 z-40 bg-ink/50 md:hidden"
          onClick={() => setMobileNavOpen(false)}
          aria-hidden="true"
        />
      )}

      {/* Navegação: drawer no mobile (abre por cima do conteúdo), fixa no desktop */}
      <aside
        className={`fixed inset-y-0 left-0 z-50 flex w-72 flex-col bg-side text-side-ink transition-transform duration-200 ease-out md:z-30 md:w-60 md:translate-x-0 ${
          mobileNavOpen ? 'translate-x-0' : '-translate-x-full'
        }`}
      >
        <div className="flex items-center gap-2.5 px-5 pt-6 pb-8">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-side-raise">
            <ShieldCheck size={20} className="text-brand-ring" aria-hidden />
          </span>
          <div className="min-w-0 leading-tight">
            <p className="truncate text-sm font-semibold text-white">Secullum Compliance</p>
            <p className="truncate text-xs text-side-faint">por Geti Soluções</p>
          </div>
          <button
            type="button"
            onClick={() => setMobileNavOpen(false)}
            aria-label="Fechar menu"
            className="ml-auto flex min-h-11 min-w-11 shrink-0 items-center justify-center rounded-field text-side-faint hover:text-white md:hidden"
          >
            <X size={20} aria-hidden />
          </button>
        </div>

        <nav className="min-h-0 flex-1 overflow-y-auto px-3" aria-label="Principal">
          <div className="flex flex-col gap-1">
            {items.map(({ to, label, icon: Icon, end }) => (
              <NavLink
                key={to}
                to={to}
                end={end}
                onClick={() => setMobileNavOpen(false)}
                className={({ isActive }) =>
                  `flex min-h-11 items-center gap-3 rounded-field px-3 text-sm font-medium transition-colors duration-150 ${
                    isActive
                      ? 'bg-side-raise text-white'
                      : 'text-side-faint hover:bg-side-raise/60 hover:text-side-ink'
                  }`
                }
              >
                <Icon size={18} aria-hidden />
                {label}
              </NavLink>
            ))}
          </div>

          <div className="mt-6 border-t border-side-raise pt-4">
            <p className="px-3 text-xs font-semibold uppercase tracking-wide text-side-faint/70">
              Configurações
            </p>
            <div className="mt-2 flex flex-col gap-1">
              {settingsNavItems.map(({ to, label, icon: Icon, end }) => (
                <NavLink
                  key={to}
                  to={to}
                  end={end}
                  onClick={() => setMobileNavOpen(false)}
                  className={({ isActive }) =>
                    `flex min-h-11 items-center gap-3 rounded-field px-3 text-xs font-medium transition-colors duration-150 ${
                      isActive
                        ? 'bg-side-raise text-side-ink'
                        : 'text-side-faint/70 hover:bg-side-raise/60 hover:text-side-ink'
                    }`
                  }
                >
                  <Icon size={16} aria-hidden />
                  {label}
                </NavLink>
              ))}
            </div>
          </div>
        </nav>

        <div className="border-t border-side-raise px-5 py-4">
          {state.phase === 'ready' &&
            (state.tenants.length > 1 ? (
              <label className="mb-2 flex items-center gap-2 text-xs text-side-faint">
                <Building2 size={14} className="shrink-0" aria-hidden />
                <span className="sr-only">Empresa em consulta</span>
                <select
                  value={state.tenant.id}
                  onChange={(e) => switchTenant(Number(e.target.value))}
                  className="min-h-9 w-full truncate rounded-field border border-side-raise bg-side-raise px-2 text-xs font-medium text-side-ink transition-colors duration-150 hover:text-white"
                >
                  {state.tenants.map((t) => (
                    <option key={t.id} value={t.id}>
                      {t.name}
                    </option>
                  ))}
                </select>
              </label>
            ) : (
              <p className="mb-2 flex items-center gap-2 text-xs text-side-faint">
                <Building2 size={14} aria-hidden />
                <span className="truncate">{state.tenant.name}</span>
              </p>
            ))}
          <p className="truncate text-xs text-side-faint">{session.user.email}</p>
          <button
            type="button"
            onClick={logout}
            className="mt-2 flex min-h-11 items-center gap-2 rounded-field text-sm font-medium text-side-ink transition-colors duration-150 hover:text-white"
          >
            <LogOut size={15} aria-hidden />
            Sair
          </button>
        </div>
      </aside>

      {/* Cabeçalho mobile */}
      <header className="sticky top-0 z-30 flex items-center gap-1 bg-side px-2 py-3 text-side-ink md:hidden">
        <button
          type="button"
          onClick={() => setMobileNavOpen(true)}
          aria-label="Abrir menu"
          aria-expanded={mobileNavOpen}
          className="flex min-h-11 min-w-11 shrink-0 items-center justify-center rounded-field text-side-faint hover:text-white"
        >
          <Menu size={20} aria-hidden />
        </button>
        <div className="flex min-w-0 flex-1 items-center gap-2.5 px-2">
          <ShieldCheck size={20} className="shrink-0 text-brand-ring" aria-hidden />
          <div className="min-w-0 leading-tight">
            <p className="truncate text-sm font-semibold text-white">Secullum Compliance</p>
            {state.phase === 'ready' && (
              <p className="truncate text-xs text-side-faint">{state.tenant.name}</p>
            )}
          </div>
        </div>
      </header>

      {/* Conteúdo */}
      <main className="min-w-0 flex-1 pb-10 md:ml-60 md:pb-10">
        <div className="mx-auto w-full max-w-4xl px-4 pt-6 md:px-10 md:pt-10">
          {state.phase === 'loading' && (
            <div className="flex flex-col gap-4">
              <Skeleton className="h-8 w-56" />
              <Skeleton className="h-40 w-full" />
              <Skeleton className="h-40 w-full" />
            </div>
          )}
          {state.phase === 'error' && <ErrorNote message={state.message} onRetry={loadTenant} />}
          {state.phase === 'empty' && admin && <CreateTenantCard onCreated={loadTenant} />}
          {state.phase === 'empty' && !admin && (
            <EmptyState
              icon={<Building2 size={32} strokeWidth={1.5} />}
              title="Nenhuma empresa vinculada à sua conta"
              description="Fale com quem administra o painel para vincular seu usuário a uma empresa."
            />
          )}
          {state.phase === 'ready' && (
            <TenantContext.Provider
              value={{
                tenant: state.tenant,
                tenants: state.tenants,
                reloadTenant: loadTenant,
                switchTenant,
              }}
            >
              <Outlet />
            </TenantContext.Provider>
          )}
        </div>
      </main>
    </div>
  )
}

// ---------- Primeiro acesso: cadastrar a empresa ----------

function CreateTenantCard({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState('')
  const [databaseId, setDatabaseId] = useState('')
  const [staffName, setStaffName] = useState('')
  const [staffContact, setStaffContact] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!isValidPhone(staffContact)) {
      setError('Informe um celular válido com DDD, ex.: (31) 99999-9999.')
      return
    }
    setBusy(true)
    setError(null)
    try {
      await api.createTenant({
        name: name.trim(),
        secullum_database_id: Number(databaseId),
        staff_name: staffName.trim(),
        staff_contact: normalizePhone(staffContact),
      })
      onCreated()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Erro inesperado ao cadastrar a empresa.')
      setBusy(false)
    }
  }

  return (
    <div className="mx-auto max-w-lg animate-rise">
      <h1 className="text-2xl font-semibold tracking-tight">Vamos começar</h1>
      <p className="mt-2 max-w-prose text-sm leading-relaxed text-ink-soft">
        Nenhuma empresa está cadastrada ainda. Informe os dados do banco Secullum e o primeiro
        gestor que receberá os alertas — os demais podem ser adicionados depois.
      </p>

      <form onSubmit={submit} className="mt-6 flex flex-col gap-4">
        <Field label="Nome da empresa">
          <Input required value={name} onChange={(e) => setName(e.target.value)} placeholder="Empresa Exemplo S.A." />
        </Field>
        <Field
          label="ID do banco no Secullum"
          hint="Número do banco de dados selecionado no Secullum Ponto Web."
        >
          <Input
            required
            inputMode="numeric"
            pattern="[0-9]+"
            value={databaseId}
            onChange={(e) => setDatabaseId(e.target.value)}
            placeholder="123"
          />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Nome do gestor">
            <Input required value={staffName} onChange={(e) => setStaffName(e.target.value)} placeholder="Fulano de Tal" />
          </Field>
          <Field label="Celular do gestor (WhatsApp)">
            <Input
              required
              inputMode="tel"
              value={staffContact}
              onChange={(e) => setStaffContact(e.target.value)}
              placeholder="(31) 99999-9999"
            />
          </Field>
        </div>
        {error && <ErrorNote message={error} />}
        <Button type="submit" busy={busy} className="self-start">
          Cadastrar empresa
        </Button>
      </form>
    </div>
  )
}
