import test from 'node:test'
import assert from 'node:assert/strict'
import { evaluateProtocol } from '../src/utils/protocolDecision.ts'

// Informative observations and review states (Protocolo_RefratIA_FINAL §0.C, rule 2.1, §5 Etapas 2-5, §8 p.21-22; guide §4.2
// cases 4, 30, 42, 43, 68, 69, 74, 75-B). Expected values hand-derived from the protocol formulas, never from the implementation.
const LP = 'LASIK ou PRK'
const both = (v) => ({ OD: v, OS: v })
function patient({ birth = '1996-01-01', refr = both({ sphere_d: -3, cylinder_d: -0.5 }), km = both(43), pachy = both(540), acd = both(3.2), endo = both(2600),
  tkc = both(null), bad = both(1.0), art = both(450), quality = both(undefined), astAnt = both(undefined), abcd = both(undefined), isv = both(undefined), bio = null } = {}) {
  const eye = (e) => ({ topometric_indices_8mm: { tkc: tkc[e], isv: isv[e] }, anterior_cornea: { km_d: km[e], astigmatism_d: astAnt[e] }, pachymetry: { thinnest_um: pachy[e] },
    cataract_preop: { acd_mm: acd[e] }, belin_ambrosio: { d: bad[e], art_max: art[e] }, quality: quality[e], abcd_keratoconus_staging: { staging: abcd[e] } })
  const exams = {
    pentacam_corneal_tomography: { source: ['p-od.pdf', 'p-os.pdf'], eyes: { OD: eye('OD'), OS: eye('OS') } },
    refractometry: { source: ['auto.jpg'], eyes: refr },
    specular_microscopy: { source: ['spec.jpg'], eyes: { OD: { cell_density_cells_per_mm2: endo.OD }, OS: { cell_density_cells_per_mm2: endo.OS } } },
  }
  if (bio) exams.iol_calculation = { source: ['bio.pdf'], eyes: { OD: { keratometry: { astigmatism_d: bio.OD }, measurement_date: '2026-11-18' }, OS: { keratometry: { astigmatism_d: bio.OS }, measurement_date: '2026-11-18' } } }
  const vi = ['p-od.pdf', 'p-os.pdf'].map((source) => ({ source, timestamp_lido: '11/18/2026', date_order: 'MDY' }))
  vi.push({ source: 'auto.jpg', timestamp_lido: '2026-11-18 10:00' }, { source: 'spec.jpg', timestamp_lido: '2026-11-18 09:46:06' })
  const dash = ['OD', 'OS'].filter((e) => tkc[e] === null)
  return { patient: { birth_date: birth }, exams, verificacao_identidade: vi,
    extraction_diagnostics: dash.map((e) => ({ exam_type: 'pentacam_corneal_tomography', eye: e, field: 'tkc', status: 'not_applicable', stage: 'recognition' })) }
}
const r = (sphere_d, cylinder_d = 0) => ({ sphere_d, cylinder_d })
const obs = (d, eye, type) => d.eyes[eye].observations.find((o) => o.type === type)
const types = (d, eye) => d.eyes[eye].observations.map((o) => o.type)
const step = (d, eye, re) => d.eyes[eye].steps.find((s) => re.test(s.rule))

test('QI other than OK -> qualidade_exame + "suspeita — revisar"; cascade and certainty unchanged (case 30)', () => {
  const d = evaluateProtocol(patient({ quality: { OD: 'Alinh.!', OS: 'OK' } }))
  assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], [LP, LP])
  assert.equal(d.eyes.OD.certainty, 'definitiva')
  assert.match(obs(d, 'OD', 'qualidade_exame').text, /QI “Alinh\.!”.*usados na cascata como lidos/)
  assert.match(d.eyes.OD.review, /suspeita — revisar/)
  assert.equal(obs(d, 'OS', 'qualidade_exame'), undefined)
  assert.equal(d.eyes.OS.review, null)
})

test('QI unread ("Não informado" placeholder, absent) or OK never warns', () => {
  for (const q of ['Não informado', undefined, ' OK ', 'ok']) {
    const d = evaluateProtocol(patient({ quality: both(q) }))
    assert.equal(obs(d, 'OD', 'qualidade_exame'), undefined, String(q))
  }
})

test('QI applies in any flow, including Flow C and the rule 2.1 referral', () => {
  const c = evaluateProtocol(patient({ birth: '1960-01-01', bio: both(-0.3), quality: both('ALINH') }))
  assert.equal(c.flow, 'C')
  assert.equal(c.eyes.OD.conduta, 'LIO Multifocal ou EDOF')
  assert.ok(obs(c, 'OD', 'qualidade_exame') && obs(c, 'OS', 'qualidade_exame'))
  const k = evaluateProtocol(patient({ tkc: { OD: 'KC1', OS: null }, quality: { OD: 'ALINH', OS: 'OK' } }))
  assert.equal(k.state, 'keratoconus')
  assert.ok(obs(k, 'OD', 'qualidade_exame'))
})

