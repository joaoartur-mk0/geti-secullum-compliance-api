import { MessageCircle, RefreshCw, Unplug } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { Button, ErrorNote, Skeleton, useToast } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'

// Fluxo real da Evolution API: o backend gerencia a instância de WhatsApp do tenant
// (criar/QR/status/desconectar). A instância é derivada no backend (prefixo + tenant),
// e o número conectado é o do celular que escanear o QR.

type Phase =
  | { step: 'loading' }
  | { step: 'error'; message: string }
  | { step: 'disconnected' }
  | { step: 'qr'; qrcode: string }
  | { step: 'connected' }

export default function WhatsApp() {
  const { tenant } = useTenant()
  const toast = useToast()
  const [phase, setPhase] = useState<Phase>({ step: 'loading' })
  const [busy, setBusy] = useState(false)

  const loadStatus = useCallback(async () => {
    setPhase({ step: 'loading' })
    try {
      const status = await api.getWhatsappStatus(tenant.id)
      setPhase(status.connected ? { step: 'connected' } : { step: 'disconnected' })
    } catch (error) {
      setPhase({
        step: 'error',
        message: error instanceof ApiError ? error.message : 'Erro ao consultar o WhatsApp.',
      })
    }
  }, [tenant.id])

  useEffect(() => {
    void loadStatus()
  }, [loadStatus])

  // Enquanto o QR está na tela, verifica o status a cada 3s para detectar o pareamento.
  useEffect(() => {
    if (phase.step !== 'qr') return
    const timer = setInterval(async () => {
      try {
        const status = await api.getWhatsappStatus(tenant.id)
        if (status.connected) {
          setPhase({ step: 'connected' })
          toast('success', 'WhatsApp conectado. Os alertas sairão deste número.')
        }
      } catch {
        // Erros transitórios no polling são ignorados; a próxima varredura tenta de novo.
      }
    }, 3000)
    return () => clearInterval(timer)
  }, [phase.step, tenant.id, toast])

  async function connect() {
    setBusy(true)
    try {
      const res = await api.connectWhatsapp(tenant.id)
      if (res.connected) {
        setPhase({ step: 'connected' })
      } else if (res.qrcode) {
        setPhase({ step: 'qr', qrcode: res.qrcode })
      } else {
        toast('error', 'A instância não devolveu um QR Code. Tente novamente em instantes.')
      }
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao conectar o WhatsApp.')
    } finally {
      setBusy(false)
    }
  }

  async function disconnect() {
    setBusy(true)
    try {
      await api.disconnectWhatsapp(tenant.id)
      toast('success', 'Instância desconectada.')
      setPhase({ step: 'disconnected' })
    } catch (error) {
      toast('error', error instanceof ApiError ? error.message : 'Falha ao desconectar.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="animate-rise">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">WhatsApp</h1>
        <p className="mt-1 max-w-prose text-sm text-ink-soft">
          Conecte o número que enviará os alertas aos gestores. O disparo é feito pela Evolution
          API com o seu próprio número — sem custo por mensagem.
        </p>
      </header>

      <div className="mt-8 max-w-lg">
        {phase.step === 'loading' && <Skeleton className="h-40 w-full" />}

        {phase.step === 'error' && <ErrorNote message={phase.message} onRetry={loadStatus} />}

        {phase.step === 'disconnected' && <DisconnectedCard onConnect={connect} busy={busy} />}

        {phase.step === 'qr' && (
          <QrCard
            qrcode={phase.qrcode}
            onCancel={() => setPhase({ step: 'disconnected' })}
            onRegenerate={connect}
            busy={busy}
          />
        )}

        {phase.step === 'connected' && <ConnectedCard onDisconnect={disconnect} busy={busy} />}
      </div>
    </div>
  )
}

function DisconnectedCard({ onConnect, busy }: { onConnect: () => void; busy: boolean }) {
  return (
    <div className="rounded-card border border-line bg-bg p-5 shadow-card">
      <div className="flex items-center gap-3">
        <span className="flex h-11 w-11 items-center justify-center rounded-full bg-panel text-ink-faint">
          <Unplug size={20} aria-hidden />
        </span>
        <div>
          <p className="font-semibold">Nenhum número conectado</p>
          <p className="text-sm text-ink-soft">Os alertas ficam represados até a conexão.</p>
        </div>
      </div>
      <p className="mt-4 max-w-prose text-sm leading-relaxed text-ink-soft">
        Gere o QR Code e escaneie com o WhatsApp do número que fará os disparos. A conexão é
        detectada automaticamente assim que o pareamento é concluído.
      </p>
      <Button onClick={onConnect} busy={busy} className="mt-5 self-start">
        <MessageCircle size={17} aria-hidden />
        Gerar QR Code
      </Button>
    </div>
  )
}

function QrCard({
  qrcode,
  onCancel,
  onRegenerate,
  busy,
}: {
  qrcode: string
  onCancel: () => void
  onRegenerate: () => void
  busy: boolean
}) {
  return (
    <div className="rounded-card border border-line bg-bg p-5 shadow-card">
      <p className="font-semibold">Escaneie com o WhatsApp</p>
      <ol className="mt-2 list-inside list-decimal text-sm leading-relaxed text-ink-soft">
        <li>Abra o WhatsApp no celular do número que fará os disparos</li>
        <li>Toque em Configurações → Dispositivos conectados</li>
        <li>Aponte a câmera para o código abaixo</li>
      </ol>

      <div className="mt-5 flex flex-col items-center gap-3">
        <img
          src={qrcode}
          alt="QR Code para conectar o WhatsApp"
          className="h-56 w-56 rounded-lg border border-line bg-white p-2"
        />
        <p className="flex items-center gap-2 text-sm text-ink-soft" role="status">
          <span className="h-1.5 w-1.5 animate-pulse-dot rounded-full bg-brand" aria-hidden />
          Aguardando leitura…
        </p>
      </div>

      <div className="mt-4 flex flex-wrap gap-2">
        <Button variant="secondary" onClick={onRegenerate} busy={busy}>
          <RefreshCw size={16} aria-hidden />
          Gerar novo QR
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Cancelar
        </Button>
      </div>
    </div>
  )
}

function ConnectedCard({ onDisconnect, busy }: { onDisconnect: () => void; busy: boolean }) {
  return (
    <div className="rounded-card border border-line bg-bg p-5 shadow-card">
      <div className="flex flex-wrap items-center gap-3">
        <span className="flex h-11 w-11 items-center justify-center rounded-full bg-ok-bg text-ok">
          <MessageCircle size={20} aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <p className="flex items-center gap-2 font-semibold">
            WhatsApp conectado
            <span className="inline-flex items-center gap-1.5 rounded-full bg-ok-bg px-2 py-0.5 text-xs font-semibold text-ok">
              <span className="h-1.5 w-1.5 animate-pulse-dot rounded-full bg-ok" aria-hidden />
              Ativo
            </span>
          </p>
          <p className="text-sm text-ink-soft">A instância está pareada e pronta para os disparos.</p>
        </div>
        <Button variant="danger" onClick={onDisconnect} busy={busy}>
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
