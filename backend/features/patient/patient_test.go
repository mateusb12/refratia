package patient

import (
	"testing"
)

func TestPatientIdentityNormalizesNameAndBirthDate(t *testing.T) {
	analysis := map[string]any{
		"patient": map[string]any{
			"full_name":  "  Paciente   Teste-Silva ",
			"birth_date": "1980-03-04",
		},
	}

	identity, ok := Identity(analysis)
	if !ok {
		t.Fatal("expected patient to be identifiable")
	}

	if identity != "paciente silva teste|1980-03-04" {
		t.Fatalf("unexpected identity: %q", identity)
	}
}

func TestPatientIdentityRequiresBirthDate(t *testing.T) {
	analysis := map[string]any{
		"patient": map[string]any{
			"full_name": "Paciente Teste Silva",
		},
	}

	if _, ok := Identity(analysis); ok {
		t.Fatal("patient without birth date must not be automatically merged")
	}
}

func TestMergePatientCaseAddsPreviouslyMissingExam(t *testing.T) {
	existing := map[string]any{
		"patient": map[string]any{
			"full_name":  "Paciente Teste Silva",
			"birth_date": "1980-03-04",
		},
		"source_files": []any{},
		"exams": map[string]any{
			"iol_calculation": map[string]any{
				"source": []any{"bio.pdf"},
				"eyes": map[string]any{
					"OD": map[string]any{"axial_length_mm": 24.1},
				},
			},
		},
	}

	incoming := map[string]any{
		"patient": map[string]any{
			"full_name":  "Paciente Teste Silva",
			"birth_date": "1980-03-04",
		},
		"source_files": []any{},
		"exams": map[string]any{
			"specular_microscopy": map[string]any{
				"source": []any{"micro.jpeg"},
				"eyes": map[string]any{
					"AO": map[string]any{"cell_density_cells_per_mm2": 2450.0},
				},
			},
		},
	}

	merged := Merge(existing, incoming)
	exams := merged["exams"].(map[string]any)

	if _, ok := exams["iol_calculation"]; !ok {
		t.Fatal("existing exam must be preserved")
	}

	if _, ok := exams["specular_microscopy"]; !ok {
		t.Fatal("new exam must be added")
	}
}

func TestMergePatientCaseReplacesOnlyIncomingEye(t *testing.T) {
	existing := map[string]any{
		"source_files": []any{},
		"exams": map[string]any{
			"pentacam_corneal_tomography": map[string]any{
				"source": []any{"old-od.pdf", "old-os.pdf"},
				"eyes": map[string]any{
					"OD": map[string]any{
						"anterior_cornea": map[string]any{"kmax_d": 44.0},
					},
					"OS": map[string]any{
						"anterior_cornea": map[string]any{"kmax_d": 45.0},
					},
				},
			},
		},
	}

	incoming := map[string]any{
		"source_files": []any{},
		"exams": map[string]any{
			"pentacam_corneal_tomography": map[string]any{
				"source": []any{"new-od.pdf"},
				"eyes": map[string]any{
					"OD": map[string]any{
						"anterior_cornea": map[string]any{"kmax_d": 47.0},
					},
				},
			},
		},
	}

	merged := Merge(existing, incoming)

	exams := merged["exams"].(map[string]any)
	pentacam := exams["pentacam_corneal_tomography"].(map[string]any)
	eyes := pentacam["eyes"].(map[string]any)

	od := eyes["OD"].(map[string]any)
	odCornea := od["anterior_cornea"].(map[string]any)

	os := eyes["OS"].(map[string]any)
	osCornea := os["anterior_cornea"].(map[string]any)

	if odCornea["kmax_d"] != 47.0 {
		t.Fatalf("OD must be replaced by incoming exam, got %#v", odCornea["kmax_d"])
	}

	if osCornea["kmax_d"] != 45.0 {
		t.Fatalf("OS must be preserved, got %#v", osCornea["kmax_d"])
	}
}

