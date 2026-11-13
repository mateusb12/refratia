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

func TestNameAndDuplicates(t *testing.T) {
	if ok, _ := nameCmp("AVERSA MARTINELLI CRESO", "Creso Aversa Martinelli"); !ok {
		t.Fatal("token multiset compare")
	}
	if ok, _ := nameCmp("DANIELA JNOGUEIRA ROCHA", "Daniela Nogueira Rocha"); ok {
		t.Fatal("typo must not match")
	}
	if structural([]string{"object size changed: x 1 -> 2"}) || !structural([]string{"object added: y"}) {
		t.Fatal("duplicate definition")
	}
}
