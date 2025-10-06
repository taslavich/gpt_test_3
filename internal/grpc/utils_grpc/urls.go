package utils

import (
	"fmt"
)

const (
	NURL = "nurl"
	BURL = "burl"
)

func WrapURL(hostname, originalURL, globalId, nurlOrBurl string) string {
	if originalURL == "" {
		return ""
	}
	return fmt.Sprintf("https://%s/bidRequest/%s?id=%s&url=%s",
		hostname, nurlOrBurl, globalId, originalURL)
}
