import {
  BarChart3,
  Bell,
  Building2,
  Contact,
  History,
  LandPlot,
  LayoutDashboard,
  LogOut,
  MessageCircle,
  ShieldAlert,
  ShieldCheck,
  Users,
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
  { to: '/', label: 'Painel', icon: LayoutDashboard, end: true, superAdminOnly: false },
  { to: '/auditorias', label: 'Auditorias', icon: History, end: false, superAdminOnly: false },
  { to: '/indicadores', label: 'Indicadores', icon: BarChart3, end: false, superAdminOnly: false },
  { to: '/colaboradores', label: 'Colaboradores', icon: Contact, end: false, superAdminOnly: false },
  { to: '/filiais', label: 'Filiais', icon: LandPlot, end: false, superAdminOnly: false },
  { to: '/gestores', label: 'Gestores', icon: Users, end: false, superAdminOnly: false },
  { to: '/avisos', label: 'Avisos', icon: Bell, end: false, superAdminOnly: false },
  { to: '/whatsapp', label: 'WhatsApp', icon: MessageCircle, end: false, superAdminOnly: false },
  { to: '/empresa', label: 'Empresa', icon: Building2, end: false, superAdminOnly: false },
  { to: '/moderacao', label: 'Moderação', icon: ShieldAlert, end: false, superAdminOnly: true },
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

  if (!session) return <Navigate to="/login" replace />

  function logout() {
    endSession()
    navigate('/login', { replace: true })
  }

  return (
    <div className="min-h-dvh bg-bg md:flex">
      {/* Navegação desktop */}
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-60 flex-col bg-side text-side-ink md:flex">
        <div className="flex items-center gap-2.5 px-5 pt-6 pb-8">
          <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-side-raise">
            <ShieldCheck size={20} className="text-brand-ring" aria-hidden />
          </span>
          <div className="leading-tight">
            <p className="text-sm font-semibold text-white">Secullum Compliance</p>
            <p className="text-xs text-side-faint">por Geti Soluções</p>
          </div>
        </div>

        <nav className="flex flex-1 flex-col gap-1 px-3" aria-label="Principal">
          {items.map(({ to, label, icon: Icon, end }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
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
            className="mt-2 flex items-center gap-2 rounded-field text-sm font-medium text-side-ink transition-colors duration-150 hover:text-white"
          >
            <LogOut size={15} aria-hidden />
            Sair
          </button>
        </div>
      </aside>

      {/* Cabeçalho mobile */}
      <header className="sticky top-0 z-30 flex items-center justify-between bg-side px-4 py-3 text-side-ink md:hidden">
        <div className="flex items-center gap-2.5">
          <ShieldCheck size={20} className="text-brand-ring" aria-hidden />
          <div className="leading-tight">
            <p className="text-sm font-semibold text-white">Secullum Compliance</p>
            {state.phase === 'ready' && (
              <p className="max-w-52 truncate text-xs text-side-faint">{state.tenant.name}</p>
            )}
          </div>
        </div>
        <button
          type="button"
          onClick={logout}
          aria-label="Sair"
          className="rounded-field p-2 text-side-faint hover:text-white"
        >
          <LogOut size={18} />
        </button>
      </header>

      {/* Conteúdo */}
      <main className="min-w-0 flex-1 pb-24 md:ml-60 md:pb-10">
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

      {/* Navegação mobile */}
      <nav
        className="fixed inset-x-0 bottom-0 z-30 flex border-t border-side-raise bg-side text-side-faint md:hidden"
        aria-label="Principal"
      >
        {items.map(({ to, label, icon: Icon, end }) => (
          <NavLink
            key={to}
            to={to}
            end={end}
            className={({ isActive }) =>
              `flex min-h-14 flex-1 flex-col items-center justify-center gap-0.5 text-[11px] font-medium transition-colors duration-150 ${
                isActive ? 'text-white' : 'hover:text-side-ink'
              }`
            }
          >
            {({ isActive }) => (
              <>
                <Icon size={20} className={isActive ? 'text-brand-ring' : undefined} aria-hidden />
                {label}
              </>
            )}
          </NavLink>
        ))}
      </nav>
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
