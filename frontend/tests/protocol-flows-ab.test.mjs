import test from 'node:test'
import assert from 'node:assert/strict'
import { evaluateProtocol } from '../src/features/protocol/protocolDecision.ts'

// Flow A (§5, p.14-17) and Flow B (§6, p.17-19) expectations, hand-derived from Protocolo_RefratIA_FINAL §2 (parameters),
// §3 (formulas: SE = esf + cil/2; ablação = (S²/3)·1,275·max(|esf_c|,|esf_c+cil_c|) on the corneal plane, d = 12 mm;
// LER = paq − flap − abl; PTA = (flap + abl)/paq; K_final = Km ∓ fator·|SE|), §0.C (4-month window) and §8 (conduta enum).
// Never derived from the implementation. Pentacam 2026-11-18.
const LP = 'LASIK ou PRK'
const P = 'Pendente — necessidade de repetir ou realizar o exame'
const AMB = 'Encaminhamento ao ambulatório do cirurgião'
const PB = 'Presbyond ou Custom-Q/PresbyLASIK'
const PY = 'Presbyond'
const OUT = 'Fora da faixa de literatura para Presbyond/PresbyLASIK'
const K = 'Encaminhamento a especialista em córnea'

const both = (v) => ({ OD: v, OS: v })
function patient({ birth = '1996-01-01', refr = both({ sphere_d: -3, cylinder_d: -0.5 }), refrDate = '2026-11-18 10:00', km = both(43), pachy = both(540),
  acd = both(3.2), endo = both(2600), specDate = '2026-11-18 09:46:06', withSpec = true, tkc = both(null), dash = ['OD', 'OS'], bad = both(1.0), art = both(450) } = {}) {
  const eye = (e) => ({ topometric_indices_8mm: { tkc: tkc[e] }, anterior_cornea: { km_d: km[e] }, pachymetry: { thinnest_um: pachy[e] },
    cataract_preop: { acd_mm: acd[e] }, belin_ambrosio: { d: bad[e], art_max: art[e] } })
  const exams = { pentacam_corneal_tomography: { source: ['p-od.pdf', 'p-os.pdf'], eyes: { OD: eye('OD'), OS: eye('OS') } } }
  const vi = [{ source: 'p-od.pdf', timestamp_lido: '11/18/2026', date_order: 'MDY' }, { source: 'p-os.pdf', timestamp_lido: '11/18/2026', date_order: 'MDY' }]
  if (refr) {
    exams.refractometry = { source: ['auto.jpg'], eyes: Object.fromEntries(Object.entries(refr).filter(([, v]) => v)) }
    if (refrDate) vi.push({ source: 'auto.jpg', timestamp_lido: refrDate })
  }
  if (withSpec) {
    exams.specular_microscopy = { source: ['spec.jpg'], eyes: { OD: { cell_density_cells_per_mm2: endo.OD }, OS: { cell_density_cells_per_mm2: endo.OS } } }
    vi.push({ source: 'spec.jpg', timestamp_lido: specDate })
  }
  return { patient: { birth_date: birth }, exams, verificacao_identidade: vi,
    extraction_diagnostics: dash.map((e) => ({ exam_type: 'pentacam_corneal_tomography', eye: e, field: 'tkc', status: 'not_applicable', stage: 'recognition' })) }
}
const B = (o = {}) => patient({ birth: '1981-01-01', ...o })
const r = (sphere_d, cylinder_d) => (cylinder_d === undefined ? { sphere_d } : { sphere_d, cylinder_d })

