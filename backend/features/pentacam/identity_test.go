package pentacam

import (
	"context"
	"os"
	"testing"

	"refratia/backend/shared/ocr"
)

func TestParsePentacamIdentityWords(t *testing.T) {
	words := []ocr.Word{
		{Text: "Sobrenome:", Left: 100, Top: 100},
		{Text: "JDE", Left: 300, Top: 100},
		{Text: "CARVALHO", Left: 380, Top: 100},
		{Text: "Nome:", Left: 100, Top: 140},
		{Text: "MARIA", Left: 300, Top: 140},
		{Text: "GORETE", Left: 400, Top: 140},
		{Text: "Nasc.:", Left: 100, Top: 180},
		{Text: "03/21/1962", Left: 300, Top: 180},
		{Text: "Data de Exame:", Left: 100, Top: 220},
		{Text: "10/27/2026", Left: 300, Top: 220},
	}

	got, err := parsePentacamIdentityWords(words)
	if err != nil {
		t.Fatal(err)
	}

	if got.FullName != "MARIA GORETE JDE CARVALHO" {
		t.Fatalf("nome inesperado: %q", got.FullName)
	}
	if got.BirthDateRaw != "03/21/1962" {
		t.Fatalf("nascimento inesperado: %q", got.BirthDateRaw)
	}
	if got.ExamDateRaw != "10/27/2026" {
		t.Fatalf("data do exame inesperada: %q", got.ExamDateRaw)
	}
}

func TestParsePentacamIdentityWordsDoesNotUseExamDateAsBirthDate(t *testing.T) {
	words := []ocr.Word{
		{Text: "Data", Left: 100, Top: 100},
		{Text: "de", Left: 150, Top: 100},
		{Text: "Exame:", Left: 180, Top: 100},
		{Text: "11/04/2026", Left: 300, Top: 100},
		{Text: "Nasc.:", Left: 100, Top: 140},
		{Text: "12/14/1979", Left: 300, Top: 140},
		{Text: "Sobrenome:", Left: 100, Top: 180},
		{Text: "AVERSA MARTINELLI", Left: 300, Top: 180},
		{Text: "Nome:", Left: 100, Top: 220},
		{Text: "CRESO", Left: 300, Top: 220},
	}

	got, err := parsePentacamIdentityWords(words)
	if err != nil {
		t.Fatal(err)
	}
	if got.BirthDateRaw != "12/14/1979" {
		t.Fatalf("nascimento inesperado: %q", got.BirthDateRaw)
	}
	if got.ExamDateRaw != "11/04/2026" {
		t.Fatalf("data do exame inesperada: %q", got.ExamDateRaw)
	}
}

func TestExtractIdentityFromMariaGoretePentacam(t *testing.T) {
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
	data, err := os.ReadFile("../../../frontend/data/maria-gorete/PENTACAM OD MARIA GORETE.pdf")
	if err != nil {
		t.Fatal(err)
	}

	got, err := ExtractIdentity(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}

	if got.FullName != "MARIA GORETE JDE CARVALHO" {
		t.Fatalf("nome inesperado: %q", got.FullName)
	}
	if got.BirthDateRaw != "03/21/1962" {
		t.Fatalf("nascimento inesperado: %q", got.BirthDateRaw)
	}
	if got.ExamDateRaw != "10/27/2026" {
		t.Fatalf("data do exame inesperada: %q", got.ExamDateRaw)
	}
}

func TestExtractIdentityFromCresoAversaPentacam(t *testing.T) {
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
	data, err := os.ReadFile("../../../frontend/data/creso-aversa/PENTACAM CRESO AVERSA OS .pdf")
	if err != nil {
		t.Fatal(err)
	}

	got, err := ExtractIdentity(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}

	if got.FullName != "CRESO AVERSA MARTINELLI" || got.BirthDateRaw != "12/14/1979" || got.ExamDateRaw != "11/04/2026" {
		t.Fatalf("identidade inesperada: %#v", got)
	}
}
