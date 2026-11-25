// RefratIA end-to-end clinical correctness benchmark (quality gate).
// Data lives OUTSIDE Git (patient data): default backend/benchmark/out/clinical/
//   cases/<P#>/stored.json   saved manifest as persisted (Garage paciente_compilado.json, name stripped)
//   verified-inputs.json     printed source values (independently read), used for OCR accuracy + counterfactuals
//   expected.json            independently derived protocol outcome per eye (never generated from this code)
//   baseline.json            last accepted run (for regression classification)
// Usage (from repo root, no OCR is re-run):
//   node backend/benchmark/clinical/run.mjs [--data DIR] [--browser] [--save-baseline]
//   --browser needs: vite on :5199 (npx vite --config frontend/vite.config.ts --port 5199) and PLAYWRIGHT=<path to playwright/index.mjs>
// Exit code 1 when any verifiable eye regresses vs baseline or a safety violation appears.
// Add a patient: copy its saved manifest to cases/P<n>/stored.json (replace patient.full_name), add printed values to
// verified-inputs.json (independent reading of the original documents) and an independently derived entry to expected.json
// (status UNRESOLVED + question when the protocol or evidence does not settle it). Never derive expected.json from this code.
import { readFileSync, writeFileSync, readdirSync, existsSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { evaluateProtocol } from '../../../frontend/src/utils/protocolDecision.ts'
import { normalizeSavedAnalysis } from '../../../frontend/src/contracts/patient-analysis.ts'
import { formatBirthDate } from '../../../frontend/src/utils/date.ts'

const arg = (k) => { const i = process.argv.indexOf(k); return i < 0 ? null : process.argv[i + 1] ?? true }
const DATA = arg('--data') ?? new URL('../out/clinical/', import.meta.url).pathname
const read = (p) => JSON.parse(readFileSync(DATA + p, 'utf8'))
const EYES = ['OD', 'OS']
const PENDING = 'Pendente — necessidade de repetir ou realizar o exame'
// Outcomes that are not a surgical recommendation (safety accounting).
const NON_SURGICAL = (c) => c === null || /^Pendente|Encaminhamento|Fora da faixa|Não elegível|ceratocone/i.test(c)

// ---------- fixture mutation (counterfactuals) ----------
const get = (o, path) => path.split('.').reduce((v, k) => (v == null ? undefined : v[k]), o)
function set(o, path, value) {
  const ks = path.split('.'); let cur = o
  for (const k of ks.slice(0, -1)) cur = cur[k] ??= {}
  if (value === undefined) delete cur[ks.at(-1)]; else cur[ks.at(-1)] = value
}
// A field override: {path, value} or a special op.
function apply(m, f) {
  if (f.op === 'exam_date') { // set an exam's own date and rewrite the identity timestamps of its sources
    const exam = m.exams?.[f.exam] ?? {}
    const srcs = [exam.source ?? []].flat()
    m.verificacao_identidade = (m.verificacao_identidade ?? []).filter((e) => !srcs.includes(e?.source))
    set(m, `exams.${f.exam}.${f.field ?? 'exam_datetime'}`, f.value)
    return
  }
  if (f.op === 'tkc_dash') { // printed "-": not classified
    set(m, `exams.pentacam_corneal_tomography.eyes.${f.eye}.topometric_indices_8mm.tkc`, null)
    m.extraction_diagnostics = [...(m.extraction_diagnostics ?? []).filter((d) => !(d.field === 'tkc' && d.eye === f.eye)),
      { exam_type: 'pentacam_corneal_tomography', eye: f.eye, field: 'tkc', status: 'not_applicable', stage: 'recognition' }]
    return
  }
  set(m, f.path, f.value)
}

// Stored value of a field whose persisted form is not a single path.
function storedOf(m, f) {
  if (f.stored_path) return get(m, f.stored_path) ?? null
  if (f.stored_kind === 'tkc') {
    const v = get(m, `exams.pentacam_corneal_tomography.eyes.${f.eye}.topometric_indices_8mm.tkc`)
    if (v != null && String(v).trim() !== '') return String(v)
    return (m.extraction_diagnostics ?? []).some((d) => d.field === 'tkc' && d.eye === f.eye && d.status === 'not_applicable') ? '-' : 'unread'
  }
  if (f.stored_kind?.startsWith('exam_date:')) {
    const exam = m.exams?.[f.stored_kind.split(':')[1]]; if (!exam) return null
    const srcs = [exam.source ?? []].flat()
    const own = [exam.exam_datetime, exam.exam_at, exam.measurement_date, ...Object.values(exam.eyes ?? {}).flatMap((e) => [e?.measurement_date, e?.exam_at, e?.calculated_at])]
    const ds = [...own, ...(m.verificacao_identidade ?? []).filter((e) => srcs.includes(e?.source)).map((e) => e.timestamp_lido)]
      .filter((d) => String(d ?? '').trim()).map((d) => String(d).slice(0, 10))
    return ds.length ? [...new Set(ds)].join(' | ') : null
  }
  return f.stored ?? null
}

function evaluate(manifest) {
  const n = normalizeSavedAnalysis(structuredClone(manifest))
  n.patient = { ...n.patient, birth_date: formatBirthDate(n.patient?.birth_date) }
  const d = evaluateProtocol(n)
  return { age: d.age, flow: d.flow, state: d.state, sentence: d.showOrientationSentence,
    eyes: Object.fromEntries(EYES.map((e) => [e, { decision: d.eyes[e]?.conduta ?? null, certainty: d.eyes[e]?.certainty ?? null,
      tkc: d.tkc?.eyes?.[e] ?? null, toric: d.eyes[e]?.toric ?? null, endothelium: d.eyes[e]?.endothelium?.status ?? null,
      steps: (d.eyes[e]?.steps ?? []).map((s) => `${s.rule} [${s.outcome}] ${s.text}`) }])) }
}

const eq = (a, b) => (typeof a === 'number' && typeof b === 'number' ? Math.abs(a - b) < 0.005 : JSON.stringify(a ?? null) === JSON.stringify(b ?? null))
const sha = (o) => createHash('sha256').update(JSON.stringify(o)).digest('hex').slice(0, 12)

// ---------- optional browser layer ----------
async function browserCards(manifests) {
  const { chromium } = await import(process.env.PLAYWRIGHT)
  const browser = await chromium.launch(); const out = {}
  for (const [id, analysis] of Object.entries(manifests)) {
    const page = await browser.newPage({ viewport: { width: 1400, height: 1000 } })
    await page.route('**/*', (route) => {
      const url = new URL(route.request().url())
      if (url.pathname.endsWith('/api/cases')) return route.fulfill({ json: { cases: [] } })
      if (url.pathname.includes('/api/cases/')) return route.fulfill({ json: { analysis } })
      if (!['localhost', '127.0.0.1'].includes(url.hostname)) return route.abort()
      return route.continue()
    })
    await page.goto((process.env.BASE_URL ?? 'http://localhost:5199/refratia') + '/relatorios/case-20990101T000000Z-0000abcd')
    await page.getByText('Acompanhe a decisão, passo a passo').waitFor({ timeout: 15000 })
    const main = await page.locator('main').innerText()
    out[id] = { flow: main.match(/Fluxo [ABC]/)?.[0] ?? null, eyes: {} }
    for (const e of EYES) {
      const card = await page.getByTestId('conduta-' + e).innerText({ timeout: 1500 }).catch(() => null)
      out[id].eyes[e] = card === null ? { decision: null, certainty: null }
        : { decision: card.split('\n')[1] ?? null, certainty: /PENDENTE/.test(card) ? 'pendente' : /CONDICIONAL/.test(card) ? 'condicional' : /DEFINITIVA/.test(card) ? 'definitiva' : null }
    }
    await page.close()
  }
  await browser.close(); return out
}

// ---------- run ----------
const expected = read('expected.json').cases
const verified = existsSync(DATA + 'verified-inputs.json') ? read('verified-inputs.json').cases : {}
const ids = readdirSync(DATA + 'cases').sort()
const stored = Object.fromEntries(ids.map((id) => [id, read(`cases/${id}/stored.json`)]))
const browser = arg('--browser') ? await browserCards(stored) : null
const rows = []; const ocr = []; const impacts = []; const checkpoints = []
for (const id of ids) {
  const exp = expected[id]; const v = verified[id]?.fields ?? []
  if (!exp) { console.warn(`${id}: no expected entry, skipped`); continue }
  const corrected = structuredClone(stored[id]); for (const f of v.filter((f) => f.status === 'VERIFIED' && f.fix)) apply(corrected, f.fix)
  const assumed = structuredClone(corrected); for (const f of v.filter((f) => f.status === 'ASSUMED' && f.fix)) apply(assumed, f.fix)
  const A = evaluate(stored[id]); const CF = evaluate(corrected); const CFA = evaluate(assumed)
  // OCR accuracy on decision-relevant fields (stored vs printed)
  for (const f of v.filter((f) => f.status === 'VERIFIED')) {
    const storedValue = storedOf(stored[id], f)
    const ok = f.compare === 'abs' ? eq(Math.abs(storedValue ?? NaN), Math.abs(f.printed)) || (storedValue == null && f.printed == null) : eq(storedValue, f.printed)
    let changes = null
    if (!ok && f.fix) {
      // Impact = leave-one-out: all verified (+assumed) printed values applied except this one, so errors that only
      // matter together (e.g. sphere and date) are attributed; plus the single fix applied to the stored data.
      const loo = structuredClone(stored[id]); for (const g of v.filter((g) => g !== f && ['VERIFIED', 'ASSUMED'].includes(g.status) && g.fix)) apply(loo, g.fix)
      if (f.stored_path) set(loo, f.stored_path, storedValue ?? undefined)
      const one = structuredClone(stored[id]); apply(one, f.fix)
      const [L, O] = [evaluate(loo), evaluate(one)]
      const diff = (x, y, tag) => EYES.filter((e) => x.eyes[e].decision !== y.eyes[e].decision || x.eyes[e].certainty !== y.eyes[e].certainty)
        .map((e) => `${e} ${tag}: "${x.eyes[e].decision}" -> "${y.eyes[e].decision}"`)
      changes = [...diff(L, CFA, 'given other fields correct'), ...diff(A, O, 'fixed alone')]
      impacts.push({ id, field: f.field, eye: f.eye, stored: storedValue ?? null, printed: f.printed, changes })
    }
    ocr.push({ id, eye: f.eye, field: f.field, relevant: f.relevant !== false, ok, stored: storedValue ?? null, printed: f.printed, changes })
  }
  // Metric C: protocol checkpoints that the expected outcome fixes unambiguously (rule 2.1 TKC; Flow C toric + endothelium).
  for (const e of EYES) {
    const x = exp.eyes[e]; const a = A.eyes[e]
    if (/^-/.test(exp.tkc ?? '')) checkpoints.push({ id, eye: e, cp: 'rule 2.1 TKC not classified', ok: a.tkc === 'not_classified', actual: a.tkc })
    if (exp.flow === 'C' && x.status === 'VERIFIED') {
      checkpoints.push({ id, eye: e, cp: 'Flow C toric (biometry AST >= 0.75)', ok: a.toric === /Tórica/.test(x.decision), actual: a.toric })
      const cond = /avaliação endotelial/.test(x.decision)
      checkpoints.push({ id, eye: e, cp: 'Flow C endothelium ' + (cond ? '< 2000 conditions' : '>= 2000 adequate'), ok: a.endothelium === (cond ? 'condiciona_conduta' : 'aprovado'), actual: a.endothelium })
    }
  }
  for (const e of EYES) {
    const x = exp.eyes[e]; const a = A.eyes[e]; const c = CF.eyes[e]; const ca = CFA.eyes[e]; const b = browser?.[id]?.eyes[e]
    const unresolved = x.status !== 'VERIFIED'
    const match = (r) => r.decision === x.decision && (!x.certainty || r.certainty === x.certainty)
    const pass = unresolved ? null : match(a)
    rows.push({ id, eye: e, expFlow: exp.flow, flow: A.flow, flowOk: A.flow === exp.flow, expected: x.decision, expCertainty: x.certainty ?? null,
      actual: a.decision, certainty: a.certainty, pass, unresolved, counterfactual: c.decision, cfPass: unresolved ? null : match(c), cfAssumed: ca.decision, cfAssumedCertainty: ca.certainty, alternatives: x.alternatives ?? [],
      browser: b?.decision, browserMatchesEvaluator: b ? b.decision === a.decision && b.certainty === a.certainty : null,
      cause: unresolved ? 'unresolved: ' + (x.question ?? '') : pass ? '' : match(c) ? 'input (OCR/extraction) — fixed by verified printed values' : 'evaluator/protocol',
      // Unsupported positive: a surgical conduct that is neither the expected one nor a documented alternative.
      // A generic "LIO Multifocal/EDOF" (no toric claim) under an expected LIO variant is imprecise, not unsupported.
      unsupportedPositive: !NON_SURGICAL(a.decision) && ![x.decision, ...(x.alternatives ?? []).map((y) => y.decision)].includes(a.decision)
        && !(/^LIO Multifocal\/EDOF/.test(a.decision) && /^LIO Multifocal/.test(x.decision)),
      missedExclusion: !!x.exclusion && a.decision !== x.decision,
      unjustifiedDefinitive: a.certainty === 'definitiva' && x.certainty !== 'definitiva',
      // Any output outside the documented outcomes of an UNRESOLVED eye (e.g. a referral the alternatives never allow).
      outsideDocumented: unresolved && ![x.decision, ...(x.alternatives ?? []).map((y) => y.decision)].includes(a.decision),
      // Exclusion / referral / out-of-range emitted where the expected outcome (and its alternatives) has none.
      wrongExclusion: /Encaminhamento|Fora da faixa|ceratocone|Não elegível/i.test(a.decision ?? '') && ![x.decision, ...(x.alternatives ?? []).map((y) => y.decision)].includes(a.decision),
      rule: x.rule ?? '', inputs: sha(stored[id]) })
  }
}

// ---------- metrics ----------
const pct = (n, d) => `${n}/${d}${d ? ` (${Math.round((100 * n) / d)}%)` : ''}`
const ver = rows.filter((r) => !r.unresolved)
const pts = [...new Set(rows.map((r) => r.id))]
const flowOk = pts.filter((id) => rows.find((r) => r.id === id).flowOk).length
const metrics = {
  A_ocr_input_accuracy_decision_relevant: pct(ocr.filter((o) => o.relevant && o.ok).length, ocr.filter((o) => o.relevant).length),
  A_ocr_input_accuracy_all_verified_fields: pct(ocr.filter((o) => o.ok).length, ocr.length),
  B_flow_accuracy: pct(flowOk, pts.length),
  C_protocol_checkpoint_accuracy: pct(checkpoints.filter((c) => c.ok).length, checkpoints.length),
  C_failed_checkpoints: checkpoints.filter((c) => !c.ok).map((c) => `${c.id} ${c.eye} ${c.cp}: actual ${c.actual}`),
  D_final_verdict_accuracy: pct(ver.filter((r) => r.pass).length, ver.length),
  D_counterfactual_with_printed_inputs: pct(ver.filter((r) => r.cfPass).length, ver.length),
  unresolved_eyes: rows.filter((r) => r.unresolved).map((r) => r.id + ' ' + r.eye),
  E_unsupported_positive: rows.filter((r) => r.unsupportedPositive).map((r) => r.id + ' ' + r.eye),
  E_missed_exclusions: rows.filter((r) => r.missedExclusion).map((r) => r.id + ' ' + r.eye),
  E_unjustified_definitive: rows.filter((r) => r.unjustifiedDefinitive).map((r) => r.id + ' ' + r.eye),
  E_wrong_exclusion_or_referral: rows.filter((r) => r.wrongExclusion).map((r) => r.id + ' ' + r.eye),
  E_outside_documented_outcomes_unresolved: rows.filter((r) => r.outsideDocumented).map((r) => r.id + ' ' + r.eye),
  browser_matches_evaluator: browser ? `${pct(rows.filter((r) => r.browserMatchesEvaluator).length, rows.length)}; with a rendered card: ${pct(rows.filter((r) => r.browserMatchesEvaluator && r.actual !== null).length, rows.filter((r) => r.actual !== null).length)}` : 'not run',
  ocr_errors_changing_verdict: impacts.filter((i) => i.changes?.length).length,
  ocr_errors_without_verdict_change: impacts.filter((i) => !i.changes?.length).length,
}

// ---------- regression vs baseline ----------
const changes = []
if (existsSync(DATA + 'baseline.json')) {
  const base = Object.fromEntries(read('baseline.json').rows.map((r) => [r.id + r.eye, r]))
  for (const r of rows) {
    const b = base[r.id + r.eye]; if (!b) { changes.push(`${r.id} ${r.eye}: NEW case`); continue }
    const tag = (t) => changes.push(`${r.id} ${r.eye}: ${t} ("${b.actual}" -> "${r.actual}")`)
    if (b.pass === false && r.pass === true) tag('FIXED wrong recommendation')
    if (b.pass === true && r.pass === false) tag('REGRESSION: broke a correct protocol decision')
    if (b.pass !== false && r.pass === false && b.actual !== r.actual && !NON_SURGICAL(r.actual)) tag('NEW incorrect recommendation')
    if (b.certainty !== 'pendente' && r.certainty === 'pendente') tag('definitive/conditional -> pending')
    if (b.certainty === 'pendente' && r.certainty !== 'pendente' && b.inputs === r.inputs) tag('pending -> definitive WITHOUT new evidence (inputs unchanged)')
    if (b.pass === false && r.pass === false && (b.actual !== r.actual || b.certainty !== r.certainty)) tag(`CHANGED while still wrong [${b.certainty} -> ${r.certainty}]`)
  }
}

console.log('Patient | Eye | Exp flow | Flow | Expected | Actual (stored) | Status | CF printed | CF +assumed dates | Browser | Root cause | Rule')
for (const r of rows) console.log([r.id, r.eye, r.expFlow, r.flow, `${r.expected}${r.expCertainty ? ' [' + r.expCertainty + ']' : ''}`, `${r.actual} [${r.certainty}]`,
  r.unresolved ? 'UNRESOLVED' : r.pass ? 'PASS' : 'FAIL', r.counterfactual, r.cfAssumed, r.browser ?? '-', r.cause, r.rule].join(' | '))
console.log('\nOCR errors (stored != printed):'); for (const o of ocr.filter((o) => !o.ok)) console.log(`  ${o.id} ${o.eye} ${o.field}${o.relevant ? '' : ' (not used by this flow)'}: stored ${JSON.stringify(o.stored)} printed ${JSON.stringify(o.printed)} -> verdict change: ${o.changes?.length ? o.changes.join('; ') : 'none'}`)
console.log('\nMetrics', JSON.stringify(metrics, null, 1))
console.log('\nChanges vs baseline:', changes.length ? '\n  ' + changes.join('\n  ') : 'none')
const result = { rows, ocr, impacts, checkpoints, metrics, changes }
writeFileSync(DATA + 'last-run.json', JSON.stringify(result, null, 1))
if (arg('--save-baseline')) writeFileSync(DATA + 'baseline.json', JSON.stringify({ rows }, null, 1))
// Gate: regressions, unexplained upgrades, and every safety counter (an UNRESOLVED eye is still gated on safety).
const bad = changes.some((c) => /REGRESSION|NEW incorrect|WITHOUT new evidence|CHANGED while still wrong/.test(c)) ||
  ['E_unsupported_positive', 'E_missed_exclusions', 'E_unjustified_definitive', 'E_wrong_exclusion_or_referral', 'E_outside_documented_outcomes_unresolved'].some((k) => metrics[k].length)
process.exit(bad ? 1 : 0)
