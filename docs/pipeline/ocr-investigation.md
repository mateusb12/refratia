# OCR / local extraction investigation (phase 1)

Author: OCR investigator. Date: 2026-11-12. Baseline code: commit `9eb5152` (HEAD). The harness was compiled at 19:46, before the persistence agent's working-tree edits to `features/patient/identity.go` (20:12), so the identity results below are HEAD behaviour.

Scope: read-only investigation. No application code was changed. Everything ran locally. `OPENAI_API_KEY` was unset and the harness refuses to start if it is set. No patient data left the machine.

Privacy note: this file names patients only by corpus folder slug. Birth dates are shown only where the day/month order is the evidence, and then with the year masked (`YYYY`). Raw renders, crops and JSON dumps stay in the session scratchpad, not in the repo.

## 0. How the baseline was produced (reproducible)

- I injected a scratch test, `zz_corpus_harness_test.go` in `package main`, with `go test -overlay`. Nothing was written to the repo. It calls the real `extractPatientLocal` once per corpus file, and once per patient folder with all files together (one intake upload). There is no storage context, so checkpoints are bypassed.
- Classification metadata comes from `CanonicalClassificationFromFilename` → `ExamTypeFromContract`, which is what the frontend sends. The second dispatch mode is exactly what `intake_handlers.go:138-142` does when no classification is sent (see RC-6).
- It also calls `pentacam.ExtractIdentity` and `pentacam.DetectEye` directly, plus `patient.CanonicalBirthDate` per file and per patient.
- Values were compared field by field with `backend/benchmark/groundtruth/<folder>.json`.
- Command:
  `set -a; . backend/testinfra/test.env; set +a; unset OPENAI_API_KEY; HARNESS_CORPUS=… HARNESS_OUT=… HARNESS_MODE=label go test -overlay overlay.json -run TestCorpusHarness .`
  Runtime is 26 min. Most of it is Pentacam: 7 pages at 450 DPI, with many Tesseract calls per file.
- Scratch artefacts are under `$SCRATCH/ocr/`:
  - `harness/` holds the test, the overlay, `summarize.py` and `compare.py`.
  - `out/*.label.json` holds the raw per-file output.
  - `out/compare.txt` holds the comparison with ground truth.
  - `titles/` and `crops/` hold the page renders.

Totals over the app contract fields that have a valid ground-truth value:

| Correct | Wrong value | Missing |
|---|---|---|
| 179 | **11** | 92 |

"Wrong value" means a number was emitted and it is not what is printed. That is worse than missing.

## 1. Extraction architecture map

| File | Responsibility | Hazards found |
|---|---|---|
| `intake_handlers.go` `analyzeIntakeHandler` | Reads the multipart upload. Applies the frontend `classifications` (by sha256), otherwise falls back to the filename. | The filename fallback (l.138-142) stores the **contract key** (`pentacam_corneal_tomography`) in `Metadata.ExamType`. Every consumer expects the **label** (`PENTACAM`). See RC-6. |
| `features/intake/classification.go` | `CanonicalClassificationFromFilename` (structured `TYPE__EYE__NAME__TS` plus a free-form regex), `ParseClassifications`, `ExamTypeFromContract`, and the PDF page-count prefilter. | PDFs are classified **only** by filename. The prefilter says "8–10 pages ⇒ Pentacam", but real Pentacams have 9, 11 or 12 pages. There is no content-based classification, so `526c239f-TOPO.pdf` and `CORNEA__*` are never typed. |
| `frontend/.../filenameClassification.ts`, `local_extraction.go localExamTypeFromFilename` | Two more filename classifiers. | That makes three rule sets that can drift apart. |
| `extraction.go` `extractPatient` | Runs local extraction, then gaps, then the optional OpenAI fallback, then IOL repair (OpenAI), then `enrichSourceFiles`. | It must stay key-less. Without a key, gaps are only recorded. |
| `local_extraction.go` `extractPatientLocal` | Per-file dispatch by `ExamType` label. RETINA and MICROSCOPIA_ESPECULAR go to their extractors. Any other non-PDF is marked "identified". PDFs go to AUTO, EYESUITE or PENTACAM. An unknown PDF tries EyeSuite, then Pentacam. | <ul><li>Any non-PDF other than RETINA/MICROSCOPIA is never extracted (for example the photographed AUTO).</li><li>The unknown-type path emits **no `file_result`**.</li><li>The `try*Local` functions return `bool`, so the failure reason is thrown away.</li></ul> |
| `pentacam_integration.go` | `DetectEye`, `ExtractPDF`, `ExtractIdentity`. Merges identity into `patient` and `verificacao_identidade`. Computes Pentacam gaps. | `mergeLocalPatientIdentity` canonicalizes the birth date **before** it appends its own `verificacao_identidade` entry. So `patient.birth_date` keeps the **raw** device string (l.99-103). |
| `features/pentacam/extractor.go` | Chooses the page layout **by page count**: 12 by default, 11 shifted by one. Renders 7 pages at 450 DPI and OCRs fixed regions (300-DPI A4 coordinates, scaled). Maps label regex to number. | There is no 9-page layout. On 9-page reports, pages 10–12 do not exist and pages 8/9 are different screens (RC-1). |
| `features/pentacam/cell_ocr.go` | Second pass over tiny fixed crops per field (ART max 42×31, IVA 52×31, Z40 62×45 in 300-DPI units), voting across thresholds and PSMs. | <ul><li>Per-field "decimal recovery" hacks: IHA `55`→`5.5`, Z40 `0173`→`0.173`, Kmax `4420`→`44.20`.</li><li>Field-specific PSM lists, with comments citing specific patients ("Fixtures reais Rolmey").</li><li>Z31/Z40 recovery forces a positive sign.</li></ul> |
| `features/pentacam/{adaptive_rescue,badd_deterministic,k1_deterministic}.go` | Rescue readers that use more fixed boxes. | BAD-D reads 3 digit glyphs at fixed x columns, which assumes a positive `d.dd` value. BAD-D can be negative or ≥10. |
| `features/pentacam/identity.go` | Name, birth date and exam date from pages 1 and 7 at 300 DPI, PSM 6 and 11. Keeps the candidate with the most name words. | <ul><li>`dateAfterLabel` also scans the **two rows after** the label, so a missed "Nasc." returns the "Data de Exame" date (RC-3).</li><li>Hard-coded `ReplaceAll("J<SURNAME>","<SURNAME>")` (`identity.go:141`) patient patch (RC-5).</li><li>"More words wins" prefers garbage tokens.</li></ul> |
| `features/pentacam/detector.go` | Laterality from OCR of **page 9** ("direito"/"esquerdo"). | The screen on page 9 differs by layout. It fails for PDFs shorter than 9 pages. |
| `features/eyesuite/{extractor,parser}.go` | Page 1 at 300 and 450 DPI, split at the x midpoint into OD/OS, label regex. Identity is the row above "Calculation ID". | <ul><li>**All-or-nothing:** if any one of 10 fields is missing in either eye, the whole exam errors.</li><li>Identity requires the timestamp too, otherwise the birth date is dropped.</li><li>The timestamp is converted to ISO inside the parser, so the device date order is lost.</li></ul> |
| `features/refractometry/extractor.go` | PRK-6000 ticket, PDF only. Page 1 at 600 DPI. `<R>/<L>` markers plus the `AVE` row. | <ul><li>No range or step validation (axis 188 accepted).</li><li>JPEG photos of the same ticket are not supported.</li></ul> |
| `features/specularmicroscopy/extractor.go` | NIDEK JPEG. Two relative panels (top = R, bottom = L). "Cell Density" regex on the raw crop with PSM 6. | <ul><li>No upscaling or binarization, so a whole row can be lost (daniela OD).</li><li>The `REFRATIA_OCR_DEBUG` redaction misses the name line.</li><li>`REFRATIA_OCR_DEBUG_SAVE` writes fixed `/tmp/specular-*.png`.</li></ul> |
| `features/retinography/extractor.go` | Retina legend: relative top-right ROI, then ID / Eye / Mode=Retina / Time. | <ul><li>Relative ROI (good).</li><li>The error string embeds raw OCR (`OCR=%q`), including the patient name, and it flows into `file_result.message`.</li></ul> |
| `features/patient/identity.go` `CanonicalBirthDate` (HEAD) | Infers the date order **globally across all documents** from `timestamp_lido`. A conflict or ambiguity yields no birth date. | Core of RC-2. It is being rewritten by the persistence agent (`ResolveBirthDate`, per-document). |
| `shared/examdate/examdate.go` `FromOCR` | `a/b/yyyy` is read as DMY unless b>12. | It knows nothing about the device. The Pentacam prints MDY. |
| `shared/ocr/tsv.go` `GroupRows(words, tolPx)` | Row grouping. | The tolerance is in absolute pixels (10/12/40) at any DPI. |
| `frontend/src/App.tsx` `reportHeaderSummary` + `utils/date.ts calculateAge` | Age = `patient.birth_date` against the **first** `verificacao_identidade[].timestamp_lido`. | <ul><li>Accepts `age === 0`.</li><li>Raw MDY strings are parsed as DMY unless day > 12.</li><li>The reference date depends on file order.</li></ul> |
| `benchmark_detection.go` | `/api/benchmark/extract-fields`: per-file `benchmarkField{key,label,found,value}`. | Requires structured filenames. It is the closest existing thing to per-field diagnostics. |

