# Baseline report (unmodified code)

Backend: commit `9eb5152` (HEAD when the benchmark was built) plus only the uncommitted `REFRATIA_NO_DOTENV` guard in `backend/database/database.go`, built from a separate git worktree so concurrent edits by other agents do not leak in. Source documents are named `<slug>/<exam_type>.<eye>` below; names and birth dates are omitted. Full per-run raw data stays local in `backend/benchmark/out/`.

# RefratIA benchmark report

Started 2026-11-12T21:51:08-03:00, runtime 1043s. Backend built from `<worktree of 9eb5152 + dotenv guard>/backend`.

Command: `npm run benchmark -- -backend-dir <worktree of 9eb5152 + dotenv guard>/backend -update-baseline`

Patients are referred to by folder slug. Field values are shown; names and birth dates are not.

## Summary per patient

| patient | extraction accuracy | unexpected values | not in app contract | persistence + identity checks | analyze |
|---|---|---|---|---|---|
| creso-aversa | 56/108 (51.9%) | 0 | 18 | 7/15 (46.7%) | 129s (HTTP 200) |
| daniela-nogueira | 10/84 (11.9%) | 0 | 28 | 6/15 (40.0%) | 125s (HTTP 200) |
| francisco | 35/112 (31.2%) | 0 | 20 | 8/15 (53.3%) | 116s (HTTP 200) |
| maria-gorete | 55/120 (45.8%) | 0 | 22 | 9/15 (60.0%) | 137s (HTTP 200) |
| rolmey | 34/112 (30.4%) | 0 | 18 | 2/13 (15.4%) | 118s (HTTP 200) |

## Accuracy per exam type

| key | accuracy |
|---|---|
| fundus_retinography | 8/8 (100.0%) |
| iol_calculation | 80/144 (55.6%) |
| pentacam_corneal_tomography | 89/270 (33.0%) |
| refractometry | 4/24 (16.7%) |
| specular_microscopy | 9/90 (10.0%) |

## Failures by stage

| stage | count |
|---|---|
| region (inferred) | 306 |
| mapping (inferred) | 21 |
| recognition (inferred) | 19 |

## incorrect

