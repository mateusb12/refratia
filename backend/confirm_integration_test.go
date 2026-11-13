package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"refratia/backend/database"
	intakefeature "refratia/backend/features/intake"
)

// Integration tests against testinfra (Postgres refratia_test + Garage
// refratia-test). Skipped unless REFRATIA_NO_DOTENV is set, so they can never
// run against the developer .env (which points at production).
func integrationSetup(t *testing.T) (context.Context, fileRepository, string) {
	t.Helper()
	if os.Getenv("REFRATIA_NO_DOTENV") == "" || !strings.HasSuffix(os.Getenv("BUCKET_NAME"), "-test") {
		t.Skip("integration test: source backend/testinfra/test.env first")
	}
	ctx := context.Background()
	db, err := database.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	appDatabase = db
	client, err := storageClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, client, os.Getenv("BUCKET_NAME")
}

type draftDoc struct {
	filename, label, eye string
}

var testDocs = []draftDoc{
	{"EYESUITE__AO__x__aaaaaaaa__1.pdf", "EYESUITE", "AO"},
	{"PENTACAM__OD__x__bbbbbbbb__1.pdf", "PENTACAM", "OD"},
	{"PENTACAM__OS__x__cccccccc__1.pdf", "PENTACAM", "OS"},
}

// putDraft stores a draft exactly like analyze does and returns its intakeId.
func putDraft(t *testing.T, ctx context.Context, client fileRepository, bucket, name, birth string, docs []draftDoc) string {
	t.Helper()
	id := fmt.Sprintf("intake-%s-%08x", time.Now().UTC().Format("20060102T150405Z"), time.Now().UnixNano()&0xffffffff)
	files := []intakefeature.FileMetadata{}
	sources := []any{}
	for i, doc := range docs {
		key := refratiaDraftsPrefix + id + "/" + doc.filename
		data := []byte(fmt.Sprintf("%s-%s-%d", id, doc.filename, i))
		if _, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(data), ContentType: aws.String("application/pdf")}); err != nil {
			t.Fatal(err)
		}
		sha := fmt.Sprintf("%064x", i+1) + id
		sha = sha[len(sha)-64:]
		files = append(files, intakefeature.FileMetadata{Filename: doc.filename, OriginalFilename: doc.filename, ContentType: "application/pdf", Size: int64(len(data)), SHA256: sha, ExamType: doc.label, Eye: doc.eye, Key: key})
		sources = append(sources, map[string]any{"path": doc.filename, "sha256": sha, "exam": examTypeKey(doc.label), "eye": doc.eye})
	}
	analysis := map[string]any{
		"patient":                map[string]any{"full_name": name, "birth_date": birth},
		"verificacao_identidade": []any{},
		"source_files":           sources,
		"exams":                  map[string]any{},
	}
	raw, _ := json.Marshal(storedIntake{CreatedAt: time.Now().UTC(), Files: files, Analysis: analysis})
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(refratiaDraftsPrefix + id + "/intake.json"), Body: bytes.NewReader(raw)}); err != nil {
		t.Fatal(err)
	}
	return id
}

func confirm(t *testing.T, id string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"intakeId": id})
	rec := httptest.NewRecorder()
	confirmIntakeHandler(rec, httptest.NewRequest(http.MethodPost, "/api/intakes/confirm", bytes.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func countKeys(t *testing.T, ctx context.Context, client fileRepository, bucket, prefix string) int {
	t.Helper()
	listed, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)})
	if err != nil {
		t.Fatal(err)
	}
	return len(listed.Contents)
}

func cleanupCase(t *testing.T, ctx context.Context, client fileRepository, bucket, caseID string) {
	t.Cleanup(func() {
		listed, _ := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(refratiaCasesPrefix + caseID + "/")})
		for _, object := range listed.Contents {
			_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: object.Key})
		}
		_ = deleteCaseRows(caseID)
	})
}

// uniqueName is alphabetic only: names with digits are rejected as OCR noise.
func uniqueName() string {
	n, letters := time.Now().UnixNano(), []byte{}
	for ; n > 0; n /= 26 {
		letters = append(letters, byte('A'+n%26))
	}
	return "TESTE PERSIST " + string(letters)
}

