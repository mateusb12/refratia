package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type tally struct{ pass, total int }

func (t tally) String() string {
	if t.total == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d (%.1f%%)", t.pass, t.total, 100*float64(t.pass)/float64(t.total))
}

type CmpField struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Prev     string `json:"previous"`
	Now      string `json:"now"`
}

type Compare struct {
	Against string                    `json:"against"`
	Partial bool                      `json:"partial"`
	Counts  map[string]map[string]int `json:"counts"` // scope (all, patient:x, type:y) -> category -> n
	Fields  []CmpField                `json:"fields"` // everything except unchanged passes
}

func cmpCategory(prev, now string) string {
	switch {
	case prev == "pass" && now == "pass":
		return "still_pass"
	case prev == "pass":
		return "regressed"
	case now == "pass":
		return "fixed"
	case now == "unverified" && prev != "unverified":
		return "newly_unverified"
	case now == "missing":
		return "still_missing"
	}
	return "still_wrong"
}

func compareRuns(run *Run, prevPath string) *Compare {
	prev := &Run{}
	if loadJSON(prevPath, prev) != nil {
		return nil
	}
	po := map[string]string{}
	for _, p := range prev.Patients {
		for _, f := range p.Fields {
			po[f.ID] = f.Outcome
		}
	}
	c := &Compare{Against: prevPath, Counts: map[string]map[string]int{}}
	bump := func(scope, cat string) {
		if c.Counts[scope] == nil {
			c.Counts[scope] = map[string]int{}
		}
		c.Counts[scope][cat]++
	}
	for _, p := range run.Patients {
		for _, f := range p.Fields {
			pv, ok := po[f.ID]
			if !ok || pv == "not_in_app" || f.Outcome == "not_in_app" {
				continue
			}
			if pv == "contract_mismatch" { // a field the app did not emit before counts as missing, so newly emitted fields show as fixed
				pv = "missing"
			}
			if f.Outcome == "contract_mismatch" {
				f.Outcome = "missing"
			}
			cat := cmpCategory(pv, f.Outcome)
			for _, sc := range []string{"all", "patient:" + p.Slug, "type:" + f.ExamType} {
				bump(sc, cat)
			}
			if cat != "still_pass" {
				c.Fields = append(c.Fields, CmpField{f.ID, cat, pv, f.Outcome})
			}
		}
	}
	return c
}

// latestPrevious returns the newest complete report.json in outBase other than current.
func latestPrevious(outBase, current string) string {
	dirs, _ := filepath.Glob(filepath.Join(outBase, "*", "report.json"))
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, p := range dirs {
		if filepath.Dir(p) == current {
			continue
		}
		var m map[string]any
		if loadJSON(p, &m) != nil {
			continue
		}
		if c, ok := m["complete"].(bool); ok && !c {
			continue // old reports without the key are complete runs
		}
		if m["patients"] == nil {
			continue
		}
		return p
	}
	return ""
}

func writeReports(run *Run, out string) {
	if out == "" {
		return
	}
	b, _ := json.MarshalIndent(run, "", " ")
	os.WriteFile(filepath.Join(out, "report.json"), b, 0o644)
	os.WriteFile(filepath.Join(out, "report.md"), []byte(render(run)), 0o644)
}

