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
	return fmt.Sprintf("http://%s:8086/%s?id=%s&url=%s",
		hostname, nurlOrBurl, globalId, originalURL)
}
