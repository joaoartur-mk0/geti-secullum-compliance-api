import { Info, MessageCircle, Smartphone, Unplug } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Button, Field, Input, useToast } from '../components/ui'
import { formatPhone, isValidPhone, normalizePhone } from '../lib/format'

// Fluxo demonstrativo: o backend ainda não expõe os endpoints da Evolution API.
// O estado vive só no navegador para a prévia; quando a integração chegar,
// estas etapas passam a refletir o status real da instância.

const EVOLUTION_KEY = 'scw.evolution'

interface EvolutionState {
  instance: string
  number: string
  connectedAt: string
}

function loadState(): EvolutionState | null {
  try {
    const raw = localStorage.getItem(EVOLUTION_KEY)
    return raw ? (JSON.parse(raw) as EvolutionState) : null
  } catch {
    return null
  }
}

type Phase =
  | { step: 'disconnected' }
  | { step: 'qr'; instance: string; number: string; expiresIn: number }
  | { step: 'connected'; state: EvolutionState }

export default function WhatsApp() {
  const toast = useToast()
  const [phase, setPhase] = useState<Phase>(() => {
    const state = loadState()
    return state ? { step: 'connected', state } : { step: 'disconnected' }
  })

  return (
    <div className="animate-rise">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">WhatsApp</h1>
        <p className="mt-1 max-w-prose text-sm text-ink-soft">
          Conecte o número que enviará os alertas aos gestores. O disparo é feito pela Evolution
          API com o seu próprio número — sem custo por mensagem.
        </p>
      </header>

      <div className="mt-4 flex max-w-prose items-start gap-2.5 rounded-card bg-brand-soft px-4 py-3 text-sm leading-relaxed text-brand-strong">
        <Info size={16} className="mt-0.5 shrink-0" aria-hidden />
        <p>
          Prévia demonstrativa: a conexão real com a Evolution API será ativada junto com o
          backend. O fluxo abaixo mostra como funcionará.
        </p>
      </div>

      <div className="mt-8 max-w-lg">
        {phase.step === 'disconnected' && (
          <DisconnectedCard
            onGenerate={(instance, number) => setPhase({ step: 'qr', instance, number, expiresIn: 45 })}
          />
        )}
        {phase.step === 'qr' && (
          <QrCard
            instance={phase.instance}
            onExpire={() => {
              toast('error', 'O QR Code expirou. Gere um novo para conectar.')
              setPhase({ step: 'disconnected' })
            }}
            onCancel={() => setPhase({ step: 'disconnected' })}
            onScanned={() => {
              const state: EvolutionState = {
                instance: phase.instance,
                number: phase.number,
                connectedAt: new Date().toISOString(),
              }
              localStorage.setItem(EVOLUTION_KEY, JSON.stringify(state))
              toast('success', 'WhatsApp conectado. Os alertas sairão deste número.')
              setPhase({ step: 'connected', state })
            }}
          />
        )}
        {phase.step === 'connected' && (
          <ConnectedCard
            state={phase.state}
            onDisconnect={() => {
              localStorage.removeItem(EVOLUTION_KEY)
              toast('success', 'Instância desconectada.')
              setPhase({ step: 'disconnected' })
            }}
          />
        )}
      </div>
    </div>
  )
}

function DisconnectedCard({ onGenerate }: { onGenerate: (instance: string, number: string) => void }) {
  const [instance, setInstance] = useState('geti-alertas')
  const [number, setNumber] = useState('')
  const [error, setError] = useState<string | null>(null)

  function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!isValidPhone(number)) {
      setError('Informe o número com DDD que ficará responsável pelos disparos.')
      return
    }
    onGenerate(instance.trim() || 'geti-alertas', normalizePhone(number))
  }

  return (
    <form onSubmit={submit} className="rounded-card border border-line bg-bg p-5 shadow-card">
      <div className="flex items-center gap-3">
        <span className="flex h-11 w-11 items-center justify-center rounded-full bg-panel text-ink-faint">
          <Unplug size={20} aria-hidden />
        </span>
        <div>
          <p className="font-semibold">Nenhum número conectado</p>
          <p className="text-sm text-ink-soft">Os alertas ficam represados até a conexão.</p>
        </div>
      </div>

      <div className="mt-5 flex flex-col gap-4">
        <Field label="Nome da instância" hint="Identifica esta conexão na Evolution API.">
          <Input value={instance} onChange={(e) => setInstance(e.target.value)} spellCheck={false} />
        </Field>
        <Field label="Número do WhatsApp">
          <Input
            required
            inputMode="tel"
            value={number}
            onChange={(e) => setNumber(e.target.value)}
            placeholder="(31) 99999-9999"
          />
        </Field>
        {error && <p className="text-sm font-medium text-critico">{error}</p>}
        <Button type="submit" className="self-start">
          <MessageCircle size={17} aria-hidden />
          Gerar QR Code
        </Button>
      </div>
    </form>
  )
}

