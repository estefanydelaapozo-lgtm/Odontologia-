package domain

type FeeRule struct {
	Type string
	Rate float64
}

func CalculateProfessionalFee(gross, laboratory, materials float64, rule FeeRule) float64 {
	switch rule.Type {
	case "fijo":
		return rule.Rate
	case "porcentaje_neto":
		return max(0, gross-laboratory-materials) * rule.Rate / 100
	default:
		return gross * rule.Rate / 100
	}
}
