import { Building2, Cpu, Hash, Pencil, Plus, RefreshCw, Search, Trash2, Users } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, EmptyState, ErrorNote, Field, Input, Skeleton, useToast } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { formatPhone, isValidPhone, normalizePhone } from '../lib/format'
import {
  branchMembers,
  buildIndex,
  clearCachedIndex,
  findLink,
  readCachedIndex,
  setFilialEmLote,
  type CollaboratorEntry,
  type LotacaoIndex,
} from '../lib/lotacao'
import type { Branch } from '../lib/types'

/** Progresso da varredura de prefills, ou null quando não há varredura em curso. */
type Indexing = { done: number; total: number } | null

type ListState =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; branches: Branch[] }

export default function Filiais() {
  const { tenant } = useTenant()
  const toast = useToast()
  const [state, setState] = useState<ListState>({ phase: 'loading' })
  const [adding, setAdding] = useState(false)

  // Índice de lotação: traduz os nº de folha guardados na filial para nomes de gente.
  // Custa uma varredura de prefills (~13s para 481), então é carregado só quando alguém
  // abre uma filial — quem entrou aqui só para corrigir um telefone não paga nada.
  const [index, setIndex] = useState<LotacaoIndex | null>(null)
  const [indexing, setIndexing] = useState<Indexing>(null)

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

  // Trocar de empresa invalida o índice: os nº de folha são de outro tenant.
  useEffect(() => {
    setIndex(readCachedIndex(tenant.id))
  }, [tenant.id])

  const ensureIndexLoaded = useCallback(
    async (force = false) => {
      if (index && !force) return index
      if (force) clearCachedIndex(tenant.id)
      setIndexing({ done: 0, total: 0 })
      try {
        const built = await buildIndex(tenant.id, (done, total) => setIndexing({ done, total }))
        setIndex(built)
        return built
      } catch {
        toast('error', 'Falha ao carregar a lista de colaboradores.')
        return null
      } finally {
        setIndexing(null)
      }
    },
    [tenant.id, index, toast],
  )

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
                allBranches={state.branches}
                index={index}
                indexing={indexing}
                ensureIndexLoaded={ensureIndexLoaded}
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
  allBranches,
  index,
  indexing,
  ensureIndexLoaded,
  onChanged,
  onDeleted,
}: {
  branch: Branch
  allBranches: Branch[]
  index: LotacaoIndex | null
  indexing: Indexing
  ensureIndexLoaded: (force?: boolean) => Promise<LotacaoIndex | null>
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
              onClick={() => {
                const next = !expanded
                setExpanded(next)
                // O índice só é necessário com a filial aberta — é ali que os nº de folha
                // precisam virar nome.
                if (next) void ensureIndexLoaded()
              }}
              className="rounded-field px-2.5 py-2 text-sm font-medium text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink"
            >
              {branch.payroll_numbers.length} colaborador
              {branch.payroll_numbers.length === 1 ? '' : 'es'} · {branch.devices.length} aparelho
              {branch.devices.length === 1 ? '' : 's'}
            </button>
            {/* min-h-11 min-w-11: alvo de toque de 37px (p-2.5 + ícone) ficava abaixo dos
                44px recomendados para mobile. */}
            <button
              type="button"
              onClick={() => setMode('edit')}
              aria-label={`Editar ${branch.name}`}
              className="flex min-h-11 min-w-11 items-center justify-center rounded-field text-ink-faint transition-colors duration-150 hover:bg-panel hover:text-ink"
            >
              <Pencil size={17} />
            </button>
            <button
              type="button"
              onClick={() => setMode('confirm-delete')}
              aria-label={`Excluir ${branch.name}`}
              className="flex min-h-11 min-w-11 items-center justify-center rounded-field text-ink-faint transition-colors duration-150 hover:bg-critico-bg hover:text-critico"
            >
              <Trash2 size={17} />
            </button>
          </div>
        )}
      </div>

      {expanded && mode === 'view' && (
        <div className="border-t border-line px-4 py-3.5">
          <BranchLinks
            branch={branch}
            allBranches={allBranches}
            index={index}
            indexing={indexing}
            ensureIndexLoaded={ensureIndexLoaded}
            onChanged={onChanged}
          />
        </div>
      )}
    </li>
  )
}

// ---------- Aparelhos e nº de folha vinculados ----------

