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

func WrapURL(hostname, originalURL, globalId, admOrnurlOrBurl, format string) string {
	encodeUrl := url.QueryEscape(originalURL)
	return fmt.Sprintf("https://%s/%s?id=%s&url=%s&f=%s",
		hostname, admOrnurlOrBurl, globalId, encodeUrl, constants.FormatToCodes[format])
}
