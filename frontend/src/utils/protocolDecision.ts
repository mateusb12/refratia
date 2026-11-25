import { clinicalFlowForAnalysis, keratoconusForAnalysis, type ClinicalFlow } from './clinicalFlow.ts'

// Protocolo_RefratIA_FINAL: the only place the Flow C thresholds live (p.5 §0.D(4), p.19 §7).
export const ENDOTHELIAL_MIN_CELLS_MM2 = 2000
export const TORIC_MIN_ASTIGMATISM_D = 0.75
export const EXAM_WINDOW_MONTHS = 4 // §0.C, item 40

// Flows A/B parameters (§2 p.8-11, §3 p.13-14). Every value is the protocol's; none is tuned here.
export const LASER = {
  ler: 250, pta: 40, kMin: 34, kMax: 48, // LER ≥ 250 µm, PTA ≤ 40 %, 34 ≤ K_final ≤ 48 D
  flapUm: 90, dkLasik: 0.839, dkPrk: 0.775, munnerlynK: 1.275, oz: 6.5, ozReduced: 6.0, vertexM: 0.012,
  phakicSE: 12, hyperopiaReferralSE: 6, acdMin: 3.0, nightSE: 8, nightK: 35, residualCyl: 0.75, presbyAddD: 2,
} as const

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
  ambulatory: 'Encaminhamento ao ambulatório do cirurgião',
  presbyOutOfRange: 'Fora da faixa de literatura para Presbyond/PresbyLASIK',
  ring: 'Anel estromal + PRK',
} as const
// §8 closed vocabulary; several options in one eye are joined with "ou".
export const OPTION = { lasik: 'LASIK', prk: 'PRK', phakic: 'Lente fácica', bioptics: 'Lente fácica combinada com PRK (bioptics)', phakicToric: 'Lente fácica tórica', presbyond: 'Presbyond', customQ: 'Custom-Q/PresbyLASIK' } as const
const NON_SURGICAL = new Set<string>([CONDUTA.pending, CONDUTA.keratoconus, CONDUTA.outOfScope, CONDUTA.ambulatory, CONDUTA.presbyOutOfRange])

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
  /** Flow A/B: each checkpoint evaluated for this eye — rule, measurement used and outcome */
  steps: Step[]
  certainty: 'definitiva' | 'condicional' | 'pendente' | null
}

export interface Step { rule: string; text: string; outcome: 'ok' | 'fail' | 'pending' | 'info' }

export type DecisionState =
  | 'no_flow' // age unknown
  | 'out_of_scope'
  | 'keratoconus' // rule 2.1 fired
  | 'screening_incomplete' // TKC not read in at least one eye, none classified
  | 'evaluated' // flow cascade evaluated per eye

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
    return { conduta: CONDUTA.pending, tone: 'yellow', astigmatism, endothelium: PENDING_CRITERION, toric: null, missing, observations, steps: [], certainty: null }
  }
  const toric = astigmatism.value === null ? null : astigmatism.value >= TORIC_MIN_ASTIGMATISM_D
  if (endothelium.status === 'pendente') {
    missing.push('contagem endotelial')
    observations.push({ type: 'exame_pendente', text: `Necessidade de repetir ou realizar o exame (microscopia especular) antes de seguir o fluxo (${endothelium.text.replace('não avaliado — ', '')}).` })
    return { conduta: CONDUTA.pending, tone: 'yellow', astigmatism, endothelium, toric, missing, observations, steps: [], certainty: null }
  }
  if (endothelium.status === 'condiciona_conduta') {
    observations.push({ type: 'endotelio_baixo', text: ENDOTELIO_BAIXO })
    const conduta = toric === null ? CONDUTA.conditionedGeneric : toric ? CONDUTA.conditionedToric : CONDUTA.conditioned
    return { conduta, tone: 'yellow', astigmatism, endothelium, toric, missing, observations, steps: [], certainty: null }
  }
  const conduta = toric === null ? CONDUTA.readyGeneric : toric ? CONDUTA.readyToric : CONDUTA.ready
  // §8.1: green = conduta pronta; the generic indication is the protocol's ready conduct when astigmatism is absent.
  return { conduta, tone: 'green', astigmatism, endothelium, toric, missing, observations, steps: [], certainty: null }
}


