package main

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
)

func validADVConfigForStartupTest() *config.AdvConfig {
	return &config.AdvConfig{
		RedisConfig: config.RedisConfig{
			RedisUUIDAddr:       "127.0.0.1:6379",
			RedisADVAddr:        "127.0.0.1:6379",
			RedisDBAdvRuntime:   5,
			RedisDBAdvWinner:    6,
			RedisDBAdvPercenter: 7,
		},
		AdvPercentMapFilePath:       "percent.json",
		AdvQualityMapFilePath:       "quality.json",
		AdvSiteIDQualityMapFilePath: "site-quality.json",
		AdvVPNDBPath:                "vpn.mmdb",
		PostgresDSN:                 "postgres://example",
		CampaignRefreshInterval:     time.Second,
		AdvWinnerTTL:                time.Minute,
		AdvPacingTickInterval:       time.Second,
		AdvPacingCurrentTTL:         time.Minute,
		AdvPacingSlotTTL:            time.Hour,
		AntiperekrutTickOffset:      8 * time.Second,
	}
}

func TestADVTelegramOptionalWhenAntiperekrutDisabled(t *testing.T) {
	cfg := validADVConfigForStartupTest()
	cfg.AntiperekrutEnabled = false
	cfg.BotBaseURL = ""
	cfg.BotInternalSecret = ""
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("Telegram must be optional for auction/percenter warnings when antiperekrut is disabled: %v", err)
	}
}

func TestADVStillRequiresTelegramForExistingAntiperekrutControlPath(t *testing.T) {
	cfg := validADVConfigForStartupTest()
	cfg.AntiperekrutEnabled = true
	cfg.AdvServiceControlURLs = config.ListString{"http://127.0.0.1:8101"}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("existing antiperekrut startup-control dependency must retain its credentials requirement")
	}
}

func TestWaitForADVTerminationServerErrorCancelsAndWaitsForTelemetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	errChan := make(chan error, 1)
	serverErr := errors.New("grpc serve failed")

	var telemetryWG sync.WaitGroup
	telemetryDone := make(chan struct{})
	telemetryWG.Add(1)
	go func() {
		defer telemetryWG.Done()
		<-ctx.Done()
		close(telemetryDone)
	}()

	gracefulStopCalled := false
	errChan <- serverErr
	err := waitForADVTermination(
		stop,
		errChan,
		cancel,
		func() { gracefulStopCalled = true },
		func() {},
		&telemetryWG,
		time.Second,
		time.Second,
	)

	if !errors.Is(err, serverErr) {
		t.Fatalf("expected original gRPC server error, got %v", err)
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("runtime server error must cancel ADV context, got %v", ctx.Err())
	}
	select {
	case <-telemetryDone:
		// waitForADVTermination must not return until the telemetry worker has
		// observed cancellation and completed its shutdown path.
	default:
		t.Fatal("telemetry worker was still running when ADV lifecycle helper returned")
	}
	if gracefulStopCalled {
		t.Fatal("GracefulStop must not be called after Serve has already terminated")
	}
}

func TestWaitForADVTerminationSignalForcesStopAfterGraceTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	errChan := make(chan error, 1)

	var telemetryWG sync.WaitGroup
	telemetryDone := make(chan struct{})
	telemetryWG.Add(1)
	go func() {
		defer telemetryWG.Done()
		<-ctx.Done()
		close(telemetryDone)
	}()

	graceRelease := make(chan struct{})
	graceStarted := make(chan struct{})
	forceStopCalled := make(chan struct{}, 1)
	gracefulStop := func() {
		close(graceStarted)
		<-graceRelease
	}
	forceStop := func() {
		select {
		case forceStopCalled <- struct{}{}:
		default:
		}
		close(graceRelease)
	}

	stop <- os.Interrupt
	started := time.Now()
	err := waitForADVTermination(
		stop,
		errChan,
		cancel,
		gracefulStop,
		forceStop,
		&telemetryWG,
		20*time.Millisecond,
		time.Second,
	)
	elapsed := time.Since(started)

	if err != nil {
		t.Fatalf("signal shutdown returned error: %v", err)
	}
	select {
	case <-graceStarted:
	default:
		t.Fatal("GracefulStop was not started")
	}
	select {
	case <-forceStopCalled:
	default:
		t.Fatal("force Stop was not called after graceful timeout")
	}
	select {
	case <-telemetryDone:
	default:
		t.Fatal("background telemetry did not finish before lifecycle helper returned")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("bounded shutdown took too long: %s", elapsed)
	}
}

func TestWaitForADVTerminationSignalDoesNotForceStopWhenGracefulCompletes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	errChan := make(chan error, 1)

	var telemetryWG sync.WaitGroup
	telemetryWG.Add(1)
	go func() {
		defer telemetryWG.Done()
		<-ctx.Done()
	}()

	gracefulStopCalled := make(chan struct{}, 1)
	forceStopCalled := false
	stop <- os.Interrupt
	err := waitForADVTermination(
		stop,
		errChan,
		cancel,
		func() { gracefulStopCalled <- struct{}{} },
		func() { forceStopCalled = true },
		&telemetryWG,
		time.Second,
		time.Second,
	)

	if err != nil {
		t.Fatalf("signal shutdown returned error: %v", err)
	}
	select {
	case <-gracefulStopCalled:
	default:
		t.Fatal("GracefulStop was not called")
	}
	if forceStopCalled {
		t.Fatal("force Stop must not be called when GracefulStop completes within timeout")
	}
}
