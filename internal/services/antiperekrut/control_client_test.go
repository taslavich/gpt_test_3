package antiperekrut

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFanoutStartupEventCallsEveryADV(t *testing.T) {
	const secret = "test-secret"
	var calls1 atomic.Int32
	var calls2 atomic.Int32
	newServer := func(calls *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/internal/antiperekrut/restart" {
				http.NotFound(w, r)
				return
			}
			if r.Header.Get("X-Antiperekrut-Secret") != secret {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			var event StartupEvent
			if err := json.NewDecoder(r.Body).Decode(&event); err != nil || event.Reason != "startup" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			calls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]int64{"generation": 7})
		}))
	}
	s1 := newServer(&calls1)
	defer s1.Close()
	s2 := newServer(&calls2)
	defer s2.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := FanoutStartupEvent(ctx, ClientConfig{
		Enabled: true, URLs: []string{s1.URL, s2.URL}, Secret: secret,
		RequestTimeout: time.Second, RetryInitial: 10 * time.Millisecond, RetryMax: 20 * time.Millisecond,
	}, NewStartupEvent("router", "router-1"), nil)
	if err != nil {
		t.Fatalf("fanout failed: %v", err)
	}
	if calls1.Load() != 1 || calls2.Load() != 1 {
		t.Fatalf("fanout calls: first=%d second=%d; every ADV must receive the event", calls1.Load(), calls2.Load())
	}
}

func TestNormalizeURLsRejectsBalancerLikeInvalidValue(t *testing.T) {
	if _, err := normalizeURLs([]string{"not-a-url"}); err == nil {
		t.Fatal("invalid URL must be rejected")
	}
	urls, err := normalizeURLs([]string{"http://adv-1:8101/", "http://adv-1:8101", "http://adv-2:8101"})
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 2 {
		t.Fatalf("normalized URLs=%v, want two unique direct URLs", urls)
	}
}
