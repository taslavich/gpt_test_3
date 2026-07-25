package bidEngine

import (
	"encoding/json"
	"net/url"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestFinalizeADVCallbacksDoNotEmbedDSPCallbacks(t *testing.T) {
	adm := "https://creative.example/render?a=1"
	unexpectedNURL := "https://dsp.example/win"
	unexpectedBURL := "https://dsp.example/bill"
	bid := &ortb.Bid{Adm: &adm, Nurl: &unexpectedNURL, Burl: &unexpectedBURL}

	got, ok := FinalizeADVCallbacks(bid, "callbacks.example", "winner-1", "ssp.example", constants.BAN)
	if !ok || got == nil {
		t.Fatal("ADV callback finalization failed")
	}
	assertCallbackQuery(t, got.GetAdm(), "/adm", map[string]string{
		"id": "winner-1", "url": adm, "f": constants.FormatToCodes[constants.BAN],
	})
	assertCallbackQuery(t, got.GetNurl(), "/nurl", map[string]string{
		"id": "winner-1", "s": "ssp.example", "f": constants.FormatToCodes[constants.BAN],
	})
	assertCallbackQuery(t, got.GetBurl(), "/burl", map[string]string{
		"id": "winner-1", "f": constants.FormatToCodes[constants.BAN],
	})
	assertQueryMissing(t, got.GetNurl(), "url")
	assertQueryMissing(t, got.GetBurl(), "url")

	if bid.GetAdm() != adm || bid.GetNurl() != unexpectedNURL || bid.GetBurl() != unexpectedBURL {
		t.Fatal("source bid was mutated")
	}
}

func TestFinalizeADVNativeCallbacksWrapOnlyLinkAndAddBURL(t *testing.T) {
	adm := `{"native":{"ver":"1.2","link":{"url":"https://creative.example/render?a=1"},"assets":[{"id":100,"title":{"text":"Title"}}]}}`
	unexpectedNURL := "https://dsp.example/win"
	unexpectedBURL := "https://dsp.example/bill"
	bid := &ortb.Bid{Adm: &adm, Nurl: &unexpectedNURL, Burl: &unexpectedBURL}
	got, ok := FinalizeADVCallbacks(bid, "callbacks.example", "winner-2", "ssp.example", constants.IPP)
	if !ok || got == nil {
		t.Fatal("ADV native callback finalization failed")
	}
	linkURL := nativeLinkURLFromADM(t, got.GetAdm())
	assertCallbackQuery(t, linkURL, "/adm", map[string]string{
		"id": "winner-2", "url": "https://creative.example/render?a=1", "f": constants.FormatToCodes[constants.IPP],
	})
	assertCallbackQuery(t, got.GetBurl(), "/burl", map[string]string{
		"id": "winner-2", "f": constants.FormatToCodes[constants.IPP],
	})
	if got.GetNurl() != "" {
		t.Fatalf("native ADV response must not contain nurl: %q", got.GetNurl())
	}
	if bid.GetAdm() != adm || bid.GetNurl() != unexpectedNURL || bid.GetBurl() != unexpectedBURL {
		t.Fatal("source native bid was mutated")
	}
}

func nativeLinkURLFromADM(t *testing.T, adm string) string {
	t.Helper()
	var payload struct {
		Native struct {
			Link struct {
				URL string `json:"url"`
			} `json:"link"`
			Assets []json.RawMessage `json:"assets"`
		} `json:"native"`
	}
	if err := json.Unmarshal([]byte(adm), &payload); err != nil {
		t.Fatalf("parse native adm: %v", err)
	}
	if len(payload.Native.Assets) != 1 {
		t.Fatalf("native assets were not preserved: %s", adm)
	}
	return payload.Native.Link.URL
}

func assertCallbackQuery(t *testing.T, raw, path string, expected map[string]string) {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse callback %q: %v", raw, err)
	}
	if parsed.Path != path {
		t.Fatalf("callback path=%q want %q", parsed.Path, path)
	}
	for key, want := range expected {
		if got := parsed.Query().Get(key); got != want {
			t.Fatalf("callback %s=%q want %q in %q", key, got, want, raw)
		}
	}
}

func assertQueryMissing(t *testing.T, raw, key string) {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse callback %q: %v", raw, err)
	}
	if _, exists := parsed.Query()[key]; exists {
		t.Fatalf("callback %q unexpectedly contains query key %q", raw, key)
	}
}
