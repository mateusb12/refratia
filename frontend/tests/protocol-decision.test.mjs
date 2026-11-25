import test from 'node:test'
import assert from 'node:assert/strict'
import { evaluateProtocol, CONDUTA, outsideExamWindow } from '../src/utils/protocolDecision.ts'
import { normalizeSavedAnalysis } from '../src/contracts/patient-analysis.ts'

// Synthetic Flow C patient. Expected values come from Protocolo_RefratIA_FINAL §7 (p.19-20), §2.1 (p.12), §0.C (p.5),
// never from the app. Pentacam 2026-11-18, birth 1959-03-04 -> 67 years -> Flow C.
const dashDiag = (eye) => ({ exam_type: 'pentacam_corneal_tomography', eye, field: 'tkc', status: 'not_applicable', stage: 'recognition' })
function patient({ birth = '1959-03-04', tkc = { OD: null, OS: null }, dash = ['OD', 'OS'], astig = { OD: -0.43, OS: -1.2 },
  endo = { OD: 2400, OS: 2300 }, bioDate = '2026-11-18', specDate = '2026-11-18 09:46:06', pentacamDate = '11/18/2026',
  withBio = true, withSpec = true } = {}) {
  const exams = {
    pentacam_corneal_tomography: { source: ['p-od.pdf', 'p-os.pdf'], eyes: {
      OD: { topometric_indices_8mm: { tkc: tkc.OD } }, OS: { topometric_indices_8mm: { tkc: tkc.OS } } } },
  }
  const vi = [
    { source: 'p-od.pdf', timestamp_lido: pentacamDate, date_order: 'MDY', nascimento_lido: '03/04/1959' },
    { source: 'p-os.pdf', timestamp_lido: pentacamDate, date_order: 'MDY', nascimento_lido: '03/04/1959' },
  ]
  if (withBio) {
    const eye = (a) => ({ measurement_date: bioDate, keratometry: a === undefined ? {} : { astigmatism_d: a } })
    exams.iol_calculation = { source: ['bio.pdf'], eyes: { OD: eye(astig.OD), OS: eye(astig.OS) } }
  }
  if (withSpec) {
    exams.specular_microscopy = { source: ['spec.jpg'], eyes: { OD: { cell_density_cells_per_mm2: endo.OD }, OS: { cell_density_cells_per_mm2: endo.OS } } }
    if (specDate) vi.push({ source: 'spec.jpg', timestamp_lido: specDate })
  }
  return { patient: { birth_date: birth }, exams, verificacao_identidade: vi, extraction_diagnostics: dash.map(dashDiag) }
}
const conducts = (a) => { const d = evaluateProtocol(a); return [d.eyes.OD.conduta, d.eyes.OS.conduta] }

test('complete Flow C, both eyes >= 2000: non-toric OD (0.43 D), toric OS (1.20 D)', () => {
  const d = evaluateProtocol(patient())
  assert.equal(d.flow, 'C')
  assert.equal(d.state, 'evaluated')
  assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], [CONDUTA.ready, CONDUTA.readyToric])
  assert.equal(d.showOrientationSentence, true)
})

test('toric boundary is >= 0.75 D on |astigmatism|', () => {
  assert.deepEqual(conducts(patient({ astig: { OD: -0.75, OS: 0.74 } })), [CONDUTA.readyToric, CONDUTA.ready])
})

test('P0 regression: biometry astigmatism absent is never presumed < 0.75 D (C3 OS shape: keratometry {})', () => {
  const d = evaluateProtocol(patient({ astig: { OD: -0.43, OS: undefined } }))
  assert.equal(d.eyes.OS.conduta, CONDUTA.readyGeneric)
  assert.equal(d.eyes.OS.toric, null)
  assert.equal(d.eyes.OS.astigmatism.status, 'ausente')
  assert.ok(d.eyes.OS.missing.some((m) => m.includes('astigmatismo')))
  assert.notEqual(d.eyes.OS.conduta, CONDUTA.ready)
})

test('P0 regression: endothelium < 2000 conditions the conduct of that eye only (never the fellow eye)', () => {
  const d = evaluateProtocol(patient({ endo: { OD: 2017, OS: 1716 } }))
  assert.equal(d.eyes.OD.conduta, CONDUTA.ready)
  assert.equal(d.eyes.OS.conduta, CONDUTA.conditionedToric)
  assert.ok(d.eyes.OS.observations.some((o) => o.type === 'endotelio_baixo'))
  assert.equal(d.eyes.OD.observations.length, 0)
})

test('endothelium boundary: 2000 is adequate, 1999 conditions', () => {
  assert.deepEqual(conducts(patient({ endo: { OD: 2000, OS: 1999 } })), [CONDUTA.ready, CONDUTA.conditionedToric])
})

test('C3-like: astigmatism absent AND endothelium < 2000 -> generic conditioned variant', () => {
  assert.equal(conducts(patient({ astig: { OD: -0.43, OS: undefined }, endo: { OD: 2017, OS: 1716 } }))[1], CONDUTA.conditionedGeneric)
})

test('P0 regression: microscopy not sent -> Pendente (exame_pendente), never "adequate"', () => {
  const d = evaluateProtocol(patient({ withSpec: false }))
  assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], [CONDUTA.pending, CONDUTA.pending])
  assert.ok(d.eyes.OD.observations.some((o) => o.type === 'exame_pendente' && o.text.includes('microscopia especular')))
  assert.equal(d.showOrientationSentence, false)
})

