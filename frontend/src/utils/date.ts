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
