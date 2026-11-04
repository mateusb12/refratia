const portugueseMonthNames = [
  'Janeiro',
  'Fevereiro',
  'Março',
  'Abril',
  'Maio',
  'Junho',
  'Julho',
  'Agosto',
  'Setembro',
  'Outubro',
  'Novembro',
  'Dezembro',
]

export function formatLongDatePtBR(value: string | Date) {
  const date = typeof value === 'string' ? new Date(value) : value

  if (Number.isNaN(date.getTime())) return 'Data não informada'

  return `${date.getUTCDate()} de ${portugueseMonthNames[date.getUTCMonth()]} de ${date.getUTCFullYear()}`
}

export function formatBirthDate(value: unknown) {
  const raw = typeof value === 'object' && value !== null && 'normalized' in value
    ? String((value as { normalized?: unknown }).normalized ?? '')
    : typeof value === 'string' ? value.trim() : ''

  if (/^\d{1,2} de [a-zç]+ de \d{4}$/i.test(raw)) return raw

  let day = 0
  let month = 0
  let year = 0
  let match = raw.match(/^(\d{4})-(\d{2})-(\d{2})$/)

  if (match) {
    year = Number(match[1])
    month = Number(match[2])
    day = Number(match[3])
  } else if ((match = raw.match(/^(\d{1,2})[/-](\d{1,2})[/-](\d{4})$/))) {
    const first = Number(match[1])
    const second = Number(match[2])
    year = Number(match[3])

    if (first > 12) {
      day = first
      month = second
    } else if (second > 12) {
      month = first
      day = second
    } else {
      day = first
      month = second
    }
  }

  if (day < 1 || month < 1 || month > 12 || year < 1000) return 'não informado'
  return `${day} de ${portugueseMonthNames[month - 1].toLowerCase()} de ${year}`
}
