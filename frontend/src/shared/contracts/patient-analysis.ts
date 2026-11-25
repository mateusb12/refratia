export type Eye = 'OD' | 'OS' | 'AO'

export const eyeLabel = (eye: string) => (eye === 'OS' ? 'OE' : eye)

export type ExamKey =
  | 'fundus_retinography'
  | 'iol_calculation'
  | 'refractometry'
  | 'oct_retina'
  | 'pentacam_corneal_tomography'
  | 'specular_microscopy'

export const examLabels: Record<string, string> = {
  fundus_retinography: 'Retinografia de fundo de olho',
  iol_calculation: 'Cálculo de lente intraocular',
  refractometry: 'Refratometria',
  oct_retina: 'OCT de retina',
  pentacam_corneal_tomography: 'Tomografia corneana Pentacam',
  specular_microscopy: 'Microscopia especular',
}

export interface ExamPayload {
  source: string[]
  eyes?: Partial<Record<Eye, Record<string, unknown>>>
  [key: string]: unknown
}

export interface PatientInfo {
  full_name?: string
  birth_date?: string
  [key: string]: unknown
}

export interface IntakeAnalysis {
  schema_version: string
  generated_on: string
  language: string
  patient: PatientInfo
  facility?: Record<string, unknown>
  conventions?: Record<string, unknown>
  source_files: Array<Record<string, unknown>>
  exams: Partial<Record<ExamKey, ExamPayload>>
  extraction_notes?: Record<string, unknown>
}

export type PatientMatch =
  | { status: 'new' }
  | { status: 'unresolved' }
  | {
      status: 'existing'
      caseId: string
      patientName?: string
    }

export type PatientChangeKind = 'added' | 'changed' | 'removed' | 'unchanged'

export interface PatientChangeRow {
  exam: string
  eye?: string
  field: string
  before: unknown
  after: unknown
  kind: PatientChangeKind
}

export interface PatientChangePreview {
  added: number
  changed: number
  removed: number
  unchanged: number
  rows: PatientChangeRow[]
}

export interface IntakePreview {
  intakeId: string
  files: Array<{ filename: string; originalFilename?: string; canonicalFilename?: string; contentType: string; size: number; sha256: string; signed_url?: string }>
  analysis: IntakeAnalysis
  patientMatch: PatientMatch
  changePreview: PatientChangePreview | null
  message: string
}

export function isIntakePreview(value: unknown): value is IntakePreview {
  if (!value || typeof value !== 'object') return false
  const result = value as Partial<IntakePreview>
  return typeof result.intakeId === 'string' && Array.isArray(result.files) && typeof result.analysis === 'object' && result.analysis !== null && typeof result.message === 'string'
}
