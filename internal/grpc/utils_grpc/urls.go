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

func WrapURL(hostname, originalURL, globalId, format string) string {
	if globalId == "" {
		log.Println("Empty globalId in WrapURL")
	}

	encodeUrl := url.QueryEscape(originalURL)
	return fmt.Sprintf("https://%s/adm?id=%s&url=%s&f=%s",
		hostname, globalId, encodeUrl, constants.FormatToCodes[format])
}

func WrapNurlURL(hostname, originalURL, globalId, ssp_domain string) string {
	if globalId == "" {
		log.Printf("Empty globalId in WrapNurlURL, DSP_DOMAIN: %s", ssp_domain)
	}
	encodeUrl := url.QueryEscape(originalURL)
	return fmt.Sprintf("http://%s:80/nurl?id=%s&url=%s&s=%s",
		hostname, globalId, encodeUrl, ssp_domain)
}

func WrapBurlURL(hostname, globalId, format string) string {
	if globalId == "" {
		log.Println("Empty globalId in WrapBurlURL")
	}
	return fmt.Sprintf("http://%s:80/burl?id=%s&f=%s",
		hostname, globalId, constants.FormatToCodes[format])
}
