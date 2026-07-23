import { Database, Info } from 'lucide-react'
import { useState } from 'react'
import { Button, Field, Input, useToast } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'

export default function Empresa() {
  const { tenant, reloadTenant } = useTenant()
  const toast = useToast()

  const [name, setName] = useState(tenant.name)
  const [databaseId, setDatabaseId] = useState(String(tenant.secullum_database_id))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const dirty = name.trim() !== tenant.name || databaseId !== String(tenant.secullum_database_id)

  async function save(event: React.FormEvent) {
    event.preventDefault()
    setSaving(true)
    setError(null)
    try {
      await api.updateTenant(tenant.id, {
        name: name.trim(),
        secullum_database_id: Number(databaseId),
      })
      toast('success', 'Dados da empresa atualizados.')
      reloadTenant()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Erro inesperado ao salvar.')
      setSaving(false)
    }
  }

  return (
    <div className="animate-rise">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Empresa</h1>
        <p className="mt-1 max-w-prose text-sm text-ink-soft">
          Dados da empresa auditada e do banco correspondente no Secullum Ponto Web.
        </p>
      </header>

      <form onSubmit={save} className="mt-8 max-w-lg rounded-card border border-line bg-bg p-5 shadow-card">
        <div className="flex flex-col gap-4">
          <Field label="Nome da empresa">
            <Input required value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Field
            label="ID do banco no Secullum"
            hint="É deste banco que as batidas são lidas. Corrija aqui se cadastrou o número errado."
          >
            <Input
              required
              inputMode="numeric"
              pattern="[0-9]+"
              value={databaseId}
              onChange={(e) => setDatabaseId(e.target.value)}
            />
          </Field>

          <div className="flex max-w-prose items-start gap-2.5 rounded-card bg-brand-soft px-4 py-3 text-sm leading-relaxed text-brand-strong">
            <Info size={16} className="mt-0.5 shrink-0" aria-hidden />
            <p>
              Trocar o ID do banco muda a fonte das próximas auditorias. Os relatórios já gerados
              continuam guardados com a empresa.
            </p>
          </div>

          {error && <p className="text-sm font-medium text-critico">{error}</p>}

          <Button type="submit" busy={saving} disabled={!dirty} className="self-start">
            <Database size={16} aria-hidden />
            Salvar dados
          </Button>
        </div>
      </form>
    </div>
  )
}
