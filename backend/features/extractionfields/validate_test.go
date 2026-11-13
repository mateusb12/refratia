package extractionfields

import (
	"bytes"
	"os"
	"testing"
)

func TestSpecCopyMatchesGroundTruthSpec(t *testing.T) {
	src, err := os.ReadFile("../../benchmark/groundtruth/_field_spec.json")
	if err != nil {
		t.Skip("ground truth spec not present")
	}
	if !bytes.Equal(src, specJSON) {
		t.Fatal("field_spec.json drifted from benchmark/groundtruth/_field_spec.json; copy it again")
	}
}

func eyeExam(key string, eye map[string]any) map[string]any {
	return map[string]any{"eyes": map[string]any{"OD": eye}}
}

func TestValidateRejectsOutOfDomain(t *testing.T) {
	cases := []struct {
		name string
		exam string
		eye  map[string]any
		path []string
		bad  bool
	}{
		{"axis above 180", "refractometry", map[string]any{"axis_deg": 188.0}, []string{"axis_deg"}, true},
		{"axis ok", "refractometry", map[string]any{"axis_deg": 100.0}, []string{"axis_deg"}, false},
		{"cyl off the 0.25 grid", "refractometry", map[string]any{"cylinder_d": -1.58}, []string{"cylinder_d"}, true},
		{"cyl on the grid keeps sign", "refractometry", map[string]any{"cylinder_d": -1.5}, []string{"cylinder_d"}, false},
		{"non-integer ISV", "pentacam_corneal_tomography",
			map[string]any{"topometric_indices_8mm": map[string]any{"isv": 4.5}}, []string{"topometric_indices_8mm", "isv"}, true},
		{"too many decimals", "pentacam_corneal_tomography",
			map[string]any{"anterior_cornea": map[string]any{"k1_d": 43.95}}, []string{"anterior_cornea", "k1_d"}, true},
		{"negative z31 coma impossible", "pentacam_corneal_tomography",
			map[string]any{"corneal_rings": map[string]any{"zernike": map[string]any{"5mm": map[string]any{"z31_coma": -0.2}}}},
			[]string{"corneal_rings", "zernike", "5mm", "z31_coma"}, true},
		{"negative z40 allowed", "pentacam_corneal_tomography",
			map[string]any{"cataract_preop": map[string]any{"total_corneal_z40_6mm_um": -0.173}},
			[]string{"cataract_preop", "total_corneal_z40_6mm_um"}, false},
		{"string where number", "refractometry", map[string]any{"sphere_d": "x"}, []string{"sphere_d"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exam := eyeExam(c.exam, c.eye)
			diags := ValidateEyes("sha", c.exam, exam, []string{"OD"})
			_, still := lookup(exam["eyes"].(map[string]any)["OD"].(map[string]any), c.path)
			if c.bad == still {
				t.Fatalf("bad=%v but value present=%v", c.bad, still)
			}
			var found bool
			for _, d := range diags {
				if d.Status == "unreadable" {
					found = true
					if d.Stage != "validation" || d.Error == "" {
						t.Fatalf("bad diagnostic %+v", d)
					}
				}
			}
			if found != c.bad {
				t.Fatalf("unreadable diagnostic=%v want %v", found, c.bad)
			}
		})
	}
}

func TestValidateReportsMissing(t *testing.T) {
	exam := eyeExam("refractometry", map[string]any{"sphere_d": 1.0})
	var missing, valid int
	for _, d := range ValidateEyes("sha", "refractometry", exam, []string{"OD", "OS"}) {
		if d.Eye != "OD" {
			t.Fatalf("absent eye must not produce diagnostics")
		}
		switch d.Status {
		case "missing":
			missing++
		case "valid":
			valid++
		}
	}
	if valid != 1 || missing == 0 {
		t.Fatalf("valid=%d missing=%d", valid, missing)
	}
}

func TestApplyHint(t *testing.T) {
	missing := Diagnostic{Status: "missing", Stage: "mapping"}
	ApplyHint(&missing, Hint{Status: "not_applicable", Stage: "region", Screen: "belin_ambrosio"})
	if missing.Status != "not_applicable" || missing.Stage != "region" {
		t.Fatalf("absent screen must become not_applicable: %+v", missing)
	}

	conflict := Diagnostic{Status: "missing", Stage: "mapping"}
	ApplyHint(&conflict, Hint{Status: "conflicting", Stage: "recognition", Candidates: 2})
	if conflict.Status != "conflicting" || conflict.Candidates != 2 {
		t.Fatalf("disagreeing readings must be conflicting: %+v", conflict)
	}

	// A hint never rescues a value that failed validation.
	bad := Diagnostic{Status: "unreadable", Stage: "validation"}
	ApplyHint(&bad, Hint{Status: "valid", Strategy: "label_anchor", BBox: [4]int{1, 2, 3, 4}})
	if bad.Status != "unreadable" || bad.Strategy != "label_anchor" || len(bad.BBox) != 4 {
		t.Fatalf("unexpected: %+v", bad)
	}
}
