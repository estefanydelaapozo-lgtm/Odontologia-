package domain

import "testing"

func TestCalculateProfessionalFee(t *testing.T) {
	cases := []struct {
		name                  string
		gross, lab, materials float64
		rule                  FeeRule
		want                  float64
	}{
		{"fixed", 100, 20, 10, FeeRule{Type: "fijo", Rate: 25}, 25},
		{"gross percentage", 100, 20, 10, FeeRule{Type: "porcentaje", Rate: 30}, 30},
		{"net percentage", 100, 20, 10, FeeRule{Type: "porcentaje_neto", Rate: 50}, 35},
		{"net cannot be negative", 10, 20, 10, FeeRule{Type: "porcentaje_neto", Rate: 50}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CalculateProfessionalFee(tc.gross, tc.lab, tc.materials, tc.rule); got != tc.want {
				t.Errorf("got %v; want %v", got, tc.want)
			}
		})
	}
}
