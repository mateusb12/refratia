package intake

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
	"gorm.io/gorm"

	"refratia/backend/src/features/cases"
	"refratia/backend/src/features/exams"
	"refratia/backend/src/features/reconcile"
	"refratia/backend/src/shared/database"
	"refratia/backend/src/shared/fileStorage"
)

// testHandler is the intake handler wired to the test database by integrationSetup.
var testHandler Handler

// Integration tests against testinfra (Postgres refratia_test + Garage
// refratia-test). Skipped unless REFRATIA_NO_DOTENV is set, so they can never
// run against the developer .env (which points at production).
func integrationSetup(t *testing.T) (context.Context, fileStorage.Repository, string) {
	t.Helper()
	if os.Getenv("REFRATIA_NO_DOTENV") == "" || !strings.HasSuffix(os.Getenv("BUCKET_NAME"), "-test") {
		t.Skip("integration test: source backend/testinfra/test.env first")
	}
	executionContext := context.Background()
	openedDatabase, errorValue := database.Open(executionContext)
	if errorValue != nil {
		t.Fatal(errorValue)
	}
	if errorValue := openedDatabase.Migrate(); errorValue != nil {
		t.Fatal(errorValue)
	}
	testHandler = Handler{Database: openedDatabase.Gorm}
	client, errorValue := fileStorage.New(executionContext)
	if errorValue != nil {
		t.Fatal(errorValue)
	}
	return executionContext, client, os.Getenv("BUCKET_NAME")
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
func putDraft(t *testing.T, executionContext context.Context, client fileStorage.Repository, bucket, name, birth string, docs []draftDoc) string {
	t.Helper()
	id := fmt.Sprintf("intake-%s-%08x", time.Now().UTC().Format("20060102T150405Z"), time.Now().UnixNano()&0xffffffff)
	files := []FileMetadata{}
	sources := []any{}
	for index, doc := range docs {
		key := fileStorage.DraftsPrefix + id + "/" + doc.filename
		data := []byte(fmt.Sprintf("%s-%s-%d", id, doc.filename, index))
		if _, errorValue := client.PutObject(executionContext, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(data), ContentType: aws.String("application/pdf")}); errorValue != nil {
			t.Fatal(errorValue)
		}
		sha := fmt.Sprintf("%064x", index+1) + id
		sha = sha[len(sha)-64:]
		files = append(files, FileMetadata{Filename: doc.filename, OriginalFilename: doc.filename, ContentType: "application/pdf", Size: int64(len(data)), SHA256: sha, ExamType: doc.label, Eye: doc.eye, Key: key})
		sources = append(sources, map[string]any{"path": doc.filename, "sha256": sha, "exam": exams.TypeKey(doc.label), "eye": doc.eye})
	}
	analysis := map[string]any{
		"patient":                map[string]any{"full_name": name, "birth_date": birth},
		"verificacao_identidade": []any{},
		"source_files":           sources,
		"exams":                  map[string]any{},
	}
	raw, _ := json.Marshal(storedIntake{CreatedAt: time.Now().UTC(), Files: files, Analysis: analysis})
	if _, errorValue := client.PutObject(executionContext, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(fileStorage.DraftsPrefix + id + "/intake.json"), Body: bytes.NewReader(raw)}); errorValue != nil {
		t.Fatal(errorValue)
	}
	return id
}

func confirm(t *testing.T, id string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"intakeId": id})
	rec := httptest.NewRecorder()
	testHandler.ConfirmIntake(rec, httptest.NewRequest(http.MethodPost, "/api/intakes/confirm", bytes.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func countKeys(t *testing.T, executionContext context.Context, client fileStorage.Repository, bucket, prefix string) int {
	t.Helper()
	listed, errorValue := client.ListObjectsV2(executionContext, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)})
	if errorValue != nil {
		t.Fatal(errorValue)
	}
	return len(listed.Contents)
}

func cleanupCase(t *testing.T, executionContext context.Context, client fileStorage.Repository, bucket, caseID string) {
	t.Cleanup(func() {
		listed, _ := client.ListObjectsV2(executionContext, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(fileStorage.CasesPrefix + caseID + "/")})
		for _, object := range listed.Contents {
			_, _ = client.DeleteObject(executionContext, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: object.Key})
		}
		_ = testHandler.Database.Transaction(func(transaction *gorm.DB) error {
			if deleteError := transaction.Where("case_id = ?", caseID).Delete(&database.Exam{}).Error; deleteError != nil {
				return deleteError
			}
			return transaction.Where("case_id = ?", caseID).Delete(&database.Patient{}).Error
		})
	})
}

