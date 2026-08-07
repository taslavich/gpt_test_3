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
	advNURL := WrapADVNurlURL(host, globalID, sspDomain, format)
	assertQuery(advNURL, map[string]string{
		"id": globalID, "s": sspDomain, "f": constants.FormatToCodes[format],
	})
	parsedADVNURL, err := url.Parse(advNURL)
	if err != nil {
		t.Fatalf("parse ADV NURL %q: %v", advNURL, err)
	}
	if _, exists := parsedADVNURL.Query()["url"]; exists {
		t.Fatalf("ADV NURL must not contain url query: %q", advNURL)
	}
	assertQuery(WrapBurlURL(host, globalID, format), map[string]string{
		"id": globalID, "f": constants.FormatToCodes[format],
	})
	assertQuery(WrapClicksWinsURL(host, globalID), map[string]string{
		"id": globalID,
	})
}

func TestCallbackWrappersRejectMissingRequiredValues(t *testing.T) {
	if got := WrapURL("adm.example.test", "", "id", constants.BAN); got != "" {
		t.Fatalf("ADM wrapper must reject empty redirect URL: %q", got)
	}
	if got := WrapNurlURL("adm.example.test", "", "id", "ssp.example.test", constants.BAN); got != "" {
		t.Fatalf("NURL wrapper must reject empty redirect URL: %q", got)
	}
	if got := WrapADVNurlURL("adm.example.test", "", "ssp.example.test", constants.BAN); got != "" {
		t.Fatalf("ADV NURL wrapper must reject empty winner UUID: %q", got)
	}
	if got := WrapBurlURL("adm.example.test", "id", "unknown"); got != "" {
		t.Fatalf("BURL wrapper must reject unknown format: %q", got)
	}
	if got := WrapClicksWinsURL("adm.example.test", ""); got != "" {
		t.Fatalf("clicks_wins wrapper must reject empty winner UUID: %q", got)
	}
}

func TestCallbackWrappersNormalizeFormat(t *testing.T) {
	got := WrapBurlURL("adm.example.test", "id", " ban ")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/burl" || parsed.Query().Get("id") != "id" || parsed.Query().Get("f") != constants.FormatToCodes[constants.BAN] {
		t.Fatalf("unexpected normalized callback URL: %s", got)
	}
}