## 2. Corpus facts that the code does not model

- **Pentacam page sets differ.** I rendered every page and checked the title band. Thumbnails are in `$SCRATCH/ocr/montage_*.png`.

  | Pages | Files | Screen order |
  |---|---|---|
  | 12 | creso OD, gorete OD | 1 Geral, 2 Scheimpflug, 3 Mapas, 4 4 Mapas Refrativos, 5–7 Aberrometry Cornea ×3, 8 Paquimétrico, 9 Topométrico/KC, 10 Cataract Pre-OP, 11 Belin/Ambrósio, 12 Anéis Corneanos |
  | 11 | creso OS, gorete OS | Same as the 12-page set with one Aberrometry page fewer |
  | **9** | **daniela OD/OS, francisco OD/OS, rolmey OD/OS (6 of 10 files)** | 1–4 same, 5 Paquimétrico, 6 Topométrico, 7 Cataract Pre-OP, 8 Belin/Ambrósio, 9 Anéis (no Aberrometry pages) |

- **Date order is a device property.** The protocol and `field-spec.md` agree: the Pentacam prints **MM/DD/YYYY** for both birth and exam date. EyeSuite/LS900 prints **DD/MM/YYYY** birth and `DD/MM/YY` exam date. NIDEK microscopy and PRK-6000 print `DD/MON/YYYY`. Retina prints ISO.
- **The same birth date has different strings per device.**
  - rolmey: Pentacam `05/12/YYYY`, EyeSuite `12/05/YYYY` (both ambiguous on their own).
  - francisco: both print `08/05/YYYY`. Read per device, that is a day/month **inversion** (protocol 0.B `data_nascimento_invertida`).
- **Free-form filenames are common** (creso and gorete folders). One file has no classifiable name at all (`526c239f-TOPO.pdf`, a TP-15 topographer with a text layer and `Idade:` but no birth date). Two exam types are outside the classifier vocabulary (`CORNEA__…`, an anterior-segment photo from the retina camera with `Mode Cornea`).
- **One refraction is a phone photo (JPEG) of the PRK-6000 ticket** (daniela). The other is a scanned PDF of the same printer (gorete). The scan also contains a NIDEK NT-530 tonometer ticket with `[R] [L]` markers and a handwritten birth date.
- **The NIDEK header layout varies.** In gorete the patient name is in the `ID` field. In daniela, `ID` holds the clinic and the name is in `Name:`.

## 3. Per-file baseline (patient × file × detected type × fields)

"Contract fields" means the fields of the app's minimum contract that have a valid ground-truth value. "OK / wrong / missing" is the comparison with ground truth.

