export type Eye = 'OD' | 'OS' | 'AO'

export type ExamKey =
  | 'fundus_retinography'
  | 'iol_calculation'
  | 'oct_retina'
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
  files: Array<{ filename: string; contentType: string; size: number; sha256: string; signed_url?: string }>
  analysis: IntakeAnalysis
  message: string
}

export function normalizeSavedAnalysis(analysis: IntakeAnalysis): IntakeAnalysis {
  const raw = analysis as any
  const pentacam = raw.exams.pentacam_corneal_tomography ?? { source: [] }
  const iol = raw.exams.iol_calculation ?? { source: [] }
  const microscopy = raw.exams.specular_microscopy ?? { source: [] }
  const eyes = Object.fromEntries((['OD', 'OS'] as const).map((eye, index) => {
    const cornea = pentacam.eyes?.[eye] ?? {}
    const ectasia = cornea.belin_ambrosio ?? cornea.ectasia_reforcada_belin_ambrosio ?? {}
    const biometry = iol.eyes?.[eye] ?? iol.eyes?.AO?.[eye] ?? {}
    const endothelium = microscopy.eyes?.[eye] ?? {}
    const coma = cornea.anéis_corneanos?.total_corneal_wfa_components_of_zernike?.diam_zone_5_mm?.z31_coma_um

    return [eye, {
      ...cornea,
      source_file: pentacam.source[index],
      quality: cornea.quality ?? cornea.general?.quality ?? 'Não informado',
      anterior_cornea: {
        ...cornea.anterior_cornea,
        kmax_d: cornea.anterior_cornea?.kmax_d
          ?? cornea.pachymetry?.k_max_anterior_diopters
          ?? cornea.general?.k_max_anterior_diopters
          ?? Number.NaN,
      },
      pachymetry: {
        ...cornea.pachymetry,
        thinnest_um: cornea.pachymetry?.thinnest_um
          ?? cornea.pachymetry?.point_and_finest_um
          ?? cornea.general?.thinnest_pachy_um
          ?? cornea.general?.pachymetry_thinnest_um
          ?? cornea.display_maps?.belin_ambrósio?.thinnest_pachy_um
          ?? ectasia.pachy_ponto_fino_um
          ?? Number.NaN,
      },
      belin_ambrosio: {
        ...ectasia,
        d: ectasia.d ?? Number.NaN,
        art_max: ectasia.art_max ?? ectasia.indice_de_progressao?.art_max ?? Number.NaN,
      },
      topometric_indices_8mm: cornea.topometric_indices_8mm ?? cornea.indices_zona_8mm ?? { tkc: null },
      cataract_preop: cornea.cataract_preop ?? cornea.cataract_pre_op ?? { total_corneal_z40_6mm_um: Number.NaN },
      corneal_rings: {
        ...cornea.corneal_rings,
        zernike: cornea.corneal_rings?.zernike ?? {
          '5mm': { z31_coma: `${cornea.display_maps?.corneal_rings?.zernike_total_corneal_wfa_5mm?.z31_coma_um ?? coma ?? Number.NaN} µm` },
        },
      },
      iol: {
        ...biometry,
        axial_length_mm: biometry.axial_length_mm ?? biometry.biometry?.al_mm,
        keratometry: {
          ...biometry.keratometry,
          // O protocolo proíbe usar Pentacam como fallback para o
          // astigmatismo da biometria no Fluxo C.
          astigmatism_d: biometry.keratometry?.astigmatism_d,
        },
      },
      endothelium: {
        ...endothelium,
        cell_density_cells_per_mm2: endothelium.cell_density_cells_per_mm2 ?? endothelium.cell_density_cells_mm2,
      },
    }]
  }))

  return {
    ...raw,
    exams: {
      ...raw.exams,
      pentacam_corneal_tomography: { ...pentacam, eyes },
      iol_calculation: { ...iol, eyes: { OD: eyes.OD.iol, OS: eyes.OS.iol } },
      specular_microscopy: { ...microscopy, eyes: { OD: eyes.OD.endothelium, OS: eyes.OS.endothelium } },
    },
  }
}

export function isIntakePreview(value: unknown): value is IntakePreview {
  if (!value || typeof value !== 'object') return false
  const result = value as Partial<IntakePreview>
  return typeof result.intakeId === 'string' && Array.isArray(result.files) && typeof result.analysis === 'object' && result.analysis !== null && typeof result.message === 'string'
}