func TestMergeSourceFilesDeduplicatesBySHA256(t *testing.T) {
	existing := []any{
		map[string]any{
			"path":   "cases/case-x/old.pdf",
			"sha256": "same-hash",
		},
	}

	incoming := []any{
		map[string]any{
			"path":   "cases/case-x/new-copy.pdf",
			"sha256": "same-hash",
		},
		map[string]any{
			"path":   "cases/case-x/another.pdf",
			"sha256": "different-hash",
		},
	}

	merged := mergeSourceFiles(existing, incoming)

	if len(merged) != 2 {
		t.Fatalf("expected 2 unique sources, got %d", len(merged))
	}
}

func TestMergeSourceFilesReplacesSameExamAndEye(t *testing.T) {
	existing := []any{
		map[string]any{
			"path":   "cases/case-x/old-od.pdf",
			"sha256": "old-od",
			"exam":   "pentacam_corneal_tomography",
			"eye":    "OD",
		},
		map[string]any{
			"path":   "cases/case-x/old-os.pdf",
			"sha256": "old-os",
			"exam":   "pentacam_corneal_tomography",
			"eye":    "OS",
		},
	}

	incoming := []any{
		map[string]any{
			"path":   "cases/case-x/new-od.pdf",
			"sha256": "new-od",
			"exam":   "pentacam_corneal_tomography",
			"eye":    "OD",
		},
	}

	merged := mergeSourceFiles(existing, incoming)

	if len(merged) != 2 {
		t.Fatalf("expected current OD + preserved OS, got %d sources", len(merged))
	}

	paths := map[string]bool{}
	for _, raw := range merged {
		source := raw.(map[string]any)
		paths[source["path"].(string)] = true
	}

	if paths["cases/case-x/old-od.pdf"] {
		t.Fatal("superseded OD source must not remain in current source_files")
	}

	if !paths["cases/case-x/new-od.pdf"] {
		t.Fatal("new OD source must become current")
	}

	if !paths["cases/case-x/old-os.pdf"] {
		t.Fatal("OS source must be preserved")
	}
}

func TestMergeSourceFilesKeepsExistingPathForExactReupload(t *testing.T) {
	existing := []any{
		map[string]any{
			"path":   "cases/case-x/original.pdf",
			"sha256": "same",
			"exam":   "pentacam_corneal_tomography",
			"eye":    "OD",
		},
	}

	incoming := []any{
		map[string]any{
			"path":   "cases/case-x/reuploaded-copy.pdf",
			"sha256": "same",
			"exam":   "pentacam_corneal_tomography",
			"eye":    "OD",
		},
	}

	merged := mergeSourceFiles(existing, incoming)

	if len(merged) != 1 {
		t.Fatalf("expected one source, got %d", len(merged))
	}

	source := merged[0].(map[string]any)
	if source["path"] != "cases/case-x/original.pdf" {
		t.Fatalf("expected original persisted source, got %#v", source["path"])
	}
}

func TestMergePatientCaseRebuildsPentacamSourceInEyeOrder(t *testing.T) {
	existing := map[string]any{
		"source_files": []any{
			map[string]any{
				"path":   "cases/case-x/old-od.pdf",
				"sha256": "old-od",
				"exam":   "pentacam_corneal_tomography",
				"eye":    "OD",
			},
			map[string]any{
				"path":   "cases/case-x/old-os.pdf",
				"sha256": "old-os",
				"exam":   "pentacam_corneal_tomography",
				"eye":    "OS",
			},
		},
		"exams": map[string]any{
			"pentacam_corneal_tomography": map[string]any{
				"source": []any{"old-od.pdf", "old-os.pdf"},
				"eyes": map[string]any{
					"OD": map[string]any{"value": "old OD"},
					"OS": map[string]any{"value": "old OS"},
				},
			},
		},
	}

	incoming := map[string]any{
		"source_files": []any{
			map[string]any{
				"path":   "cases/case-x/new-od.pdf",
				"sha256": "new-od",
				"exam":   "pentacam_corneal_tomography",
				"eye":    "OD",
			},
		},
		"exams": map[string]any{
			"pentacam_corneal_tomography": map[string]any{
				"source": []any{"new-od.pdf"},
				"eyes": map[string]any{
					"OD": map[string]any{"value": "new OD"},
				},
			},
		},
	}

	merged := Merge(existing, incoming)

	exams := merged["exams"].(map[string]any)
	pentacam := exams["pentacam_corneal_tomography"].(map[string]any)
	sources := pentacam["source"].([]any)

	if len(sources) != 2 {
		t.Fatalf("expected OD + OS sources, got %#v", sources)
	}

	if sources[0] != "cases/case-x/new-od.pdf" {
		t.Fatalf("source[0] must be current OD, got %#v", sources[0])
	}

	if sources[1] != "cases/case-x/old-os.pdf" {
		t.Fatalf("source[1] must preserve OS, got %#v", sources[1])
	}

	eyes := pentacam["eyes"].(map[string]any)

	if eyes["OD"].(map[string]any)["value"] != "new OD" {
		t.Fatal("OD payload must be updated")
	}

	if eyes["OS"].(map[string]any)["value"] != "old OS" {
		t.Fatal("OS payload must remain untouched")
	}
}

