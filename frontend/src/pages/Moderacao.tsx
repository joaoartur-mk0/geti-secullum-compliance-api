import {
  Ban,
  Building2,
  CheckCircle2,
  Link2,
  Plus,
  ShieldAlert,
  Trash2,
  Unlink,
  UserCog,
  Users as UsersIcon,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { Button, EmptyState, ErrorNote, Field, Input, Select, Skeleton, useToast } from '../components/ui'
import { api, ApiError } from '../lib/api'
import { isValidPhone, normalizePhone } from '../lib/format'
import { isSuperAdmin } from '../lib/session'
import type { Tenant, User } from '../lib/types'

// Painel de moderação: gestão global de usuários e empresas (tenants), incluindo o
// vínculo entre eles. Restrito a super admin — ver docs/05_Auth_Backend_Contract.md.
export default function Moderacao() {
  const [tab, setTab] = useState<'usuarios' | 'empresas'>('usuarios')

  if (!isSuperAdmin()) return <Navigate to="/" replace />

  return (
    <div className="animate-rise">
      <header>
        <div className="flex items-center gap-2.5">
          <ShieldAlert size={22} className="text-brand" aria-hidden />
          <h1 className="text-2xl font-semibold tracking-tight">Moderação</h1>
        </div>
        <p className="mt-1 max-w-prose text-sm text-ink-soft">
          Gestão global de usuários e empresas: cadastro, ativação/desativação, exclusão e o
          vínculo entre quem acessa qual empresa. Visível só para super admins.
        </p>
      </header>

      <div className="mt-6 flex gap-1 border-b border-line">
        <TabButton active={tab === 'usuarios'} onClick={() => setTab('usuarios')} icon={UsersIcon}>
          Usuários
        </TabButton>
        <TabButton active={tab === 'empresas'} onClick={() => setTab('empresas')} icon={Building2}>
          Empresas
        </TabButton>
      </div>

      <div className="mt-6">
        {tab === 'usuarios' ? <UsersPanel /> : <TenantsPanel />}
      </div>
    </div>
  )
}

function TabButton({
  active,
  onClick,
  icon: Icon,
  children,
}: {
  active: boolean
  onClick: () => void
  icon: typeof UsersIcon
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex min-h-11 items-center gap-2 border-b-2 px-3 text-sm font-semibold transition-colors duration-150 ${
        active ? 'border-brand text-brand' : 'border-transparent text-ink-soft hover:text-ink'
      }`}
    >
      <Icon size={16} aria-hidden />
      {children}
    </button>
  )
}

function StatusBadge({ active }: { active: boolean }) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-semibold ${
        active ? 'bg-ok-bg text-ok' : 'bg-critico-bg text-critico'
      }`}
    >
      {active ? <CheckCircle2 size={13} aria-hidden /> : <Ban size={13} aria-hidden />}
      {active ? 'Ativo' : 'Inativo'}
    </span>
  )
}

// ================= Usuários =================

type UsersState =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; users: User[] }

