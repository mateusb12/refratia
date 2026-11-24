package intake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"refratia/backend/src/features/persistence/cases"
	"refratia/backend/src/features/persistence/exams"
	"refratia/backend/src/shared/fileStorage"
)

// seedCase puts a manifest and one source object under a fresh case id.
func seedCase(t *testing.T, manifest string) (caseID, objectKey string, exists func() bool) {
	t.Helper()
	executionContext, client, bucket := integrationSetup(t)
	t.Setenv("CASE_DELETE_TOKEN", "t")
	caseID = fmt.Sprintf("case-%s-%08x", time.Now().UTC().Format("20060102T150405Z"), time.Now().UnixNano()&0xffffffff)
	cleanupCase(t, executionContext, client, bucket, caseID)
	objectKey = fileStorage.CasesPrefix + caseID + "/1-src.pdf"
	for key, body := range map[string]string{fileStorage.CasesPrefix + caseID + "/paciente_compilado.json": strings.ReplaceAll(manifest, "KEY", objectKey), objectKey: "pdf"} {
		if _, err := client.PutObject(executionContext, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: strings.NewReader(body)}); err != nil {
			t.Fatal(err)
		}
	}
	exists = func() bool {
		_, err := client.HeadObject(executionContext, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(objectKey)})
		return err == nil
	}
	return
}

func callExams(h func(http.ResponseWriter, *http.Request), method, url string, payload map[string]any) *httptest.ResponseRecorder {
	body, _ := json.Marshal(payload)
	request := httptest.NewRequest(method, url, bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	h(rec, request)
	return rec
}

func confirmExam(caseID, key string) *httptest.ResponseRecorder {
	return callExams(exams.Handler{Database: testHandler.Database}.ServeConfirm, http.MethodPost, "/api/exams/confirm",
		map[string]any{"caseId": caseID, "path": key, "filename": "src.pdf", "contentType": "application/pdf", "size": 3})
}

// D1: a source document the manifest references, with no SQL row, must survive a failed confirm.
func TestConfirmNeverDeletesReferencedObjectOnIdentityFailure(t *testing.T) {
	_, _, _ = integrationSetup(t)
	caseID, key, exists := seedCase(t, `{"patient":{"full_name":"SEM NASCIMENTO"},"source_files":[{"path":"KEY","exam":"PENTACAM","eye":"OD"}]}`)
	rec := confirmExam(caseID, key)
	if rec.Code < 400 {
		t.Fatalf("want error, got %d", rec.Code)
	}
	if !exists() {
		t.Fatal("referenced source document was deleted")
	}
}

// D1: when the manifest cannot be read we cannot prove the object is unreferenced.
func TestConfirmNeverDeletesObjectWhenManifestUnreadable(t *testing.T) {
	_, _, _ = integrationSetup(t)
	caseID, key, exists := seedCase(t, "{not json")
	rec := confirmExam(caseID, key)
	if rec.Code < 500 {
		t.Fatalf("want 5xx, got %d", rec.Code)
	}
	if !exists() {
		t.Fatal("object deleted although the manifest could not be read")
	}
}

// D2: undecodable manifest => fail without deleting anything.
func TestDeleteExamWithUnreadableManifestFailsAndKeepsObject(t *testing.T) {
	_, _, _ = integrationSetup(t)
	caseID, key, exists := seedCase(t, "{not json")
	rec := callExams(exams.Handler{Database: testHandler.Database}.ServeExams, http.MethodDelete, "/api/exams", map[string]any{"caseId": caseID, "path": key})
	if rec.Code < 500 {
		t.Fatalf("want 5xx, got %d %s", rec.Code, rec.Body.String())
	}
	if !exists() {
		t.Fatal("object deleted while manifest still points at it")
	}
}

// D3: more case prefixes than one ListObjectsV2 page (1000) must all be listed.
func TestListCasesPaginatesBeyondOnePage(t *testing.T) {
	executionContext, client, bucket := integrationSetup(t)
	const total = 1005
	stamp := time.Now().UTC().Format("20060102T150405Z")
	ids := make([]string, total)
	sem := make(chan struct{}, 32)
	done := make(chan error, total)
	for i := range ids {
		ids[i] = fmt.Sprintf("case-%s-%08x", stamp, 0x7e570000+i)
		key := fileStorage.CasesPrefix + ids[i] + "/paciente_compilado.json"
		go func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			_, err := client.PutObject(executionContext, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: strings.NewReader(`{"patient":{"full_name":"X"}}`)})
			done <- err
		}()
	}
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = client.DeleteObject(executionContext, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(fileStorage.CasesPrefix + id + "/paciente_compilado.json")})
		}
	})
	for range ids {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	cases.Handler{Database: testHandler.Database}.ServeCases(rec, httptest.NewRequest(http.MethodGet, "/api/cases", nil))
	var out struct {
		Cases []struct {
			CaseID string `json:"caseId"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, c := range out.Cases {
		have[c.CaseID] = true
	}
	missing := 0
	for _, id := range ids {
		if !have[id] {
			missing++
		}
	}
	if missing != 0 {
		t.Fatalf("%d of %d cases missing from listing (%d listed)", missing, total, len(out.Cases))
	}
}