func TestPatientIdentityUsesDocumentDateOrderInsteadOfModelNormalizedDate(t *testing.T) {
	analysis := map[string]any{
		"patient": map[string]any{
			"full_name": "PACIENTE TESTE SILVA",
			"birth_date": map[string]any{
				"normalized": "1980-04-03",
			},
		},
		"verificacao_identidade": []any{
			map[string]any{
				"nascimento_lido": "03/04/1980",
				"timestamp_lido":  "08/31/2026 10:15:00",
			},
		},
	}

	identity, ok := Identity(analysis)
	if !ok {
		t.Fatal("expected patient identity to be deterministically resolved")
	}

	if identity != "paciente silva teste|1980-03-04" {
		t.Fatalf("unexpected identity: %q", identity)
	}
}

func TestPatientIdentityMatchesBothObservedPentacamShapes(t *testing.T) {
	od := map[string]any{
		"patient": map[string]any{
			"full_name": "PACIENTE TESTE SILVA",
			"birth_date": map[string]any{
				"normalized": "1980-04-03",
			},
		},
		"verificacao_identidade": []any{
			map[string]any{
				"nascimento_lido": "03/04/1980",
				"timestamp_lido":  "08/31/2026 10:15:00",
			},
		},
	}

	os := map[string]any{
		"patient": map[string]any{
			"full_name":             "PACIENTE TESTE SILVA",
			"birth_date_normalized": "1980-04-03",
		},
		"verificacao_identidade": []any{
			map[string]any{
				"nascimento_lido": "03/04/1980",
				"timestamp_lido":  "08/31/2026 10:16:00",
			},
		},
	}

	odIdentity, odOK := Identity(od)
	osIdentity, osOK := Identity(os)

	if !odOK || !osOK {
		t.Fatal("both Pentacam shapes should produce a patient identity")
	}

	if odIdentity != osIdentity {
		t.Fatalf(
			"OD and OS should resolve to same identity: %q != %q",
			odIdentity,
			osIdentity,
		)
	}

	if odIdentity != "paciente silva teste|1980-03-04" {
		t.Fatalf("unexpected canonical identity: %q", odIdentity)
	}
}

func TestPatientIdentityRejectsAmbiguousSlashDateWithoutContext(t *testing.T) {
	analysis := map[string]any{
		"patient": map[string]any{
			"full_name":             "PACIENTE TESTE SILVA",
			"birth_date_normalized": "1980-04-03",
		},
		"verificacao_identidade": []any{
			map[string]any{
				"nascimento_lido": "03/04/1980",
			},
		},
	}

	if _, ok := Identity(analysis); ok {
		t.Fatal("ambiguous raw birth date must not fall back to model normalization")
	}
}

func TestSourceExamArrayMatchesRealPentacamShape(t *testing.T) {
	source := map[string]any{
		"exam": []any{"pentacam_corneal_tomography"},
		"eye":  "OD",
	}

	if sourceExamName(source) != "pentacam_corneal_tomography" {
		t.Fatal("single-item exam array should resolve to its exam name")
	}

	if !sourceHasExam(source, "pentacam_corneal_tomography") {
		t.Fatal("source should match Pentacam")
	}

	if sourceGroupKey(source) != "pentacam_corneal_tomography|OD" {
		t.Fatalf("unexpected source group: %q", sourceGroupKey(source))
	}
}

