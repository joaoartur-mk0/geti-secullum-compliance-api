import { Pencil, Plus, Trash2, UserRound } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { Button, EmptyState, ErrorNote, Field, Input, Skeleton, useToast } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { formatPhone, isValidPhone, normalizePhone } from '../lib/format'
import type { Staff } from '../lib/types'

type ListState =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; staffs: Staff[] }

export default function Gestores() {
  const { tenant } = useTenant()
  const toast = useToast()
  const [state, setState] = useState<ListState>({ phase: 'loading' })
  const [adding, setAdding] = useState(false)

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      setState({ phase: 'ready', staffs: await api.listStaffs(tenant.id) })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar gestores.',
      })
    }
  }, [tenant.id])

  useEffect(() => {
    void load()
  }, [load])

  async function remove(staff: Staff) {
    try {
      await api.deleteStaff(staff.id)
      toast('success', `${staff.name} não receberá mais alertas.`)
      void load()
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao excluir o gestor.')
    }
  }

  return (
    <div className="animate-rise">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Gestores</h1>
          <p className="mt-1 max-w-prose text-sm text-ink-soft">
            Quem recebe os alertas de irregularidade no WhatsApp. Os avisos seguem as regras
            definidas na aba Avisos.
          </p>
        </div>
        {!adding && state.phase === 'ready' && state.staffs.length > 0 && (
          <Button onClick={() => setAdding(true)}>
            <Plus size={17} aria-hidden />
            Adicionar gestor
          </Button>
        )}
      </header>

      <div className="mt-8">
        {adding && (
          <div className="mb-4">
            <StaffForm
              title="Novo gestor"
              onCancel={() => setAdding(false)}
              onSubmit={async (name, celular) => {
                await api.createStaff(tenant.id, { name, celular })
                toast('success', `${name} agora recebe os alertas.`)
                setAdding(false)
                void load()
              }}
            />
          </div>
        )}

        {state.phase === 'loading' && (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </div>
        )}

        {state.phase === 'error' && <ErrorNote message={state.message} onRetry={load} />}

        {state.phase === 'ready' && state.staffs.length === 0 && !adding && (
          <EmptyState
            icon={<UserRound size={32} strokeWidth={1.5} />}
            title="Nenhum gestor cadastrado"
            description="Sem gestores, os alertas não têm destino. Cadastre ao menos um nome com WhatsApp para receber os avisos."
            action={
              <Button onClick={() => setAdding(true)}>
                <Plus size={17} aria-hidden />
                Adicionar gestor
              </Button>
            }
          />
        )}

        {state.phase === 'ready' && state.staffs.length > 0 && (
          <ul className="flex flex-col gap-2">
            {state.staffs.map((staff) => (
              <StaffRow
                key={staff.id}
                staff={staff}
                onDelete={() => remove(staff)}
                onSaved={() => {
                  toast('success', 'Gestor atualizado.')
                  void load()
                }}
              />
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}

function StaffRow({
  staff,
  onDelete,
  onSaved,
}: {
  staff: Staff
  onDelete: () => void
  onSaved: () => void
}) {
  const [mode, setMode] = useState<'view' | 'edit' | 'confirm-delete'>('view')

  if (mode === 'edit') {
    return (
      <li>
        <StaffForm
          title={`Editar ${staff.name}`}
          initialName={staff.name}
          initialPhone={formatPhone(staff.celular)}
          onCancel={() => setMode('view')}
          onSubmit={async (name, celular) => {
            await api.updateStaff(staff.id, { name, celular })
            setMode('view')
            onSaved()
          }}
        />
      </li>
    )
  }

  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-card border border-line bg-bg px-4 py-3.5 shadow-card">
      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-brand-soft font-semibold text-brand">
        {initials(staff.name)}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate font-semibold">{staff.name}</p>
        <p className="text-sm text-ink-soft">{formatPhone(staff.celular)}</p>
      </div>

      {mode === 'confirm-delete' ? (
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-critico">Excluir?</span>
          <Button variant="danger" onClick={onDelete}>
            Sim, excluir
          </Button>
          <Button variant="ghost" onClick={() => setMode('view')}>
            Cancelar
          </Button>
        </div>
      ) : (
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => setMode('edit')}
            aria-label={`Editar ${staff.name}`}
            className="rounded-field p-2.5 text-ink-faint transition-colors duration-150 hover:bg-panel hover:text-ink"
          >
            <Pencil size={17} />
          </button>
          <button
            type="button"
            onClick={() => setMode('confirm-delete')}
            aria-label={`Excluir ${staff.name}`}
            className="rounded-field p-2.5 text-ink-faint transition-colors duration-150 hover:bg-critico-bg hover:text-critico"
          >
            <Trash2 size={17} />
          </button>
        </div>
      )}
    </li>
  )
}

function StaffForm({
  title,
  initialName = '',
  initialPhone = '',
  onSubmit,
  onCancel,
}: {
  title: string
  initialName?: string
  initialPhone?: string
  onSubmit: (name: string, celular: string) => Promise<void>
  onCancel: () => void
}) {
  const [name, setName] = useState(initialName)
  const [phone, setPhone] = useState(initialPhone)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!isValidPhone(phone)) {
      setError('Informe um celular válido com DDD, ex.: (31) 99999-9999.')
      return
    }
    setBusy(true)
    setError(null)
    try {
      await onSubmit(name.trim(), normalizePhone(phone))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Erro inesperado ao salvar.')
      setBusy(false)
    }
  }

  return (
    <form
      onSubmit={submit}
      className="animate-rise rounded-card border border-brand/30 bg-brand-soft/40 p-4"
    >
      <p className="font-semibold">{title}</p>
      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <Field label="Nome">
          <Input required autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Fulano de Tal" />
        </Field>
        <Field label="Celular (WhatsApp)">
          <Input
            required
            inputMode="tel"
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            placeholder="(31) 99999-9999"
          />
        </Field>
      </div>
      {error && <p className="mt-2 text-sm font-medium text-critico">{error}</p>}
      <div className="mt-4 flex gap-2">
        <Button type="submit" busy={busy}>
          Salvar
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
