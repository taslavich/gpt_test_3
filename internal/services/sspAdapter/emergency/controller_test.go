package emergency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStopAndNotifyCallsAllADVURLs(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPut || r.URL.Query().Get("work") != "false" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := NewController([]string{srv.URL + "/one", srv.URL + "/two"}, time.Second, nil)
	if err := c.StopAndNotify(context.Background(), "billing failed"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want 2", calls)
	}
}

func TestStopAndNotifyReturnsFirstADVErrorAfterCallingAll(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := NewController([]string{srv.URL + "/one", srv.URL + "/two"}, time.Second, nil)
	if err := c.StopAndNotify(context.Background(), "billing failed"); err == nil {
		t.Fatal("expected first ADV error")
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want 2", calls)
	}
}
