package pub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
)

func TestPubAlertUsesSystemBotAndRetainsCountsOnFailure(t *testing.T) {
	messages := make(chan string, 2)
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct{ Text string }
		if r.Method != http.MethodPost || r.URL.Path != "/internal/messages/send" ||
			r.Header.Get("X-Bot-Secret") != "test-secret" || json.NewDecoder(r.Body).Decode(&payload) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		messages <- payload.Text
		if len(messages) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer bot.Close()
	a := NewSystemAlerts("test-pub", utils.NewBotMessage(bot.URL, "test-secret").SendTextMessageToBot)
	a.recordRTBFailure(classifyRTBHTTPFailure(403, "", "Ip is limited: 192.0.2.42"))
	if !a.flush(context.Background()) {
		t.Fatal("missing first send")
	}
	a.recordRTBFailure(classifyRTBHTTPFailure(429, "", "rate limit"))
	a.Record(AlertDelivery)
	if !a.flush(context.Background()) || a.flush(context.Background()) || len(messages) != 2 {
		t.Fatal("failed count was not retried exactly once")
	}
	if first := <-messages; first != "[pub] host=test-pub\nRTB errors: 1\n- 1 x [ssp_ip_limited] SSP Adapter rejected the visitor IP under the request-rate policy (HTTP 403)" {
		t.Fatalf("first notice: %q", first)
	}
	if second := <-messages; second != "[pub] host=test-pub\nRTB errors: 2\n- 1 x [ssp_ip_limited] SSP Adapter rejected the visitor IP under the request-rate policy (HTTP 403)\n- 1 x [upstream_unclassified] RTB or its proxy throttled the request (HTTP 429)\ndelivery=1: Publisher could not prepare or validate the ad delivery" {
		t.Fatalf("retry notice: %q", second)
	}
}

func TestPubAlertWorkerDoesNotWaitInRecordAndLimitsSends(t *testing.T) {
	sends := make(chan time.Time, 2)
	unblock := make(chan struct{})
	a := NewSystemAlerts("test-pub", func(ctx context.Context, _ string) error {
		sends <- time.Now()
		select {
		case <-unblock:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	done := make(chan struct{})
	go func() { defer close(done); a.run(ctx, 40*time.Millisecond) }()
	defer func() { cancel(); <-done }()
	next := func() time.Time {
		t.Helper()
		select {
		case at := <-sends:
			return at
		case <-ctx.Done():
			t.Fatal("missing digest")
			return time.Time{}
		}
	}
	a.recordRTBFailure(classifyRTBHTTPFailure(403, "", "Ip is limited: 192.0.2.42"))
	first := next()
	recorded := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			a.recordRTBFailure(classifyRTBHTTPFailure(403, "", "Ip is limited: 192.0.2.42"))
		}
		close(recorded)
	}()
	select {
	case <-recorded:
	case <-ctx.Done():
		t.Fatal("recording blocked on the bot")
	}
	close(unblock)
	if second := next(); second.Sub(first) < 40*time.Millisecond {
		t.Fatal("digest was sent during cooldown")
	}
}

func TestPubAlertForUnavailableConfigNotClientRefusals(t *testing.T) {
	var message string
	a := NewSystemAlerts("test-pub", func(_ context.Context, text string) error {
		message = text
		return nil
	})
	_, proxy, _ := net.ParseCIDR("192.0.2.1/32")
	s := &Server{Alerts: a, proxies: []*net.IPNet{proxy}, InfraSettings: infraSettings{Paths: defaultInfraPaths()}}
	for _, tc := range []struct {
		path, remote string
		want         int
	}{
		{"/config", "192.0.2.1:1234", 503},
		{"/delivery/invalid", "192.0.2.1:1234", 410},
		{"/config", "192.0.2.2:1234", 403},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, tc.path, nil)
		r.RemoteAddr = tc.remote
		s.Handler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s %s: status %d, want %d", tc.path, tc.remote, w.Code, tc.want)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	s.reportRedisRead(canceled, errors.New("browser left"))
	if !a.flush(context.Background()) || !strings.HasSuffix(message, "\nconfig_unready=1: Publisher configuration in Redis is unavailable or Redis safety has not been verified") ||
		strings.Count(message, "\n") != 1 {
		t.Fatalf("unexpected digest: %q", message)
	}
}

func TestPubRTBAlertBoundsDigest(t *testing.T) {
	var text string
	a := NewSystemAlerts("test-pub", func(_ context.Context, message string) error {
		text = message
		return nil
	})
	for i := 0; i < maxRTBAlertGroups+1; i++ {
		a.recordRTBFailure(rtbFailure{reason: fmt.Sprintf("test_%d", i), explanation: "diagnostic"})
	}
	a.recordRTBFailure(rtbFailure{reason: "test_0", explanation: "diagnostic"})
	a.recordRTBFailure(errors.New("secret upstream payload"))
	if !a.flush(context.Background()) || !strings.Contains(text, fmt.Sprintf("RTB errors: %d", maxRTBAlertGroups+3)) ||
		!strings.Contains(text, "- 2 x [test_0]") || !strings.Contains(text, "Other RTB errors: 2") ||
		strings.Contains(text, "secret upstream payload") || !utf8.ValidString(text) {
		t.Fatalf("incorrect digest: %q", text)
	}
}
