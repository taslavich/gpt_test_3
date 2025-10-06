package utils

import (
	"fmt"
)

const (
	NURL = "nurl"
	BURL = "burl"
)

func WrapURL(hostname, originalURL, globalId, isItNurlOrBurl string) string {
	if originalURL == "" {
		return ""
	}
	return fmt.Sprintf("https://%s/%s?id=%s&url=%s",
		hostname, isItNurlOrBurl, globalId, originalURL)
}