| Patient | File | Classified as | Status emitted | OK / wrong / missing | Notes |
|---|---|---|---|---|---|
| creso-aversa | BIOMETRIA SRKT AO.pdf | EYESUITE/AO (free-form regex) | extracted | 20/0/0 | Birth date DMY `14/12`, unambiguous |
| creso-aversa | MICROSCOPIA ESPECULAR.jpeg | MICROSCOPIA_ESPECULAR/AO | extracted | 2/0/0 | |
| creso-aversa | PENTACAM … OD .pdf (12 p) | PENTACAM/OD | extracted | 18/0/0 | This is the folder the Pentacam code was tuned on |
| creso-aversa | PENTACAM … OS .pdf (11 p) | PENTACAM/OS | extracted | 17/**1**/0 | ISV 7, printed 17 (leading `1` lost) |
| daniela-nogueira | 526c239f-TOPO.pdf | — (unclassified) | **no file_result event** | n/a | Silently "reserved for fallback" |
| daniela-nogueira | AUTO__AO__….jpeg | AUTO/AO | identified (not extracted) | 0/0/6 | Non-PDF AUTO has no extractor |
| daniela-nogueira | MICROSCOPIA_ESPECULAR__AO__….jpg | MICROSCOPIA_ESPECULAR/AO | partial | 1/0/1 | OD panel: the "Cell Density" row is unreadable in the raw crop |
| daniela-nogueira | PENTACAM__OD (9 p) | PENTACAM/OD | **extracted** | 3/**1**/14 | ISV 44 (printed 42) read from the wrong screen |
| daniela-nogueira | PENTACAM__OS (9 p) | PENTACAM/OS | **extracted** | 4/**1**/13 | Identity name read from Scheimpflug segment text (`"° 2/25 Segmento: 294° … <SURNAME>"`). Exam date empty |
| francisco | CORNEA__OD / __OS .jpeg | — (`CORNEA` unknown) | identified | n/a | Out of protocol (anterior-segment photo) |
| francisco | EYESUITE__AO__….pdf | EYESUITE/AO | extracted | 20/0/0 | Birth date `08/05/YYYY` alone ⇒ ambiguous ⇒ no birth date |
| francisco | MICROSCOPIA_ESPECULAR__AO | MICROSCOPIA_ESPECULAR/AO | extracted | 2/0/0 | |
| francisco | PENTACAM__OD (9 p) | PENTACAM/OD | **extracted** | 3/**1**/14 | ISV 44 (printed 9). Name has `J` border artifacts |
| francisco | PENTACAM__OS (9 p) | PENTACAM/OS | **extracted** | 4/**1**/13 | ISV 44 (printed 14) |
| francisco | RETINA__OD / __OS | RETINA | extracted | (id/eye/mode/time all present) | |
| maria-gorete | AUTO AO MARIA GORETE.pdf | AUTO/AO (free-form) | extracted | 4/**2**/0 | OD cylinder −1.58 (printed −1.50). OD axis **188** (printed 100, impossible > 180) |
| maria-gorete | BIOMETRIA AO MARIA GORETE.pdf | EYESUITE/AO | extracted | 20/0/0 | |
| maria-gorete | MICROSCOPIA AO MARIA GORETE.jpeg | MICROSCOPIA_ESPECULAR/AO | extracted | 2/0/0 | |
| maria-gorete | PENTACAM OD (12 p) | PENTACAM/OD | extracted | 14/**1**/3 | Km, astig and ISV missing (fixed crop clips the glyphs). KI 0.9 (printed 0.97). Name gets an injected `J` token (`… JDE …`) |
| maria-gorete | PENTACAM OE (11 p) | PENTACAM/OS | extracted | 17/**1**/0 | ISV 2 (printed 12, leading `1` lost again) |
| rolmey | EYESUITE__AO | EYESUITE/AO | extracted | 20/0/0 | Birth date `12/05/YYYY` alone ⇒ ambiguous ⇒ none |
| rolmey | MICROSCOPIA_ESPECULAR__AO | MICROSCOPIA_ESPECULAR/AO | extracted | 2/0/0 | |
| rolmey | PENTACAM__OD (9 p) | PENTACAM/OD | **extracted** | 4/**1**/13 | ISV 44 (printed 10) |
| rolmey | PENTACAM__OS (9 p) | PENTACAM/OS | **extracted** | 2/**1**/15 | K2 missing (red text). ISV 44 (printed 20) |
| rolmey | RETINA__OD / __OS | RETINA | extracted | ok | |

**Patient-level identity, combined upload:**

| Folder | Combined `patient.birth_date` | `CanonicalBirthDate` | Name kept | Correct? |
|---|---|---|---|---|
| creso-aversa | ISO, correct | ok | EyeSuite "SURNAME, GIVEN" order (should come from the Pentacam per 0.B) | birth date yes |
| daniela-nogueira | **raw** `01/28/YYYY` (MDY string) | ok | Pentacam name with injected `J` (`<GIVEN> J<SURNAME> …`) | birth date yes, name no |
| francisco | **raw** `08/05/YYYY` | ok = `YYYY-08-05`, chosen **silently**. The EyeSuite DMY reading (`YYYY-05-08`) was reinterpreted with the Pentacam's order, and no inversion warning was emitted | EyeSuite | needs `data_nascimento_invertida` warning |
| maria-gorete | ISO, correct | ok | EyeSuite | yes |
| rolmey | **raw** `05/12/YYYY` | **ok = false**. The global order is MDY (from the Pentacam timestamp) and is applied to the EyeSuite DMY string, so the two readings conflict | Retina/EyeSuite | **no birth date**. The frontend then parses the raw `05/12` as DMY, which is the wrong date |

## 4. Failure taxonomy with evidence

The stages are those of `contracts.md` C4: classification, recognition, region, mapping, normalization, validation.

