package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---- ground truth + spec (read-only inputs) ----

type FieldValue struct {
	Value        any    `json:"value"`
	Status       string `json:"status"`
	Verification string `json:"verification"`
	ProtocolRole string `json:"protocol_role"`
	Tolerance    *struct {
		Abs      float64 `json:"abs"`
		Decimals *int    `json:"decimals_printed"`
	} `json:"tolerance"`
}

type GTFile struct {
	File     string                           `json:"file"`
	ExamType string                           `json:"exam_type"`
	Eye      string                           `json:"eye"`
	Fields   map[string]map[string]FieldValue `json:"fields"`
}

type GTRecord struct {
	ExamType   string `json:"exam_type"`
	Eye        string `json:"eye"`
	SourceFile string `json:"source_file"`
}

type GT struct {
	Folder  string `json:"folder"`
	Patient struct {
		FullName  FieldValue `json:"full_name"`
		BirthDate FieldValue `json:"birth_date"`
	} `json:"patient"`
	IdentityChecks []struct {
		File   string `json:"file"`
		Result string `json:"result"`
		Method string `json:"method"`
	} `json:"identity_checks"`
	Expected struct {
		Count   int        `json:"count"`
		Records []GTRecord `json:"records"`
	} `json:"expected_exam_records"`
	Files      []GTFile `json:"files"`
	NeedsHuman []string `json:"needs_human"`
}

type SpecField struct {
	Kind     string     `json:"kind"`
	Decimals *int       `json:"decimals_printed"`
	Role     string     `json:"protocol_role"`
	AppPaths [][]string `json:"app_paths"`
}

type Spec struct {
	AppExamKey map[string]*string              `json:"app_exam_key"`
	Fields     map[string]map[string]SpecField `json:"fields"`
}

func loadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func loadGT(dir, slug string) (*GT, error) {
	g := &GT{}
	return g, loadJSON(filepath.Join(dir, slug+".json"), g)
}

// ---- scoring ----

type FieldRes struct {
	ID        string `json:"id"`
	Patient   string `json:"patient"`
	ExamType  string `json:"exam_type"`
	Eye       string `json:"eye"`
	Field     string `json:"field"`
	Role      string `json:"role"`
	Outcome   string `json:"outcome"` // pass|incorrect|missing|conflicting|unexpected_value|not_in_app|unverified
	Expected  string `json:"expected"`
	Got       string `json:"got"`
	Stage     string `json:"stage,omitempty"`
	StageFrom string `json:"stage_from,omitempty"`      // diagnostics|inferred
	Diag      string `json:"app_diagnostics,omitempty"` // app-reported status/stage; shown next to the score, never the score
}

func (f FieldRes) Required() bool {
	return f.Role == "decisive" || f.Role == "cross_check" || f.Role == "identity"
}

var numRe = regexp.MustCompile(`-?\d+(?:[.,]\d+)?`)

func walk(v any, path []string) (any, bool) {
	for _, p := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return v, true
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		return s == "" || s == "-"
	}
	return false
}

func toNums(v any) []float64 {
	switch x := v.(type) {
	case float64:
		return []float64{x}
	case string:
		var out []float64
		for _, m := range numRe.FindAllString(x, -1) {
			if f, err := strconv.ParseFloat(strings.Replace(m, ",", ".", 1), 64); err == nil {
				out = append(out, f)
			}
		}
		return out
	}
	return nil
}

var fold = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "é", "e", "ê", "e", "è", "e", "í", "i", "î", "i",
	"ó", "o", "ô", "o", "õ", "o", "ú", "u", "ü", "u", "ç", "c", "ñ", "n")

func normStr(s string) string {
	return strings.Join(strings.Fields(fold.Replace(strings.ToLower(s))), " ")
}

