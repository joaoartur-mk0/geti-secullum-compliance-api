// Datas chegam como "2026-07-12" (date) e ISO completo (data_generated).

export function formatDate(isoDate: string): string {
  const [y, m, d] = isoDate.slice(0, 10).split('-').map(Number)
  if (!y || !m || !d) return isoDate
  return new Date(y, m - 1, d).toLocaleDateString('pt-BR', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
  })
}

export function formatDateTime(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleString('pt-BR', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

/** Normaliza para o formato aceito pela Evolution API: "5531999999999" (só dígitos, com DDI). */
export function normalizePhone(input: string): string {
  let digits = input.replace(/\D/g, '')
  if (digits.length === 10 || digits.length === 11) digits = `55${digits}`
  return digits
}

/** Exibe "5531999999999" como "+55 (31) 99999-9999". */
export function formatPhone(digits: string): string {
  const d = digits.replace(/\D/g, '')
  const local = d.startsWith('55') && d.length >= 12 ? d.slice(2) : d
  if (local.length < 10) return digits
  const ddd = local.slice(0, 2)
  const number = local.slice(2)
  const cut = number.length - 4
  return `+55 (${ddd}) ${number.slice(0, cut)}-${number.slice(cut)}`
}

export function isValidPhone(input: string): boolean {
  const digits = normalizePhone(input)
  return digits.length === 12 || digits.length === 13
}

// Só dias já encerrados (antes de hoje) podem ser auditados sob demanda — o backend
// recusa hoje/futuro (ver TriggerRequest.resolveDate no handler de auditoria). Usado como
// limite máximo em qualquer seletor de data ligado a auditoria.
// Formata pelos componentes LOCAIS da data, não por toISOString().
//
// `toISOString` converte para UTC: a partir das 21h no horário de Brasília (UTC-3), o
// "ontem" local já é "hoje" em UTC, e a função devolvia a data de hoje. O seletor então
// oferecia como limite um dia que o backend recusa auditar — bug silencioso que só
// aparecia no fim da noite.
export function yesterday(): string {
  const d = new Date()
  d.setDate(d.getDate() - 1)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