test('endothelial value missing for one eye -> only that eye is pending', () => {
  assert.deepEqual(conducts(patient({ endo: { OD: 2400, OS: null } })), [CONDUTA.ready, CONDUTA.pending])
})

test('biometry not sent: astigmatism absent -> generic indication (not pending, not non-toric)', () => {
  assert.deepEqual(conducts(patient({ withBio: false })), [CONDUTA.readyGeneric, CONDUTA.readyGeneric])
})

test('4-month window (§0.C): microscopy older than 4 months -> pending; exactly 4 months is inside', () => {
  assert.equal(conducts(patient({ specDate: '2026-07-17 10:00' }))[0], CONDUTA.pending)
  assert.equal(conducts(patient({ specDate: '2026-07-18 10:00' }))[0], CONDUTA.ready)
  assert.equal(outsideExamWindow('2026-03-17', '2026-07-18'), true)
  assert.equal(outsideExamWindow('2026-03-18', '2026-07-18'), false)
  assert.equal(outsideExamWindow('2026-11-19', '2026-07-18'), true)
})

test('microscopy date unread -> window unverifiable -> pending (never assumed inside)', () => {
  const d = evaluateProtocol(patient({ specDate: null }))
  assert.equal(d.eyes.OD.conduta, CONDUTA.pending)
  assert.match(d.eyes.OD.endothelium.text, /janela/)
})

test('biometry outside window -> eye pending at the astigmatism checkpoint', () => {
  assert.deepEqual(conducts(patient({ bioDate: '2026-01-10' })), [CONDUTA.pending, CONDUTA.pending])
})

test('rule 2.1: classified TKC in one eye closes the cascade for the whole patient, in every flow', () => {
  for (const birth of ['1959-03-04', '1975-03-04', '2000-08-16']) {
    const d = evaluateProtocol(patient({ birth, tkc: { OD: null, OS: 'KC1' }, dash: ['OD'] }))
    assert.equal(d.state, 'keratoconus', birth)
    assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], [CONDUTA.keratoconus, CONDUTA.keratoconus])
    assert.ok(d.eyes.OD.observations.some((o) => o.type === 'ceratocone_unilateral_genetica'))
    assert.equal(d.showOrientationSentence, false)
  }
  const both = evaluateProtocol(patient({ tkc: { OD: 'Poss.', OS: 'KC2-3' }, dash: [] }))
  assert.equal(both.eyes.OD.observations.some((o) => o.type === 'ceratocone_unilateral_genetica'), false)
})

test('TKC classified with missing microscopy: rule 2.1 wins (endothelium never evaluated)', () => {
  const d = evaluateProtocol(patient({ tkc: { OD: 'KC1', OS: null }, dash: ['OS'], withSpec: false }))
  assert.equal(d.eyes.OS.conduta, CONDUTA.keratoconus)
  assert.equal(d.eyes.OS.endothelium.status, 'nao_avaliado')
})

test('TKC unread (null without printed-dash diagnostic) -> screening incomplete, no conduct', () => {
  const d = evaluateProtocol(patient({ dash: ['OD'] }))
  assert.equal(d.state, 'screening_incomplete')
  assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], [null, null])
})

test('bare digit TKC ("2") is unknown, never classified nor negative', () => {
  const d = evaluateProtocol(patient({ tkc: { OD: null, OS: '2' }, dash: ['OD'] }))
  assert.equal(d.state, 'screening_incomplete')
})

test('Flow A / B: no automatic conduct when TKC is not classified', () => {
  for (const [birth, flow] of [['2000-08-16', 'A'], ['1975-03-04', 'B'], ['1982-06-21', 'B']]) {
    const d = evaluateProtocol(patient({ birth }))
    assert.equal(d.flow, flow)
    assert.equal(d.state, 'manual')
    assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], [null, null])
  }
})

test('age boundaries 54/55 and under 18', () => {
  assert.equal(evaluateProtocol(patient({ birth: '1971-11-19' })).flow, 'B') // 54 on 2026-11-18
  assert.equal(evaluateProtocol(patient({ birth: '1971-11-18' })).state, 'evaluated') // 55
  const minor = evaluateProtocol(patient({ birth: '2010-01-01' }))
  assert.equal(minor.state, 'out_of_scope')
  assert.equal(minor.eyes.OD.conduta, CONDUTA.outOfScope)
})

test('unknown age (ambiguous birth date) -> no flow, no conduct', () => {
  const d = evaluateProtocol(patient({ birth: '03/04/1959' }))
  assert.equal(d.state, 'no_flow')
  assert.equal(d.eyes.OD.conduta, null)
})

test('works on the normalized saved-case shape the app renders (AO-nested biometry)', () => {
  const raw = patient()
  raw.schema_version = '1.0'; raw.source_files = []
  raw.exams.iol_calculation = { source: ['bio.pdf'], eyes: { AO: { OD: { measurement_date: '2026-11-18', keratometry: { astigmatism_d: -0.2 } }, OS: { measurement_date: '2026-11-18', keratometry: { astigmatism_d: -2 } } } } }
  assert.deepEqual(conducts(normalizeSavedAnalysis(raw)), [CONDUTA.ready, CONDUTA.readyToric])
})

test('rule 2.1 fires even when age is unknown (sovereign in any flow)', () => {
  const p = patient({ birth: '03/04/1959' })
  p.exams.pentacam_corneal_tomography.eyes.OD.topometric_indices_8mm.tkc = 'KC1'
  const d = evaluateProtocol(p)
  assert.equal(d.state, 'keratoconus')
  assert.equal(d.eyes.OS.conduta, CONDUTA.keratoconus)
})
