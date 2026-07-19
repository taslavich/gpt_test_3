package auction

import (
	"math"
	"strings"
)

func CalculateChargePrice(basePrice float64, pricingModel, format string) float64 {
	if basePrice <= 0 || math.IsNaN(basePrice) || math.IsInf(basePrice, 0) {
		return 0
	}
	switch strings.ToUpper(strings.TrimSpace(pricingModel)) {
	case PricingModelCPM:
		return basePrice / 1000
	case PricingModelCPC:
		if normalizeFormat(format) == "POP" {
			return basePrice / 1000
		}
		return basePrice
	default:
		return 0
	}
}

func CalculateEffectiveAuctionPrice(basePrice, deductionPercent float64) float64 {
	if basePrice <= 0 || math.IsNaN(basePrice) || math.IsInf(basePrice, 0) {
		return 0
	}
	if math.IsNaN(deductionPercent) || math.IsInf(deductionPercent, 0) || deductionPercent < 0 || deductionPercent > 1 {
		return 0
	}
	return basePrice * (1 - deductionPercent)
}