func TestConfirmWritesOneRowPerDocumentAndEyeAndIsIdempotent(t *testing.T) {
	ctx, client, bucket := integrationSetup(t)
	name := uniqueName()
	id := putDraft(t, ctx, client, bucket, name, "1970-01-02", testDocs)

	status, first := confirm(t, id)
	if status != 200 || first["action"] != "created" {
		t.Fatalf("confirm #1: %d %v", status, first)
	}
	caseID := first["caseId"].(string)
	cleanupCase(t, ctx, client, bucket, caseID)

	var rows []database.Exam
	appDatabase.Gorm.Where("case_id = ?", caseID).Find(&rows)
	if len(rows) != 4 { // EyeSuite AO -> OD+OS, Pentacam OD, Pentacam OS
		t.Fatalf("want 4 rows, got %d", len(rows))
	}
	for _, row := range rows {
		if row.ExamType == "" || row.PatientID == nil {
			t.Fatalf("incomplete row: %+v", row)
		}
	}
	var patient database.Patient
	appDatabase.Gorm.Where("case_id = ?", caseID).First(&patient)
	if patient.ID == 0 || patient.CaseID != caseID {
		t.Fatalf("patient not linked to case: %+v", patient)
	}

	status, second := confirm(t, id)
	if status != 200 || second["caseId"] != caseID {
		t.Fatalf("confirm #2 must replay the receipt: %d %v", status, second)
	}
	var count int64
	appDatabase.Gorm.Model(&database.Exam{}).Where("case_id = ?", caseID).Count(&count)
	if count != 4 {
		t.Fatalf("retry changed rows: %d", count)
	}

	// A second intake for the same patient joins the same case (SQL match).
	id2 := putDraft(t, ctx, client, bucket, strings.ToLower(name), "1970-01-02", testDocs[:1])
	status, third := confirm(t, id2)
	if status != 200 || third["action"] != "updated" || third["caseId"] != caseID {
		t.Fatalf("second intake: %d %v", status, third)
	}
	appDatabase.Gorm.Model(&database.Patient{}).Where("case_id = ?", caseID).Count(&count)
	if count != 1 {
		t.Fatalf("one patient row expected, got %d", count)
	}
}

func TestConfirmRejectsUnresolvedIdentityBeforeAnyGarageWrite(t *testing.T) {
	ctx, client, bucket := integrationSetup(t)
	id := putDraft(t, ctx, client, bucket, uniqueName(), "", testDocs)
	status, _ := confirm(t, id)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", status)
	}
	caseID := "case-" + strings.TrimPrefix(id, "intake-")
	if n := countKeys(t, ctx, client, bucket, refratiaCasesPrefix+caseID+"/"); n != 0 {
		t.Fatalf("422 must leave no case objects, found %d", n)
	}
	if n := countKeys(t, ctx, client, bucket, confirmationKey(id)); n != 0 {
		t.Fatal("422 must leave no receipt")
	}
}

func TestConcurrentConfirmsOfSamePatientShareOneCase(t *testing.T) {
	ctx, client, bucket := integrationSetup(t)
	name := uniqueName()
	ids := []string{
		putDraft(t, ctx, client, bucket, name, "1971-03-04", testDocs[:1]),
		putDraft(t, ctx, client, bucket, name, "1971-03-04", testDocs[1:2]),
		putDraft(t, ctx, client, bucket, name, "1971-03-04", testDocs[2:3]),
	}
	cases := make([]string, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, out := confirm(t, id)
			if status != 200 {
				t.Errorf("confirm %d: %d %v", i, status, out)
				return
			}
			cases[i], _ = out["caseId"].(string)
		}()
	}
	wg.Wait()
	for _, caseID := range cases {
		if caseID != cases[0] {
			t.Fatalf("one patient, several cases: %v", cases)
		}
	}
	cleanupCase(t, ctx, client, bucket, cases[0])
	var count int64
	appDatabase.Gorm.Model(&database.Exam{}).Where("case_id = ?", cases[0]).Count(&count)
	if count != 4 {
		t.Fatalf("want 4 rows (AO=2 + OD + OS), got %d", count)
	}
}

func TestDeleteCaseRemovesSQLRows(t *testing.T) {
	ctx, client, bucket := integrationSetup(t)
	status, out := confirm(t, putDraft(t, ctx, client, bucket, uniqueName(), "1972-05-06", testDocs))
	if status != 200 {
		t.Fatalf("confirm: %d", status)
	}
	caseID := out["caseId"].(string)
	cleanupCase(t, ctx, client, bucket, caseID)

	t.Setenv("CASE_DELETE_TOKEN", "t")
	request := httptest.NewRequest(http.MethodDelete, "/api/cases/"+caseID, nil)
	request.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	caseHandler(rec, request)
	if rec.Code != 200 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	var exams, patients int64
	appDatabase.Gorm.Model(&database.Exam{}).Where("case_id = ?", caseID).Count(&exams)
	appDatabase.Gorm.Model(&database.Patient{}).Where("case_id = ?", caseID).Count(&patients)
	if exams != 0 || patients != 0 {
		t.Fatalf("orphans left: exams=%d patients=%d", exams, patients)
	}
}