// ---------------------------------------------------------------- Flows A and B (§5 p.14-17, §6 p.17-19)

type Obs = { type: string; text: string }
const signed = (v: number) => (v > 0 ? '+' : v < 0 ? '−' : '') + fmt(Math.abs(v))
const join = (...options: string[]) => options.join(' ou ')

/** Refraction of one eye in negative-cylinder notation (§3); cylinder absent = 0,00 D (§1 note). */
function refractionOf(analysis: any, eye: Eye, reference: string | null) {
  const exam = analysis?.exams?.refractometry
  const data = exam?.eyes?.[eye] ?? exam?.eyes?.AO?.[eye] ?? null
  let sph = num(data?.sphere_d ?? data?.refraction?.sphere_d)
  let cyl = num(data?.cylinder_d ?? data?.refraction?.cylinder_d) ?? 0
  const source = sourcesOf(exam)[0] ?? null
  if (sph === null) return { ok: false as const, text: exam && sourcesOf(exam).length > 0 ? 'refração deste olho não lida' : 'refratometria não enviada' }
  const window = checkWindow(examDates(analysis, exam, data, ['exam_datetime']), reference)
  if (!window.ok) {
    return { ok: false as const, text: window.reason === 'outside' ? `refratometria de ${window.date}, mais de ${EXAM_WINDOW_MONTHS} meses do Pentacam` : `data da refratometria não lida; janela de ${EXAM_WINDOW_MONTHS} meses não verificável` }
  }
  if (cyl > 0) [sph, cyl] = [sph + cyl, -cyl]
  return { ok: true as const, sph, cyl, se: sph + cyl / 2, source }
}

function firstNum(obj: any, paths: string[][]) {
  for (const path of paths) {
    const v = num(path.reduce((o, k) => o?.[k], obj))
    if (v !== null) return v
  }
  return null
}
const pentacamEye = (analysis: any, eye: Eye) => analysis?.exams?.pentacam_corneal_tomography?.eyes?.[eye] ?? null
const kmOf = (p: any) => firstNum(p, [['anterior_cornea', 'km_d'], ['keratometry', 'mean_k_d'], ['general', 'km_d']])
const thinnestOf = (p: any) => firstNum(p, [['pachymetry', 'thinnest_um'], ['pachymetry', 'point_and_finest_um'], ['general', 'thinnest_pachy_um'], ['general', 'pachymetry_thinnest_um']])
// §2: ACD always from the Pentacam (Cataract Pre-OP, Int.), never the biometer.
const acdOf = (p: any) => firstNum(p, [['cataract_preop', 'acd_mm'], ['cataract_pre_op', 'acd_mm'], ['anterior_segment', 'internal_anterior_chamber_depth_mm']])

const toCornea = (f: number) => f / (1 - LASER.vertexM * f) // §3, item 59
const perD = (oz: number) => ((oz * oz) / 3) * LASER.munnerlynK // 17,96 µm/D at 6,5 mm; 15,30 at 6,0 (item 60)
const inK = (k: number) => k >= LASER.kMin && k <= LASER.kMax
const kFinal = (km: number, se: number, factor: number) => (se < 0 ? km - factor * Math.abs(se) : km + factor * Math.abs(se))

