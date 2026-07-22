import { Clock, Plus, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Button, ErrorNote, Select, Skeleton, Toggle, useToast } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import type { Settings, Severity } from '../lib/types'

interface RuleMeta {
  flag: 'almoco' | 'interjornada' | 'esquecimento' | 'hextras'
  severityField: 'almoco_severity' | 'interjornada_severity' | 'esquecimento_severity' | null
  title: string
  description: string
}

const rules: RuleMeta[] = [
  {
    flag: 'interjornada',
    severityField: 'interjornada_severity',
    title: 'Interjornada curta',
    description: 'Descanso menor que 11 horas entre o fim de uma jornada e o início da seguinte (Art. 66 da CLT).',
  },
  {
    flag: 'almoco',
    severityField: 'almoco_severity',
    title: 'Intervalo de almoço reduzido',
    description: 'Intervalo de repouso abaixo de 1 hora em jornadas maiores que 6 horas (Art. 71 da CLT).',
  },
  {
    flag: 'esquecimento',
    severityField: 'esquecimento_severity',
    title: 'Batida esquecida',
    description: 'Colaborador sem uma marcação esperada — por exemplo, só a entrada registrada bem depois do horário de almoço.',
  },
  {
    flag: 'hextras',
    severityField: null,
    title: 'Hora extra excedente',
    description: 'Mais de 1 hora extra no dia gera alerta; acima de 2 horas, crítico. Limiares fixados pelo Art. 59 da CLT.',
  },
]

type LoadState =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'ready' }