function BranchLinks({
  branch,
  allBranches,
  index,
  indexing,
  ensureIndexLoaded,
  onChanged,
}: {
  branch: Branch
  allBranches: Branch[]
  index: LotacaoIndex | null
  indexing: Indexing
  ensureIndexLoaded: (force?: boolean) => Promise<LotacaoIndex | null>
  onChanged: () => void
}) {
  const toast = useToast()
  const [equipId, setEquipId] = useState('')
  const [label, setLabel] = useState('')
  const [numero, setNumero] = useState('')
  const [busyDevice, setBusyDevice] = useState(false)
  const [busyPayroll, setBusyPayroll] = useState(false)
  const [picking, setPicking] = useState(false)
  const [showRawForm, setShowRawForm] = useState(false)

  const members = useMemo(() => branchMembers(branch, index), [branch, index])

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
          <Users size={15} aria-hidden />
          Colaboradores lotados
        </p>
        <p className="mb-2 text-xs text-ink-faint">
          Resolve a filial quando a batida não identifica o aparelho (ex.: marcação pelo
          app/web). O vínculo é gravado pelo nº de folha do colaborador.
        </p>

        {indexing && !index && (
          <p className="mb-2 text-xs text-ink-faint" role="status">
            Carregando colaboradores…{' '}
            {indexing.total > 0 && `${indexing.done} de ${indexing.total}`}
          </p>
        )}

        {members.length > 0 && (
          <ul className="mb-2 flex flex-col gap-1.5">
            {members.map((m) => (
              <li
                key={m.payrollNumberId}
                className="flex items-center justify-between gap-2 rounded-field border border-line bg-panel px-3 py-2 text-sm"
              >
                <span className="min-w-0">
                  {m.collaborator ? (
                    <>
                      <span className="block truncate">{m.collaborator.name}</span>
                      <span className="text-xs text-ink-faint">folha {m.numeroFolha}</span>
                    </>
                  ) : (
                    // Número que não bate com ninguém — foi o que aconteceu ao digitar o
                    // ID da Secullum no lugar da folha. Fica visível para ser removido.
                    <>
                      <span className="block truncate">folha {m.numeroFolha}</span>
                      <span className="text-xs text-critico">
                        {index ? 'não corresponde a nenhum colaborador' : 'carregando…'}
                      </span>
                    </>
                  )}
                </span>
                <button
                  type="button"
                  onClick={() => removePayroll(m.payrollNumberId)}
                  aria-label={`Desvincular ${m.collaborator?.name ?? `folha ${m.numeroFolha}`}`}
                  className="shrink-0 rounded-field p-1 text-ink-faint transition-colors duration-150 hover:bg-critico-bg hover:text-critico"
                >
                  <Trash2 size={14} />
                </button>
              </li>
            ))}
          </ul>
        )}

        {members.length === 0 && !indexing && (
          <p className="mb-2 text-sm text-ink-faint">Nenhum colaborador lotado nesta filial.</p>
        )}

        {picking ? (
          <CollaboratorPicker
            branch={branch}
            allBranches={allBranches}
            index={index}
            onCancel={() => setPicking(false)}
            onDone={() => {
              setPicking(false)
              onChanged()
            }}
          />
        ) : (
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant="secondary"
              busy={!!indexing}
              onClick={async () => {
                if (await ensureIndexLoaded()) setPicking(true)
              }}
            >
              <Plus size={15} aria-hidden />
              Adicionar colaboradores
            </Button>
            <button
              type="button"
              onClick={() => void ensureIndexLoaded(true)}
              className="rounded-field px-2 py-1.5 text-xs font-medium text-ink-faint transition-colors duration-150 hover:bg-panel hover:text-ink"
              title="Recarrega os nº de folha da Secullum"
            >
              <RefreshCw size={13} aria-hidden className="mr-1 inline" />
              Atualizar lista
            </button>
          </div>
        )}

        {/* O vínculo por número avulso continua existindo para o caso de alguém já ter a
            lista de folhas do RH em mãos. Fica recolhido porque é justamente o caminho
            que induziu a digitar o número errado. */}
        <details
          open={showRawForm}
          onToggle={(e) => setShowRawForm((e.target as HTMLDetailsElement).open)}
          className="mt-3"
        >
          <summary className="cursor-pointer text-xs font-medium text-ink-faint hover:text-ink">
            <Hash size={12} aria-hidden className="mr-1 inline" />
            Vincular por nº de folha (avançado)
          </summary>
          <p className="mt-2 text-xs text-ink-faint">
            Use o nº de folha da Secullum, não o ID que aparece na ficha do colaborador —
            são números diferentes.
          </p>
          <form onSubmit={addPayroll} className="mt-2 flex flex-wrap gap-2">
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
        </details>
      </div>
    </div>
  )
}