// conclude finalizes the run: failures by kind, regressions, completeness, reports, exit code (0 ok, 1 app failures, 2 infra/incomplete).
func conclude(run *Run, e *env, baselinePath string, update bool, compareArg, outBase string, t0 time.Time, final bool) int {
	markContractMismatch(run.Patients)
	run.Seconds = time.Since(t0).Seconds()
	passed := map[string]bool{}
	ran := map[string]bool{}
	run.Failures, run.Why = nil, nil
	run.Failures = nil
	infra := append([]string{}, run.RunInfra...)
	for _, p := range run.Patients {
		ran[p.Slug] = true
		if p.Incomplete != "" {
			run.Why = append(run.Why, p.Slug+": "+p.Incomplete)
		}
		for _, f := range p.Fields {
			if f.Outcome == "pass" {
				passed[f.ID] = true
			}
			if f.Required() && scored(f) && f.Outcome != "pass" {
				run.Failures = append(run.Failures, fmt.Sprintf("required field %s: %s", f.ID, f.Outcome))
			}
		}
		for _, c := range p.Checks {
			if c.OK || c.Info || c.NE || c.Group == "classification" {
				continue
			}
			msg := fmt.Sprintf("%s %s/%s: %s", p.Slug, c.Group, c.Name, c.Detail)
			if c.Kind == "infra_failure" {
				infra = append(infra, msg)
			} else {
				run.Failures = append(run.Failures, msg)
			}
		}
	}
	run.InfraFails = infra
	for _, s := range run.Requested {
		if !ran[s] {
			if final {
				run.InfraFails = append(run.InfraFails, s+": not run")
				run.Why = append(run.Why, s+": not run")
			} else {
				run.Why = append(run.Why, s+": pending")
			}
		}
	}
	for _, f := range run.InfraFails {
		if !strings.HasSuffix(f, ": not run") {
			run.Why = append(run.Why, "infra: "+f)
		}
	}
	run.Final = final
	run.Complete = len(run.Why) == 0

	run.Regressions = nil
	var base struct {
		Passed []string `json:"passed"`
	}
	loadJSON(baselinePath, &base)
	for _, id := range base.Passed {
		if ran[strings.SplitN(id, "|", 2)[0]] && !passed[id] {
			run.Regressions = append(run.Regressions, id+" (last_pass.json)")
		}
	}
	if compareArg != "-" {
		prev := compareArg
		if prev == "" {
			prev = latestPrevious(outBase, e.out)
		} else if st, err := os.Stat(prev); err == nil && st.IsDir() {
			prev = filepath.Join(prev, "report.json")
		}
		if prev != "" {
			run.Compare = compareRuns(run, prev)
			if run.Compare != nil {
				run.Compare.Partial = !final || len(run.Why) > 0
			}
			if run.Compare == nil {
				run.InfraFails = append(run.InfraFails, "compare: cannot read previous report "+prev)
				run.Why = append(run.Why, "infra: compare: cannot read previous report "+prev)
				run.Complete = false
			} else {
				for _, f := range run.Compare.Fields {
					if f.Category == "regressed" {
						run.Regressions = append(run.Regressions, f.ID+" (vs previous run)")
					}
				}
			}
		}
	}
	if !final {
		writeReports(run, e.out)
		return 0
	}
	if update && run.Complete {
		set := map[string]bool{}
		for _, id := range base.Passed {
			if !ran[strings.SplitN(id, "|", 2)[0]] {
				set[id] = true
			}
		}
		for id := range passed {
			set[id] = true
		}
		ids := keys(set)
		b, _ := json.MarshalIndent(map[string]any{"passed": ids}, "", " ")
		os.WriteFile(baselinePath, append(b, '\n'), 0o644)
		fmt.Printf("baseline rewritten: %d passing fields -> %s\n", len(ids), baselinePath)
		run.Regressions = nil
	} else if update {
		fmt.Println("baseline NOT rewritten: run is incomplete")
	}
	writeReports(run, e.out)
	saveJSON(filepath.Join(e.out, "state.json"), map[string]any{"pid": os.Getpid(), "state": "finished", "updated": time.Now().Format(time.RFC3339)})
	fmt.Println()
	fmt.Println(summaryTable(run))
	status := "COMPLETE"
	if !run.Complete {
		status = "INCOMPLETE (" + strings.Join(run.Why, "; ") + ")"
	}
	fmt.Printf("run %s; app failures: %d, regressions: %d, infra failures: %d; report: %s\n", status, len(run.Failures), len(run.Regressions), len(run.InfraFails), e.out)
	switch {
	case !run.Complete:
		return 2
	case len(run.Failures)+len(run.Regressions) > 0:
		return 1
	}
	return 0
}

