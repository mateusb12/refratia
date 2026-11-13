# Persistence investigation: intake → Postgres + Garage

Date: 2026-11-12. Scope: phase 1 (investigation and reproduction only, no application code changed).
Environment: isolated infra from `backend/testinfra` (Postgres `127.0.0.1:55432/refratia_test`, Garage `127.0.0.1:53900/refratia-test`), `REFRATIA_NO_DOTENV=1`, `OPENAI_API_KEY` empty (local OCR only). Backend built from the working tree at `9eb5152` (plus the uncommitted `REFRATIA_NO_DOTENV` guard) and run on `:3100`.
Patients are referred to by corpus folder slug only.

## 1. Architecture map

```
Frontend (useIntakeWorkflow.ts)
  1. sha256 each file in the browser → POST /api/file-checkpoints/check {files:[{filename,sha256}]}
  2. user classifies each file (examType + eye), mandatory in UI only
  3. POST /api/intakes/analyze?stream=1[&force=1]   multipart:
        files=<each file>, classifications=[{sha256,examType,eye}], processFiles=[sha256...]
     → NDJSON progress events, final event = {intakeId, files, analysis, patientMatch, changePreview}
  4. user reviews → POST /api/intakes/confirm {intakeId}      (or DELETE /api/intakes/{intakeId})
  5. later, SavedExamsPage: POST /api/exams/upload-url → browser PUT (presigned) → POST /api/exams/confirm
```

### analyze (`intake_handlers.go:75`)
| Step | Store | Key / table |
|---|---|---|
| cleanupExpiredDrafts (first 1000 keys, >24h by LastModified) | Garage delete | `refratia/drafts/**` |
| per-file checkpoint of the raw upload (before OCR) | Garage put | `refratia/file-checkpoints/v1/{sha256}/source` |
| per-file extraction result, only when the result is "complete" | Garage put | `refratia/file-checkpoints/v1/{sha256}/result.json` |
| identity lookup: **lists every case and GETs every `paciente_compilado.json`** (O(cases)) | Garage read | `refratia/cases/*/paciente_compilado.json` |
| draft documents. Name = `CanonicalExamFilename(name, exam, eye, ext)` with a second-resolution timestamp, or `{rand8}-{original}` when unclassified | Garage put | `refratia/drafts/{intakeId}/{name}` |
| draft manifest `{createdAt, files[], analysis}` | Garage put | `refratia/drafts/{intakeId}/intake.json` |

No SQL is written by analyze.

### confirm (`intake_handlers.go:485`)
Order of operations, all on Garage except step 7:
1. GET receipt `refratia/intake-confirmations/{intakeId}.json` → if present return it (idempotency).
2. GET draft `intake.json`, check age < 24h, `validateStoredAnalysis` (only checks that file list ↔ `source_files` sha/filename match).
3. `findExistingPatientCase` (full scan again) → target `caseId` = existing case or `case-{intakeId suffix}`.
4. CopyObject each draft file → `refratia/cases/{caseId}/{basename}`.
5. Merge with the existing compiled JSON when found, then PUT `refratia/cases/{caseId}/paciente_compilado.json` (the "case complete" marker that `/api/cases` lists).
6. Delete copied keys that are no longer referenced (exact re-upload dedup).
7. **SQL**: `upsertPatientFromAnalysis` → `patients` (lookup by `identity_key`, then UPDATE or INSERT; no transaction).
8. PUT receipt, then delete the draft objects.

### manual exam upload (`exams_storage.go:438`, `exam_database.go:20`)
`upload-url` presigns `refratia/cases/{caseId}/{unixnano}-{rand8}-{filename}` → browser PUT → `exams/confirm`: HEAD the object, look up `exams.object_key`, upsert the patient from the case manifest, INSERT `exams`. On SQL failure it deletes the object (compensation).

### SQL schema (GORM AutoMigrate, `database/exam.go`)
- `patients(id, identity_key UNIQUE NOT NULL, full_name, birth_date TEXT, created_at, updated_at)`. `identity_key = lower(unaccent(full_name)) + "|" + YYYY-MM-DD`.
- `exams(id, patient_id NULL, case_id, patient_name, filename, exam_type, eye, content_type, bucket, object_key UNIQUE, size, referenced, ...)`.
- Written by: `patient_database.go` (patients), `exam_database.go` (exams insert/delete). **Read by nothing user-facing.** `/api/cases` and `/api/exams` list from Garage.
- No `cases` table and no `case_id` on `patients`. The patient↔case link exists only inside Garage JSON.