test('ectasia: Belin ABCD grade >= 1 without TKC -> risco_ectasia citing the staging; conduct unchanged', () => {
  const d = evaluateProtocol(patient({ abcd: { OD: 'A1 B0 C0 D0', OS: 'A0 B0 C0 D' } }))
  assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], [LP, LP])
  assert.match(obs(d, 'OD', 'risco_ectasia_nao_confirmado').text, /estadiamento Belin ABCD A1 B0 C0 D0.*neste exame/)
  assert.equal(obs(d, 'OS', 'risco_ectasia_nao_confirmado'), undefined)
})

test('ectasia: topometric indices have no protocol cut (item 36) -> never judged from the number alone', () => {
  const d = evaluateProtocol(patient({ isv: both(99) }))
  assert.equal(obs(d, 'OD', 'risco_ectasia_nao_confirmado'), undefined)
})

test('ectasia: Flow C also records the suspicion (rule 2.1, every flow); conduct unchanged', () => {
  const d = evaluateProtocol(patient({ birth: '1960-01-01', bio: { OD: -0.28, OS: -3.17 }, bad: { OD: 0.65, OS: 2.27 }, art: { OD: 416, OS: 366 } }))
  assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], ['LIO Multifocal ou EDOF', 'LIO Multifocal Tórica ou EDOF Tórica'])
  assert.match(obs(d, 'OS', 'risco_ectasia_nao_confirmado').text, /BAD-D 2,27 \/ ARTmax 366 µm/)
  assert.equal(obs(d, 'OD', 'risco_ectasia_nao_confirmado'), undefined)
})

test('item 43: unilateral keratoconus -> contralateral eye records its altered indices ("…neste olho"), referral kept', () => {
  const d = evaluateProtocol(patient({ tkc: { OD: 'KC1', OS: null }, bad: { OD: 1.0, OS: 2.0 } }))
  assert.equal(d.eyes.OS.conduta, 'Encaminhamento a especialista em córnea')
  assert.match(obs(d, 'OS', 'risco_ectasia_nao_confirmado').text, /neste olho/)
  assert.equal(obs(d, 'OD', 'risco_ectasia_nao_confirmado'), undefined)
})

test('item 48: PTA 38 % -> margem_seguranca_pta_ler (case 4); PTA 32 % -> none (case 43)', () => {
  // -6.00 at the corneal plane -5.597 D -> ablation 100.5 µm; LASIK PTA = 190.5/500 = 38.1 %, LER 309.5 µm
  const d = evaluateProtocol(patient({ refr: both(r(-6)), km: both(44), pachy: both(500) }))
  assert.equal(d.eyes.OD.conduta, LP)
  const m = obs(d, 'OD', 'margem_seguranca_pta_ler').text
  assert.match(m, /^LASIK: Percentual de Tecido Alterado \(PTA\) = 38,1 %/)
  assert.doesNotMatch(m, /Leito Estromal Residual/)
  assert.equal(obs(evaluateProtocol(patient({ refr: both(r(-6)), km: both(44), pachy: both(600) })), 'OD', 'margem_seguranca_pta_ler'), undefined)
})

test('item 48: LER 289.7 µm -> LER text; both triggers -> PTA then LER in one observation', () => {
  const ler = obs(evaluateProtocol(patient({ pachy: both(440) })), 'OD', 'margem_seguranca_pta_ler').text
  assert.match(ler, /^LASIK: Leito Estromal Residual \(LER\) = 290 µm/)
  const two = obs(evaluateProtocol(patient({ pachy: both(420) })), 'OD', 'margem_seguranca_pta_ler').text // PTA 35,8 %, LER 270 µm
  assert.ok(two.indexOf('PTA') < two.indexOf('LER') && two.includes('35,8 %') && two.includes('270 µm'))
})

test('items 72/74: mitomicina_prk by estimated depth >= 75 µm (case 74); 70 µm -> none', () => {
  const deep = evaluateProtocol(patient({ refr: both(r(-5)), km: both(44), pachy: both(560) })) // 84.7 µm
  assert.match(obs(deep, 'OD', 'mitomicina_prk').text, /profundidade estimada de 85 µm/)
  assert.doesNotMatch(obs(deep, 'OD', 'mitomicina_prk').text, /LASIK/)
  assert.equal(obs(evaluateProtocol(patient({ refr: both(r(-4.1)), km: both(44), pachy: both(560) })), 'OD', 'mitomicina_prk'), undefined) // 70.2 µm
})

