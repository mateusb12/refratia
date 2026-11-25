import { eyeLabel, type IntakeAnalysis } from '../../shared/contracts/patient-analysis.ts'
import { formatBirthDate, formatDateLongPtBR } from '../../shared/date.ts'
import { birthDateInversion, clinicalFlowForAnalysis, flowLabel } from '../protocol/clinicalFlow.ts'
import type { Eye } from '../protocol/protocolDecision.ts'
import { normalizeSavedAnalysis } from './normalizeSavedAnalysis.ts'

export const BUNDLED_REPORT_ID = '1'

export type DataKind = 'Dado bruto' | 'Dado ausente'
export type Confidence = 'Consistente' | 'Não informado'

export type ReportData = typeof import('../../../data/paciente_compilado.json')

export function formatNumber(value: unknown, options?: Intl.NumberFormatOptions) {
  return typeof value === 'number' && Number.isFinite(value) ? value.toLocaleString('pt-BR', options) : 'não informado'
}

function hasReportValue(value: unknown) {
  if (typeof value === 'number') return Number.isFinite(value)
  if (typeof value === 'string') return value.trim() !== '' && value.trim().toLocaleLowerCase() !== 'não informado'
  return value !== null && value !== undefined
}

export function normalizeSavedReport(analysis: IntakeAnalysis): ReportData {
  const raw = normalizeSavedAnalysis(analysis) as any

  return {
    ...raw,
    patient: { ...raw.patient, birth_date: formatBirthDate(raw.patient?.birth_date) },
  } as ReportData
}

function formatRefractionValue(value: unknown) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 'não informado'
  const formatted = value.toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  return value > 0 ? `+${formatted}` : formatted
}

function examLabelFromSource(source: string) {
  const normalized = source.toUpperCase()
  if (normalized.includes('PENTACAM')) return 'Pentacam'
  if (normalized.includes('BIOMETRIA') || normalized.includes('EYESUITE')) return 'Biometria'
  if (normalized.includes('AUTO') || normalized.includes('REFRAT')) return 'Autorrefrator'
  if (normalized.includes('MICROSCOPIA') || normalized.includes('SPECULAR')) return 'Microscopia especular'
  if (normalized.includes('RETINA') || normalized.includes('OCT')) return 'Imagem de retina'
  return 'Documento'
}

export function reportHeaderSummary(data: ReportData) {
  const raw = data as any
  const verification = Array.isArray(raw.verificacao_identidade) ? raw.verificacao_identidade : []
  const timestamps = verification
    .map((entry: any) => ({ source: String(entry?.source ?? ''), date: formatDateLongPtBR(entry?.timestamp_lido) }))
    .filter((entry: { source: string; date: string }) => entry.date !== 'não informado')
  const { age, flow: clinicalFlow } = clinicalFlowForAnalysis(raw)
  const flow = flowLabel(clinicalFlow, age)
  const refraction = raw.exams?.refractometry?.eyes ?? {}
  const formatEyeRefraction = (eye: string) => {
    const value = refraction[eye]
    if (!value) return `${eye}: não informado`
    return `${eye} ${formatRefractionValue(value.sphere_d)} ${formatRefractionValue(value.cylinder_d)} × ${value.axis_deg ?? '—'}°`
  }
  const identityGaps = raw.extraction_notes?.fallback_gaps ?? []
  const identity = identityGaps.includes('patient_identity')
    ? 'Revisar identidade'
    : verification.length > 0 ? `Confirmada por ${verification.length} documento(s)` : 'Sem evidência documental'
  const inversion = birthDateInversion(raw)
  const identityLine = inversion && identity !== 'Revisar identidade'
    ? `${identity}. ${inversionText(inversion)}`
    : identity
  const uniqueDates = [...new Set(timestamps.map((entry: { source: string; date: string }) => `${examLabelFromSource(entry.source)}: ${entry.date}`))]

  return {
    age: age === null ? 'não informado' : `${age} anos`,
    flow,
    dates: uniqueDates.length > 0
      ? uniqueDates.join('\n')
      : `Análise: ${formatDateLongPtBR(raw.generated_on)}`,
    refraction: `${formatEyeRefraction('OD')} · ${formatEyeRefraction('OS')}`,
    identity: identityLine,
  }
}

function inversionText(inversion: NonNullable<ReturnType<typeof birthDateInversion>>) {
  const base = 'Datas de nascimento divergem entre documentos por inversão dia/mês, compatível com diferença de formato entre equipamentos — tratadas como mesmo paciente. Conferir se necessário.'
  if (!inversion.changesFlow) return base
  return `${base} Leitura do Pentacam: ${formatBirthDate(inversion.pentacamReading)} → ${flowLabel(inversion.flow, inversion.age)}. Leitura invertida: ${formatBirthDate(inversion.otherReading)} → ${flowLabel(inversion.otherFlow, inversion.otherAge)}. Fluxo aplicado pela leitura do Pentacam (item 45g), sem bloqueio.`
}