### delete paths
- `DELETE /api/cases/{id}` (`cases.go:132`): deletes case objects (marker last) and matching receipts. **No SQL.**
- `DELETE /api/exams` (`exams_storage.go:620`): rewrites the manifest, deletes the object, then deletes the `exams` row.
- `DELETE /api/intakes/{id}`: deletes the draft objects.

## 2. Reproduction log

Driver: `/api/intakes/inspect` to get the auto-classification (stands in for the UI step), then `analyze` (non-stream) with `classifications` built from it, then `confirm` twice. Each analyze took ~2–3 min under shared CPU load.

| Slug | analyze | patientMatch | confirm #1 | confirm #2 | patients row | exams rows |
|---|---|---|---|---|---|---|
| daniela-nogueira | 200 | new | 200 created | 200 (receipt) | `daniela jnogueira rocha\|2001-01-28` (OCR typo in name) | 0 |
| creso-aversa | 200 | new | 200 created | 200 (receipt) | `aversa martinelli creso\|1979-12-14` | 0 |
| francisco | 200 | new | 200 created | 200 (receipt) | `joao francisco de aguiar\|1969-08-05` (see D5) | 0 |
| maria-gorete | 200 | new | 200 created | 200 (receipt) | `maria gorete de carvalho\|1962-03-21` | 0 |
| rolmey | 200 | **unresolved** | **422** | **422** | none | 0 |

rolmey 422 body: `{"error":"não foi possível salvar o paciente: nome ou data de nascimento estão incompletos ou conflitantes"}`. Even so, Garage then held `refratia/cases/case-20261112T225657Z-59a694a7/` with 6 documents and `paciente_compilado.json`, plus the full draft. The case shows up in `GET /api/cases`.

Example object layout after a successful confirm (daniela-nogueira):
```
refratia/cases/case-…-670878e9/668bc237-526c239f-TOPO.pdf          (unclassified → random prefix)
refratia/cases/case-…-670878e9/EXAME__AO__DANIELA_JNOGUEIRA_ROCHA__20261112_224428.jpeg   (refractometry has no label → "EXAME")
refratia/cases/case-…-670878e9/MICROSCOPIA_ESPECULAR__AO__…jpg
refratia/cases/case-…-670878e9/PENTACAM__OD__… / PENTACAM__OS__…
refratia/cases/case-…-670878e9/paciente_compilado.json
refratia/intake-confirmations/intake-…-670878e9.json
refratia/file-checkpoints/v1/{sha}/source   (one per uploaded file; no result.json for these files)
```

### Fault-injection runs
Synthetic drafts were built from a stored analyze response by `scratchpad/persist/craft.sh`, with no OCR involved. It writes the same `intake.json` and draft objects that analyze writes, with a jq tweak to `analysis`.

| # | Scenario | Result |
|---|---|---|
| F1 | `patient.full_name=""`, confirm ×2 | 422 ×2. Case `case-…-b1d97fe1` (5 docs + marker) written anyway and listed by `/api/cases` with `patientName:""`. Draft kept, so each retry re-copies everything. |
| F2 | Two different intakes, same new patient, confirmed concurrently | Both 200 `created` → **two cases for one patient** (`…413c0f1c`, `…ea48e823`). One `patients` row (the unique index held, by luck of timing). |
| F3 | Same intake confirmed 3× concurrently | 3× 200, same caseId. Converges because keys are deterministic, but all three did the full copy, merge and write. |
| F4 | Two intakes for an existing patient confirmed concurrently, adding top-level marker `persist_x` vs `persist_y` | Both 200 `updated` to the same case. Final `paciente_compilado.json` has `persist_y=1` and **`persist_x=null`, a lost update**. |
| F5 | Analyze RETINA-OD + CORNEA-OD (both classified `fundus_retinography/OD`) in one batch | Draft has **1 object** for 2 files: both map to `RETINA__OD__…__20261112_225025.jpeg`. Size 211470 = the cornea image, so the retina bytes were overwritten. The manifest lists 2 files pointing to one key. Confirm then gave 422 (images carry no birth date) but still wrote the case (F1 again). |
| F6 | DELETE both F2 cases | 200. `patients` row `persist race alpha` remains with no case anywhere. |
| F7 | Manual exam on francisco case, `exams/confirm` ×2 | Object + `exams` row id=1 (`exam_type=''`, `eye=''`, `referenced=false`). Second call is idempotent (same row). |
| F8 | Manual exam on F1 case (no identity) | PUT 200, confirm **500** "não foi possível salvar o paciente; arquivo revertido" and the object is deleted. Retry gives 404. It returns 500 rather than 422, and a case without identity can never receive exams. |

