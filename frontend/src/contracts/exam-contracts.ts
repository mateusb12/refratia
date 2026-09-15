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
  refractometry: {
    key: 'refractometry',
    label: 'Refratometria',
    fields: [
      { key: 'sphere', label: 'Esfera', paths: [['sphere_d'], ['refraction', 'sphere_d']] },
      { key: 'cylinder', label: 'Cilindro', paths: [['cylinder_d'], ['refraction', 'cylinder_d']] },
      { key: 'axis', label: 'Eixo', paths: [['axis_deg'], ['refraction', 'axis_deg']] },
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
      {
        key: 'id',
        label: 'Nome/ID do paciente',
        paths: [
          ['patient_id'],
          ['id'],
          ['identification', 'id'],
          ['identification', 'name'],
        ],
      },
      {
        key: 'eye',
        label: 'Lateralidade',
        paths: [
          ['eye'],
        ],
      },
      {
        key: 'mode',
        label: 'Modo do exame',
        paths: [
          ['mode'],
          ['device_or_mode'],
        ],
      },
      {
        key: 'timestamp',
        label: 'Data/hora do exame',
        paths: [
          ['exam_datetime'],
          ['timestamp'],
          ['performed_at'],
          ['time'],
          ['metadata', 'time'],
        ],
      },
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

export function resolveExamKeyFromSource(
  source: Record<string, unknown>,
) {
  if (
    typeof source.exam === 'string'
    && examContracts[source.exam]
  ) {
    return source.exam
  }

  const filename = String(
    source.path
      ?? source.filename
      ?? '',
  )

  const stem = filename
    .split(/[\\/]/)
    .pop()
    ?.replace(/\.[^.]+$/, '')
    .toUpperCase()

  const prefix = stem?.split('__')[0]

  switch (prefix) {
    case 'EYESUITE':
      return 'iol_calculation'

    case 'PENTACAM':
      return 'pentacam_corneal_tomography'

    case 'RETINA':
      return 'fundus_retinography'

    case 'MICROSCOPIA_ESPECULAR':
      return 'specular_microscopy'

    default:
      return undefined
  }
}

export function assessExamContract(analysis: IntakeAnalysis, source: Record<string, unknown>) {
  const resolvedExamKey = resolveExamKeyFromSource(source)
  const contract = getExamContract(resolvedExamKey)
  if (!contract) return null
  const exam = analysis.exams[contract.key as keyof IntakeAnalysis['exams']]

  // O card representa um arquivo específico. Para selecionar eyes.OD/OS,
  // usamos primeiro a lateralidade codificada no filename padronizado.
  //
  // Isso é somente roteamento entre o arquivo e o payload já extraído;
  // não transforma o filename em evidência OCR.
  const sourceFilename = String(
    source.path
      ?? source.filename
      ?? '',
  )

  const filenameEye = sourceFilename
    .toUpperCase()
    .match(/__(OD|OS|AO)__/)
    ?.[1]

  const metadataEye = typeof source.eye === 'string'
    ? source.eye.trim().toUpperCase()
    : undefined

  const requestedEye = filenameEye ?? metadataEye

  const examEyes = exam?.eyes as
    | Record<string, Record<string, unknown> | undefined>
    | undefined

  // Também normalizamos as chaves existentes para não quebrar por
  // whitespace/case vindos de payloads históricos.
  const eyeKey = requestedEye && examEyes
    ? Object.keys(examEyes).find(
        (key) => key.trim().toUpperCase() === requestedEye,
      )
    : undefined

  const eye = eyeKey ?? requestedEye

  const eyePayload = eyeKey
    ? examEyes?.[eyeKey]
    : undefined
  const payload = eyePayload
    ? { ...exam, ...eyePayload }
    : exam

  // Um documento AO pode conter OD e OS com valores diferentes.
  //
  // Não fazemos merge raso dos dois olhos, porque isso faria o último
  // olho sobrescrever silenciosamente o primeiro.
  //
  // Para auditoria do documento AO, avaliamos o contrato em cada olho
  // separadamente e preservamos os dois valores na apresentação.
  const values = Object.fromEntries(contract.fields.map((field) => {
    if (eye === 'AO' && examEyes) {
      const valuesByEye = ['OD', 'OS']
        .map((eyeName) => {
          const specificEye = examEyes[eyeName]

          if (!specificEye) return undefined

          const eyeSpecificPayload = {
            ...exam,
            ...specificEye,
          }

          const value = field.paths
            .map((path) => valueAtPath(eyeSpecificPayload, path))
            .find(hasValue)

          if (!hasValue(value)) return undefined

          const formattedValue = typeof value === 'number'
            ? value.toLocaleString('pt-BR', {
                maximumFractionDigits: 4,
              })
            : String(value)

          return `${eyeName} ${formattedValue}`
        })
        .filter((value): value is string => Boolean(value))

      if (valuesByEye.length > 0) {
        return [
          field.key,
          valuesByEye.join(' · '),
        ]
      }
    }

    return [
      field.key,
      field.paths
        .map((path) => valueAtPath(payload, path))
        .find(hasValue),
    ]
  }))
  if (contract.key === 'refractometry' && Number(values.cylinder) === 0 && !hasValue(values.axis)) {
    values.axis = 'não aplicável — cilindro 0,00 D'
  }
  const extracted = contract.fields.filter((field) => hasValue(values[field.key]))
  return {
    contract,
    eye,
    extracted,
    values,
    missing: contract.fields.filter((field) => !extracted.includes(field)),
  }
}
