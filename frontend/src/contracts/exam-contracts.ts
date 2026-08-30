import type { IntakeAnalysis } from './patient-analysis'

export interface ExamFieldContract {
  key: string
  label: string
  paths: string[][]
}

export interface ExamContract {
  key: string
  label: string
  fields: ExamFieldContract[]
}

// Campos mínimos de auditoria derivados da tabela “Dados de entrada necessários”
// do Protocolo_RefratIA_FINAL.pdf. A extração pode conter outros campos além destes.
export const examContracts: Record<string, ExamContract> = {
  pentacam_corneal_tomography: {
    key: 'pentacam_corneal_tomography',
    label: 'Pentacam',
    fields: [
      { key: 'k1', label: 'K1', paths: [['anterior_cornea', 'k1_d'], ['keratometry', 'k1_d'], ['general', 'k1_d']] },
      { key: 'k2', label: 'K2', paths: [['anterior_cornea', 'k2_d'], ['keratometry', 'k2_d'], ['general', 'k2_d']] },
      { key: 'km', label: 'Km', paths: [['anterior_cornea', 'km_d'], ['keratometry', 'mean_k_d'], ['general', 'km_d']] },
      { key: 'astigmatism', label: 'Astigmatismo corneano anterior', paths: [['anterior_cornea', 'astigmatism_d'], ['anterior_cornea', 'astig_diopters'], ['keratometry', 'astigmatism_d']] },
      { key: 'thinnest', label: 'Paquimetria — ponto mais fino', paths: [['pachymetry', 'thinnest_um'], ['pachymetry', 'point_and_finest_um'], ['general', 'thinnest_pachy_um']] },
      { key: 'bad_d', label: 'BAD-D', paths: [['belin_ambrosio', 'd'], ['ectasia_reforcada_belin_ambrosio', 'd']] },
      { key: 'art_max', label: 'ARTmax', paths: [['belin_ambrosio', 'art_max'], ['belin_ambrosio', 'indice_de_progressao', 'art_max'], ['ectasia_reforcada_belin_ambrosio', 'art_max']] },
      { key: 'isv', label: 'ISV', paths: [['topometric_indices_8mm', 'isv'], ['indices_zona_8mm', 'isv']] },
      { key: 'iva', label: 'IVA', paths: [['topometric_indices_8mm', 'iva'], ['indices_zona_8mm', 'iva']] },
      { key: 'iha', label: 'IHA', paths: [['topometric_indices_8mm', 'iha'], ['indices_zona_8mm', 'iha']] },
      { key: 'ki', label: 'KI', paths: [['topometric_indices_8mm', 'ki'], ['indices_zona_8mm', 'ki']] },
      { key: 'cki', label: 'CKI', paths: [['topometric_indices_8mm', 'cki'], ['indices_zona_8mm', 'cki']] },
      { key: 'tkc', label: 'TKC', paths: [['topometric_indices_8mm', 'tkc'], ['indices_zona_8mm', 'tkc']] },
      { key: 'coma', label: 'Coma Z31 — zona 5 mm', paths: [['corneal_rings', 'zernike', '5mm', 'z31_coma'], ['anéis_corneanos', 'total_corneal_wfa_components_of_zernike', 'diam_zone_5_mm', 'z31_coma_um']] },
      { key: 'acd', label: 'ACD — Cataract Pre-OP', paths: [['cataract_preop', 'acd_internal_external_mm'], ['cataract_preop', 'acd_mm'], ['cataract_pre_op', 'acd_mm'], ['anterior_segment', 'internal_anterior_chamber_depth_mm']] },
      { key: 'z40', label: 'Z40 — zona 6 mm', paths: [['cataract_preop', 'total_corneal_z40_6mm_um'], ['cataract_pre_op', 'total_corneal_z40_6mm_um']] },
    ],
  },
  iol_calculation: {
    key: 'iol_calculation',
    label: 'Biometria / cálculo de LIO',
    fields: [
      { key: 'axial_length', label: 'Comprimento axial', paths: [['axial_length_mm'], ['biometry', 'al_mm']] },
      { key: 'k1', label: 'K1', paths: [['keratometry', 'k1_d']] },
      { key: 'k2', label: 'K2', paths: [['keratometry', 'k2_d']] },
      { key: 'km', label: 'Km', paths: [['keratometry', 'mean_k_d']] },
      { key: 'astigmatism', label: 'Astigmatismo da biometria', paths: [['keratometry', 'astigmatism_d']] },
      { key: 'astigmatism_axis', label: 'Eixo do astigmatismo', paths: [['keratometry', 'astigmatism_axis_deg']] },
      { key: 'acd', label: 'ACD', paths: [['anterior_chamber_depth_mm'], ['aqueous_depth_mm']] },
      { key: 'lens_thickness', label: 'Espessura do cristalino', paths: [['lens_thickness_mm']] },
      { key: 'white_to_white', label: 'White-to-white', paths: [['white_to_white_mm']] },
      { key: 'target_refraction', label: 'Refração alvo', paths: [['target_refraction_d']] },
    ],
  },
  specular_microscopy: {
    key: 'specular_microscopy',
    label: 'Microscopia especular',
    fields: [
      { key: 'cell_density', label: 'Contagem endotelial', paths: [['cell_density_cells_per_mm2'], ['cell_density_cells_mm2'], ['endothelium', 'cell_density_cells_per_mm2']] },
    ],
  },
  fundus_retinography: {
    key: 'fundus_retinography',
    label: 'Retinografia',
    fields: [
      { key: 'id', label: 'Nome/ID do paciente', paths: [['id'], ['patient_id'], ['identification', 'id'], ['identification', 'name']] },
      { key: 'findings', label: 'Achados / observações', paths: [['findings'], ['observations'], ['observacoes'], ['content', 'findings']] },
      { key: 'timestamp', label: 'Data/hora do exame', paths: [['timestamp'], ['exam_datetime'], ['performed_at'], ['time'], ['metadata', 'time']] },
    ],
  },
  oct_retina: {
    key: 'oct_retina',
    label: 'OCT de retina',
    fields: [
      { key: 'patient', label: 'Identificação do paciente', paths: [['patient'], ['patient_name'], ['identification', 'name'], ['id']] },
      { key: 'findings', label: 'Achados / observações', paths: [['findings'], ['observations'], ['observacoes'], ['content', 'findings']] },
      { key: 'timestamp', label: 'Data/hora do exame', paths: [['timestamp'], ['exam_datetime'], ['performed_at']] },
    ],
  },
}

