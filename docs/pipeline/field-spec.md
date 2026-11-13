# RefratIA: field specification and benchmark ground truth

Sources: `docs/Protocolo_RefratIA_FINAL.pdf` (revision 17/10/2026, items 1–78: section 0, 0.A–0.D, section 1 "Dados de entrada necessários", section 2 parameters, Fluxos A/B/C, section 8) and `docs/RefratIA_Guia_Implementacao_Programador.pdf`. The app shapes come from `backend/features/patient/contract.go` (official exam keys), `frontend/src/contracts/exam-contracts.ts` (audit field paths) and `frontend/src/contracts/patient-analysis.ts`.

There is also a machine-readable version: `backend/benchmark/groundtruth/_field_spec.json`. It gives every field's unit, kind, printed decimals, plausible range, protocol role and app JSON path(s).

## 1. Protocol rules that matter for extraction

- **Never infer (section 0).** A value that is not printed is `missing`, never estimated. Every value used is either `[bruto]`, with its source screen named, or `[calculado: fórmula]`.
- **Decisive values (section 0).** These must be declared and cross-checked automatically:
  - K, thinnest pachymetry, ACD Int, endothelial count, refraction and biometry astigmatism.
  - Cross-checks named by the protocol: ACD Int vs Ext vs biometer, and ACD Ext − ACD Int ≈ central pachymetry.
  - A cross-check that does not close gives confidence `suspeita — revisar`. It never blocks the cascade.
- **Identity (0.B) is the only blocking category.**
  - The display name always comes from the Pentacam.
  - Birth dates are compared exactly. The Pentacam prints **MM/DD/YYYY**; EyeSuite/LS900 prints **DD/MM/YY(YY)**. A difference that is only a day/month inversion is `data_nascimento_invertida`: moderate confidence, non-blocking. Any other difference blocks.
  - Documents without a birth date (Nidek, retinography) use a tolerant name match plus the timestamp.
  - An omitted or abbreviated surname is `nome_parcial` ("⚠ REVISAR"). It blocks only when that document supplies a decisive value, such as the endothelial count in Fluxo A Etapa 6 or Fluxo C.
- **Time window (0.C).** An exam more than 4 months from the Pentacam date (the most recent of OD/OS) does not enter the engine.
- **Quality (0.C).** Pentacam QI ≠ "OK" adds the observation `qualidade_exame`, and that exam's decisive data gets confidence "suspeita".
- **Minimum age** is 18. Flows by age: A is 18–39, B is 40–54, C is ≥55.
- **Refraction.** Any type is accepted (dynamic, static or autorefraction), and the type is recorded. A missing cylinder means cylinder = 0 and is *not* "dado ausente". The refraction is clinical (spectacle plane), with a vertex distance of 12.00 mm assumed.
- **Ectasia screens.** BAD-D/ARTmax come only from "Ectasia Reforçada Belin/Ambrósio", and ISV…TKC only from "Topométrico/Estadiamento KC". When those screens are absent, the fields stay `[ausente]`; the system never assumes "normal".
- **ACD for a phakic lens** is Pentacam **Cataract Pre-OP (Int.)** only, with a cut of ≥3.0 mm. Never use Ext. or the biometer ACD.
- **Biometry astigmatism (Fluxo C, toric vs non-toric, cut 0.75 D)** comes **only from the biometer**. The Pentacam is never a fallback.
- **Coma (items 75/77/78).**
  - It is an informative record with no cut, reported with the zone and column.
  - The zone is now 4.0 mm, from the Pentacam "Aberrometry Cornea" page (Diâmetro Máximo 4.00 mm). That page gives the Coma 0° and Coma 90° components in the WFA Anterior / WFA Posterior / WFA Córnea columns.
  - The magnitude has to be composed as √(c0² + c90²). Which column to use is still undecided; WFA Córnea is the protocol's suggestion.
  - The Anéis Corneanos 5/6 mm Z31 is the legacy source. The app contract still reads 5 mm.
- **Informative only:** Z40 (6 mm, Cataract Pre-OP), PNS, retinography, OCT (Fluxo C only), scotopic pupil, and total ocular aberrometry (iTrace, not yet owned by the clinic).
- **Equipment outside the section 1 table** (e.g. the TP-15 Apramed topographer) is used for identity and cross-checks only, never in formulas or cuts.

## 2. Patient-level fields

