package domain

import (
	"fmt"
	"strconv"
	"strings"
)

var dentalSurfaces = map[string]struct{}{"mesial": {}, "distal": {}, "oclusal": {}, "incisal": {}, "vestibular": {}, "lingual": {}, "palatino": {}}

func NormalizeToFDI(code, notation string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	notation = strings.ToUpper(strings.TrimSpace(notation))
	if notation == "FDI" {
		if validFDI(code) {
			return code, nil
		}
		return "", fmt.Errorf("pieza FDI inválida: %s", code)
	}
	if notation != "UNIVERSAL" {
		return "", fmt.Errorf("notación dental no reconocida")
	}
	if len(code) == 1 && code[0] >= 'A' && code[0] <= 'T' {
		return universalPrimaryToFDI(code), nil
	}
	n, err := strconv.Atoi(code)
	if err != nil || n < 1 || n > 32 {
		return "", fmt.Errorf("pieza Universal inválida: %s", code)
	}
	switch {
	case n <= 8:
		return fmt.Sprintf("1%d", 9-n), nil
	case n <= 16:
		return fmt.Sprintf("2%d", n-8), nil
	case n <= 24:
		return fmt.Sprintf("3%d", 25-n), nil
	default:
		return fmt.Sprintf("4%d", n-24), nil
	}
}

func ToUniversal(code string) (string, error) {
	fdi, err := NormalizeToFDI(code, "FDI")
	if err != nil {
		return "", err
	}
	q, d := int(fdi[0]-'0'), int(fdi[1]-'0')
	if q >= 1 && q <= 4 {
		var n int
		switch q {
		case 1:
			n = 9 - d
		case 2:
			n = 8 + d
		case 3:
			n = 25 - d
		case 4:
			n = 24 + d
		}
		return strconv.Itoa(n), nil
	}
	return fdiPrimaryToUniversal(q, d), nil
}

func ValidateSurfaces(surfaces []string) error {
	for _, s := range surfaces {
		if _, ok := dentalSurfaces[strings.ToLower(strings.TrimSpace(s))]; !ok {
			return fmt.Errorf("superficie dental no válida: %s", s)
		}
	}
	return nil
}
func validFDI(code string) bool {
	if len(code) != 2 {
		return false
	}
	q, d := int(code[0]-'0'), int(code[1]-'0')
	return (q >= 1 && q <= 4 && d >= 1 && d <= 8) || (q >= 5 && q <= 8 && d >= 1 && d <= 5)
}
func universalPrimaryToFDI(code string) string {
	n := int(code[0] - 'A')
	switch {
	case n <= 4:
		return fmt.Sprintf("5%d", 5-n)
	case n <= 9:
		return fmt.Sprintf("6%d", n-4)
	case n <= 14:
		return fmt.Sprintf("7%d", 15-n)
	default:
		return fmt.Sprintf("8%d", n-14)
	}
}
func fdiPrimaryToUniversal(q, d int) string {
	switch q {
	case 5:
		return string(rune('A' + 5 - d))
	case 6:
		return string(rune('A' + 5 + d - 1))
	case 7:
		return string(rune('A' + 15 - d))
	case 8:
		return string(rune('A' + 15 + d - 1))
	}
	return ""
}
