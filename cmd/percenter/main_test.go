package main

import (
	"context"
	"net"
	"sync"
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
