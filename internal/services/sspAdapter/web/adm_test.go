package sppAdapterWeb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ggicci/httpin"
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

func TestADMRejectsUntrustedURLWhenContextUnavailable(t *testing.T) {
	req := httptest.NewRequest("GET", "/adm?id=10000000-0000-4000-8000-000000000001&url=https://attacker.test&f=4", nil)
	req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &admRequest{GlobalId: "10000000-0000-4000-8000-000000000001"}))
	w := httptest.NewRecorder()
	getAdm(context.Background(), w, req, nil, nil, "clicks", nil, "", nil, nil, nil)
	if w.Code != 503 || strings.Contains(w.Body.String(), "attacker.test") {
		t.Fatalf("unsafe Redis fallback: %d %s", w.Code, w.Body.String())
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
