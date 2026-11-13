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

func TestABCDAndNotEvaluable(t *testing.T) {
	if composeABCD(map[string]any{"a": 0.2, "b": 0.3, "c": 0.3, "d": nil}) != "A0 B0 C0 D" {
		t.Fatal("abcd compose")
	}
	ps := []*PatientRun{{Slug: "x", Fields: []FieldRes{{ExamType: "e", Field: "f", Outcome: "missing"}, {ExamType: "e", Field: "g", Outcome: "missing", PathSeen: true}}}}
	markContractMismatch(ps)
	if ps[0].Fields[0].Outcome != "contract_mismatch" || ps[0].Fields[1].Outcome != "missing" {
		t.Fatal("contract_mismatch only when the path is absent everywhere")
	}
	g := &GT{}
	g.Expected.Count = 2
	sn := &Snaps{Pre: &Snap{}, Post1: &Snap{Objects: map[string]int64{}}, Post2: &Snap{}, Post3: &Snap{}, CS1: 200, CS2: 200, CS3: 200}
	cks, _ := evalPersistence(g, sn)
	ne := 0
	for _, c := range cks {
		if c.NE {
			ne++
		}
	}
	if ne != 4 {
		t.Fatalf("with 0 rows 4 checks are not evaluable, got %d", ne)
	}
}
