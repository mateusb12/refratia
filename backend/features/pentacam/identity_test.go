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
}

func TestExtractIdentityFromMariaGoretePentacam(t *testing.T) {
	data, err := os.ReadFile("../../../frontend/data/maria-gorete/PENTACAM OD MARIA GORETE.pdf")
	if err != nil {
		t.Fatal(err)
	}

	got, err := ExtractIdentity(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}

	if got.FullName != "MARIA GORETE DE CARVALHO" {
		t.Fatalf("nome inesperado: %q", got.FullName)
	}
	if got.BirthDateRaw != "03/21/1962" {
		t.Fatalf("nascimento inesperado: %q", got.BirthDateRaw)
	}
}
