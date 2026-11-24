import { calculateAge } from './date.ts'

// Protocolo_RefratIA_FINAL p14 / p45 item 45g: the only place the age thresholds live.
export const MIN_PROTOCOL_AGE = 18
export const FLOW_B_MIN_AGE = 40
export const FLOW_C_MIN_AGE = 55

export type ClinicalFlow = 'A' | 'B' | 'C' | 'out_of_scope'

export function selectClinicalFlow(age: number | null): ClinicalFlow | null {
  if (age === null || !Number.isFinite(age)) return null
  if (age < MIN_PROTOCOL_AGE) return 'out_of_scope'
  if (age < FLOW_B_MIN_AGE) return 'A'
  if (age < FLOW_C_MIN_AGE) return 'B'
  return 'C'
}

const isLeap = (y: number) => (y % 4 === 0 && y % 100 !== 0) || y % 400 === 0
const validIso = (y: number, m: number, d: number) => {
  const days = [31, isLeap(y) ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]
  if (y < 1000 || m < 1 || m > 12 || d < 1 || d > days[m - 1]) return null
  return `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
}

const MONTHS = ['janeiro', 'fevereiro', 'março', 'abril', 'maio', 'junho', 'julho', 'agosto', 'setembro', 'outubro', 'novembro', 'dezembro']

// patient.birth_date arrives as ISO from the API, but App's normalizeSavedReport rewrites it with
// formatBirthDate ("16 de agosto de 2000"); both are unambiguous, anything else is unknown.
function parseBirthDate(value: unknown) {
  const raw = typeof value === 'string' ? value.trim() : ''
  const iso = raw.match(/^(\d{4})-(\d{2})-(\d{2})$/)
  if (iso) return validIso(+iso[1], +iso[2], +iso[3])
  const long = raw.match(/^(\d{1,2}) de ([a-zç]+) de (\d{4})$/i)
  const month = long ? MONTHS.indexOf(long[2].toLowerCase()) + 1 : 0
  return long && month > 0 ? validIso(+long[3], month, +long[1]) : null
}

// Pure string parsing (no Date objects) so the result never shifts with the timezone.
function parseExamDate(value: unknown, order: unknown) {
  const raw = typeof value === 'string' ? value.trim() : ''
  const iso = raw.match(/^(\d{4})-(\d{2})-(\d{2})(?:[\sT]|$)/)
  if (iso) return validIso(Number(iso[1]), Number(iso[2]), Number(iso[3]))
  const m = raw.match(/^(\d{1,2})[/-](\d{1,2})[/-](\d{4})(?:\s|$)/)
  if (!m) return null
  const a = Number(m[1])
  const b = Number(m[2])
  const y = Number(m[3])
  if (order === 'DMY') return validIso(y, b, a)
  if (order === 'MDY') return validIso(y, a, b)
  if (a > 12) return validIso(y, b, a)
  if (b > 12) return validIso(y, a, b)
  return a === b ? validIso(y, b, a) : null // ambiguous without date_order: unknown
}

function pentacamDates(analysis: any): string[] {
  const src = analysis?.exams?.pentacam_corneal_tomography?.source
  const sources: string[] = Array.isArray(src) ? src : typeof src === 'string' ? [src] : []
  const entries = Array.isArray(analysis?.verificacao_identidade) ? analysis.verificacao_identidade : []
  const dates = entries
    .filter((e: any) => sources.includes(e?.source) && String(e?.timestamp_lido ?? '').trim() !== '') // empty = date not read, not a conflict
    .map((e: any) => parseExamDate(e?.timestamp_lido, e?.date_order))
  // eyes.<OD|OS>.exam_at: ISO datetime; parseExamDate keeps the printed calendar date (no tz conversion)
  const eyes = analysis?.exams?.pentacam_corneal_tomography?.eyes
  for (const eye of ['OD', 'OS']) {
    const at = eyes?.[eye]?.exam_at
    if (String(at ?? '').trim() !== '') dates.push(parseExamDate(at, undefined))
  }
  // one unparseable Pentacam date makes the reference unknown, never silently skipped
  if (dates.length === 0 || dates.includes(null)) return []
  return [...new Set<string>(dates)].sort()
}

const DASH = /^\s*[-–—]+\s*$/
const CLASSIFIED_TKC = /^\s*(poss|kc\s*[1-4])/i // rule 2.1: Poss., KC1..KC4

export function keratoconusStatus(tkcValues: unknown[]): 'classified' | 'not_classified' | 'unknown' {
  const v = tkcValues.map((x) => (typeof x === 'string' ? x : null))
  if (v.some((x) => x !== null && CLASSIFIED_TKC.test(x))) return 'classified'
  return v.length > 0 && v.every((x) => x !== null && DASH.test(x)) ? 'not_classified' : 'unknown'
}

type TkcStatus = 'classified' | 'not_classified' | 'unknown'

/**
 * Per-eye TKC (rule 2.1) from the saved analysis. The backend stores a printed dash as tkc=null plus an
 * extraction diagnostic {field:'tkc', status:'not_applicable', stage:'recognition'}; any other null
 * (missing / not read) or an unrecognised value is unknown, never a negative finding (protocol section 0, item 52).
 */
export function keratoconusForAnalysis(analysis: unknown): { status: TkcStatus; eyes: Record<'OD' | 'OS', TkcStatus> } {
  const a = analysis as any
  const diagnostics: any[] = Array.isArray(a?.extraction_diagnostics) ? a.extraction_diagnostics : []
  const eyeStatus = (eye: 'OD' | 'OS'): TkcStatus => {
    const value = a?.exams?.pentacam_corneal_tomography?.eyes?.[eye]?.topometric_indices_8mm?.tkc
    if (typeof value === 'string' && value.trim() !== '') return keratoconusStatus([value])
    const printedDash = diagnostics.some((d) => d?.exam_type === 'pentacam_corneal_tomography' && d?.eye === eye &&
      d?.field === 'tkc' && d?.status === 'not_applicable' && d?.stage === 'recognition')
    return printedDash ? 'not_classified' : 'unknown'
  }
  const eyes = { OD: eyeStatus('OD'), OS: eyeStatus('OS') }
  const all = [eyes.OD, eyes.OS]
  const status: TkcStatus = all.includes('classified') ? 'classified' : all.every((s) => s === 'not_classified') ? 'not_classified' : 'unknown'
  return { status, eyes }
}

/** ISO date of the Pentacam exam, or null when absent, unparseable or conflicting. */
export function pentacamReferenceDate(analysis: unknown): string | null {
  const dates = pentacamDates(analysis)
  return dates.length === 1 ? dates[0] : null
}

export function clinicalFlowForAnalysis(analysis: unknown) {
  const birthIso = parseBirthDate((analysis as any)?.patient?.birth_date)
  const dates = birthIso ? pentacamDates(analysis) : []
  const ages = dates.map((d) => calculateAge(birthIso, d))
  const age = ages.length > 0 && ages.every((a) => a !== null && a === ages[0]) ? ages[0] : null
  return { age, flow: selectClinicalFlow(age), referenceDate: age === null ? null : dates[0] }
}

/**
 * Protocol item 35 + 45(g): another device's birth date that differs from the Pentacam's only by a
 * day/month swap (2- and 4-digit years equivalent) is the same patient. The flow stays on the Pentacam
 * reading; when the swapped reading would give another flow, both readings are reported (never blocking).
 */
export function birthDateInversion(analysis: unknown) {
  const a = analysis as any
  const { age, flow, referenceDate } = clinicalFlowForAnalysis(a)
  const birth = parseBirthDate(a?.patient?.birth_date)
  if (!birth) return null
  const [by, bm, bd] = [+birth.slice(0, 4), +birth.slice(5, 7), +birth.slice(8)]
  const src = a?.exams?.pentacam_corneal_tomography?.source
  const sources: string[] = Array.isArray(src) ? src : typeof src === 'string' ? [src] : []
  const entries = Array.isArray(a?.verificacao_identidade) ? a.verificacao_identidade : []
  const inverted = entries.some((e: any) => {
    if (sources.includes(e?.source)) return false
    const raw = String(e?.nascimento_lido ?? '').trim()
    let d = 0, m = 0, y = 0
    const iso = raw.match(/^(\d{4})-(\d{2})-(\d{2})$/)
    const slash = raw.match(/^(\d{1,2})[/-](\d{1,2})[/-](\d{2}|\d{4})$/)
    if (iso) [y, m, d] = [+iso[1], +iso[2], +iso[3]]
    else if (slash && e?.date_order === 'DMY') [d, m, y] = [+slash[1], +slash[2], +slash[3]]
    else if (slash && e?.date_order === 'MDY') [m, d, y] = [+slash[1], +slash[2], +slash[3]]
    else return false // order unknown: no inference
    const sameYear = y === by || (y < 100 && y === by % 100)
    return sameYear && bm !== bd && d === bm && m === bd
  })
  const swapped = validIso(by, bd, bm)
  if (!inverted || !swapped) return null
  const otherAge = referenceDate ? calculateAge(swapped, referenceDate) : null
  const otherFlow = selectClinicalFlow(otherAge)
  return { pentacamReading: birth, otherReading: swapped, age, flow, otherAge, otherFlow, changesFlow: flow !== null && otherFlow !== null && otherFlow !== flow }
}

export function flowLabel(flow: ClinicalFlow | null, age: number | null) {
  if (flow === 'A') return `Fluxo A — ${MIN_PROTOCOL_AGE} a ${FLOW_B_MIN_AGE - 1} anos (${age} anos)`
  if (flow === 'B') return `Fluxo B — ${FLOW_B_MIN_AGE} a ${FLOW_C_MIN_AGE - 1} anos (${age} anos)`
  if (flow === 'C') return `Fluxo C — ${FLOW_C_MIN_AGE} anos ou mais (${age} anos)`
  if (flow === 'out_of_scope') return `Fora do escopo do protocolo (menor de ${MIN_PROTOCOL_AGE} anos)`
  return 'não informado'
}
