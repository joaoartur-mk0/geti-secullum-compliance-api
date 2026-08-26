import { DatabaseZap, Fingerprint, RefreshCw, Search, Wifi, WifiOff } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, EmptyState, ErrorNote, Input, Skeleton, useToast } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import type { Equipment } from '../lib/types'

type Loadable<T> =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; data: T }

// Equipamentos são somente leitura no painel: o cadastro (criar/editar/remover) vive na
// Secullum. Chega aqui pela rotina diária às 03:00 (ver usecase.SchedulerService) ou sob
// demanda pelo botão "Ressincronizar" (POST /tenants/:id/sync) — a mesma fila que
// sincroniza colaboradores.
export default function Equipamentos() {
  const { tenant } = useTenant()
  const toast = useToast()
  const [state, setState] = useState<Loadable<Equipment[]>>({ phase: 'loading' })
  const [query, setQuery] = useState('')
  const [resyncing, setResyncing] = useState(false)

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      const { equipamentos } = await api.listEquipamentos(tenant.id)
      setState({ phase: 'ready', data: equipamentos })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar equipamentos.',
      })
    }
  }, [tenant.id])

  useEffect(() => {
    void load()
  }, [load])

  // Ressincroniza com a Secullum (equipamentos E colaboradores, mesma fila) sob demanda —
  // ex.: depois de instalar/trocar um relógio de ponto lá, sem esperar a rotina diária das
  // 03:00. É assíncrono (fila tenant.provisioning): o resultado só aparece ao atualizar a
  // lista depois de alguns instantes.
  async function resync() {
    setResyncing(true)
    try {
      await api.syncTenant(tenant.id)
      toast('success', 'Sincronização enfileirada. Atualize a lista em instantes.')
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao ressincronizar.')
    } finally {
      setResyncing(false)
    }
  }

  const rows = useMemo(() => {
    if (state.phase !== 'ready') return []
    const q = query.trim().toLowerCase()
    return state.data
      .filter((e) => !q || e.descricao.toLowerCase().includes(q) || String(e.secullum_id).includes(q))
      .sort((a, b) => a.descricao.localeCompare(b.descricao, 'pt-BR'))
  }, [state, query])

  const total = state.phase === 'ready' ? state.data.length : 0

  return (
    <div className="animate-rise">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Equipamentos</h1>
          <p className="mt-1 text-sm text-ink-soft">
            {state.phase === 'ready'
              ? `${total} sincronizado${total === 1 ? '' : 's'} da Secullum`
              : 'Relógios de ponto cadastrados na Secullum, sincronizados diariamente.'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={load}
            className="flex min-h-11 items-center gap-1.5 rounded-field px-2.5 text-sm font-medium text-ink-soft transition-colors duration-150 hover:bg-panel hover:text-ink"
          >
            <RefreshCw size={15} aria-hidden />
            Atualizar
          </button>
          <Button variant="secondary" busy={resyncing} onClick={resync}>
            <DatabaseZap size={16} aria-hidden />
            Ressincronizar
          </Button>
        </div>
      </header>

      {state.phase === 'loading' && (
        <div className="mt-8 flex flex-col gap-2">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-14 w-full" />
          ))}
        </div>
      )}

      {state.phase === 'error' && (
        <div className="mt-8">
          <ErrorNote message={state.message} onRetry={load} />
        </div>
      )}

      {state.phase === 'ready' && state.data.length === 0 && (
        <div className="mt-8">
          <EmptyState
            icon={<Fingerprint size={32} strokeWidth={1.5} />}
            title="Nenhum equipamento sincronizado"
            description="A sincronização diária com a Secullum ainda não rodou, ou nenhum relógio de ponto está cadastrado lá para esta empresa."
          />
        </div>
      )}

      {state.phase === 'ready' && state.data.length > 0 && (
        <>
          <div className="mt-6 relative min-w-56 max-w-md">
            <Search
              size={16}
              aria-hidden
              className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-faint"
            />
            <Input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Buscar por descrição ou ID Secullum"
              aria-label="Buscar equipamento"
              className="w-full pl-9"
            />
          </div>

          {rows.length === 0 ? (
            <p className="mt-8 text-center text-sm text-ink-soft">
              Nenhum equipamento corresponde à busca.
            </p>
          ) : (
            <ul className="mt-4 flex flex-col gap-2">
              {rows.map((equipment) => (
                <EquipmentRow key={equipment.id} equipment={equipment} />
              ))}
            </ul>
          )}
        </>
      )}
    </div>
  )
}

function EquipmentRow({ equipment }: { equipment: Equipment }) {
  return (
    <li className="flex items-center gap-3 rounded-card border border-line bg-bg px-4 py-3 shadow-card">
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-field bg-panel text-ink-soft">
        <Fingerprint size={17} aria-hidden />
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate font-medium text-ink">{equipment.descricao || `Equipamento ${equipment.secullum_id}`}</p>
        <p className="text-xs text-ink-faint">ID Secullum {equipment.secullum_id}</p>
      </div>

      {equipment.endereco_ip ? (
        <span className="inline-flex items-center gap-1.5 rounded-full bg-ok-bg px-2.5 py-1 text-xs font-semibold text-ok">
          <Wifi size={13} aria-hidden />
          {equipment.endereco_ip}
        </span>
      ) : (
        <span className="inline-flex items-center gap-1.5 rounded-full bg-panel px-2.5 py-1 text-xs font-semibold text-ink-faint">
          <WifiOff size={13} aria-hidden />
          Sem IP
        </span>
      )}
    </li>
  )
}
