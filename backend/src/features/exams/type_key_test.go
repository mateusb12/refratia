package exams

import "testing"

func TestExamTypeKeyNeverRemapsUnknownOrOutOfProtocol(t *testing.T) {
	for in, want := range map[string]string{
		"ANTSEG": "anterior_segment_photo", "anterior_segment_photo": "anterior_segment_photo",
		"TOPO": "corneal_topography_other", "PENTACAM": "pentacam_corneal_tomography",
		"mystery": "unclassified", "": "unclassified",
	} {
		if got := TypeKey(in); got != want {
			t.Errorf("examTypeKey(%q)=%q want %q", in, got, want)
		}
	}
}
