package utils

import (
	"fmt"
	"log"
	"net/url"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
)

const (
	NURL = "nurl"
	ADM  = "adm"
)

func WrapURL(hostname, originalURL, globalId, admOrnurlOrBurl, format string) string {
	if globalId == "" {
		log.Printf("Empty uuid")
	}

	encodeUrl := url.QueryEscape(originalURL)
	return fmt.Sprintf("https://%s/%s?id=%s&url=%s&f=%s",
		hostname, admOrnurlOrBurl, globalId, encodeUrl, constants.FormatToCodes[format])
}
