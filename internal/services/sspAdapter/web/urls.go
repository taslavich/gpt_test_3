package sppAdapterWeb

import "strings"

func fixAllUnicodeEscapes(url string) string {
	fixed := strings.ReplaceAll(url, "\\u0026", "&")  // &
	fixed = strings.ReplaceAll(fixed, "\\u003d", "=") // =
	fixed = strings.ReplaceAll(fixed, "\\u0025", "%") // %
	fixed = strings.ReplaceAll(fixed, "\\u002b", "+") // +
	fixed = strings.ReplaceAll(fixed, "\\u0020", " ") // space
	return fixed
}
