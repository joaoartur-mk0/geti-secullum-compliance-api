// Linha expansível de uma varredura (relatório), usada em Situação por dia (1 linha por
// dia, sempre a mais recente).

import { ChevronDown } from 'lucide-react'
import { useState } from 'react'
import { SeverityBadge } from './ui'
import { formatDate, formatDateTime } from '../lib/format'
import type { Report } from '../lib/types'

export default function ReportRow({ report }: { report: Report }) {
  const [open, setOpen] = useState(false)
  const inconsistencies = report.inconsistencies ?? []
  const criticos = inconsistencies.filter((i) => i.Severity === 'CRITICO').length
  const alertas = inconsistencies.length - criticos
  const clean = report.total === 0

  return (
    <li className="rounded-card border border-line bg-bg shadow-card">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex w-full flex-wrap items-center gap-x-4 gap-y-1 rounded-card px-4 py-3.5 text-left transition-colors duration-150 hover:bg-panel"
      >
        <div className="min-w-32">
          <p className="font-semibold">{formatDate(report.date)}</p>
          <p className="text-xs text-ink-faint">gerado {formatDateTime(report.data_generated)}</p>
        </div>
        <div className="flex flex-1 flex-wrap items-center gap-2 text-sm">
          {clean ? (
            <span className="inline-flex items-center gap-1 rounded-full bg-ok-bg px-2.5 py-1 text-xs font-semibold text-ok">
              Sem inconsistências
            </span>
          ) : (
            <>
              {criticos > 0 && (
                <span className="inline-flex items-center rounded-full bg-critico-bg px-2.5 py-1 text-xs font-semibold text-critico">
                  {criticos} crítica{criticos > 1 ? 's' : ''}
                </span>
              )}
              {alertas > 0 && (
                <span className="inline-flex items-center rounded-full bg-alerta-bg px-2.5 py-1 text-xs font-semibold text-alerta">
                  {alertas} alerta{alertas > 1 ? 's' : ''}
                </span>
              )}
            </>
          )}
        </div>
        <ChevronDown
          size={18}
          aria-hidden
          className={`shrink-0 text-ink-faint transition-transform duration-200 ${open ? 'rotate-180' : ''}`}
        />
      </button>

      {open && (
        <div className="border-t border-line px-4 py-3">
          {inconsistencies.length === 0 ? (
            <p className="py-2 text-sm text-ink-soft">
              Todas as batidas do dia estavam em conformidade com as regras ativas.
            </p>
          ) : (
            <ul className="divide-y divide-line">
              {inconsistencies.map((item, index) => (
                <li key={index} className="flex flex-wrap items-start gap-x-4 gap-y-1 py-3">
                  <div className="min-w-0 flex-1">
                    <p className="font-medium">{item.CollaboratorName}</p>
                    <p className="mt-0.5 text-sm text-ink-soft">
                      {item.Type} — {item.Description}
                    </p>
                  </div>
                  <SeverityBadge severity={item.Severity} />
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </li>
  )
}
