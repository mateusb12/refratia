import { clinicalFlowForAnalysis, keratoconusForAnalysis, type ClinicalFlow } from './clinicalFlow.ts'

// Protocolo_RefratIA_FINAL: the only place the Flow C thresholds live (p.5 §0.D(4), p.19 §7).
export const ENDOTHELIAL_MIN_CELLS_MM2 = 2000
export const TORIC_MIN_ASTIGMATISM_D = 0.75
export const EXAM_WINDOW_MONTHS = 4 // §0.C, item 40

export type Eye = 'OD' | 'OS'
export type Tone = 'green' | 'yellow' | 'red' | 'gray'

/** conduta_final vocabulary of §8 (p.20), plus two states the protocol leaves to the physician (see report). */
export const CONDUTA = {
  ready: 'LIO Multifocal ou EDOF',
  readyToric: 'LIO Multifocal Tórica ou EDOF Tórica',
  // §7: astigmatism absent -> "manter a indicação genérica de LIO MULTIFOCAL/EDOF"; not in the §8 enum (adjudication item)
  readyGeneric: 'LIO Multifocal/EDOF',
  conditioned: 'LIO Multifocal ou EDOF — a considerar após avaliação endotelial',
  conditionedToric: 'LIO Multifocal Tórica ou EDOF Tórica — a considerar após avaliação endotelial',
  conditionedGeneric: 'LIO Multifocal/EDOF — a considerar após avaliação endotelial',
  pending: 'Pendente — necessidade de repetir ou realizar o exame',
  keratoconus: 'Encaminhamento a especialista em córnea',
  outOfScope: 'Fora do escopo do protocolo (menor de 18 anos)',
} as const

export type CriterionStatus = 'aprovado' | 'condiciona_conduta' | 'pendente' | 'ausente' | 'nao_avaliado'

export interface Criterion {
  value: number | null
  status: CriterionStatus
  /** human text for the table cell */
  text: string
  source: string | null
  examDate: string | null
}

export interface EyeDecision {
  conduta: string | null // null: no automatic conduct (flow not automated / screening incomplete)
  tone: Tone
  astigmatism: Criterion
  endothelium: Criterion
  toric: boolean | null // null: biometry astigmatism absent
  missing: string[] // dados_ausentes of this eye (no eye prefix; the screen adds the eye label)
  observations: { type: string; text: string }[]
}

export type DecisionState =
  | 'no_flow' // age unknown
  | 'out_of_scope'
  | 'keratoconus' // rule 2.1 fired
  | 'screening_incomplete' // TKC not read in at least one eye, none classified
  | 'manual' // Flow A/B: cascade not automated here
  | 'evaluated' // Flow C cascade evaluated per eye

export interface ProtocolDecision {
  age: number | null
  flow: ClinicalFlow | null
  state: DecisionState
  tkc: ReturnType<typeof keratoconusForAnalysis>
  eyes: Record<Eye, EyeDecision>
  /** §8.1 bloco 3: the fixed orientation sentence is shown only when at least one eye has an indicated surgery */
  showOrientationSentence: boolean
}

const PENDING_CRITERION: Criterion = { value: null, status: 'nao_avaliado', text: 'não avaliado', source: null, examDate: null }

const num = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : null)

function sourcesOf(exam: any): string[] {
  const src = exam?.source
  return Array.isArray(src) ? src.filter((s: unknown) => typeof s === 'string') : typeof src === 'string' ? [src] : []
}

const isLeap = (y: number) => (y % 4 === 0 && y % 100 !== 0) || y % 400 === 0
const daysIn = (y: number, m: number) => [31, isLeap(y) ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][m - 1]