export default function Avisos() {
  const { tenant } = useTenant()
  const toast = useToast()
  const [state, setState] = useState<LoadState>({ phase: 'loading' })
  const [saved, setSaved] = useState<Settings | null>(null)
  const [draft, setDraft] = useState<Settings | null>(null)
  const [newTime, setNewTime] = useState('')
  const [saving, setSaving] = useState(false)

  const load = useCallback(async () => {
    setState({ phase: 'loading' })
    try {
      const settings = await api.getSettings(tenant.id)
      settings.horarios = [...(settings.horarios ?? [])].sort()
      setSaved(settings)
      setDraft(settings)
      setState({ phase: 'ready' })
    } catch (error) {
      setState({
        phase: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao carregar as configurações.',
      })
    }
  }, [tenant.id])

  useEffect(() => {
    void load()
  }, [load])

  const dirty = useMemo(() => JSON.stringify(saved) !== JSON.stringify(draft), [saved, draft])

  function patch(partial: Partial<Settings>) {
    setDraft((current) => (current ? { ...current, ...partial } : current))
  }

  function addTime() {
    if (!draft || !/^\d{2}:\d{2}$/.test(newTime)) return
    if (draft.horarios.includes(newTime)) {
      setNewTime('')
      return
    }
    patch({ horarios: [...draft.horarios, newTime].sort() })
    setNewTime('')
  }

  async function save() {
    if (!draft) return
    if (draft.horarios.length === 0) {
      toast('error', 'Mantenha ao menos um horário de varredura — sem ele, nada é auditado.')
      return
    }
    setSaving(true)
    try {
      await api.updateSettings(tenant.id, draft)
      setSaved(draft)
      toast('success', 'Configurações de avisos salvas.')
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao salvar as configurações.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="animate-rise">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Avisos</h1>
        <p className="mt-1 max-w-prose text-sm text-ink-soft">
          Escolha quais irregularidades geram aviso e com que gravidade. Os alertas vão para o
          WhatsApp dos{' '}
          <Link to="/gestores" className="font-medium text-brand underline underline-offset-2 hover:text-brand-strong">
            gestores cadastrados
          </Link>
          .
        </p>
      </header>

      {state.phase === 'loading' && (
        <div className="mt-8 flex flex-col gap-2">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-24 w-full" />
        </div>
      )}

      {state.phase === 'error' && (
        <div className="mt-8">
          <ErrorNote message={state.message} onRetry={load} />
        </div>
      )}

      {state.phase === 'ready' && draft && (
        <>
          <ul className="mt-8 divide-y divide-line rounded-card border border-line bg-bg shadow-card">
            {rules.map((rule) => {
              const enabled = draft[rule.flag]
              return (
                <li key={rule.flag} className="flex flex-wrap items-start gap-x-4 gap-y-3 px-4 py-4">
                  <Toggle
                    checked={enabled}
                    onChange={(next) => patch({ [rule.flag]: next })}
                    label={`Ativar aviso: ${rule.title}`}
                  />
                  <div className="min-w-0 flex-1 basis-52">
                    <p className={`font-semibold ${enabled ? '' : 'text-ink-faint'}`}>{rule.title}</p>
                    <p className={`mt-0.5 max-w-prose text-sm leading-relaxed ${enabled ? 'text-ink-soft' : 'text-ink-faint'}`}>
                      {rule.description}
                    </p>
                  </div>
                  {rule.severityField ? (
                    <label className="flex items-center gap-2 text-sm text-ink-soft">
                      <span className="sr-only">Gravidade de {rule.title}</span>
                      <Select
                        disabled={!enabled}
                        value={draft[rule.severityField]}
                        onChange={(e) =>
                          patch({ [rule.severityField as string]: e.target.value as Severity })
                        }
                        className="disabled:opacity-50"
                      >
                        <option value="ALERTA">Alerta</option>
                        <option value="CRITICO">Crítico</option>
                      </Select>
                    </label>
                  ) : (
                    <span className={`self-center text-xs font-medium ${enabled ? 'text-ink-soft' : 'text-ink-faint'}`}>
                      gravidade definida em lei
                    </span>
                  )}
                </li>
              )
            })}
          </ul>

          <section className="mt-8" aria-label="Horários de varredura">
            <h2 className="text-sm font-semibold text-ink-soft">Horários de varredura</h2>
            <p className="mt-1 max-w-prose text-sm text-ink-soft">
              Nesses horários o sistema varre as batidas do dia e dispara os avisos preventivos. O
              fechamento oficial roda de madrugada.
            </p>
            <div className="mt-3 flex flex-wrap items-center gap-2">
              {draft.horarios.map((time) => (
                <span
                  key={time}
                  className="inline-flex items-center gap-1.5 rounded-full border border-line bg-panel py-1.5 pl-3 pr-1.5 text-sm font-medium tabular-nums"
                >
                  <Clock size={14} className="text-ink-faint" aria-hidden />
                  {time}
                  <button
                    type="button"
                    aria-label={`Remover horário ${time}`}
                    onClick={() => patch({ horarios: draft.horarios.filter((t) => t !== time) })}
                    className="rounded-full p-1 text-ink-faint transition-colors duration-150 hover:bg-critico-bg hover:text-critico"
                  >
                    <X size={14} />
                  </button>
                </span>
              ))}
              <span className="inline-flex items-center gap-1">
                <input
                  type="time"
                  value={newTime}
                  onChange={(e) => setNewTime(e.target.value)}
                  aria-label="Novo horário de varredura"
                  className="min-h-10 rounded-field border border-line bg-bg px-2 text-sm tabular-nums transition-colors duration-150 hover:border-ink-faint focus:border-brand"
                />
                <Button type="button" variant="secondary" onClick={addTime} disabled={!newTime} className="min-h-10 px-3">
                  <Plus size={16} aria-hidden />
                  Adicionar
                </Button>
              </span>
            </div>
          </section>

          <div className="sticky bottom-20 mt-8 md:bottom-6">
            {dirty && (
              <div className="flex animate-rise items-center justify-between gap-3 rounded-card border border-line bg-bg px-4 py-3 shadow-float">
                <p className="text-sm font-medium">Há alterações não salvas.</p>
                <div className="flex gap-2">
                  <Button variant="ghost" onClick={() => setDraft(saved)}>
                    Descartar
                  </Button>
                  <Button onClick={save} busy={saving}>
                    Salvar alterações
                  </Button>
                </div>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}