function UsersPanel() {
  const toast = useToast()
  const [state, setState] = useState<UsersState>({ phase: 'loading' })
  const [adding, setAdding] = useState(false)

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      setState({ phase: 'ready', users: await api.listUsers() })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar usuários.',
      })
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-ink-soft">
          {state.phase === 'ready' ? `${state.users.length} usuário(s) cadastrado(s).` : ''}
        </p>
        {!adding && (
          <Button onClick={() => setAdding(true)}>
            <Plus size={17} aria-hidden />
            Novo usuário
          </Button>
        )}
      </div>

      {adding && (
        <div className="mt-4">
          <NewUserForm
            onCancel={() => setAdding(false)}
            onCreated={(user) => {
              toast('success', `${user.name} cadastrado.`)
              setAdding(false)
              void load()
            }}
          />
        </div>
      )}

      <div className="mt-4">
        {state.phase === 'loading' && (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </div>
        )}

        {state.phase === 'error' && <ErrorNote message={state.message} onRetry={load} />}

        {state.phase === 'ready' && state.users.length === 0 && !adding && (
          <EmptyState
            icon={<UsersIcon size={32} strokeWidth={1.5} />}
            title="Nenhum usuário cadastrado"
            description="Cadastre o primeiro usuário para liberar o acesso ao painel."
            action={
              <Button onClick={() => setAdding(true)}>
                <Plus size={17} aria-hidden />
                Novo usuário
              </Button>
            }
          />
        )}

        {state.phase === 'ready' && state.users.length > 0 && (
          <ul className="flex flex-col gap-2">
            {state.users.map((user) => (
              <UserRow
                key={user.id}
                user={user}
                onChanged={(message) => {
                  toast('success', message)
                  void load()
                }}
                onError={(message) => toast('error', message)}
              />
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}

function UserRow({
  user,
  onChanged,
  onError,
}: {
  user: User
  onChanged: (message: string) => void
  onError: (message: string) => void
}) {
  const [mode, setMode] = useState<'view' | 'confirm-delete'>('view')
  const [busy, setBusy] = useState(false)

  async function toggleActive() {
    setBusy(true)
    try {
      if (user.active) {
        await api.deactivateUser(user.id)
        onChanged(`${user.name} desativado — não conseguirá mais entrar no painel.`)
      } else {
        await api.activateUser(user.id)
        onChanged(`${user.name} reativado.`)
      }
    } catch (error) {
      onError(error instanceof ApiError ? error.message : 'Falha ao atualizar o usuário.')
    } finally {
      setBusy(false)
    }
  }

  async function remove() {
    setBusy(true)
    try {
      await api.deleteUser(user.id)
      onChanged(`${user.name} excluído.`)
    } catch (error) {
      onError(error instanceof ApiError ? error.message : 'Falha ao excluir o usuário.')
      setBusy(false)
    }
  }

  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-card border border-line bg-bg px-4 py-3.5 shadow-card">
      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-brand-soft font-semibold text-brand">
        {initials(user.name)}
      </span>
      <div className="min-w-0 flex-1">
        <p className="flex flex-wrap items-center gap-2 truncate font-semibold">
          {user.name}
          {user.is_super_admin && (
            <span className="inline-flex items-center gap-1 rounded-full bg-brand-soft px-2 py-0.5 text-[11px] font-semibold text-brand-strong">
              <ShieldAlert size={11} aria-hidden />
              Super admin
            </span>
          )}
        </p>
        <p className="truncate text-sm text-ink-soft">{user.email}</p>
      </div>
      <StatusBadge active={user.active} />

      {mode === 'confirm-delete' ? (
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-critico">Excluir?</span>
          <Button variant="danger" busy={busy} onClick={remove}>
            Sim, excluir
          </Button>
          <Button variant="ghost" disabled={busy} onClick={() => setMode('view')}>
            Cancelar
          </Button>
        </div>
      ) : (
        <div className="flex items-center gap-1">
          <Button variant="secondary" busy={busy} onClick={toggleActive} className="min-h-9 px-3 text-xs">
            {user.active ? 'Desativar' : 'Ativar'}
          </Button>
          <button
            type="button"
            onClick={() => setMode('confirm-delete')}
            aria-label={`Excluir ${user.name}`}
            className="rounded-field p-2.5 text-ink-faint transition-colors duration-150 hover:bg-critico-bg hover:text-critico"
          >
            <Trash2 size={17} />
          </button>
        </div>
      )}
    </li>
  )
}

function NewUserForm({
  onCreated,
  onCancel,
}: {
  onCreated: (user: User) => void
  onCancel: () => void
}) {
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (password.length < 8) {
      setError('A senha precisa ter no mínimo 8 caracteres.')
      return
    }
    setBusy(true)
    setError(null)
    try {
      const { user } = await api.registerUser({ name: name.trim(), email: email.trim(), password })
      onCreated(user)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Erro inesperado ao cadastrar o usuário.')
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="animate-rise rounded-card border border-brand/30 bg-brand-soft/40 p-4">
      <p className="font-semibold">Novo usuário</p>
      <div className="mt-3 grid gap-3 sm:grid-cols-3">
        <Field label="Nome">
          <Input required autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Fulano de Tal" />
        </Field>
        <Field label="E-mail">
          <Input
            required
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="voce@empresa.com.br"
          />
        </Field>
        <Field label="Senha" hint="Mínimo de 8 caracteres.">
          <Input
            required
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="••••••••"
          />
        </Field>
      </div>
      {error && <p className="mt-2 text-sm font-medium text-critico">{error}</p>}
      <div className="mt-4 flex gap-2">
        <Button type="submit" busy={busy}>
          Cadastrar usuário
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancelar
        </Button>
      </div>
    </form>
  )
}

// ================= Empresas =================

type TenantsState =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; tenants: Tenant[] }

function TenantsPanel() {
  const toast = useToast()
  const [state, setState] = useState<TenantsState>({ phase: 'loading' })
  const [adding, setAdding] = useState(false)
  const [linkingTenant, setLinkingTenant] = useState<Tenant | null>(null)

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      setState({ phase: 'ready', tenants: await api.listTenants(true) })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar empresas.',
      })
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-ink-soft">
          {state.phase === 'ready' ? `${state.tenants.length} empresa(s) cadastrada(s).` : ''}
        </p>
        {!adding && (
          <Button onClick={() => setAdding(true)}>
            <Plus size={17} aria-hidden />
            Nova empresa
          </Button>
        )}
      </div>

      {adding && (
        <div className="mt-4">
          <NewTenantForm
            onCancel={() => setAdding(false)}
            onCreated={(tenant) => {
              toast('success', `${tenant.name} cadastrada.`)
              setAdding(false)
              void load()
            }}
          />
        </div>
      )}

      <div className="mt-4">
        {state.phase === 'loading' && (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </div>
        )}

        {state.phase === 'error' && <ErrorNote message={state.message} onRetry={load} />}

        {state.phase === 'ready' && state.tenants.length === 0 && !adding && (
          <EmptyState
            icon={<Building2 size={32} strokeWidth={1.5} />}
            title="Nenhuma empresa cadastrada"
            description="Cadastre a primeira empresa para começar a auditoria."
            action={
              <Button onClick={() => setAdding(true)}>
                <Plus size={17} aria-hidden />
                Nova empresa
              </Button>
            }
          />
        )}

        {state.phase === 'ready' && state.tenants.length > 0 && (
          <ul className="flex flex-col gap-2">
            {state.tenants.map((tenant) => (
              <TenantRow
                key={tenant.id}
                tenant={tenant}
                linking={linkingTenant?.id === tenant.id}
                onToggleLinking={() =>
                  setLinkingTenant((current) => (current?.id === tenant.id ? null : tenant))
                }
                onChanged={(message) => {
                  toast('success', message)
                  void load()
                }}
                onError={(message) => toast('error', message)}
              />
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}

function TenantRow({
  tenant,
  linking,
  onToggleLinking,
  onChanged,
  onError,
}: {
  tenant: Tenant
  linking: boolean
  onToggleLinking: () => void
  onChanged: (message: string) => void
  onError: (message: string) => void
}) {
  const [mode, setMode] = useState<'view' | 'confirm-delete'>('view')
  const [busy, setBusy] = useState(false)

  async function toggleActive() {
    setBusy(true)
    try {
      if (tenant.active) {
        await api.deactivateTenant(tenant.id)
        onChanged(`${tenant.name} desativada.`)
      } else {
        await api.activateTenant(tenant.id)
        onChanged(`${tenant.name} reativada.`)
      }
    } catch (error) {
      onError(error instanceof ApiError ? error.message : 'Falha ao atualizar a empresa.')
    } finally {
      setBusy(false)
    }
  }

  async function remove() {
    setBusy(true)
    try {
      await api.deleteTenant(tenant.id)
      onChanged(`${tenant.name} apagada, junto com todo o histórico de auditoria.`)
    } catch (error) {
      onError(error instanceof ApiError ? error.message : 'Falha ao apagar a empresa.')
      setBusy(false)
    }
  }

  return (
    <li className="rounded-card border border-line bg-bg shadow-card">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3.5">
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-brand-soft text-brand">
          <Building2 size={18} aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate font-semibold">{tenant.name}</p>
          <p className="text-sm text-ink-soft">banco Secullum {tenant.secullum_database_id}</p>
        </div>
        <StatusBadge active={tenant.active} />

        {mode === 'confirm-delete' ? (
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-medium text-critico">
              Apagar {tenant.name} e todo o histórico (relatórios, colaboradores, gestores)?
            </span>
            <Button variant="danger" busy={busy} onClick={remove}>
              Sim, apagar
            </Button>
            <Button variant="ghost" disabled={busy} onClick={() => setMode('view')}>
              Cancelar
            </Button>
          </div>
        ) : (
          <div className="flex items-center gap-1">
            <Button variant="secondary" onClick={onToggleLinking} className="min-h-9 px-3 text-xs">
              <Link2 size={14} aria-hidden />
              Usuários
            </Button>
            <Button variant="secondary" busy={busy} onClick={toggleActive} className="min-h-9 px-3 text-xs">
              {tenant.active ? 'Desativar' : 'Ativar'}
            </Button>
            <button
              type="button"
              onClick={() => setMode('confirm-delete')}
              aria-label={`Apagar ${tenant.name}`}
              className="rounded-field p-2.5 text-ink-faint transition-colors duration-150 hover:bg-critico-bg hover:text-critico"
            >
              <Trash2 size={17} />
            </button>
          </div>
        )}
      </div>

      {linking && (
        <div className="border-t border-line px-4 py-3.5">
          <TenantUserLinks tenant={tenant} onError={onError} />
        </div>
      )}
    </li>
  )
}

function TenantUserLinks({ tenant, onError }: { tenant: Tenant; onError: (message: string) => void }) {
  const toast = useToast()
  const [linked, setLinked] = useState<User[] | null>(null)
  const [allUsers, setAllUsers] = useState<User[] | null>(null)
  const [selected, setSelected] = useState('')
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      const [linkedUsers, users] = await Promise.all([api.listTenantUsers(tenant.id), api.listUsers()])
      setLinked(linkedUsers)
      setAllUsers(users)
    } catch (error) {
      onError(error instanceof ApiError ? error.message : 'Falha ao carregar os usuários vinculados.')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tenant.id])

  useEffect(() => {
    void load()
  }, [load])

  async function link() {
    if (!selected) return
    setBusy(true)
    try {
      await api.addUserToTenant(tenant.id, { user_id: Number(selected) })
      setSelected('')
      toast('success', 'Usuário vinculado à empresa.')
      void load()
    } catch (error) {
      onError(error instanceof ApiError ? error.message : 'Falha ao vincular o usuário.')
    } finally {
      setBusy(false)
    }
  }

  async function unlink(user: User) {
    setBusy(true)
    try {
      await api.removeUserFromTenant(tenant.id, user.id)
      toast('success', `${user.name} desvinculado de ${tenant.name}.`)
      void load()
    } catch (error) {
      onError(error instanceof ApiError ? error.message : 'Falha ao desvincular o usuário.')
    } finally {
      setBusy(false)
    }
  }

  if (linked === null || allUsers === null) {
    return <Skeleton className="h-10 w-full" />
  }

  const linkedIds = new Set(linked.map((u) => u.id))
  const available = allUsers.filter((u) => !linkedIds.has(u.id) && !u.is_super_admin)

  return (
    <div>
      <p className="text-xs font-semibold uppercase tracking-wide text-ink-faint">
        Usuários com acesso a {tenant.name}
      </p>

      {linked.length === 0 ? (
        <p className="mt-2 text-sm text-ink-soft">Nenhum usuário vinculado ainda (além dos super admins, que acessam tudo).</p>
      ) : (
        <ul className="mt-2 flex flex-col gap-1.5">
          {linked.map((user) => (
            <li
              key={user.id}
              className="flex items-center justify-between gap-3 rounded-field bg-panel px-3 py-2 text-sm"
            >
              <span className="min-w-0 flex-1 truncate">
                <span className="font-medium">{user.name}</span>{' '}
                <span className="text-ink-soft">{user.email}</span>
              </span>
              <button
                type="button"
                onClick={() => unlink(user)}
                disabled={busy}
                aria-label={`Desvincular ${user.name} de ${tenant.name}`}
                className="shrink-0 rounded-field p-1.5 text-ink-faint transition-colors duration-150 hover:bg-critico-bg hover:text-critico disabled:opacity-50"
              >
                <Unlink size={15} />
              </button>
            </li>
          ))}
        </ul>
      )}

      {available.length > 0 && (
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Select value={selected} onChange={(e) => setSelected(e.target.value)} className="min-h-9 flex-1">
            <option value="">Selecione um usuário para vincular…</option>
            {available.map((u) => (
              <option key={u.id} value={u.id}>
                {u.name} ({u.email})
              </option>
            ))}
          </Select>
          <Button onClick={link} disabled={!selected} busy={busy} className="min-h-9 px-3 text-xs">
            <UserCog size={14} aria-hidden />
            Vincular
          </Button>
        </div>
      )}
    </div>
  )
}

function NewTenantForm({
  onCreated,
  onCancel,
}: {
  onCreated: (tenant: Tenant) => void
  onCancel: () => void
}) {
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
      const { tenant } = await api.createTenant({
        name: name.trim(),
        secullum_database_id: Number(databaseId),
        staff_name: staffName.trim(),
        staff_contact: normalizePhone(staffContact),
      })
      onCreated(tenant)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Erro inesperado ao cadastrar a empresa.')
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="animate-rise rounded-card border border-brand/30 bg-brand-soft/40 p-4">
      <p className="font-semibold">Nova empresa</p>
      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <Field label="Nome da empresa">
          <Input required autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Empresa Exemplo S.A." />
        </Field>
        <Field label="ID do banco no Secullum">
          <Input
            required
            inputMode="numeric"
            pattern="[0-9]+"
            value={databaseId}
            onChange={(e) => setDatabaseId(e.target.value)}
            placeholder="123"
          />
        </Field>
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
      {error && <p className="mt-2 text-sm font-medium text-critico">{error}</p>}
      <div className="mt-4 flex gap-2">
        <Button type="submit" busy={busy}>
          Cadastrar empresa
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancelar
        </Button>
      </div>
    </form>
  )
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/)
  const first = parts[0]?.[0] ?? ''
  const last = parts.length > 1 ? parts[parts.length - 1][0] : ''
  return (first + last).toUpperCase()
}