| # | Stage | File(s) | Field | On the page | Produced | Mechanism |
|---|---|---|---|---|---|---|
| F1 | **region** (page selection) | all 6 nine-page Pentacams | pachymetry, BAD-D, ARTmax, ISV…CKI, TKC, Z31, Z40, ACD int (13–15 fields per file) | Values printed on pages 5–9 | missing | `pentacamPageLayoutFor(9)` returns the 12-page layout, so pages 10/11/12 are skipped and pages 8/9 are the wrong screens |
| F2 | **region → wrong value** | same 6 files | ISV | 42/45/9/14/10/20 | **44** in all six | The ISV crop on "page 9" is read on the Anéis Corneanos screen. A stable garbage token passes the 0–200 range check. Consistent garbage looks like a value |
| F3 | recognition (leading `1` lost next to a box border) | creso OS, gorete OS | ISV | 17, 12 | 7, 2 | `isv` region plus `\b(?:isv\|15v)\b`; the `1` merges with the cell border. No cross-check (ISV is an integer ≥ 0; a single-digit result where neighbours are 2-digit is not flagged) |
| F4 | region (crop clips glyphs) | gorete OD (12 p) | Km, astig | 42.8 D, 0.8 D | missing | Fixed boxes `{535,1385,155,85}` / `{535,1455,180,70}` cut through the value box (overlay at `$SCRATCH/ocr/crops/kcells2.png`) |
| F5 | recognition (coloured text) | rolmey OS | K2 | `44.1 D` in red | missing | Red (out-of-norm) values lose contrast after grayscale and threshold |
| F6 | recognition / normalization | gorete AUTO pdf | OD axis, OD cyl | 100, −1.50 (slashed-zero font `1ØØ`, `-1.5Ø`) | 188, −1.58 | `Ø` read as `8`. Validation is missing too (F11) |
| F7 | recognition (no preprocessing) | daniela microscopy | OD cell density | 3312 | missing → status `partial` | The raw crop with PSM 6 OCRs the row as `Cite of als uw)…`, so the marker is not found. The other panel succeeds |
| F8 | mapping (label lookahead) | Pentacam identity (synthetic case, reproduced) | birth date | `Nasc.` box unreadable (e.g. 150-DPI render gives `[TOAAABTS`) | birth = **exam date** | `dateAfterLabel` scans the label row plus 2 rows, and the next row is "Data de Exame". Reproduced with a scratch test: `BirthDateRaw:11/04/2026 ExamDateRaw:11/04/2026` |
| F9 | normalization (date order) | rolmey, francisco (combined), every EyeSuite alone | birth date | Device-specific MDY/DMY | missing (rolmey), silently chosen (francisco) | Global order inference in `CanonicalBirthDate`, plus EyeSuite `FromOCR` converts the exam timestamp to ISO before storing it, which erases the order hint |
| F10 | normalization (raw persisted) | all Pentacam-only and Pentacam-first runs | `patient.birth_date` | — | raw `MM/DD/YYYY` string | `mergeLocalPatientIdentity` canonicalizes before appending its own evidence (`pentacam_integration.go:99-121`) |
| F11 | validation (absent) | gorete AUTO; all ISV cases | axis, cyl, ISV | — | 188°, −1.58 D accepted | No per-field domain rules: axis 0–180; refraction in 0.25 D steps; ISV integer; Z40/Z31 sign allowed; birth < exam date; age ≥ 18 |
| F12 | normalization (name) | daniela, francisco, gorete, rolmey Pentacams | name | clean name in the box | `J`/`j` injected before tokens (`J<SURNAME>`, `jJO40 J<NAME>`, `JDE`), or Scheimpflug text | The input-box left border OCR'd as `J`. The only fix is the patient-specific the hard-coded surname patch. "More words wins" scoring picks noisy candidates |
| F13 | classification | daniela TOPO, francisco CORNEA ×2 | exam type | TP-15 topographer / anterior-segment photo | unclassified (TOPO: no event at all) | Filename-only classification; the vocabulary is missing both types |
| F14 | classification (dispatch) | any upload without frontend classification | all | — | RETINA and MICROSCOPIA ⇒ "identified", nothing extracted. EYESUITE ⇒ extracted but **no file_result** | Reproduced on the rolmey files (scratch test on the HEAD snapshot): `examType="fundus_retinography" -> identified`, `examType="specular_microscopy" -> identified`, `examType="iol_calculation" -> (no file_result)` |
| F15 | classification (format) | daniela AUTO jpeg | refraction | Clearly printed AVE rows | not extracted | The AUTO extractor is PDF-only |
| F16 | mapping (silent all-or-nothing) | EyeSuite (latent, not triggered in this corpus) | all 20 fields | — | — | `parseEyeSuiteEye` returns an error if any of 10 labels is missing in either eye, which discards the 19 good values |

### Why age becomes 0 (traced)

- The frontend computes age in `App.tsx:117-122`. It calls `calculateAge(formatBirthDate(patient.birth_date), first verificacao.timestamp_lido)`. `calculateAge` returns `age >= 0 ? age : null`, so **0 is accepted as a real age** and drives Fluxo A.
- Age 0 means the birth year equals the reference year. That happens when the birth date is the exam date:
  - **HEAD:** Pentacam `dateAfterLabel` falls through to the "Data de Exame" row (F8, reproduced). Nothing downstream checks that birth < exam date or that age ≥ 18.
  - **Before `3e5a061`:** the birth date was simply "the first `d/d/yyyy` anywhere on the page" (`git show 3e5a061^:backend/features/pentacam/identity.go`). That is very likely the original age-0 source.
- **Missing birth date (F9)** gives "não informado", not 0.
- **The raw MDY string kept in `patient.birth_date` (F10)** gives a wrong but non-zero age. For rolmey, `05/12` is read as 5 Dec instead of 12 May.

### Silent zero / empty coercions found

- **Specular microscopy:** `local_extraction.go:974` `value, ok := payload[...].(float64); !ok || value <= 0`. Correct, it treats 0 as missing.
- **EyeSuite:** `eyesuite/parser.go` `astigAxis` defaults to `0` but is guarded by `axisFound`. OK.
- **Refraction and Pentacam:** `parsePentacamFieldNumber` and the refractometry `parseAverageRow` return `(0,false)`. That is fine. The problem is the *recovery* paths that invent decimals and drop signs:
  - IHA `/10`
  - Z40 `"0."+digits`
  - Z31 `"0."+token` (a negative coma would become positive)
  - Kmax compact recovery
- **Retinography and specular:** `tryExtractSpecularMicroscopyLocal`/`Retinography` set `patient.full_name = PatientID` when it is empty. That makes the identity name order-dependent.
- **Frontend:**
  - `calculateAge` accepts 0.
  - `parseDateParts` assumes DMY unless the second part > 12.
  - `formatBirthDate` turns anything unparseable into "não informado". Good, but it hides the cause.
- **Hard-coded patient in the UI:** `App.tsx:1880` hard-codes a birth date string for the `real` demo case. That is demo-only, but it is a patient value in app code.

### Evidence of patient-specific tuning (git history)

| Commit | What changed | Effect |
|---|---|---|
| `ca38311` | `identity_test.go` expectation changed from the correct name to the same name with a `JDE` token | The golden test was updated to accept OCR garbage instead of fixing the `J` artifact (F12) |
| `3e5a061` | Z31 region moved `{970,1490,290,180}` → `{1000,1500,200,170}`. BAD-D deterministic reader promoted to first ("values such as 0.98", the creso OS value). the hard-coded surname patch added. Creso fixtures added to the tests | Changes fitted to the 11/12-page creso layout. Nothing was tested on 9-page reports |
| `c89e0f6` | `pentacamPageLayoutFor` added with only the `pageCount == 11` branch | Fixed gorete/creso OS. 9-page reports were never handled, even though the code comment in `classification.go` claims "relatórios completos observados chegam com 9 páginas" |
| `cell_ocr.go` comments | "Fixtures reais <patient>: OD = 44.2 D; OS = 45.5 D". PSM lists chosen per field after one runtime ("PSM 13 confundiu 45.5 com 41.5 no Alpine") | Per-field, per-runtime voting tuned to single values |

Each fix was validated against one folder's fixtures. No test runs the whole corpus, so a region nudge that fixes one layout silently breaks another (F1–F5). The only integration tests (`REFRATIA_RUN_INTEGRATION_TESTS=1`) cover the creso and gorete Pentacams.

## 5. Root causes, ranked by impact

- **RC-1. Pentacam fields are located by page index and fixed pixel boxes, not by screen identity and label anchors.**
  - The 9-page layout (6 of 10 Pentacams) loses 13–15 of 18 fields and fabricates ISV=44.
  - The same mechanism (fixed crops) explains F3–F5 on the 11- and 12-page reports.
  - Evidence: F1, F2, F4, F5; `extractor.go:31-54`, `cell_ocr.go:32-61`, `detector.go:12`.
