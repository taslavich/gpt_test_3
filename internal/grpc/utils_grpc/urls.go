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

func WrapNurlURL(hostname, originalURL, globalId, format string) string {
	encodeUrl := url.QueryEscape(originalURL)
	return fmt.Sprintf("http://%s:80/nurl?id=%s&url=%s&f=%s",
		hostname, globalId, encodeUrl, constants.FormatToCodes[format])
}
