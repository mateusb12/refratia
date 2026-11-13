package main

import (
	"context"
	"testing"

	progress "refratia/backend/shared/progress"
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
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var results []map[string]any
			ctx := progress.WithReporter(context.Background(), func(e progress.Event) {
				if e.Type == "file_result" {
					results = append(results, e.Payload.(map[string]any))
				}
			})
			file := uploadedFile{Data: []byte("not a document")}
			file.Metadata.Filename = "anything"
			file.Metadata.ContentType = c.contentType
			file.Metadata.ExamType = c.examType
			file.Metadata.Eye = "OD"
			file.Metadata.SHA256 = "00"
			extractPatientLocal(ctx, []uploadedFile{file})
			if len(results) != 1 {
				t.Fatalf("got %d file_result events, want 1", len(results))
			}
			if results[0]["status"] != c.wantStatus || results[0]["examType"] != c.wantType {
				t.Fatalf("got status=%v examType=%v", results[0]["status"], results[0]["examType"])
			}
		})
	}
}
