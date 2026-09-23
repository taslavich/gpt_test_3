package main

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbeKafkaBrokersAcceptsReachableConfiguredBroker(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := probeKafkaBrokers(ctx, []string{"127.0.0.1:1", listener.Addr().String()}); err != nil {
		t.Fatalf("one reachable configured broker should satisfy startup connectivity probe: %v", err)
	}
}

func TestProbeKafkaBrokersReportsUnavailableWithoutPanicking(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := probeKafkaBrokers(ctx, []string{"127.0.0.1:1"}); err == nil {
		t.Fatal("unavailable Kafka broker must be reported as degraded")
	}
}

func TestWaitForBackgroundCompletesAfterWorkerExit(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
	}()
	if !waitForBackground(&wg, time.Second) {
		t.Fatal("background worker should finish within shutdown budget")
	}
}

func TestRunBoundedOptimizerTickCancelsBlockingQueryOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan struct{})
	var complexStarted atomic.Bool

	go func() {
		defer close(done)
		runBoundedOptimizerTick(ctx, time.Minute, time.Now(), func(tickCtx context.Context, _ time.Time) {
			close(started)
			<-tickCtx.Done() // fake blocking ClickHouse query that honors cancellation
		}, func(context.Context, time.Time) {
			complexStarted.Store(true)
		})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking tick did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("shutdown did not cancel blocking optimizer tick within bound")
	}
	if complexStarted.Load() {
		t.Fatal("complex iteration must not start after shutdown cancellation")
	}
}
