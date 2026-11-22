package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"refratia/backend/src/shared/fileStorage"
)

// memoryStorage is the slice of fileStorage.Repository the checkpoints use.
type memoryStorage struct {
	fileStorage.Repository
	mutex   sync.Mutex
	objects map[string][]byte
}

func newMemoryStorage() *memoryStorage { return &memoryStorage{objects: map[string][]byte{}} }

func (storage *memoryStorage) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	data, _ := io.ReadAll(input.Body)
	storage.mutex.Lock()
	defer storage.mutex.Unlock()
	storage.objects[aws.ToString(input.Key)] = data
	return &s3.PutObjectOutput{}, nil
}

func (storage *memoryStorage) object(key string) ([]byte, error) {
	storage.mutex.Lock()
	defer storage.mutex.Unlock()
	if data, found := storage.objects[key]; found {
		return data, nil
	}
	return nil, &smithy.GenericAPIError{Code: "NoSuchKey"}
}

func (storage *memoryStorage) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	data, err := storage.object(aws.ToString(input.Key))
	if err != nil {
		return nil, err
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func (storage *memoryStorage) HeadObject(_ context.Context, input *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	_, err := storage.object(aws.ToString(input.Key))
	return &s3.HeadObjectOutput{}, err
}

func completeRetinaAnalysis() map[string]any {
	return map[string]any{"exams": map[string]any{"fundus_retinography": map[string]any{
		"id": "R1", "device_or_mode": "Retina",
		"eyes": map[string]any{"OD": map[string]any{"patient_id": "R1", "eye": "OD", "exam_datetime": "2024-01-01T10:00:00"}},
	}}}
}

func retinaFile(eye string) UploadedFile {
	return UploadedFile{Metadata: FileMetadata{
		Filename: "doc.jpg", ContentType: "image/jpeg", Size: 3, SHA256: strings.Repeat("ab", 32),
		ExamType: "fundus_retinography", Eye: eye,
	}, Data: []byte("img")}
}

// The analyze loop passes canonical contract keys; completeness must accept them.
func TestCheckpointCompletenessAcceptsContractKeys(t *testing.T) {
	if !fileCheckpointResultIsComplete(completeRetinaAnalysis(), "fundus_retinography", "OD", "") {
		t.Fatal("complete retina result rejected for contract key fundus_retinography")
	}
	if !fileCheckpointResultIsComplete(completeRetinaAnalysis(), "RETINA", "OD", "") {
		t.Fatal("legacy label no longer accepted")
	}
}

// saveThenPrepare runs the real write path (emitLocalFileResult) for file, then
// mutate may tamper with the stored object, then the real read path runs for next.
func saveThenPrepare(t *testing.T, file, next UploadedFile, mutate func(storage *memoryStorage, key string)) (bool, string, map[string]any) {
	t.Helper()
	storage := newMemoryStorage()
	executionContext := withFileCheckpointStorage(context.Background(), storage, "bucket", false, nil)
	emitLocalFileResult(withCurrentCheckpointFile(executionContext, file), file.Metadata.Filename,
		"fundus_retinography", file.Metadata.Eye, "extracted", "ok", completeRetinaAnalysis())
	key := processedFileCheckpointObjectKey(file.Metadata.SHA256)
	if _, err := storage.object(key); err != nil {
		t.Fatal("complete result was not checkpointed")
	}
	if mutate != nil {
		mutate(storage, key)
	}
	analysis := map[string]any{"exams": map[string]any{}}
	reused, miss, err := prepareFileForIdempotentProcessing(withCurrentCheckpointFile(executionContext, next), next, analysis)
	if err != nil {
		t.Fatal(err)
	}
	return reused, miss, analysis
}

func rewriteCheckpoint(edit func(*processedFileCheckpoint)) func(*memoryStorage, string) {
	return func(storage *memoryStorage, key string) {
		var checkpoint processedFileCheckpoint
		data, _ := storage.object(key)
		json.Unmarshal(data, &checkpoint)
		edit(&checkpoint)
		storage.objects[key], _ = json.Marshal(checkpoint)
	}
}

func TestCheckpointReuse(t *testing.T) {
	file := retinaFile("OD")
	for _, test := range []struct {
		name   string
		next   UploadedFile
		mutate func(*memoryStorage, string)
		miss   string
	}{
		{"identical upload hits", file, nil, ""},
		{"renamed upload hits", func() UploadedFile { renamed := file; renamed.Metadata.Filename = "other.jpg"; return renamed }(), nil, ""},
		{"stale processor version", file, rewriteCheckpoint(func(c *processedFileCheckpoint) { c.ProcessorVersion = "v1" }), "version"},
		{"previous processor version v6", file, rewriteCheckpoint(func(c *processedFileCheckpoint) { c.ProcessorVersion = "v6" }), "version"},
		{"previous processor version v7", file, rewriteCheckpoint(func(c *processedFileCheckpoint) { c.ProcessorVersion = "v7" }), "version"},
		{"previous processor version v8", file, rewriteCheckpoint(func(c *processedFileCheckpoint) { c.ProcessorVersion = "v8" }), "version"},
		{"incomplete result", file, rewriteCheckpoint(func(c *processedFileCheckpoint) {
			exam := c.Analysis["exams"].(map[string]any)["fundus_retinography"].(map[string]any)
			delete(exam["eyes"].(map[string]any)["OD"].(map[string]any), "exam_datetime")
		}), "incomplete"},
		{"unreadable object", file, func(storage *memoryStorage, key string) { storage.objects[key] = []byte("{not json") }, "unreadable"},
		{"declared eye changed", retinaFile("OS"), nil, "classification_inputs"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reused, miss, analysis := saveThenPrepare(t, file, test.next, test.mutate)
			if reused != (test.miss == "") || miss != test.miss {
				t.Fatalf("reused=%v miss=%q; want miss %q", reused, miss, test.miss)
			}
			exam, _ := analysis["exams"].(map[string]any)["fundus_retinography"].(map[string]any)
			if reused && (exam == nil || exam["source"].([]any)[0] != test.next.Metadata.Filename) {
				t.Fatalf("reused analysis not merged for the current file: %v", analysis)
			}
			if !reused && exam != nil {
				t.Fatal("a miss merged checkpoint data")
			}
		})
	}
}

