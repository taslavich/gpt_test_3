package utils

import (
	"net/url"
	"strings"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
)

const (
	NURL = "nurl"
	ADM  = "adm"
)

func WrapURL(hostname, originalURL, globalID, format string) string {
	return buildCallbackURL(hostname, ADM, map[string]string{
		"id":  globalID,
		"url": originalURL,
		"f":   formatCode(format),
	}, "id", "url", "f")
}

func WrapNurlURL(hostname, originalURL, globalID, sspDomain, format string) string {
	return buildCallbackURL(hostname, NURL, map[string]string{
		"id":  globalID,
		"url": originalURL,
		"s":   sspDomain,
		"f":   formatCode(format),
	}, "id", "url", "s", "f")
}

func WrapBurlURL(hostname, globalID, format string) string {
	return buildCallbackURL(hostname, "burl", map[string]string{
		"id": globalID,
		"f":  formatCode(format),
	}, "id", "f")
}

func formatCode(format string) string {
	return constants.FormatToCodes[strings.ToUpper(strings.TrimSpace(format))]
}

func buildCallbackURL(hostname, path string, values map[string]string, required ...string) string {
	hostname = strings.TrimSpace(hostname)
	path = strings.TrimSpace(path)
	if hostname == "" || path == "" {
		return ""
	}
	for _, key := range required {
		if strings.TrimSpace(values[key]) == "" {
			return ""
		}
	}
	query := make(url.Values, len(values))
	for key, value := range values {
		query.Set(key, strings.TrimSpace(value))
	}
	return (&url.URL{Scheme: "https", Host: hostname, Path: "/" + path, RawQuery: query.Encode()}).String()
}
