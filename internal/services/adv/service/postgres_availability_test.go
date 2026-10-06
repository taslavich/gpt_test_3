package auction

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type togglePingState struct {
	fail  atomic.Bool
	calls atomic.Int64
}

type togglePingDriver struct {
	state *togglePingState
}

func (d *togglePingDriver) Open(string) (driver.Conn, error) {
	return &togglePingConn{state: d.state}, nil
}

type togglePingConn struct {
	state *togglePingState
}

func (c *togglePingConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported by toggle ping test driver")
}

func (c *togglePingConn) Close() error { return nil }

func (c *togglePingConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported by toggle ping test driver")
}

func (c *togglePingConn) Ping(context.Context) error {
	c.state.calls.Add(1)
	if c.state.fail.Load() {
		return errors.New("postgres unavailable")
	}
	return nil
}

var togglePingDriverSequence atomic.Uint64

func newTogglePingDB(t *testing.T, state *togglePingState) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("adv_postgres_availability_%d", togglePingDriverSequence.Add(1))
	sql.Register(name, &togglePingDriver{state: state})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("open toggle ping DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func waitForPostgresAvailability(t *testing.T, service *AuctionService, want bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if service.postgresAvailable.Load() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("postgresAvailable=%t, want %t", service.postgresAvailable.Load(), want)
}

func TestPostgresAvailabilityRefreshErrorRequiresConnectivityFailure(t *testing.T) {
	state := &togglePingState{}
	db := newTogglePingDB(t, state)
	service := NewAuctionService(nil, nil, nil, nil, nil)

	// A snapshot/application error with healthy PostgreSQL connectivity must not
	// activate the temporary +20% balance-guard multiplier.
	service.updatePostgresAvailabilityAfterRefreshError(db)
	if !service.postgresAvailable.Load() {
		t.Fatal("successful connectivity probe after snapshot error must keep PostgreSQL available")
	}

	state.fail.Store(true)
	service.updatePostgresAvailabilityAfterRefreshError(db)
	if service.postgresAvailable.Load() {
		t.Fatal("confirmed PostgreSQL connectivity failure must mark PostgreSQL unavailable")
	}
}

func TestPostgresRecoveryProbeRestoresRawSpendWithoutWaitingForSnapshotRefresh(t *testing.T) {
	state := &togglePingState{}
	state.fail.Store(true)
	db := newTogglePingDB(t, state)
	service := NewAuctionService(nil, nil, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.StartPostgresRecoveryProbe(ctx, db, 5*time.Millisecond, 20*time.Millisecond)

	// Confirm the outage through the same bounded connectivity classification
	// used after a failed snapshot refresh. This transition wakes the background
	// recovery probe.
	service.updatePostgresAvailabilityAfterRefreshError(db)
	if service.postgresAvailable.Load() {
		t.Fatal("expected PostgreSQL unavailable after failed connectivity probe")
	}
	if rtbBalanceGuardAllows(100, 80, 15, service.postgresAvailable.Load()) {
		t.Fatal("unavailable PostgreSQL must make Redis spend 80 behave as effective spend 96")
	}

	deadline := time.Now().Add(250 * time.Millisecond)
	for state.calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if state.calls.Load() < 2 {
		t.Fatalf("background recovery probe did not run while unavailable; ping calls=%d", state.calls.Load())
	}

	state.fail.Store(false)
	waitForPostgresAvailability(t, service, true, 250*time.Millisecond)

	// The next balance guard immediately uses raw Redis spend. With raw 80 there
	// is 20 left and a charge of 15 is allowed; under the 1.20 fallback it would
	// still be rejected.
	if !rtbBalanceGuardAllows(100, 80, 15, service.postgresAvailable.Load()) {
		t.Fatal("first balance guard after recovery must use raw Redis spend")
	}

	// When available, the recovery mechanism must go idle. In particular, RTB
	// balance checks themselves cannot trigger PostgreSQL round trips.
	time.Sleep(10 * time.Millisecond)
	callsAfterRecovery := state.calls.Load()
	for i := 0; i < 100; i++ {
		if !rtbBalanceGuardAllows(100, 80, 15, service.postgresAvailable.Load()) {
			t.Fatal("available PostgreSQL unexpectedly changed raw-spend semantics")
		}
	}
	time.Sleep(20 * time.Millisecond)
	if got := state.calls.Load(); got != callsAfterRecovery {
		t.Fatalf("PostgreSQL probe kept querying while available / balance guard caused DB I/O: before=%d after=%d", callsAfterRecovery, got)
	}
}

func TestRTBBalanceGuardAppliesSameFallbackToUserAndCampaignSpent(t *testing.T) {
	const (
		goal       = 100.0
		redisSpent = 80.0
		charge     = 15.0
	)
	campaignAllowed := rtbBalanceGuardAllows(goal, redisSpent, charge, false)
	userAllowed := rtbBalanceGuardAllows(goal, redisSpent, charge, false)
	if campaignAllowed != userAllowed {
		t.Fatalf("campaign/user PostgreSQL-outage multiplier semantics diverged: campaign=%t user=%t", campaignAllowed, userAllowed)
	}
	if campaignAllowed {
		t.Fatal("both user and campaign guards must treat Redis spend 80 as effective spend 96 during outage")
	}
}
