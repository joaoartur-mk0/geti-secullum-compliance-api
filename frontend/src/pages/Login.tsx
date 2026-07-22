import { ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import { Button, Field, Input } from '../components/ui'
import { getApiUrl, setApiUrl } from '../lib/api'
import { getSession, startSession } from '../lib/session'

export default function Login() {
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [apiUrl, setApiUrlInput] = useState(getApiUrl())
  const [error, setError] = useState<string | null>(null)

  if (getSession()) return <Navigate to="/" replace />

  function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!email.trim() || !password.trim()) {
      setError('Preencha e-mail e senha para entrar.')
      return
    }
    setApiUrl(apiUrl)
    startSession(email.trim())
    navigate('/', { replace: true })
  }

  return (
    <div className="flex min-h-dvh flex-col bg-side md:flex-row">
      {/* Painel institucional */}
      <div className="flex flex-col justify-between px-6 py-8 text-side-ink md:w-[44%] md:px-12 md:py-12">
        <div className="flex items-center gap-2.5">
          <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-side-raise">
            <ShieldCheck size={22} className="text-brand-ring" aria-hidden />
          </span>
          <div className="leading-tight">
            <p className="font-semibold text-white">Secullum Compliance</p>
            <p className="text-xs text-side-faint">por Geti Soluções</p>
          </div>
        </div>

        <div className="hidden md:block">
          <h1 className="max-w-md text-3xl font-semibold leading-snug tracking-tight text-white">
            Irregularidades de ponto detectadas no mesmo dia, avisadas no WhatsApp do gestor.
          </h1>
          <p className="mt-4 max-w-md text-sm leading-relaxed text-side-faint">
            Auditoria automática das batidas do Secullum Ponto Web contra as regras da CLT:
            interjornada, intervalo de almoço, horas extras e batidas esquecidas.
          </p>
        </div>

        <p className="hidden text-xs text-side-faint md:block">
          © {new Date().getFullYear()} Geti Soluções · ambiente de validação
        </p>
      </div>

      {/* Formulário */}
      <div className="flex flex-1 items-start justify-center rounded-t-3xl bg-bg px-6 py-10 md:items-center md:rounded-none md:px-12">
        <form onSubmit={submit} className="flex w-full max-w-sm animate-rise flex-col gap-4">
          <div className="mb-2">
            <h2 className="text-xl font-semibold tracking-tight">Entrar no painel</h2>
            <p className="mt-1 text-sm text-ink-soft">
              Acesso provisório de validação — a autenticação definitiva chega com o modo
              multiempresa.
            </p>
          </div>

          <Field label="E-mail">
            <Input
              type="email"
              autoComplete="username"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="voce@empresa.com.br"
            />
          </Field>
          <Field label="Senha">
            <Input
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
            />
          </Field>

          {error && <p className="text-sm font-medium text-critico">{error}</p>}

          <Button type="submit">Entrar</Button>

          <details className="mt-2 text-sm">
            <summary className="cursor-pointer text-ink-soft transition-colors hover:text-ink">
              Configuração avançada
            </summary>
            <div className="mt-3">
              <Field label="Endereço da API" hint="Padrão: http://localhost:8080">
                <Input value={apiUrl} onChange={(e) => setApiUrlInput(e.target.value)} spellCheck={false} />
              </Field>
            </div>
          </details>
        </form>
      </div>
    </div>
  )
}