| field id | role | expected | got | stage |
|---|---|---|---|---|
| creso-aversa|pentacam_corneal_tomography|OD|coma_z31_5mm_axis_deg | informative | 207 | 0.09 | recognition (inferred) |
| creso-aversa|pentacam_corneal_tomography|OD|tkc | screening | null | "—" | recognition (inferred) |
| creso-aversa|pentacam_corneal_tomography|OS|coma_z31_5mm_axis_deg | informative | 280 | 0.21 | recognition (inferred) |
| creso-aversa|pentacam_corneal_tomography|OS|isv | screening | 17 | 7 | recognition (inferred) |
| creso-aversa|pentacam_corneal_tomography|OS|tkc | screening | null | "—" | recognition (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|isv | screening | 42 | 44 | recognition (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|isv | screening | 45 | 44 | recognition (inferred) |
| francisco|pentacam_corneal_tomography|OD|isv | screening | 9 | 44 | recognition (inferred) |
| francisco|pentacam_corneal_tomography|OS|isv | screening | 14 | 44 | recognition (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|coma_z31_5mm_axis_deg | informative | 93 | 0.213 | recognition (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|ki | screening | 0.97 | 0.9 | recognition (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|tkc | screening | null | "—" | recognition (inferred) |
| maria-gorete|pentacam_corneal_tomography|OS|coma_z31_5mm_axis_deg | informative | 95 | 0.178 | recognition (inferred) |
| maria-gorete|pentacam_corneal_tomography|OS|isv | screening | 12 | 2 | recognition (inferred) |
| maria-gorete|pentacam_corneal_tomography|OS|tkc | screening | null | "—" | recognition (inferred) |
| maria-gorete|refractometry|OD|axis_deg | decisive | 100 | 188 | recognition (inferred) |
| maria-gorete|refractometry|OD|cylinder_d | decisive | -1.5 | -1.58 | recognition (inferred) |
| rolmey|pentacam_corneal_tomography|OD|isv | screening | 10 | 44 | recognition (inferred) |
| rolmey|pentacam_corneal_tomography|OS|isv | screening | 20 | 44 | recognition (inferred) |

## missing

| field id | role | expected | got | stage |
|---|---|---|---|---|
| creso-aversa|iol_calculation|OD|ad_mm | cross_check | 3.21 | null | region (inferred) |
| creso-aversa|iol_calculation|OD|cct_um | cross_check | 534 | null | region (inferred) |
| creso-aversa|iol_calculation|OD|k1_axis_deg | extra | 168 | null | region (inferred) |
| creso-aversa|iol_calculation|OD|k1_radius_mm | extra | 7.64 | null | region (inferred) |
| creso-aversa|iol_calculation|OD|k2_axis_deg | extra | 78 | null | region (inferred) |
| creso-aversa|iol_calculation|OD|k2_radius_mm | extra | 7.47 | null | region (inferred) |
| creso-aversa|iol_calculation|OD|k_mean_radius_mm | extra | 7.56 | null | region (inferred) |
| creso-aversa|iol_calculation|OD|lens_status | informative | "Phakic" | null | region (inferred) |
| creso-aversa|iol_calculation|OS|ad_mm | cross_check | 3.23 | null | region (inferred) |
| creso-aversa|iol_calculation|OS|cct_um | cross_check | 532 | null | region (inferred) |
| creso-aversa|iol_calculation|OS|k1_axis_deg | extra | 12 | null | region (inferred) |
| creso-aversa|iol_calculation|OS|k1_radius_mm | extra | 7.59 | null | region (inferred) |
| creso-aversa|iol_calculation|OS|k2_axis_deg | extra | 102 | null | region (inferred) |
| creso-aversa|iol_calculation|OS|k2_radius_mm | extra | 7.37 | null | region (inferred) |
| creso-aversa|iol_calculation|OS|k_mean_radius_mm | extra | 7.48 | null | region (inferred) |
| creso-aversa|iol_calculation|OS|lens_status | informative | "Phakic" | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OD|abcd_staging | screening | "A0 B0 C0 D" | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OD|acd_ext_mm | cross_check | 3.77 | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OD|axis_flat_deg | informative | 174.8 | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OD|coma_z31_6mm_um | informative | 0.184 | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OD|pachy_pupil_center_um | cross_check | 543 | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OD|pachy_vertex_um | cross_check | 543 | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OD|qi | informative | "OK" | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OS|abcd_staging | screening | "A0 B0 C0 D" | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OS|acd_ext_mm | cross_check | 3.79 | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OS|axis_flat_deg | informative | 7.7 | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OS|coma_z31_6mm_um | informative | 0.27 | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OS|pachy_pupil_center_um | cross_check | 540 | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OS|pachy_vertex_um | cross_check | 539 | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OS|pns | informative | 0 | null | region (inferred) |
| creso-aversa|pentacam_corneal_tomography|OS|qi | informative | "OK" | null | region (inferred) |
| creso-aversa|specular_microscopy|OD|avg_area_um2 | extra | 335 | null | region (inferred) |
| creso-aversa|specular_microscopy|OD|ct_um | cross_check | 534 | null | region (inferred) |
| creso-aversa|specular_microscopy|OD|cv_pct | informative | 29 | null | region (inferred) |
| creso-aversa|specular_microscopy|OD|hex_pct | informative | 68 | null | region (inferred) |
| creso-aversa|specular_microscopy|OD|max_area_um2 | extra | 899 | null | region (inferred) |
| creso-aversa|specular_microscopy|OD|min_area_um2 | extra | 133 | null | region (inferred) |
| creso-aversa|specular_microscopy|OD|num_cells | extra | 239 | null | region (inferred) |
| creso-aversa|specular_microscopy|OD|sd_um2 | extra | 98 | null | region (inferred) |
| creso-aversa|specular_microscopy|OS|avg_area_um2 | extra | 345 | null | region (inferred) |
| creso-aversa|specular_microscopy|OS|ct_um | cross_check | 534 | null | region (inferred) |
| creso-aversa|specular_microscopy|OS|cv_pct | informative | 31 | null | region (inferred) |
| creso-aversa|specular_microscopy|OS|hex_pct | informative | 66 | null | region (inferred) |
| creso-aversa|specular_microscopy|OS|max_area_um2 | extra | 750 | null | region (inferred) |
| creso-aversa|specular_microscopy|OS|min_area_um2 | extra | 73 | null | region (inferred) |
| creso-aversa|specular_microscopy|OS|num_cells | extra | 237 | null | region (inferred) |
| creso-aversa|specular_microscopy|OS|sd_um2 | extra | 109 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|abcd_staging | screening | "A0 B0 C0 D" | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|acd_ext_mm | cross_check | 3.64 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|acd_int_mm | decisive | 3.1 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|art_max_um | screening | 467 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|astig_ant_d | informative | 4.6 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|axis_flat_deg | informative | 6 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|bad_d | screening | 1 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|cki | screening | 1 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|coma_z31_5mm_axis_deg | informative | 96 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|coma_z31_5mm_um | informative | 0.165 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|coma_z31_6mm_um | informative | 0.2 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|iha | screening | 6.7 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|ihd | screening | 0.009 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|iva | screening | 0.11 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|ki | screening | 1 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|kmax_ant_d | screening | 45.5 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|pachy_pupil_center_um | cross_check | 544 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|pachy_thinnest_um | decisive | 542 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|pachy_vertex_um | cross_check | 546 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|pns | informative | 0 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|qi | informative | "OK" | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OD|z40_6mm_um | informative | 0.233 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|abcd_staging | screening | "A0 B0 C0 D" | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|acd_ext_mm | cross_check | 3.65 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|acd_int_mm | decisive | 3.1 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|art_max_um | screening | 470 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|axis_flat_deg | informative | 173.4 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|bad_d | screening | 0.99 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|cki | screening | 1.01 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|coma_z31_5mm_axis_deg | informative | 82 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|coma_z31_5mm_um | informative | 0.163 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|coma_z31_6mm_um | informative | 0.168 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|iha | screening | 7.2 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|ihd | screening | 0.011 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|iva | screening | 0.12 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|ki | screening | 1 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|kmax_ant_d | screening | 46.3 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|pachy_pupil_center_um | cross_check | 549 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|pachy_thinnest_um | decisive | 544 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|pachy_vertex_um | cross_check | 549 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|pns | informative | 0 | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|qi | informative | "OK" | null | region (inferred) |
| daniela-nogueira|pentacam_corneal_tomography|OS|z40_6mm_um | informative | 0.19 | null | region (inferred) |
| daniela-nogueira|refractometry|OD|axis_deg | decisive | 15 | null | mapping (inferred) |
| daniela-nogueira|refractometry|OD|cylinder_d | decisive | -6.75 | null | mapping (inferred) |
| daniela-nogueira|refractometry|OD|pd_mm | extra | 66 | null | mapping (inferred) |
| daniela-nogueira|refractometry|OD|refraction_type | informative | "autorrefração" | null | mapping (inferred) |
| daniela-nogueira|refractometry|OD|sphere_d | decisive | -3.5 | null | mapping (inferred) |
| daniela-nogueira|refractometry|OD|vertex_distance_mm | informative | 12 | null | mapping (inferred) |
| daniela-nogueira|refractometry|OS|axis_deg | decisive | 172 | null | mapping (inferred) |
| daniela-nogueira|refractometry|OS|cylinder_d | decisive | -7.25 | null | mapping (inferred) |
| daniela-nogueira|refractometry|OS|pd_mm | extra | 66 | null | mapping (inferred) |
| daniela-nogueira|refractometry|OS|refraction_type | informative | "autorrefração" | null | mapping (inferred) |
| daniela-nogueira|refractometry|OS|sphere_d | decisive | -8 | null | mapping (inferred) |
| daniela-nogueira|refractometry|OS|vertex_distance_mm | informative | 12 | null | mapping (inferred) |
| daniela-nogueira|specular_microscopy|OD|avg_area_um2 | extra | 302 | null | mapping (inferred) |
| daniela-nogueira|specular_microscopy|OD|cell_density_cells_mm2 | decisive | 3312 | null | mapping (inferred) |
| daniela-nogueira|specular_microscopy|OD|ct_um | cross_check | 545 | null | mapping (inferred) |
| daniela-nogueira|specular_microscopy|OD|cv_pct | informative | 30 | null | mapping (inferred) |
| daniela-nogueira|specular_microscopy|OD|hex_pct | informative | 60 | null | mapping (inferred) |
| daniela-nogueira|specular_microscopy|OD|max_area_um2 | extra | 805 | null | mapping (inferred) |
| daniela-nogueira|specular_microscopy|OD|min_area_um2 | extra | 131 | null | mapping (inferred) |
| daniela-nogueira|specular_microscopy|OD|num_cells | extra | 118 | null | mapping (inferred) |
| daniela-nogueira|specular_microscopy|OD|sd_um2 | extra | 86 | null | mapping (inferred) |
| daniela-nogueira|specular_microscopy|OS|avg_area_um2 | extra | 297 | null | region (inferred) |
| daniela-nogueira|specular_microscopy|OS|ct_um | cross_check | 539 | null | region (inferred) |
| daniela-nogueira|specular_microscopy|OS|cv_pct | informative | 26 | null | region (inferred) |
| daniela-nogueira|specular_microscopy|OS|hex_pct | informative | 65 | null | region (inferred) |
| daniela-nogueira|specular_microscopy|OS|max_area_um2 | extra | 1168 | null | region (inferred) |
| daniela-nogueira|specular_microscopy|OS|min_area_um2 | extra | 132 | null | region (inferred) |
| daniela-nogueira|specular_microscopy|OS|num_cells | extra | 168 | null | region (inferred) |
| daniela-nogueira|specular_microscopy|OS|sd_um2 | extra | 70 | null | region (inferred) |
| francisco|iol_calculation|OD|ad_mm | cross_check | 2.56 | null | region (inferred) |
| francisco|iol_calculation|OD|cct_um | cross_check | 507 | null | region (inferred) |
| francisco|iol_calculation|OD|k1_axis_deg | extra | 1 | null | region (inferred) |
| francisco|iol_calculation|OD|k1_radius_mm | extra | 7.73 | null | region (inferred) |
| francisco|iol_calculation|OD|k2_axis_deg | extra | 91 | null | region (inferred) |
| francisco|iol_calculation|OD|k2_radius_mm | extra | 7.64 | null | region (inferred) |
| francisco|iol_calculation|OD|k_mean_radius_mm | extra | 7.68 | null | region (inferred) |
| francisco|iol_calculation|OD|lens_status | informative | "Phakic" | null | region (inferred) |
| francisco|iol_calculation|OS|ad_mm | cross_check | 2.51 | null | region (inferred) |
| francisco|iol_calculation|OS|cct_um | cross_check | 512 | null | region (inferred) |
| francisco|iol_calculation|OS|k1_axis_deg | extra | 1 | null | region (inferred) |
| francisco|iol_calculation|OS|k1_radius_mm | extra | 7.75 | null | region (inferred) |
| francisco|iol_calculation|OS|k2_axis_deg | extra | 91 | null | region (inferred) |
| francisco|iol_calculation|OS|k2_radius_mm | extra | 7.63 | null | region (inferred) |
| francisco|iol_calculation|OS|k_mean_radius_mm | extra | 7.69 | null | region (inferred) |
| francisco|iol_calculation|OS|lens_status | informative | "Phakic" | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|abcd_staging | screening | "A0 B0 C0 D" | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|acd_ext_mm | cross_check | 3.11 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|acd_int_mm | decisive | 2.6 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|art_max_um | screening | 388 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|astig_ant_d | informative | 0.5 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|axis_flat_deg | informative | 174.6 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|bad_d | screening | 1.15 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|cki | screening | 1 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|coma_z31_5mm_axis_deg | informative | 199 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|coma_z31_5mm_um | informative | 0.081 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|coma_z31_6mm_um | informative | 0.108 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|iha | screening | 2.9 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|ihd | screening | 0.006 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|iva | screening | 0.06 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|ki | screening | 1.02 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|kmax_ant_d | screening | 44.2 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|pachy_pupil_center_um | cross_check | 514 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|pachy_thinnest_um | decisive | 508 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|pachy_vertex_um | cross_check | 515 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|pns | informative | 1 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|qi | informative | "OK" | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OD|z40_6mm_um | informative | 0.328 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|abcd_staging | screening | "A0 B0 C0 D" | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|acd_ext_mm | cross_check | 3.07 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|acd_int_mm | decisive | 2.54 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|art_max_um | screening | 518 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|axis_flat_deg | informative | 1 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|bad_d | screening | 0.73 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|cki | screening | 1 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|coma_z31_5mm_axis_deg | informative | 316 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|coma_z31_5mm_um | informative | 0.049 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|coma_z31_6mm_um | informative | 0.1 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|iha | screening | 1.7 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|ihd | screening | 0.006 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|iva | screening | 0.1 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|ki | screening | 1.02 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|kmax_ant_d | screening | 44.5 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|pachy_pupil_center_um | cross_check | 528 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|pachy_thinnest_um | decisive | 527 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|pachy_vertex_um | cross_check | 529 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|pns | informative | 0 | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|qi | informative | "OK" | null | region (inferred) |
| francisco|pentacam_corneal_tomography|OS|z40_6mm_um | informative | 0.325 | null | region (inferred) |
| francisco|specular_microscopy|OD|avg_area_um2 | extra | 346 | null | region (inferred) |
| francisco|specular_microscopy|OD|ct_um | cross_check | 512 | null | region (inferred) |
| francisco|specular_microscopy|OD|cv_pct | informative | 27 | null | region (inferred) |
| francisco|specular_microscopy|OD|hex_pct | informative | 67 | null | region (inferred) |
| francisco|specular_microscopy|OD|max_area_um2 | extra | 791 | null | region (inferred) |
| francisco|specular_microscopy|OD|min_area_um2 | extra | 135 | null | region (inferred) |
| francisco|specular_microscopy|OD|num_cells | extra | 221 | null | region (inferred) |
| francisco|specular_microscopy|OD|sd_um2 | extra | 93 | null | region (inferred) |
| francisco|specular_microscopy|OS|avg_area_um2 | extra | 335 | null | region (inferred) |
| francisco|specular_microscopy|OS|ct_um | cross_check | 525 | null | region (inferred) |
| francisco|specular_microscopy|OS|cv_pct | informative | 32 | null | region (inferred) |
| francisco|specular_microscopy|OS|hex_pct | informative | 62 | null | region (inferred) |
| francisco|specular_microscopy|OS|max_area_um2 | extra | 1011 | null | region (inferred) |
| francisco|specular_microscopy|OS|min_area_um2 | extra | 135 | null | region (inferred) |
| francisco|specular_microscopy|OS|num_cells | extra | 205 | null | region (inferred) |
| francisco|specular_microscopy|OS|sd_um2 | extra | 107 | null | region (inferred) |
| maria-gorete|iol_calculation|OD|ad_mm | cross_check | 2.62 | null | region (inferred) |
| maria-gorete|iol_calculation|OD|cct_um | cross_check | 537 | null | region (inferred) |
| maria-gorete|iol_calculation|OD|k1_axis_deg | extra | 110 | null | region (inferred) |
| maria-gorete|iol_calculation|OD|k1_radius_mm | extra | 7.88 | null | region (inferred) |
| maria-gorete|iol_calculation|OD|k2_axis_deg | extra | 20 | null | region (inferred) |
| maria-gorete|iol_calculation|OD|k2_radius_mm | extra | 7.82 | null | region (inferred) |
| maria-gorete|iol_calculation|OD|k_mean_radius_mm | extra | 7.85 | null | region (inferred) |
| maria-gorete|iol_calculation|OD|lens_status | informative | "Phakic" | null | region (inferred) |
| maria-gorete|iol_calculation|OS|ad_mm | cross_check | 2.68 | null | region (inferred) |
| maria-gorete|iol_calculation|OS|cct_um | cross_check | 544 | null | region (inferred) |
| maria-gorete|iol_calculation|OS|k1_axis_deg | extra | 28 | null | region (inferred) |
| maria-gorete|iol_calculation|OS|k1_radius_mm | extra | 7.89 | null | region (inferred) |
| maria-gorete|iol_calculation|OS|k2_axis_deg | extra | 118 | null | region (inferred) |
| maria-gorete|iol_calculation|OS|k2_radius_mm | extra | 7.76 | null | region (inferred) |
| maria-gorete|iol_calculation|OS|k_mean_radius_mm | extra | 7.83 | null | region (inferred) |
| maria-gorete|iol_calculation|OS|lens_status | informative | "Phakic" | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|abcd_staging | screening | "A0 B0 C0 D" | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|acd_ext_mm | cross_check | 3.2 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|astig_ant_d | informative | 0.8 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|axis_flat_deg | informative | 108.9 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|coma_z31_6mm_um | informative | 0.331 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|isv | screening | 14 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|km_d | decisive | 42.8 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|pachy_pupil_center_um | cross_check | 537 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|pachy_vertex_um | cross_check | 538 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|pns | informative | 1 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OD|qi | informative | "OK" | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OS|abcd_staging | screening | "A0 B0 C0 D" | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OS|acd_ext_mm | cross_check | 3.25 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OS|axis_flat_deg | informative | 41.4 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OS|coma_z31_6mm_um | informative | 0.279 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OS|pachy_pupil_center_um | cross_check | 545 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OS|pachy_vertex_um | cross_check | 545 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OS|pns | informative | 1 | null | region (inferred) |
| maria-gorete|pentacam_corneal_tomography|OS|qi | informative | "OK" | null | region (inferred) |
| maria-gorete|refractometry|OD|pd_mm | extra | 66 | null | region (inferred) |
| maria-gorete|refractometry|OD|refraction_type | informative | "autorrefração" | null | region (inferred) |
| maria-gorete|refractometry|OD|vertex_distance_mm | informative | 12 | null | region (inferred) |
| maria-gorete|refractometry|OS|pd_mm | extra | 66 | null | region (inferred) |
| maria-gorete|refractometry|OS|refraction_type | informative | "autorrefração" | null | region (inferred) |
| maria-gorete|refractometry|OS|vertex_distance_mm | informative | 12 | null | region (inferred) |
| maria-gorete|specular_microscopy|OD|avg_area_um2 | extra | 463 | null | region (inferred) |
| maria-gorete|specular_microscopy|OD|ct_um | cross_check | 535 | null | region (inferred) |
| maria-gorete|specular_microscopy|OD|cv_pct | informative | 27 | null | region (inferred) |
| maria-gorete|specular_microscopy|OD|hex_pct | informative | 67 | null | region (inferred) |
| maria-gorete|specular_microscopy|OD|max_area_um2 | extra | 932 | null | region (inferred) |
| maria-gorete|specular_microscopy|OD|min_area_um2 | extra | 140 | null | region (inferred) |
| maria-gorete|specular_microscopy|OD|num_cells | extra | 169 | null | region (inferred) |
| maria-gorete|specular_microscopy|OD|sd_um2 | extra | 125 | null | region (inferred) |
| maria-gorete|specular_microscopy|OS|avg_area_um2 | extra | 464 | null | region (inferred) |
| maria-gorete|specular_microscopy|OS|ct_um | cross_check | 551 | null | region (inferred) |
| maria-gorete|specular_microscopy|OS|cv_pct | informative | 32 | null | region (inferred) |
| maria-gorete|specular_microscopy|OS|hex_pct | informative | 73 | null | region (inferred) |
| maria-gorete|specular_microscopy|OS|max_area_um2 | extra | 1122 | null | region (inferred) |
| maria-gorete|specular_microscopy|OS|min_area_um2 | extra | 139 | null | region (inferred) |
| maria-gorete|specular_microscopy|OS|num_cells | extra | 154 | null | region (inferred) |
| maria-gorete|specular_microscopy|OS|sd_um2 | extra | 150 | null | region (inferred) |
| rolmey|iol_calculation|OD|ad_mm | cross_check | 3.31 | null | region (inferred) |
| rolmey|iol_calculation|OD|cct_um | cross_check | 529 | null | region (inferred) |
| rolmey|iol_calculation|OD|k1_axis_deg | extra | 54 | null | region (inferred) |
| rolmey|iol_calculation|OD|k1_radius_mm | extra | 7.73 | null | region (inferred) |
| rolmey|iol_calculation|OD|k2_axis_deg | extra | 144 | null | region (inferred) |
| rolmey|iol_calculation|OD|k2_radius_mm | extra | 7.68 | null | region (inferred) |
| rolmey|iol_calculation|OD|k_mean_radius_mm | extra | 7.7 | null | region (inferred) |
| rolmey|iol_calculation|OD|lens_status | informative | "Phakic" | null | region (inferred) |
| rolmey|iol_calculation|OS|ad_mm | cross_check | 3.31 | null | region (inferred) |
| rolmey|iol_calculation|OS|cct_um | cross_check | 527 | null | region (inferred) |
| rolmey|iol_calculation|OS|k1_axis_deg | extra | 3 | null | region (inferred) |
| rolmey|iol_calculation|OS|k1_radius_mm | extra | 8.21 | null | region (inferred) |
| rolmey|iol_calculation|OS|k2_axis_deg | extra | 93 | null | region (inferred) |
| rolmey|iol_calculation|OS|k2_radius_mm | extra | 7.62 | null | region (inferred) |
| rolmey|iol_calculation|OS|k_mean_radius_mm | extra | 7.92 | null | region (inferred) |
| rolmey|iol_calculation|OS|lens_status | informative | "Phakic" | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|abcd_staging | screening | "A0 B0 C0 D" | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|acd_ext_mm | cross_check | 3.9 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|acd_int_mm | decisive | 3.36 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|art_max_um | screening | 416 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|axis_flat_deg | informative | 47.6 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|bad_d | screening | 0.65 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|cki | screening | 1 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|coma_z31_5mm_axis_deg | informative | 151 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|coma_z31_5mm_um | informative | 0.097 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|coma_z31_6mm_um | informative | 0.143 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|iha | screening | 4.2 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|ihd | screening | 0.006 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|iva | screening | 0.09 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|ki | screening | 1.01 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|kmax_ant_d | screening | 44.2 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|pachy_pupil_center_um | cross_check | 534 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|pachy_thinnest_um | decisive | 529 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|pachy_vertex_um | cross_check | 534 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|pns | informative | 1 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|qi | informative | "OK" | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OD|z40_6mm_um | informative | 0.287 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|abcd_staging | screening | "A0 B0 C0 D" | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|acd_ext_mm | cross_check | 3.9 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|acd_int_mm | decisive | 3.37 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|art_max_um | screening | 366 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|astig_ant_d | informative | 2 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|axis_flat_deg | informative | 177.9 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|bad_d | screening | 2.27 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|cki | screening | 0.98 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|coma_z31_5mm_axis_deg | informative | 180 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|coma_z31_5mm_um | informative | 0.299 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|coma_z31_6mm_um | informative | 0.568 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|iha | screening | 12.8 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|ihd | screening | 0.017 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|iva | screening | 0.18 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|k2_d | decisive | 44.1 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|ki | screening | 1.01 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|kmax_ant_d | screening | 45.5 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|pachy_pupil_center_um | cross_check | 537 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|pachy_thinnest_um | decisive | 526 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|pachy_vertex_um | cross_check | 538 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|pns | informative | 1 | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|qi | informative | "OK" | null | region (inferred) |
| rolmey|pentacam_corneal_tomography|OS|z40_6mm_um | informative | 0.602 | null | region (inferred) |
| rolmey|specular_microscopy|OD|avg_area_um2 | extra | 394 | null | region (inferred) |
| rolmey|specular_microscopy|OD|ct_um | cross_check | 532 | null | region (inferred) |
| rolmey|specular_microscopy|OD|cv_pct | informative | 34 | null | region (inferred) |
| rolmey|specular_microscopy|OD|hex_pct | informative | 64 | null | region (inferred) |
| rolmey|specular_microscopy|OD|max_area_um2 | extra | 933 | null | region (inferred) |
| rolmey|specular_microscopy|OD|min_area_um2 | extra | 137 | null | region (inferred) |
| rolmey|specular_microscopy|OD|num_cells | extra | 234 | null | region (inferred) |
| rolmey|specular_microscopy|OD|sd_um2 | extra | 133 | null | region (inferred) |
| rolmey|specular_microscopy|OS|avg_area_um2 | extra | 432 | null | region (inferred) |
| rolmey|specular_microscopy|OS|ct_um | cross_check | 542 | null | region (inferred) |
| rolmey|specular_microscopy|OS|cv_pct | informative | 34 | null | region (inferred) |
| rolmey|specular_microscopy|OS|hex_pct | informative | 65 | null | region (inferred) |
| rolmey|specular_microscopy|OS|max_area_um2 | extra | 1034 | null | region (inferred) |
| rolmey|specular_microscopy|OS|min_area_um2 | extra | 156 | null | region (inferred) |
| rolmey|specular_microscopy|OS|num_cells | extra | 206 | null | region (inferred) |
| rolmey|specular_microscopy|OS|sd_um2 | extra | 146 | null | region (inferred) |

## conflicting

| field id | role | expected | got | stage |
|---|---|---|---|---|
none

## unexpected_value

| field id | role | expected | got | stage |
|---|---|---|---|---|
none

## Persistence, identity and idempotency checks

| patient | group | check | result | detail |
|---|---|---|---|---|
| creso-aversa | identity | name | FAIL | mismatch: same words, different order |
| creso-aversa | identity | birth_date | pass | match |
| creso-aversa | identity | age_never_zero_or_from_missing_birthdate | pass |  |
| creso-aversa | persistence | confirm_http_200 | pass | HTTP 200 action=created |
| creso-aversa | persistence | exactly_one_new_patient_row | pass | 1 new patient rows |
| creso-aversa | persistence | patient_row_name | FAIL | mismatch: same words, different order |
| creso-aversa | persistence | patient_row_birth_date | pass | match |
| creso-aversa | persistence | exam_row_count | FAIL | 0 rows, expected 6 |
| creso-aversa | persistence | exam_rows_match_expected | FAIL | missing row iol_calculation/OD from creso-aversa/iol_calculation.AO; missing row iol_calculation/OS from creso-aversa/iol_calculation.AO; missing row specular_microscopy/OD from creso-aversa/specular_microscopy.AO; missing row specular_microscopy/OS from creso-aversa/specular_microscopy.AO; missing row pentacam_corneal_tomography/OD from creso-aversa/pentacam_corneal_tomography.OD; missing row pentacam_corneal_tomography/OS from creso-aversa/pentacam_corneal_tomography.OS |
| creso-aversa | persistence | exam_patient_id_matches_patient | FAIL | 0 rows with wrong/null patient_id |
| creso-aversa | persistence | case_id_consistent | FAIL | 0 distinct/empty case ids among exam rows |
| creso-aversa | persistence | exam_objects_exist_with_size | FAIL | 0 rows whose object is missing or has a different size |
| creso-aversa | persistence | marker_exists | FAIL | no case id to look up |
| creso-aversa | idempotency | confirm_retry | pass | HTTP 200; changes: none |
| creso-aversa | idempotency | reingest_no_duplicates | pass | HTTP 200 action=updated; changes: object size changed: refratia/cases/case-20261113T005319Z-8c0f4ed4/paciente_compilado.json 5949 -> 6274 |
| daniela-nogueira | identity | name | FAIL | mismatch |
| daniela-nogueira | identity | birth_date | FAIL | mismatch |
| daniela-nogueira | identity | age_never_zero_or_from_missing_birthdate | pass |  |
| daniela-nogueira | persistence | confirm_http_200 | pass | HTTP 200 action=created |
| daniela-nogueira | persistence | exactly_one_new_patient_row | pass | 1 new patient rows |
| daniela-nogueira | persistence | patient_row_name | FAIL | mismatch |
| daniela-nogueira | persistence | patient_row_birth_date | pass | match |
| daniela-nogueira | persistence | exam_row_count | FAIL | 0 rows, expected 8 |
| daniela-nogueira | persistence | exam_rows_match_expected | FAIL | 8 changes, e.g. missing row corneal_topography_other/OD from daniela-nogueira/corneal_topography_other.AO; missing row corneal_topography_other/OS from daniela-nogueira/corneal_topography_other.AO; missing row refractometry/OD from daniela-nogueira/refractometry.AO; missing row refractometry/OS from daniela-nogueira/refractometry.AO; missing row specular_microscopy/OD from daniela-nogueira/specular_microscopy.AO; missing row specular_microscopy/OS from daniela-nogueira/specular_microscopy.AO |
| daniela-nogueira | persistence | exam_patient_id_matches_patient | FAIL | 0 rows with wrong/null patient_id |
| daniela-nogueira | persistence | case_id_consistent | FAIL | 0 distinct/empty case ids among exam rows |
| daniela-nogueira | persistence | exam_objects_exist_with_size | FAIL | 0 rows whose object is missing or has a different size |
| daniela-nogueira | persistence | marker_exists | FAIL | no case id to look up |
| daniela-nogueira | idempotency | confirm_retry | pass | HTTP 200; changes: none |
| daniela-nogueira | idempotency | reingest_no_duplicates | pass | HTTP 200 action=updated; changes: object size changed: refratia/cases/case-20261113T005526Z-6a8b71d5/paciente_compilado.json 6541 -> 6700 |
| francisco | identity | name | pass | match |
| francisco | identity | birth_date | pass | unverified (needs_human in ground truth); value not scored |
| francisco | identity | age_never_zero_or_from_missing_birthdate | pass |  |
| francisco | identity | warning_data_nascimento_invertida | FAIL | required by ground truth (C2) |
| francisco | persistence | confirm_http_200 | pass | HTTP 200 action=created |
| francisco | persistence | exactly_one_new_patient_row | pass | 1 new patient rows |
| francisco | persistence | patient_row_name | pass | match |
| francisco | persistence | exam_row_count | FAIL | 0 rows, expected 10 |
| francisco | persistence | exam_rows_match_expected | FAIL | 10 changes, e.g. missing row anterior_segment_photo/OD from francisco/anterior_segment_photo.OD; missing row anterior_segment_photo/OS from francisco/anterior_segment_photo.OS; missing row iol_calculation/OD from francisco/iol_calculation.AO; missing row iol_calculation/OS from francisco/iol_calculation.AO; missing row specular_microscopy/OD from francisco/specular_microscopy.AO; missing row specular_microscopy/OS from francisco/specular_microscopy.AO |
| francisco | persistence | exam_patient_id_matches_patient | FAIL | 0 rows with wrong/null patient_id |
| francisco | persistence | case_id_consistent | FAIL | 0 distinct/empty case ids among exam rows |
| francisco | persistence | exam_objects_exist_with_size | FAIL | 0 rows whose object is missing or has a different size |
| francisco | persistence | marker_exists | FAIL | no case id to look up |
| francisco | idempotency | confirm_retry | pass | HTTP 200; changes: none |
| francisco | idempotency | reingest_no_duplicates | pass | HTTP 200 action=updated; changes: object size changed: refratia/cases/case-20261113T005930Z-f838d32c/paciente_compilado.json 10170 -> 10448 |
| maria-gorete | identity | name | pass | match |
| maria-gorete | identity | birth_date | pass | match |
| maria-gorete | identity | age_never_zero_or_from_missing_birthdate | pass |  |
| maria-gorete | persistence | confirm_http_200 | pass | HTTP 200 action=created |
| maria-gorete | persistence | exactly_one_new_patient_row | pass | 1 new patient rows |
| maria-gorete | persistence | patient_row_name | pass | match |
| maria-gorete | persistence | patient_row_birth_date | pass | match |
| maria-gorete | persistence | exam_row_count | FAIL | 0 rows, expected 8 |
| maria-gorete | persistence | exam_rows_match_expected | FAIL | 8 changes, e.g. missing row refractometry/OD from maria-gorete/refractometry.AO; missing row refractometry/OS from maria-gorete/refractometry.AO; missing row iol_calculation/OD from maria-gorete/iol_calculation.AO; missing row iol_calculation/OS from maria-gorete/iol_calculation.AO; missing row specular_microscopy/OD from maria-gorete/specular_microscopy.AO; missing row specular_microscopy/OS from maria-gorete/specular_microscopy.AO |
| maria-gorete | persistence | exam_patient_id_matches_patient | FAIL | 0 rows with wrong/null patient_id |
| maria-gorete | persistence | case_id_consistent | FAIL | 0 distinct/empty case ids among exam rows |
| maria-gorete | persistence | exam_objects_exist_with_size | FAIL | 0 rows whose object is missing or has a different size |
| maria-gorete | persistence | marker_exists | FAIL | no case id to look up |
| maria-gorete | idempotency | confirm_retry | pass | HTTP 200; changes: none |
| maria-gorete | idempotency | reingest_no_duplicates | pass | HTTP 200 action=updated; changes: object size changed: refratia/cases/case-20261113T010334Z-916fd710/paciente_compilado.json 7021 -> 7602 |
| rolmey | identity | name | pass | match |
| rolmey | identity | birth_date | FAIL | mismatch |
| rolmey | identity | age_never_zero_or_from_missing_birthdate | pass |  |
| rolmey | persistence | confirm_http_200 | FAIL | HTTP 422 action= |
| rolmey | persistence | exactly_one_new_patient_row | FAIL | 0 new patient rows |
| rolmey | persistence | exam_row_count | FAIL | 0 rows, expected 8 |
| rolmey | persistence | exam_rows_match_expected | FAIL | 8 changes, e.g. missing row iol_calculation/OD from rolmey/iol_calculation.AO; missing row iol_calculation/OS from rolmey/iol_calculation.AO; missing row specular_microscopy/OD from rolmey/specular_microscopy.AO; missing row specular_microscopy/OS from rolmey/specular_microscopy.AO; missing row pentacam_corneal_tomography/OD from rolmey/pentacam_corneal_tomography.OD; missing row pentacam_corneal_tomography/OS from rolmey/pentacam_corneal_tomography.OS |
| rolmey | persistence | exam_patient_id_matches_patient | FAIL | 0 rows with wrong/null patient_id |
| rolmey | persistence | case_id_consistent | FAIL | 0 distinct/empty case ids among exam rows |
| rolmey | persistence | exam_objects_exist_with_size | FAIL | 0 rows whose object is missing or has a different size |
| rolmey | persistence | marker_exists | FAIL | no case id to look up |
| rolmey | idempotency | confirm_retry | FAIL | HTTP 422; changes: none |
| rolmey | idempotency | reingest_no_duplicates | FAIL | HTTP 422 action=; changes: 7 changes, e.g. object added: refratia/cases/case-20261113T010831Z-087a2bca/<canonical-name AO pdf>; object added: refratia/cases/case-20261113T010831Z-087a2bca/MICROSCOPIA_<canonical-name AO jpeg>; object added: refratia/cases/case-20261113T010831Z-087a2bca/<canonical-name OD pdf>; object added: refratia/cases/case-20261113T010831Z-087a2bca/<canonical-name OS pdf>; object added: refratia/cases/case-20261113T010831Z-087a2bca/<canonical-name OD jpeg>; object added: refratia/cases/case-20261113T010831Z-087a2bca/<canonical-name OS jpeg> |

## Classification (informational)

| patient | file | source | recognized | expected | ok |
|---|---|---|---|---|---|
| creso-aversa | creso-aversa/iol_calculation.AO | recognized | iol_calculation/AO | iol_calculation/AO | true |
| creso-aversa | creso-aversa/specular_microscopy.AO | recognized | specular_microscopy/AO | specular_microscopy/AO | true |
| creso-aversa | creso-aversa/pentacam_corneal_tomography.OD | recognized | pentacam_corneal_tomography/OD | pentacam_corneal_tomography/OD | true |
| creso-aversa | creso-aversa/pentacam_corneal_tomography.OS | recognized | pentacam_corneal_tomography/OS | pentacam_corneal_tomography/OS | true |
| daniela-nogueira | daniela-nogueira/corneal_topography_other.AO | human-assisted | / | corneal_topography_other/AO | true |
| daniela-nogueira | daniela-nogueira/refractometry.AO | recognized | refractometry/AO | refractometry/AO | true |
| daniela-nogueira | daniela-nogueira/specular_microscopy.AO | recognized | specular_microscopy/AO | specular_microscopy/AO | true |
| daniela-nogueira | daniela-nogueira/pentacam_corneal_tomography.OD | recognized | pentacam_corneal_tomography/OD | pentacam_corneal_tomography/OD | true |
| daniela-nogueira | daniela-nogueira/pentacam_corneal_tomography.OS | recognized | pentacam_corneal_tomography/OS | pentacam_corneal_tomography/OS | true |
| francisco | francisco/anterior_segment_photo.OD | human-assisted | / | anterior_segment_photo/OD | true |
| francisco | francisco/anterior_segment_photo.OS | human-assisted | / | anterior_segment_photo/OS | true |
| francisco | francisco/iol_calculation.AO | recognized | iol_calculation/AO | iol_calculation/AO | true |
| francisco | francisco/specular_microscopy.AO | recognized | specular_microscopy/AO | specular_microscopy/AO | true |
| francisco | francisco/pentacam_corneal_tomography.OD | recognized | pentacam_corneal_tomography/OD | pentacam_corneal_tomography/OD | true |
| francisco | francisco/pentacam_corneal_tomography.OS | recognized | pentacam_corneal_tomography/OS | pentacam_corneal_tomography/OS | true |
| francisco | francisco/fundus_retinography.OD | recognized | fundus_retinography/OD | fundus_retinography/OD | true |
| francisco | francisco/fundus_retinography.OS | recognized | fundus_retinography/OS | fundus_retinography/OS | true |
| maria-gorete | maria-gorete/refractometry.AO | recognized | refractometry/AO | refractometry/AO | true |
| maria-gorete | maria-gorete/iol_calculation.AO | recognized | iol_calculation/AO | iol_calculation/AO | true |
| maria-gorete | maria-gorete/specular_microscopy.AO | recognized | specular_microscopy/AO | specular_microscopy/AO | true |
| maria-gorete | maria-gorete/pentacam_corneal_tomography.OD | recognized | pentacam_corneal_tomography/OD | pentacam_corneal_tomography/OD | true |
| maria-gorete | maria-gorete/pentacam_corneal_tomography.OS | recognized | pentacam_corneal_tomography/OS | pentacam_corneal_tomography/OS | true |
| rolmey | rolmey/iol_calculation.AO | recognized | iol_calculation/AO | iol_calculation/AO | true |
| rolmey | rolmey/specular_microscopy.AO | recognized | specular_microscopy/AO | specular_microscopy/AO | true |
| rolmey | rolmey/pentacam_corneal_tomography.OD | recognized | pentacam_corneal_tomography/OD | pentacam_corneal_tomography/OD | true |
| rolmey | rolmey/pentacam_corneal_tomography.OS | recognized | pentacam_corneal_tomography/OS | pentacam_corneal_tomography/OS | true |
| rolmey | rolmey/fundus_retinography.OD | recognized | fundus_retinography/OD | fundus_retinography/OD | true |
| rolmey | rolmey/fundus_retinography.OS | recognized | fundus_retinography/OS | fundus_retinography/OS | true |

## Unverified (needs_human, never counted as pass)

- francisco: patient.birth_date
- maria-gorete: maria-gorete/refractometry.AO :: identity.birth_date

106 ground-truth fields have no app JSON path (not scored; see _field_spec.json).

## Regressions vs last_pass.json

none

## Failures that make the run exit nonzero

- required field creso-aversa|iol_calculation|OD|ad_mm: missing
- required field creso-aversa|iol_calculation|OD|cct_um: missing
- required field creso-aversa|iol_calculation|OS|ad_mm: missing
- required field creso-aversa|iol_calculation|OS|cct_um: missing
- required field creso-aversa|pentacam_corneal_tomography|OD|acd_ext_mm: missing
- required field creso-aversa|pentacam_corneal_tomography|OD|pachy_pupil_center_um: missing
- required field creso-aversa|pentacam_corneal_tomography|OD|pachy_vertex_um: missing
- required field creso-aversa|pentacam_corneal_tomography|OS|acd_ext_mm: missing
- required field creso-aversa|pentacam_corneal_tomography|OS|pachy_pupil_center_um: missing
- required field creso-aversa|pentacam_corneal_tomography|OS|pachy_vertex_um: missing
- required field creso-aversa|specular_microscopy|OD|ct_um: missing
- required field creso-aversa|specular_microscopy|OS|ct_um: missing
- creso-aversa identity/name: mismatch: same words, different order
- creso-aversa persistence/patient_row_name: mismatch: same words, different order
- creso-aversa persistence/exam_row_count: 0 rows, expected 6
- creso-aversa persistence/exam_rows_match_expected: missing row iol_calculation/OD from creso-aversa/iol_calculation.AO; missing row iol_calculation/OS from creso-aversa/iol_calculation.AO; missing row specular_microscopy/OD from creso-aversa/specular_microscopy.AO; missing row specular_microscopy/OS from creso-aversa/specular_microscopy.AO; missing row pentacam_corneal_tomography/OD from creso-aversa/pentacam_corneal_tomography.OD; missing row pentacam_corneal_tomography/OS from creso-aversa/pentacam_corneal_tomography.OS
- creso-aversa persistence/exam_patient_id_matches_patient: 0 rows with wrong/null patient_id
- creso-aversa persistence/case_id_consistent: 0 distinct/empty case ids among exam rows
- creso-aversa persistence/exam_objects_exist_with_size: 0 rows whose object is missing or has a different size
- creso-aversa persistence/marker_exists: no case id to look up
- required field daniela-nogueira|pentacam_corneal_tomography|OD|acd_ext_mm: missing
- required field daniela-nogueira|pentacam_corneal_tomography|OD|acd_int_mm: missing
- required field daniela-nogueira|pentacam_corneal_tomography|OD|pachy_pupil_center_um: missing
- required field daniela-nogueira|pentacam_corneal_tomography|OD|pachy_thinnest_um: missing
- required field daniela-nogueira|pentacam_corneal_tomography|OD|pachy_vertex_um: missing
- required field daniela-nogueira|pentacam_corneal_tomography|OS|acd_ext_mm: missing
- required field daniela-nogueira|pentacam_corneal_tomography|OS|acd_int_mm: missing
- required field daniela-nogueira|pentacam_corneal_tomography|OS|pachy_pupil_center_um: missing
- required field daniela-nogueira|pentacam_corneal_tomography|OS|pachy_thinnest_um: missing
- required field daniela-nogueira|pentacam_corneal_tomography|OS|pachy_vertex_um: missing
- required field daniela-nogueira|refractometry|OD|axis_deg: missing
- required field daniela-nogueira|refractometry|OD|cylinder_d: missing
- required field daniela-nogueira|refractometry|OD|sphere_d: missing
- required field daniela-nogueira|refractometry|OS|axis_deg: missing
- required field daniela-nogueira|refractometry|OS|cylinder_d: missing
- required field daniela-nogueira|refractometry|OS|sphere_d: missing
- required field daniela-nogueira|specular_microscopy|OD|cell_density_cells_mm2: missing
- required field daniela-nogueira|specular_microscopy|OD|ct_um: missing
- required field daniela-nogueira|specular_microscopy|OS|ct_um: missing
- daniela-nogueira identity/name: mismatch
- daniela-nogueira identity/birth_date: mismatch
- daniela-nogueira persistence/patient_row_name: mismatch
- daniela-nogueira persistence/exam_row_count: 0 rows, expected 8
- daniela-nogueira persistence/exam_rows_match_expected: 8 changes, e.g. missing row corneal_topography_other/OD from daniela-nogueira/corneal_topography_other.AO; missing row corneal_topography_other/OS from daniela-nogueira/corneal_topography_other.AO; missing row refractometry/OD from daniela-nogueira/refractometry.AO; missing row refractometry/OS from daniela-nogueira/refractometry.AO; missing row specular_microscopy/OD from daniela-nogueira/specular_microscopy.AO; missing row specular_microscopy/OS from daniela-nogueira/specular_microscopy.AO
- daniela-nogueira persistence/exam_patient_id_matches_patient: 0 rows with wrong/null patient_id
- daniela-nogueira persistence/case_id_consistent: 0 distinct/empty case ids among exam rows
- daniela-nogueira persistence/exam_objects_exist_with_size: 0 rows whose object is missing or has a different size
- daniela-nogueira persistence/marker_exists: no case id to look up
- required field francisco|iol_calculation|OD|ad_mm: missing
- required field francisco|iol_calculation|OD|cct_um: missing
- required field francisco|iol_calculation|OS|ad_mm: missing
- required field francisco|iol_calculation|OS|cct_um: missing
- required field francisco|pentacam_corneal_tomography|OD|acd_ext_mm: missing
- required field francisco|pentacam_corneal_tomography|OD|acd_int_mm: missing
- required field francisco|pentacam_corneal_tomography|OD|pachy_pupil_center_um: missing
- required field francisco|pentacam_corneal_tomography|OD|pachy_thinnest_um: missing
- required field francisco|pentacam_corneal_tomography|OD|pachy_vertex_um: missing
- required field francisco|pentacam_corneal_tomography|OS|acd_ext_mm: missing
- required field francisco|pentacam_corneal_tomography|OS|acd_int_mm: missing
- required field francisco|pentacam_corneal_tomography|OS|pachy_pupil_center_um: missing
- required field francisco|pentacam_corneal_tomography|OS|pachy_thinnest_um: missing
- required field francisco|pentacam_corneal_tomography|OS|pachy_vertex_um: missing
- required field francisco|specular_microscopy|OD|ct_um: missing
- required field francisco|specular_microscopy|OS|ct_um: missing
- francisco identity/warning_data_nascimento_invertida: required by ground truth (C2)
- francisco persistence/exam_row_count: 0 rows, expected 10
- francisco persistence/exam_rows_match_expected: 10 changes, e.g. missing row anterior_segment_photo/OD from francisco/anterior_segment_photo.OD; missing row anterior_segment_photo/OS from francisco/anterior_segment_photo.OS; missing row iol_calculation/OD from francisco/iol_calculation.AO; missing row iol_calculation/OS from francisco/iol_calculation.AO; missing row specular_microscopy/OD from francisco/specular_microscopy.AO; missing row specular_microscopy/OS from francisco/specular_microscopy.AO
- francisco persistence/exam_patient_id_matches_patient: 0 rows with wrong/null patient_id
- francisco persistence/case_id_consistent: 0 distinct/empty case ids among exam rows
- francisco persistence/exam_objects_exist_with_size: 0 rows whose object is missing or has a different size
- francisco persistence/marker_exists: no case id to look up
- required field maria-gorete|iol_calculation|OD|ad_mm: missing
- required field maria-gorete|iol_calculation|OD|cct_um: missing
- required field maria-gorete|iol_calculation|OS|ad_mm: missing
- required field maria-gorete|iol_calculation|OS|cct_um: missing
- required field maria-gorete|pentacam_corneal_tomography|OD|acd_ext_mm: missing
- required field maria-gorete|pentacam_corneal_tomography|OD|km_d: missing
- required field maria-gorete|pentacam_corneal_tomography|OD|pachy_pupil_center_um: missing
- required field maria-gorete|pentacam_corneal_tomography|OD|pachy_vertex_um: missing
- required field maria-gorete|pentacam_corneal_tomography|OS|acd_ext_mm: missing
- required field maria-gorete|pentacam_corneal_tomography|OS|pachy_pupil_center_um: missing
- required field maria-gorete|pentacam_corneal_tomography|OS|pachy_vertex_um: missing
- required field maria-gorete|refractometry|OD|axis_deg: incorrect
- required field maria-gorete|refractometry|OD|cylinder_d: incorrect
- required field maria-gorete|specular_microscopy|OD|ct_um: missing
- required field maria-gorete|specular_microscopy|OS|ct_um: missing
- maria-gorete persistence/exam_row_count: 0 rows, expected 8
- maria-gorete persistence/exam_rows_match_expected: 8 changes, e.g. missing row refractometry/OD from maria-gorete/refractometry.AO; missing row refractometry/OS from maria-gorete/refractometry.AO; missing row iol_calculation/OD from maria-gorete/iol_calculation.AO; missing row iol_calculation/OS from maria-gorete/iol_calculation.AO; missing row specular_microscopy/OD from maria-gorete/specular_microscopy.AO; missing row specular_microscopy/OS from maria-gorete/specular_microscopy.AO
- maria-gorete persistence/exam_patient_id_matches_patient: 0 rows with wrong/null patient_id
- maria-gorete persistence/case_id_consistent: 0 distinct/empty case ids among exam rows
- maria-gorete persistence/exam_objects_exist_with_size: 0 rows whose object is missing or has a different size
- maria-gorete persistence/marker_exists: no case id to look up
- required field rolmey|iol_calculation|OD|ad_mm: missing
- required field rolmey|iol_calculation|OD|cct_um: missing
- required field rolmey|iol_calculation|OS|ad_mm: missing
- required field rolmey|iol_calculation|OS|cct_um: missing
- required field rolmey|pentacam_corneal_tomography|OD|acd_ext_mm: missing
- required field rolmey|pentacam_corneal_tomography|OD|acd_int_mm: missing
- required field rolmey|pentacam_corneal_tomography|OD|pachy_pupil_center_um: missing
- required field rolmey|pentacam_corneal_tomography|OD|pachy_thinnest_um: missing
- required field rolmey|pentacam_corneal_tomography|OD|pachy_vertex_um: missing
- required field rolmey|pentacam_corneal_tomography|OS|acd_ext_mm: missing
- required field rolmey|pentacam_corneal_tomography|OS|acd_int_mm: missing
- required field rolmey|pentacam_corneal_tomography|OS|k2_d: missing
- required field rolmey|pentacam_corneal_tomography|OS|pachy_pupil_center_um: missing
- required field rolmey|pentacam_corneal_tomography|OS|pachy_thinnest_um: missing
- required field rolmey|pentacam_corneal_tomography|OS|pachy_vertex_um: missing
- required field rolmey|specular_microscopy|OD|ct_um: missing
- required field rolmey|specular_microscopy|OS|ct_um: missing
- rolmey identity/birth_date: mismatch
- rolmey persistence/confirm_http_200: HTTP 422 action=
- rolmey persistence/exactly_one_new_patient_row: 0 new patient rows
- rolmey persistence/exam_row_count: 0 rows, expected 8
- rolmey persistence/exam_rows_match_expected: 8 changes, e.g. missing row iol_calculation/OD from rolmey/iol_calculation.AO; missing row iol_calculation/OS from rolmey/iol_calculation.AO; missing row specular_microscopy/OD from rolmey/specular_microscopy.AO; missing row specular_microscopy/OS from rolmey/specular_microscopy.AO; missing row pentacam_corneal_tomography/OD from rolmey/pentacam_corneal_tomography.OD; missing row pentacam_corneal_tomography/OS from rolmey/pentacam_corneal_tomography.OS
- rolmey persistence/exam_patient_id_matches_patient: 0 rows with wrong/null patient_id
- rolmey persistence/case_id_consistent: 0 distinct/empty case ids among exam rows
- rolmey persistence/exam_objects_exist_with_size: 0 rows whose object is missing or has a different size
- rolmey persistence/marker_exists: no case id to look up
- rolmey idempotency/confirm_retry: HTTP 422; changes: none
- rolmey idempotency/reingest_no_duplicates: HTTP 422 action=; changes: 7 changes, e.g. object added: refratia/cases/case-20261113T010831Z-087a2bca/<canonical-name AO pdf>; object added: refratia/cases/case-20261113T010831Z-087a2bca/MICROSCOPIA_<canonical-name AO jpeg>; object added: refratia/cases/case-20261113T010831Z-087a2bca/<canonical-name OD pdf>; object added: refratia/cases/case-20261113T010831Z-087a2bca/<canonical-name OS pdf>; object added: refratia/cases/case-20261113T010831Z-087a2bca/<canonical-name OD jpeg>; object added: refratia/cases/case-20261113T010831Z-087a2bca/<canonical-name OS jpeg>

## Accuracy per field

| key | accuracy |
|---|---|
| fundus_retinography / findings | 4/4 (100.0%) |
| fundus_retinography / mode | 4/4 (100.0%) |
| iol_calculation / acd_mm | 8/8 (100.0%) |
| iol_calculation / ad_mm | 0/8 (0.0%) |
| iol_calculation / al_mm | 8/8 (100.0%) |
| iol_calculation / astig_axis_deg | 8/8 (100.0%) |
| iol_calculation / astig_d | 8/8 (100.0%) |
| iol_calculation / cct_um | 0/8 (0.0%) |
| iol_calculation / k1_axis_deg | 0/8 (0.0%) |
| iol_calculation / k1_d | 8/8 (100.0%) |
| iol_calculation / k1_radius_mm | 0/8 (0.0%) |
| iol_calculation / k2_axis_deg | 0/8 (0.0%) |
| iol_calculation / k2_d | 8/8 (100.0%) |
| iol_calculation / k2_radius_mm | 0/8 (0.0%) |
| iol_calculation / k_mean_d | 8/8 (100.0%) |
| iol_calculation / k_mean_radius_mm | 0/8 (0.0%) |
| iol_calculation / lens_status | 0/8 (0.0%) |
| iol_calculation / lt_mm | 8/8 (100.0%) |
| iol_calculation / target_refraction_d | 8/8 (100.0%) |
| iol_calculation / wtw_mm | 8/8 (100.0%) |
| pentacam_corneal_tomography / abcd_staging | 0/10 (0.0%) |
| pentacam_corneal_tomography / acd_ext_mm | 0/10 (0.0%) |
| pentacam_corneal_tomography / acd_int_mm | 4/10 (40.0%) |
| pentacam_corneal_tomography / art_max_um | 4/10 (40.0%) |
| pentacam_corneal_tomography / astig_ant_d | 6/10 (60.0%) |
| pentacam_corneal_tomography / axis_flat_deg | 0/10 (0.0%) |
| pentacam_corneal_tomography / bad_d | 4/10 (40.0%) |
| pentacam_corneal_tomography / cki | 4/10 (40.0%) |
| pentacam_corneal_tomography / coma_z31_5mm_axis_deg | 0/10 (0.0%) |
| pentacam_corneal_tomography / coma_z31_5mm_um | 4/10 (40.0%) |
| pentacam_corneal_tomography / coma_z31_6mm_um | 0/10 (0.0%) |
| pentacam_corneal_tomography / iha | 4/10 (40.0%) |
| pentacam_corneal_tomography / ihd | 4/10 (40.0%) |
| pentacam_corneal_tomography / isv | 1/10 (10.0%) |
| pentacam_corneal_tomography / iva | 4/10 (40.0%) |
| pentacam_corneal_tomography / k1_d | 10/10 (100.0%) |
| pentacam_corneal_tomography / k2_d | 9/10 (90.0%) |
| pentacam_corneal_tomography / ki | 3/10 (30.0%) |
| pentacam_corneal_tomography / km_d | 9/10 (90.0%) |
| pentacam_corneal_tomography / kmax_ant_d | 4/10 (40.0%) |
| pentacam_corneal_tomography / pachy_pupil_center_um | 0/10 (0.0%) |
| pentacam_corneal_tomography / pachy_thinnest_um | 4/10 (40.0%) |
| pentacam_corneal_tomography / pachy_vertex_um | 0/10 (0.0%) |
| pentacam_corneal_tomography / pns | 1/10 (10.0%) |
| pentacam_corneal_tomography / qi | 0/10 (0.0%) |
| pentacam_corneal_tomography / tkc | 6/10 (60.0%) |
| pentacam_corneal_tomography / z40_6mm_um | 4/10 (40.0%) |
| refractometry / axis_deg | 1/4 (25.0%) |
| refractometry / cylinder_d | 1/4 (25.0%) |
| refractometry / pd_mm | 0/4 (0.0%) |
| refractometry / refraction_type | 0/4 (0.0%) |
| refractometry / sphere_d | 2/4 (50.0%) |
| refractometry / vertex_distance_mm | 0/4 (0.0%) |
| specular_microscopy / avg_area_um2 | 0/10 (0.0%) |
| specular_microscopy / cell_density_cells_mm2 | 9/10 (90.0%) |
| specular_microscopy / ct_um | 0/10 (0.0%) |
| specular_microscopy / cv_pct | 0/10 (0.0%) |
| specular_microscopy / hex_pct | 0/10 (0.0%) |
| specular_microscopy / max_area_um2 | 0/10 (0.0%) |
| specular_microscopy / min_area_um2 | 0/10 (0.0%) |
| specular_microscopy / num_cells | 0/10 (0.0%) |
| specular_microscopy / sd_um2 | 0/10 (0.0%) |
