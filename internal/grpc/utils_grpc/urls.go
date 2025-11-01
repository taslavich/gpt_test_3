package utils

import (
	"fmt"
	"net/url"
)

const (
	NURL = "nurl"
	ADM  = "adm"
)

func WrapURL(hostname, originalURL, globalId, admOrnurlOrBurl string) string {
	encodeUrl := url.QueryEscape(originalURL)
	return fmt.Sprintf("https://%s/%s?id=%s&url=%s",
		hostname, admOrnurlOrBurl, globalId, encodeUrl)
}