func TestMergeSourceFilesReplacesEyeWhenExamIsArray(t *testing.T) {
	existing := []any{
		map[string]any{
			"path":   "cases/x/old-od.pdf",
			"sha256": "old-od",
			"exam":   []any{"pentacam_corneal_tomography"},
			"eye":    "OD",
		},
		map[string]any{
			"path":   "cases/x/old-os.pdf",
			"sha256": "old-os",
			"exam":   []any{"pentacam_corneal_tomography"},
			"eye":    "OS",
		},
	}

	incoming := []any{
		map[string]any{
			"path":   "cases/x/new-od.pdf",
			"sha256": "new-od",
			"exam":   []any{"pentacam_corneal_tomography"},
			"eye":    "OD",
		},
	}

	merged := mergeSourceFiles(existing, incoming)

	paths := map[string]bool{}
	for _, raw := range merged {
		source := raw.(map[string]any)
		paths[source["path"].(string)] = true
	}

	if paths["cases/x/old-od.pdf"] {
		t.Fatal("old OD source should have been replaced")
	}

	if !paths["cases/x/new-od.pdf"] {
		t.Fatal("new OD source should be current")
	}

	if !paths["cases/x/old-os.pdf"] {
		t.Fatal("OS source should be preserved")
	}
}

func TestNormalizePatientIdentityFieldsRemovesWrongNormalizedAlias(t *testing.T) {
	analysis := map[string]any{
		"patient": map[string]any{
			"full_name":             "PACIENTE TESTE SILVA",
			"birth_date":            "1980-03-04",
			"birth_date_normalized": "1980-04-03",
		},
	}

	NormalizeIdentityFields(analysis)

	patient := analysis["patient"].(map[string]any)

	if patient["birth_date"] != "1980-03-04" {
		t.Fatalf("unexpected canonical birth date: %#v", patient["birth_date"])
	}

	if _, exists := patient["birth_date_normalized"]; exists {
		t.Fatal("birth_date_normalized contraditório deve ser removido")
	}
}

func TestMergePatientCaseRemovesLegacyBirthDateAlias(t *testing.T) {
	existing := map[string]any{
		"patient": map[string]any{
			"full_name":             "PACIENTE TESTE SILVA",
			"birth_date":            "1980-03-04",
			"birth_date_normalized": "1980-04-03",
		},
		"source_files": []any{},
		"exams":        map[string]any{},
	}

	incoming := map[string]any{
		"patient": map[string]any{
			"full_name":  "PACIENTE TESTE SILVA",
			"birth_date": "1980-03-04",
		},
		"source_files": []any{},
		"exams":        map[string]any{},
	}

	merged := Merge(existing, incoming)
	patient := merged["patient"].(map[string]any)

	if patient["birth_date"] != "1980-03-04" {
		t.Fatalf("unexpected canonical birth date: %#v", patient["birth_date"])
	}

	if _, exists := patient["birth_date_normalized"]; exists {
		t.Fatal("legacy birth_date_normalized should be removed during merge")
	}
}

func TestBuildPatientChangePreviewChangedField(t *testing.T) {
	existing := map[string]any{
		"exams": map[string]any{
			"pentacam_corneal_tomography": map[string]any{
				"eyes": map[string]any{
					"OD": map[string]any{
						"anterior_cornea": map[string]any{"kmax_d": 44.2},
					},
				},
			},
		},
	}

	incoming := map[string]any{
		"exams": map[string]any{
			"pentacam_corneal_tomography": map[string]any{
				"eyes": map[string]any{
					"OD": map[string]any{
						"anterior_cornea": map[string]any{"kmax_d": 44.4},
					},
				},
			},
		},
	}

	preview := BuildChangePreview(existing, incoming)

	if preview.Changed != 1 || preview.Added != 0 || preview.Removed != 0 {
		t.Fatalf("unexpected counters: %#v", preview)
	}

	row := preview.Rows[0]
	if row.Eye != "OD" ||
		row.Field != "anterior_cornea.kmax_d" ||
		row.Before != 44.2 ||
		row.After != 44.4 ||
		row.Kind != "changed" {
		t.Fatalf("unexpected row: %#v", row)
	}
}

