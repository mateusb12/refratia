package intake

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
)

// Negative regression test for the intentional optical-biometry gate (c63d800): an intake without
// EyeSuite/IOL biometry (the benchmark's daniela-nogueira folder) must be rejected with 422 before any
// extraction or storage. Handler{} has no database: reaching past the gate would fail loudly.
func TestAnalyzeRejectsIntakeWithoutOpticalBiometry(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	var cls []map[string]string
	for _, eye := range []string{"OD", "OS"} {
		data := []byte("%PDF-1.4 pentacam " + eye)
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="files"; filename="PENTACAM_`+eye+`.pdf"`)
		h.Set("Content-Type", "application/pdf")
		pw, _ := w.CreatePart(h)
		pw.Write(data)
		sum := sha256.Sum256(data)
		cls = append(cls, map[string]string{"sha256": hex.EncodeToString(sum[:]), "examType": "pentacam_corneal_tomography", "eye": eye})
	}
	b, _ := json.Marshal(cls)
	w.WriteField("classifications", string(b))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/intakes/analyze", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	Handler{}.AnalyzeIntake(rec, req)

	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "biometria óptica (EyeSuite) obrigatória") {
		t.Fatalf("want 422 biometry rejection, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHasCompleteOpticalBiometry(t *testing.T) {
	f := func(typ, eye string) UploadedFile {
		return UploadedFile{Metadata: FileMetadata{ExamType: typ, Eye: eye}}
	}
	cases := []struct {
		files []UploadedFile
		want  bool
	}{
		{[]UploadedFile{f("pentacam_corneal_tomography", "OD"), f("pentacam_corneal_tomography", "OS")}, false},
		{[]UploadedFile{f("iol_calculation", "OD")}, false},
		{[]UploadedFile{f("iol_calculation", "AO")}, true},
		{[]UploadedFile{f("EYESUITE", "OD"), f("EYESUITE", "OS")}, true},
	}
	for i, c := range cases {
		if got := hasCompleteOpticalBiometry(c.files); got != c.want {
			t.Errorf("case %d: got %v want %v", i, got, c.want)
		}
	}
}
