// Painel de advertências de um colaborador: cadastro (nasce em rascunho) e controle de
// status draft -> enviada -> assinada. Vive na tela de colaborador (ColaboradorHistorico),
// mas é um componente à parte por não ser uma rota.

import { CheckCircle2, FileWarning, Plus, Send } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { Button, ErrorNote, Skeleton, WarningStatusBadge, useToast } from './ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { formatDateTime } from '../lib/format'
import type { Warning, WarningCounts, WarningStatus } from '../lib/types'

type ListState =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready'; warnings: Warning[]; counts: WarningCounts }

// nextStatus dá o próximo passo do fluxo de mão única draft -> enviada -> assinada.
// null quando já está no fim (assinada) — nada mais a avançar.
function nextStatus(status: WarningStatus): WarningStatus | null {
  if (status === 'draft') return 'enviada'
  if (status === 'enviada') return 'assinada'
  return null
}

export default function WarningPanel({
  collaboratorId,
  collaboratorName,
  branchId,
}: {
  collaboratorId: number
  collaboratorName: string
  branchId: number | null
}) {
  const { tenant } = useTenant()
  const toast = useToast()
  const [state, setState] = useState<ListState>({ phase: 'loading' })
  const [adding, setAdding] = useState(false)

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      const { warnings, counts } = await api.listWarnings(tenant.id, { collaborator_id: collaboratorId })
      setState({ phase: 'ready', warnings, counts })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar advertências.',
      })
    }
  }, [tenant.id, collaboratorId])

  useEffect(() => {
    void load()
  }, [load])

  async function advance(warning: Warning) {
    const next = nextStatus(warning.status)
    if (!next) return
    try {
      await api.updateWarningStatus(warning.id, next)
      toast('success', next === 'enviada' ? 'Advertência marcada como enviada.' : 'Advertência marcada como assinada.')
      void load()
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao atualizar o status.')
    }
  }

  return (
    <div>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h2 className="flex items-center gap-1.5 text-sm font-semibold text-ink-soft">
          <FileWarning size={15} aria-hidden />
          Advertências
        </h2>
        {!adding && (
          <Button variant="secondary" onClick={() => setAdding(true)}>
            <Plus size={15} aria-hidden />
            Nova advertência
          </Button>
        )}
      </div>

      {state.phase === 'ready' && state.warnings.length > 0 && (
        <p className="mb-3 text-xs text-ink-soft">
          {state.counts.draft > 0 && `${state.counts.draft} em rascunho`}
          {state.counts.draft > 0 && (state.counts.enviada > 0 || state.counts.assinada > 0) && ' · '}
          {state.counts.enviada > 0 && `${state.counts.enviada} enviada${state.counts.enviada === 1 ? '' : 's'}`}
          {state.counts.enviada > 0 && state.counts.assinada > 0 && ' · '}
          {state.counts.assinada > 0 && `${state.counts.assinada} assinada${state.counts.assinada === 1 ? '' : 's'}`}
        </p>
      )}

      {adding && (
        <div className="mb-4">
          <WarningForm
            onCancel={() => setAdding(false)}
            onSubmit={async (body) => {
              await api.createWarning(tenant.id, {
                collaborator_id: collaboratorId,
                branch_id: branchId,
                body,
              })
              toast('success', `Advertência registrada para ${collaboratorName}.`)
              setAdding(false)
              void load()
            }}
          />
        </div>
      )}

      {state.phase === 'loading' && (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-16 w-full" />
        </div>
      )}

      {state.phase === 'error' && <ErrorNote message={state.message} onRetry={load} />}

      {state.phase === 'ready' && state.warnings.length === 0 && !adding && (
        <p className="rounded-card border border-dashed border-line px-4 py-6 text-center text-sm text-ink-soft">
          Nenhuma advertência registrada para este colaborador.
        </p>
      )}

      {state.phase === 'ready' && state.warnings.length > 0 && (
        <ul className="flex flex-col gap-2">
          {state.warnings.map((warning) => (
            <li key={warning.id} className="rounded-card border border-line bg-bg px-4 py-3 shadow-card">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div className="min-w-0 flex-1">
                  <p className="whitespace-pre-wrap text-sm text-ink">{warning.body || '(sem texto)'}</p>
                  <p className="mt-1 text-xs text-ink-faint">
                    criada {formatDateTime(warning.created_at)}
                    {warning.sent_at && ` · enviada ${formatDateTime(warning.sent_at)}`}
                    {warning.signed_at && ` · assinada ${formatDateTime(warning.signed_at)}`}
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <WarningStatusBadge status={warning.status} />
                  {nextStatus(warning.status) && (
                    <Button variant="secondary" onClick={() => advance(warning)}>
                      {warning.status === 'draft' ? (
                        <>
                          <Send size={14} aria-hidden />
                          Marcar enviada
                        </>
                      ) : (
                        <>
                          <CheckCircle2 size={14} aria-hidden />
                          Marcar assinada
                        </>
                      )}
                    </Button>
                  )}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function WarningForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (body: string) => Promise<void>
  onCancel: () => void
}) {
  const [body, setBody] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await onSubmit(body.trim())
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Erro inesperado ao salvar.')
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="animate-rise rounded-card border border-brand/30 bg-brand-soft/40 p-4">
      <p className="font-semibold">Nova advertência</p>
      <p className="mt-1 text-xs text-ink-soft">Nasce em rascunho — marque como enviada quando entregar ao colaborador.</p>
      <textarea
        required
        autoFocus
        value={body}
        onChange={(e) => setBody(e.target.value)}
        placeholder="Descreva o motivo da advertência…"
        rows={4}
        className="mt-3 w-full rounded-field border border-line bg-bg p-3 text-sm text-ink placeholder:text-ink-faint transition-colors duration-150 hover:border-ink-faint focus:border-brand"
      />
      {error && <p className="mt-2 text-sm font-medium text-critico">{error}</p>}
      <div className="mt-3 flex gap-2">
        <Button type="submit" busy={busy}>
          Salvar rascunho
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancelar
        </Button>
      </div>
    </form>
  )
}
