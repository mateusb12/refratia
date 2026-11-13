package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type tally struct{ pass, total int }

func (t tally) String() string {
	if t.total == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d (%.1f%%)", t.pass, t.total, 100*float64(t.pass)/float64(t.total))
}

func scored(r FieldRes) bool { return r.Outcome != "not_in_app" && r.Outcome != "unverified" }

func finish(run *Run, baselinePath string, update bool, out string) {
	passed := map[string]bool{}
	ran := map[string]bool{}
	for _, p := range run.Patients {
		ran[p.Slug] = true
		for _, f := range p.Fields {
			if f.Outcome == "pass" {
				passed[f.ID] = true
			}
			if f.Required() && scored(f) && f.Outcome != "pass" {
				run.Failures = append(run.Failures, fmt.Sprintf("required field %s: %s", f.ID, f.Outcome))
			}
		}
		for _, c := range p.Checks {
			if !c.OK && !c.Info && c.Group != "classification" {
				run.Failures = append(run.Failures, fmt.Sprintf("%s %s/%s: %s", p.Slug, c.Group, c.Name, c.Detail))
			}
		}
	}
	var base struct {
		Passed []string `json:"passed"`
	}
	loadJSON(baselinePath, &base)
	for _, id := range base.Passed {
		if ran[strings.SplitN(id, "|", 2)[0]] && !passed[id] {
			run.Regressions = append(run.Regressions, id)
		}
	}
	if update {
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
	}
	b, _ := json.MarshalIndent(run, "", " ")
	os.WriteFile(filepath.Join(out, "report.json"), b, 0o644)
	md := render(run)
	os.WriteFile(filepath.Join(out, "report.md"), []byte(md), 0o644)
	fmt.Println()
	fmt.Println(summaryTable(run))
	fmt.Printf("regressions: %d, failures: %d, report: %s\n", len(run.Regressions), len(run.Failures), out)
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
		if c.Info || c.Group == "classification" {
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
	w("# RefratIA benchmark report\n\nStarted %s, runtime %.0fs. Backend built from `%s`.\n\nCommand: `%s`\n\n", run.Started, run.Seconds, run.BackendDir, run.Command)
	w("Patients are referred to by folder slug. Field values are shown; names and birth dates are not.\n\n")
	w("## Summary per patient\n\n| patient | extraction accuracy | unexpected values | not in app contract | persistence + identity checks | analyze |\n|---|---|---|---|---|---|\n")
	byType, byField := map[string]*tally{}, map[string]*tally{}
	stages := map[string]int{}
	for _, p := range run.Patients {
		acc, un, na, ps := perPatient(p)
		w("| %s | %s | %d | %d | %s | %.0fs (HTTP %d) |\n", p.Slug, acc, un, na, ps, p.AnalyzeSecs, p.AnalyzeStatus)
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
	for _, outcome := range []string{"incorrect", "missing", "conflicting", "unexpected_value"} {
		w("\n## %s\n\n| field id | role | expected | got | stage |\n|---|---|---|---|---|\n", outcome)
		n := 0
		for _, p := range run.Patients {
			for _, f := range p.Fields {
				if f.Outcome == outcome {
					n++
					w("| %s | %s | %s | %s | %s (%s) |\n", f.ID, f.Role, f.Expected, f.Got, f.Stage, f.StageFrom)
				}
			}
		}
		if n == 0 {
			w("none\n")
		}
	}
	w("\n## Persistence, identity and idempotency checks\n\n| patient | group | check | result | detail |\n|---|---|---|---|---|\n")
	for _, p := range run.Patients {
		for _, c := range p.Checks {
			if c.Group == "classification" {
				continue
			}
			r := map[bool]string{true: "pass", false: "FAIL"}[c.OK]
			if c.Info && !c.OK {
				r = "info"
			}
			w("| %s | %s | %s | %s | %s |\n", p.Slug, c.Group, c.Name, r, strings.ReplaceAll(c.Detail, "|", "/"))
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
	w("\n## Regressions vs last_pass.json\n\n")
	if len(run.Regressions) == 0 {
		w("none\n")
	}
	for _, r := range run.Regressions {
		w("- %s\n", r)
	}
	w("\n## Failures that make the run exit nonzero\n\n")
	if len(run.Failures) == 0 {
		w("none\n")
	}
	for _, f := range run.Failures {
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
