package intake

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"refratia/backend/src/features/examParsers/eyesuite"
	"refratia/backend/src/features/examParsers/retinography"
	"refratia/backend/src/features/examParsers/specularmicroscopy"
)

// boundaryDoc is one upload: its file and the writes its extractor makes (the
// real post-OCR writers, fed a fixed OCR result).
type boundaryDoc struct {
	file          UploadedFile
	examType, eye string
	write         func(analysis map[string]any, file UploadedFile) error
}

func retinaDoc(name, sha, patientID, eye string) boundaryDoc {
	return boundaryDoc{
		file: UploadedFile{Data: []byte(sha), Metadata: FileMetadata{
			Filename: name, ContentType: "image/jpeg", Size: 3, SHA256: strings.Repeat(sha, 64/len(sha)),
			ExamType: "fundus_retinography", Eye: eye,
		}},
		examType: "fundus_retinography", eye: eye,
		write: func(analysis map[string]any, file UploadedFile) error {
			_, err := mergeRetinographyResult(analysis, file, retinography.Result{
				PatientID: patientID, Eye: eye, Mode: "Retina", ExamDateTime: "2024-01-01T10:00:00",
				Exam: map[string]any{"eyes": map[string]any{eye: map[string]any{
					"patient_id": patientID, "eye": eye, "exam_datetime": "2024-01-01T10:00:00",
				}}},
			})
			return err
		},
	}
}

// identityDoc reads a name and birth date (the EyeSuite writer); it is never
// checkpointed, so every batch extracts it.
func identityDoc(name, sha, fullName, birth string) boundaryDoc {
	return boundaryDoc{
		file: UploadedFile{Data: []byte(sha), Metadata: FileMetadata{
			Filename: name, ContentType: "application/pdf", Size: 3, SHA256: strings.Repeat(sha, 64/len(sha)),
		}},
		write: func(analysis map[string]any, file UploadedFile) error {
			mergeLocalPatientIdentity(analysis, file.Metadata.Filename, fullName, birth, "01/01/2024", "DMY")
			return nil
		},
	}
}

// runBoundaryBatch mirrors extractPatientLocal: prepare, then on a miss the
// extractor's writes and emitLocalFileResult (which saves the checkpoint).
func runBoundaryBatch(t *testing.T, storage *memoryStorage, force bool, docs []boundaryDoc) (map[string]any, []string) {
	t.Helper()
	executionContext := withFileCheckpointStorage(context.Background(), storage, "bucket", force, nil)
	analysis := map[string]any{"exams": map[string]any{}}
	outcomes := []string{}
	for _, doc := range docs {
		fileCtx := withCurrentCheckpointFile(executionContext, doc.file)
		reused, miss, err := prepareFileForIdempotentProcessing(fileCtx, doc.file, analysis)
		if err != nil {
			outcomes = append(outcomes, "error")
			return analysis, outcomes
		}
		if reused {
			outcomes = append(outcomes, "hit")
			continue
		}
		outcomes = append(outcomes, miss)
		fileCtx = withExtractionStart(fileCtx, analysis)
		if doc.write(analysis, doc.file) == nil && doc.examType != "" {
			emitLocalFileResult(fileCtx, doc.file.Metadata.Filename, doc.examType, doc.eye, "extracted", "ok", analysis)
		}
	}
	return analysis, outcomes
}

func boundaryJSON(value map[string]any) any {
	var normalized any
	encoded, _ := json.Marshal(value)
	json.Unmarshal(encoded, &normalized)
	return normalized
}

// A document reused in a changed batch must yield what extracting it again in
// that batch yields: nothing of the batch that created its checkpoint may ride
// along (patient identity, identity readings, exam fields, source attribution).
func TestCheckpointCarriesOnlyItsOwnDocument(t *testing.T) {
	maria := identityDoc("eyesuite.pdf", "a1", "MARIA SILVA SANTOS", "05/03/1960")
	jose := identityDoc("eyesuite.pdf", "a2", "JOSE PEREIRA LIMA", "11/07/1955")
	retinaOD := retinaDoc("od.jpg", "b1", "R1", "OD")
	for _, test := range []struct {
		name           string
		created, batch []boundaryDoc
		reusedIndex    int
	}{
		{"alone after a batch with another identity document", []boundaryDoc{maria, retinaOD}, []boundaryDoc{retinaOD}, 0},
		{"next to a different patient's document", []boundaryDoc{maria, retinaOD}, []boundaryDoc{jose, retinaOD}, 1},
		{"same patient, identity document changed", []boundaryDoc{maria, retinaOD}, []boundaryDoc{
			identityDoc("eyesuite.pdf", "a3", "MARIA SILVA SANTOS", "05/03/1960"), retinaOD}, 1},
		{"same basename as another document of its batch",
			[]boundaryDoc{identityDoc("scan.jpg", "a1", "MARIA SILVA SANTOS", "05/03/1960"), retinaDoc("scan.jpg", "b2", "R1", "OD")},
			[]boundaryDoc{retinaDoc("scan.jpg", "b2", "R1", "OD")}, 0},
		{"next to a retina photo of another patient id", []boundaryDoc{retinaOD},
			[]boundaryDoc{retinaDoc("os.jpg", "c1", "R2", "OS"), retinaOD}, 1},
		{"identical batch", []boundaryDoc{maria, retinaOD, retinaDoc("os.jpg", "c2", "R1", "OS")},
			[]boundaryDoc{maria, retinaOD, retinaDoc("os.jpg", "c2", "R1", "OS")}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage := newMemoryStorage()
			runBoundaryBatch(t, storage, false, test.created)
			cached, cachedOutcomes := runBoundaryBatch(t, storage, false, test.batch)
			fresh, freshOutcomes := runBoundaryBatch(t, newMemoryStorage(), true, test.batch)
			if freshOutcomes[len(freshOutcomes)-1] == "error" {
				t.Fatalf("forced-fresh run failed: %v", freshOutcomes)
			}
			if cachedOutcomes[len(cachedOutcomes)-1] == "error" {
				t.Fatalf("cached run aborted where extraction succeeds: %v", cachedOutcomes)
			}
			if !reflect.DeepEqual(boundaryJSON(cached), boundaryJSON(fresh)) {
				a, _ := json.MarshalIndent(cached, "", " ")
				b, _ := json.MarshalIndent(fresh, "", " ")
				t.Fatalf("cached (%v) differs from forced-fresh:\ncached %s\nfresh  %s", cachedOutcomes, a, b)
			}
			t.Logf("outcomes %v", cachedOutcomes)
		})
	}
}