/** LER/PTA of one technique, with the 6,0 mm optical-zone recalculation when PTA alone fails (Etapas 4/5). */
function tissue(pachy: number, diopters: number, flap: number) {
  const at = (oz: number) => { const abl = perD(oz) * diopters; return { oz, abl, ler: pachy - flap - abl, pta: ((flap + abl) / pachy) * 100 } }
  let t = at(LASER.oz)
  if (t.ler >= LASER.ler && t.pta > LASER.pta) { const reduced = at(LASER.ozReduced); if (reduced.pta <= LASER.pta) t = reduced }
  return { ...t, lerOk: t.ler >= LASER.ler, ptaOk: t.pta <= LASER.pta }
}
const tissueText = (name: string, t: ReturnType<typeof tissue>, k: number) =>
  `${name}: LER ${fmt(t.ler, 0)} µm ${t.lerOk ? '≥' : '<'} 250 · PTA ${fmt(t.pta, 1)} % ${t.ptaOk ? '≤' : '>'} 40${t.oz === LASER.ozReduced ? ' (zona óptica reduzida a 6,0 mm)' : ''} · K_final ${fmt(k)} D ${inK(k) ? 'dentro de' : 'fora de'} 34–48`

const EXAME_PENDENTE = (what: string) => ({ type: 'exame_pendente', text: `Necessidade de repetir ou realizar o exame (${what}) antes de seguir o fluxo.` })

function abEye(conduta: string, steps: Step[], missing: string[] = [], observations: Obs[] = []): EyeDecision {
  return { conduta, tone: NON_SURGICAL.has(conduta) ? 'yellow' : 'green', astigmatism: PENDING_CRITERION, endothelium: PENDING_CRITERION, toric: null, missing, observations, steps, certainty: null }
}
function pendingEye(steps: Step[], rule: string, what: string, missingItem: string, observations: Obs[] = []) {
  steps.push({ rule, text: `${what} → cascata deste olho para aqui`, outcome: 'pending' })
  return abEye(CONDUTA.pending, steps, [missingItem], [...observations, EXAME_PENDENTE(what)])
}

/** risco_ectasia_nao_confirmado (Etapa 3; §2: BAD-D ≥ 1,6 suspeito, ARTmax ≤ 405 µm zona cinzenta). Never changes the conduct. */
function ectasiaObservation(p: any): Obs | null {
  const bad = firstNum(p, [['belin_ambrosio', 'd'], ['ectasia_reforcada_belin_ambrosio', 'd']])
  const art = firstNum(p, [['belin_ambrosio', 'art_max'], ['ectasia_reforcada_belin_ambrosio', 'art_max']])
  const altered = [bad !== null && bad >= 1.6 ? `BAD-D ${fmt(bad)}` : null, art !== null && art <= 405 ? `ARTmax ${art} µm` : null].filter(Boolean)
  if (altered.length === 0) return null
  return { type: 'risco_ectasia_nao_confirmado', text: `Índices de superfície da córnea (${altered.join(' / ')}) acima da faixa de referência. Achado inespecífico, que pode decorrer de astigmatismo mais elevado, do posicionamento do olho durante o exame, de alterações do filme lacrimal, como olho seco ou piscada incompleta, do uso recente de lente de contato ou de alteração inicial da curvatura corneana. Não há diagnóstico de ceratocone neste exame. Recomenda-se acompanhamento tomográfico; a interpretação cabe ao médico cirurgião. A topografia anormal é o fator de risco isolado mais importante para ectasia após cirurgia refrativa a laser, segundo a Refractive Surgery Preferred Practice Pattern da Academia Americana de Oftalmologia.` }
}

/** Residual astigmatism PRK after a phakic lens (§3, items 49/50/58): same cuts as Etapas 4/5. */
function residualSupported(pachy: number, km: number, cyl: number) {
  const t = tissue(pachy, Math.abs(cyl), 0)
  // §3 writes K_final_residual = Km − 0,775 × (cilindro/2). With negative cylinder that literally steepens (Km + …), while the
  // general rule (myopic SE → Km − ΔK) flattens. The PDF supports both readings (adjudication item), so bioptics is offered
  // only when the residual K_final is inside 34–48 under both; otherwise only the toric lens (always an option) remains.
  const literal = km - LASER.dkPrk * (cyl / 2)
  const myopic = km - LASER.dkPrk * Math.abs(cyl / 2)
  const kOk = inK(literal) && inK(myopic)
  return {
    ok: t.lerOk && t.ptaOk && kOk,
    text: `${tissueText('PRK do resíduo cilíndrico', t, literal)}${literal !== myopic ? ` (leitura literal da fórmula; pela regra miópica, ${fmt(myopic)} D ${inK(myopic) ? 'dentro' : 'fora'})` : ''}`,
  }
}

