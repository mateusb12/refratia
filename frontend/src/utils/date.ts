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

export function formatDatePtBR(value: unknown) {
  const raw = typeof value === 'string' ? value.trim() : ''
  const isoMatch = raw.match(/^(\d{4})-(\d{2})-(\d{2})(?:\s|$)/)
  if (isoMatch) return `${isoMatch[3]}/${isoMatch[2]}/${isoMatch[1]}`

  const slashMatch = raw.match(/^(\d{1,2})[/-](\d{1,2})[/-](\d{4})$/)
  if (!slashMatch) return 'não informado'

  const first = Number(slashMatch[1])
  const second = Number(slashMatch[2])
  const year = slashMatch[3]
  const month = second > 12 ? first : second
  const day = second > 12 ? second : first

  return `${String(day).padStart(2, '0')}/${String(month).padStart(2, '0')}/${year}`
}

export function formatDateLongPtBR(value: unknown) {
  const raw = typeof value === 'string' ? value.trim() : ''
  const date = raw.match(/\d{4}-\d{2}-\d{2}|\d{1,2}[/-]\d{1,2}[/-]\d{4}/)?.[0]
  return date ? formatBirthDate(date) : 'não informado'
}

export function calculateAge(birthDate: unknown, referenceDate: unknown) {
  const birth = parseDateParts(birthDate)
  const reference = parseDateParts(referenceDate)
  if (!birth || !reference) return null

  let age = reference.year - birth.year
  if (reference.month < birth.month || (reference.month === birth.month && reference.day < birth.day)) age -= 1
  // ponytail: age 0 here has only ever meant "birth date read as the exam date"
  // (refractive-surgery exams are not done on infants), so treat it as unknown
  // instead of a real age. Eligibility bounds (e.g. >=18) belong to the protocol layer.
  return age >= 1 ? age : null
}

function parseDateParts(value: unknown) {
  const raw = typeof value === 'string' ? value.trim() : ''
	const longMatch = raw.match(/^(\d{1,2}) de ([a-zç]+) de (\d{4})$/i)
	if (longMatch) {
		const month = portugueseMonthNames.findIndex((name) => name.toLowerCase() === longMatch[2].toLowerCase()) + 1
		if (month > 0) return { year: Number(longMatch[3]), month, day: Number(longMatch[1]) }
	}

  const isoMatch = raw.match(/^(\d{4})-(\d{2})-(\d{2})(?:\s|$)/)
  if (isoMatch) return { year: Number(isoMatch[1]), month: Number(isoMatch[2]), day: Number(isoMatch[3]) }

  const slashMatch = raw.match(/^(\d{1,2})[/-](\d{1,2})[/-](\d{2,4})(?:\s|$)/)
  if (!slashMatch) return null

  const first = Number(slashMatch[1])
  const second = Number(slashMatch[2])
  const rawYear = Number(slashMatch[3])
  const year = rawYear < 100 ? rawYear + 2000 : rawYear
  return second > 12
    ? { year, month: first, day: second }
    : { year, month: second, day: first }
}