/** ISO calendar date from an ISO date/datetime or an unambiguous DD/MM/YYYY-like string; null otherwise. */
function isoDate(value: unknown, order?: unknown): string | null {
  const raw = typeof value === 'string' ? value.trim() : ''
  const iso = raw.match(/^(\d{4})-(\d{2})-(\d{2})(?:[\sT]|$)/)
  let y = 0, m = 0, d = 0
  if (iso) [y, m, d] = [+iso[1], +iso[2], +iso[3]]
  else {
    const s = raw.match(/^(\d{1,2})[/-](\d{1,2})[/-](\d{4})(?:\s|$)/)
    if (!s) return null
    const [a, b] = [+s[1], +s[2]]
    y = +s[3]
    if (order === 'DMY' || (order !== 'MDY' && a > 12)) [d, m] = [a, b]
    else if (order === 'MDY' || b > 12) [m, d] = [a, b]
    else if (a === b) [d, m] = [a, b]
    else return null
  }
  if (y < 1000 || m < 1 || m > 12 || d < 1 || d > daysIn(y, m)) return null
  return `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
}

function addMonths(iso: string, months: number) {
  const [y, m, d] = iso.split('-').map(Number)
  const total = y * 12 + (m - 1) + months
  const ny = Math.floor(total / 12)
  const nm = (total % 12) + 1
  return `${ny}-${String(nm).padStart(2, '0')}-${String(Math.min(d, daysIn(ny, nm))).padStart(2, '0')}`
}

/** §0.C: more than 4 months apart from the Pentacam reference (most recent Pentacam date). */
export function outsideExamWindow(examIso: string, referenceIso: string) {
  return examIso < addMonths(referenceIso, -EXAM_WINDOW_MONTHS) || examIso > addMonths(referenceIso, EXAM_WINDOW_MONTHS)
}

/** Every distinct date this exam's documents carry; [] when none was read. */
function examDates(analysis: any, exam: any, eyeData: any, ownDateFields: string[]): (string | null)[] {
  const sources = sourcesOf(exam)
  const entries = Array.isArray(analysis?.verificacao_identidade) ? analysis.verificacao_identidade : []
  const dates: (string | null)[] = entries
    .filter((e: any) => sources.includes(e?.source) && String(e?.timestamp_lido ?? '').trim() !== '')
    .map((e: any) => isoDate(e.timestamp_lido, e.date_order))
  for (const f of ownDateFields) {
    const v = eyeData?.[f] ?? exam?.[f]
    if (String(v ?? '').trim() !== '') dates.push(isoDate(v))
  }
  return dates
}

/** Pentacam reference for the 4-month window: the most recent Pentacam date (§0.C); null when unknown. */
function windowReference(analysis: any): string | null {
  const exam = analysis?.exams?.pentacam_corneal_tomography
  const dates: (string | null)[] = examDates(analysis, exam, null, [])
  for (const eye of ['OD', 'OS']) {
    const at = exam?.eyes?.[eye]?.exam_at
    if (String(at ?? '').trim() !== '') dates.push(isoDate(at))
  }
  if (dates.length === 0 || dates.includes(null)) return null
  return (dates as string[]).sort().at(-1) ?? null
}

type WindowCheck = { ok: true; date: string } | { ok: false; reason: 'unread' | 'outside'; date: string | null }

function checkWindow(dates: (string | null)[], reference: string | null): WindowCheck {
  const read = [...new Set(dates)]
  if (read.length !== 1 || read[0] === null || reference === null) return { ok: false, reason: 'unread', date: null }
  return outsideExamWindow(read[0], reference) ? { ok: false, reason: 'outside', date: read[0] } : { ok: true, date: read[0] }
}

const fmt = (v: number, digits = 2) => v.toLocaleString('pt-BR', { minimumFractionDigits: digits, maximumFractionDigits: digits })

function biometryEye(exam: any, eye: Eye) {
  return exam?.eyes?.[eye] ?? exam?.eyes?.AO?.[eye] ?? null
}

/** Flow C astigmatism checkpoint (§7, p.19): biometry only, never the Pentacam. */
function astigmatismCriterion(analysis: any, eye: Eye, reference: string | null): Criterion {
  const exam = analysis?.exams?.iol_calculation
  const data = biometryEye(exam, eye)
  const source = sourcesOf(exam)[0] ?? null
  const value = num(data?.keratometry?.astigmatism_d)
  if (!data || value === null) return { value: null, status: 'ausente', text: 'dado ausente — astigmatismo de biometria não informado', source, examDate: null }
  const window = checkWindow(examDates(analysis, exam, data, ['measurement_date', 'calculated_at']), reference)
  if (!window.ok && window.reason === 'outside') {
    return { value, status: 'pendente', text: `não avaliado — exame pendente (biometria de ${window.date}, mais de ${EXAM_WINDOW_MONTHS} meses do Pentacam)`, source, examDate: window.date }
  }
  if (!window.ok) {
    return { value: null, status: 'ausente', text: `dado ausente — data da biometria não lida; janela de ${EXAM_WINDOW_MONTHS} meses não verificável`, source, examDate: null }
  }
  const magnitude = Math.abs(value)
  return {
    value: magnitude,
    status: 'aprovado',
    text: `${fmt(magnitude)} D ${magnitude >= TORIC_MIN_ASTIGMATISM_D ? '≥' : '<'} ${fmt(TORIC_MIN_ASTIGMATISM_D)} D → ${magnitude >= TORIC_MIN_ASTIGMATISM_D ? 'tórica' : 'não tórica'}`,
    source,
    examDate: window.date,
  }
}

/** Flow C endothelium checkpoint (§7, p.19-20): absent / unread / outside window -> exame pendente. */
function endotheliumCriterion(analysis: any, eye: Eye, reference: string | null): Criterion {
  const exam = analysis?.exams?.specular_microscopy
  const data = exam?.eyes?.[eye] ?? null
  const source = sourcesOf(exam)[0] ?? null
  const value = num(data?.cell_density_cells_per_mm2 ?? data?.cell_density_cells_mm2)
  if (value === null) {
    return { value: null, status: 'pendente', text: exam && sourcesOf(exam).length > 0 ? 'não avaliado — contagem endotelial não lida' : 'não avaliado — microscopia especular não enviada', source, examDate: null }
  }
  const window = checkWindow(examDates(analysis, exam, data, ['exam_at']), reference)
  if (!window.ok) {
    return {
      value,
      status: 'pendente',
      text: window.reason === 'outside'
        ? `não avaliado — microscopia de ${window.date}, mais de ${EXAM_WINDOW_MONTHS} meses do Pentacam`
        : `não avaliado — data da microscopia não lida; janela de ${EXAM_WINDOW_MONTHS} meses não verificável`,
      source,
      examDate: window.date,
    }
  }
  const below = value < ENDOTHELIAL_MIN_CELLS_MM2
  return {
    value,
    status: below ? 'condiciona_conduta' : 'aprovado',
    text: `${value.toLocaleString('pt-BR')} células/mm² ${below ? '<' : '≥'} ${ENDOTHELIAL_MIN_CELLS_MM2.toLocaleString('pt-BR')}`,
    source,
    examDate: window.date,
  }
}

const ENDOTELIO_BAIXO = 'Contagem endotelial abaixo de 2.000 células/mm². A indicação de lente intraocular multifocal ou de foco estendido (EDOF) fica condicionada a avaliação criteriosa do endotélio corneano pelo médico especialista. Dois fatores justificam essa cautela: o risco de descompensação endotelial no pós-operatório, e o fato de que lentes multifocais e de foco estendido reduzem a sensibilidade ao contraste por desenho óptico — efeito que pode somar-se a uma eventual perda de transparência da córnea. A decisão de prosseguir com esse perfil de lente é do médico cirurgião.'

function flowCEye(analysis: any, eye: Eye, reference: string | null): EyeDecision {
  const astigmatism = astigmatismCriterion(analysis, eye, reference)
  const endothelium = endotheliumCriterion(analysis, eye, reference)
  const missing: string[] = []
  const observations: { type: string; text: string }[] = []
  if (astigmatism.status === 'ausente') missing.push('astigmatismo da biometria óptica — escolha tórica/não tórica pendente')
  // §0.C: decisive data from an exam outside the window stops the eye's cascade at that checkpoint
  if (astigmatism.status === 'pendente') {
    missing.push(`biometria fora da janela de ${EXAM_WINDOW_MONTHS} meses`)
    observations.push({ type: 'exame_pendente', text: `Necessidade de repetir ou realizar o exame (biometria óptica) antes de seguir o fluxo (${astigmatism.text}).` })
    return { conduta: CONDUTA.pending, tone: 'yellow', astigmatism, endothelium: PENDING_CRITERION, toric: null, missing, observations }
  }
  const toric = astigmatism.value === null ? null : astigmatism.value >= TORIC_MIN_ASTIGMATISM_D
  if (endothelium.status === 'pendente') {
    missing.push('contagem endotelial')
    observations.push({ type: 'exame_pendente', text: `Necessidade de repetir ou realizar o exame (microscopia especular) antes de seguir o fluxo (${endothelium.text.replace('não avaliado — ', '')}).` })
    return { conduta: CONDUTA.pending, tone: 'yellow', astigmatism, endothelium, toric, missing, observations }
  }
  if (endothelium.status === 'condiciona_conduta') {
    observations.push({ type: 'endotelio_baixo', text: ENDOTELIO_BAIXO })
    const conduta = toric === null ? CONDUTA.conditionedGeneric : toric ? CONDUTA.conditionedToric : CONDUTA.conditioned
    return { conduta, tone: 'yellow', astigmatism, endothelium, toric, missing, observations }
  }
  const conduta = toric === null ? CONDUTA.readyGeneric : toric ? CONDUTA.readyToric : CONDUTA.ready
  // §8.1: green = conduta pronta; the generic indication is the protocol's ready conduct when astigmatism is absent.
  return { conduta, tone: 'green', astigmatism, endothelium, toric, missing, observations }
}

function blankEye(conduta: string | null, tone: Tone, observations: { type: string; text: string }[] = []): EyeDecision {
  return { conduta, tone, astigmatism: PENDING_CRITERION, endothelium: PENDING_CRITERION, toric: null, missing: [], observations }
}

/**
 * Pure protocol evaluation of a saved analysis. Rule 2.1 (p.12) runs for every flow; the Flow C cascade (§7, p.19-20)
 * runs per eye; Flow A/B cascades are not automated here and never produce a conduct.
 */
export function evaluateProtocol(analysis: unknown): ProtocolDecision {
  const a = analysis as any
  const { age, flow } = clinicalFlowForAnalysis(a)
  const tkc = keratoconusForAnalysis(a)
  const both = (make: () => EyeDecision) => ({ OD: make(), OS: make() })
  const result = (state: DecisionState, eyes: Record<Eye, EyeDecision>): ProtocolDecision => ({
    age, flow, state, tkc, eyes,
    showOrientationSentence: state === 'evaluated' && Object.values(eyes).some((e) => e.conduta !== null && e.conduta !== CONDUTA.pending),
  })

  if (flow === 'out_of_scope') return result('out_of_scope', both(() => blankEye(CONDUTA.outOfScope, 'yellow')))
  if (tkc.status === 'classified') {
    const tkcText = (eye: Eye) => String(a?.exams?.pentacam_corneal_tomography?.eyes?.[eye]?.topometric_indices_8mm?.tkc ?? '').trim()
    const affected = (['OD', 'OS'] as const).filter((eye) => tkc.eyes[eye] === 'classified')
    const staging = affected.map((eye) => `${tkcText(eye)} no olho ${eye}`).join('; ')
    const obs = [{ type: 'encaminhamento_ceratocone', text: `Encaminhamento a especialista em córnea — avaliação para implante de anel intraestromal, com ou sem combinação com outra técnica cirúrgica. TKC: ${staging}.` }]
    if (affected.length === 1) {
      obs.push({ type: 'ceratocone_unilateral_genetica', text: `Diagnóstico de ceratocone confirmado em apenas um dos olhos (${affected[0]}, ${tkcText(affected[0])}). Ainda assim, o paciente deve ser avaliado pelo médico especialista em córnea, pois o ceratocone tem componente genético e o olho contralateral pode desenvolver a doença. Uma cirurgia que viesse a ser indicada no olho aparentemente normal poderia induzir ectasia. A avaliação especializada considera o paciente como um todo, não apenas o olho já diagnosticado.` })
    }
    return result('keratoconus', both(() => blankEye(CONDUTA.keratoconus, 'red', obs)))
  }
  // Rule 2.1 is sovereign in any flow, so it fires even when the age (and so the flow) is unknown.
  if (flow === null) return result('no_flow', both(() => blankEye(null, 'yellow')))
  if (tkc.status === 'unknown') return result('screening_incomplete', both(() => blankEye(null, 'yellow')))
  if (flow !== 'C') return result('manual', both(() => blankEye(null, 'gray')))
  const reference = windowReference(a)
  return result('evaluated', { OD: flowCEye(a, 'OD', reference), OS: flowCEye(a, 'OS', reference) })
}
