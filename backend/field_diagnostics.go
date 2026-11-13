package main

import (
	"encoding/json"
	"fmt"
	"sort"

	"refratia/backend/features/extractionfields"
)

// snapshotEyes fingerprints every exam/eye so that we can tell which eyes a
// file touched (several files merge into one exam).
func snapshotEyes(analysis map[string]any) map[string]string {
	out := map[string]string{}
	exams, _ := analysis["exams"].(map[string]any)
	for key, raw := range exams {
		exam, _ := raw.(map[string]any)
		eyes, _ := exam["eyes"].(map[string]any)
		for eye, payload := range eyes {
			b, _ := json.Marshal(payload)
			out[key+"/"+eye] = string(b)
		}
	}
	return out
}

// validateTouchedEyes validates, in place, the exam eyes this file added or
// changed since `before`, and appends the C4 diagnostics to
// analysis["extraction_diagnostics"]. It must run before the file's result is
// emitted or checkpointed, so invalid values never leave the extractor.
func validateTouchedEyes(analysis map[string]any, file uploadedFile, before map[string]string) {
	after := snapshotEyes(analysis)
	touched := map[string][]string{}
	for k, v := range after {
		if before[k] == v {
			continue
		}
		var key, eye string
		for i := len(k) - 1; i >= 0; i-- {
			if k[i] == '/' {
				key, eye = k[:i], k[i+1:]
				break
			}
		}
		touched[key] = append(touched[key], eye)
	}
	exams, _ := analysis["exams"].(map[string]any)
	diagnostics, _ := analysis["extraction_diagnostics"].([]any)
	keys := make([]string, 0, len(touched))
	for k := range touched {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		exam, _ := exams[key].(map[string]any)
		if exam == nil {
			continue
		}
		eyes := touched[key]
		sort.Strings(eyes)
		for _, d := range extractionfields.ValidateEyes(file.Metadata.SHA256, key, exam, eyes) {
			diagnostics = append(diagnostics, d.Map())
			if d.Status == "unreadable" {
				// Enumerations and hash prefix only: no values, no names.
				fmt.Printf("extraction_field sha=%.8s exam=%s eye=%s field=%s status=%s stage=%s\n",
					d.FileSHA256, d.ExamType, d.Eye, d.Field, d.Status, d.Stage)
			}
		}
	}
	if len(diagnostics) > 0 {
		analysis["extraction_diagnostics"] = diagnostics
	}
}