func TestBuildPatientChangePreviewAddsOSWithoutTouchingOD(t *testing.T) {
	existing := map[string]any{
		"exams": map[string]any{
			"pentacam_corneal_tomography": map[string]any{
				"eyes": map[string]any{
					"OD": map[string]any{"value": 44.2},
				},
			},
		},
	}

	incoming := map[string]any{
		"exams": map[string]any{
			"pentacam_corneal_tomography": map[string]any{
				"eyes": map[string]any{
					"OS": map[string]any{"value": 45.5},
				},
			},
		},
	}

	preview := BuildChangePreview(existing, incoming)

	if preview.Added != 1 ||
		preview.Changed != 0 ||
		preview.Removed != 0 {
		t.Fatalf("unexpected counters: %#v", preview)
	}

	if len(preview.Rows) != 1 {
		t.Fatalf("expected only incoming OS changes, got %#v", preview.Rows)
	}

	row := preview.Rows[0]
	if row.Eye != "OS" ||
		row.Field != "value" ||
		row.Before != nil ||
		row.After != 45.5 ||
		row.Kind != "added" {
		t.Fatalf("unexpected OS row: %#v", row)
	}
}

func TestMergePatientCaseExactReuploadKeepsExistingPayload(t *testing.T) {
	existing := map[string]any{
		"source_files": []any{
			map[string]any{
				"exam": "pentacam_corneal_tomography",
				"eye":  "OS", "sha256": "same",
				"path": "cases/x/original.pdf",
			},
		},
		"exams": map[string]any{
			"pentacam_corneal_tomography": map[string]any{
				"eyes": map[string]any{
					"OS": map[string]any{"value": 45.5},
				},
			},
		},
	}

	incoming := map[string]any{
		"source_files": []any{
			map[string]any{
				"exam": "pentacam_corneal_tomography",
				"eye":  "OS", "sha256": "same",
				"path": "copy.pdf",
			},
		},
		"exams": map[string]any{
			"pentacam_corneal_tomography": map[string]any{
				"eyes": map[string]any{
					"OS": map[string]any{"value": 99.9},
				},
			},
		},
	}

	merged := Merge(existing, incoming)
	os := merged["exams"].(map[string]any)["pentacam_corneal_tomography"].(map[string]any)["eyes"].(map[string]any)["OS"].(map[string]any)

	if os["value"] != 45.5 {
		t.Fatalf("exact reupload must preserve existing payload: %#v", os)
	}
}

func TestBuildPatientChangePreviewExactReuploadHasNoChanges(t *testing.T) {
	existing := map[string]any{
		"source_files": []any{
			map[string]any{
				"exam": "pentacam_corneal_tomography",
				"eye":  "OS", "sha256": "same",
			},
		},
		"exams": map[string]any{
			"pentacam_corneal_tomography": map[string]any{
				"eyes": map[string]any{
					"OS": map[string]any{"value": 45.5},
				},
			},
		},
	}

	incoming := map[string]any{
		"source_files": []any{
			map[string]any{
				"exam": "pentacam_corneal_tomography",
				"eye":  "OS", "sha256": "same",
			},
		},
		"exams": map[string]any{
			"pentacam_corneal_tomography": map[string]any{
				"eyes": map[string]any{
					"OS": map[string]any{"value": 99.9},
				},
			},
		},
	}

	preview := BuildChangePreview(existing, incoming)

	if len(preview.Rows) != 0 ||
		preview.Added != 0 ||
		preview.Changed != 0 ||
		preview.Removed != 0 ||
		preview.Unchanged != 0 {
		t.Fatalf("exact reupload must produce empty preview: %#v", preview)
	}
}

