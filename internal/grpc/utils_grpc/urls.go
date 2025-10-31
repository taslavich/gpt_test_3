package utils

import (
	"encoding/base64"
	"fmt"
)

const (
	NURL = "nurl"
	BURL = "burl"
	ADM  = "adm"
)

func WrapURL(hostname, originalURL, globalId, admOrnurlOrBurl string) string {
	encodeUrl := base64.RawURLEncoding.EncodeToString([]byte(originalURL))
	return fmt.Sprintf("https://%s/%s?id=%s&url=%s",
		hostname, admOrnurlOrBurl, globalId, encodeUrl)
}