func show(v any) string {
	if v == nil {
		return "null"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func roundTo(x float64, dp int) float64 {
	p := math.Pow(10, float64(dp))
	return math.Round(x*p) / p
}

// compareField returns (equal, got-display).
func compareField(key string, gt FieldValue, spec SpecField, got any) bool {
	if s, ok := gt.Value.(string); ok {
		gs, _ := got.(string)
		if gs == "" {
			gs = fmt.Sprint(got)
		}
		return normStr(gs) == normStr(s)
	}
	want, ok := gt.Value.(float64)
	if !ok {
		return false
	}
	nums := toNums(got)
	if len(nums) == 0 {
		return false
	}
	g := nums[0]
	if strings.HasPrefix(key, "coma_z31") && strings.HasSuffix(key, "axis_deg") {
		if len(nums) < 2 {
			return false
		}
		g = nums[1]
	}
	dp := 0
	if spec.Decimals != nil {
		dp = *spec.Decimals
	}
	abs := 0.0
	if gt.Tolerance != nil {
		abs = gt.Tolerance.Abs
		if gt.Tolerance.Decimals != nil {
			dp = *gt.Tolerance.Decimals
		}
	}
	if abs > 0 {
		return math.Abs(g-want) <= abs+1e-9
	}
	return math.Abs(roundTo(g, dp)-roundTo(want, dp)) < 1e-9
}

type Diag struct {
	ExamType, Eye, Field, Status, Stage string
}

func parseDiags(analysis any) map[string]Diag {
	out := map[string]Diag{}
	list, _ := func() ([]any, bool) {
		v, ok := walk(analysis, []string{"extraction_diagnostics"})
		l, ok2 := v.([]any)
		return l, ok && ok2
	}()
	for _, it := range list {
		m, _ := it.(map[string]any)
		s := func(k string) string { x, _ := m[k].(string); return x }
		d := Diag{s("exam_type"), s("eye"), s("field"), s("status"), s("stage")}
		out[d.ExamType+"|"+d.Eye+"|"+d.Field] = d
	}
	return out
}

// scorePatient scores every ground-truth field of one patient against the analyze payload.
// classOK[file] reports whether the app classified that file as ground truth says.
func scorePatient(slug string, g *GT, spec *Spec, analysis any, classOK map[string]bool) []FieldRes {
	diags := parseDiags(analysis)
	hasDiags := len(diags) > 0
	var res []FieldRes
	for _, f := range g.Files {
		keyPtr := spec.AppExamKey[f.ExamType]
		for eye, fields := range f.Fields {
			for key, fv := range fields {
				sp, known := spec.Fields[f.ExamType][key]
				r := FieldRes{ID: strings.Join([]string{slug, f.ExamType, eye, key}, "|"), Patient: slug,
					ExamType: f.ExamType, Eye: eye, Field: key, Role: fv.ProtocolRole, Expected: show(fv.Value)}
				if fv.Verification != "agent_visual" {
					r.Outcome = "unverified"
					res = append(res, r)
					continue
				}
				if !known || keyPtr == nil || len(sp.AppPaths) == 0 {
					r.Outcome = "not_in_app"
					res = append(res, r)
					continue
				}
				var got any
				exists := false
				for _, p := range sp.AppPaths {
					pp := append([]string{}, p...)
					for i := range pp {
						if pp[i] == "<EYE>" {
							pp[i] = eye
						}
					}
					v, ok := walk(analysis, pp)
					if ok && !isEmpty(v) {
						got, exists = v, true
						break
					}
				}
				r.Got = "null"
				if exists {
					r.Got = show(got)
				}
				dg, hasDiag := diags[*keyPtr+"|"+eye+"|"+key]
				switch {
				case fv.Status == "valid" && fv.Value == nil:
					if !exists {
						r.Outcome = "pass"
					} else {
						r.Outcome = "incorrect"
					}
				case fv.Status == "valid":
					switch {
					case !exists && hasDiag && dg.Status == "conflicting":
						r.Outcome = "conflicting"
					case !exists:
						r.Outcome = "missing"
					case compareField(key, fv, sp, got):
						r.Outcome = "pass"
					default:
						r.Outcome = "incorrect"
					}
				default: // missing / not_applicable in ground truth
					if exists {
						r.Outcome = "unexpected_value"
					} else {
						r.Outcome = "pass"
					}
				}
				if hasDiag {
					r.Diag = dg.Status + "/" + dg.Stage
				}
				if r.Outcome != "pass" {
					switch {
					case hasDiag && dg.Stage != "":
						r.Stage, r.StageFrom = dg.Stage, "diagnostics"
					default:
						r.StageFrom = "inferred"
						_, examPresent := walk(analysis, []string{"exams", *keyPtr, "eyes", eye})
						switch {
						case !classOK[f.File]:
							r.Stage = "classification"
						case !examPresent:
							r.Stage = "mapping"
						case r.Outcome == "missing":
							r.Stage = "region"
						case r.Outcome == "unexpected_value":
							r.Stage = "validation"
						default:
							r.Stage = "recognition"
						}
					}
				}
				_ = hasDiags
				res = append(res, r)
			}
		}
	}
	sort.Slice(res, func(i, j int) bool { return res[i].ID < res[j].ID })
	return res
}

// normDate returns ISO date from ISO or DD/MM/YYYY strings ("" if unparseable).
var isoRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})`)
var dmyRe = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})/(\d{4})$`)

func normDate(s string) string {
	s = strings.TrimSpace(s)
	if m := isoRe.FindStringSubmatch(s); m != nil {
		return m[1] + "-" + m[2] + "-" + m[3]
	}
	if m := dmyRe.FindStringSubmatch(s); m != nil {
		return fmt.Sprintf("%s-%02s-%02s", m[3], m[2], m[1])
	}
	return ""
}

// ageViolations finds age/idade keys equal to 0 anywhere in the payload, or an age set while birth date is empty.
func ageViolations(analysis any) []string {
	var out []string
	birth := ""
	if v, ok := walk(analysis, []string{"patient", "birth_date"}); ok {
		birth, _ = v.(string)
	}
	var rec func(v any, path string)
	rec = func(v any, path string) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				lk := strings.ToLower(k)
				if lk == "age" || lk == "idade" || strings.HasPrefix(lk, "age_") || strings.HasPrefix(lk, "idade_") {
					if n, ok := c.(float64); ok {
						if n == 0 {
							out = append(out, path+"."+k+"=0")
						} else if strings.TrimSpace(birth) == "" {
							out = append(out, path+"."+k+" set without birth_date")
						}
					}
				}
				rec(c, path+"."+k)
			}
		case []any:
			for i, c := range x {
				rec(c, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	rec(analysis, "analysis")
	return out
}
