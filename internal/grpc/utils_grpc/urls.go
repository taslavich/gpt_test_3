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

func WrapNurlURL(hostname, originalURL, globalId, ssp_domain, format string) string {
	encodeUrl := url.QueryEscape(originalURL)
	return fmt.Sprintf("https://%s/nurl?id=%s&url=%s&s=%s&f=%s",
		hostname, globalId, encodeUrl, ssp_domain, constants.FormatToCodes[format])
}

func WrapBurlURL(hostname, globalId, format string) string {
	return fmt.Sprintf("https://%s/burl?id=%s&f=%s",
		hostname, globalId, constants.FormatToCodes[format])
}
