package intake

import (
	"encoding/json"
	"fmt"
	"sort"

	"refratia/backend/src/features/extractionfields"
)

const fieldHintsKey = "_field_hints"

// setFieldHints stores extractor hints until validateTouchedEyes merges them
// into the file's diagnostics (and removes them from the analysis).
func setFieldHints(analysis map[string]any, examKey, eye string, hints []extractionfields.Hint) {
	store, _ := analysis[fieldHintsKey].(map[string]extractionfields.Hint)
	if store == nil {
		store = map[string]extractionfields.Hint{}
		analysis[fieldHintsKey] = store
	}
	for _, hint := range hints {
		store[examKey+"|"+eye+"|"+hint.Spec] = hint
	}
}

// snapshotEyes fingerprints every exam/eye so that we can tell which eyes a
// file touched (several files merge into one exam).
func snapshotEyes(analysis map[string]any) map[string]string {
	out := map[string]string{}
	exams, _ := analysis["exams"].(map[string]any)
	for key, raw := range exams {
		exam, _ := raw.(map[string]any)
		eyes, _ := exam["eyes"].(map[string]any)
		for eye, payload := range eyes {
			encoded, _ := json.Marshal(payload)
			out[key+"/"+eye] = string(encoded)
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
	for eyeKey, fingerprint := range after {
		if before[eyeKey] == fingerprint {
			continue
		}
		var key, eye string
		for index := len(eyeKey) - 1; index >= 0; index-- {
			if eyeKey[index] == '/' {
				key, eye = eyeKey[:index], eyeKey[index+1:]
				break
			}
		}
		touched[key] = append(touched[key], eye)
	}
	exams, _ := analysis["exams"].(map[string]any)
	hints, _ := analysis[fieldHintsKey].(map[string]extractionfields.Hint)
	delete(analysis, fieldHintsKey)
	diagnostics, _ := analysis["extraction_diagnostics"].([]any)
	keys := make([]string, 0, len(touched))
	for examKey := range touched {
		keys = append(keys, examKey)
	}
	sort.Strings(keys)
	for _, key := range keys {
		exam, _ := exams[key].(map[string]any)
		if exam == nil {
			continue
		}
		eyes := touched[key]
		sort.Strings(eyes)
		for _, diagnostic := range extractionfields.ValidateEyes(file.Metadata.SHA256, key, exam, eyes) {
			if hint, ok := hints[key+"|"+diagnostic.Eye+"|"+diagnostic.Field]; ok {
				extractionfields.ApplyHint(&diagnostic, hint)
			}
			diagnostics = append(diagnostics, diagnostic.Map())
			if diagnostic.Status == "unreadable" {
				// Enumerations and hash prefix only: no values, no names.
				fmt.Printf("extraction_field sha=%.8s exam=%s eye=%s field=%s status=%s stage=%s\n",
					diagnostic.FileSHA256, diagnostic.ExamType, diagnostic.Eye, diagnostic.Field, diagnostic.Status, diagnostic.Stage)
			}
		}
	}
	if len(diagnostics) > 0 {
		analysis["extraction_diagnostics"] = diagnostics
	}
}
