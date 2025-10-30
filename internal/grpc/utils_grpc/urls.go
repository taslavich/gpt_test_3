package utils

import (
	"encoding/base64"
	"fmt"
	"net/url"
)

const (
	NURL = "nurl"
	BURL = "burl"
	ADM  = "adm"
)

type CallbackHost struct {
	Scheme string
	Host   string
	Port   uint16
}

func (h CallbackHost) normalizedScheme() string {
	if h.Scheme == "" {
		return "https"
	}
	return h.Scheme
}

func (h CallbackHost) normalizedHost() string {
	return h.Host
}

func (h CallbackHost) baseURL() string {
	scheme := h.normalizedScheme()
	host := h.normalizedHost()
	port := h.Port

	if host == "" {
		return ""
	}

	if port == 0 || (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
		return fmt.Sprintf("%s://%s", scheme, host)
	}

	return fmt.Sprintf("%s://%s:%d", scheme, host, port)
}

func encodeDSPURL(kind, originalURL string) string {
	if kind == ADM {
		return base64.RawURLEncoding.EncodeToString([]byte(originalURL))
	}

	return url.QueryEscape(originalURL)
}

func WrapURL(host CallbackHost, originalURL, globalId, admOrnurlOrBurl string) string {
	baseURL := host.baseURL()
	encodedURL := encodeDSPURL(admOrnurlOrBurl, originalURL)

	if baseURL == "" {
		return originalURL
	}

	return fmt.Sprintf("%s/%s?id=%s&url=%s",
		baseURL, admOrnurlOrBurl, globalId, encodedURL)
}

func DecodeWrappedURL(kind, encoded string) (string, error) {
	if kind == ADM {
		decoded, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return "", err
		}

		return string(decoded), nil
	}

	return url.QueryUnescape(encoded)
}
