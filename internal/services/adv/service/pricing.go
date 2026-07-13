package auction

import (
	"math"
	"strings"
)

const (
	PricingModelCPM = "CPM"
	PricingModelCPC = "CPC"
)

func normalizedAuctionFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "ipp", "in-page-push", "in_page_push":
		return "ipp"
	case "nat", "native":
		return "nat"
	case "ban", "banner":
		return "ban"
	case "pop", "popunder", "pop_under":
		return "pop"
	default:
		return strings.ToLower(strings.TrimSpace(format))
	}
}

func ChargePrice(base float64, pricingModel, format string) float64 {
	if base < 0 || math.IsNaN(base) || math.IsInf(base, 0) {
		return 0
	}
	model := strings.ToUpper(strings.TrimSpace(pricingModel))
	switch model {
	case PricingModelCPM:
		return base / 1000
	case PricingModelCPC:
		if normalizedAuctionFormat(format) == "pop" {
			return base / 1000
		}
		return base
	default:
		return 0
	}
}

func EffectivePrice(base float64, pricingModel, format string, deduction float64) float64 {
	if deduction < 0 || deduction > 1 || math.IsNaN(deduction) || math.IsInf(deduction, 0) {
		return 0
	}
	charge := ChargePrice(base, pricingModel, format)
	if charge <= 0 || math.IsNaN(charge) || math.IsInf(charge, 0) {
		return 0
	}
	return charge * (1 - deduction)
}
