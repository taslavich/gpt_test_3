package utils

import (
	"encoding/base64"
	"fmt"
)

const (
	NURL = "sun"
	ADM  = "in"
)

func WrapURL(hostname, originalURL, globalId, admOrnurlOrBurl string) string {
	encodeUrl := base64.RawURLEncoding.EncodeToString([]byte(originalURL))
	return fmt.Sprintf("https://%s/%s?ifo=%s&sego=%s",
		hostname, admOrnurlOrBurl, globalId, encodeUrl)
}