| Field | Normalized form | Source / rule | App path |
|---|---|---|---|
| full_name | "First Surname" | Pentacam Nome + Sobrenome (the protocol display name) | `patient.full_name` |
| birth_date | ISO `YYYY-MM-DD` | Pentacam (MDY), cross-checked with the biometer (DMY) | `patient.birth_date` |
| sex | `F`/`M`/null | Only if printed. In this corpus it appears only on the TP-15 topographer | — |
| reference_exam_date | ISO | Pentacam Data de Exame (most recent of OD/OS) | `exams.pentacam_corneal_tomography.eyes.<EYE>.exam_at` |
| age | calculated | birth_date → reference_exam_date (selects Fluxo A/B/C) | — |

Each document also carries an identity block: name/ID as printed, birth date (raw and ISO), exam date/time, eye as printed and device IDs.

## 3. Per-exam fields

**Roles:**

- **decisive:** a cut changes the conduct.
- **screening:** ectasia rule 2.1.
- **informative:** reported, no cut.
- **cross_check:** used for the automatic consistency checks.
- **extra:** printed but not used by the protocol.

All numeric fields are per eye.

### 3.1 Pentacam: `pentacam_corneal_tomography` (35 fields, 1 file per eye)

App paths are relative to `exams.pentacam_corneal_tomography.eyes.<EYE>`.

| Key | Unit | Printed dp | Range | Role | Screen | App path |
|---|---|---|---|---|---|---|
| qi | – | – | "OK"/other | informative | Topométrico, Córnea Ant | `quality` |
| k1_d, k2_d, km_d | D | 1 | 30–60 | decisive | Topométrico, Córnea Ant | `anterior_cornea.k1_d/k2_d/km_d` |
| astig_ant_d | D | 1 | 0–15 | informative (item 69) | Topométrico | `anterior_cornea.astigmatism_d` |
| axis_flat_deg | ° | 1 | 0–180 | informative | Topométrico | `anterior_cornea.axis_deg` |
| kmax_ant_d | D | 1 | 30–80 | screening | Topométrico "K Máx. (ant.)" | `anterior_cornea.kmax_d` |
| pachy_thinnest_um | µm | 0 | 300–750 | decisive (LER/PTA) | Topométrico "Ponto + fino" | `pachymetry.thinnest_um` |
| pachy_pupil_center_um, pachy_vertex_um | µm | 0 | 300–750 | cross_check | Topométrico | `pachymetry.pupil_center_um`, `.vertex_normal_um` |
| acd_ext_mm | mm | 2 | 1.5–5.5 | cross_check | Cataract Pre-OP "Prof.Câm.Ant (Ext.)" | `anterior_segment.external_anterior_chamber_depth_mm` |
| acd_int_mm | mm | 2 | 1.0–5.0 | **decisive** (phakic ≥3.0) | Cataract Pre-OP "(Int.)" | `anterior_segment.internal_anterior_chamber_depth_mm` (frontend contract also tries `cataract_preop.acd_mm`) |
| pns | – | 0 | 0–5 | informative | Cataract Pre-OP | `anterior_segment.pns` |
| z40_6mm_um | µm | 3 | −0.5–1.5 | informative | Cataract Pre-OP "Total Corneal Z40 (6mm)" | `cataract_preop.total_corneal_z40_6mm_um` |
| bad_d | – | 2 | −3–25 | screening (<1.6 / 1.6–2.6 / >2.6) | Belin/Ambrósio "D:" | `belin_ambrosio.d` |
| art_max_um | µm | 0 | 50–1000 | screening (>405 / 340–405 / <340) | Belin/Ambrósio | `belin_ambrosio.art_max` |
| isv, iva, iha, ihd, ki, cki | – | 0/2/1/3/2/2 | see json | screening | Topométrico "Índices (zona de 8mm)" | `topometric_indices_8mm.*` |
| tkc | – | – | text | screening (rule 2.1) | Topométrico | `topometric_indices_8mm.tkc` (printed "-" → `null`) |
| abcd_staging | – | – | e.g. "A0 B0 C0 D" | screening | Topométrico "Belin ABCD" summary line | the app stores numeric `abcd_keratoconus_staging.a/b/c/d`; it has no summary string |
| coma_z31_5mm_um, coma_z31_5mm_axis_deg, coma_z31_6mm_um | µm, ° | 3, 0 | 0–5 | informative (legacy 5 mm) | Anéis Corneanos | `corneal_rings.zernike.5mm.z31_coma`, stored as the string "0.097 µm @ 151°", so parse it |
| aberr_max_diameter_mm | mm | 2 | – | informative | Aberrometry Cornea header | — (not in the app) |
| coma0/coma90 _4mm_wfa_{ant,post,cornea}_um | µm | 3 | −3–3 | informative (items 75–78) | Aberrometry Cornea rows N=8 / N=9 | — (not in the app) |
| coma_4mm_wfa_cornea_magnitude_um | µm | 3 | 0–3 | informative, **calculated** √(c0²+c90²) | – | — |

