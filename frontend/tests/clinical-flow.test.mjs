import test from 'node:test'
import assert from 'node:assert/strict'
import { clinicalFlowForAnalysis, flowLabel, keratoconusStatus, pentacamReferenceDate, selectClinicalFlow } from '../src/features/protocol/clinicalFlow.ts'

const entry = (source, timestamp_lido, date_order) => ({ source, timestamp_lido, date_order })
const analysis = (birth, entries, pentacamSources = ['pentacam.pdf']) => ({
  patient: { birth_date: birth },
  exams: { pentacam_corneal_tomography: { source: pentacamSources } },
  verificacao_identidade: entries,
})
const flowOf = (birth, exam) => clinicalFlowForAnalysis(analysis(birth, [entry('pentacam.pdf', exam)]))

// birth 2000-06-15: day before birthday vs day of birthday
const boundaries = [
  ['2017-06-14', 16, 'out_of_scope'], ['2017-06-15', 17, 'out_of_scope'],
  ['2018-06-14', 17, 'out_of_scope'], ['2018-06-15', 18, 'A'],
  ['2039-06-14', 38, 'A'], ['2040-06-14', 39, 'A'], ['2040-06-15', 40, 'B'],
  ['2053-06-14', 52, 'B'], ['2054-06-14', 53, 'B'], ['2054-06-15', 54, 'B'],
  ['2055-06-14', 54, 'B'], ['2055-06-15', 55, 'C'],
]
for (const [exam, age, flow] of boundaries) {
  test(`boundary ${exam} -> ${age} ${flow}`, () => {
    const r = flowOf('2000-06-15', exam)
    assert.equal(r.age, age)
    assert.equal(r.flow, flow)
    assert.equal(selectClinicalFlow(age), flow)
  })
}

test('leap-day birth', () => {
  assert.equal(flowOf('2000-02-29', '2018-02-28').age, 17)
  assert.equal(flowOf('2000-02-29', '2018-03-01').age, 18)
})

test('MDY vs DMY ambiguous Pentacam timestamp', () => {
  const mk = (order) => analysis('1980-01-01', [entry('pentacam.pdf', '03/05/2026', order)])
  assert.equal(pentacamReferenceDate(mk('MDY')), '2026-03-05')
  assert.equal(pentacamReferenceDate(mk('DMY')), '2026-05-03')
  assert.equal(pentacamReferenceDate(mk(undefined)), null)
  assert.equal(pentacamReferenceDate(analysis('1980-01-01', [entry('pentacam.pdf', '11/24/2026')])), '2026-11-24')
  assert.equal(pentacamReferenceDate(analysis('1980-01-01', [entry('pentacam.pdf', '2026-11-24 10:10')])), '2026-11-24')
  // birthday 01 Apr: order changes the age
  const b = (order) => clinicalFlowForAnalysis(analysis('2000-04-01', [entry('pentacam.pdf', '03/05/2026', order)])).age
  assert.equal(b('MDY'), 25)
  assert.equal(b('DMY'), 26)
})

test('invalid inputs give null flow', () => {
  for (const birth of [undefined, '', 'abc', '2000-13-40', '15/06/2000']) {
    assert.deepEqual(flowOf(birth, '2026-01-01'), { age: null, flow: null, referenceDate: null })
  }
  assert.equal(flowOf('2000-06-15', '').flow, null)
  assert.equal(clinicalFlowForAnalysis(null).flow, null)
  assert.equal(clinicalFlowForAnalysis({ patient: { birth_date: '2000-06-15' } }).flow, null)
})

test('conflicting Pentacam dates with different ages -> null; same age keeps flow', () => {
  const conflict = analysis('2000-06-15', [entry('pentacam.pdf', '2026-06-14'), entry('pentacam2.pdf', '2026-06-15')], ['pentacam.pdf', 'pentacam2.pdf'])
  assert.equal(clinicalFlowForAnalysis(conflict).flow, null)
  const same = analysis('2000-06-15', [entry('pentacam.pdf', '2026-07-01'), entry('pentacam2.pdf', '2026-07-02')], ['pentacam.pdf', 'pentacam2.pdf'])
  assert.deepEqual(clinicalFlowForAnalysis(same), { age: 26, flow: 'A', referenceDate: '2026-07-01' })
})

test('non-Pentacam timestamps are ignored', () => {
  const a = analysis('2000-06-15', [entry('biometria.pdf', '2026-07-01')])
  assert.equal(pentacamReferenceDate(a), null)
  assert.equal(clinicalFlowForAnalysis({ ...a, generated_on: '2026-07-01' }).flow, null)
})