func identityEntries(entries ...map[string]any) map[string]any {
	list := make([]any, 0, len(entries))
	for _, entry := range entries {
		list = append(list, entry)
	}
	return map[string]any{
		"patient":                map[string]any{"full_name": "JOAO FRANCISCO DE AGUIAR"},
		"verificacao_identidade": list,
	}
}

// francisco: EyeSuite prints DMY, Pentacam MDY, same string 08/05/1969.
func TestBirthDateDayMonthInversionKeepsPentacamAndWarns(t *testing.T) {
	analysis := identityEntries(
		map[string]any{"nascimento_lido": "08/05/1969", "timestamp_lido": "2026-08-28 16:36"},
		map[string]any{"nascimento_lido": "08/05/1969", "timestamp_lido": "08/28/2026"},
	)
	date, inverted, ok := ResolveBirthDate(analysis)
	if !ok || !inverted || date != "1969-08-05" {
		t.Fatalf("got %q inverted=%v ok=%v", date, inverted, ok)
	}
	warnings := IdentityWarnings(analysis)
	if len(warnings) != 1 || warnings[0]["code"] != "data_nascimento_invertida" {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

// rolmey: EyeSuite 12/05/1967 (DMY) is the same day as Pentacam 05/12/1967 (MDY).
func TestBirthDatePerDocumentOrderAgrees(t *testing.T) {
	analysis := identityEntries(
		map[string]any{"nascimento_lido": "12/05/1967", "timestamp_lido": "2026-07-30 12:03"},
		map[string]any{"nascimento_lido": "05/12/1967", "timestamp_lido": "07/30/2026"},
	)
	date, inverted, ok := ResolveBirthDate(analysis)
	if !ok || inverted || date != "1967-05-12" {
		t.Fatalf("got %q inverted=%v ok=%v", date, inverted, ok)
	}
	if len(IdentityWarnings(analysis)) != 0 {
		t.Fatal("no warning expected")
	}
}

func TestBirthDateRealConflictIsUnresolved(t *testing.T) {
	analysis := identityEntries(
		map[string]any{"nascimento_lido": "14/12/1979", "timestamp_lido": "2026-08-28 16:36"},
		map[string]any{"nascimento_lido": "11/04/2026", "timestamp_lido": "08/28/2026"},
	)
	if _, ok := CanonicalBirthDate(analysis); ok {
		t.Fatal("conflicting dates must not resolve")
	}
}

func TestNameKeyIgnoresOrderAccentsAndWhitespace(t *testing.T) {
	if NameKey("AVERSA MARTINELLI,  CRESO") != NameKey("Creso  Aversa Martinelli") ||
		NameKey("FRANCISCO DE AGUIAR, JOÃO") != NameKey("joao francisco de aguiar") {
		t.Fatal("device name variants must share one key")
	}
	if NameKey("DANIELA JNOGUEIRA ROCHA") == NameKey("DANIELA NOGUEIRA ROCHA") {
		t.Fatal("OCR typo must not silently collapse")
	}
}

func TestNameWarningForIncompatibleDocumentName(t *testing.T) {
	analysis := identityEntries(
		map[string]any{"nome_lido": "JOAO FRANCISCO"},
		map[string]any{"nome_lido": "jJO40 JFRANCISCO DE AGUIAR", "source": "x.pdf"},
	)
	warnings := IdentityWarnings(analysis)
	if len(warnings) != 1 || warnings[0]["code"] != "nome_divergente" || warnings[0]["source"] != "x.pdf" {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if !NamesCompatible("MARIA GORETE DE C", "MARIA GORETE DE CARVALHO") {
		t.Fatal("truncated device name is compatible")
	}
}

func TestSuspiciousNameIsFlagged(t *testing.T) {
	analysis := map[string]any{"patient": map[string]any{"full_name": "° 2/25 Segmento: 294° NOGUEIRA ROCHA"}}
	warnings := IdentityWarnings(analysis)
	if len(warnings) != 1 || warnings[0]["code"] != "nome_suspeito" {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	analysis["patient"] = map[string]any{"full_name": "Joāo D'Ávila-Silva Jr."}
	if len(IdentityWarnings(analysis)) != 0 {
		t.Fatal("ordinary names must not warn")
	}
}

func TestSameIdentityAcceptsDayMonthInversion(t *testing.T) {
	eyeSuite := map[string]any{"patient": map[string]any{"full_name": "JOAO FRANCISCO DE AGUIAR", "birth_date": "1969-05-08"}}
	pentacam := map[string]any{"patient": map[string]any{"full_name": "AGUIAR DE, FRANCISCO JOAO", "birth_date": "1969-08-05"}}
	other := map[string]any{"patient": map[string]any{"full_name": "OUTRA PESSOA", "birth_date": "1969-08-05"}}
	if !SameIdentity(eyeSuite, pentacam) {
		t.Fatal("inversion of the same patient must match")
	}
	if SameIdentity(eyeSuite, other) {
		t.Fatal("different names must not match")
	}
}

func TestDeclaredDateOrderWinsOverHeuristic(t *testing.T) {
	// Pentacam timestamp ambiguous (08/05/2026): heuristic says MDY, but the
	// extractor declared DMY for this document, so 08/05/1969 is 8 May.
	analysis := identityEntries(
		map[string]any{"nascimento_lido": "08/05/1969", "timestamp_lido": "08/05/2026", "date_order": "DMY"},
	)
	if date, _, ok := ResolveBirthDate(analysis); !ok || date != "1969-05-08" {
		t.Fatalf("declared DMY: got %q %v", date, ok)
	}
	delete(analysis["verificacao_identidade"].([]any)[0].(map[string]any), "date_order")
	if date, _, ok := ResolveBirthDate(analysis); !ok || date != "1969-08-05" {
		t.Fatalf("heuristic fallback: got %q %v", date, ok)
	}
	nidek := identityEntries(map[string]any{"nascimento_lido": "28/JAN/2001", "date_order": "DMONY"})
	if date, _, ok := ResolveBirthDate(nidek); !ok || date != "2001-01-28" {
		t.Fatalf("DMONY: got %q %v", date, ok)
	}
	inv := identityEntries(
		map[string]any{"nascimento_lido": "08/05/1969", "date_order": "DMY"},
		map[string]any{"nascimento_lido": "08/05/1969", "date_order": "MDY"},
	)
	if date, inverted, ok := ResolveBirthDate(inv); !ok || !inverted || date != "1969-08-05" {
		t.Fatalf("declared inversion: got %q %v %v", date, inverted, ok)
	}
}

func TestNormalizeNeverKeepsRawBirthDate(t *testing.T) {
	analysis := identityEntries(map[string]any{"nascimento_lido": "03/04/1980"})
	analysis["patient"].(map[string]any)["birth_date"] = "03/04/1980"
	NormalizeIdentityFields(analysis)
	if _, has := analysis["patient"].(map[string]any)["birth_date"]; has {
		t.Fatal("unresolved birth_date must be absent, not raw")
	}
}

func TestGarbageNameNeverBecomesIdentity(t *testing.T) {
	garbage := "° 2/25 Segmento: 294° 114° \\ NOGUEIRA ROCHA"
	only := identityEntries(map[string]any{"nome_lido": garbage}, map[string]any{"nome_lido": "jJO40 JFRANCISCO"})
	only["patient"] = map[string]any{"full_name": garbage, "birth_date": "2001-01-28"}
	if key, ok := Identity(only); ok {
		t.Fatalf("garbage-only must be unresolved, got %q", key)
	}
	mixed := identityEntries(
		map[string]any{"nome_lido": garbage},
		map[string]any{"nome_lido": "NOGUEIRA ROCHA, DANIELA"},
		map[string]any{"nome_lido": "DANIELA NOGUEIRA ROCHA"},
	)
	mixed["patient"] = map[string]any{"full_name": garbage, "birth_date": "2001-01-28"}
	key, ok := Identity(mixed)
	if !ok || key != "daniela nogueira rocha|2001-01-28" {
		t.Fatalf("clean name must win, got %q %v", key, ok)
	}
	NormalizeIdentityFields(mixed)
	if mixed["patient"].(map[string]any)["full_name"] == garbage {
		t.Fatal("garbage must be replaced in patient.full_name")
	}
}
