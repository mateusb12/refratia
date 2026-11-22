package pentacam

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// shape masks the digits: patient dates must never reach test logs.
func shape(s string) string { return regexp.MustCompile(`\d`).ReplaceAllString(s, "#") }

// Real documents live in the gitignored frontend/data folder; absent -> skip.
// A date is read in full or not at all (never with a lost digit); the exam
// date may be absent on a screen that prints a single date.
func TestIdentityRealDocumentsReadFullDates(t *testing.T) {
	files, _ := filepath.Glob("../../../../../frontend/data/*/PENTACAM*.pdf")
	if len(files) == 0 {
		t.Skip("no real Pentacam documents available")
	}
	full := regexp.MustCompile(`^\d{1,2}/\d{1,2}/\d{4}$`)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		slug := filepath.Base(filepath.Dir(f)) + "/" + filepath.Base(f)[:11]
		id, err := ExtractIdentity(context.Background(), data)
		if err != nil {
			t.Errorf("%s: %v", slug, err)
			continue
		}
		t.Logf("%s birth=%s exam=%s issue=%q", slug, shape(id.BirthDateRaw), shape(id.ExamDateRaw), id.BirthDateIssue)
		if id.BirthDateIssue != "" || !full.MatchString(id.BirthDateRaw) || (id.ExamDateRaw != "" && !full.MatchString(id.ExamDateRaw)) {
			t.Errorf("%s: unreadable or malformed identity dates", slug)
		}
	}
}