test('regression: 26-year-old whose first verification entry is a non-Pentacam exam', () => {
  const a = analysis('1999-03-10', [
    entry('biometria.pdf', '2030-01-01 09:00'),
    entry('pentacam.pdf', '11/24/2026', 'MDY'),
  ])
  assert.deepEqual(clinicalFlowForAnalysis(a), { age: 27, flow: 'A', referenceDate: '2026-11-24' })
  assert.equal(flowLabel('A', 27), 'Fluxo A — 18 a 39 anos (27 anos)')
  const b = analysis('2000-03-10', [entry('biometria.pdf', '2030-01-01 09:00'), entry('pentacam.pdf', '11/24/2026', 'MDY')])
  assert.equal(clinicalFlowForAnalysis(b).age, 26)
})

test('empty Pentacam timestamp is skipped, garbage is not', () => {
  const two = (a, b) => analysis('2000-03-10', [entry('p1.pdf', a), entry('p2.pdf', b, 'MDY')], ['p1.pdf', 'p2.pdf'])
  assert.equal(clinicalFlowForAnalysis(two('', '11/24/2026')).age, 26)
  assert.equal(clinicalFlowForAnalysis(two('garbage', '11/24/2026')).age, null)
  assert.equal(clinicalFlowForAnalysis(two('', '')).age, null)
})

test('keratoconusStatus', () => {
  for (const v of ['—', '-']) assert.equal(keratoconusStatus([v]), 'not_classified')
  for (const v of ['KC2', 'Poss.', 'KC 1-2', 'KC2-3']) assert.equal(keratoconusStatus([v]), 'classified')
  assert.equal(keratoconusStatus([null]), 'unknown')
  assert.equal(keratoconusStatus([]), 'unknown')
  for (const v of ['2', 'KK 2', '2.600']) assert.equal(keratoconusStatus([v]), 'unknown') // non-canonical text is never a class nor a negative
  assert.equal(keratoconusStatus(['—', null]), 'unknown')
  assert.equal(keratoconusStatus(['—', 'KC1']), 'classified')
})

test('exam_at fallback', () => {
  const ex = (od, os, entries = []) => ({
    patient: { birth_date: '2000-03-10' },
    exams: { pentacam_corneal_tomography: { source: ['pentacam.pdf'], eyes: { OD: { exam_at: od }, OS: { exam_at: os } } } },
    verificacao_identidade: entries,
  })
  assert.equal(clinicalFlowForAnalysis(ex('2026-07-30T11:52:26-03:00', '')).age, 26)
  assert.equal(pentacamReferenceDate(ex('2026-07-30T23:30:00-03:00', null)), '2026-07-30')
  assert.equal(clinicalFlowForAnalysis(ex('2026-07-30T11:52:26-03:00', '2026-07-30T11:53:00-03:00')).age, 26)
  assert.equal(clinicalFlowForAnalysis(ex('2026-07-30T11:52:26-03:00', '', [entry('pentacam.pdf', '2030-07-30')])).age, null)
  assert.equal(clinicalFlowForAnalysis(ex('garbage', '2026-07-30T11:52:26-03:00')).age, null)
})

test('labels', () => {
  assert.equal(flowLabel('B', 45), 'Fluxo B — 40 a 54 anos (45 anos)')
  assert.equal(flowLabel('C', 60), 'Fluxo C — 55 anos ou mais (60 anos)')
  assert.equal(flowLabel('out_of_scope', 15), 'Fora do escopo do protocolo (menor de 18 anos)')
  assert.equal(flowLabel(null, null), 'não informado')
})

