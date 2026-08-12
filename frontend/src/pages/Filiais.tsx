import { Building2, Cpu, Hash, Pencil, Plus, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { Button, EmptyState, ErrorNote, Field, Input, Skeleton, useToast } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { formatPhone, isValidPhone, normalizePhone } from '../lib/format'
import type { Branch } from '../lib/types'

type ListState =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; branches: Branch[] }

export default function Filiais() {
  const { tenant } = useTenant()
  const toast = useToast()
  const [state, setState] = useState<ListState>({ phase: 'loading' })
  const [adding, setAdding] = useState(false)

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      setState({ phase: 'ready', branches: await api.listBranches(tenant.id) })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar filiais.',
      })
    }
  }, [tenant.id])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="animate-rise">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Filiais</h1>
          <p className="mt-1 max-w-prose text-sm text-ink-soft">
            Unidades da empresa, seus aparelhos de ponto e o gestor responsável. É o que liga
            uma ocorrência a quem deve resolvê-la.
          </p>
        </div>
        {!adding && state.phase === 'ready' && state.branches.length > 0 && (
          <Button onClick={() => setAdding(true)}>
            <Plus size={17} aria-hidden />
            Nova filial
          </Button>
        )}
      </header>

      <div className="mt-8">
        {adding && (
          <div className="mb-4">
            <BranchForm
              title="Nova filial"
              onCancel={() => setAdding(false)}
              onSubmit={async (body) => {
                await api.createBranch(tenant.id, body)
                toast('success', `${body.name} cadastrada.`)
                setAdding(false)
                void load()
              }}
            />
          </div>
        )}

        {state.phase === 'loading' && (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-28 w-full" />
            <Skeleton className="h-28 w-full" />
          </div>
        )}

        {state.phase === 'error' && <ErrorNote message={state.message} onRetry={load} />}

        {state.phase === 'ready' && state.branches.length === 0 && !adding && (
          <EmptyState
            icon={<Building2 size={32} strokeWidth={1.5} />}
            title="Nenhuma filial cadastrada"
            description="Cadastre as unidades da empresa para vincular aparelhos de ponto e nº de folha — é como o painel resolve de onde veio cada ocorrência."
            action={
              <Button onClick={() => setAdding(true)}>
                <Plus size={17} aria-hidden />
                Nova filial
              </Button>
            }
          />
        )}

        {state.phase === 'ready' && state.branches.length > 0 && (
          <ul className="flex flex-col gap-3">
            {state.branches.map((branch) => (
              <BranchCard
                key={branch.id}
                branch={branch}
                onChanged={() => {
                  toast('success', 'Filial atualizada.')
                  void load()
                }}
                onDeleted={() => {
                  toast('success', `${branch.name} excluída.`)
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

function BranchCard({
  branch,
  onChanged,
  onDeleted,
}: {
  branch: Branch
  onChanged: () => void
  onDeleted: () => void
}) {
  const toast = useToast()
  const [mode, setMode] = useState<'view' | 'edit' | 'confirm-delete'>('view')
  const [expanded, setExpanded] = useState(false)

  async function remove() {
    try {
      await api.deleteBranch(branch.id)
      onDeleted()
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao excluir a filial.')
    }
  }

  if (mode === 'edit') {
    return (
      <li>
        <BranchForm
          title={`Editar ${branch.name}`}
          initialName={branch.name}
          initialManagerName={branch.manager_name}
          initialManagerPhone={branch.manager_phone ? formatPhone(branch.manager_phone) : ''}
          onCancel={() => setMode('view')}
          onSubmit={async (body) => {
            await api.updateBranch(branch.id, body)
            setMode('view')
            onChanged()
          }}
        />
      </li>
    )
  }

  return (
    <li className="rounded-card border border-line bg-bg shadow-card">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3.5">
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-brand-soft text-brand">
          <Building2 size={18} aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate font-semibold">{branch.name}</p>
          <p className="text-sm text-ink-soft">
            {branch.manager_name ? (
              <>
                {branch.manager_name}
                {branch.manager_phone && ` · ${formatPhone(branch.manager_phone)}`}
              </>
            ) : (
              'Sem gestor cadastrado'
            )}
          </p>
        </div>

        {mode === 'confirm-delete' ? (
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-critico">Excluir?</span>
            <Button variant="danger" onClick={remove}>
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
              onClick={() => setExpanded((e) => !e)}
              className="rounded-field px-2.5 py-2 text-sm font-medium text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink"
            >
              {branch.devices.length} aparelho{branch.devices.length === 1 ? '' : 's'} ·{' '}
              {branch.payroll_numbers.length} nº folha
            </button>
            <button
              type="button"
              onClick={() => setMode('edit')}
              aria-label={`Editar ${branch.name}`}
              className="rounded-field p-2.5 text-ink-faint transition-colors duration-150 hover:bg-panel hover:text-ink"
            >
              <Pencil size={17} />
            </button>
            <button
              type="button"
              onClick={() => setMode('confirm-delete')}
              aria-label={`Excluir ${branch.name}`}
              className="rounded-field p-2.5 text-ink-faint transition-colors duration-150 hover:bg-critico-bg hover:text-critico"
            >
              <Trash2 size={17} />
            </button>
          </div>
        )}
      </div>

      {expanded && mode === 'view' && (
        <div className="border-t border-line px-4 py-3.5">
          <BranchLinks branch={branch} onChanged={onChanged} />
        </div>
      )}
    </li>
  )
}

// ---------- Aparelhos e nº de folha vinculados ----------

function BranchLinks({ branch, onChanged }: { branch: Branch; onChanged: () => void }) {
  const toast = useToast()
  const [equipId, setEquipId] = useState('')
  const [label, setLabel] = useState('')
  const [numero, setNumero] = useState('')
  const [busyDevice, setBusyDevice] = useState(false)
  const [busyPayroll, setBusyPayroll] = useState(false)

  async function addDevice(event: React.FormEvent) {
    event.preventDefault()
    setBusyDevice(true)
    try {
      await api.addBranchDevice(branch.id, { secullum_equip_id: Number(equipId), label })
      setEquipId('')
      setLabel('')
      onChanged()
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao vincular o aparelho.')
    } finally {
      setBusyDevice(false)
    }
  }

  async function removeDevice(deviceId: number) {
    try {
      await api.removeBranchDevice(branch.id, deviceId)
      onChanged()
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao desvincular o aparelho.')
    }
  }

  async function addPayroll(event: React.FormEvent) {
    event.preventDefault()
    setBusyPayroll(true)
    try {
      await api.addBranchPayrollNumber(branch.id, { numero })
      setNumero('')
      onChanged()
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao vincular o nº de folha.')
    } finally {
      setBusyPayroll(false)
    }
  }

  async function removePayroll(pnId: number) {
    try {
      await api.removeBranchPayrollNumber(branch.id, pnId)
      onChanged()
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao desvincular o nº de folha.')
    }
  }

  return (
    <div className="grid gap-5 sm:grid-cols-2">
      <div>
        <p className="mb-2 flex items-center gap-1.5 text-sm font-semibold text-ink-soft">
          <Cpu size={15} aria-hidden />
          Aparelhos de ponto
        </p>
        <p className="mb-2 text-xs text-ink-faint">
          Resolve a filial pela batida do dia (EquipId). Prioridade sobre o nº de folha.
        </p>
        {branch.devices.length > 0 && (
          <ul className="mb-2 flex flex-col gap-1.5">
            {branch.devices.map((d) => (
              <li
                key={d.id}
                className="flex items-center justify-between gap-2 rounded-field border border-line bg-panel px-3 py-2 text-sm"
              >
                <span className="min-w-0 truncate">
                  #{d.secullum_equip_id}
                  {d.label && ` — ${d.label}`}
                </span>
                <button
                  type="button"
                  onClick={() => removeDevice(d.id)}
                  aria-label="Desvincular aparelho"
                  className="shrink-0 rounded-field p-1 text-ink-faint transition-colors duration-150 hover:bg-critico-bg hover:text-critico"
                >
                  <Trash2 size={14} />
                </button>
              </li>
            ))}
          </ul>
        )}
        <form onSubmit={addDevice} className="flex flex-wrap gap-2">
          <Input
            required
            inputMode="numeric"
            pattern="[0-9]+"
            value={equipId}
            onChange={(e) => setEquipId(e.target.value)}
            placeholder="EquipId (ex.: 4)"
            className="w-32"
          />
          <Input
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="Rótulo (opcional)"
            className="min-w-32 flex-1"
          />
          <Button type="submit" variant="secondary" busy={busyDevice}>
            <Plus size={15} aria-hidden />
            Vincular
          </Button>
        </form>
      </div>

      <div>
        <p className="mb-2 flex items-center gap-1.5 text-sm font-semibold text-ink-soft">
          <Hash size={15} aria-hidden />
          Nº de folha lotados
        </p>
        <p className="mb-2 text-xs text-ink-faint">
          Usado quando a batida não identifica o aparelho (ex.: marcação pelo app/web).
        </p>
        {branch.payroll_numbers.length > 0 && (
          <ul className="mb-2 flex flex-col gap-1.5">
            {branch.payroll_numbers.map((p) => (
              <li
                key={p.id}
                className="flex items-center justify-between gap-2 rounded-field border border-line bg-panel px-3 py-2 text-sm"
              >
                <span className="min-w-0 truncate">nº {p.numero}</span>
                <button
                  type="button"
                  onClick={() => removePayroll(p.id)}
                  aria-label="Desvincular nº de folha"
                  className="shrink-0 rounded-field p-1 text-ink-faint transition-colors duration-150 hover:bg-critico-bg hover:text-critico"
                >
                  <Trash2 size={14} />
                </button>
              </li>
            ))}
          </ul>
        )}
        <form onSubmit={addPayroll} className="flex flex-wrap gap-2">
          <Input
            required
            value={numero}
            onChange={(e) => setNumero(e.target.value)}
            placeholder="Nº de folha"
            className="min-w-32 flex-1"
          />
          <Button type="submit" variant="secondary" busy={busyPayroll}>
            <Plus size={15} aria-hidden />
            Vincular
          </Button>
        </form>
      </div>
    </div>
  )
}

// ---------- Formulário de filial ----------

function BranchForm({
  title,
  initialName = '',
  initialManagerName = '',
  initialManagerPhone = '',
  onSubmit,
  onCancel,
}: {
  title: string
  initialName?: string
  initialManagerName?: string
  initialManagerPhone?: string
  onSubmit: (body: { name: string; manager_name: string; manager_phone: string }) => Promise<void>
  onCancel: () => void
}) {
  const [name, setName] = useState(initialName)
  const [managerName, setManagerName] = useState(initialManagerName)
  const [managerPhone, setManagerPhone] = useState(initialManagerPhone)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (managerPhone && !isValidPhone(managerPhone)) {
      setError('Informe um celular válido com DDD, ex.: (31) 99999-9999.')
      return
    }
    setBusy(true)
    setError(null)
    try {
      await onSubmit({
        name: name.trim(),
        manager_name: managerName.trim(),
        manager_phone: managerPhone ? normalizePhone(managerPhone) : '',
      })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Erro inesperado ao salvar.')
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="animate-rise rounded-card border border-brand/30 bg-brand-soft/40 p-4">
      <p className="font-semibold">{title}</p>
      <div className="mt-3 grid gap-3 sm:grid-cols-3">
        <Field label="Nome da filial">
          <Input
            required
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Filial Centro"
          />
        </Field>
        <Field label="Gestor responsável">
          <Input
            value={managerName}
            onChange={(e) => setManagerName(e.target.value)}
            placeholder="Fulano de Tal"
          />
        </Field>
        <Field label="Celular do gestor">
          <Input
            inputMode="tel"
            value={managerPhone}
            onChange={(e) => setManagerPhone(e.target.value)}
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