- **RC-2. Birth-date normalization is batch-global, not per-document. The device date order is discarded too early.**
  - `CanonicalBirthDate` (HEAD) infers one order for all documents.
  - `examdate.FromOCR` converts EyeSuite timestamps to ISO, which erases the evidence.
  - Result: rolmey has no birth date; francisco silently picks one reading and emits no inversion warning; every EyeSuite-only upload has no birth date.
  - The persistence agent's in-flight `ResolveBirthDate` (per-document) fixes the resolution part. It still *infers* the device order from the timestamp's string shape ("slash ⇒ MDY, ISO ⇒ DMY"). See fix 3.
- **RC-3. Identity parsing does not bind a value to its label box.**
  - The 2-row lookahead turns the exam date into the birth date, which gives age 0 (F8).
  - The box-border `J` and Scheimpflug text get into names (F12).
  - Nothing validates birth < exam date or age ≥ 18.
- **RC-4. There is no field validation layer.** Out-of-domain values (axis 188, cyl −1.58, ISV 44 on a non-topometric screen) are emitted with status "extracted". Diagnostics only distinguish found from missing.
- **RC-5. Patient- and runtime-specific patches stand in for mechanism fixes.** Examples: the hard-coded surname patch, decimal-recovery hacks per field, per-field PSM lists, golden tests updated to garbage. There is no whole-corpus regression gate.
- **RC-6. Classification and dispatch are filename-driven and inconsistent.**
  - Contract key vs label mismatch in the handler fallback (F14).
  - Three filename classifiers.
  - No content-based classification (F13).
  - PDF-only AUTO (F15).
  - Silent paths with no `file_result`.
- **RC-7. Failure reasons are discarded.** The `try*Local` functions return `bool`. Errors carry raw OCR text, including names (retina `OCR=%q`). The debug redaction is incomplete (the specular `Name:` line leaks).

## 6. Fix plan (ordered, mechanism-level)

Each step must leave the whole-corpus harness/benchmark no worse on **every** folder. No patient names, filenames or fixture values go into app code.

1. **Pentacam screen detection replaces page-count layouts.**
   - Files: `features/pentacam/extractor.go`, `detector.go`, `identity.go`, `cell_ocr.go`, `adaptive_rescue.go`, `badd_deterministic.go`, `k1_deterministic.go`; new `features/pentacam/screens.go`.
   - Render each page once at low DPI (~100) and OCR the title band, which sits at ≈23–26% of page height. Build `map[screen]page` for `4 Mapas Refrativos`, `Paquimétrico`, `Topométrico`, `Cataract Pre-OP`, `Belin/Ambrósio` (`Ectasia`), `Anéis Corneanos`, `Geral`.
   - Each field spec references a **screen**, not a page.
   - A missing screen means the field status is `not_applicable` (protocol: "When those screens are absent, the fields stay [ausente]"). It never means reading another page.
   - `DetectEye` and `ExtractIdentity` use the detected screens or the left identity panel, which is present on every screen.
   - Render each needed page at 450 DPI once and share it.
2. **Anchor cells on labels instead of fixed boxes.**
   - Same files.
   - Run one TSV OCR per screen and find the label word (`K1`, `Km`, `Astig.`, `ISV`, `ART max`, `Z40`, `Prof.Câmara Ant.(Int.)`…).
   - Take the value box as the label's row band, to the right of the label, up to the next label. Then run the existing threshold×PSM voting on that dynamic box.
   - Keep the current fixed boxes only as a fallback, and record them as `strategy:"fixed_box"`.
   - Delete the per-field decimal hacks. Use field-format rules from `_field_spec.json` ("printed decimals") in a single normalizer that **preserves the sign**.
   - Binarize per colour channel (max(R,G,B) darkness) so red and blue values survive (F5).
3. **Per-document dates with an explicit device order.**
   - OCR-owned files: `features/pentacam/identity.go`, `features/eyesuite/parser.go`, `shared/examdate/examdate.go`, `pentacam_integration.go`, `local_extraction.go`.
   - Consumer: persistence's `features/patient/identity.go`.
   - Each extractor emits `nascimento_lido` raw **plus** `date_order` (`MDY` for Pentacam, `DMY` for EyeSuite, `DMONY` for NIDEK and PRK) and `timestamp_lido` raw plus ISO. The order is device knowledge from the identified template. It should not be inferred from the string shape.
   - `ResolveBirthDate` should prefer the declared `date_order` and fall back to its current heuristic only when it is absent. I will message persistence with this contract.
   - Fix `mergeLocalPatientIdentity` to append the evidence first and canonicalize after. Never write a raw string to `patient.birth_date`; write ISO or nothing.
4. **Identity bound to label boxes.**
   - Files: `features/pentacam/identity.go`, `features/eyesuite/parser.go`, `features/specularmicroscopy/extractor.go` (name/ID fields).
   - Bind each value to the same row band as its label. No multi-row lookahead into another label's row: stop at the next known label.
   - Strip box-border artifacts mechanically. A single leading `J|[]Il` glyph immediately adjacent to the box's left edge is dropped when the remaining token is alphabetic. Remove the hard-coded surname patch.
   - Score name candidates by agreement across PSMs and pages, not by word count.
   - Validation:
     - birth date ≠ exam date
     - birth < exam date
     - 18 ≤ age ≤ 110 at the exam date
     - otherwise status `conflicting`/`unreadable`, never a value
   - Frontend (lead/frontend owner): `calculateAge` should reject `age < 18`, or at least `0`, and show "revisar".
5. **Validation layer driven by `_field_spec.json`.**
   - New file: `features/extractionfields/validate.go` (or `shared/fields`). Called from `local_extraction.go`.
   - Rules: plausible range, printed decimals, step (refraction 0.25 D), integer-ness (ISV, ARTmax, cell density), axis 0–180, sign rules.
   - An out-of-domain value becomes status `unreadable` with stage `validation`. Its raw text is kept in the local diagnostics only.
   - This catches F2, F3, F6 and F11 even before the region fixes land.
