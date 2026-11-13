// Package extractionfields validates extracted clinical values against the
// per-field domain described by field_spec.json (plausible range, printed
// decimals, integer-ness, device step) and emits the C4 per-field diagnostics.
//
// field_spec.json is a copy of backend/benchmark/groundtruth/_field_spec.json
// (embed cannot reach across packages); spec_test.go fails if they drift.
package extractionfields

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

//go:embed field_spec.json
var specJSON []byte

type fieldSpec struct {
	Kind            string      `json:"kind"`
	DecimalsPrinted *int        `json:"decimals_printed"`
	PlausibleRange  *[2]float64 `json:"plausible_range"`
	AppPaths        [][]string  `json:"app_paths"`
}

type specFile struct {
	Fields map[string]map[string]fieldSpec `json:"fields"`
}

// deviceStep holds domain rules that the spec does not carry: the grid on
// which a device prints a value (refraction is printed in 0.25 D steps).
var deviceStep = map[string]float64{
	"refractometry/sphere_d":   0.25,
	"refractometry/cylinder_d": 0.25,
}

// Diagnostic is one C4 entry. Identity fields never appear here, so RawText
// (the offending value, clinical only) is allowed; Error is always templated.
type Diagnostic struct {
	FileSHA256 string `json:"file_sha256"`
	ExamType   string `json:"exam_type"`
	Eye        string `json:"eye"`
	Field      string `json:"field"`
	Status     string `json:"status"`
	Stage      string `json:"stage"`
	Page       int    `json:"page,omitempty"`
	Screen     string `json:"screen,omitempty"`
	Strategy   string `json:"strategy,omitempty"`
	BBox       []int  `json:"bbox,omitempty"`
	BBoxUnits  string `json:"bbox_units,omitempty"`
	Votes      int    `json:"votes,omitempty"`
	Candidates int    `json:"candidates,omitempty"`
	RawText    string `json:"raw_text,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Map renders the diagnostic in the shape stored inside the analysis payload.
func (d Diagnostic) Map() map[string]any {
	raw, _ := json.Marshal(d)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

type located struct {
	field string
	spec  fieldSpec
	path  []string // below eyes/<EYE>
}

var locations = buildLocations()

func buildLocations() map[string][]located {
	var sf specFile
	if err := json.Unmarshal(specJSON, &sf); err != nil {
		panic("extractionfields: bad field_spec.json: " + err.Error())
	}
	out := map[string][]located{}
	for examKey, fields := range sf.Fields {
		byPath := map[string]located{}
		for name, spec := range fields {
			for _, p := range spec.AppPaths {
				// ["exams", key, "eyes", "<EYE>", ...rest]
				if len(p) < 5 || p[2] != "eyes" || p[3] != "<EYE>" {
					continue
				}
				rest := p[4:]
				id := strings.Join(rest, "/")
				// Several spec fields may share one app path (a compound
				// value): the one with the most printed decimals is the value.
				if prev, ok := byPath[id]; ok && decimals(prev.spec) >= decimals(spec) {
					continue
				}
				byPath[id] = located{field: name, spec: spec, path: rest}
			}
		}
		list := make([]located, 0, len(byPath))
		for _, l := range byPath {
			list = append(list, l)
		}
		sort.Slice(list, func(i, j int) bool {
			return strings.Join(list[i].path, "/") < strings.Join(list[j].path, "/")
		})
		out[examKey] = list
	}
	return out
}

func decimals(s fieldSpec) int {
	if s.DecimalsPrinted == nil {
		return -1
	}
	return *s.DecimalsPrinted
}

func lookup(root map[string]any, path []string) (any, bool) {
	var cur any = root
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[k]; !ok {
			return nil, false
		}
	}
	return cur, true
}

func remove(root map[string]any, path []string) {
	cur := root
	for _, k := range path[:len(path)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return
		}
		cur = next
	}
	delete(cur, path[len(path)-1])
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func onGrid(v, step float64) bool {
	q := v / step
	return math.Abs(q-math.Round(q)) < 1e-6
}

// check returns "" when the value is acceptable, otherwise a templated error.
func check(examKey string, l located, v any) string {
	switch l.spec.Kind {
	case "num", "int":
		n, ok := toFloat(v)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return "tipo de valor inesperado para o campo"
		}
		if r := l.spec.PlausibleRange; r != nil && (n < r[0] || n > r[1]) {
			return "valor fora do intervalo plausível do campo"
		}
		d := decimals(l.spec)
		if l.spec.Kind == "int" {
			d = 0
		}
		if d >= 0 {
			scaled := n * math.Pow(10, float64(d))
			if math.Abs(scaled-math.Round(scaled)) > 1e-6 {
				return "valor incompatível com as casas decimais impressas"
			}
		}
		if step, ok := deviceStep[examKey+"/"+l.field]; ok && !onGrid(n, step) {
			return "valor fora do passo impresso pelo dispositivo"
		}
	case "str":
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			return "texto vazio"
		}
	}
	return ""
}

// ValidateEyes validates the listed eyes of exam (the value of
// analysis.exams[examKey]) in place: values that fail validation are removed
// (never coerced) and reported as unreadable/validation. Every spec field of
// the exam gets exactly one diagnostic per eye: valid, missing or unreadable.
func ValidateEyes(fileSHA256, examKey string, exam map[string]any, eyeNames []string) []Diagnostic {
	locs := locations[examKey]
	eyes, _ := exam["eyes"].(map[string]any)
	var out []Diagnostic
	for _, eyeName := range eyeNames {
		eye, _ := eyes[eyeName].(map[string]any)
		if eye == nil {
			continue
		}
		for _, l := range locs {
			d := Diagnostic{FileSHA256: fileSHA256, ExamType: examKey, Eye: eyeName, Field: l.field}
			v, present := lookup(eye, l.path)
			if !present || v == nil {
				d.Status, d.Stage = "missing", "mapping"
			} else if msg := check(examKey, l, v); msg != "" {
				d.Status, d.Stage, d.Error = "unreadable", "validation", msg
				d.RawText = fmt.Sprint(v)
				remove(eye, l.path)
			} else {
				d.Status, d.Stage = "valid", "validation"
			}
			out = append(out, d)
		}
	}
	return out
}

// Rule is the printed format of one field, for extractors that need to
// normalise raw OCR text (how many decimals the device prints, the plausible
// range, whether it is an integer).
type Rule struct {
	Kind     string // "num" | "int" | "str" | "date"
	Decimals int    // printed decimals; 0 for int, -1 when unknown
	Min, Max float64
	HasRange bool
}

// RuleFor returns the printed-format rule of a spec field.
func RuleFor(examKey, field string) (Rule, bool) {
	for _, l := range locations[examKey] {
		if l.field != field {
			continue
		}
		r := Rule{Kind: l.spec.Kind, Decimals: decimals(l.spec)}
		if l.spec.Kind == "int" {
			r.Decimals = 0
		}
		if l.spec.PlausibleRange != nil {
			r.Min, r.Max, r.HasRange = l.spec.PlausibleRange[0], l.spec.PlausibleRange[1], true
		}
		return r, true
	}
	return Rule{}, false
}

// Hint is what an extractor knows about how it read (or why it could not read)
// a field; ApplyHint merges it into the validation diagnostic of that field.
type Hint struct {
	Spec       string // field name in _field_spec.json
	Status     string // "valid" | "not_applicable" (others come from validation)
	Stage      string
	Screen     string
	Page       int
	Strategy   string
	BBox       [4]int // x, y, w, h in px at 300 dpi
	Votes      int
	Candidates int
}

// ApplyHint enriches d with the extractor's hint. A hint can turn a missing
// field into not_applicable (its screen is absent from the report); it never
// turns a failed validation into a success.
func ApplyHint(d *Diagnostic, h Hint) {
	switch {
	case h.Status == "not_applicable" && d.Status == "missing":
		d.Status, d.Stage = "not_applicable", h.Stage
		d.Screen = h.Screen
	case d.Status == "valid" || d.Status == "unreadable":
		d.Screen, d.Page, d.Strategy = h.Screen, h.Page, h.Strategy
		d.Votes, d.Candidates = h.Votes, h.Candidates
		if h.BBox != [4]int{} {
			d.BBox, d.BBoxUnits = h.BBox[:], "px@300dpi"
		}
		if d.Status == "unreadable" {
			d.Stage = "validation"
		}
	}
}
