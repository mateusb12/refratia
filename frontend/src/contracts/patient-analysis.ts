export type Eye = 'OD' | 'OS' | 'AO'

export type ExamKey =
  | 'fundus_retinography'
  | 'iol_calculation'
  | 'pentacam_corneal_tomography'
  | 'specular_microscopy'

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

export interface IntakePreview {
  intakeId: string
  files: Array<{ filename: string; contentType: string; size: number; sha256: string }>
  analysis: IntakeAnalysis
  message: string
}

export function isIntakePreview(value: unknown): value is IntakePreview {
  if (!value || typeof value !== 'object') return false
  const result = value as Partial<IntakePreview>
  return typeof result.intakeId === 'string' && Array.isArray(result.files) && typeof result.analysis === 'object' && result.analysis !== null && typeof result.message === 'string'
}
