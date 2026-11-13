# Regression benchmark

Command: `npm run benchmark` (= `go -C backend run ./cmd/benchmark`). Flags: `-patients a,b`, `-backend-dir DIR` (build and run another checkout), `-update-baseline`, `-reingest-force`, `-env-file`, `-out`.
Runtime about 17 min for the whole corpus (local OCR, about 2 min per patient for analyze, plus re-ingest).

## What it does
1. Loads `backend/testinfra/test.env` itself (never the repo `.env`), forces `POSTGRES_DB=refratia_bench`, `BUCKET_NAME=refratia-bench`, `REFRATIA_NO_DOTENV=1`, `OPENAI_API_KEY=""`, and refuses to run unless the endpoints are the isolated testinfra (127.0.0.1:55432 / :53900). The backend subprocess gets only that env plus PATH/HOME, not the caller's environment.
2. Takes a lock (`$TMPDIR/refratia-benchmark.lock`): only one run at a time, because the bench DB and bucket are shared and wiped at start.
3. Drops/recreates `refratia_bench`, empties the bucket, builds the backend, starts it on a free port, waits for `/health`, kills it on exit.
4. Per folder: `/api/intakes/inspect` (what the UI prefills; files it cannot recognize get the ground-truth classification, as a human would choose, and are labeled `human-assisted`), then ONE `/api/intakes/analyze?stream=1` with all files, `classifications` and `processFiles`, like `useIntakeWorkflow.ts`.
5. Scores every ground-truth field (`status valid` and `verification agent_visual`) through the app paths in `_field_spec.json`. Outcomes: `pass`, `incorrect`, `missing`, `conflicting` (only from `extraction_diagnostics`), `unexpected_value` (the app has a value where ground truth is missing/not_applicable), `not_in_app` (spec has no app path; not scored), `unverified` (needs_human; never a pass).
   Numbers are compared after rounding to the printed decimals (coma magnitude: abs 0.001); strings after trim/case/accent folding. Coma Z31 strings like "0.097 µm @ 151°" are parsed. Stage comes from `analysis.extraction_diagnostics` (C4) when present, else it is inferred and labeled `inferred`.
6. Identity: name, birth date (not scored when ground truth is needs_human), no age 0 / no age without a birth date, and the `data_nascimento_invertida` warning anywhere in the analysis when ground truth requires it (francisco).
7. Persistence after `confirm`: exactly 1 new `patients` row (name/birth checked); new `exams` rows == `expected_exam_records.count`; each row's (exam_type, eye, source file) matches (source derived from the sha256 prefix or original name in `object_key`/`filename`; when not derivable the match is on type+eye and says so); `patient_id` and `case_id` relations; every row's object exists in Garage with the same size; `paciente_compilado.json` marker exists. Then confirm again (same intakeId) and re-ingest (analyze + confirm again, checkpoints reused unless `-reingest-force`): no patient/row/object added or removed. Size-only changes (the merged manifest) are recorded, not failed.
8. Output: `backend/benchmark/out/<timestamp>/{report.md,report.json,raw/,backend.log}` (gitignored; raw responses and logs contain patient data, keep local).

## Pass/fail
Exit 1 on: a regression (field in `backend/benchmark/last_pass.json` that no longer passes, for the patients that were run), any failed persistence/identity/idempotency check, or any failing field with role decisive, cross_check or identity. `-update-baseline` rewrites `last_pass.json` (merging patients that were not run). Classification checks are informational.

## Not done
Overlay/crop rendering (needs `extraction_diagnostics` page+bbox from the OCR agent, not emitted yet). `not_in_app` fields (e.g. 4 mm coma, `iol_calculation.measurement_date`) are listed but not scored until the app gets a path.

## Out-of-protocol documents
Fields on exam types with `in_protocol: false` in `_field_spec.json` (e.g. `anterior_segment_photo`, `corneal_topography_other`) are still extracted, persisted and scored, but are always optional (role `optional_out_of_protocol`): they never count as required failures or block a run. Product decision, 2026-11-13.

## Reference state
Engineering reference: `cac8a1b` (gate `out/20261113-150559`, scorer `048e9b6`); `last_pass.json` = its 509 passing fields. Known non-passing fields that stay in the benchmark (regressions vs option A, unresolved required fields, the one wrong informative value) are listed in `final-report.md`.
