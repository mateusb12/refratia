import test from 'node:test'
import assert from 'node:assert/strict'
import { reportHeaderSummary } from '../src/features/reports/reportModel.ts'

const analysis = (bioBirth) => ({
  generated_on: '2026-04-04',
  patient: { birth_date: '1986-05-03' },
  exams: {
    pentacam_corneal_tomography: { source: ['p_od.pdf'] },
    refractometry: { eyes: { OD: { sphere_d: -2.5, cylinder_d: -0.75, axis_deg: 180 } } },
  },
  verificacao_identidade: [
    { source: 'BIOMETRIA.pdf', nascimento_lido: bioBirth, date_order: 'DMY', timestamp_lido: '2026-04-04 10:00' },
    { source: 'p_od.pdf', nascimento_lido: '05/03/1986', date_order: 'MDY', timestamp_lido: '04/04/2026' },
  ],
})

test('header age and flow come from the analysis, not fixed text', () => {
  const summary = reportHeaderSummary(analysis('03/05/1986'))
  assert.equal(summary.age, '39 anos')
  assert.match(summary.flow, /^Fluxo A/)
  assert.equal(summary.identity, 'Confirmada por 2 documento(s)')
  assert.equal(summary.refraction, 'OD -2,50 -0,75 × 180° · OS: não informado')
})

test('item 45(g): a day/month birth inversion is noted on the identity line, with both readings when the flow changes', () => {
  const identity = reportHeaderSummary(analysis('05/03/1986')).identity
  assert.match(identity, /^Confirmada por 2 documento\(s\)\. Datas de nascimento divergem/)
  assert.match(identity, /Leitura invertida: .*Fluxo B/)
  assert.match(identity, /item 45g/)
})