// Checkpoints from before v9 (v6: no "+" in staging grades; v7: no birth-date re-check after padding; v8: ISV/KI
// and EyeSuite K axes read by the older readers) live under
// other keys, so the current reader never sees them and re-extracts.
func TestOlderCheckpointsAreNotReused(t *testing.T) {
	key := processedFileCheckpointObjectKey(strings.Repeat("ab", 32))
	for _, old := range []string{"/v6/", "/v7/", "/v8/"} {
		if strings.Contains(key, old) {
			t.Fatalf("current checkpoint key %q still reads %s objects", key, old)
		}
	}
}

// An old v1 object sits under another key: a v2 reader never sees it.
func TestCheckpointKeyIsVersioned(t *testing.T) {
	if !strings.Contains(processedFileCheckpointObjectKey(strings.Repeat("ab", 32)), "/"+fileCheckpointVersion+"/") {
		t.Fatal("checkpoint key does not carry the processor version")
	}
}

// A hit appends its file to exam.source like the extractor does: OD extracted,
// OS reused keeps both, in upload order.
func TestCheckpointHitAppendsExamSource(t *testing.T) {
	current := map[string]any{"exams": map[string]any{"pentacam_corneal_tomography": map[string]any{
		"source": []any{"od.pdf"}, "eyes": map[string]any{"OD": map[string]any{"k": 1.0}},
	}}}
	checkpoint := map[string]any{"exams": map[string]any{"pentacam_corneal_tomography": map[string]any{
		"source": []any{"os.pdf"}, "eyes": map[string]any{"OS": map[string]any{"k": 2.0}},
	}}}
	merged, err := mergeFileCheckpointAnalysis(current, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	exam := merged["exams"].(map[string]any)["pentacam_corneal_tomography"].(map[string]any)
	if fmt.Sprint(exam["source"]) != "[od.pdf os.pdf]" || len(exam["eyes"].(map[string]any)) != 2 {
		t.Fatalf("source %v eyes %v", exam["source"], exam["eyes"])
	}
}

// The checkpoint keeps only the eyes this file wrote, not what earlier files
// of the upload put in the same exam (per-eye writers such as Pentacam).
func TestCheckpointKeepsOnlyEyesTheFileWrote(t *testing.T) {
	analysis := map[string]any{"exams": map[string]any{"pentacam_corneal_tomography": map[string]any{
		"eyes": map[string]any{"OD": map[string]any{"k": 1.0}},
	}}}
	before := snapshotEyes(analysis)
	analysis["exams"].(map[string]any)["pentacam_corneal_tomography"].(map[string]any)["eyes"].(map[string]any)["OS"] =
		map[string]any{"k": 2.0}
	checkpoint := buildAnalysisForFileCheckpoint(analysis, retinaFile("OS"), "pentacam_corneal_tomography", "OS", before, 0)
	eyes := checkpoint["exams"].(map[string]any)["pentacam_corneal_tomography"].(map[string]any)["eyes"].(map[string]any)
	if _, leaked := eyes["OD"]; leaked || eyes["OS"] == nil {
		t.Fatalf("eyes in checkpoint: %v", eyes)
	}
	if buildAnalysisForFileCheckpoint(analysis, retinaFile("OS"), "pentacam_corneal_tomography", "OS", snapshotEyes(analysis), 0) != nil {
		t.Fatal("a file that wrote no eye must not be checkpointed")
	}
}
