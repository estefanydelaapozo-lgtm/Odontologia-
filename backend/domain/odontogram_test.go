package domain

import "testing"

func TestNormalizeToFDIFromUniversalPermanent(t *testing.T) {
	cases := []struct{ universal, fdi string }{
		{"1", "18"}, {"8", "11"}, {"9", "21"}, {"16", "28"},
		{"17", "38"}, {"24", "31"}, {"25", "41"}, {"32", "48"},
	}
	for _, tc := range cases {
		got, err := NormalizeToFDI(tc.universal, "UNIVERSAL")
		if err != nil || got != tc.fdi {
			t.Errorf("NormalizeToFDI(%q, UNIVERSAL) = %q, %v; want %q", tc.universal, got, err, tc.fdi)
		}
	}
}

func TestPrimaryDentitionRoundTrip(t *testing.T) {
	for _, letter := range []string{"A", "E", "F", "J", "K", "O", "P", "T"} {
		fdi, err := NormalizeToFDI(letter, "UNIVERSAL")
		if err != nil {
			t.Fatalf("NormalizeToFDI(%q): %v", letter, err)
		}
		got, err := ToUniversal(fdi)
		if err != nil || got != letter {
			t.Errorf("ToUniversal(%q) = %q, %v; want %q", fdi, got, err, letter)
		}
	}
}

func TestRejectInvalidToothAndSurface(t *testing.T) {
	if _, err := NormalizeToFDI("99", "FDI"); err == nil {
		t.Fatal("expected invalid FDI tooth to be rejected")
	}
	if err := ValidateSurfaces([]string{"mesial", "vestibular"}); err != nil {
		t.Fatalf("valid surfaces rejected: %v", err)
	}
	if err := ValidateSurfaces([]string{"mesial", "unknown"}); err == nil {
		t.Fatal("expected invalid surface to be rejected")
	}
}
