package eyesuite

import (
	"context"
	"os"
	"reflect"
	"testing"
)

func BenchmarkExtractPDFStrategies(b *testing.B) {
	data, errorValue := os.ReadFile("../../../../../frontend/data/maria-gorete/BIOMETRIA AO MARIA GORETE.pdf")
	if errorValue != nil {
		b.Fatal(errorValue)
	}

	serial, errorValue := extractPDF(context.Background(), data, false)
	if errorValue != nil {
		b.Fatal(errorValue)
	}
	parallel, errorValue := extractPDF(context.Background(), data, true)
	if errorValue != nil {
		b.Fatal(errorValue)
	}
	if !reflect.DeepEqual(serial, parallel) {
		b.Fatalf("estratégias produziram resultados diferentes:\nserial=%#v\nparallel=%#v", serial, parallel)
	}

	for _, strategy := range []struct {
		name        string
		parallelDPI bool
	}{
		{name: "serial", parallelDPI: false},
		{name: "parallel_dpi", parallelDPI: true},
	} {
		b.Run(strategy.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, errorValue := extractPDF(context.Background(), data, strategy.parallelDPI); errorValue != nil {
					b.Fatal(errorValue)
				}
			}
		})
	}
}

func TestExtractPDFMariaGoreteIdentity(t *testing.T) {
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
	data, err := os.ReadFile("../../../../../frontend/data/maria-gorete/BIOMETRIA AO MARIA GORETE.pdf")
	if err != nil {
		t.Fatal(err)
	}

	result, err := ExtractPDF(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if result.Identity == nil {
		t.Fatalf("identidade não extraída: %#v", result)
	}
	if result.Identity.TimestampRaw == "" {
		t.Fatalf("timestamp não extraído: %#v", result.Identity)
	}
}