/** Etapa 6 (and Etapa 7 fallback) of Flow A. */
function phakicStep(analysis: any, eye: Eye, reference: string | null, refr: { cyl: number }, steps: Step[], observations: Obs[]): EyeDecision {
  const p = pentacamEye(analysis, eye)
  const acd = acdOf(p)
  const endo = endotheliumCriterion(analysis, eye, reference)
  const acdText = acd === null ? 'ACD ausente' : `ACD ${fmt(acd)} mm ${acd >= LASER.acdMin ? '≥' : '<'} 3,00`
  if (acd === null || endo.status === 'pendente') {
    const what = [acd === null ? 'ACD do Pentacam (Cataract Pre-OP)' : null, endo.status === 'pendente' ? `microscopia especular — ${endo.text.replace('não avaliado — ', '')}` : null].filter(Boolean).join('; ')
    return pendingEye(steps, 'Etapa 6 · lente fácica', `${acdText} · ${endo.status === 'pendente' ? 'contagem endotelial pendente' : endo.text}: ${what}`, acd === null ? 'ACD (Pentacam)' : 'contagem endotelial', observations)
  }
  if (acd < LASER.acdMin || endo.status === 'condiciona_conduta') {
    steps.push({ rule: 'Etapa 6 · lente fácica', text: `${acdText} · endotélio ${endo.text} → critérios anatômicos não atendidos`, outcome: 'fail' })
    steps.push({ rule: 'Etapa 7 · fallback final', text: 'Anel estromal + PRK (cirurgia combinada)', outcome: 'ok' })
    return abEye(CONDUTA.ring, steps, [], observations)
  }
  steps.push({ rule: 'Etapa 6 · lente fácica', text: `${acdText} · endotélio ${endo.text} → lente fácica indicada`, outcome: 'ok' })
  if (Math.abs(refr.cyl) < LASER.residualCyl) return abEye(OPTION.phakic, steps, [], observations)
  const km = kmOf(p), pachy = thinnestOf(p)
  if (km === null || pachy === null) return pendingEye(steps, 'Etapa 6 · resíduo cilíndrico', 'Km/paquimetria do Pentacam ausentes para o cálculo residual', 'Km/paquimetria (Pentacam)', observations)
  const res = residualSupported(pachy, km, refr.cyl)
  steps.push({ rule: 'Etapa 6 · cilindro ≥ 0,75 D', text: `cilindro ${signed(refr.cyl)} D · ${res.text} → ${res.ok ? 'córnea suporta o resíduo' : 'córnea não suporta o resíduo'}`, outcome: res.ok ? 'ok' : 'fail' })
  return abEye(res.ok ? join(OPTION.bioptics, OPTION.phakicToric) : OPTION.phakicToric, steps, [], observations)
}

const QVN = (se: number, k: number, acd: number | null, endo: Criterion, verdict: string) =>
  `Correção miópica elevada (SE ${signed(se)} D) e/ou ceratometria final estimada baixa (${fmt(k)} D). Ablações corneanas induzem aberração esférica, e a magnitude induzida cresce com a correção: em série de olhos medidos por traçado de raios, a aberração esférica aumentou em média quase quatro vezes numa pupila de 6,5 mm, e a correlação com a dioptria programada no laser foi de 0,88. Isso pode reduzir a qualidade visual em baixa luminosidade (halos, ofuscamento, menor sensibilidade ao contraste), sobretudo com pupila escotópica ampla. Critérios anatômicos para lente fácica: ACD ${acd === null ? 'não informado' : fmt(acd) + ' mm'} e contagem endotelial ${endo.value === null ? 'não informada' : endo.value.toLocaleString('pt-BR') + ' céls/mm²'} — ${verdict}. A escolha entre a cirurgia a laser e a lente fácica fica a critério do cirurgião.`

