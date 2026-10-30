package domain

func ClinicalAttachmentLevel(probingDepth, gingivalMargin float64) float64 {
	// A positive margin means recession apical to the cemento-enamel junction.
	return probingDepth + gingivalMargin
}
