package patient

import (
	"reflect"
	"sort"
)

type ChangeRow struct {
	Exam   string `json:"exam"`
	Eye    string `json:"eye,omitempty"`
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
	Kind   string `json:"kind"`
}

type ChangePreview struct {
	Added     int         `json:"added"`
	Changed   int         `json:"changed"`
	Removed   int         `json:"removed"`
	Unchanged int         `json:"unchanged"`
	Rows      []ChangeRow `json:"rows"`
}

func BuildChangePreview(existing, incoming map[string]any) ChangePreview {
	preview := ChangePreview{Rows: []ChangeRow{}}
	existingExams, _ := existing["exams"].(map[string]any)
	incomingExams, _ := incoming["exams"].(map[string]any)
	exactGroups := exactReuploadGroups(
		existing["source_files"],
		incoming["source_files"],
	)

	for examKey, incomingRaw := range incomingExams {
		effectiveRaw, include := incomingExamForMerge(
			examKey,
			incomingRaw,
			exactGroups,
		)
		if !include {
			continue
		}

		incomingExam, ok := effectiveRaw.(map[string]any)
		if !ok {
			continue
		}

		existingExam, _ := existingExams[examKey].(map[string]any)
		incomingEyes, hasEyes := incomingExam["eyes"].(map[string]any)

		if !hasEyes {
			appendChangeRows(&preview, examKey, "", existingExam, incomingExam)
			continue
		}

		existingEyes, _ := existingExam["eyes"].(map[string]any)
		for eye, incomingEye := range incomingEyes {
			appendChangeRows(&preview, examKey, eye, existingEyes[eye], incomingEye)
		}
	}

	sort.Slice(preview.Rows, func(i, j int) bool {
		a, b := preview.Rows[i], preview.Rows[j]
		if a.Exam != b.Exam {
			return a.Exam < b.Exam
		}
		if a.Eye != b.Eye {
			return eyeRank(a.Eye) < eyeRank(b.Eye)
		}
		return a.Field < b.Field
	})

	return preview
}

func appendChangeRows(preview *ChangePreview, exam, eye string, before, after any) {
	beforeFields := map[string]any{}
	afterFields := map[string]any{}
	flattenPreviewFields("", before, beforeFields)
	flattenPreviewFields("", after, afterFields)

	keys := map[string]bool{}
	for key := range beforeFields {
		keys[key] = true
	}
	for key := range afterFields {
		keys[key] = true
	}

	names := make([]string, 0, len(keys))
	for key := range keys {
		names = append(names, key)
	}
	sort.Strings(names)

	for _, field := range names {
		oldValue, hadOld := beforeFields[field]
		newValue, hasNew := afterFields[field]

		kind := "unchanged"
		switch {
		case !hadOld:
			kind = "added"
			preview.Added++
		case !hasNew:
			kind = "removed"
			preview.Removed++
		case !reflect.DeepEqual(oldValue, newValue):
			kind = "changed"
			preview.Changed++
		default:
			preview.Unchanged++
		}

		preview.Rows = append(preview.Rows, ChangeRow{
			Exam: exam, Eye: eye, Field: field,
			Before: oldValue, After: newValue, Kind: kind,
		})
	}
}

func flattenPreviewFields(prefix string, value any, result map[string]any) {
	if prefix == "" && value == nil {
		return
	}

	object, ok := value.(map[string]any)
	if !ok {
		if prefix == "" {
			prefix = "value"
		}
		result[prefix] = cloneValue(value)
		return
	}

	for key, child := range object {
		if key == "source" {
			continue
		}

		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		flattenPreviewFields(path, child, result)
	}
}

func eyeRank(eye string) int {
	switch eye {
	case "OD":
		return 0
	case "OS":
		return 1
	case "AO":
		return 2
	default:
		return 3
	}
}
