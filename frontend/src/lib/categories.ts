// Metadados de exibição da categoria de ocorrência (eixo diferente de Severity — ver
// tipos em lib/types.ts). Centralizado aqui para o rótulo/cor/ordem não divergir entre
// Indicadores, a tela de colaborador e qualquer outro lugar que liste ocorrências.

import type { OccurrenceCategory } from './types'

export const CATEGORY_ORDER: OccurrenceCategory[] = [
  'CRITICO',
  'ALERTA',
  'ALTERACAO_ESCALA',
  'NAO_CONFIRMADA',
]

export const CATEGORY_LABEL: Record<OccurrenceCategory, string> = {
  CRITICO: 'Crítico',
  ALERTA: 'Alerta',
  ALTERACAO_ESCALA: 'Alteração de escala',
  NAO_CONFIRMADA: 'Não confirmada',
}

export const CATEGORY_HINT: Record<OccurrenceCategory, string> = {
  CRITICO: 'Infração grave, exige ação imediata.',
  ALERTA: 'Atenção preventiva.',
  ALTERACAO_ESCALA: 'Escala provavelmente desatualizada — não é infração da CLT.',
  NAO_CONFIRMADA: 'O valor mudou desde a última varredura; precisa de nova conferência.',
}

// Classes utilitárias (texto/fundo) por categoria, seguindo o padrão dos tokens
// semânticos já usados para severidade (bg-*-bg / text-*).
export const CATEGORY_CLASSES: Record<OccurrenceCategory, { text: string; bg: string; dot: string }> = {
  CRITICO: { text: 'text-critico', bg: 'bg-critico-bg', dot: 'bg-critico' },
  ALERTA: { text: 'text-alerta', bg: 'bg-alerta-bg', dot: 'bg-alerta' },
  ALTERACAO_ESCALA: { text: 'text-operacional', bg: 'bg-operacional-bg', dot: 'bg-operacional' },
  NAO_CONFIRMADA: { text: 'text-revisar', bg: 'bg-revisar-bg', dot: 'bg-revisar' },
}
