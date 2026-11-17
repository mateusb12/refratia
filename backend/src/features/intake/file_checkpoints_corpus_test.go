package intake

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"refratia/backend/src/features/examParsers/pentacam"
	"refratia/backend/src/features/fileIdentification/classification"
	"refratia/backend/src/shared/profiling"
)

// profileSince returns, per document index, the OCR calls and the checkpoint
// miss reason written to REFRATIA_PROFILE after offset.
func profileSince(t *testing.T, offset int64) (ocrCalls map[int]int, miss map[int]string, end int64) {
	t.Helper()
	file, err := os.Open(os.Getenv("REFRATIA_PROFILE"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	file.Seek(offset, 0)
	ocrCalls, miss = map[int]int{}, map[int]string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var line struct {
			Doc   *int           `json:"doc"`
			Stage string         `json:"stage"`
			Attrs map[string]any `json:"attrs"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) != nil || line.Doc == nil {
			continue
		}
		switch line.Stage {
		case "ocr.recognize":
			ocrCalls[*line.Doc]++
		case "checkpoint.prepare":
			reason, _ := line.Attrs["miss"].(string)
			if line.Attrs["reused"] == true {
				reason = "hit"
			}
			miss[*line.Doc] = reason
		}
	}
	end, _ = file.Seek(0, 2)
	return ocrCalls, miss, end
}

// Real extraction path, one corpus patient: a fresh upload writes checkpoints,
// an identical upload reuses every eligible document without extraction OCR,
// and a batch with one changed document extracts only that document again.
func TestCheckpointReuseOnCorpus(t *testing.T) {
	uploads := corpusUploads(t)
	if !profiling.Enabled() {
		t.Skip("set REFRATIA_PROFILE to count OCR calls")
	}
	// As the frontend does: analyze receives the type and eye inspect recognized.
	inspections := make([]classification.FileInspection, len(uploads))
	for index, upload := range uploads {
		inspections[index] = classification.InspectFile(context.Background(), classification.File{
			Filename: upload.name, ContentType: upload.contentType, Size: int64(len(upload.data)), Data: upload.data,
		})
		// The user assigns the eye inspect left open (the benchmark sends the
		// ground truth); DetectEye stands in for that here.
		if inspections[index].RecognizedExamType == "pentacam_corneal_tomography" && inspections[index].RecognizedEye == "" {
			inspections[index].RecognizedEye, _ = pentacam.DetectEye(context.Background(), upload.data)
		}
	}
	toFiles := func() []UploadedFile {
		files := make([]UploadedFile, len(uploads))
		for index, upload := range uploads {
			sum := sha256.Sum256(upload.data)
			files[index] = UploadedFile{Data: upload.data, Metadata: FileMetadata{
				Filename: upload.name, ContentType: upload.contentType, Size: int64(len(upload.data)), SHA256: hex.EncodeToString(sum[:]),
				ExamType: inspections[index].RecognizedExamType, Eye: inspections[index].RecognizedEye,
			}}
		}
		return files
	}
	storage := newMemoryStorage()
	_, _, offset := profileSince(t, 0)
	runWith := func(storage *memoryStorage, force bool, files []UploadedFile) (map[string]any, map[int]int, map[int]string) {
		executionContext := withFileCheckpointStorage(profiling.WithRequest(context.Background(), "t", "analyze", "t"), storage, "bucket", force, nil)
		analysis := extractPatientLocal(executionContext, files)
		if err := currentFileCheckpointStorageError(executionContext); err != nil {
			t.Fatal(err)
		}
		var ocrCalls map[int]int
		var miss map[int]string
		ocrCalls, miss, offset = profileSince(t, offset)
		return analysis, ocrCalls, miss
	}
	run := func(files []UploadedFile) (map[string]any, map[int]int, map[int]string) {
		return runWith(storage, false, files)
	}
	// Re-running OCR on the same report can move a diagnostic's evidence
	// (bbox, candidates, votes) without changing status or value: known
	// variation, compared without it.
	normalized := func(analysis map[string]any) any {
		var value map[string]any
		encoded, _ := json.Marshal(analysis)
		json.Unmarshal(encoded, &value)
		diagnostics, _ := value["extraction_diagnostics"].([]any)
		for _, raw := range diagnostics {
			if diagnostic, _ := raw.(map[string]any); diagnostic != nil {
				delete(diagnostic, "bbox")
				delete(diagnostic, "candidates")
				delete(diagnostic, "votes")
			}
		}
		return value
	}

	files := toFiles()
	fresh, freshOCR, freshMiss := run(files)
	eligible := map[int]bool{}
	for index, file := range files {
		if _, err := storage.object(processedFileCheckpointObjectKey(file.Metadata.SHA256)); err == nil {
			eligible[index] = true
		}
	}
	t.Logf("fresh: %d/%d documents checkpointed; OCR calls per doc %v; miss %v", len(eligible), len(files), freshOCR, freshMiss)
	if len(eligible) == 0 {
		t.Fatal("fresh upload wrote no checkpoint")
	}

	repeat, repeatOCR, repeatMiss := run(toFiles())
	t.Logf("identical: OCR calls per doc %v; miss %v", repeatOCR, repeatMiss)
	for index := range files {
		if eligible[index] && (repeatMiss[index] != "hit" || repeatOCR[index] != 0) {
			t.Errorf("doc %d: eligible but %q with %d OCR calls", index, repeatMiss[index], repeatOCR[index])
		}
		if !eligible[index] && repeatMiss[index] == "hit" {
			t.Errorf("doc %d: reused without a checkpoint", index)
		}
	}
	if !reflect.DeepEqual(normalized(fresh), normalized(repeat)) {
		t.Error("identical upload: reused analysis differs from the fresh one")
		os.WriteFile(os.Getenv("REFRATIA_CK_DUMP")+".fresh.json", mustJSON(fresh), 0o600)
		os.WriteFile(os.Getenv("REFRATIA_CK_DUMP")+".repeat.json", mustJSON(repeat), 0o600)
	}

	changed := -1
	for index := range files {
		if eligible[index] && files[index].Metadata.ContentType == "application/pdf" {
			changed = index
			break
		}
	}
	if changed < 0 {
		t.Skip("no eligible PDF to change")
	}
	batch := toFiles()
	// Bytes after %%EOF: a different SHA-256, the same rendered document.
	batch[changed].Data = append(append([]byte{}, batch[changed].Data...), '\n')
	sum := sha256.Sum256(batch[changed].Data)
	batch[changed].Metadata.SHA256, batch[changed].Metadata.Size = hex.EncodeToString(sum[:]), int64(len(batch[changed].Data))
	cachedBatch, batchOCR, batchMiss := run(batch)
	t.Logf("one changed (doc %d): OCR calls per doc %v; miss %v", changed, batchOCR, batchMiss)
	if batchMiss[changed] != "not_found" || batchOCR[changed] == 0 {
		t.Errorf("changed doc: %q with %d OCR calls", batchMiss[changed], batchOCR[changed])
	}
	for index := range files {
		if index != changed && eligible[index] && (batchMiss[index] != "hit" || batchOCR[index] != 0) {
			t.Errorf("doc %d: unchanged eligible but %q with %d OCR calls", index, batchMiss[index], batchOCR[index])
		}
	}

	// A reused document must yield what extracting it again in the same batch
	// yields: the changed batch, and the checkpointed documents alone (none of
	// the identity documents of the batch that wrote their checkpoints).
	var subset []UploadedFile
	for index, file := range toFiles() {
		if eligible[index] {
			subset = append(subset, file)
		}
	}
	cachedSubset, _, subsetMiss := run(subset)
	t.Logf("checkpointed documents alone: miss %v", subsetMiss)
	for name, pair := range map[string][2][]UploadedFile{"changed batch": {batch}, "checkpointed alone": {subset}} {
		cached := map[string]map[string]any{"changed batch": cachedBatch, "checkpointed alone": cachedSubset}[name]
		fresh, _, _ := runWith(newMemoryStorage(), true, pair[0])
		if !reflect.DeepEqual(normalized(cached), normalized(fresh)) {
			t.Errorf("%s: cached analysis differs from forced-fresh extraction", name)
			os.WriteFile(os.Getenv("REFRATIA_CK_DUMP")+"."+strings.ReplaceAll(name, " ", "_")+".cached.json", mustJSON(cached), 0o600)
			os.WriteFile(os.Getenv("REFRATIA_CK_DUMP")+"."+strings.ReplaceAll(name, " ", "_")+".fresh.json", mustJSON(fresh), 0o600)
		}
	}
}

func mustJSON(value any) []byte {
	encoded, _ := json.MarshalIndent(value, "", " ")
	return encoded
}