// specularDoc and iolDoc use the real post-parse writers, which replace the
// whole exam (wrapped in the eye validation their callers defer).
func specularDoc(name, sha, eye string, density float64) boundaryDoc {
	return boundaryDoc{
		file: UploadedFile{Data: []byte(sha), Metadata: FileMetadata{
			Filename: name, ContentType: "image/jpeg", Size: 3, SHA256: strings.Repeat(sha, 64/len(sha)),
			ExamType: "specular_microscopy", Eye: eye,
		}},
		examType: "specular_microscopy", eye: eye,
		write: func(analysis map[string]any, file UploadedFile) error {
			defer validateTouchedEyes(analysis, file, snapshotEyes(analysis))
			_, err := mergeSpecularResult(analysis, file, specularmicroscopy.Result{
				ExamDateTime: "2024-01-01T10:00:00",
				Exam: map[string]any{"eyes": map[string]any{eye: map[string]any{
					"cell_density_cells_per_mm2": density,
				}}},
			})
			return err
		},
	}
}

func iolEye(axial float64) map[string]any {
	return map[string]any{
		"axial_length_mm": axial, "anterior_chamber_depth_mm": 3.1, "lens_thickness_mm": 4.2,
		"white_to_white_mm": 11.9, "target_refraction_d": -0.5,
		"keratometry": map[string]any{"k1_d": 43.0, "k2_d": 44.0, "mean_k_d": 43.5, "astigmatism_d": 1.0, "astigmatism_axis_deg": 90.0},
	}
}

func iolDoc(name, sha string, axialOD, axialOS float64) boundaryDoc {
	return boundaryDoc{
		file: UploadedFile{Data: []byte(sha), Metadata: FileMetadata{
			Filename: name, ContentType: "application/pdf", Size: 3, SHA256: strings.Repeat(sha, 64/len(sha)),
			ExamType: "iol_calculation", Eye: "AO",
		}},
		examType: "iol_calculation", eye: "AO",
		write: func(analysis map[string]any, file UploadedFile) error {
			defer validateTouchedEyes(analysis, file, snapshotEyes(analysis))
			mergeEyeSuiteResult(analysis, file, eyesuite.Result{Exam: map[string]any{
				"device": "IOLMaster", "eyes": map[string]any{"OD": iolEye(axialOD), "OS": iolEye(axialOS)},
			}})
			return nil
		},
	}
}