// [name, input, expected conduct OD, OS, extra checks]
const matrix = [
  // ---------------- Flow A ----------------
  ['A01 low myope -3.00 -0.50: LASIK and PRK both pass (Etapas 4/5, item 69)', patient(), LP, LP],
  ['A02 -9.00: LASIK passes after OZ 6.0 (PTA-only fail); 5A fires (SE >= 8), ACD/endo ok, cyl < 0.75 -> + Lente fácica',
    patient({ refr: both(r(-9, 0)), km: both(44), pachy: both(560) }), 'LASIK ou PRK ou Lente fácica', 'LASIK ou PRK ou Lente fácica', { obs: 'qualidade_visual_noturna' }],
  ['A03 -8.00 -1.50: 5A + cyl >= 0.75, residual supported -> three options (item 58)',
    patient({ refr: both(r(-8, -1.5)), km: both(44), pachy: both(560), acd: both(3.3), endo: both(2500) }),
    'LASIK ou PRK ou Lente fácica combinada com PRK (bioptics) ou Lente fácica tórica', 'LASIK ou PRK ou Lente fácica combinada com PRK (bioptics) ou Lente fácica tórica'],
  ['A04 SE -13.25 (>= 12 D): straight to Etapa 6, cyl 0.50 -> Lente fácica', patient({ refr: both(r(-13, -0.5)), km: both(45), pachy: both(520), acd: both(3.4), endo: both(2800) }), 'Lente fácica', 'Lente fácica'],
  ['A05 SE -13 cyl -2.00: residual supported -> bioptics or tórica', patient({ refr: both(r(-12, -2)), km: both(44), pachy: both(500), acd: both(3.1), endo: both(2300) }),
    'Lente fácica combinada com PRK (bioptics) ou Lente fácica tórica', 'Lente fácica combinada com PRK (bioptics) ou Lente fácica tórica'],
  ['A06 SE -13 cyl -2.00, K_final residual 48.28 > 48 -> Lente fácica tórica only', patient({ refr: both(r(-12, -2)), km: both(47.5), pachy: both(500), acd: both(3.1), endo: both(2300) }),
    'Lente fácica tórica', 'Lente fácica tórica'],
  ['A06b SE -13 cyl -2.00, Km 34.5: myopic reading of the residual K_final gives 33.73 < 34 -> no bioptics (spec ambiguity resolved conservatively)',
    patient({ refr: both(r(-12, -2)), km: both(34.5), pachy: both(500), acd: both(3.1), endo: both(2300) }), 'Lente fácica tórica', 'Lente fácica tórica'],
  ['A07 SE >= 12, ACD 2.8 < 3.0 -> Etapa 7 Anel estromal + PRK', patient({ refr: both(r(-13, 0)), acd: both(2.8), endo: both(2500) }), 'Anel estromal + PRK', 'Anel estromal + PRK'],
  ['A08 SE >= 12, microscopy not sent -> Pendente (Etapa 6)', patient({ refr: both(r(-13, 0)), withSpec: false }), P, P],
  ['A09 -10.00 Km 40.5: both K_final < 34 -> laser contraindicated -> Lente fácica', patient({ refr: both(r(-10, 0)), km: both(40.5), pachy: both(650), endo: both(2400) }), 'Lente fácica', 'Lente fácica'],
  ['A10 -6.00 paq 430: LASIK fails LER, PRK passes -> PRK', patient({ refr: both(r(-6, 0)), km: both(44), pachy: both(430) }), 'PRK', 'PRK'],
  ['A11 OD SE +7.00 (> +6) -> ambulatory referral; OS continues', patient({ refr: { OD: r(7, 0), OS: r(-3, -0.5) } }), AMB, LP, { obs: 'hipermetropia_alta' }],
  ['A12 SE +6.00 boundary is not referral -> laser', patient({ refr: both(r(6, 0)), km: both(42), pachy: both(550) }), LP, LP],
  ['A13 -5.00 Km 38.5: 5A fires on K_final < 35 but ACD absent -> laser kept (never presumed)', patient({ refr: both(r(-5, 0)), km: both(38.5), pachy: both(560), acd: both(null), endo: both(2400) }), LP, LP, { obs: 'qualidade_visual_noturna' }],
  ['A14 -5.00 Km 38.0: LASIK K 33.81 out, PRK K 34.13 in -> eye operable, LASIK kept (item 69); 5A -> + fácica',
    patient({ refr: both(r(-5, 0)), km: both(38), pachy: both(560) }), 'LASIK ou PRK ou Lente fácica', 'LASIK ou PRK ou Lente fácica'],
  ['A15 refraction not sent -> Pendente', patient({ refr: null }), P, P, { missing: 'refração' }],
  ['A16 Km absent -> Pendente (laser checkpoints)', patient({ km: { OD: null, OS: 43 } }), P, LP],
  ['A17 refraction > 4 months from Pentacam -> Pendente (§0.C)', patient({ refrDate: '2026-06-01 10:00' }), P, P],
  ['A18 refraction date unread -> Pendente (window unverifiable)', patient({ refrDate: null }), P, P],
  ['A19 cylinder absent = 0.00 D (§1 note) -> laser', patient({ refr: both(r(-3)) }), LP, LP],
  ['A20 BAD-D 2.0 without TKC: conduct unchanged + risco_ectasia_nao_confirmado', patient({ bad: { OD: 2.0, OS: 1.0 } }), LP, LP, { obs: 'risco_ectasia_nao_confirmado', obsEye: 'OD' }],
  ['A21 age 39 is Flow A', patient({ birth: '1987-11-18' }), LP, LP, { flow: 'A' }],
  ['A22 KC1 in Flow A -> rule 2.1', patient({ tkc: { OD: 'KC1', OS: null }, dash: ['OS'] }), K, K, { state: 'keratoconus' }],
  ['A23 SE >= 12, ACD 2.8 but endothelium absent -> Pendente (Etapa 6 literal)', patient({ refr: both(r(-13, 0)), acd: both(2.8), withSpec: false }), P, P],
  // ---------------- Flow B ----------------
  ['B01 emmetrope (plano): cornea apt with +2.00 D presbyopic add -> both platforms', B({ refr: both(r(0, 0)) }), PB, PB, { flow: 'B' }],
  ['B02 +2.00: Presbyond and Custom-Q central range', B({ refr: both(r(2, 0)) }), PB, PB],
  ['B03 +4.50: Presbyond only (Custom-Q central +0.50..+3.25)', B({ refr: both(r(4.5, 0)), pachy: both(560) }), PY, PY],
  ['B04 -5.00: Presbyond and Custom-Q (-3..-7)', B({ refr: both(r(-5, 0)), pachy: both(560) }), PB, PB],
  ['B05 -1.50 -1.00: Presbyond and Custom-Q (mild myopia, astig <= 3.00)', B({ refr: both(r(-1.5, -1)) }), PB, PB],
  ['B06 -10.00, cornea apt: outside every documented range', B({ refr: both(r(-10, 0)), km: both(46), pachy: both(650) }), OUT, OUT],
  ['B07 -6.00 Km 39: K_final 33.97 < 34 -> cornea not apt -> ambulatory referral', B({ refr: both(r(-6, 0)), km: both(39), pachy: both(560) }), AMB, AMB, { obs: 'encaminhamento_ambulatorio_presbiopia' }],
  ['B08 -4.00 paq 420: LER < 250 -> not apt', B({ refr: both(r(-4, 0)), pachy: both(420) }), AMB, AMB],
  ['B09 refraction not sent -> Pendente', B({ refr: null }), P, P],
  ['B10 -9.00 boundary: Presbyond only', B({ refr: both(r(-9, 0)), km: both(45), pachy: both(640) }), PY, PY],
  ['B11 +6.00 (> +5.75), cornea apt -> outside range', B({ refr: both(r(6, 0)), km: both(42), pachy: both(600) }), OUT, OUT],
  ['B12 OD plano, OS refraction absent -> eyes differ', B({ refr: { OD: r(0, 0), OS: null } }), PB, P],
  ['B13 KC2 in Flow B -> rule 2.1', B({ tkc: { OD: null, OS: 'KC2' }, dash: ['OD'] }), K, K],
  ['B14 -0.25: Presbyond only (not emmetropic, below Custom-Q mild myopia -0.50)', B({ refr: both(r(-0.25, 0)) }), PY, PY],
  ['B15 -8.50: no night-vision alert / phakic option in Flow B (item 46)', B({ refr: both(r(-8.5, 0)), km: both(42), pachy: both(640), acd: both(3.3) }), PY, PY],
  ['B16 age 40 is Flow B', B({ birth: '1986-11-18', refr: both(r(0, 0)) }), PB, PB, { flow: 'B' }],
  ['B17 age 54 is Flow B', B({ birth: '1971-11-19', refr: both(r(0, 0)) }), PB, PB, { flow: 'B' }],
]