// uniqueName is alphabetic only: names with digits are rejected as OCR noise.
func uniqueName() string {
	remaining, letters := time.Now().UnixNano(), []byte{}
	for ; remaining > 0; remaining /= 26 {
		letters = append(letters, byte('A'+remaining%26))
	}
	return "TESTE PERSIST " + string(letters)
}

func TestConfirmWritesOneRowPerDocumentAndEyeAndIsIdempotent(t *testing.T) {
	executionContext, client, bucket := integrationSetup(t)
	name := uniqueName()
	id := putDraft(t, executionContext, client, bucket, name, "1970-01-02", testDocs)

	status, first := confirm(t, id)
	if status != 200 || first["action"] != "created" {
		t.Fatalf("confirm #1: %d %v", status, first)
	}
	caseID := first["caseId"].(string)
	cleanupCase(t, executionContext, client, bucket, caseID)

	var rows []database.Exam
	testHandler.Database.Where("case_id = ?", caseID).Find(&rows)
	if len(rows) != 4 { // EyeSuite AO -> OD+OS, Pentacam OD, Pentacam OS
		t.Fatalf("want 4 rows, got %d", len(rows))
	}
	for _, row := range rows {
		if row.ExamType == "" || row.PatientID == nil {
			t.Fatalf("incomplete row: %+v", row)
		}
	}
	var patient database.Patient
	testHandler.Database.Where("case_id = ?", caseID).First(&patient)
	if patient.ID == 0 || patient.CaseID != caseID {
		t.Fatalf("patient not linked to case: %+v", patient)
	}

	status, second := confirm(t, id)
	if status != 200 || second["caseId"] != caseID {
		t.Fatalf("confirm #2 must replay the receipt: %d %v", status, second)
	}
	var count int64
	testHandler.Database.Model(&database.Exam{}).Where("case_id = ?", caseID).Count(&count)
	if count != 4 {
		t.Fatalf("retry changed rows: %d", count)
	}

	// A second intake for the same patient joins the same case (SQL match).
	id2 := putDraft(t, executionContext, client, bucket, strings.ToLower(name), "1970-01-02", testDocs[:1])
	status, third := confirm(t, id2)
	if status != 200 || third["action"] != "updated" || third["caseId"] != caseID {
		t.Fatalf("second intake: %d %v", status, third)
	}
	testHandler.Database.Model(&database.Patient{}).Where("case_id = ?", caseID).Count(&count)
	if count != 1 {
		t.Fatalf("one patient row expected, got %d", count)
	}
}

func TestConfirmRejectsUnresolvedIdentityBeforeAnyGarageWrite(t *testing.T) {
	executionContext, client, bucket := integrationSetup(t)
	id := putDraft(t, executionContext, client, bucket, uniqueName(), "", testDocs)
	status, _ := confirm(t, id)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", status)
	}
	caseID := "case-" + strings.TrimPrefix(id, "intake-")
	if objectCount := countKeys(t, executionContext, client, bucket, fileStorage.CasesPrefix+caseID+"/"); objectCount != 0 {
		t.Fatalf("422 must leave no case objects, found %d", objectCount)
	}
	if objectCount := countKeys(t, executionContext, client, bucket, cases.ConfirmationKey(id)); objectCount != 0 {
		t.Fatal("422 must leave no receipt")
	}
}

func TestConcurrentConfirmsOfSamePatientShareOneCase(t *testing.T) {
	executionContext, client, bucket := integrationSetup(t)
	name := uniqueName()
	ids := []string{
		putDraft(t, executionContext, client, bucket, name, "1971-03-04", testDocs[:1]),
		putDraft(t, executionContext, client, bucket, name, "1971-03-04", testDocs[1:2]),
		putDraft(t, executionContext, client, bucket, name, "1971-03-04", testDocs[2:3]),
	}
	cases := make([]string, len(ids))
	var wg sync.WaitGroup
	for index, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, out := confirm(t, id)
			if status != 200 {
				t.Errorf("confirm %d: %d %v", index, status, out)
				return
			}
			cases[index], _ = out["caseId"].(string)
		}()
	}
	wg.Wait()
	for _, caseID := range cases {
		if caseID != cases[0] {
			t.Fatalf("one patient, several cases: %v", cases)
		}
	}
	cleanupCase(t, executionContext, client, bucket, cases[0])
	var count int64
	testHandler.Database.Model(&database.Exam{}).Where("case_id = ?", cases[0]).Count(&count)
	if count != 4 {
		t.Fatalf("want 4 rows (AO=2 + OD + OS), got %d", count)
	}
}

