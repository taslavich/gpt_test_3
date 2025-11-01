package utils

import (
	"fmt"
	"net/url"
)

const (
	NURL = "sun"
	ADM  = "in"
)

func WrapURL(hostname, originalURL, globalId, admOrnurlOrBurl string) string {
	encodeUrl := url.QueryEscape(originalURL)
	return fmt.Sprintf("https://%s/%s?ifo=%s&sego=%s",
		hostname, admOrnurlOrBurl, globalId, encodeUrl)
}
