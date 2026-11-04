package pentacam

import (
	"context"
	"os"
	"reflect"
	"testing"
)

func BenchmarkExtractPDFStrategies(b *testing.B) {
	data, errorValue := os.ReadFile("../../../frontend/data/maria-gorete/PENTACAM OD MARIA GORETE.pdf")
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
		name           string
		parallelRender bool
	}{
		{name: "serial", parallelRender: false},
		{name: "parallel_render", parallelRender: true},
	} {
		b.Run(strategy.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, errorValue := extractPDF(context.Background(), data, strategy.parallelRender); errorValue != nil {
					b.Fatal(errorValue)
				}
			}
		})
	}
}
