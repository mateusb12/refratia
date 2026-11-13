# Shared contracts (lead-owned; change only via the lead)

Inputs: `field-spec.md`, `persistence-investigation.md`, `ocr-investigation.md` (pending), ground truth in `backend/benchmark/groundtruth/`.

## C1. Exam record ↔ SQL row
- One `exams` row per **(persisted source document, eye)**. AO documents (EyeSuite/biometry, specular, autorefraction) → 2 rows (OD, OS) sharing `object_key`. Single-eye docs → 1 row.
- Uniqueness: `UNIQUE(object_key, eye)` (replaces `UNIQUE(object_key)`). Retries upsert with `ON CONFLICT (object_key, eye)`.
- `exam_type` vocabulary = ground-truth `exam_type`: `pentacam_corneal_tomography`, `iol_calculation`, `specular_microscopy`, `refractometry`, `fundus_retinography`, plus out-of-protocol types used in ground truth (e.g. `anterior_segment_photo`, topography). Never `''`.
- Out-of-protocol documents that are persisted also get rows (ground truth `expected_exam_records.count`, not `count_in_protocol`).
- `patient_id` NOT NULL for intake-confirmed rows; `case_id` matches the Garage case.
- Expected counts per folder = `expected_exam_records.count` in ground truth.

## C2. Patient row & identity
- Exactly one `patients` row per real patient; `patients.case_id` added; `identity_key` unique.
- Batch must be identity-consistent before ANY Garage case write. Unresolved identity → HTTP 422, no case objects, no marker, no SQL rows.
- Birthdate: day/month order resolved **per document**. Extractors (OCR-owned) emit per identity entry `nascimento_lido` (raw), `timestamp_lido` (raw) and `date_order` ∈ `MDY` (Pentacam) | `DMY` (EyeSuite) | `DMONY` (Nidek/PRK): this is device knowledge, not inferred from the string. `features/patient` (persistence-owned) resolves with the declared `date_order` first, heuristic only if absent. `patient.birth_date` is ISO or absent, never a raw string. Missing → `null`, never age 0, never today's date.
- Pentacam vs EyeSuite day/month inversion (francisco): protocol 0.B treats it as non-blocking. Persist the Pentacam reading (protocol reference document) and emit an identity warning `data_nascimento_invertida` in the analysis. The benchmark checks that the warning exists and skips the value (needs_human).
- OCR name typos must not mint a new patient silently: if name differs but other evidence matches, flag it, don't guess. Exact rule is owned by persistence and must be documented in the report.

## C3. Commit protocol (no distributed tx)
Garage objects at deterministic keys (unique per source file: include sha256 prefix) → `paciente_compilado.json` → **single SQL transaction** (patient upsert + all exam rows) → receipt. A retry with the same intakeId converges to the same state: no duplicate rows or objects. Garage objects not committed in SQL count as garbage (reconcile script).

## C4. Field diagnostics (OCR-owned emitter; benchmark consumer)
In the analyze response: `analysis.extraction_diagnostics: [{file_sha256, exam_type, eye, field, status, stage, page, strategy, bbox:[x,y,w,h], raw_text, error}]`
- `status` ∈ `valid|missing|unreadable|conflicting|not_applicable`
- `stage` ∈ `classification|recognition|region|mapping|normalization|validation`
- Full shape and privacy rules: `ocr-investigation.md` §7 (file_sha256, not the filename; `raw_text` empty for identity fields; templated errors; never logged).
- Local artifacts only. `raw_text` never goes to stdout/stderr logs.
- One vocabulary: `file_result`/`Metadata.ExamType` use the contract keys (C1 `exam_type`). Every uploaded file emits exactly one `file_result`.
- Until the OCR agent emits it, the benchmark infers the stage itself (wrong type → classification; field absent → mapping/region; value present but wrong → recognition/normalization) and labels it `inferred`.

## C5. Benchmark harness ↔ app
- Black-box: the benchmark builds the real backend, runs it as a subprocess with `testinfra/test.env` overridden to `POSTGRES_DB=refratia_bench`, `DATABASE_URL=…/refratia_bench`, `BUCKET_NAME=refratia-bench`, on a free port, and drives the exact HTTP calls the frontend makes.
- Fresh state per run: drop/recreate DB `refratia_bench` and empty bucket `refratia-bench`. Never touch `refratia_test`/`refratia-test` (persistence dev state) or anything outside testinfra.
- Command: `npm run benchmark` → `go -C backend run ./cmd/benchmark`. Nonzero exit on failure.

## File ownership (implementation phase)
| Owner | Files |
|---|---|
| Benchmark | `backend/cmd/benchmark/**`, `backend/benchmark/**` (ground truth values read-only; report bugs to the lead), `docs/pipeline/benchmark*.md`, `package.json` script entry |
| Persistence | `intake_handlers.go`, `patient_database.go`, `patient_upsert.go`, `exam_database.go`, `exams_storage.go`, `cases.go`, `main.go`, `database/**`, `fileStorage/**`, `features/patient/**` (identity/date canonicalization, merge), `features/intake/classification.go` (draft key naming only), their `_test.go`, `cmd/reconcile/**` |
| OCR | `features/{pentacam,eyesuite,refractometry,retinography,specularmicroscopy}/**`, `shared/{ocr,pdf}/**`, `local_extraction.go`, `pentacam_integration.go`, `extraction.go`, `file_checkpoints.go`, `benchmark_detection.go`, `pdf_preprocess.go`, `features/intake/classification.go` (classification logic) |
| Lead | `docs/pipeline/contracts.md`, `testinfra/**`, `database/database.go` dotenv guard |

Rules: no patient names, filenames or fixture values hardcoded in app code. Don't edit files you don't own; message the owner instead. Run the full benchmark before claiming a fix.
