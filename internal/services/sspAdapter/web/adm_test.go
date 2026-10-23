package sppAdapterWeb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ggicci/httpin"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

func TestAppendClickIDParameterUsesConfiguredNameAndReplacesExistingValue(t *testing.T) {
	got := appendClickIDParameter("https://example.test/path?subid=old&keep=1", "subid", "click-1")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if values := parsed.Query()["subid"]; len(values) != 1 || values[0] != "click-1" {
		t.Fatalf("subid values=%#v URL=%q", values, got)
	}
	if parsed.Query().Get("keep") != "1" {
		t.Fatalf("existing query parameter was lost: %q", got)
	}
}

func TestAppendClickIDParameterSkipsMissingOrInvalidName(t *testing.T) {
	const raw = "https://example.test/path?keep=1"
	for _, name := range []string{"", "bad&name", "white space"} {
		if got := appendClickIDParameter(raw, name, "click-1"); got != raw {
			t.Fatalf("name=%q got %q want unchanged URL", name, got)
		}
	}
}

func TestADMRedirectContinuesWhenWinnerRedisIsUnavailable(t *testing.T) {
	store, err := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	target := "https://advertiser.example/landing?keep=1"
	req := httptest.NewRequest(http.MethodGet, "/adm", nil)
	req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &admRequest{
		GlobalId: "winner-1",
		DspURL:   url.QueryEscape(target),
		Format:   constants.FormatToCodes[constants.IPP],
	}))
	response := httptest.NewRecorder()

	getAdm(
		context.Background(),
		response,
		req,
		nil,
		nil,
		"clicks",
		nil,
		"",
		nil,
		store,
		nil,
	)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, target) {
		t.Fatalf("redirect page does not contain target URL: %q", body)
	}
	for _, want := range []string{"window.self !== window.top", "navigator.sendBeacon", `window.open(advertiserUrl, "_blank")`, "window.top.location = advertiserUrl", "window.location.replace(advertiserUrl)"} {
		if !strings.Contains(body, want) {
			t.Fatalf("redirect page missing %q: %q", want, body)
		}
	}
	if strings.Contains(body, "setTimeout") {
		t.Fatalf("redirect page must not add an artificial timeout: %q", body)
	}
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("outbox records=%#v", records)
	}
	if records[0].Kind != outbox.KindADM || records[0].GlobalID != "winner-1" || records[0].WinnerType != outbox.WinnerUnknown {
		t.Fatalf("unexpected outbox record: %#v", records[0])
	}
}
func TestADMRedirectPageReportsIframeWithoutBlockingRedirect(t *testing.T) {
	response := httptest.NewRecorder()
	writeADMRedirectPage(response, "https://advertiser.example/landing?a=1&b=2", "11111111-1111-1111-1111-111111111111")

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{
		GetAdmIframeReportURL + "?click_id=11111111-1111-1111-1111-111111111111",
		"&is_iframe=",
		"keepalive: true",
		"window.location.replace(advertiserUrl)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %q", want, body)
		}
	}
}

func TestADMIframeReportRejectsMalformedInputBeforeRedis(t *testing.T) {
	for _, rawURL := range []string{
		GetAdmIframeReportURL + "?click_id=bad&is_iframe=1",
		GetAdmIframeReportURL + "?click_id=11111111-1111-1111-1111-111111111111&is_iframe=maybe",
	} {
		req := httptest.NewRequest(http.MethodPost, rawURL, nil)
		response := httptest.NewRecorder()
		postAdmIframeReport(response, req, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("url=%q status=%d body=%q", rawURL, response.Code, response.Body.String())
		}
	}
}
