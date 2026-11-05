package pub

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPublisherKeepsSendingAuctionsAfterRTBFailures(t *testing.T) {
	for _, tc := range []struct {
		name        string
		upstreamErr error
	}{
		{name: "HTTP 503"},
		{name: "transport timeout", upstreamErr: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, code := fixture(t)
			var calls int
			const failures = 7
			s.HTTP.Transport = transport(func(_ *http.Request) (*http.Response, error) {
				calls++
				if calls > failures {
					return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
				}
				if tc.upstreamErr != nil {
					return nil, tc.upstreamErr
				}
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Body:       io.NopCloser(strings.NewReader("upstream unavailable")),
				}, nil
			})

			for i := 1; i <= failures; i++ {
				w := send(s, input(code))
				if calls != i || w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "rtb_failed") {
					t.Fatalf("request %d: reached RTB %d times, HTTP %d %s", i, calls, w.Code, w.Body.String())
				}
			}
			w := send(s, input(code))
			if calls != failures+1 || w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "no_fill") {
				t.Fatalf("first request after RTB recovered: reached RTB %d times, HTTP %d %s", calls, w.Code, w.Body.String())
			}
		})
	}
}
