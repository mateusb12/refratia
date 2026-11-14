package pentacam

import (
	"testing"

	"refratia/backend/src/shared/ocr"
)

func TestScreenForTitle(t *testing.T) {
	cases := map[string]Screen{
		"Topométrico / Estadiamento KC":    ScreenTopometric,
		"Paquimétrico":                     ScreenPachymetry,
		"Cataract Pre-OP":                  ScreenCataract,
		"Belin/Ambrósio Ectasia Reforçada": ScreenBelin,
		"Anéis Corneanos":                  ScreenRings,
		"4 Mapas Refrativos":               ScreenRefractive,
		"Mapas":                            "",
		"Aberrometry Cornea":               ScreenAberrometry,
		"Geral":                            ScreenGeneral,
		"something else":                   "",
	}
	for title, want := range cases {
		if got := ScreenForTitle(title); got != want {
			t.Errorf("ScreenForTitle(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestTitleAfterHeaderIgnoresRowsWithoutHeader(t *testing.T) {
	words := []ocr.Word{
		{Text: "OCULUS", Left: 10, Top: 100, Width: 80, Height: 20},
		{Text: "PENTACAM", Left: 150, Top: 101, Width: 90, Height: 20},
		{Text: "Paquimétrico", Left: 260, Top: 102, Width: 120, Height: 20},
		{Text: "Paquimétrico", Left: 10, Top: 400, Width: 120, Height: 20}, // body text, other row
	}
	if got := titleAfterHeader(words, 12); got != "Paquimétrico" {
		t.Fatalf("got %q", got)
	}
	if got := titleAfterHeader(words[3:], 12); got != "" {
		t.Fatalf("got %q for a page without header", got)
	}
}

func TestEyeFromText(t *testing.T) {
	for text, want := range map[string]string{
		"Olho: Direito": "OD", "olho esquerdo": "OS",
		"direito esquerdo": "", "nothing": "",
	} {
		if got := eyeFromText(text); got != want {
			t.Errorf("eyeFromText(%q)=%q want %q", text, got, want)
		}
	}
}