### 3.2 Biometry / IOL: `iol_calculation` (19 fields, AO document → OD + OS)

Device: Haag-Streit EyeSuite IOL / LS900. It uses a decimal comma and DD/MM/YYYY dates. App paths are relative to `exams.iol_calculation.eyes.<EYE>`.

| Key | Unit | dp | Role | App path |
|---|---|---|---|---|
| astig_d | D | 2 | **decisive** (Fluxo C toric ≥0.75) | `keratometry.astigmatism_d`. Printed as "-AST −0,28 @ 54" and kept **signed as printed**; compare `abs()` for the protocol cut |
| astig_axis_deg | ° | 0 | informative | `keratometry.astigmatism_axis_deg` |
| k1_d / k2_d / k_mean_d (+ radius_mm, axis_deg) | D / mm / ° | 2 / 2 / 0 | cross_check / extra | `keratometry.k1_d`, `.k2_d`, `.mean_k_d`, … |
| al_mm, lt_mm, wtw_mm | mm | 2 | informative | `axial_length_mm`, `lens_thickness_mm`, `white_to_white_mm` |
| acd_mm (from epithelium), ad_mm (aqueous), cct_um | mm / µm | 2 / 0 | cross_check only, **never** the phakic criterion | `anterior_chamber_depth_mm`, `aqueous_depth_mm`, `central_corneal_thickness_um` |
| lens_status, measurement_date, target_refraction_d | – | – | informative / extra | `status`, —, `target_refraction_d` (exam level in the app) |

### 3.3 Specular microscopy: `specular_microscopy` (9 fields, AO → R = OD on top, L = OS on the bottom)

The decisive field is `cell_density_cells_mm2` (cells/mm², integer; cut ≥2000 for phakic, and a conditioning value for multifocal/EDOF in Fluxo C). App path: `exams.specular_microscopy.eyes.<EYE>.cell_density_cells_per_mm2`.

The other fields are `num_cells`, `avg_area_um2`, `sd_um2`, `cv_pct`, `max_area_um2`, `min_area_um2`, `hex_pct` and `ct_um` (cross_check against the Pentacam pachymetry).

The Nidek header has an "ID" field that holds the (often truncated) name, and a "Name" field that in this corpus always reads "HCV,". There is no birth date.

### 3.4 Refractometry: `refractometry` (8 fields, AO ticket)

The source is the PRK-6000 autorefractor ticket, AVE row per eye.

- `sphere_d`, `cylinder_d` (minus-cylinder form) and `axis_deg` are **decisive**. A missing cylinder means 0.
- `refraction_type` is "autorrefração".
- `vertex_distance_mm` comes from "UD: 12.0".
- `pd_mm` and `cyl_form` are extra.
- `iop_avg_mmhg` comes from the NIDEK NT-530 tonometer on the same scan. It is outside the protocol (extra).

App paths: `exams.refractometry.eyes.<EYE>.sphere_d|cylinder_d|axis_deg`.

### 3.5 Retinography: `fundus_retinography` (2 fields)

- `mode` is the overlay "Mode".
- `findings` is `not_applicable`: the image has no printed findings, and the clinical reading is the physician's. The protocol treats retinography as informative only.
- Identity comes from the overlay: ID (= name), Eye, Time.

### 3.6 Out of protocol and app contract

- **`anterior_segment_photo`** (francisco `CORNEA__*`, 1 field: `mode`). This is the fundus camera in "Cornea" mode. `resolveExamKeyFromSource` returns undefined for the `CORNEA` prefix.
- **`corneal_topography_other`** (daniela TP-15, 5 fields: SimK 3 mm steep/flat D and axis, cyl). It is cross-check only, and it is the only document that prints sex and age.
- **OCT** (`oct_retina`): no OCT document exists in the corpus.

