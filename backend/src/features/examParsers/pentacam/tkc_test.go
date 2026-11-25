package pentacam

import "testing"

func TestParseTKCCell(t *testing.T) {
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{"-", "—", true}, {"[-", "—", true}, {"|—|", "—", true}, {"–", "—", true},
		{"KK 2", "KC2", true}, {"KK 2-3", "KC2-3", true}, {"KC1", "KC1", true},
		{"kc 4", "KC4", true}, {"Poss.", "Poss.", true}, {"Possível", "Poss.", true},
		{"2", "", false}, {"2.600", "", false}, {"KK 5", "", false}, {"KK 2-4", "", false},
		{"KK", "", false}, {"I", "", false}, {"EW", "", false}, {"a", "", false},
		{"-0.35", "", false}, {"KC2 1.03", "", false}, {"", "", false},
		{"POSSIBLE", "Poss.", true}, {"POSSE", "", false}, {"Poss 2", "", false},
	} {
		got, ok := parseTKCCell(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("parseTKCCell(%q)=%q,%v want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestPentacamFocusedTKCStatus(t *testing.T) {
	for _, c := range []struct {
		name  string
		texts []string
		val   string
		st    string
	}{
		{"dash", []string{"TKC: -"}, "—", "valid"},
		{"dash bracket", []string{"CKI: 1.03 TKC: [-"}, "—", "valid"},
		{"class", []string{"CKI: 1.03 TKC: KK 2"}, "KC2", "valid"},
		{"missing", []string{"CKI: 1.03"}, "", "missing"},
		{"no texts", nil, "", "missing"},
		{"unreadable I", []string{"TKC: I"}, "", "unreadable"},
		{"unreadable EW", []string{"TKC: [EW"}, "", "unreadable"},
		{"bare digit", []string{"TKC: 2"}, "", "unreadable"},
		// Regression: the KISA cell on the next line must never leak into TKC.
		{"KISA contamination 1", []string{"CKI: |1.U1 TKC: I\nK|SAI|2.BUU IS: IU.99"}, "", "unreadable"},
		{"KISA contamination 2", []string{"TKC: I\n\nKISA:|2.600\n\nIS: IU.SS"}, "", "unreadable"},
		{"negative neighbour not dash", []string{"TKC: I\nIS: -0.35"}, "", "unreadable"},
		{"intermediate class not dash", []string{"TKC: KK 2-3"}, "KC2-3", "valid"},
		{"conflict dash vs class", []string{"TKC: -", "TKC: KK 2"}, "", "conflicting"},
		{"conflict class vs intermediate", []string{"TKC: KK 2", "TKC: KK 2-3"}, "", "conflicting"},
		{"agreement", []string{"TKC: KK 2", "x TKC: kc2"}, "KC2", "valid"},
		{"one reading, one unreadable", []string{"TKC: I", "TKC: KK 2"}, "KC2", "valid"},
		{"garbled label ignored", []string{"TK C: KK 3", "CKI: 1.03"}, "", "missing"},
	} {
		v, st := pentacamFocusedTKC(c.texts)
		if v != c.val || st != c.st {
			t.Errorf("%s: got %q,%q want %q,%q", c.name, v, st, c.val, c.st)
		}
	}
}

func TestPentacamTKCParserNoKISAContamination(t *testing.T) {
	if v, ok := pentacamTKC("TKC: I\nKISA: 2.600"); ok {
		t.Fatalf("got %q", v)
	}
	if v, ok := pentacamTKC("TKC -"); !ok || v != "—" {
		t.Fatalf("got %q %v", v, ok)
	}
}

func TestCombineTKC(t *testing.T) {
	for _, c := range []struct {
		name                       string
		focused, status, anchored  string
		wantVal, wantSt, wantStrat string
	}{
		{"both dash", "—", "valid", "—", "—", "valid", "focused_tkc+label_anchor"},
		{"focused dash, anchored silent", "—", "valid", "", "—", "valid", "focused_tkc"},
		{"anchored dash alone is not enough", "", "unreadable", "—", "", "unreadable", "focused_tkc"},
		{"anchored dash alone, focused missing", "", "missing", "—", "", "missing", "focused_tkc"},
		{"anchored class recovers a red cell", "", "unreadable", "KC2-3", "KC2-3", "valid", "label_anchor"},
		{"both class", "KC2", "valid", "KC2", "KC2", "valid", "focused_tkc+label_anchor"},
		{"stage disagreement", "KC2", "valid", "KC2-3", "", "conflicting", "focused_tkc+label_anchor"},
		{"dash vs class", "—", "valid", "KC1", "", "conflicting", "focused_tkc+label_anchor"},
		{"class vs dash", "KC1", "valid", "—", "", "conflicting", "focused_tkc+label_anchor"},
		{"focused conflicting stays conflicting", "", "conflicting", "KC2", "", "conflicting", "focused_tkc"},
		{"nothing read", "", "unreadable", "", "", "unreadable", "focused_tkc"},
	} {
		v, st, strat := combineTKC(c.focused, c.status, c.anchored)
		if v != c.wantVal || st != c.wantSt || strat != c.wantStrat {
			t.Errorf("%s: got %q,%q,%q want %q,%q,%q", c.name, v, st, strat, c.wantVal, c.wantSt, c.wantStrat)
		}
	}
}