// rescoreRun re-scores an earlier run offline: fields and identity from the saved analyze responses; persistence
// checks are carried over (SQL/Garage are not re-queried) with the current duplicate / not_evaluable rules applied.
func (e *env) rescoreRun(dir string, run *Run) *Run {
	old := &Run{}
	must(loadJSON(filepath.Join(dir, "report.json"), old))
	run.Note = "Rescored offline from " + filepath.Base(dir) + ": field and identity scoring recomputed from raw analyze responses; persistence/idempotency re-evaluated from saved SQL/Garage snapshots when present (raw/<slug>.snapshots.json), otherwise carried over from the live run with the current rules applied (marker_exists then not re-evaluated)."
	run.BackendDir = old.BackendDir
	run.Started = old.Started
	for _, p := range old.Patients {
		np := *p
		np.Checks = nil
		var payload map[string]any
		raw, err := os.ReadFile(filepath.Join(dir, "raw", p.Slug+".analyze.json"))
		g, gerr := loadGT(e.gtDir, p.Slug)
		if err != nil || gerr != nil || json.Unmarshal(raw, &payload) != nil {
			np.Incomplete = "rescore: raw analyze response or ground truth unreadable"
			run.Patients = append(run.Patients, &np)
			continue
		}
		classOK := map[string]bool{}
		for _, f := range p.Files {
			classOK[f.File] = f.OK
		}
		analysis := payload["analysis"]
		np.Fields = scorePatient(p.Slug, g, e.spec, analysis, classOK)
		if sb, err := os.ReadFile(filepath.Join(dir, "raw", p.Slug+".snapshots.json")); err == nil {
			sn := &Snaps{}
			if json.Unmarshal(sb, sn) == nil && sn.Post1 != nil {
				for _, c := range p.Checks { // keep non-persistence, non-identity checks (classification, harness)
					if c.Group != "identity" && c.Group != "persistence" && c.Group != "idempotency" {
						np.Checks = append(np.Checks, c)
					}
				}
				cks, ch := evalPersistence(g, sn)
				np.Checks = append(np.Checks, cks...)
				np.Changes = ch
				np.Checks = append(np.Checks, identityChecks(g, analysis)...)
				b, _ := json.MarshalIndent(&np, "", " ")
				os.WriteFile(filepath.Join(e.out, "patients", p.Slug+".json"), b, 0o644)
				run.Patients = append(run.Patients, &np)
				continue
			}
		}
		zeroRows := false
		for _, c := range p.Checks {
			if c.Name == "exam_row_count" && strings.HasPrefix(c.Detail, "0 rows") {
				zeroRows = true
			}
		}
		for _, c := range p.Checks {
			switch {
			case c.Group == "identity":
				continue
			case zeroRows && (c.Name == "exam_rows_match_expected" || c.Name == "exam_patient_id_matches_patient" || c.Name == "case_id_consistent" || c.Name == "exam_objects_exist_with_size"):
				c = Check{Group: c.Group, Name: c.Name, NE: true, Detail: "no exam rows; see exam_row_count"}
			case c.Name == "marker_exists":
				c = Check{Group: c.Group, Name: c.Name, NE: true, Detail: "not re-evaluated in rescore (Garage not re-queried)"}
			case c.Name == "reingest_no_duplicates":
				c.OK = strings.HasPrefix(c.Detail, "HTTP 200") && !structural(p.Changes)
			}
			if c.Kind == "" {
				c.Kind = "app_failure"
			}
			np.Checks = append(np.Checks, c)
		}
		np.Checks = append(np.Checks, identityChecks(g, analysis)...)
		b, _ := json.MarshalIndent(&np, "", " ")
		os.WriteFile(filepath.Join(e.out, "patients", p.Slug+".json"), b, 0o644)
		run.Patients = append(run.Patients, &np)
	}
	return run
}

func perPatient(p *PatientRun) (acc tally, unexpected, notInApp int, persist tally) {
	for _, f := range p.Fields {
		switch {
		case f.Outcome == "not_in_app":
			notInApp++
		case scored(f):
			acc.total++
			if f.Outcome == "pass" {
				acc.pass++
			}
			if f.Outcome == "unexpected_value" {
				unexpected++
			}
		}
	}
	for _, c := range p.Checks {
		if c.Info || c.NE || c.Group == "classification" {
			continue
		}
		persist.total++
		if c.OK {
			persist.pass++
		}
	}
	return
}

func summaryTable(run *Run) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-18s %-20s %-10s %-8s %-14s\n", "patient", "extraction", "unexpected", "not_in_app", "persist+ident")
	for _, p := range run.Patients {
		acc, un, na, ps := perPatient(p)
		fmt.Fprintf(&b, "%-18s %-20s %-10d %-8d %-14s\n", p.Slug, acc, un, na, ps)
	}
	return b.String()
}

