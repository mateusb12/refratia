# Ingestion reliability: final report

Branch `fix/ingestion-reliability` (31 commits on `main`, not pushed). Engineering reference: **`cac8a1b`** (option D), gate run `backend/benchmark/out/20261113-150559`, scored with scorer **`048e9b6`**. `backend/benchmark/last_pass.json` is regenerated offline from that completed gate (no extra OCR run).

All numbers below are rescored with the same scorer (`048e9b6`) against the same ground truth, so they compare directly. The denominator is 544 everywhere: the 8 IOL `measurement_date` fields gained an app path during this work, so older runs count them as missing instead of excluding them. Reports quoted earlier at /536 used the earlier spec.

## Before and after

| | Baseline (`9eb5152`, run 213130) | Reference D (`cac8a1b`, run 150559) |
|---|---|---|
| Field accuracy | 190/544 (34.9%) | **509/544 (93.6%)** |
| Required roles (decisive, cross_check, identity) | 93/180 (51.7%) | **177/180 (98.3%)** |
| Storage, SQL and identity checks | 61/101; 0 exam rows; rolmey rejected (422) | **108/108** |
| Wrong values (incorrect/conflicting) | many, incl. invented placeholders | **1** (informative field) |

Per patient:

| Patient | Fields: baseline → D | Required: baseline → D | Checks: baseline → D |
|---|---|---|---|
| creso-aversa | 56/110 → **104/110** | 22/36 → **36/36** | 12/19 → **20/20** |
| daniela-nogueira | 10/84 → **80/84** | 7/26 → **26/26** | 11/20 → **21/21** |
| francisco | 35/114 → **111/114** | 20/38 → **38/38** | 16/23 → **24/24** |
| maria-gorete | 55/122 → **113/122** | 25/42 → **41/42** | 14/20 → **21/21** |
| rolmey | 34/114 → **101/114** | 19/38 → **36/38** | 8/19 → **22/22** |

Gate history (same scorer): `c4bc7c2` 460/544 (required 158/180), `559f373` 503 (171), `7182b1e` (option A) 509 (177), option C 506 (174), **`cac8a1b` (D) 509 (177)**.

The 108 checks per run cover, per patient: exam rows in SQL matching ground truth `expected_exam_records` (type, eye, `UNIQUE(object_key, eye)`), one patient row with the right identity, Garage objects at deterministic keys plus `paciente_compilado.json`, the receipt, idempotent re-confirm (3 confirms → same rows and objects), re-ingest stability, and identity warnings (francisco `data_nascimento_invertida`). Separately, `confirm_integration_test.go` covers an SQL failure after Garage writes: reconcile lists the debris and a retry converges.

## Tradeoff of D versus A

A (`7182b1e`) accepted a cell's whole-page token when it held a 3:1 majority of the grey-scale reads. That 75% bar was fitted to this corpus: 80% lost a field, 2/3 let a wrong 3-vs-9 read (creso OD coma 0.03 instead of 0.09) through as valid. D removes the bar and instead uses a printed physical identity: external ACD = internal ACD + vertex pachymetry/1000. The tolerance, 0.005 + 0.005 + 0.0005, comes from the printed decimals. D selects among values the OCR actually read, never computes one, and fills a cell only when every consistent assignment agrees.

| Field | A | D | Why |
|---|---|---|---|
| rolmey OD acd_ext_mm (required) | missing | **3.9 ✓** | Only 3.9 of the reads {3.3, 3.5, 3.9} fits internal 3.36 + vertex |
| rolmey OS acd_ext_mm (required) | missing | **3.9 ✓** | Same, internal 3.37 |
| maria-gorete OS pachy_pupil_center_um (required, cross_check) | 545 ✓ | missing | Reads 545:17 / 645:6; no printed identity involves this cell |
| rolmey OD pachy_pupil_center_um (required, cross_check) | 534 ✓ | missing | Reads 534 / 538 / 634; no identity involves this cell |

The totals are identical (509, required 177) and neither version has a new wrong value. D has no corpus-tuned constant; A's two pupil-pachymetry passes rested on one.