test('item 73: teto_plataforma only above 6,00 D of myopic SE in a laser eye (case 75-B)', () => {
  assert.equal(obs(evaluateProtocol(patient({ refr: both(r(-5, -1.5)), km: both(44), pachy: both(600) })), 'OD', 'teto_plataforma'), undefined)
  assert.equal(obs(evaluateProtocol(patient({ refr: both(r(-6)), km: both(44), pachy: both(600) })), 'OD', 'teto_plataforma'), undefined)
  const mid = obs(evaluateProtocol(patient({ refr: both(r(-7, -1)), km: both(45), pachy: both(620) })), 'OD', 'teto_plataforma').text
  assert.match(mid, /LASIK no WaveLight EX500: dentro.*LASIK no ZEISS MEL 90: dentro.*ablação de superfície no WaveLight EX500: fora.*ablação de superfície no ZEISS MEL 90: não consta/)
  const high = evaluateProtocol(patient({ refr: both(r(-10.5, -1)), km: both(46), pachy: both(650) }))
  assert.ok(high.eyes.OD.conduta.startsWith(LP), high.eyes.OD.conduta)
  assert.match(obs(high, 'OD', 'teto_plataforma').text, /LASIK no WaveLight EX500: dentro.*LASIK no ZEISS MEL 90: fora/)
  const phakic = evaluateProtocol(patient({ refr: both(r(-12.5)), km: both(45), pachy: both(520), acd: both(3.4) }))
  assert.equal(phakic.eyes.OD.conduta, 'Lente fácica')
  assert.equal(obs(phakic, 'OD', 'teto_plataforma'), undefined)
})

test('item 69: PRK K_final 48.08 out while LASIK 47.88 in -> eye operable, PRK yellow + k_final_margem_estreita (case 68)', () => {
  const d = evaluateProtocol(patient({ refr: both(r(-2.5, -1)), km: both(50.4), pachy: both(530) }))
  assert.equal(d.eyes.OD.conduta, LP)
  assert.equal(step(d, 'OD', /Etapa 5 · PRK/).outcome, 'warn')
  // LASIK 47.88 is inside by 0.12 < 0.19 D, so the same item 69 rule marks it yellow too (the guide's case 68 names only the PRK line)
  assert.equal(step(d, 'OD', /Etapa 4 · LASIK/).outcome, 'warn')
  assert.match(obs(d, 'OD', 'k_final_margem_estreita').text, /LASIK 47,88 D, dentro da faixa a 0,12 D do teto de 48 D; PRK 48,0[78] D, acima do teto de 48 D por 0,0[78] D.*0,19 D/) // 48.075: binary float shows 48,07
  const both_out = evaluateProtocol(patient({ refr: both(r(-2.5, -1)), km: both(50.6), pachy: both(530) }))
  assert.ok(!both_out.eyes.OD.conduta.includes('LASIK'))
  assert.equal(obs(both_out, 'OD', 'k_final_margem_estreita'), undefined)
})

test('item 69: inside by less than 0,064 × |SE| -> yellow, approved (case 69, both limits)', () => {
  const d = evaluateProtocol(patient({ refr: both(r(-2.5, -1)), km: both(50.25), pachy: both(530) }))
  assert.equal(d.eyes.OD.conduta, LP)
  assert.equal(step(d, 'OD', /Etapa 5 · PRK/).outcome, 'warn')
  assert.match(obs(d, 'OD', 'k_final_margem_estreita').text, /PRK 47,93 D, dentro da faixa a 0,08 D do teto de 48 D/)
  const floor = evaluateProtocol(patient({ refr: both(r(-5.5)), km: both(38.5), pachy: both(560) }))
  assert.ok(floor.eyes.OD.conduta.startsWith(LP))
  assert.equal(step(floor, 'OD', /Etapa 4 · LASIK/).outcome, 'warn')
  assert.match(obs(floor, 'OD', 'k_final_margem_estreita').text, /LASIK 33,89 D, abaixo do piso de 34 D/)
  assert.equal(obs(evaluateProtocol(patient()), 'OD', 'k_final_margem_estreita'), undefined)
})

test('item 47: refractive vs anterior corneal astigmatism differing > 0,75 D -> discordancia (case 42); Flow A only', () => {
  const d = evaluateProtocol(patient({ astAnt: { OD: 1.6, OS: 1.25 } })) // cylinder 0.50: 1.10 and 0.75
  assert.equal(d.eyes.OD.conduta, LP)
  assert.match(obs(d, 'OD', 'discordancia_astigmatismo').text, /\(0,50 D\).*\(1,60 D\) de 1,10 D/)
  assert.equal(obs(d, 'OS', 'discordancia_astigmatismo'), undefined)
  const b = evaluateProtocol(patient({ birth: '1981-01-01', astAnt: both(2.5) }))
  assert.equal(b.flow, 'B')
  assert.equal(obs(b, 'OD', 'discordancia_astigmatismo'), undefined)
})

test('informative observations never change the conduct or certainty of an otherwise identical eye', () => {
  const plain = evaluateProtocol(patient({ refr: both(r(-6)), km: both(44), pachy: both(500) }))
  const loaded = evaluateProtocol(patient({ refr: both(r(-6)), km: both(44), pachy: both(500), quality: both('ALINH'), astAnt: both(2), abcd: both('A2 B1 C0 D0') }))
  for (const e of ['OD', 'OS']) assert.deepEqual([loaded.eyes[e].conduta, loaded.eyes[e].certainty, loaded.eyes[e].tone], [plain.eyes[e].conduta, plain.eyes[e].certainty, plain.eyes[e].tone])
  assert.deepEqual(types(loaded, 'OD').sort(), ['discordancia_astigmatismo', 'margem_seguranca_pta_ler', 'mitomicina_prk', 'qualidade_exame', 'risco_ectasia_nao_confirmado'])
})