for (const [name, input, od, os, extra = {}] of matrix) {
  test(name, () => {
    const d = evaluateProtocol(input)
    assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], [od, os])
    if (extra.flow) assert.equal(d.flow, extra.flow)
    if (extra.state) assert.equal(d.state, extra.state)
    const eyes = extra.obsEye ? [extra.obsEye] : ['OD', 'OS'].filter((e) => d.eyes[e].conduta !== LP || extra.obs)
    if (extra.obs) assert.ok(eyes.some((e) => d.eyes[e].observations.some((o) => o.type === extra.obs)), 'observation ' + extra.obs)
    if (extra.missing) assert.ok(d.eyes.OD.missing.some((m) => m.includes(extra.missing)), 'missing ' + extra.missing)
  })
}

test('every A/B eye decision explains itself: flow, steps with measurements, certainty', () => {
  const d = evaluateProtocol(patient({ refr: { OD: r(-3, -0.5), OS: null } }))
  assert.equal(d.flow, 'A')
  assert.ok(d.eyes.OD.steps.some((s) => /SE/.test(s.text) && /-3,25|−3,25/.test(s.text)), JSON.stringify(d.eyes.OD.steps))
  assert.ok(d.eyes.OD.steps.some((s) => /LER/.test(s.text)))
  assert.equal(d.eyes.OD.certainty, 'definitiva')
  assert.equal(d.eyes.OS.certainty, 'pendente')
})

