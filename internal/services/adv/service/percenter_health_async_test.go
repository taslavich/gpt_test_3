package auction

import (
	"context"
	"testing"
	"time"
)

func TestPercenterHealthReporterIsAsyncAndRecoveryWaitsForQuietPeriod(t *testing.T) {
	service := NewAuctionService(nil, nil, nil, nil, nil)
	failureStarted := make(chan string, 1)
	failureRelease := make(chan struct{})
	recoveries := make(chan string, 1)
	service.SetPercenterStateHealthReporter(
		func(_ context.Context, message string) {
			failureStarted <- message
			<-failureRelease
		},
		func(_ context.Context, message string) { recoveries <- message },
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.StartPercenterStateHealthReporter(ctx, 100*time.Millisecond)

	returned := make(chan struct{})
	go func() {
		service.reportPercenterStateFailure(context.Background(), "boom")
		close(returned)
	}()
	select {
	case <-returned:
		// Request-side signaling must not wait for notifier HTTP/callback work.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("failure signal blocked behind notifier callback")
	}
	select {
	case message := <-failureStarted:
		if message != "boom" {
			t.Fatalf("failure message=%q", message)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("async failure was not delivered")
	}
	close(failureRelease)

	service.reportPercenterStateRecovered(context.Background())
	select {
	case <-recoveries:
		t.Fatal("recovery fired before quiet period")
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case <-recoveries:
	case <-time.After(750 * time.Millisecond):
		t.Fatal("recovery was not delivered after quiet period")
	}
}