## 4. Ground-truth file schema (`backend/benchmark/groundtruth/<folder>.json`, `schema_version: refratia-groundtruth/1`)

```
{
  folder, method, generated_on,
  patient: { full_name|birth_date|sex|reference_exam_date: <IdentValue> },
  identity_checks: [ { file, result: confirmed|confirmed_moderate|revisar|NAO_CONFIRMADA, method, detail, blocking_if_decisive? } ],
  expected_exam_records: { count, count_in_protocol,
     records: [ { exam_type, app_exam_key (null = no app key), in_protocol, eye: OD|OS, source_file } ] },
  files: [ { file, exam_type, app_exam_key, device, eye: OD|OS|AO, pages, screens?,
             identity: { <name>: <IdentValue> },
             fields: { "OD"|"OS": { <field_key>: <FieldValue> } } } ],
  needs_human: [ "<file> :: <EYE>.<field>" | "<file> :: identity.<k>" | "patient.<k>" ],
  notes: [ ... ]
}
FieldValue = { value: number|string|null, raw: "as printed"|null, unit, status, verification,
               evidence: { page, screen, region }, protocol_role,
               tolerance?: { abs, decimals_printed }, note? }
IdentValue = { value (normalized: ISO date / trimmed text), raw, status, verification, evidence, note? }
```

**`status`:**

- `valid`: printed and read.
- `missing`: not printed, or the screen is absent from the export.
- `unreadable`: printed but illegible.
- `not_applicable`: the field does not apply to this document.

**`verification`:**

- `agent_visual`: read from a 200 dpi render and re-read on a 400–500 dpi crop (PDF), or at native resolution with a zoom crop (JPEG).
- `needs_human`: still ambiguous after zooming. The value may hold the best reading, but **do not score it**.

**Normalization:**

- Decimal comma → dot.
- Signs as printed.
- Units stripped (unit kept in `unit`).
- Axes are integers or one-decimal degrees as printed.
- Dates are ISO, converted using each device's convention (Pentacam MDY; EyeSuite, Nidek, PRK-6000 DMY; TP-15 M/D/YYYY; retina overlay YMD).
- TKC printed "-" → `value: null`, `status: valid`.

**Tolerance and scoring:**

- `abs: 0` means an exact match at printed precision. Compare after rounding the app value to `decimals_printed`.
- The only calculated field, the 4 mm coma magnitude, allows `abs: 0.001`.

**Records:** one record per (exam_type, eye, source file). AO documents produce two records. In-protocol record counts are rolmey 8, creso-aversa 6, daniela-nogueira 6 (+2 TP-15), francisco 8 (+2 CORNEA photos), maria-gorete 8.

## 5. Corpus inventory

| Folder | Files | Exam records (in protocol) |
|---|---|---|
| rolmey | EyeSuite AO, Nidek AO, Pentacam OD/OS (9 p), Retina OD/OS | 8 (8) |
| creso-aversa | Biometria SRK/T AO (5 p), Microscopia (filename has no name), Pentacam OD (12 p, incl. 3 Aberrometry Cornea) / OS (11 p, 2 Aberrometry) | 6 (6) |
| daniela-nogueira | TP-15 TOPO AO, AUTO AO (photo of a ticket), Nidek AO, Pentacam OD/OS (9 p) | 8 (6) |
| francisco | CORNEA OD/OS, EyeSuite AO (5 p), Nidek AO, Pentacam OD/OS (9 p), Retina OD/OS | 10 (8) |
| maria-gorete | AUTO AO (scan: PRK-6000 + NT-530 + handwriting), Biometria AO (5 p), Nidek AO, Pentacam OD (12 p) / OE (11 p) | 8 (8) |

Only creso-aversa and maria-gorete have the 4 mm Aberrometry Cornea pages. No folder has an OCT. Refractometry exists only for daniela-nogueira and maria-gorete.

## 6. Identity findings (relevant for grouping)

No document in any folder belongs to a different person.