test('missing data never becomes a negative or positive: no surgery and no orientation sentence when every eye is pending', () => {
  const d = evaluateProtocol(patient({ refr: null }))
  assert.equal(d.showOrientationSentence, false)
  assert.equal(evaluateProtocol(B({ refr: both(r(-6, 0)), km: both(39), pachy: both(560) })).showOrientationSentence, false)
  assert.equal(evaluateProtocol(patient()).showOrientationSentence, true)
})


// Regression (Schiavini): a refraction date in the future (2028-08-17, a misparsed 2017-08-28 lensmeter slip) was reported
// as "mais de 4 meses do Pentacam". A future exam date is impossible: flagged invalid, it never satisfies the 4-month window.
const TODAY = '2026-11-25'
const stepText = (d, eye) => d.eyes[eye].steps.map((s) => s.text).join(' | ')

test('future refraction date is flagged invalid, not stale (Flow B -> Pendente)', () => {
  const d = evaluateProtocol(B({ refr: both(r(1, -0.75)), refrDate: '2028-08-17 14:36' }), TODAY)
  for (const eye of ['OD', 'OS']) {
    assert.equal(d.eyes[eye].conduta, P)
    assert.match(stepText(d, eye), /refratometria com data futura \(2028-08-17\) — data inválida/)
    assert.doesNotMatch(stepText(d, eye), /mais de 4 meses/)
  }
})

test('future refraction date inside the 4-month span cannot satisfy recency (Flow A -> Pendente)', () => {
  const ok = evaluateProtocol(patient({ refrDate: '2026-11-25 10:00' }), TODAY)
  assert.deepEqual([ok.eyes.OD.conduta, ok.eyes.OS.conduta], [LP, LP]) // same day is valid
  const d = evaluateProtocol(patient({ refrDate: '2026-12-01 10:00' }), TODAY) // 13 days after the Pentacam, but tomorrow+
  assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], [P, P])
  assert.match(stepText(d, 'OD'), /data futura \(2026-12-01\)/)
})

test('future specular microscopy date is invalid for the endothelium checkpoint', () => {
  const d = evaluateProtocol(patient({ refr: both(r(-13, 0)), specDate: '2027-01-10 09:00:00' }), TODAY)
  assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], [P, P])
  assert.match(JSON.stringify(d.eyes.OD), /microscopia com data futura \(2027-01-10\)/)
})

test('a future Pentacam reference date invalidates the window for every exam', () => {
  const input = patient()
  for (const e of input.verificacao_identidade) if (e.source.startsWith('p-')) e.timestamp_lido = '01/10/2027'
  const d = evaluateProtocol(input, TODAY)
  assert.deepEqual([d.eyes.OD.conduta, d.eyes.OS.conduta], [P, P])
  assert.match(stepText(d, 'OD'), /Pentacam com data futura \(2027-01-10\)/)
})
