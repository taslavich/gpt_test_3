package utils

import (
	"fmt"
)

const (
	NURL = "nurl"
	BURL = "burl"
	ADM  = "adm"
)

func WrapURL(hostname, originalURL, globalId, admOrnurlOrBurl string) string {
	if originalURL == "" {
		return ""
	}
	return fmt.Sprintf("http://%s:8086/%s?id=%s&url=%s",
		hostname, admOrnurlOrBurl, globalId, originalURL)
}
