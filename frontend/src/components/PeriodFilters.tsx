// Barra de filtros compartilhada por Histórico, Ranking e Investigar.
//
// Existe para que o recorte de período seja idêntico nas três telas — presets iguais,
// mesma aparência, e sobretudo o mesmo contrato de estado: TUDO na querystring
// (docs/11_Historico_Ranking_Frontend_Contract.md, §5). Filtro em estado local quebra o
// link colado no chat, que é como esse time troca informação.
//
// Deliberadamente NÃO é uma extração do PeriodFilterBar de Indicadores: aquela barra tem
// o limite de "só dias encerrados" (é ligada a disparo de auditoria), que aqui não faz
// sentido — consulta de histórico inclui hoje.

import { useSearchParams } from 'react-router-dom'
import { Input, Select } from './ui'
import {
  PRESET_LABEL,
  periodForPreset,
  today,
  type Period,
  type PeriodPreset,
} from '../lib/periods'
import type { Branch } from '../lib/types'

const PRESETS: PeriodPreset[] = ['7d', '30d', 'mes', 'custom']

/**
 * usePeriodParams lê o período da querystring e devolve o intervalo resolvido.
 *
 * O preset é a fonte da verdade quando presente; `start_date`/`end_date` só valem no
 * modo personalizado. Guardar os dois na URL permite que um link com datas explícitas
 * continue abrindo o mesmo recorte meses depois — um link com `preset=7d` não faria isso.
 */
export function usePeriodParams(defaultPreset: PeriodPreset = '30d'): {
  preset: PeriodPreset
  period: Period
  setParam: (key: string, value: string) => void
  setPreset: (preset: PeriodPreset) => void
  params: URLSearchParams
} {
  const [searchParams, setSearchParams] = useSearchParams()

  const preset = (searchParams.get('preset') as PeriodPreset | null) ?? defaultPreset
  const fallback = periodForPreset(preset) ?? periodForPreset(defaultPreset) ?? { start: '', end: today() }

  const period: Period =
    preset === 'custom'
      ? {
          start: searchParams.get('start_date') ?? fallback.start,
          end: searchParams.get('end_date') ?? fallback.end,
        }
      : fallback

  function setParam(key: string, value: string) {
    const next = new URLSearchParams(searchParams)
    if (value) next.set(key, value)
    else next.delete(key)
    setSearchParams(next, { replace: true })
  }

  function setPreset(nextPreset: PeriodPreset) {
    const next = new URLSearchParams(searchParams)
    next.set('preset', nextPreset)
    if (nextPreset === 'custom') {
      // Semear as datas com o intervalo que já estava na tela evita o estado
      // "personalizado com campos vazios", que devolve a lista inteira sem aviso.
      next.set('start_date', period.start)
      next.set('end_date', period.end)
    } else {
      next.delete('start_date')
      next.delete('end_date')
    }
    setSearchParams(next, { replace: true })
  }

  return { preset, period, setParam, setPreset, params: searchParams }
}

export function PeriodFilters({
  preset,
  period,
  onPreset,
  onParam,
  branches,
  branchId,
  children,
}: {
  preset: PeriodPreset
  period: Period
  onPreset: (preset: PeriodPreset) => void
  onParam: (key: string, value: string) => void
  branches?: Branch[]
  branchId?: string
  children?: React.ReactNode
}) {
  // [&>label]:max-w-full contém os filtros no mobile: cada <label> é item deste flex-wrap
  // e, sem teto, dimensiona pela opção mais longa do <select> que carrega (nome completo
  // de colaborador), empurrando a página inteira para o scroll horizontal. Aplicar no
  // label — e não um teto em px no <select> — preserva a largura disponível no desktop.
  // Vale também para os filtros que as páginas injetam via children.
  return (
    <section
      aria-label="Filtros"
      className="mt-6 flex flex-wrap items-end gap-3 rounded-card border border-line bg-bg p-4 shadow-card [&>label]:max-w-full"
    >
      <div className="flex flex-col gap-1.5">
        <span className="text-sm font-medium text-ink">Período</span>
        <div className="flex flex-wrap items-center gap-1.5">
          {PRESETS.map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => onPreset(p)}
              aria-pressed={preset === p}
              className={`flex min-h-11 items-center rounded-field px-3 text-sm font-medium transition-colors duration-150 ${
                preset === p
                  ? 'bg-brand text-white'
                  : 'border border-line text-ink-soft hover:border-ink-faint hover:text-ink'
              }`}
            >
              {PRESET_LABEL[p]}
            </button>
          ))}
        </div>
      </div>

      {preset === 'custom' && (
        <>
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-ink">De</span>
            <Input
              type="date"
              value={period.start}
              max={period.end || today()}
              onChange={(e) => onParam('start_date', e.target.value)}
              aria-label="Início do período"
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-ink">Até</span>
            <Input
              type="date"
              value={period.end}
              min={period.start || undefined}
              onChange={(e) => onParam('end_date', e.target.value)}
              aria-label="Fim do período"
            />
          </label>
        </>
      )}

      {branches && branches.length > 0 && (
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink">Filial</span>
          <Select
            value={branchId ?? ''}
            onChange={(e) => onParam('branch_id', e.target.value)}
            className="min-w-40"
          >
            <option value="">Todas</option>
            {branches.map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </Select>
        </label>
      )}

      {children}
    </section>
  )
}

/**
 * BranchResolutionNote explica de onde veio a filial numa consulta de período.
 *
 * Em intervalo de mais de um dia o backend não carrega as batidas, então a resolução por
 * aparelho não roda e sobra o cadastro de lotação. Sem esta nota, o usuário lê o número
 * por filial como se fosse presença física confirmada.
 */
export function BranchResolutionNote() {
  return (
    <p className="mt-2 text-xs text-ink-faint">
      Filial resolvida pelo cadastro de lotação (nº de folha). Em consulta de período, a
      origem pelo aparelho de ponto não é considerada.
    </p>
  )
}