Final SQL state: 6 patients (4 real + 2 synthetic), **1 exam row in total** (from F7). All intake-confirmed documents have no `exams` row.

## 3. Defects

Severity: **Critical** = loses or corrupts clinical data or misattributes a patient. **High** = inconsistent stores or unusable data. **Medium** = correctness under retry or concurrency. **Low** = hygiene.

| ID | Sev | Defect | Evidence |
|---|---|---|---|
| D1 | High | **Intake confirm writes zero `exams` rows.** Only the manual upload path writes them. SQL cannot answer "which exams does patient X have". | Table §2: 0 rows for all 4 confirmed patients |
| D2 | High | **Garage case + marker are written before identity is validated.** A 422 leaves a case listed as complete, with an empty or wrong patient, and the draft stays. "Invalid data persisted as complete." | rolmey, F1, F5 |
| D3 | Critical | **Draft key collision**: `CanonicalExamFilename` uses patient+exam+eye+second-resolution time, so two files with the same exam+eye in one batch overwrite each other. The manifest references two files with one object. | F5 (size proves overwrite) |
| D4 | Critical | **Name-only exact identity**: an OCR typo or name order change creates a new patient and a new case (`daniela jnogueira rocha`, `aversa martinelli creso` vs Pentacam's `CRESO AVERSA MARTINELLI`). Name conflicts between documents in the same batch are never checked. The daniela OS Pentacam `nome_lido` was garbage and was accepted. | §2 rows, `verificacao_identidade` dumps |
| D5 | Critical | **Birth date is guessed across documents.** `CanonicalBirthDate` takes one global DMY/MDY hint from any Pentacam timestamp and applies it to every document, including DMY-format EyeSuite/Biometria. francisco: EyeSuite `08/05/1969` (DMY = 8 May) and Pentacam `08/05/1969` (MDY = 5 Aug) were both read as MDY, so `1969-08-05` was persisted silently. rolmey: the same rule turned EyeSuite `12/05/1967` into Dec 5, which conflicts with Pentacam May 12, so the result is 422. The date order must be resolved per document. | francisco/rolmey identity entries |
| D6 | Medium | **Duplicate cases for one patient** under concurrent confirms: find-then-create on Garage has no lock. Legacy duplicates are then resolved by "lexicographically last case". | F2 |
| D7 | Medium | **Lost update** on `paciente_compilado.json` (read-merge-write without If-Match or a lock). | F4 |
| D8 | Medium | Patient upsert is lookup-then-insert without a transaction or `ON CONFLICT`. Concurrent first-time confirms can hit a unique violation, and the result is a 500 after Garage was written. Patient and exam inserts in `exams/confirm` are not in one transaction. | code `patient_database.go:42-61`, `exam_database.go:88-121` |
| D9 | Medium | Case delete does not touch SQL, which leaves orphan `patients` and `exams` rows. | F6 |
| D10 | Medium | Swallowed errors: `cleanupObjects` ignores delete errors, `findExistingPatientCase` skips unreadable cases (it can miss the real match and create a duplicate), and `examMetadataFromManifest` drops manifest errors (nil analysis gives the misleading "patient" 500). | code |
| D11 | Medium | Backend does not enforce classification. Unclassified files are persisted with a random name (`668bc237-526c239f-TOPO.pdf`), and manual exams are stored with `exam_type=''`. The UI is the only gate. | §2 layout, F7 |
| D12 | Low | Superseded documents (same exam+eye replaced on merge) stay in the case folder unreferenced. Only this request's own copies are cleaned. | code `intake_handlers.go:628-638` |
| D13 | Low | `refractometry` missing from `CanonicalExamFilename` labels, so files are named `EXAME__…`. | §2 layout |
| D14 | Low | GORM default logger prints `identity_key` (name + DOB) to stdout on every lookup miss: PII in logs. | `backend.log` |
| D15 | Low | Identity lookup lists every case and GETs every manifest on each analyze and confirm: O(cases) Garage reads. The `patients` table exists but is not used for this lookup. | code |

What already works: confirm retry after success is idempotent via the receipt (F3, double-confirm in §2). Analyze-time draft failures clean up their own keys. `exams/confirm` is idempotent on `object_key`. The 422 identity guard does refuse to create a SQL patient without name+DOB.

## 4. Proposed fix (smallest correct change)

Principle: Garage first, SQL last as the single commit point. Every Garage key is deterministic per `intakeId`, so retries overwrite instead of duplicating. SQL commits in one transaction. Whatever is in Garage but not committed in SQL counts as garbage and is collected later.

1. **Validate before writing** (D2, D5): in `confirmIntakeHandler`, call `patientfeature.Identity(draft.Analysis)` right after `validateStoredAnalysis` and return 422 before any CopyObject. Same in `confirmSavedExam`: identity failure → 422, not 500. Files: `intake_handlers.go`, `exam_database.go`.
2. **Per-document date order** (D5): in `CanonicalBirthDate`, derive the DMY/MDY hint from each entry's own `timestamp_lido` and only canonicalize that entry's `nascimento_lido` with it. An ambiguous entry without its own hint is skipped if another entry resolves the date, and only conflicts fail. Add the francisco and rolmey cases to `patient_test.go`. File: `features/patient/identity.go`. (This overlaps OCR/identity, so coordinate with the OCR investigator.)
3. **Unique draft keys** (D3, D13): append the 8-char sha256 prefix to the canonical name (`…__{ts}__{sha8}.{ext}`) and add `refractometry: "AUTO"` to the labels. File: `features/intake/classification.go`.
4. **One SQL transaction at the end of confirm** (D1, D8): `appDatabase.Gorm.Transaction(func(tx) { upsert patient with ON CONFLICT(identity_key) DO UPDATE; for each referenced source file: INSERT exams … ON CONFLICT(object_key) DO UPDATE })`. Do this after `paciente_compilado.json` is written and before the receipt. A retry re-runs it idempotently. Reuse the `upsertPatientFromAnalysis` body with a `tx` parameter. Files: `patient_database.go`, `exam_database.go` (extract a `upsertExamRows(tx, caseID, patientID, sourceFiles)`), `intake_handlers.go`.
5. **Patient match from SQL instead of scanning Garage** (D6, D15): add `case_id` to `patients` (one case per patient, as today). In confirm, `SELECT … FOR UPDATE` on `identity_key` inside the transaction from step 4. If the row exists, merge into its `case_id`; if not, insert it with the new case id. Concurrent first confirms then serialize on the unique index: the loser retries and becomes "updated". Keep the Garage scan only as a fallback for legacy cases without a row. Files: `database/exam.go`, `patient_upsert.go`, `intake_handlers.go`.
6. **Lost update on the compiled JSON** (D7): hold the patient row lock (step 5) while doing read-merge-write of `paciente_compilado.json`. Postgres becomes the mutex, so no Garage conditional writes are needed. ponytail: a row lock held across Garage I/O is fine at clinic scale; switch to If-Match when Garage supports it, if contention ever shows.
7. **Case delete removes SQL rows** (D9): after the objects are deleted, run `DELETE FROM exams WHERE case_id=?`, and delete the patient if it has no remaining case. File: `cases.go`.
8. **Log hygiene** (D14): `logger.Default.LogMode(logger.Silent)` (or Warn without params) in `database/database.go`.
9. **Reconciliation** (D2 legacy, D12): skip a background job for now. A one-off `cmd/` script that lists cases without a `patients.case_id` row, and case objects not referenced by the manifest, covers the existing debris. Add a scheduled job only if new orphans keep appearing after steps 1–6.

Not proposed: an outbox or saga table, a distributed lock service, content-addressed storage for all case files. Steps 1–6 close every reproduced defect, and each extra mechanism can wait until a defect actually needs it.

Rough diff: about 150 lines of Go across `intake_handlers.go`, `patient_database.go`, `exam_database.go`, `database/exam.go`, `features/patient/identity.go`, `features/intake/classification.go`, `cases.go`, plus tests in `features/patient/patient_test.go` and one confirm-path integration test against `testinfra`.

## Artifacts
Scratch scripts and raw responses are in `/tmp/claude-1000/-home-mateus-Desktop-Javascript-refratia/1fd31442-3487-4858-b33c-f8d40209f14c/scratchpad/persist/`: `run.sh`, `craft.sh`, `s3.sh`, `tamper.sh`, `*.analyze.json`, `batch1.log`, `backend.log`. The test bucket and DB still hold the state described above.

## Remaining work
- All 5 patients ran (daniela-nogueira, creso-aversa, francisco, maria-gorete, rolmey). Nothing from the corpus is left unrun.
- Not run: a real (OCR) re-analyze of an already-confirmed patient. Merge/"updated" behaviour was exercised only with synthetic drafts (F4). Superseded-file orphans (D12) were found by code inspection, not reproduced.
- Not run: a mid-confirm SQL outage (Postgres down after the Garage writes). The outcome is inferred from code: same as F1 (Garage case written, 500 returned, draft kept).
- D5 (birth-date order) was derived from the `verificacao_identidade` dumps plus code reading. It still needs a unit test with the francisco and rolmey entries in `features/patient/patient_test.go`.
- Phase 2 (implementing §4) has not started.

## Processes
- None left running. The backend I started (PID 43640, `PORT=3100 ./refratia` in scratchpad/persist, log `scratchpad/persist/backend.log`) was stopped after the runs finished, along with my monitors. Docker containers `refratia-test-pg` and `refratia-test-garage` were started by the lead and were not touched.

## Implementation log

Phase 2, 2026-11-12/13. All tests green (`go build ./... && go test ./...`, 153 tests; integration tests in `backend/confirm_integration_test.go` run only with `testinfra/test.env` sourced, they skip otherwise). Evidence runs: backend built from the working tree on `:3101` against `refratia_test`/`refratia-test` (wiped first), local OCR, all 5 folders analyze → confirm → confirm(retry), then a re-analyze (`force=1`) + confirm of francisco.

### Evidence summary (SQL after the 5-folder run, expected = ground truth `expected_exam_records.count`)
| folder | expected | rows | docs | empty exam_type | patients row |
|---|---|---|---|---|---|
| creso-aversa | 6 | 6 | 4 | 0 | `aversa creso martinelli\|1979-12-14`, case_id set |
| francisco | 10 | 10 | 8 | 0 | `aguiar de francisco joao\|1969-08-05` (Pentacam reading), warning `data_nascimento_invertida` |
| maria-gorete | 8 | 8 | 5 | 0 | `carvalho de gorete maria\|1962-03-21` |
| rolmey | 8 | 8 | 6 | 0 | `arantes rolmey silva\|1967-05-12` (was 422) |
| daniela-nogueira | 8 | 8 | 5 | 0 | OCR name garbage, flagged `nome_suspeito` (OCR-owned defect) |

Re-analyze + confirm of francisco: `patientMatch=existing` (found through `patients.case_id`, no Garage scan), confirm `updated`, same caseId, still 10 rows and 1 patient, 8 case objects (no duplicates), `paciente_compilado.json` `patient.birth_date=1969-08-05` equal to SQL. `go run ./cmd/reconcile`: 0 findings. Retry of confirm returned the receipt (200, same case) for all five.

### D5 per-document birth date (+C2 inversion warning)
- Root cause: one global DMY/MDY hint from any Pentacam timestamp applied to every document.
- Change: `features/patient/identity.go` `ResolveBirthDate`: order per document (own conclusive slash timestamp; else slash timestamp = Pentacam style MDY; else ISO timestamp = EyeSuite style DMY; else only unambiguous dates). Two documents may disagree only by a day/month swap: the Pentacam (reference) reading wins and `IdentityWarnings` emits `data_nascimento_invertida`. Any other disagreement stays unresolved (422). `analyze` puts warnings in `analysis.identity_warnings` (list of `{code, ...}`) and calls `NormalizeIdentityFields` so `patient.birth_date` is the final canonical value.
- Age: the backend computes no age anywhere (the only age code is `frontend/src/utils/date.ts`, which returns `null` for a missing/invalid date). Nothing to change; age 0 cannot come from the backend.
- Tests: `TestBirthDateDayMonthInversionKeepsPentacamAndWarns` (francisco), `TestBirthDatePerDocumentOrderAgrees` (rolmey), `TestBirthDateRealConflictIsUnresolved`, existing identity tests.
- Evidence: rolmey 422 → 200 with 1967-05-12; francisco 1969-08-05 + warning.
- Side effect: partial analyses of one patient (an EyeSuite-only and a Pentacam-only checkpoint) now resolve to different dates. `file_checkpoints.go:939` compares them with `!=` and fails ("arquivos pertencem a pacientes diferentes") on re-analyze without `force`. OPEN, OCR-owned: use the new `patientfeature.SameIdentity(a, b)` there.

### D2 identity before any Garage write
- Root cause: identity was checked only in the SQL upsert at the end.
- Change: `confirmIntakeHandler` validates `patientIdentity` and file classification before `commitIntake` (422 with no copy/marker/receipt); `confirmSavedExam` validates the case identity before SQL and answers 422 (was 500), reverting the just-uploaded object.
- Tests: `TestConfirmRejectsUnresolvedIdentityBeforeAnyGarageWrite` (no case objects, no receipt), `TestSavedExamConfirmIdentityFailureIs422AndRevertsObject`.

### D3/D13 unique draft keys, refractometry label
- Root cause: name = label+eye+patient+second; two same-type files in a batch collided; no `refractometry` label.
- Change: `CanonicalExamFilename(..., sha256)` returns `LABEL__EYE__PATIENT__{sha8}__{ts}.ext` (the canonical-name parser still accepts it) and labels refractometry `AUTO`.
- Test: `TestCanonicalExamFilenameIsUniquePerFileAndReparses`. Evidence: francisco case holds both CORNEA files as distinct objects.

### D1/D8/C1 one SQL transaction, row per (document, eye)
- Root cause: confirm wrote no exam rows; patient upsert was find-then-create outside a transaction; `UNIQUE(object_key)`.
- Change: `commitIntake` (`intake_handlers.go`): single `Gorm.Transaction`: lock patient, Garage copy + marker, then `savePatient` + `upsertExamRows` (`ON CONFLICT (object_key, eye) DO UPDATE`) + prune rows of documents the merge superseded. AO documents give OD+OS rows. `exam_type` uses the contract vocabulary (`examTypeKey`), never `''` (`unclassified` for legacy/manual). `database.Migrate` drops `idx_exams_object_key`, creates `idx_exams_object_eye UNIQUE(object_key, eye)`, rewrites legacy `''` types.
- Tests: `TestConfirmWritesOneRowPerDocumentAndEyeAndIsIdempotent` (4 rows from AO+OD+OS, retry no change). Evidence: table above.
- Vocabulary gap (OPEN, needs lead): the app can only classify the 6 `ExamTypeFromContract` types, so `CORNEA` photos and the `TOPO` pdf were classified `oct_retina` in the e2e run (ground truth: `anterior_segment_photo`, `corneal_topography_other`). Counts match, those two types do not. Fixing it means extending `ExamTypeFromContract`/label maps (classification.go logic, OCR-owned) and the frontend list.

### D6/D7/D15 patients.case_id + row lock
- Root cause: find-then-create and read-merge-write on Garage with no mutex; identity match by scanning every case.
- Change: `patients.case_id`; `lockPatient` = `INSERT ... ON CONFLICT (identity_key) DO NOTHING` + `SELECT ... FOR UPDATE`; the lock is held through the Garage merge+write, so concurrent confirms serialize. `resolveExistingCase` (analyze) and `commitIntake` use `patients.case_id`; the Garage scan stays only as fallback for legacy cases without a row (it still runs for brand-new patients: `ponytail` comment, drop after backfill).
- Tests: `TestConcurrentConfirmsOfSamePatientShareOneCase` (3 concurrent intakes, one case, 4 rows, none lost), second-intake "updated" in the idempotency test. Evidence: francisco re-analyze matched `existing` via SQL.

### D4 identity name rule
- Rule (`NameKey`): lowercase, accent/punctuation/whitespace stripped, tokens sorted and joined, so `SURNAME, NAME` and `NAME SURNAME` give the same key. identity_key = `NameKey|YYYY-MM-DD`. (Legacy rows keep their old key text; the Garage fallback re-matches them by the new key, `savePatient` then links the case.)
- Not merged automatically, but flagged (`analysis.identity_warnings`, non-blocking, persisted in the compiled JSON): `nome_divergente` (a document's name is not compatible with `patient.full_name`: compatible = every token of the shorter name appears in the longer one, whole or as a 1-letter initial, covering `MARIA GORETE DE C`), `nome_suspeito` (name has digits/symbols, OCR garbage), `nome_divergente_paciente_existente` (new identity but another patient has the same birth date: possible OCR typo or namesake; includes that patient's caseId; never merged or guessed).
- Tests: `TestNameKeyIgnoresOrderAccentsAndWhitespace`, `TestNameWarningForIncompatibleDocumentName`, `TestSuspiciousNameIsFlagged`. Evidence: francisco/maria/rolmey runs carry `nome_divergente` for the `jJO40`/`JDE`/`JROLMEY` Pentacam reads. OPEN (OCR): daniela's `patient.full_name` was `° 2/25 Segmento: 294° 114° \ NOGUEIRA ROCHA` in this run (only flagged).

### D9, D10, D14, D11
- D9: `deleteCaseRows` (exams then patient by `case_id`, one transaction) runs before the Garage deletes, so a retry after a partial Garage failure still cleans. Test `TestDeleteCaseRemovesSQLRows`.
- D10: `cleanupObjects` (`storage.go`, owner not listed, tiny edit) logs failures (counts only, keys carry patient names); the identity scan skips only not-found/corrupt manifests (logged by case id) and returns storage errors; `examMetadataFromManifest` returns its error (404/502, not a misleading 500). Not changed: `casesHandler` still skips unreadable cases in the listing.
- D14: `logger.Silent` in `database.Open` (identity_key no longer printed). Verified: backend log of the 5-folder run holds only the start line.
- D11: analyze answers 422 listing files without exam+eye (before OCR); confirm rejects drafts with unclassified files. Manual `exams/confirm` accepts optional `examType`/`eye` for files not in the manifest, else `unclassified`. Verified with a real call: 422. OPEN for the frontend: for files the app cannot classify (see vocabulary gap) the user must now pick a type.

### Reconcile
`go run ./cmd/reconcile` (report only, deletes nothing): `garage-case-without-sql`, `sql-case-without-marker`, `unreferenced-object`, `exam-without-object`, `case-without-marker`; prints case ids only. On the old repro debris it listed 9 findings; after the clean run, 0.

### Still open
1. OCR: `file_checkpoints.go` identity compare must use `patientfeature.SameIdentity` (re-analyze of an inverted-date patient without `force` returns 502).
2. Lead/OCR/frontend: exam_type vocabulary for out-of-protocol documents (`anterior_segment_photo`, `corneal_topography_other`).
3. OCR: daniela full_name garbage; Pentacam `nome_lido` garbage (`jJO40 JFRANCISCO`) only flagged.
4. D12 superseded objects stay in the case folder (SQL rows are pruned, objects not); reconcile reports them as `unreferenced-object`.
5. Not run: a Postgres outage mid-confirm (by design the SQL transaction rolls back, Garage objects are overwritten by the retry).

### Evidence run 2 (daniela-nogueira, real OCR, no force): analyze → confirm → confirm retry → re-analyze → confirm
After wiping `refratia_test`/`refratia-test`. Backend built after the name-plausibility and out-of-protocol fixes.
- analyze: `status=new`, `patient.full_name="DANIELA JNOGUEIRA ROCHA"` (the plausible OCR variant; the garbage `° 2/25 Segmento…` read was rejected and only reported as `nome_divergente`), birth 2001-01-28. confirm #1 `created`, confirm #2 receipt (same case).
- `patients`: 1 row `daniela jnogueira rocha|2001-01-28`, `case_id=case-20261113T001524Z-411ea140`.
- `exams` (8 rows, ids 1-8, all `patient_id=1`, same `case_id`): `corneal_topography_other` OD+OS (548345 bytes), `refractometry` OD+OS (124525), `specular_microscopy` OD+OS (511253), `pentacam_corneal_tomography` OD (3392460) and OS (3397551). No `oct_retina`, no empty type.
- Garage `cases/<case>/`: `TOPO__AO__…__6eb3a5f1__…pdf` 548345, `AUTO__AO__…30e84a93…jpeg` 124525, `MICROSCOPIA_ESPECULAR__AO__…2967b9a9…jpg` 511253, `PENTACAM__OD__…934368e1…pdf` 3392460, `PENTACAM__OS__…5aea700a…pdf` 3397551, `paciente_compilado.json` 32849; receipt 148 bytes; drafts emptied.
- re-analyze without force: `status=existing` (via `patients.case_id`), confirm `updated`, same caseId; afterwards still 1 patient, 8 rows, 8 distinct (object_key, eye), same 5 documents, 0 drafts left, `cmd/reconcile` 0 findings.

### Near-duplicate identity decision (C2)
- Rule: at confirm, if no patient has the exact `identity_key` but another has the same birth date and a `NamesSimilar` name (compatible/partial, or same token count with at most 2 edits per sorted token), confirm answers `409 {"error":"identity_review_required","candidates":[{caseId,name}]}` before any Garage or SQL write. The body may carry `identityDecision`: `{"action":"existing","caseId":…}` merges into that patient (keeps its identity_key and name, adds an `nome_variante` warning to the analysis) or `{"action":"new"}` creates another patient knowingly. A caseId that is not a candidate gives 400.
- Test (synthetic drafts against testinfra): `TestNameTypoVariantRequiresIdentityDecision`: typo seed, clean variant with no decision gives 409 with 1 candidate, no case objects, 1 patient; "existing" gives `updated` same case, 1 patient; "new" gives a second patient (2). Unit: `TestNamesSimilarCatchesOCRTypoButNotOtherPeople`.
- Frontend: `useIntakeWorkflow.ts` (`identityReview` state, `confirmIntake(decision)`) and the confirm block in `NewAnalysisPage.tsx` (candidate list, two buttons). `tsc --noEmit` clean.

### Evidence run 3 (francisco, real OCR, no force): analyze → confirm → confirm retry → re-analyze → confirm
After wiping `refratia_test`/`refratia-test`, with the OCR vocabulary (`anterior_segment_photo`) and `date_order` landed.
- Date resolution: the identity entries carry `date_order` (EyeSuite `DMY`, both Pentacam `MDY`; others null with no birth date), so the declared-order path resolved both dates (EyeSuite 08/05/1969 DMY = 1969-05-08, Pentacam 08/05/1969 MDY = 1969-08-05); the heuristic was not needed. Result: `patient.birth_date=1969-08-05`, warning `data_nascimento_invertida`.
- `patients`: 1 row `aguiar de francisco joao|1969-08-05`, `case_id=case-20261113T010012Z-1deff0d9`.
- `exams`: 10 rows, all `patient_id=1`, same `case_id`: `anterior_segment_photo` OD 211470 / OS 177797 bytes; `iol_calculation` OD+OS 1167950; `specular_microscopy` OD+OS 466512; `pentacam_corneal_tomography` OD 3358236 / OS 3356788; `fundus_retinography` OD 210797 / OS 220387. No `oct_retina`.
- Garage case objects (after confirm #2 and after the re-analyze + confirm, identical): ANTSEG OD 211470, ANTSEG OS 177797, EYESUITE 1167950, MICROSCOPIA_ESPECULAR 466512, PENTACAM OD 3358236, PENTACAM OS 3356788, RETINA OD 210797, RETINA OS 220387, `paciente_compilado.json` 45667 (after the update merge).
- Re-analyze without force: `patientMatch=existing` (SQL), confirm `updated`, same caseId; still 1 patient, 10 rows, 8 documents. `cmd/reconcile`: 0 findings. (Three unrelated synthetic drafts from the integration tests sit under `drafts/`; they are not from this run.)

### SQL failure after the Garage writes (C3)
`TestConfirmSurvivesSQLFailureAfterGarageWrites` injects a failure through the test seam `afterGarageWrites` (inside the commit transaction, after the Garage writes, before the SQL rows). Asserted: confirm 500 with no receipt; draft objects kept; 0 patients and 0 exams committed; the failed attempt's case objects (3 docs + marker) exist as garbage (`cmd/reconcile` lists them as `garage-case-without-sql`); retry with the same intakeId succeeds on the same caseId, 1 patient, 4 exam rows, the object count unchanged (same deterministic keys reused, no duplicates), draft consumed. Passed 3 of 3 runs. No defect found; the only code change is the 4-line seam.