// Writers that replace the whole exam (specular, IOL) keep only the last file
// of their type; a hit must land the same way, in either order and mixed with
// fresh extractions.
func TestCheckpointHitReplacesLikeTheWriter(t *testing.T) {
	specOD := specularDoc("spec-od.jpg", "d1", "OD", 2500)
	specOS := specularDoc("spec-os.jpg", "d2", "OS", 2600)
	iolA := iolDoc("iol-a.pdf", "e1", 23.1, 23.4)
	iolB := iolDoc("iol-b.pdf", "e2", 24.0, 24.2)
	iolC := iolDoc("iol-c.pdf", "e3", 23.1, 25.0) // same OD as iolA
	for _, test := range []struct {
		name           string
		created, batch []boundaryDoc
	}{
		{"specular OD then OS, both hits", []boundaryDoc{specOD, specOS}, []boundaryDoc{specOD, specOS}},
		{"specular OS then OD, both hits", []boundaryDoc{specOD, specOS}, []boundaryDoc{specOS, specOD}},
		{"specular hit then miss", []boundaryDoc{specOD}, []boundaryDoc{specOD, specOS}},
		{"specular miss then hit", []boundaryDoc{specOD}, []boundaryDoc{specOS, specOD}},
		{"iol A then B, both hits", []boundaryDoc{iolA, iolB}, []boundaryDoc{iolA, iolB}},
		{"iol B then A, both hits", []boundaryDoc{iolA, iolB}, []boundaryDoc{iolB, iolA}},
		{"iol miss then hit", []boundaryDoc{iolA}, []boundaryDoc{iolB, iolA}},
		{"iol sharing an eye with the batch that created it, alone", []boundaryDoc{iolA, iolC}, []boundaryDoc{iolC}},
		{"iol sharing an eye, after the other", []boundaryDoc{iolA, iolC}, []boundaryDoc{iolA, iolC}},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage := newMemoryStorage()
			runBoundaryBatch(t, storage, false, test.created)
			cached, cachedOutcomes := runBoundaryBatch(t, storage, false, test.batch)
			fresh, _ := runBoundaryBatch(t, newMemoryStorage(), true, test.batch)
			if !reflect.DeepEqual(boundaryJSON(cached), boundaryJSON(fresh)) {
				a, _ := json.MarshalIndent(cached, "", " ")
				b, _ := json.MarshalIndent(fresh, "", " ")
				t.Fatalf("cached (%v) differs from forced-fresh:\ncached %s\nfresh  %s", cachedOutcomes, a, b)
			}
			t.Logf("outcomes %v", cachedOutcomes)
		})
	}
}

// pentacamEye is a complete, plausible eye (k1 offset by delta to tell
// documents apart); tkc as given, nil leaves it out.
func pentacamEye(delta float64, tkc any) map[string]any {
	plausible := map[string]float64{
		"k1_d": 43 + delta, "k2_d": 44, "km_d": 43.5, "astigmatism_d": 1, "thinnest_um": 540, "d": 1.2,
		"art_max": 450, "isv": 20, "iva": 0.15, "iha": 3, "ihd": 0.01, "ki": 1.02, "cki": 1.01,
		"z31_coma": 0.1, "internal_anterior_chamber_depth_mm": 3, "total_corneal_z40_6mm_um": 0.25,
	}
	eye := map[string]any{}
	for _, path := range pentacamRequiredNumericPaths {
		node := eye
		for _, key := range path[:len(path)-1] {
			child, _ := node[key].(map[string]any)
			if child == nil {
				child = map[string]any{}
				node[key] = child
			}
			node = child
		}
		node[path[len(path)-1]] = plausible[path[len(path)-1]]
	}
	if tkc != nil {
		eye["topometric_indices_8mm"].(map[string]any)["tkc"] = tkc
	}
	return eye
}

func pentacamFile(name, sha, eye string) UploadedFile {
	return UploadedFile{Data: []byte(sha), Metadata: FileMetadata{
		Filename: name, ContentType: "application/pdf", Size: 3, SHA256: strings.Repeat(sha, 64/len(sha)),
		ExamType: "pentacam_corneal_tomography", Eye: eye,
	}}
}

func pentacamDoc(name, sha, eye string, base float64) boundaryDoc {
	return boundaryDoc{
		file: pentacamFile(name, sha, eye), examType: "pentacam_corneal_tomography", eye: eye,
		write: func(analysis map[string]any, file UploadedFile) error {
			defer validateTouchedEyes(analysis, file, snapshotEyes(analysis))
			mergeLocalPentacamExam(analysis, eye, file.Metadata.Filename, pentacamEye(base, "Normal"))
			return nil
		},
	}
}

// The Pentacam writer replaces one eye and appends its file to exam.source.
func TestCheckpointHitReplacesPentacamEyeLikeTheWriter(t *testing.T) {
	odA := pentacamDoc("penta-od-a.pdf", "f1", "OD", 0)
	odB := pentacamDoc("penta-od-b.pdf", "f2", "OD", 0.5)
	os := pentacamDoc("penta-os.pdf", "f3", "OS", 1)
	for _, test := range []struct {
		name           string
		created, batch []boundaryDoc
	}{
		{"same eye A then B, both hits", []boundaryDoc{odA, odB}, []boundaryDoc{odA, odB}},
		{"same eye B then A, both hits", []boundaryDoc{odA, odB}, []boundaryDoc{odB, odA}},
		{"same eye miss then hit", []boundaryDoc{odA}, []boundaryDoc{odB, odA}},
		{"OS miss, OD hit, OD miss", []boundaryDoc{odA}, []boundaryDoc{os, odA, odB}},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage := newMemoryStorage()
			runBoundaryBatch(t, storage, false, test.created)
			cached, cachedOutcomes := runBoundaryBatch(t, storage, false, test.batch)
			fresh, _ := runBoundaryBatch(t, newMemoryStorage(), true, test.batch)
			if !reflect.DeepEqual(boundaryJSON(cached), boundaryJSON(fresh)) {
				a, _ := json.MarshalIndent(cached["exams"], "", " ")
				b, _ := json.MarshalIndent(fresh["exams"], "", " ")
				t.Fatalf("cached (%v) differs from forced-fresh:\ncached %s\nfresh  %s", cachedOutcomes, a, b)
			}
			t.Logf("outcomes %v", cachedOutcomes)
		})
	}
}