test('item 35/45(g): day/month birth inversion records both readings without blocking', async () => {
  const { birthDateInversion, clinicalFlowForAnalysis } = await import('../src/features/protocol/clinicalFlow.ts')
  const base = (bioBirth, order, pentacamBirth = '1986-05-03') => ({
    patient: { birth_date: pentacamBirth },
    exams: { pentacam_corneal_tomography: { source: ['p_od.pdf'] } },
    verificacao_identidade: [
      { source: 'bio.pdf', nascimento_lido: bioBirth, date_order: order, timestamp_lido: '2026-04-04 10:00' },
      { source: 'p_od.pdf', nascimento_lido: '05/03/1986', date_order: 'MDY', timestamp_lido: '04/04/2026' },
    ],
  })
  // Pentacam 1986-05-03 -> 39 on 2026-04-04 (Flow A); swapped 1986-03-05 -> 40 (Flow B)
  const crossing = base('05/03/1986', 'DMY') // bio DMY 05/03/1986 = 5 March
  const r = birthDateInversion(crossing)
  assert.deepEqual([r.age, r.flow, r.otherAge, r.otherFlow, r.changesFlow, r.otherReading], [39, 'A', 40, 'B', true, '1986-03-05'])
  // flow is still taken from the Pentacam reading (no block, no switch)
  assert.deepEqual([clinicalFlowForAnalysis(crossing).age, clinicalFlowForAnalysis(crossing).flow], [39, 'A'])
  // 2-digit year equivalent (protocol example DD/MM/AA)
  assert.equal(birthDateInversion(base('05/03/86', 'DMY')).changesFlow, true)
  // inversion that does not change the flow: recorded, changesFlow false
  const same = base('05/03/1960', 'DMY', '1960-05-03')
  const s = birthDateInversion(same)
  assert.equal(s.changesFlow, false)
  assert.equal(s.flow, 'C')
  // exact match -> no inversion
  assert.equal(birthDateInversion(base('03/05/1986', 'DMY')), null)
  // unknown order -> no inference
  assert.equal(birthDateInversion(base('05/03/1986', undefined)), null)
  // other divergence (not a swap) -> not an inversion (identity block belongs to the backend)
  assert.equal(birthDateInversion(base('06/03/1986', 'DMY')), null)
  // day == month cannot be an inversion
  assert.equal(birthDateInversion(base('03/03/1986', 'DMY', '1986-03-03')), null)
})

test('birth_date in the display form written by normalizeSavedReport (formatBirthDate) is accepted', async () => {
  const { clinicalFlowForAnalysis, birthDateInversion } = await import('../src/features/protocol/clinicalFlow.ts')
  const { formatBirthDate } = await import('../src/shared/date.ts')
  const a = (birth) => ({
    patient: { birth_date: birth },
    exams: { pentacam_corneal_tomography: { source: ['p.pdf'] } },
    verificacao_identidade: [
      { source: 'bio.pdf', nascimento_lido: '05/03/1986', date_order: 'DMY', timestamp_lido: '2026-04-04 10:00' },
      { source: 'p.pdf', nascimento_lido: '05/03/1986', date_order: 'MDY', timestamp_lido: '04/04/2026' },
    ],
  })
  const shown = formatBirthDate('1986-05-03')
  assert.equal(shown, '3 de maio de 1986')
  assert.deepEqual([clinicalFlowForAnalysis(a(shown)).age, clinicalFlowForAnalysis(a(shown)).flow], [39, 'A'])
  assert.equal(birthDateInversion(a(shown)).changesFlow, true)
  assert.equal(clinicalFlowForAnalysis(a(formatBirthDate('2000-03-01'))).age, 26) // 'março' with cedilla
  assert.equal(clinicalFlowForAnalysis(a('não informado')).age, null)
  assert.equal(clinicalFlowForAnalysis(a('31 de fevereiro de 1986')).age, null)
})

test('keratoconusForAnalysis: printed dash diagnostic vs not read (never a negative finding)', async () => {
  const { keratoconusForAnalysis } = await import('../src/features/protocol/clinicalFlow.ts')
  const diag = (eye, status, stage) => ({ exam_type: 'pentacam_corneal_tomography', eye, field: 'tkc', status, stage })
  const a = (od, os, diagnostics) => ({ exams: { pentacam_corneal_tomography: { eyes: { OD: { topometric_indices_8mm: { tkc: od } }, OS: { topometric_indices_8mm: { tkc: os } } } } }, extraction_diagnostics: diagnostics })
  const dash = [diag('OD', 'not_applicable', 'recognition'), diag('OS', 'not_applicable', 'recognition')]
  assert.equal(keratoconusForAnalysis(a(null, null, dash)).status, 'not_classified')
  assert.equal(keratoconusForAnalysis(a('—', '-', [])).status, 'not_classified')
  // OCR miss (mapping/missing) or region absent: unknown, not negative
  assert.equal(keratoconusForAnalysis(a(null, null, [diag('OD', 'missing', 'mapping'), diag('OS', 'missing', 'mapping')])).status, 'unknown')
  assert.equal(keratoconusForAnalysis(a(null, null, [diag('OD', 'not_applicable', 'region'), dash[1]])).status, 'unknown')
  assert.equal(keratoconusForAnalysis(a(null, null, undefined)).status, 'unknown')
  // unrecognised value (e.g. a lone digit) is unknown
  assert.deepEqual(keratoconusForAnalysis(a(null, '2', [dash[0]])).eyes, { OD: 'not_classified', OS: 'unknown' })
  // classified in one eye closes the cascade whatever the other eye says
  assert.equal(keratoconusForAnalysis(a('KC1', null, [])).status, 'classified')
})