func render(run *Run) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }
	w("# RefratIA benchmark report\n\n")
	if !run.Final {
		w("**Run IN PROGRESS** (this file is rewritten after every patient; if it stops changing and state.json says running with a dead pid, the run died): %s\n\n", strings.Join(run.Why, "; "))
	} else if run.Complete {
		w("**Run COMPLETE**: all %d requested patients evaluated, no infra failures.\n\n", len(run.Requested))
	} else {
		w("**Run INCOMPLETE**: %s\n\n", strings.Join(run.Why, "; "))
	}
	if run.Note != "" {
		w("%s\n\n", run.Note)
	}
	w("Commit `%s`. Started %s, runtime %.0fs. Backend built from `%s`.\n\nCommand: `%s`\n\n", run.Commit, run.Started, run.Seconds, run.BackendDir, run.Command)
	w("Outcome categories: pass; incorrect (value differs from the printed one); missing (app path present, value null); conflicting (app reported conflicting readings and gave no value); unexpected_value (app value where ground truth is missing/not_applicable); contract_mismatch (the field's app path exists in no patient's output: the app never emits it; counts as a FAILURE in the accuracy denominator, listed separately to tell contract gaps from OCR misses); unverified (needs_human: excluded, never a pass); not_in_app (the spec defines no app path at all: excluded from accuracy, its count is shown next to it).\n\n")
	w("Patients are referred to by folder slug. Field values are shown; names and birth dates are not.\n\n")
	w("## Summary per patient\n\n| patient | extraction accuracy | unexpected values | not_in_app (excluded) | contract_mismatch (in denominator) | persistence + identity checks | analyze |\n|---|---|---|---|---|---|---|\n")
	byType, byField := map[string]*tally{}, map[string]*tally{}
	stages := map[string]int{}
	for _, p := range run.Patients {
		acc, un, na, ps := perPatient(p)
		cm := 0
		for _, f := range p.Fields {
			if f.Outcome == "contract_mismatch" {
				cm++
			}
		}
		w("| %s | %s | %d | %d | %d | %s | %.0fs (HTTP %d) |\n", p.Slug, acc, un, na, cm, ps, p.AnalyzeSecs, p.AnalyzeStatus)
		for _, f := range p.Fields {
			if !scored(f) {
				continue
			}
			for k, m := range map[string]map[string]*tally{f.ExamType: byType, f.ExamType + " / " + f.Field: byField} {
				if m[k] == nil {
					m[k] = &tally{}
				}
				m[k].total++
				if f.Outcome == "pass" {
					m[k].pass++
				}
			}
			if f.Outcome != "pass" {
				stages[f.Stage+" ("+f.StageFrom+")"]++
			}
		}
	}
	var all, req tally
	notIn := 0
	for _, p := range run.Patients {
		for _, f := range p.Fields {
			if f.Outcome == "not_in_app" {
				notIn++
			}
			if scored(f) {
				all.total++
				if f.Outcome == "pass" {
					all.pass++
				}
				if f.Required() {
					req.total++
					if f.Outcome == "pass" {
						req.pass++
					}
				}
			}
		}
	}
	w("\n**Overall accuracy** (pass / all scored fields; contract_mismatch counts as non-pass): %s\n\n**Required roles** (decisive, cross_check, identity): %s\n\n**Coverage**: %d fields have no app path in the spec (not_in_app, excluded from accuracy).\n", all, req, notIn)
	section := func(title string, m map[string]*tally) {
		w("\n## %s\n\n| key | accuracy |\n|---|---|\n", title)
		for _, k := range sortedKeys(m) {
			w("| %s | %s |\n", k, m[k])
		}
	}
	section("Accuracy per exam type", byType)
	w("\n## Failures by stage\n\n| stage | count |\n|---|---|\n")
	for _, k := range sortedInts(stages) {
		w("| %s | %d |\n", k, stages[k])
	}
	for _, outcome := range []string{"incorrect", "missing", "conflicting", "unexpected_value", "contract_mismatch"} {
		w("\n## %s\n\n| field id | role | expected | got | stage | app diagnostics (not the score) |\n|---|---|---|---|---|---|\n", outcome)
		n := 0
		for _, p := range run.Patients {
			for _, f := range p.Fields {
				if f.Outcome == outcome {
					n++
					w("| %s | %s | %s | %s | %s (%s) | %s |\n", f.ID, f.Role, f.Expected, f.Got, f.Stage, f.StageFrom, f.Diag)
				}
			}
		}
		if n == 0 {
			w("none\n")
		}
	}
	w("\n## Persistence, identity and idempotency checks\n\n| patient | group | check | result | kind | detail |\n|---|---|---|---|---|---|\n")
	for _, p := range run.Patients {
		for _, c := range p.Checks {
			if c.Group == "classification" {
				continue
			}
			r := map[bool]string{true: "pass", false: "FAIL"}[c.OK]
			if c.Info && !c.OK {
				r = "info"
			}
			if c.NE {
				r = "not_evaluable"
			}
			k := ""
			if !c.OK && !c.NE && !c.Info {
				k = c.Kind
			}
			w("| %s | %s | %s | %s | %s | %s |\n", p.Slug, c.Group, c.Name, r, k, strings.ReplaceAll(c.Detail, "|", "/"))
		}
	}
	w("\n## Classification (informational)\n\n| patient | file | source | recognized | expected | ok |\n|---|---|---|---|---|---|\n")
	for _, p := range run.Patients {
		for _, f := range p.Files {
			w("| %s | %s | %s | %s/%s | %s/%s | %v |\n", p.Slug, strings.ReplaceAll(f.File, "|", "/"), f.Source, f.RecType, f.RecEye, f.GTType, f.GTEye, f.OK)
		}
	}
	w("\n## Unverified (needs_human, never counted as pass)\n\n")
	for _, p := range run.Patients {
		for _, u := range p.Unverified {
			w("- %s: %s\n", p.Slug, u)
		}
	}
	nia := 0
	for _, p := range run.Patients {
		for _, f := range p.Fields {
			if f.Outcome == "not_in_app" {
				nia++
			}
		}
	}
	w("\n%d ground-truth fields have no app JSON path (not scored; see _field_spec.json).\n", nia)
	if run.Compare != nil {
		w("\n## Comparison with previous run\n\nAgainst `%s`%s. Categories: fixed (fail to pass), regressed (pass to fail), still_wrong, still_missing, newly_unverified.\n\n| scope | still_pass | fixed | regressed | still_wrong | still_missing | newly_unverified |\n|---|---|---|---|---|---|---|\n", filepath.Base(filepath.Dir(run.Compare.Against)), map[bool]string{true: " (PARTIAL: only the patients finished so far)", false: ""}[run.Compare.Partial])
		var sc []string
		for k := range run.Compare.Counts {
			sc = append(sc, k)
		}
		sort.Strings(sc)
		for _, k := range sc {
			c := run.Compare.Counts[k]
			w("| %s | %d | %d | %d | %d | %d | %d |\n", k, c["still_pass"], c["fixed"], c["regressed"], c["still_wrong"], c["still_missing"], c["newly_unverified"])
		}
		w("\n| field id | category | previous | now |\n|---|---|---|---|\n")
		for _, f := range run.Compare.Fields {
			if f.Category == "fixed" || f.Category == "regressed" || f.Category == "newly_unverified" {
				w("| %s | %s | %s | %s |\n", f.ID, f.Category, f.Prev, f.Now)
			}
		}
		w("\n(still_wrong and still_missing field lists are in report.json under compare.fields.)\n")
	}
	w("\n## Regressions\n\n")
	if len(run.Regressions) == 0 {
		w("none\n")
	}
	for _, r := range run.Regressions {
		w("- %s\n", r)
	}
	w("\n## App failures (exit 1)\n\n")
	if len(run.Failures) == 0 {
		w("none\n")
	}
	for _, f := range run.Failures {
		w("- %s\n", f)
	}
	w("\n## Infra failures (exit 2)\n\n")
	if len(run.InfraFails) == 0 {
		w("none\n")
	}
	for _, f := range run.InfraFails {
		w("- %s\n", f)
	}
	section("Accuracy per field", byField)
	return b.String()
}

func sortedKeys(m map[string]*tally) []string {
	var o []string
	for k := range m {
		o = append(o, k)
	}
	sort.Strings(o)
	return o
}

func sortedInts(m map[string]int) []string {
	var o []string
	for k := range m {
		o = append(o, k)
	}
	sort.Slice(o, func(i, j int) bool { return m[o[i]] > m[o[j]] || m[o[i]] == m[o[j]] && o[i] < o[j] })
	return o
}
