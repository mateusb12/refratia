package intake

import (
	"context"
	"testing"

	progress "refratia/backend/src/shared/progress"
)

// Every uploaded file must produce exactly one file_result, whatever the
// declared type (contract key or legacy label), including unknown and
// out-of-protocol ones.
func TestExtractPatientLocalEmitsOneFileResultPerFile(t *testing.T) {
	cases := []struct {
		name, examType, contentType, wantStatus, wantType string
	}{
		{"contract key, undecodable image", "fundus_retinography", "image/jpeg", "failed", "fundus_retinography"},
		{"legacy label is normalised", "RETINA", "image/jpeg", "failed", "fundus_retinography"},
		{"eyesuite as contract key is dispatched", "iol_calculation", "application/pdf", "failed", "iol_calculation"},
		{"unknown type", "", "image/jpeg", "failed", ""},
		{"out of protocol type", "anterior_segment_photo", "image/jpeg", "identified", "anterior_segment_photo"},
		{"pentacam needs pdf", "pentacam_corneal_tomography", "image/jpeg", "failed", "pentacam_corneal_tomography"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var results []map[string]any
			executionContext := progress.WithReporter(context.Background(), func(event progress.Event) {
				if event.Type == "file_result" {
					results = append(results, event.Payload.(map[string]any))
				}
			})
			file := uploadedFile{Data: []byte("not a document")}
			file.Metadata.Filename = "anything"
			file.Metadata.ContentType = testCase.contentType
			file.Metadata.ExamType = testCase.examType
			file.Metadata.Eye = "OD"
			file.Metadata.SHA256 = "00"
			extractPatientLocal(executionContext, []uploadedFile{file})
			if len(results) != 1 {
				t.Fatalf("got %d file_result events, want 1", len(results))
			}
			if results[0]["status"] != testCase.wantStatus || results[0]["examType"] != testCase.wantType {
				t.Fatalf("got status=%v examType=%v", results[0]["status"], results[0]["examType"])
			}
		})
	}
}