- **francisco, a real birth-date ambiguity.** The Pentacam and EyeSuite both print the *string* "08/05/1969". Under each device's convention that is 1969-08-05 (Pentacam) vs 1969-05-08 (EyeSuite): a day/month inversion. It is non-blocking under 0.B, but the true birth date is unknown, so `patient.birth_date` is needs_human. A naive string comparison would wrongly report an exact match.
- **rolmey, creso-aversa, maria-gorete:** biometer and Pentacam birth dates match once the formats are applied. For rolmey the raw strings look inverted (05/12 vs 12/05); for creso-aversa and maria-gorete they are unambiguous (day >12).
- **`nome_parcial` (⚠ REVISAR)** cases. Each blocks if its decisive value is used:
  - Nidek microscopy in every folder: "ROLMEY ARANTES S", "CRESO AVERSA", "DANIELA NOGUEIRA", "JOAO FRANCISCO", "MARIA GORETE DE C".
  - daniela-nogueira autorefraction: handwritten "DANIELA N.".
  - daniela-nogueira TP-15: non-blocking.
- **Name variants:** "JOÃO" (Pentacam) vs "JOAO" (others). The daniela-nogueira Pentacam reads "NOGUEIRA ROCHA, DANIELA" while the folder name is daniela-nogueira.
- **maria-gorete:** the autorefraction/tonometry tickets carry no name. Identity rests on the handwriting "Maria Gorete de Carvalho, 2?/03/1962".

## 7. needs_human items (2)

1. `francisco.json :: patient.birth_date`: 1969-08-05 (Pentacam MDY) vs 1969-05-08 (EyeSuite DMY) from the same string "08/05/1969".
2. `maria-gorete.json :: AUTO AO MARIA GORETE.pdf :: identity.birth_date`: handwritten "2?/03/1962". The best reading is 21, which matches the Pentacam, but the second digit could be an 8.

Every clinical numeric value in the corpus was read unambiguously at high zoom. There are no numeric needs_human items.

## 8. Discrepancies with prior app outputs (`frontend/data/paciente_compilado.json`, `frontend/data/rolmey/paciente_compilado.json`, both rolmey)

- **OS ARTmax:** printed **366** (verified at 500 dpi); the app output says 368.
- **OS Prof.Câm.Ant (Ext.):** printed **3.90 mm**; the app output says 4.0.
- **OS Belin progression index "Méd":** printed 1.15; the app output says 1.19. This field is not part of the spec.

## 9. Other observations for the benchmark

- **ACD Int below the 3.0 mm phakic cut:**
  - francisco: OD 2.60, OS 2.54.
  - maria-gorete: OD 2.66, OS 2.71.

  The biometer ACD (≥3.0) is the wrong source. This is a good regression test for source exclusivity.
- **creso-aversa OD PNS** box is printed empty → `missing`.
- **creso-aversa ABCD summary vs bars.** The summary line prints "A0 B0 C0 D" while the bars show non-zero values. The summary is recorded and the difference is noted.
- **daniela-nogueira:** ISV is flagged red in both eyes (42/45) with TKC "-" and BAD-D around 1.0.
- **daniela-nogueira refraction:** the OD AVE sphere −3.50 is recorded as printed, although the individual readings are −3.75 / −3.50 / −3.75.
- **maria-gorete:** the autorefraction is dated one day after the Pentacam (2026-10-28 vs 10-27). The tonometer clock reads "00:26".
- **Builder scripts:** `gtlib.py`, `gt_<folder>.py` and the crop helpers are in the session scratchpad, `/tmp/claude-1000/-home-mateus-Desktop-Javascript-refratia/1fd31442-3487-4858-b33c-f8d40209f14c/scratchpad/gt/`. They are not committed.

## Remaining work

- **Unread documents: none.** All files in all 5 folders (rolmey, creso-aversa, daniela-nogueira, francisco, maria-gorete) have been read. Their ground truth is written to `backend/benchmark/groundtruth/<slug>.json`, and no field was guessed or omitted.
- **Open items: only the 2 needs_human identity items in section 7.** They need a human to resolve.
- **Not read on purpose** (outside the spec):
  - IOL power tables
  - Pentacam maps and Scheimpflug pages
  - Belin Df/Db/Dp/Dt/Da
  - Belin progression indices
  - Individual autorefraction readings other than the AVE row
- **Render scratch dir:** `/tmp/claude-1000/-home-mateus-Desktop-Javascript-refratia/1fd31442-3487-4858-b33c-f8d40209f14c/scratchpad/gt/`, with one subfolder per patient. It also holds the builder `gtlib.py`, the `gt_<folder>.py` scripts and the crop helpers `panels.py` / `crop.py`. Rebuild with `cd` there, then `python3 gt_<folder>.py`.