function hasValue(value: unknown) {
  return value !== null && value !== undefined && value !== ''
}

function valueAtPath(value: unknown, path: string[]) {
  let current = value
  for (const key of path) {
    if (!current || typeof current !== 'object' || !(key in current)) return undefined
    current = (current as Record<string, unknown>)[key]
  }
  return hasValue(current) ? current : undefined
}

export function getExamContract(examKey: unknown) {
  return typeof examKey === 'string' ? examContracts[examKey] : undefined
}

export function assessExamContract(analysis: IntakeAnalysis, source: Record<string, unknown>) {
  const contract = getExamContract(source.exam)
  if (!contract) return null
  const exam = analysis.exams[contract.key as keyof IntakeAnalysis['exams']]
  const eye = typeof source.eye === 'string' ? source.eye : undefined
  const eyePayload = eye && exam?.eyes?.[eye as keyof NonNullable<typeof exam.eyes>]
  // Biometria AO frequentemente entrega OD e OS separados, sem um bloco AO.
  // Para auditoria de um arquivo AO, considerar os dois olhos evita falso
  // negativo sem misturar valores em um único olho.
  const payload = eyePayload
    ? { ...exam, ...eyePayload }
    : eye === 'AO' && exam?.eyes
      ? {
          ...exam,
          ...Object.values(exam.eyes).reduce<Record<string, unknown>>((merged, value) => ({ ...merged, ...(value ?? {}) }), {}),
        }
      : exam
  const extracted = contract.fields.filter((field) => field.paths.some((path) => hasValue(valueAtPath(payload, path))))
  return { contract, extracted, missing: contract.fields.filter((field) => !extracted.includes(field)) }
}
