package main

import "testing"

func TestCompareAndAge(t *testing.T) {
	dp := 1
	sp := SpecField{Decimals: &dp}
	if !compareField("k1_d", FieldValue{Value: 43.2}, sp, 43.24) || compareField("k1_d", FieldValue{Value: 43.2}, sp, 43.3) {
		t.Fatal("numeric rounding compare")
	}
	if !compareField("coma_z31_5mm_um", FieldValue{Value: 0.097}, SpecField{Decimals: func() *int { d := 3; return &d }()}, "0.097 µm @ 151°") {
		t.Fatal("coma string")
	}
	if !compareField("x", FieldValue{Value: "Autorrefração"}, SpecField{}, "autorrefracao") {
		t.Fatal("accent fold")
	}
	a := map[string]any{"patient": map[string]any{"birth_date": ""}, "age": 0.0}
	if len(ageViolations(a)) == 0 {
		t.Fatal("age 0 must be flagged")
	}
}