function flowAEye(analysis: any, eye: Eye, reference: string | null): EyeDecision {
  const steps: Step[] = []
  const p = pentacamEye(analysis, eye)
  const ectasia = ectasiaObservation(p)
  const observations: Obs[] = ectasia ? [ectasia] : []
  const refr = refractionOf(analysis, eye, reference)
  if (!refr.ok) return pendingEye(steps, 'Etapa 1 · faixa dióptrica', refr.text, 'refração', observations)
  const { sph, cyl, se } = refr
  const seText = `SE ${signed(se)} D (esfera ${signed(sph)}, cilindro ${signed(cyl)}; ${refr.source ?? 'refratometria'})`
  if (se > LASER.hyperopiaReferralSE) {
    steps.push({ rule: 'Etapa 1 · hipermetropia > +6,00 D', text: `${seText} → fora da indicação automática`, outcome: 'fail' })
    return abEye(CONDUTA.ambulatory, steps, [], [...observations, { type: 'hipermetropia_alta', text: `Hipermetropia acima de 6,00 D (SE ${signed(se)} D): fora da faixa de indicação automática deste protocolo. Encaminhamento ao ambulatório do cirurgião para avaliação.` }])
  }
  if (Math.abs(se) >= LASER.phakicSE) {
    steps.push({ rule: 'Etapa 1 · faixa dióptrica', text: `${seText} → ≥ 12 D: candidato preferencial a lente fácica`, outcome: 'info' })
    return phakicStep(analysis, eye, reference, refr, steps, observations)
  }
  steps.push({ rule: 'Etapa 1 · faixa dióptrica', text: `${seText} → < 12 D: LASIK e PRK avaliados`, outcome: 'info' })
  const km = kmOf(p), pachy = thinnestOf(p)
  if (km === null || pachy === null) {
    const what = [km === null ? 'Km' : null, pachy === null ? 'paquimetria do ponto mais fino' : null].filter(Boolean).join(' e ')
    return pendingEye(steps, 'Etapas 4/5 · LASIK e PRK', `${what} do Pentacam ausente(s)`, `${what} (Pentacam)`, observations)
  }
  const diopters = Math.max(Math.abs(toCornea(sph)), Math.abs(toCornea(sph + cyl)))
  const lasik = tissue(pachy, diopters, LASER.flapUm), prk = tissue(pachy, diopters, 0)
  const kL = kFinal(km, se, LASER.dkLasik), kP = kFinal(km, se, LASER.dkPrk)
  const eyeK = inK(kL) || inK(kP) // item 69: K_final qualifies the eye, not the technique
  const lasikAlive = lasik.lerOk && lasik.ptaOk && eyeK
  const prkAlive = prk.lerOk && prk.ptaOk && eyeK
  steps.push({ rule: 'Etapa 4 · LASIK (flap 90 µm, ΔK 0,839)', text: `Km ${fmt(km)} D, paquimetria ${pachy} µm, ablação sobre ${fmt(diopters)} D (plano corneano) · ${tissueText('LASIK', lasik, kL)}${!inK(kL) && eyeK ? ' — K_final fora, mas a PRK está dentro: técnica mantida (item 69)' : ''} → ${lasikAlive ? 'indicado' : 'contraindicado'}`, outcome: lasikAlive ? 'ok' : 'fail' })
  steps.push({ rule: 'Etapa 5 · PRK (flap 0, ΔK 0,775)', text: `${tissueText('PRK', prk, kP)}${!inK(kP) && eyeK ? ' — K_final fora, mas o LASIK está dentro: técnica mantida (item 69)' : ''} → ${prkAlive ? 'indicado' : 'contraindicado'}`, outcome: prkAlive ? 'ok' : 'fail' })
  if (!lasikAlive && !prkAlive) return phakicStep(analysis, eye, reference, refr, steps, observations)
  const laser = [lasikAlive ? OPTION.lasik : null, prkAlive ? OPTION.prk : null].filter(Boolean) as string[]
  // Etapa 5A — night-vision alert (Flow A only, item 46)
  const lowestK = Math.min(...(lasikAlive ? [kL] : []), ...(prkAlive ? [kP] : []))
  if (se > -LASER.nightSE && lowestK >= LASER.nightK) return abEye(join(...laser), steps, [], observations)
  // §0.C: in Etapa 5A an exam outside the window is treated as absent and the laser conduct stands
  const acd = acdOf(p)
  const endo = endotheliumCriterion(analysis, eye, reference)
  const met = acd !== null && acd >= LASER.acdMin && endo.status === 'aprovado'
  const unknown = !met && (acd === null || endo.status === 'pendente') && !(acd !== null && acd < LASER.acdMin) && endo.status !== 'condiciona_conduta'
  const trigger = `${se <= -LASER.nightSE ? `SE ${signed(se)} D (≥ 8 D)` : ''}${se <= -LASER.nightSE && lowestK < LASER.nightK ? ' e ' : ''}${lowestK < LASER.nightK ? `K_final ${fmt(lowestK)} D < 35` : ''}`
  if (!met) {
    steps.push({ rule: 'Etapa 5A · qualidade visual noturna', text: `${trigger} → alerta; lente fácica ${unknown ? 'não avaliável (dado ausente)' : 'não atende aos critérios'} — conduta do laser mantida`, outcome: 'info' })
    return abEye(join(...laser), steps, [], [...observations, { type: 'qualidade_visual_noturna', text: QVN(se, lowestK, acd, endo, unknown ? 'não avaliáveis por dado ausente' : 'não atendidos') }])
  }
  const qvn = { type: 'qualidade_visual_noturna', text: QVN(se, lowestK, acd, endo, 'atendidos: a lente fácica passa a constar da conduta como alternativa') }
  steps.push({ rule: 'Etapa 5A · qualidade visual noturna', text: `${trigger} → alerta; ACD ${fmt(acd)} mm ≥ 3,00 e endotélio ${endo.text} → lente fácica como alternativa`, outcome: 'ok' })
  if (Math.abs(cyl) < LASER.residualCyl) return abEye(join(...laser, OPTION.phakic), steps, [], [...observations, qvn])
  const res = residualSupported(pachy, km, cyl)
  steps.push({ rule: 'Etapa 5A · cilindro ≥ 0,75 D (item 58)', text: `cilindro ${signed(cyl)} D · ${res.text} → ${res.ok ? 'córnea suporta o resíduo' : 'córnea não suporta o resíduo'}`, outcome: res.ok ? 'ok' : 'fail' })
  return abEye(res.ok ? join(...laser, OPTION.bioptics, OPTION.phakicToric) : join(...laser, OPTION.phakicToric), steps, [], [...observations, qvn])
}

