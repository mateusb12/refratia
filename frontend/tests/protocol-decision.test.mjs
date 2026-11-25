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
  // §7: OD only carries the fixed retina-mapping suggestion (no Pentacam Z40/PNS/coma in this fixture)
  assert.deepEqual(d.eyes.OD.observations.map((o) => o.type), ['mapeamento_retina'])
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

test('Flow A / B without refraction: cascade evaluated, every eye pending at Etapa 1 (never a surgery)', () => {
  for (const [birth, flow] of [['2000-08-16', 'A'], ['1975-03-04', 'B'], ['1982-06-21', 'B']]) {
    const d = evaluateProtocol(patient({ birth }))
    assert.equal(d.flow, flow)
    assert.equal(d.state, 'evaluated')
    assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], [CONDUTA.pending, CONDUTA.pending])
    assert.equal(d.showOrientationSentence, false)
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


// ---------------------------------------------------------------- Flow C informative records (§7, items 45/77/78; guide 31, 32, 39, 76, 76-B)
const withPentacam = (eyes, opts) => {
  const a = patient(opts)
  for (const [eye, extra] of Object.entries(eyes)) Object.assign(a.exams.pentacam_corneal_tomography.eyes[eye], extra)
  return a
}
const wfa = (c0, c90) => ({ coma0_um: c0, coma90_um: c90 })
const coma = (cornea, ant = wfa(0.5, 0.5), post = wfa(0.011, 0.011)) => ({ aberrometry_cornea_4mm: { wfa_anterior: ant, wfa_posterior: post, wfa_cornea: cornea } })
const obs = (d, eye, type) => d.eyes[eye].observations.filter((o) => o.type === type)
const decisionOf = (d) => ['OD', 'OS'].map((e) => [d.eyes[e].conduta, d.eyes[e].tone, d.eyes[e].certainty])

test('case 76-B: 4 mm coma magnitude is √(c0² + c90²) of the WFA Córnea column, never a component, a sum or another column', () => {
  const d = evaluateProtocol(withPentacam({ OD: coma(wfa(-0.1, 0.218)) }))
  const [o] = obs(d, 'OD', 'registro_coma')
  assert.match(o.text, /^Coma corneano \(Z31\) de 0,240 µm, composto a partir dos componentes Coma 0° e Coma 90° da coluna WFA Córnea do Pentacam, em zona de 4,0 mm\./)
  for (const wrong of ['0,100', '0,218', '0,318', '0,707', '0,016']) assert.ok(!o.text.includes(wrong), wrong)
})

test('case 76: registro_coma is emitted with identical text at 0.08, 0.24 and 0.55 µm; no limit, no topo-guided suggestion, conduct unchanged', () => {
  const base = decisionOf(evaluateProtocol(patient()))
  const texts = [0.08, 0.24, 0.55].map((m) => {
    const d = evaluateProtocol(withPentacam({ OD: coma(wfa(m, 0)), OS: coma(wfa(0, m)) }))
    assert.deepEqual(decisionOf(d), base)
    const [o] = obs(d, 'OD', 'registro_coma')
    assert.equal(obs(d, 'OS', 'registro_coma')[0].text, o.text)
    assert.ok(!/topo-?guiad/i.test(o.text) && /NÃO aplica limiar/.test(o.text))
    assert.ok(!/(dentro|fora) d[oe] (limite|faixa)/i.test(o.text))
    return o.text.replace(/de \d,\d{3} µm/, 'de X µm')
  })
  assert.equal(new Set(texts).size, 1)
})

test('coma absent (or one component missing, or only the 5 mm reading) -> no registro_coma, "dado ausente", never presumed', () => {
  for (const eye of [{}, coma(wfa(0.1, null)), { corneal_rings: { zernike: { '5mm': { z31_coma: 0.3 } } } }, coma(undefined)]) {
    const d = evaluateProtocol(withPentacam({ OD: eye }))
    assert.equal(obs(d, 'OD', 'registro_coma').length, 0)
    assert.ok(d.eyes.OD.missing.some((m) => m.startsWith('coma corneano em 4,0 mm')))
  }
})

test('endothelium pending: coma (evaluated before) stays registered; Z40, PNS and retina mapping are not reached', () => {
  const d = evaluateProtocol(withPentacam({ OS: { ...coma(wfa(0.1, 0.1)), cataract_preop: { total_corneal_z40_6mm_um: -0.05 }, anterior_segment: { pns: 2 } } }, { endo: { OD: 2400, OS: null } }))
  assert.equal(d.eyes.OS.conduta, CONDUTA.pending)
  assert.deepEqual(d.eyes.OS.observations.map((o) => o.type).sort(), ['exame_pendente', 'registro_coma'])
})

test('case 39 / §7 Z40: standard record always; atypical outside 0.017–0.708 µm (inclusive range); negative adds the stronger warning', () => {
  const z = (v) => evaluateProtocol(withPentacam({ OD: { cataract_preop: { total_corneal_z40_6mm_um: v } } }))
  for (const v of [0.017, 0.28, 0.708]) {
    const d = z(v)
    assert.equal(obs(d, 'OD', 'sugestao_perfil_lio').length, 1)
    assert.equal(obs(d, 'OD', 'aberracao_esferica_atipica').length, 0, String(v))
  }
  assert.match(obs(z(0.28), 'OD', 'sugestao_perfil_lio')[0].text, /^Aberração esférica corneana total \(Z40, 6mm\) = 0,280 µm\./)
  for (const v of [0.016, 0.709]) {
    const [o] = obs(z(v), 'OD', 'aberracao_esferica_atipica')
    assert.ok(o && !o.text.includes('negativos'), String(v))
    assert.match(o.text, /antes de usar esse valor para escolha de perfil de LIO\.$/)
  }
  const [neg] = obs(z(-0.05), 'OD', 'aberracao_esferica_atipica')
  assert.match(neg.text, /^Aberração esférica corneana \(Z40 = −0,050 µm\) fora da faixa/)
  assert.match(neg.text, /Valores negativos são particularmente incomuns .* ou RK\.$/)
  const none = evaluateProtocol(patient())
  assert.equal(obs(none, 'OD', 'sugestao_perfil_lio').length + obs(none, 'OD', 'aberracao_esferica_atipica').length, 0)
  assert.ok(none.eyes.OD.missing.some((m) => m.startsWith('aberração esférica corneana (Z40')))
})

test('case 31 PNS: canonical text with the stored value (0 is a value, not absence); absent -> "PNS não informado"', () => {
  const d = evaluateProtocol(withPentacam({ OD: { anterior_segment: { pns: 0 } }, OS: { cataract_preop: { pns: 2 } } }))
  assert.equal(obs(d, 'OD', 'pns_cristalino')[0].text, 'Densitometria do cristalino (PNS — Pentacam Nucleus Staging) = 0. Considerar este dado, e os índices do iTrace quando estiver disponível (DLI e aberrometria interna), na avaliação do cristalino, a critério do cirurgião.')
  assert.match(obs(d, 'OS', 'pns_cristalino')[0].text, /= 2\./)
  const none = evaluateProtocol(patient())
  assert.equal(obs(none, 'OD', 'pns_cristalino').length, 0)
  assert.ok(none.eyes.OD.missing.includes('PNS não informado'))
})

test('case 32: mapeamento_retina is fixed in every Flow C eye with an LIO indication (ready or conditioned), never under rule 2.1', () => {
  const d = evaluateProtocol(patient({ endo: { OD: 2400, OS: 1500 } }))
  assert.equal(obs(d, 'OD', 'mapeamento_retina').length, 1)
  assert.equal(obs(d, 'OS', 'mapeamento_retina').length, 1)
  const kc = evaluateProtocol(withPentacam({ OD: { ...coma(wfa(0.1, 0.1)), anterior_segment: { pns: 1 }, cataract_preop: { total_corneal_z40_6mm_um: 0.3 } } }, { tkc: { OD: 'KC1', OS: null } }))
  assert.equal(kc.state, 'keratoconus')
  for (const t of ['mapeamento_retina', 'registro_coma', 'pns_cristalino', 'sugestao_perfil_lio']) assert.equal(obs(kc, 'OD', t).length + obs(kc, 'OS', t).length, 0, t)
})

test('retina findings are never invented from the mere presence of an OCT or retinography file', () => {
  const a = patient()
  a.exams.oct_retina = { source: ['oct.pdf'], eyes: { OD: {}, OS: {} } }
  a.exams.fundus_retinography = { source: ['retina.jpg'], eyes: { OD: {}, OS: {} } }
  const types = Object.values(evaluateProtocol(a).eyes).flatMap((e) => e.observations.map((o) => o.type))
  assert.ok(!types.some((t) => /oct|retinograf|achado/.test(t)), types.join())
})

test('informative records never change conduct, tone or certainty (full records vs none)', () => {
  const full = { ...coma(wfa(0.4, -0.3)), anterior_segment: { pns: 3 }, cataract_preop: { total_corneal_z40_6mm_um: -0.2 } }
  for (const opts of [{}, { endo: { OD: 1500, OS: 2400 } }, { astig: { OD: undefined, OS: -2 } }]) {
    assert.deepEqual(decisionOf(evaluateProtocol(withPentacam({ OD: full, OS: full }, opts))), decisionOf(evaluateProtocol(patient(opts))))
  }
})

test('saved-case shape: normalizeSavedAnalysis keeps the three 4 mm columns and the 5 mm reading side by side', () => {
  const a = withPentacam({ OD: { ...coma(wfa(-0.038, -0.013)), corneal_rings: { zernike: { '5mm': { z31_coma: 0.213 } } } } })
  const n = normalizeSavedAnalysis(a)
  const od = n.exams.pentacam_corneal_tomography.eyes.OD
  assert.deepEqual(od.aberrometry_cornea_4mm.wfa_anterior, wfa(0.5, 0.5))
  assert.equal(od.corneal_rings.zernike['5mm'].z31_coma, 0.213)
  assert.match(obs(evaluateProtocol(n), 'OD', 'registro_coma')[0].text, /de 0,040 µm/)
})