function QrCard({
  instance,
  onScanned,
  onCancel,
  onExpire,
}: {
  instance: string
  onScanned: () => void
  onCancel: () => void
  onExpire: () => void
}) {
  const [remaining, setRemaining] = useState(45)

  useEffect(() => {
    const timer = setInterval(() => setRemaining((s) => s - 1), 1000)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    if (remaining <= 0) onExpire()
  }, [remaining, onExpire])

  return (
    <div className="rounded-card border border-line bg-bg p-5 shadow-card">
      <p className="font-semibold">
        Escaneie com o WhatsApp <span className="font-normal text-ink-soft">· instância {instance}</span>
      </p>
      <ol className="mt-2 list-inside list-decimal text-sm leading-relaxed text-ink-soft">
        <li>Abra o WhatsApp no celular do número informado</li>
        <li>Toque em Configurações → Dispositivos conectados</li>
        <li>Aponte a câmera para o código abaixo</li>
      </ol>

      <div className="mt-5 flex flex-col items-center gap-3">
        <FakeQr seed={instance} />
        <p className="text-sm tabular-nums text-ink-soft" role="timer">
          Expira em {remaining}s
        </p>
      </div>

      <div className="mt-4 flex flex-wrap gap-2">
        <Button onClick={onScanned}>
          <Smartphone size={17} aria-hidden />
          Simular leitura (demo)
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Cancelar
        </Button>
      </div>
    </div>
  )
}

/** QR ilustrativo (não escaneável) gerado deterministicamente a partir da instância. */
function FakeQr({ seed }: { seed: string }) {
  const cells = useMemo(() => {
    let hash = 2166136261
    for (const char of seed) {
      hash = Math.imul(hash ^ char.charCodeAt(0), 16777619)
    }
    const size = 25
    const grid: boolean[] = []
    for (let i = 0; i < size * size; i++) {
      hash = Math.imul(hash ^ (hash >>> 15), 2246822519)
      grid.push(((hash >>> 13) & 1) === 1)
    }
    return grid
  }, [seed])

  const size = 25
  const finder = (x: number, y: number) => (
    <g key={`${x}-${y}`}>
      <rect x={x} y={y} width={7} height={7} fill="none" stroke="currentColor" strokeWidth={1} />
      <rect x={x + 2} y={y + 2} width={3} height={3} fill="currentColor" />
    </g>
  )

  return (
    <svg
      viewBox={`-1 -1 ${size + 2} ${size + 2}`}
      className="h-48 w-48 rounded-lg border border-line bg-white p-2 text-ink"
      role="img"
      aria-label="QR Code ilustrativo para conexão do WhatsApp"
    >
      {cells.map((on, index) => {
        const x = index % size
        const y = Math.floor(index / size)
        const inFinder = (x < 8 && y < 8) || (x > size - 9 && y < 8) || (x < 8 && y > size - 9)
        if (!on || inFinder) return null
        return <rect key={index} x={x} y={y} width={1} height={1} fill="currentColor" />
      })}
      {finder(0, 0)}
      {finder(size - 7, 0)}
      {finder(0, size - 7)}
    </svg>
  )
}

function ConnectedCard({ state, onDisconnect }: { state: EvolutionState; onDisconnect: () => void }) {
  return (
    <div className="rounded-card border border-line bg-bg p-5 shadow-card">
      <div className="flex flex-wrap items-center gap-3">
        <span className="flex h-11 w-11 items-center justify-center rounded-full bg-ok-bg text-ok">
          <MessageCircle size={20} aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <p className="flex items-center gap-2 font-semibold">
            {formatPhone(state.number)}
            <span className="inline-flex items-center gap-1.5 rounded-full bg-ok-bg px-2 py-0.5 text-xs font-semibold text-ok">
              <span className="h-1.5 w-1.5 animate-pulse-dot rounded-full bg-ok" aria-hidden />
              Conectado
            </span>
          </p>
          <p className="text-sm text-ink-soft">
            instância {state.instance} · desde{' '}
            {new Date(state.connectedAt).toLocaleDateString('pt-BR', { day: '2-digit', month: 'short' })}
          </p>
        </div>
        <Button variant="danger" onClick={onDisconnect}>
          <Unplug size={16} aria-hidden />
          Desconectar
        </Button>
      </div>
      <p className="mt-4 max-w-prose text-sm leading-relaxed text-ink-soft">
        Os alertas de irregularidade sairão deste número para os gestores cadastrados, nos
        horários de varredura configurados.
      </p>
    </div>
  )
}