const ENCAMINHAMENTO_PRESBIOPIA = 'A córnea não atende aos critérios deste protocolo para tratamento corneano de presbiopia (Presbyond ou Custom-Q/PresbyLASIK). O cirurgião pode considerar o implante de lente intraocular multifocal, que corrige a presbiopia, na avaliação presencial.'

function flowBEye(analysis: any, eye: Eye, reference: string | null): EyeDecision {
  const steps: Step[] = []
  const p = pentacamEye(analysis, eye)
  const ectasia = ectasiaObservation(p)
  const observations: Obs[] = ectasia ? [ectasia] : []
  const refr = refractionOf(analysis, eye, reference)
  if (!refr.ok) return pendingEye(steps, 'Fluxo B · faixa refracional', refr.text, 'refração', observations)
  const { sph, cyl, se } = refr
  const km = kmOf(p), pachy = thinnestOf(p)
  if (km === null || pachy === null) {
    const what = [km === null ? 'Km' : null, pachy === null ? 'paquimetria do ponto mais fino' : null].filter(Boolean).join(' e ')
    return pendingEye(steps, 'Fluxo B · Saída 1 (córnea apta)', `${what} do Pentacam ausente(s)`, `${what} (Pentacam)`, observations)
  }
  // §6 item 45 / §10 (m): + 2,00 D presbyopic near target on both eyes; flap 90 µm and ΔK 0,839 (flap ablations, §2)
  const diopters = Math.max(Math.abs(toCornea(sph)), Math.abs(toCornea(sph + cyl))) + LASER.presbyAddD
  const t = tissue(pachy, diopters, LASER.flapUm)
  const k = kFinal(km, se, LASER.dkLasik)
  const apt = t.lerOk && t.ptaOk && inK(k)
  steps.push({ rule: 'Fluxo B · córnea apta? (critérios da Etapa 4)', text: `SE ${signed(se)} D (esfera ${signed(sph)}, cilindro ${signed(cyl)}) · Km ${fmt(km)} D, paquimetria ${pachy} µm, ablação sobre ${fmt(diopters)} D (inclui +2,00 D do alvo de perto) · ${tissueText('perfil presbiópico', t, k)} → ${apt ? 'apta' : 'não apta'}`, outcome: apt ? 'ok' : 'fail' })
  if (!apt) return abEye(CONDUTA.ambulatory, steps, [], [...observations, { type: 'encaminhamento_ambulatorio_presbiopia', text: ENCAMINHAMENTO_PRESBIOPIA }])
  // Saída 1 ranges (§2, §6): Presbyond −9,00 to +5,75 D incl. emmetropia; Custom-Q +0,50 to +3,25, −3 to −7,
  // −0,50 to −6,00 with astigmatism up to 3,00 D, or emmetropia (plano).
  const presbyond = se >= -9 && se <= 5.75
  const customQ = (se >= 0.5 && se <= 3.25) || (se <= -3 && se >= -7) || (se <= -0.5 && se >= -6 && Math.abs(cyl) <= 3) || (sph === 0 && cyl === 0)
  const options = [presbyond ? OPTION.presbyond : null, customQ ? OPTION.customQ : null].filter(Boolean) as string[]
  steps.push({ rule: 'Fluxo B · Saída 1 (faixa refracional)', text: `SE ${signed(se)} D → Presbyond ${presbyond ? 'dentro' : 'fora'} de −9,00 a +5,75 D · Custom-Q/PresbyLASIK ${customQ ? 'dentro' : 'fora'} das faixas documentadas`, outcome: options.length ? 'ok' : 'fail' })
  return abEye(options.length ? join(...options) : CONDUTA.presbyOutOfRange, steps, [], observations)
}

