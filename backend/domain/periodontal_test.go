package domain

import "testing"

func TestClinicalAttachmentLevel(t *testing.T) {
	cases := []struct{ probing, margin, want float64 }{{3, 2, 5}, {4, -1, 3}, {2, 0, 2}}
	for _, tc := range cases {
		if got := ClinicalAttachmentLevel(tc.probing, tc.margin); got != tc.want {
			t.Errorf("ClinicalAttachmentLevel(%v, %v) = %v; want %v", tc.probing, tc.margin, got, tc.want)
		}
	}
}