// ---------- Seleção de colaboradores em massa ----------

// Lotar 481 pessoas uma a uma pela ficha seria inviável; aqui a filial puxa quem quiser,
// buscando por nome. Quem já está em outra filial aparece com a lotação atual, porque
// marcar essa pessoa é uma MUDANÇA de filial, não uma adição — e isso precisa estar à
// vista antes de confirmar.
function CollaboratorPicker({
  branch,
  allBranches,
  index,
  onCancel,
  onDone,
}: {
  branch: Branch
  allBranches: Branch[]
  index: LotacaoIndex | null
  onCancel: () => void
  onDone: () => void
}) {
  const toast = useToast()
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [progress, setProgress] = useState<Indexing>(null)

  const rows = useMemo(() => {
    if (!index) return []
    const q = query.trim().toLowerCase()
    return index.collaborators
      .map((entry) => ({ entry, link: findLink(allBranches, entry.numeroFolha) }))
      .filter(({ entry }) => {
        if (entry.numeroFolha && findLink([branch], entry.numeroFolha)) return false // já está aqui
        if (!q) return true
        return entry.name.toLowerCase().includes(q) || entry.numeroFolha.includes(q)
      })
      .sort((a, b) => a.entry.name.localeCompare(b.entry.name, 'pt-BR'))
  }, [index, query, allBranches, branch])

  function toggle(secullumId: number) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(secullumId)) next.delete(secullumId)
      else next.add(secullumId)
      return next
    })
  }

  async function submit() {
    if (!index || selected.size === 0) return
    const entries = [...selected]
      .map((id) => index.bySecullumId.get(id))
      .filter((e): e is CollaboratorEntry => !!e)

    setProgress({ done: 0, total: entries.length })
    const result = await setFilialEmLote(entries, branch.id, allBranches, (done, total) =>
      setProgress({ done, total }),
    )
    setProgress(null)

    if (result.failures.length === 0) {
      toast('success', `${result.ok} colaborador${result.ok === 1 ? '' : 'es'} lotado${result.ok === 1 ? '' : 's'} em ${branch.name}.`)
    } else {
      toast(
        'error',
        `${result.ok} vinculado(s), ${result.failures.length} com falha: ${result.failures
          .slice(0, 3)
          .map((f) => f.name)
          .join(', ')}${result.failures.length > 3 ? '…' : ''}`,
      )
    }
    onDone()
  }

  return (
    <div className="rounded-card border border-brand/30 bg-brand-soft/40 p-3">
      <div className="relative mb-2">
        <Search
          size={15}
          aria-hidden
          className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-faint"
        />
        <Input
          autoFocus
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Buscar por nome ou nº de folha"
          aria-label="Buscar colaborador"
          className="w-full pl-9"
        />
      </div>

      {rows.length === 0 ? (
        <p className="py-3 text-center text-sm text-ink-soft">
          {index ? 'Nenhum colaborador corresponde à busca.' : 'Carregando colaboradores…'}
        </p>
      ) : (
        <ul className="max-h-64 overflow-y-auto rounded-field border border-line bg-bg">
          {rows.map(({ entry, link }) => (
            <li key={entry.secullumId} className="border-b border-line last:border-b-0">
              <label className="flex cursor-pointer items-center gap-2.5 px-3 py-2 text-sm hover:bg-panel">
                <input
                  type="checkbox"
                  checked={selected.has(entry.secullumId)}
                  onChange={() => toggle(entry.secullumId)}
                  disabled={!entry.numeroFolha}
                  className="h-4 w-4 shrink-0 accent-brand"
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate">{entry.name}</span>
                  <span className="text-xs text-ink-faint">
                    {entry.numeroFolha ? `folha ${entry.numeroFolha}` : 'sem nº de folha'}
                    {link && ` · hoje em ${link.branchName}`}
                  </span>
                </span>
              </label>
            </li>
          ))}
        </ul>
      )}

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Button type="button" busy={!!progress} disabled={selected.size === 0} onClick={submit}>
          {selected.size === 0
            ? 'Selecione colaboradores'
            : `Lotar ${selected.size} em ${branch.name}`}
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancelar
        </Button>
        {progress && (
          <span className="text-xs text-ink-faint" role="status">
            {progress.done} de {progress.total}
          </span>
        )}
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