function blankEye(conduta: string | null, tone: Tone, observations: { type: string; text: string }[] = []): EyeDecision {
  return { conduta, tone, astigmatism: PENDING_CRITERION, endothelium: PENDING_CRITERION, toric: null, missing: [], observations, steps: [], certainty: null }
}

/**
 * Pure protocol evaluation of a saved analysis. Rule 2.1 (p.12) runs for every flow; then the cascade of the patient's
 * flow runs per eye: Flow A (§5, p.14-17), Flow B (§6, p.17-19) or Flow C (§7, p.19-20).
 */
export function evaluateProtocol(analysis: unknown): ProtocolDecision {
  const a = analysis as any
  const { age, flow } = clinicalFlowForAnalysis(a)
  const tkc = keratoconusForAnalysis(a)
  const both = (make: () => EyeDecision) => ({ OD: make(), OS: make() })
  const result = (state: DecisionState, eyes: Record<Eye, EyeDecision>): ProtocolDecision => {
    for (const e of Object.values(eyes)) {
      e.certainty = e.conduta === null ? null : e.conduta === CONDUTA.pending ? 'pendente' : e.conduta.includes('a considerar após') ? 'condicional' : 'definitiva'
    }
    return { age, flow, state, tkc, eyes, showOrientationSentence: state === 'evaluated' && Object.values(eyes).some((e) => e.conduta !== null && !NON_SURGICAL.has(e.conduta)) }
  }

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
  const reference = windowReference(a)
  const evalEye = flow === 'A' ? flowAEye : flow === 'B' ? flowBEye : flowCEye
  return result('evaluated', { OD: evalEye(a, 'OD', reference), OS: evalEye(a, 'OS', reference) })
}
