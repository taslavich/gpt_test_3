package validation

import (
	"fmt"
	"mime/multipart"
	"strings"
)

var allowedVerticals = map[string]bool{"dating": true, "sweepstakes": true, "crypto": true, "nutra": true, "utilities": true}
var allowedGEOs = map[string]bool{"DE": true, "US": true, "FR": true, "ES": true, "IT": true, "BR": true, "IN": true, "RU": true}
var allowedLanguages = map[string]bool{"en": true, "de": true, "fr": true, "es": true, "it": true, "pt": true, "ru": true}
var allowedVisualModes = map[string]bool{"upload": true, "generate": true}

func GenerateRequest(vertical, geo, language, offerURL, visualMode string, variantsCount int, fileHeader *multipart.FileHeader) error {
	if !allowedVerticals[vertical] {
		return fmt.Errorf("unsupported vertical")
	}
	if !allowedGEOs[geo] {
		return fmt.Errorf("unsupported geo")
	}
	if !allowedLanguages[language] {
		return fmt.Errorf("unsupported language")
	}
	if !allowedVisualModes[visualMode] {
		return fmt.Errorf("unsupported visual_mode")
	}
	if !strings.HasPrefix(offerURL, "http://") && !strings.HasPrefix(offerURL, "https://") {
		return fmt.Errorf("offer_url must start with http:// or https://")
	}
	if variantsCount < 1 || variantsCount > 10 {
		return fmt.Errorf("variants_count must be between 1 and 10")
	}
	if visualMode == "upload" && fileHeader == nil {
		return fmt.Errorf("uploaded_visual is required when visual_mode=upload")
	}
	if fileHeader != nil && !strings.HasPrefix(fileHeader.Header.Get("Content-Type"), "image/") {
		return fmt.Errorf("uploaded_visual must be an image")
	}
	return nil
}
