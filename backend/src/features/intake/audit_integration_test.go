package intake

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"refratia/backend/src/shared/database"
)

func postFiles(t *testing.T, handler http.HandlerFunc, files map[string][]byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for filename, data := range files {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="files"; filename="`+filename+`"`)
		header.Set("Content-Type", "image/png")
		part, _ := form.CreatePart(header)
		part.Write(data)
	}
	form.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/intakes/inspect", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

func latestAudit(t *testing.T, kind string) (database.ProcessingAudit, auditDetails) {
	t.Helper()
	var row database.ProcessingAudit
	if findError := testHandler.Database.Where("kind = ?", kind).Order("id DESC").First(&row).Error; findError != nil {
		t.Fatal(findError)
	}
	var details auditDetails
	if decodeError := json.Unmarshal([]byte(row.Details), &details); decodeError != nil {
		t.Fatal(decodeError)
	}
	return row, details
}

func TestAuditRecordsInspectRunAndFiles(t *testing.T) {
	integrationSetup(t)
	var blank bytes.Buffer
	png.Encode(&blank, image.NewGray(image.Rect(0, 0, 40, 20)))

	audited := testHandler.Audit("inspect", InspectIntakeHandler)
	if response := postFiles(t, audited, map[string][]byte{"blank.png": blank.Bytes()}); response.Code != http.StatusOK {
		t.Fatalf("inspect status %d: %s", response.Code, response.Body.String())
	}
	_, details := latestAudit(t, "inspect")
	if details.HTTPStatus != http.StatusOK || details.Error != "" || len(details.Files) != 1 {
		t.Fatalf("unexpected audit: %+v", details)
	}
	if file := details.Files[0]; file.Filename != "blank.png" || file.SHA256 == "" || file.Outcome != "unrecognized" {
		t.Fatalf("unexpected file entry: %+v", file)
	}

	// A rejected request is audited too, with the error message it returned.
	if response := postFiles(t, audited, map[string][]byte{}); response.Code < 400 {
		t.Fatalf("expected an error for an empty upload, got %d", response.Code)
	}
	_, details = latestAudit(t, "inspect")
	if details.HTTPStatus < 400 || details.Error == "" {
		t.Fatalf("error not audited: %+v", details)
	}
}
