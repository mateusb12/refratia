import type { IntakeExamType } from './IntakeFileClassification'

export interface FilenameClassification {
  examType: IntakeExamType
  eye: 'OD' | 'OS' | 'AO'
}

const examTypes: Record<string, IntakeExamType> = {
  PENTACAM: 'pentacam_corneal_tomography',
  BIOMETRIA: 'iol_calculation',
  AUTO: 'refractometry',
  EYESUITE: 'iol_calculation',
  MICROSCOPIA_ESPECULAR: 'specular_microscopy',
  MICROSCOPIA: 'specular_microscopy',
  RETINA: 'fundus_retinography',
  OCT: 'oct_retina',
}

export function classifyFilename(name: string): FilenameClassification | null {
  const stem = name.split(/[\\/]/).pop()?.replace(/\.[^.]+$/, '').trim().toUpperCase()
  if (!stem) return null

  const canonical = stem.split('__')
  const spaced = stem.match(/^([A-ZÀ-Ú_]+)\s+(OD|OE|OS|AO)(?:\s|$)/)
  const examLabel = canonical.length >= 2 ? canonical[0].trim() : spaced?.[1]
  const eyeLabel = canonical.length >= 2 ? canonical[1].trim() : spaced?.[2]
  const examType = examLabel ? examTypes[examLabel] : undefined

  if (!examType || !eyeLabel) return null

  return {
    examType,
    eye: eyeLabel === 'OE' ? 'OS' : eyeLabel as 'OD' | 'OS' | 'AO',
  }
}
