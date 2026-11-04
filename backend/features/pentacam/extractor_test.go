package pentacam

import "testing"

func TestParsePentacamFieldNumberRequiresPrintedDecimal(t *testing.T) {
	if _, ok := parsePentacamFieldNumber("K1 435D", `\bk1\b`, true); ok {
		t.Fatal("não pode transformar 435 em 43.5")
	}

	got, ok := parsePentacamFieldNumber("K1 43.5D", `\bk1\b`, true)
	if !ok || got != 43.5 {
		t.Fatalf("decimal impresso deveria ser aceito: %v %v", got, ok)
	}
}

func TestPentacamFocusedTKCDash(t *testing.T) {
	got, ok := pentacamFocusedTKC([]string{"CKI 1.01   TKC: —"})
	if !ok || got != "—" {
		t.Fatalf("TKC inesperado: %#v %v", got, ok)
	}
}