func TestSavedExamConfirmIdentityFailureIs422AndRevertsObject(t *testing.T) {
	ctx, client, bucket := integrationSetup(t)
	t.Setenv("CASE_DELETE_TOKEN", "t")
	caseID := "case-" + time.Now().UTC().Format("20060102T150405Z") + "-deadbeef"
	cleanupCase(t, ctx, client, bucket, caseID)
	put := func(key, body string) {
		if _, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: strings.NewReader(body)}); err != nil {
			t.Fatal(err)
		}
	}
	put(refratiaCasesPrefix+caseID+"/paciente_compilado.json", `{"patient":{"full_name":"SEM NASCIMENTO"},"source_files":[]}`)
	objectKey := refratiaCasesPrefix + caseID + "/1-extra.pdf"
	put(objectKey, "pdf")

	body, _ := json.Marshal(map[string]any{"caseId": caseID, "path": objectKey, "filename": "extra.pdf", "contentType": "application/pdf", "size": 3})
	request := httptest.NewRequest(http.MethodPost, "/api/exams/confirm", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	confirmSavedExam(rec, request)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d %s", rec.Code, rec.Body.String())
	}
	if _, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(objectKey)}); err == nil {
		t.Fatal("uncommitted object must be reverted")
	}
}

func confirmWith(t *testing.T, id string, decision map[string]string) (int, map[string]any) {
	t.Helper()
	payload := map[string]any{"intakeId": id}
	if decision != nil {
		payload["identityDecision"] = decision
	}
	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	confirmIntakeHandler(rec, httptest.NewRequest(http.MethodPost, "/api/intakes/confirm", bytes.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestNameTypoVariantRequiresIdentityDecision(t *testing.T) {
	ctx, client, bucket := integrationSetup(t)
	// Fixed names (deterministic): the typo changes the sorted token order.
	typo, clean := "ANA JZ BRAGA", "ANA Z BRAGA"
	// Fixed identity: clear debris of earlier runs (test DB only).
	appDatabase.Gorm.Where("birth_date = ?", "1973-07-08").Delete(&database.Patient{})
	status, first := confirm(t, putDraft(t, ctx, client, bucket, typo, "1973-07-08", testDocs[:1]))
	if status != 200 {
		t.Fatalf("seed: %d %v", status, first)
	}
	caseID := first["caseId"].(string)
	cleanupCase(t, ctx, client, bucket, caseID)

	// No decision: 409, nothing written.
	id := putDraft(t, ctx, client, bucket, clean, "1973-07-08", testDocs[1:2])
	status, out := confirmWith(t, id, nil)
	candidates, _ := out["candidates"].([]any)
	if status != http.StatusConflict || out["error"] != "identity_review_required" || len(candidates) != 1 {
		t.Fatalf("want 409 with candidate, got %d %v", status, out)
	}
	newCase := "case-" + strings.TrimPrefix(id, "intake-")
	if n := countKeys(t, ctx, client, bucket, refratiaCasesPrefix+newCase+"/"); n != 0 {
		t.Fatalf("409 must write no objects, found %d", n)
	}
	var patients int64
	appDatabase.Gorm.Model(&database.Patient{}).Where("birth_date = ?", "1973-07-08").Count(&patients)
	if patients != 1 {
		t.Fatalf("409 must write no rows, patients=%d", patients)
	}

	// existing: merged, still one patient, original identity kept.
	status, out = confirmWith(t, id, map[string]string{"action": "existing", "caseId": caseID})
	if status != 200 || out["caseId"] != caseID || out["action"] != "updated" {
		t.Fatalf("existing: %d %v", status, out)
	}
	appDatabase.Gorm.Model(&database.Patient{}).Where("birth_date = ?", "1973-07-08").Count(&patients)
	if patients != 1 {
		t.Fatalf("want 1 patient after merge, got %d", patients)
	}

	// new: a second patient, knowingly.
	id2 := putDraft(t, ctx, client, bucket, "ANA KZ BRAGA", "1973-07-08", testDocs[2:3])
	status, out = confirmWith(t, id2, map[string]string{"action": "new"})
	if status != 200 || out["action"] != "created" {
		t.Fatalf("new: %d %v", status, out)
	}
	cleanupCase(t, ctx, client, bucket, out["caseId"].(string))
	appDatabase.Gorm.Model(&database.Patient{}).Where("birth_date = ?", "1973-07-08").Count(&patients)
	if patients != 2 {
		t.Fatalf("want 2 patients after explicit new, got %d", patients)
	}
}