func TestDeleteCaseRemovesSQLRows(t *testing.T) {
	executionContext, client, bucket := integrationSetup(t)
	status, out := confirm(t, putDraft(t, executionContext, client, bucket, uniqueName(), "1972-05-06", testDocs))
	if status != 200 {
		t.Fatalf("confirm: %d", status)
	}
	caseID := out["caseId"].(string)
	cleanupCase(t, executionContext, client, bucket, caseID)

	t.Setenv("CASE_DELETE_TOKEN", "t")
	request := httptest.NewRequest(http.MethodDelete, "/api/cases/"+caseID, nil)
	request.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	cases.Handler{Database: testHandler.Database}.ServeCase(rec, request)
	if rec.Code != 200 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	var exams, patients int64
	testHandler.Database.Model(&database.Exam{}).Where("case_id = ?", caseID).Count(&exams)
	testHandler.Database.Model(&database.Patient{}).Where("case_id = ?", caseID).Count(&patients)
	if exams != 0 || patients != 0 {
		t.Fatalf("orphans left: exams=%d patients=%d", exams, patients)
	}
}

func TestSavedExamConfirmIdentityFailureIs422AndRevertsObject(t *testing.T) {
	executionContext, client, bucket := integrationSetup(t)
	t.Setenv("CASE_DELETE_TOKEN", "t")
	caseID := "case-" + time.Now().UTC().Format("20060102T150405Z") + "-deadbeef"
	cleanupCase(t, executionContext, client, bucket, caseID)
	put := func(key, body string) {
		if _, errorValue := client.PutObject(executionContext, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: strings.NewReader(body)}); errorValue != nil {
			t.Fatal(errorValue)
		}
	}
	put(fileStorage.CasesPrefix+caseID+"/paciente_compilado.json", `{"patient":{"full_name":"SEM NASCIMENTO"},"source_files":[]}`)
	objectKey := fileStorage.CasesPrefix + caseID + "/1-extra.pdf"
	put(objectKey, "pdf")

	body, _ := json.Marshal(map[string]any{"caseId": caseID, "path": objectKey, "filename": "extra.pdf", "contentType": "application/pdf", "size": 3})
	request := httptest.NewRequest(http.MethodPost, "/api/exams/confirm", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	exams.Handler{Database: testHandler.Database}.ServeConfirm(rec, request)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d %s", rec.Code, rec.Body.String())
	}
	if _, errorValue := client.HeadObject(executionContext, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(objectKey)}); errorValue == nil {
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
	testHandler.ConfirmIntake(rec, httptest.NewRequest(http.MethodPost, "/api/intakes/confirm", bytes.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestNameTypoVariantRequiresIdentityDecision(t *testing.T) {
	executionContext, client, bucket := integrationSetup(t)
	// Fixed names (deterministic): the typo changes the sorted token order.
	typo, clean := "ANA JZ BRAGA", "ANA Z BRAGA"
	// Fixed identity: clear debris of earlier runs (test DB only).
	testHandler.Database.Where("birth_date = ?", "1973-07-08").Delete(&database.Patient{})
	status, first := confirm(t, putDraft(t, executionContext, client, bucket, typo, "1973-07-08", testDocs[:1]))
	if status != 200 {
		t.Fatalf("seed: %d %v", status, first)
	}
	caseID := first["caseId"].(string)
	cleanupCase(t, executionContext, client, bucket, caseID)

	// No decision: 409, nothing written.
	id := putDraft(t, executionContext, client, bucket, clean, "1973-07-08", testDocs[1:2])
	status, out := confirmWith(t, id, nil)
	candidates, _ := out["candidates"].([]any)
	if status != http.StatusConflict || out["error"] != "identity_review_required" || len(candidates) != 1 {
		t.Fatalf("want 409 with candidate, got %d %v", status, out)
	}
	newCase := "case-" + strings.TrimPrefix(id, "intake-")
	if objectCount := countKeys(t, executionContext, client, bucket, fileStorage.CasesPrefix+newCase+"/"); objectCount != 0 {
		t.Fatalf("409 must write no objects, found %d", objectCount)
	}
	var patients int64
	testHandler.Database.Model(&database.Patient{}).Where("birth_date = ?", "1973-07-08").Count(&patients)
	if patients != 1 {
		t.Fatalf("409 must write no rows, patients=%d", patients)
	}

	// existing: merged, still one patient, original identity kept.
	status, out = confirmWith(t, id, map[string]string{"action": "existing", "caseId": caseID})
	if status != 200 || out["caseId"] != caseID || out["action"] != "updated" {
		t.Fatalf("existing: %d %v", status, out)
	}
	testHandler.Database.Model(&database.Patient{}).Where("birth_date = ?", "1973-07-08").Count(&patients)
	if patients != 1 {
		t.Fatalf("want 1 patient after merge, got %d", patients)
	}

	// new: a second patient, knowingly.
	id2 := putDraft(t, executionContext, client, bucket, "ANA KZ BRAGA", "1973-07-08", testDocs[2:3])
	status, out = confirmWith(t, id2, map[string]string{"action": "new"})
	if status != 200 || out["action"] != "created" {
		t.Fatalf("new: %d %v", status, out)
	}
	cleanupCase(t, executionContext, client, bucket, out["caseId"].(string))
	testHandler.Database.Model(&database.Patient{}).Where("birth_date = ?", "1973-07-08").Count(&patients)
	if patients != 2 {
		t.Fatalf("want 2 patients after explicit new, got %d", patients)
	}
}

func TestConfirmSurvivesSQLFailureAfterGarageWrites(t *testing.T) {
	executionContext, client, bucket := integrationSetup(t)
	name := uniqueName()
	birth := "1974-09-10"
	id := putDraft(t, executionContext, client, bucket, name, birth, testDocs)
	caseID := "case-" + strings.TrimPrefix(id, "intake-")
	cleanupCase(t, executionContext, client, bucket, caseID)

	testHandler.afterGarageWrites = func() error { return fmt.Errorf("injected sql failure") }
	status, out := confirm(t, id)
	testHandler.afterGarageWrites = nil
	if status < 500 {
		t.Fatalf("want 5xx, got %d %v", status, out)
	}
	// (1) no receipt, (3) draft kept.
	if objectCount := countKeys(t, executionContext, client, bucket, cases.ConfirmationKey(id)); objectCount != 0 {
		t.Fatal("failed confirm must leave no receipt")
	}
	if objectCount := countKeys(t, executionContext, client, bucket, fileStorage.DraftsPrefix+id+"/"); objectCount != len(testDocs)+1 {
		t.Fatalf("draft must be kept for retry, found %d objects", objectCount)
	}
	// (2) nothing committed.
	var patients, exams int64
	testHandler.Database.Model(&database.Patient{}).Where("birth_date = ?", birth).Count(&patients)
	testHandler.Database.Model(&database.Exam{}).Where("case_id = ?", caseID).Count(&exams)
	if patients != 0 || exams != 0 {
		t.Fatalf("rolled back: patients=%d exams=%d", patients, exams)
	}
	// Garage objects of the failed attempt exist (garbage until the retry).
	garbage := countKeys(t, executionContext, client, bucket, fileStorage.CasesPrefix+caseID+"/")
	if garbage != len(testDocs)+1 {
		t.Fatalf("expected %d orphan objects, got %d", len(testDocs)+1, garbage)
	}

	// (4a) reconcile lists the failed attempt's case as garbage.
	if !reconcileLists(t, executionContext, client, bucket, "garage-case-without-sql", caseID) {
		t.Fatal("reconcile must list the failed attempt's case as garage-case-without-sql")
	}

	// (4) retry converges; (5) the same keys are reused, no duplicates.
	status, out = confirm(t, id)
	if status != 200 {
		t.Fatalf("retry: %d %v", status, out)
	}
	if out["caseId"] != caseID {
		t.Fatalf("retry must reuse the case of the failed attempt: %v", out)
	}
	testHandler.Database.Model(&database.Patient{}).Where("birth_date = ?", birth).Count(&patients)
	testHandler.Database.Model(&database.Exam{}).Where("case_id = ?", caseID).Count(&exams)
	if patients != 1 || exams != 4 {
		t.Fatalf("converged state: patients=%d exams=%d (want 1, 4)", patients, exams)
	}
	if objectCount := countKeys(t, executionContext, client, bucket, fileStorage.CasesPrefix+caseID+"/"); objectCount != garbage {
		t.Fatalf("retry changed object count: %d -> %d", garbage, objectCount)
	}
	if objectCount := countKeys(t, executionContext, client, bucket, fileStorage.DraftsPrefix+id+"/"); objectCount != 0 {
		t.Fatalf("draft must be consumed after success, found %d", objectCount)
	}
	if reconcileLists(t, executionContext, client, bucket, "garage-case-without-sql", caseID) {
		t.Fatal("after the retry reconcile must no longer list the case")
	}
}

func reconcileLists(t *testing.T, executionContext context.Context, client fileStorage.Repository, bucket, kind, caseID string) bool {
	t.Helper()
	findings, errorValue := reconcile.Run(executionContext, testHandler.Database, client, bucket)
	if errorValue != nil {
		t.Fatal(errorValue)
	}
	for _, finding := range findings {
		if finding.Kind == kind && finding.Detail == caseID {
			return true
		}
	}
	return false
}