## Documented known gaps (kept in the benchmark, not passing, ground truth unchanged)

**Regressed versus A**: the two pupil-pachymetry fields in the table above.

**Unresolved required fields (3)**, all reported missing, never guessed:
1. maria-gorete OS `pachy_pupil_center_um` (truth 545): reads split 545/645 (a 5→6 glyph flip). No independent evidence. The agent declined a "pupil centre near vertex" rule because its tolerance cannot be derived from printed decimals.
2. rolmey OD `pachy_pupil_center_um` (truth 534): reads 534/538/634.
3. rolmey OD `pachy_vertex_um` (truth 534): the ACD identity excludes 634, but 534 and 538 both satisfy it within 0.0105 mm. The Cataract screen also prints vertex pachymetry, but its page-OCR label merges with the cell border ("Ni534") and is not located. A garble-tolerant label would be a second independent printing; not implemented.

**Remaining wrong informative value**: maria-gorete OD `coma_z31_5mm_axis_deg` reads **3** (truth 93): the leading 9 is lost. Downstream (checked against the completed gate's bench state):
- It is **persisted**: Garage `paciente_compilado.json` holds `z31_coma_axis_deg: 3` for that eye (verified in bucket `refratia-bench`). It is in the analyze response and its `extraction_diagnostics` say `valid`.
- It is **not in SQL**: `exams` rows carry no extracted values.
- It is **not displayed or consumed**: no frontend code reads `z31_coma_axis_deg` (`App.tsx`, `contracts/exam-contracts.ts` and `contracts/patient-analysis.ts` read only the coma magnitude `z31_coma`), and no backend logic reads it besides the Pentacam extractor that writes it.
- Risk: low today, but anything that later reads the compiled JSON would get a wrong axis marked valid. Fix candidates: a range/consistency check against the 6 mm axis, or require the axis cell to pass the same split rule as other cells.

Other non-passing fields (all missing, none required): `abcd_staging` on all 10 Pentacam eyes (mapping), `isv` 5, `axis_flat_deg` 3, coma 6 mm and axis cells 5, refraction `vertex_distance_mm` 4, rolmey IOL K1/K2 axes 4 (role `extra`). 98 ground-truth fields have no app output path (`not_in_app`, excluded from accuracy): 80 informative Pentacam aberrometry, 12 on out-of-protocol documents (optional by product decision), 6 extras. No required field is in this set.

## Follow-ups

1. **Fixed-box fallback** (`features/pentacam/cell_ocr.go`, `fillPentacamFocusedCells`): when a label-anchored cell is split, a fixed pixel-box crop of the same cell is read instead. It supplies creso OD coma 0.09 (anchor votes 0.03:19 / 0.09:18). It is not independent evidence, it contradicts "split cells end missing", and the boxes are page offsets. Before replacing it, add a test on an independently labeled Pentacam report layout (not one of these five), so the replacement is not designed against the same files.
2. **Held-out validation**: the five cases now detect regressions. Whether the fixes generalize needs a Pentacam report not used to design them, labeled independently, then one gate including it.
3. Coma-axis wrong value (above). Then vertex pachymetry via a garble-tolerant Cataract-screen label.

## Root causes fixed (summary; details in `ocr-investigation.md`, `persistence-investigation.md`)

- Persistence: identity resolved before any Garage write (422/409 instead of silent near-duplicate patients); one exam row per (document, eye) with `UNIQUE(object_key, eye)`; per-document birth-date order; no age 0; SQL failure after Garage writes is reconcilable and retry-idempotent.
- OCR: content-based classification and eye detection; Pentacam label-anchored reads with the later screens used only as fallbacks; keratometry (Km = (K1+K2)/2, astig = K2−K1) and ACD identities that select among read candidates; specular row tolerance scaled with the re-read zoom; refraction average row voting; IOL measurement date; printed-format normalisation (unit glyph read as a digit).
- Benchmark: contract mismatches count as failures; dash placeholders are wrong values, not missing; typed coma axis compared directly; out-of-protocol fields optional; run state with heartbeat/resume; infra vs app failures and exit codes.