6. **Classification and dispatch.**
   - Files: `features/intake/classification.go` (classification logic), `local_extraction.go`, `intake_handlers.go` (one-line fix in persistence's file; requested via the owner).
   - In `intake_handlers.go:141`, convert with `ExamTypeFromContract` before storing. Even better, make `Metadata.ExamType` always the contract key and switch on contract keys in `local_extraction.go`, so there is one vocabulary.
   - Content-based PDF classification from the title band: Pentacam ("OCULUS – PENTACAM"), EyeSuite ("EyeSuite™ IOL"/"Calculation ID"), PRK ("PRK-6000"), TP-15 (text layer "TC-15/TP-15"). The filename becomes a hint, and a conflict between filename and content is reported.
   - Add `anterior_segment_photo` (retina camera `Mode Cornea`) and `corneal_topography_other` (TP-15) as recognized out-of-protocol types, used for identity only.
   - Every file emits exactly one `file_result`.
   - Run the AUTO extractor on images: decode JPEG, deskew or crop the ticket, then use the same `<R>/<L>`/`AVE` parser.
7. **Specular robustness.**
   - File: `features/specularmicroscopy/extractor.go`.
   - Crop the table region by layout (the right half of each panel), upscale ×3, binarize, and OCR rows with PSM 6/4. Read the `Cell Density` row by anchor.
   - Read the name from `Name:` or `ID:` by label, not by position.
8. **EyeSuite partial results.**
   - File: `features/eyesuite/parser.go`.
   - Return the fields found plus a per-field missing status instead of erroring the whole exam (F16).
   - Keep the raw timestamp and date order (fix 3).
9. **Tests and gate.**
   - Each mechanism gets one unit test on synthetic TSV words (as in `age0_test.go`), never patient fixtures.
   - The corpus-wide check is the benchmark (`npm run benchmark`). Revert golden expectations that encode OCR garbage (`identity_test.go` "JDE").

Order rationale:

- Fixes 5 (validation) and 6 (the handler one-liner) are cheap and stop wrong values and lost files immediately.
- Fixes 1 and 2 recover the bulk of the missing Pentacam fields (92 missing → expected ≤ 10).
- Fixes 3 and 4 fix identity and age.
- Fixes 7–9 are hardening.

Fixes 1 and 2 can be done together. Fix 5 should land first so that 1 and 2 are measured with validation on.

### File ownership

| Step | OCR agent | Needs another owner |
|---|---|---|
| 1, 2, 7, 8 | `features/pentacam/**`, `features/specularmicroscopy/**`, `features/eyesuite/**` | — |
| 3 | extractors, `shared/examdate`, `pentacam_integration.go`, `local_extraction.go` | persistence: `features/patient/identity.go` consumes `date_order` |
| 4 | `features/pentacam/identity.go`, `eyesuite/parser.go`, specular | frontend: `calculateAge` ≥ 18 guard (lead decides owner) |
| 5 | new validation package plus calls in `local_extraction.go` | — |
| 6 | `features/intake/classification.go`, `local_extraction.go`, `refractometry/**` | persistence: `intake_handlers.go:141` contract key → label (or agree on one vocabulary) |
| 9 | unit tests in the OCR packages | benchmark: corpus gate |

## 7. Observability: per-field diagnostics (implements contracts.md C4)

Shape, emitted by every extractor and collected in `local_extraction.go`:

```json
"extraction_diagnostics": [{
  "file_sha256": "…",            // not the filename: filenames contain patient names
  "exam_type": "pentacam_corneal_tomography",
  "eye": "OD",
  "field": "topometric_indices_8mm.isv",
  "status": "valid|missing|unreadable|conflicting|not_applicable",
  "stage": "classification|recognition|region|mapping|normalization|validation",
  "page": 6, "screen": "topometric",
  "strategy": "label_anchor|fixed_box|rescue_badd_digits|…",
  "bbox": [x, y, w, h], "bbox_units": "px@300dpi",
  "dpi": 450, "psm": 7, "votes": 5, "candidates": 2,
  "raw_text": "ISV 10",          // LOCAL ONLY: see rules
  "normalized": 10,
  "error": "value 188 outside 0..180"
}]
```

Rules that keep patient data out of logs:

- Diagnostics travel **only** inside the analysis artifact (local response, Garage draft). They never go to `stdout`/`stderr`/`log` or the SSE `message`.
- Logs may carry only `{file_sha256[:8], exam_type, eye, field, status, stage, strategy}`. Those values are enumerations or hashes.
- `raw_text` is set only for clinical fields. It is always empty for identity fields (name, birth date, ID), which instead get `raw_text_len` and a `matches_pattern` boolean.
- Error strings are templated (`"campo %s fora do domínio"`) and never interpolate OCR text. This applies to the retina `OCR=%q`, the specular debug lines, and `file_result.message`.
- `REFRATIA_OCR_DEBUG_SAVE` writes into a per-run `os.MkdirTemp` directory (0700), never to fixed `/tmp/specular-*.png`, and only for clinical crops. The header band is masked before saving.
- `status` semantics:
  - `valid`: passed validation.
  - `missing`: label not found.
  - `unreadable`: label found but no value that passes validation.
  - `conflicting`: two strategies or pages disagree beyond tolerance, or identity disagrees across documents.
  - `not_applicable`: the screen is absent from this report (e.g. a 9-page report has no Aberrometry pages), or the field does not exist for the device.
- The existing `benchmarkField{found,value}` becomes a projection of this list. The benchmark reads `stage` directly instead of inferring it.

## 8. Uncertainties

- I could not confirm the francisco birth date beyond what the ground truth already marks as `needs_human`. The two devices disagree by inversion.
- The original reports of "age 0" probably came from the pre-`3e5a061` "first date on page" logic, or from the F8 fall-through. Neither happened in this corpus at HEAD, where every Pentacam `Nasc.` was read at 300 DPI. F8 is reproduced synthetically. The 150-DPI render shows the box can come out unreadable.
- The ISV leading-`1` loss (F3) is inferred from 2 of 2 cases where the printed value starts with `1`. I did not inspect the Tesseract output.
- Runtime differences (host vs Alpine Tesseract) were not tested. All numbers are from the host Tesseract.

## Implementation log

### Step a (fix plan 5): validation layer + C4 emitter

- Reproduced: gorete AUTO emitted axis 188 and cylinder -1.58 as `extracted` (F6/F11). Tests: `features/extractionfields/validate_test.go` (synthetic eyes), `field_diagnostics_test.go`.
- Cause: no per-field domain check between extractor and analysis.
- Change: new `features/extractionfields` (embeds a copy of `_field_spec.json`, drift-guarded by a test). Rules from the spec: plausible range, printed decimals, integer kind; plus the device step 0.25 D for refraction (not in the spec, kept as a small table in code). Failing values are removed (never coerced) and reported `unreadable/validation` with a templated error. Every spec field of every touched eye gets one C4 diagnostic (`valid|missing|unreadable`). `field_diagnostics.go` snapshots eyes before each file and validates what changed; it runs by `defer` inside each `tryExtract*Local` so values are cleaned before the file result is emitted or checkpointed. Diagnostics go in `analysis.extraction_diagnostics`; stdout gets only `sha[:8] exam eye field status stage` for unreadable fields.
- Limit, stated plainly: ISV=44 on the nine-page Pentacams is inside the spec range 0..300, so validation cannot catch it. It is fixed at the source in step c (wrong screen). ISV 7/2 (leading 1 lost) is also in range.
- Whole-corpus effect (scratch harness, affected file only): maria-gorete AUTO 4 ok / 2 wrong / 0 missing -> 4 / 0 / 2 (wrong values became missing).

Step a addendum (final): `ValidateEyes` also accepts extractor hints (`extractionfields.Hint`): an absent Pentacam screen turns the fields' `missing` into `not_applicable/region`; two disagreeing readings become `conflicting/recognition`; a hint never rescues a value that failed validation.

### Step b (fix plan 6): one vocabulary, one file_result per file, content classification

- Reproduced: F13/F14/F15. `local_extraction_dispatch` test (`local_dispatch_test.go`) asserts one `file_result` for: contract-key and legacy-label types, an unknown type, an out-of-protocol type, a PDF-only type given an image. Before the change an unknown PDF produced no event and `iol_calculation` (contract key) produced none either.
- Cause: three filename classifiers, label vs key mix-up, silent `continue` paths, no content classification, PDF-only AUTO.
- Change:
  - `intake.ContractExamType` is the single vocabulary (accepts key or legacy label; knows `anterior_segment_photo`, `corneal_topography_other`, `oct_retina`). `ParseClassifications`, `local_extraction.go`, `file_checkpoints.go` use keys. Filename classifier knows `CORNEA` and `TOPO`.
  - `dispatchLocalFile` (replaces the 300-line chain) emits exactly one `file_result` on every path. Failure messages are templates (no OCR text; the retina `OCR=%q` and `%q` identity errors are gone).
  - `intake.ClassifyContent`: PDF text layer, else a 100-dpi OCR of page 1, matched against device titles (Pentacam/OCULUS, EyeSuite/Calculation ID, TP-15, PRK-xxxx/[REF]); images: phone photo of a PRK ticket, retina camera `Mode Cornea`. `InspectFile` returns it as `recognizedExamType` plus `filenameConflict`; the page-count prefilter ("8-10 pages => Pentacam") is deleted. A declared-vs-content disagreement is recorded as a `classification/conflicting` diagnostic.
  - `refractometry.ExtractImage` (JPEG/PNG) shares the PDF parser. The AVE row is located by its own label word (a photographed ticket is rotated, so row grouping fails), then re-OCR'd upscaled with a digit whitelist under 9 preprocessings and parsed by its printed format (2 decimals, signed, integer axis). Reads off the 0.25 D grid or outside 0..180 are discarded before voting and a column needs a strict majority; otherwise the row falls back to the page-level text and validation.
- Result (scratch harness): daniela AUTO jpeg 0 ok/0/6 missing -> every printed value equals ground truth (OD -3.5/-6.75/15, OS -8/-7.25/172). gorete AUTO OD cylinder -1.50 is still not recovered: the slashed-zero scan makes 5/0 reads disagree, so the value is left missing (never the wrong -1.58/-1.00).
- `ParseClassifications` storing keys changed three asserts in `classification_test.go`.

### Step c (fix plan 1+2): Pentacam screens and label anchors

- Reproduced: F1/F2/F4/F5 (all nine-page reports lost 13-15 fields; ISV=44 was read off the Aneis screen).
- Cause: page-index layouts and fixed pixel boxes.
- Change (`features/pentacam`):
  - `screens.go`: `DetectScreens` renders every page once at 100 dpi, OCRs the band after the "PENTACAM" header word and returns screen -> page. The old layout struct is now derived from it; a missing screen means page 0, nothing is read, and the fields get `not_applicable`. `DetectEye` reads the identity panel ("Olho: Direito/Esquerdo") instead of page 9. A shared pdftoppm bug for 10+ page PDFs (zero-padded names) is fixed in `shared/pdf` with a test.
  - `anchors.go`: each value is located through its printed label (topmost match, or "below ISV in the same column" for the index box, bottom-most for the BAD-D row), the cell is cut out of the label's row band (border and unit trimmed by character share, white margin added, light-on-dark polarity inverted, darkest-channel grey so red text survives) and read by voting across scale 2/3/4, two thresholds, three PSM/whitelist modes and two crop extents. A value needs 2 agreeing reads and, when a rival reading exists, 80% of the valid reads: otherwise it stays missing. A field printed on several screens is read on each and must agree, else `conflicting`.
  - `printed.go`: the single normalizer. It uses the spec's printed decimals and range, preserves the sign, re-inserts a lost separator only when digits remain. Removed: IHA `/10`, Z40 `0.`+digits, Kmax compact recovery, the broad BAD-D reader, the per-field comments citing patients. Fixed-box reading stays only as a fallback (and only for fields the anchors did not read).
  - New fields now extracted because the anchors make them cheap: pachymetry pupil centre and vertex, external ACD, flat axis.
- Tests: `screens_test`, `anchors_test` (synthetic words), `printed_test`, `binarize_test` (black/red/white-on-red cells).
- Result (scratch harness, correct/wrong/missing against ground truth, my compare counts spec fields that have an app path; "wrong" includes tkc "-" vs null and the Z31 axis path collision, which are comparison artefacts): daniela OD 3/1/14 -> 20/1/6, OS 4/1/13 -> 21/2/4; francisco OD 3/1/14 -> 19/2/6, OS 4/1/13 -> 19/2/6; rolmey OD 4/1/13 -> 16/3/8, OS 2/1/15 -> 18/2/7. ISV is now read correctly where the cell is white-on-red (ISV 42). Remaining misses are cells where scale/threshold reads disagree (3/9 and 5/6 on the pixel font); they stay missing on purpose. Known wrong value left: rolmey OD astigmatism read 2 instead of 0.2 before the two-screen agreement rule; the rule is in the code now and is covered by the next benchmark run.

### Step d (fix plan 3+4): per-document dates and identity bound to label bands

- Reproduced: F8 (`TestIdentityNeverBorrowsExamDateForUnreadBirthLabel`), F9/F10, F12.
- Change: `date_order` is declared by the extractor (`MDY` Pentacam, `DMY` EyeSuite) and `mergeLocalPatientIdentity` appends the evidence first and then calls `NormalizeIdentityFields`, so `patient.birth_date` is ISO or absent. The EyeSuite path no longer overwrites `patient` and `verificacao_identidade` written by earlier files. Pentacam identity: values are bound to their label's row band and stop at the next label on that row; a label that is not read yields no date (no lookahead). The surname patch is gone, as is the golden test that encoded "JDE" (replaced by synthetic-word tests). The name box is re-read from pixels with the box border cut by geometry (a column dark over >=90% of the cell height), voted across scales; no text rewriting, because a text rule would corrupt real names that start with J, I or L. Dates must satisfy birth < exam and 18 <= age <= 110 (`examdate.PlausibleBirth`), else the birth date is dropped and `BirthDateIssue=conflicting`.
- Still open: name quality of the Pentacam is only as good as the box read; when the vote has no majority the band text (which can carry a leading box-border letter) is kept, and persistence's name-compatibility rule has to absorb it.

### Step e (fix plan 7+8): specular and EyeSuite

- Specular: if the raw-crop OCR does not recognise the density row, the same panel is re-read upscaled x3, auto-contrast and binarized under PSM 6 and 4. Debug output no longer prints raw OCR and no longer writes `/tmp/specular-*.png` (the panel header holds the patient name).
- EyeSuite: `parseEyeSuiteEye` returns the fields it read (absent ones are reported by the diagnostics) and errors only when an eye has nothing; one empty eye no longer discards the other; the identity no longer needs the timestamp.
- Tests: `partial_test.go`, `TestParseEyeSuiteIdentityKeepsNameAndBirthWithoutTimestamp`.

### Step e addendum: IOL fields and specular table (measured against ground truth values, direct field comparison on the files changed)

- EyeSuite: the parser only knew 9 fields, so cct, aqueous depth, K1/K2/K radii, K1/K2 axes and lens status were never extracted (baseline: missing for every EyeSuite file). Added from the same rows (`CCT`, `AD`, `K1/K2/K [D/mm/°] power/radius @ axis`, `Phakic`). Creso biometry OD: every ground-truth IOL field now matches (checked against the ground-truth json by eye and value). `partial_test.go` covers it.
- Specular: the extractor only read cell density. `table.go` reads the other eight table rows by their label text (rows are found by label, value is the right-most integer, plausible range from the spec), over the main reading and four upscaled/binarized re-readings that vote (two agreeing readings, or the only value any reading produced; a tie is no value), then cross-checks CV = SD/AVG and density x AVG ~ 1e6 and drops the inconsistent ones. Detection (`Detect`) stops at the first good reading as before. Ground-truth matches over the five specular files: 9/90 (baseline benchmark) -> 66/90, 0 wrong; missing are mostly the last table rows (MIN, CT) and the poorer daniela image (9/18).

### Gate c4bc7c2 follow-up (Pentacam regressions, required gaps)

- Keratometry identities (`features/pentacam/keratometry.go`): Km=(K1+K2)/2 (tol 0.1) and astig=K2-K1 (tol 0.15), from the printed 0.1 rounding. `keratoResolve` only selects among values the cells actually read (never computes); `keratoDrop` drops only the odd key when one identity holds, else every value of the failing identity. Replaces the astig-only cross-check and `fitPrior`. Fixes rolmey OS K1 (1 read as 7), creso OD / francisco OS astig.
- `normalizePrinted`: a trailing 0 one decimal beyond the printed format is the unit (D, deg) read as a digit.
- `readValueBox`: a rival confined to the thresholded reads (5 read as 6 at threshold 150) no longer blocks a value the page token and a 2/3 grey-scale majority agree on (gorete OS pachy 545, rolmey OD pachy centre 534, rolmey acd_ext 3.9).
- Specular: row grouping tolerance now scales with the upscale factor (CT value sits above its label at 3x); two threshold-120 re-reads added. ct_um matches ground truth on all 10 eyes.
- Still missing: rolmey OD pachy_vertex (534/538/634 split, no reliable majority), gorete OD axis_flat (3 vs 9 pixel glyph, 108.3 vs 108.9).

### Gate 559f373 follow-up (creso coma regression, IOL measurement_date)

- Regression: creso OD `coma_z31_5mm_um` came out 0.03 (valid, expected 0.09). Cause: the grey-token plurality rule in `readValueBox` accepted the whole-page token when it held only 2/3 of the grey-scale reads (votes 0.03:12 / 0.09:6), i.e. a genuine 3-vs-9 split. Now the token needs a 3:1 grey majority (75%); fixed cells measured: gorete OS pachy centre/vertex 545 (grey 10:2), rolmey OD pachy centre 534 (grey 6:2) still pass; creso OD coma is 0.09. Limit: 75% is a calibrated bar, not a derived one; rolmey sits exactly on it.
- IOL `measurement_date`: EyeSuite prints "<device> DD/MM/YYYY" at the head of each eye's biometry block. `parseEyeSuiteEye` reads it (DMY -> ISO via `examdate.FromOCR`) into `exams.iol_calculation.eyes.<EYE>.measurement_date`; `extractionfields` validates kind `date` (ISO). `app_paths` set in both `field_spec.json` copies. All 8 eyes match ground truth on the real PDFs.

### Option D (grey-token plurality removed; independent evidence instead of a vote bar)

- `readValueBox`: the "token holds 3:1 of the grey reads" block is deleted (it was a corpus-calibrated bar). A cell with a rival reading now stays missing unless independent evidence resolves it.
- External ACD: the real PDFs (rolmey) print `Prof.Câm.Ant (Ext.)` on both the Topometric and the Cataract Pre-OP screens, and `(Int.)` on the Cataract screen. Physical identity: external ACD = internal ACD + vertex pachymetry/1000 (epithelium vs endothelium reference). Printed rounding gives the tolerance: 0.005 + 0.005 + 0.0005 mm. It holds on all 10 Pentacam eyes of the ground truth (max residual 0.008 < 0.0105).
- `features/pentacam/depth.go` (`depthResolve`, tests in `depth_test.go`): joint selection over the OCR-read candidates (each read >= 2 times, the base rule of `readValueBox`); a missing/split cell is filled only when every assignment that satisfies the identity agrees on it; accepted readings are never overridden; nothing is computed. Wired next to `keratoResolve`. The `acd_ext` and `pachy_vertex` anchors also try the Cataract screen, and the `prof c... ant ext/int` label regex accepts the abbreviated `Prof.Câm.Ant` printed there.
- Results on the real PDFs: rolmey OD acd_ext 3.9 (candidates 3.3/3.5/3.9, int 3.36), rolmey OS acd_ext 3.9, gorete OS pachy_vertex 545 (candidates 545/645; ext-int = 0.54 excludes 645). rolmey OS K1 42.1, creso OD coma_z31_5mm 0.09.
- Still missing, with evidence: gorete OS pachy_pupil_center (545:17/645:6) and rolmey OD pachy_pupil_center / pachy_vertex (534/538/634): the identity ties only the vertex, and 534 vs 538 are both inside the +-10 um window; no printed identity links the pupil-centre cell. The Cataract screen's vertex label is merged with the cell border by the page OCR ("Ni534") and is not located.
