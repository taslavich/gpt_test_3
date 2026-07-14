package bidEngine

import (
	"net/url"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestFinalizeBidCallbacksForADVBanner(t *testing.T) {
	adm := "https://creative.example/render?a=1"
	nurl := "https://dsp.example/win"
	bid := &ortb.Bid{Adm: &adm, Nurl: &nurl}

	got, ok := FinalizeBidCallbacks(bid, "callbacks.example", "winner-1", "ssp.example", constants.BAN, true, ADVUsesBURL(constants.BAN))
	if !ok || got == nil {
		t.Fatal("callback finalization failed")
	}
	assertCallbackQuery(t, got.GetAdm(), "/adm", map[string]string{"id": "winner-1", "url": adm, "f": constants.FormatToCodes[constants.BAN]})
	assertCallbackQuery(t, got.GetNurl(), "/nurl", map[string]string{"id": "winner-1", "url": nurl, "s": "ssp.example", "f": constants.FormatToCodes[constants.BAN]})
	assertCallbackQuery(t, got.GetBurl(), "/burl", map[string]string{"id": "winner-1", "f": constants.FormatToCodes[constants.BAN]})
	if bid.GetBurl() != "" || bid.GetAdm() != adm || bid.GetNurl() != nurl {
		t.Fatal("source bid was mutated")
	}
}

func TestFinalizeBidCallbacksForADVIPPDoesNotAddBURL(t *testing.T) {
	adm := "https://creative.example/render"
	bid := &ortb.Bid{Adm: &adm}
	got, ok := FinalizeBidCallbacks(bid, "callbacks.example", "winner-2", "ssp.example", constants.IPP, true, ADVUsesBURL(constants.IPP))
	if !ok || got == nil {
		t.Fatal("callback finalization failed")
	}
	assertCallbackQuery(t, got.GetAdm(), "/adm", map[string]string{"id": "winner-2", "url": adm, "f": constants.FormatToCodes[constants.IPP]})
	if got.GetBurl() != "" {
		t.Fatalf("IPP ADV bid must not contain BURL, got %q", got.GetBurl())
	}
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
