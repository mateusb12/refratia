import assert from 'node:assert/strict'
import { normalizeSavedAnalysis } from '../src/contracts/patient-analysis.ts'

const legacyEye = (thinnest, kmax, badD, artMax, coma, z40) => ({
  pachymetry: { point_and_finest_um: thinnest, k_max_anterior_diopters: kmax },
  ectasia_reforcada_belin_ambrosio: { d: badD, indice_de_progressao: { art_max: artMax } },
  anéis_corneanos: { total_corneal_wfa_components_of_zernike: { diam_zone_5_mm: { z31_coma_um: coma } } },
  cataract_pre_op: { total_corneal_z40_6mm_um: z40 },
})

const currentEye = (thinnest, kmax, badD, artMax, coma, z40) => ({
  general: { pachymetry_thinnest_um: thinnest, k_max_anterior_diopters: kmax },
  belin_ambrosio: { d: badD, art_max: artMax },
  anéis_corneanos: { total_corneal_wfa_components_of_zernike: { diam_zone_5_mm: { z31_coma_um: coma } } },
  cataract_pre_op: { total_corneal_z40_6mm_um: z40 },
})

const normalized = normalizeSavedAnalysis({
  schema_version: '1.0', generated_on: '2026-08-15', language: 'pt-BR', patient: {}, source_files: [],
  exams: {
    pentacam_corneal_tomography: { source: ['od.pdf', 'os.pdf'], eyes: {
      OD: legacyEye(529, 44.2, 0.65, 416, 0.097, 0.287),
      OS: currentEye(526, 45.5, 2.27, 366, 0.299, 0.602),
    } },
    iol_calculation: { source: ['bio.pdf'], eyes: { AO: {
      OD: { biometry: { al_mm: 24.6 }, anterior_cornea: { astig_diopters: -0.28 } },
      OS: { biometry: { al_mm: 24.74 }, anterior_cornea: { astig_diopters: -3.17 } },
    } } },
    specular_microscopy: { source: ['micro.jpg'], eyes: { OD: { cell_density_cells_mm2: 2403 }, OS: { cell_density_cells_mm2: 2184 } } },
  },
})

const { exams } = normalized
assert.deepEqual(['OD', 'OS'].flatMap((eye) => [
  exams.pentacam_corneal_tomography.eyes[eye].pachymetry.thinnest_um,
  exams.pentacam_corneal_tomography.eyes[eye].anterior_cornea.kmax_d,
  exams.pentacam_corneal_tomography.eyes[eye].belin_ambrosio.d,
  exams.pentacam_corneal_tomography.eyes[eye].belin_ambrosio.art_max,
  exams.specular_microscopy.eyes[eye].cell_density_cells_per_mm2,
  exams.iol_calculation.eyes[eye].axial_length_mm,
]), [529, 44.2, 0.65, 416, 2403, 24.6, 526, 45.5, 2.27, 366, 2184, 24.74])

assert.deepEqual(['OD', 'OS'].flatMap((eye) => [
  exams.iol_calculation.eyes[eye].keratometry.astigmatism_d,
  Number.parseFloat(exams.pentacam_corneal_tomography.eyes[eye].corneal_rings.zernike['5mm'].z31_coma),
  exams.pentacam_corneal_tomography.eyes[eye].cataract_preop.total_corneal_z40_6mm_um,
]), [-0.28, 0.097, 0.287, -3.17, 0.299, 0.602])
