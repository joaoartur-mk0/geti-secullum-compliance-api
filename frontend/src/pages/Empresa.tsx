import { ArrowLeftRight, Building2, Database, Info, Plus } from 'lucide-react'
import { useState } from 'react'
import { Button, Field, Input, useToast } from '../components/ui'
import { useTenant } from '../layouts/AppShell'
import { api, ApiError } from '../lib/api'
import { isValidPhone, normalizePhone } from '../lib/format'
import { isSuperAdmin, saveTenantId } from '../lib/session'
import type { Tenant } from '../lib/types'

export default function Empresa() {
  const { tenant, tenants, reloadTenant, switchTenant } = useTenant()

  return (
    <div className="animate-rise">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Empresa</h1>
        <p className="mt-1 max-w-prose text-sm text-ink-soft">
          Dados da empresa auditada e do banco correspondente no Secullum Ponto Web.
        </p>
      </header>

      {/* key força o formulário a recarregar ao trocar de empresa */}
      <CompanyDataForm key={tenant.id} tenant={tenant} onSaved={reloadTenant} />

      <CompanyList tenants={tenants} current={tenant} onSwitch={switchTenant} onCreated={reloadTenant} />
    </div>
  )
}

// ---------- Dados da empresa em consulta ----------

function CompanyDataForm({ tenant, onSaved }: { tenant: Tenant; onSaved: () => void }) {
  const toast = useToast()
  const admin = isSuperAdmin()
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
      onSaved()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Erro inesperado ao salvar.')
      setSaving(false)
    }
  }

  return (
    <form onSubmit={save} className="mt-8 max-w-lg rounded-card border border-line bg-bg p-5 shadow-card">
      <div className="flex flex-col gap-4">
        <Field label="Nome da empresa">
          <Input required disabled={!admin} value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field
          label="ID do banco no Secullum"
          hint="É deste banco que as batidas são lidas. Corrija aqui se cadastrou o número errado."
        >
          <Input
            required
            disabled={!admin}
            inputMode="numeric"
            pattern="[0-9]+"
            value={databaseId}
            onChange={(e) => setDatabaseId(e.target.value)}
          />
        </Field>

        {admin ? (
          <div className="flex max-w-prose items-start gap-2.5 rounded-card bg-brand-soft px-4 py-3 text-sm leading-relaxed text-brand-strong">
            <Info size={16} className="mt-0.5 shrink-0" aria-hidden />
            <p>
              Trocar o ID do banco muda a fonte das próximas auditorias. Os relatórios já gerados
              continuam guardados com a empresa.
            </p>
          </div>
        ) : (
          <p className="text-sm text-ink-soft">
            Só um super admin pode alterar esses dados — fale com quem administra o painel.
          </p>
        )}

        {error && <p className="text-sm font-medium text-critico">{error}</p>}

        {admin && (
          <Button type="submit" busy={saving} disabled={!dirty} className="self-start">
            <Database size={16} aria-hidden />
            Salvar dados
          </Button>
        )}
      </div>
    </form>
  )
}

// ---------- Prévia do modo multiempresa ----------

function CompanyList({
  tenants,
  current,
  onSwitch,
  onCreated,
}: {
  tenants: Tenant[]
  current: Tenant
  onSwitch: (id: number) => void
  onCreated: () => void
}) {
  const toast = useToast()
  const [adding, setAdding] = useState(false)

  return (
    <section aria-label="Suas empresas" className="mt-8 max-w-lg">
      <h2 className="text-sm font-semibold text-ink-soft">Suas empresas</h2>
      <p className="mt-1 max-w-prose text-sm text-ink-soft">
        Prévia do modo multiempresa: todas as empresas deste acesso, com gestores, avisos e
        relatórios separados. Alterne qual está em consulta.
      </p>

      <ul className="mt-3 flex flex-col gap-2">
        {tenants.map((t) => {
          const active = t.id === current.id
          return (
            <li
              key={t.id}
              className={`flex flex-wrap items-center gap-x-4 gap-y-2 rounded-card border px-4 py-3 ${
                active ? 'border-brand/40 bg-brand-soft/40' : 'border-line bg-bg'
              }`}
            >
              <Building2 size={18} className={active ? 'text-brand' : 'text-ink-faint'} aria-hidden />
              <div className="min-w-0 flex-1">
                <p className="truncate font-semibold">{t.name}</p>
                <p className="text-xs text-ink-soft">banco Secullum {t.secullum_database_id}</p>
              </div>
              {active ? (
                <span className="rounded-full bg-brand-soft px-2.5 py-1 text-xs font-semibold text-brand-strong">
                  Em consulta
                </span>
              ) : (
                <Button variant="secondary" onClick={() => onSwitch(t.id)} className="min-h-9 px-3 text-xs">
                  <ArrowLeftRight size={14} aria-hidden />
                  Consultar
                </Button>
              )}
            </li>
          )
        })}
      </ul>

      {isSuperAdmin() && (
        <div className="mt-3">
          {adding ? (
            <AddCompanyForm
              onCancel={() => setAdding(false)}
              onCreated={(tenant) => {
                toast('success', `${tenant.name} cadastrada e em consulta.`)
                setAdding(false)
                saveTenantId(tenant.id)
                onCreated()
              }}
            />
          ) : (
            <Button variant="secondary" onClick={() => setAdding(true)}>
              <Plus size={16} aria-hidden />
              Adicionar empresa
            </Button>
          )}
        </div>
      )}
    </section>
  )
}

function AddCompanyForm({
  onCreated,
  onCancel,
}: {
  onCreated: (tenant: Tenant) => void
  onCancel: () => void
}) {
  const [name, setName] = useState('')
  const [databaseId, setDatabaseId] = useState('')
  const [staffName, setStaffName] = useState('')
  const [staffContact, setStaffContact] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!isValidPhone(staffContact)) {
      setError('Informe um celular válido com DDD, ex.: (31) 99999-9999.')
      return
    }
    setBusy(true)
    setError(null)
    try {
      const { tenant } = await api.createTenant({
        name: name.trim(),
        secullum_database_id: Number(databaseId),
        staff_name: staffName.trim(),
        staff_contact: normalizePhone(staffContact),
      })
      onCreated(tenant)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Erro inesperado ao cadastrar a empresa.')
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="animate-rise rounded-card border border-brand/30 bg-brand-soft/40 p-4">
      <p className="font-semibold">Nova empresa</p>
      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <Field label="Nome da empresa">
          <Input required autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Empresa Exemplo S.A." />
        </Field>
        <Field label="ID do banco no Secullum">
          <Input
            required
            inputMode="numeric"
            pattern="[0-9]+"
            value={databaseId}
            onChange={(e) => setDatabaseId(e.target.value)}
            placeholder="123"
          />
        </Field>
        <Field label="Nome do gestor">
          <Input required value={staffName} onChange={(e) => setStaffName(e.target.value)} placeholder="Fulano de Tal" />
        </Field>
        <Field label="Celular do gestor (WhatsApp)">
          <Input
            required
            inputMode="tel"
            value={staffContact}
            onChange={(e) => setStaffContact(e.target.value)}
            placeholder="(31) 99999-9999"
          />
        </Field>
      </div>
      {error && <p className="mt-2 text-sm font-medium text-critico">{error}</p>}
      <div className="mt-4 flex gap-2">
        <Button type="submit" busy={busy}>
          Cadastrar empresa
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancelar
        </Button>
      </div>
    </form>
  )
}