function fileBaseName(value: unknown, fallback = 'Documento') {
  const path = typeof value === 'string' ? value : ''
  return path.split('/').pop() || fallback
}

export function getReportDocuments(data: ReportData): DocumentReview[] {
  return data.source_files.map((file) => {
    const signedURL = (file as typeof file & { signed_url?: string }).signed_url
    const exam = typeof file.exam === 'string' ? file.exam : ''
    return {
      name: exam ? `${exam.charAt(0).toUpperCase()}${exam.slice(1)}${file.eye ? ` · ${eyeLabel(file.eye as Eye)}` : ''}` : 'Documento',
      filename: fileBaseName(file.path),
      status: 'Processado',
      detail: `${file.type === 'application/pdf' ? `${file.pages} páginas` : 'Imagem'} disponível no caso.`,
      url: signedURL,
    }
  })
}

function datum(raw: unknown, row: Omit<ExtractedDatum, 'value' | 'kind' | 'confidence'>): ExtractedDatum {
  const present = hasReportValue(raw)
  return { ...row, value: formatNumber(raw), kind: present ? 'Dado bruto' : 'Dado ausente', confidence: present ? 'Consistente' : 'Não informado' }
}

export function getReportExtractedData(data: ReportData): ExtractedDatum[] {
  return (['OD', 'OS'] as const).flatMap((eye) => {
    const pentacam = data.exams.pentacam_corneal_tomography.eyes[eye]
    const biometry = data.exams.iol_calculation.eyes[eye]
    const microscopy = data.exams.specular_microscopy.eyes[eye]
    const label = eyeLabel(eye)
    const source = `Pentacam ${label}`
    const document = fileBaseName(pentacam.source_file, source)

    return [
      datum(pentacam.pachymetry.thinnest_um, { name: `Paquimetria mínima · ${label}`, unit: 'µm', source, document, screen: 'Pachymetry', field: 'Thinnest' }),
      datum(pentacam.anterior_cornea.kmax_d, { name: `Kmax · ${label}`, fullName: 'Ceratometria máxima', unit: 'D', source, document, screen: 'Topometric', field: 'Kmax' }),
      datum(pentacam.belin_ambrosio.d, { name: `BAD-D · ${label}`, fullName: 'Belin/Ambrósio Enhanced Ectasia Display', source, document, screen: 'Belin/Ambrósio', field: 'Final D' }),
      datum(pentacam.belin_ambrosio.art_max, { name: `ARTmax · ${label}`, fullName: 'Ambrósio Relational Thickness máximo', source, document, screen: 'Belin/Ambrósio', field: 'ARTmax' }),
      datum(microscopy.cell_density_cells_per_mm2, {
        name: `Celularidade endotelial · ${label}`,
        fullName: 'Densidade celular endotelial',
        unit: 'células/mm²',
        source: `Microscopia especular ${label}`,
        document: fileBaseName(data.exams.specular_microscopy.source[0], source),
        screen: 'Microscopia especular',
        field: 'Cell Density (CD)',
      }),
      datum(biometry.axial_length_mm, {
        name: `Comprimento axial · ${label}`,
        unit: 'mm',
        source: `Biometria ${label}`,
        document: fileBaseName(data.exams.iol_calculation.source[0], `Biometria ${label}`),
        screen: 'Biometria',
        field: 'Axial length',
      }),
    ]
  })
}

export function getReportMetrics(data: ReportData, extractedData: ExtractedDatum[]): Metric[] {
  return [
  {
    label: 'Arquivos recebidos',
    value: String(data.source_files.length),
    detail: 'Todos disponíveis para revisão',
    tone: 'success',
  },
  {
    label: 'Dados em destaque',
    value: String(extractedData.length),
    detail: 'Parâmetros reais de OD e OE',
    tone: 'success',
  },
  {
    label: 'Consolidação',
    value: 'Revisar',
    detail: 'Datas e identidade entre documentos não são conferidas automaticamente nesta tela',
    tone: 'warning',
  },
  ]
}

export interface Metric {
  label: string
  value: string
  detail: string
  tone: 'default' | 'success' | 'warning'
}

export interface DocumentReview {
  name: string
  filename: string
  status: 'Processado' | 'Atenção' | 'Não enviado'
  detail: string
  url?: string
}

export interface ExtractedDatum {
  name: string
  fullName?: string
  value: string
  unit?: string
  source: string
  kind: DataKind
  confidence: Confidence
  document: string
  screen: string
  field: string
}
