import type { IntakeExamType } from './IntakeFileClassification'

export interface FilenameClassification {
  examType: IntakeExamType
  eye: 'OD' | 'OS' | 'AO'
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
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
  const searchable = stem.replace(/[_-]+/g, ' ')
  const examLabel = canonical.length >= 2
    ? canonical[0].trim()
    : Object.keys(examTypes)
      .sort((left, right) => right.length - left.length)
      .find((label) => new RegExp(`(?:^|\\s)${escapeRegExp(label.replace(/_/g, ' '))}(?=\\s|$)`).test(searchable))
  const eyeLabel = canonical.length >= 2
    ? canonical[1].trim()
    : searchable.match(/(?:^|\s)(OD|OE|OS|AO)(?=\s|$)/)?.[1]
  const examType = examLabel ? examTypes[examLabel] : undefined

  if (!examType || (!eyeLabel && examType !== 'specular_microscopy')) return null

  return {
    examType,
    eye: examType === 'specular_microscopy'
      ? 'AO'
      : eyeLabel === 'OE' ? 'OS' : eyeLabel as 'OD' | 'OS' | 'AO',
  }
}
