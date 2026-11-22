package eyesuite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gtAxes collects k1/k2 axis values per eye from a ground-truth file,
// wherever the eye blocks sit in the document.
func gtAxes(node any, out map[string]map[string]float64) {
	switch v := node.(type) {
	case map[string]any:
		for eye, child := range v {
			if m, ok := child.(map[string]any); ok && (eye == "OD" || eye == "OS") {
				for _, key := range []string{"k1_axis_deg", "k2_axis_deg"} {
					if f, ok := m[key].(map[string]any); ok {
						if val, ok := f["value"].(float64); ok {
							if out[eye] == nil {
								out[eye] = map[string]float64{}
							}
							out[eye][key] = val
						}
					}
				}
			}
			gtAxes(child, out)
		}
	case []any:
		for _, child := range v {
			gtAxes(child, out)
		}
	}
}

func readGT(t *testing.T, path string) map[string]map[string]float64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no ground truth %s", path)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]float64{}
	gtAxes(doc, out)
	return out
}

func extractBiometry(t *testing.T, folder string) map[string]any {
	files, _ := filepath.Glob("../../../../../frontend/data/" + folder + "/*.pdf")
	for _, f := range files {
		n := strings.ToUpper(filepath.Base(f))
		if !strings.Contains(n, "BIO") && !strings.Contains(n, "EYESUITE") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		res, err := ExtractPDF(context.Background(), data)
		if err != nil {
			t.Fatalf("%s: %v", folder, err)
		}
		return res.Exam["eyes"].(map[string]any)
	}
	t.Skipf("no biometry document for %s", folder)
	return nil
}

func checkAxes(t *testing.T, folder string, want map[string]map[string]float64) {
	eyes := extractBiometry(t, folder)
	for eye, keys := range want {
		k, _ := eyes[eye].(map[string]any)["keratometry"].(map[string]any)
		for key, w := range keys {
			if got, ok := k[key].(float64); !ok || got != w {
				t.Errorf("%s %s %s: got %v (present=%v), want %v", folder, eye, key, got, ok, w)
			}
		}
	}
}

func TestKAxesRolmey(t *testing.T) {
	checkAxes(t, "rolmey", map[string]map[string]float64{
		"OD": {"k1_axis_deg": 54, "k2_axis_deg": 144},
		"OS": {"k1_axis_deg": 3, "k2_axis_deg": 93},
	})
}

func TestKAxesMatchGroundTruth(t *testing.T) {
	root := "../../../../../"
	for folder, gt := range map[string]string{
		"creso-aversa": root + "backend/benchmark/groundtruth/creso-aversa.json",
		"francisco":    root + "backend/benchmark/groundtruth/francisco.json",
		"maria-gorete": root + "backend/benchmark/groundtruth/maria-gorete.json",
		"ademildo":     root + "frontend/data/_gt-local/ademildo.json",
	} {
		t.Run(folder, func(t *testing.T) {
			want := readGT(t, gt)
			if len(want) == 0 {
				t.Skip("no axis ground truth")
			}
			checkAxes(t, folder, want)
		})
	}
}
