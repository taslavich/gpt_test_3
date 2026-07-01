package utils

import (
	"fmt"
	"net/url"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
)

const (
	NURL = "nurl"
	ADM  = "adm"
)

func WrapURL(hostname, originalURL, globalId, format string) string {
	encodeUrl := url.QueryEscape(originalURL)
	return fmt.Sprintf("https://%s/adm?id=%s&url=%s&f=%s",
		hostname, globalId, encodeUrl, constants.FormatToCodes[format])
}

func WrapNurlURL(hostname, originalURL, globalId string) string {
	encodeUrl := url.QueryEscape(originalURL)
	return fmt.Sprintf("http://%s:80/nurl?id=%s&url=%s",
		hostname, globalId, encodeUrl)
}

func WrapBurlURL(hostname, globalId, format string) string {
	return fmt.Sprintf("http://%s:80/burl?id=%s&f=%s",
		hostname, globalId, constants.FormatToCodes[format])
}
