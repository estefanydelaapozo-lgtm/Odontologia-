package controllers

import "testing"

func TestClinicalNoteEditHours(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
		valid bool
	}{
		{name: "default", value: "", want: 24, valid: true},
		{name: "configured", value: "72", want: 72, valid: true},
		{name: "zero", value: "0"},
		{name: "too large", value: "721"},
		{name: "not an integer", value: "one day"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CLINICAL_NOTE_EDIT_HOURS", test.value)
			got, err := clinicalNoteEditHours()
			if test.valid && (err != nil || got != test.want) {
				t.Fatalf("clinicalNoteEditHours() = (%d, %v), want (%d, nil)", got, err, test.want)
			}
			if !test.valid && err == nil {
				t.Fatalf("clinicalNoteEditHours() accepted invalid value %q", test.value)
			}
		})
	}
}
