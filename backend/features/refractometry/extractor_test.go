package refractometry

import (
	"context"
	"os"
	"testing"
)

func TestExtractPDFMariaGoreteAutoRecoversBothEyes(t *testing.T) {
	data, err := os.ReadFile(
		"../../../frontend/data/maria-gorete/AUTO AO MARIA GORETE.pdf",
	)
	if err != nil {
		t.Fatal(err)
	}

	exam, err := ExtractPDF(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}

	eyes, ok := exam["eyes"].(map[string]any)
	if !ok {
		t.Fatalf("olhos ausentes: %#v", exam)
	}

	for _, eye := range []string{"OD", "OS"} {
		payload, ok := eyes[eye].(map[string]any)
		if !ok {
			t.Fatalf("olho %s ausente: %#v", eye, eyes)
		}
		for _, field := range []string{"sphere_d", "cylinder_d", "axis_deg"} {
			if _, ok := payload[field].(float64); !ok {
				t.Fatalf("campo %s.%s ausente: %#v", eye, field, payload)
			}
		}
	}
}
