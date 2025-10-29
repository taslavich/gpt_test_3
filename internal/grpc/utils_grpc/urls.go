package utils

import (
	"fmt"
	"net/url"
)

const (
	NURL = "nurl"
	BURL = "burl"
	ADM  = "adm"
)

func WrapURL(hostname, originalURL, globalId, admOrnurlOrBurl string) string {
	encodeUrl := url.QueryEscape(originalURL)
	return fmt.Sprintf("http://%s:8086/%s?id=%s&url=%s",
		hostname, admOrnurlOrBurl, globalId, encodeUrl)
}
