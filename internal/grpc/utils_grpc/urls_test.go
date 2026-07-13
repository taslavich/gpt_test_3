package utils

import (
	"net/url"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
)

func TestADVCallbackWrappersAlwaysIncludeFormat(t *testing.T) {
	const (
		host      = "adm.example.test"
		globalID  = "winner-uuid"
		original  = "https://creative.example/path?a=1&b=two words"
		sspDomain = "ssp.example.test"
		format    = constants.BAN
	)

	assertQuery := func(rawURL string, expected map[string]string) {
		t.Helper()
		parsed, err := url.Parse(rawURL)
		if err != nil {
			t.Fatalf("parse %q: %v", rawURL, err)
		}
		for key, value := range expected {
			if got := parsed.Query().Get(key); got != value {
				t.Fatalf("%s: query %s=%q, want %q", rawURL, key, got, value)
			}
		}
	}

	assertQuery(WrapURL(host, original, globalID, format), map[string]string{
		"id": globalID, "url": original, "f": constants.FormatToCodes[format],
	})
	assertQuery(WrapNurlURL(host, original, globalID, sspDomain, format), map[string]string{
		"id": globalID, "url": original, "s": sspDomain, "f": constants.FormatToCodes[format],
	})
	assertQuery(WrapBurlURL(host, globalID, format), map[string]string{
		"id": globalID, "f": constants.FormatToCodes[format],
	})
}
