package sppAdapterWeb

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ggicci/httpin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/billing"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

func publisherFastRecord() publisherBURLInput {
	return publisherBURLInput{ID: uuid.NewString(), Record: publisherBURLRecord{Format: "POP", WinnerType: "dsp", PublisherID: "publisher"}}
}

func TestPublisherFastSuccessNeverSavesTask(t *testing.T) {
	box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	var completed atomic.Int32
	q := newPublisherBURLQueue(context.Background(), box, nil, nil, 2, 1000, time.Second, 24*time.Hour, func(context.Context, publisherBURLInput) error { completed.Add(1); return nil })
	for i := int32(1); i <= 100; i++ {
		q.enqueue(context.Background(), publisherFastRecord())
		waitPublisher(t, func() bool { return completed.Load() == i && len(q.slots) == 0 })
	}
	q.Close()
	if q.fallbackAttempts.Load() != 0 || q.lost.Load() != 0 {
		t.Fatal("successful jobs touched fallback", q.fallbackAttempts.Load(), q.lost.Load())
	}
	if n, e := box.TaskCount(); e != nil || n != 0 {
		t.Fatal(n, e)
	}
}

func TestPublisherInternalFailurePersistsAndRetries(t *testing.T) {
	box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	var recovered atomic.Bool
	var calls atomic.Int32
	q := newPublisherBURLQueue(context.Background(), box, nil, nil, 1, 1000, time.Second, 24*time.Hour, func(context.Context, publisherBURLInput) error {
		calls.Add(1)
		if !recovered.Load() {
			return errors.New("mandatory Redis operation failed")
		}
		return nil
	})
	defer q.Close()
	q.enqueue(context.Background(), publisherFastRecord())
	waitPublisher(t, func() bool { n, _ := box.TaskCount(); return n == 1 })
	if q.fallbackAttempts.Load() != 1 {
		t.Fatal("no durable fallback")
	}
	recovered.Store(true)
	waitPublisher(t, func() bool { n, _ := box.TaskCount(); return n == 0 })
	if calls.Load() < 2 || q.lost.Load() != 0 {
		t.Fatal(calls.Load(), q.lost.Load())
	}
}

func TestPublisherUnavailableBoltAndSaturatedWorkersDoNotBlockAdmission(t *testing.T) {
	box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if e != nil {
		t.Fatal(e)
	}
	box.Close() // Deterministic storage-write failure, including the ENOSPC path.
	before := runtime.NumGoroutine()
	q := newPublisherBURLQueue(context.Background(), box, nil, nil, 2, 1000, time.Second, 24*time.Hour, func(c context.Context, _ publisherBURLInput) error { <-c.Done(); return c.Err() })
	begin := time.Now()
	for i := 0; i < 2000; i++ {
		q.enqueue(context.Background(), publisherFastRecord())
	}
	if elapsed := time.Since(begin); elapsed > time.Second {
		t.Fatal("admission blocked by unavailable storage", elapsed)
	}
	if runtime.NumGoroutine()-before > 10 || len(q.slots) > 2 || len(q.fallback) > cap(q.fallback) {
		t.Fatal("unbounded overload")
	}
	q.Close()
	if q.lost.Load() == 0 {
		t.Fatal("lost fallback not signaled")
	}
}

func TestPublisherDSPOutcomeDoesNotRetryInternalSuccess(t *testing.T) {
	for _, mode := range []string{"500", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			guard, stats := callbackRedis(t, 10), callbackRedis(t, 12)
			r := publisherFastRecord()
			r.Redis = guard
			r.Record.BURL = "https://dsp.test/callback"
			if e := seedPublisherWinner(ctx, guard, r.ID, r.Record, time.Hour, false); e != nil {
				t.Fatal(e)
			}
			set := "v3-dsp:" + r.ID
			defer guard.Del(ctx, r.ID)
			defer func() {
				for _, id := range stats.SMembers(ctx, set).Val() {
					stats.Del(ctx, id)
				}
				stats.Del(ctx, set)
			}()
			var calls atomic.Int32
			client := &http.Client{Timeout: 20 * time.Millisecond, Transport: callbackTransport(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				if mode == "timeout" {
					<-req.Context().Done()
					return nil, req.Context().Err()
				}
				return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("DSP error")), Header: make(http.Header)}, nil
			})}
			box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
			if e != nil {
				t.Fatal(e)
			}
			defer box.Close()
			executor := newPublisherDSPExecutor(ctx, client, 2, 16)
			defer executor.Close()
			q := newPublisherBURLQueue(ctx, box, guard, nil, 1, 100, time.Second, 24*time.Hour, func(c context.Context, r publisherBURLInput) error {
				return processBurl(c, r, client, []*redis.Client{stats}, set, nil, "", nil, box, nil, executor)
			})
			q.enqueue(ctx, r)
			waitPublisher(t, func() bool { return guard.HGet(ctx, r.ID, "pub_burl_done").Val() == "1" && len(q.slots) == 0 })
			q.enqueue(ctx, r)
			waitPublisher(t, func() bool { return len(q.slots) == 0 })
			q.Close()
			waitPublisher(t, func() bool { return calls.Load() == 1 && executor.active.Load() == 0 })
			if q.fallbackAttempts.Load() != 0 || calls.Load() != 1 || stats.SCard(ctx, set).Val() != 1 {
				t.Fatal("external error caused repeat", q.fallbackAttempts.Load(), calls.Load())
			}
		})
	}
}

func TestPublisherRedisFailureNotMaskedByDSPAndRecoversFromBolt(t *testing.T) {
	ctx := context.Background()
	guard, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	r := publisherFastRecord()
	r.Redis = guard
	r.Record.BURL = "https://dsp.test/callback"
	if e := seedPublisherWinner(ctx, guard, r.ID, r.Record, time.Hour, false); e != nil {
		t.Fatal(e)
	}
	set := "v3-fail:" + r.ID
	defer guard.Del(ctx, r.ID)
	defer func() {
		for _, id := range stats.SMembers(ctx, set).Val() {
			stats.Del(ctx, id)
		}
		stats.Del(ctx, set)
	}()
	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 10 * time.Millisecond})
	defer broken.Close()
	var ready atomic.Bool
	var downstream atomic.Int32
	client := &http.Client{Transport: callbackTransport(func(*http.Request) (*http.Response, error) {
		downstream.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})}
	box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	executor := newPublisherDSPExecutor(ctx, client, 2, 16)
	defer executor.Close()
	q := newPublisherBURLQueue(ctx, box, guard, nil, 1, 100, time.Second, 24*time.Hour, func(c context.Context, r publisherBURLInput) error {
		destination := stats
		if !ready.Load() {
			destination = broken
		}
		return processBurl(c, r, client, []*redis.Client{destination}, set, nil, "", nil, box, nil, executor)
	})
	defer q.Close()
	q.enqueue(ctx, r)
	waitPublisher(t, func() bool { n, _ := box.TaskCount(); return n == 1 })
	if downstream.Load() != 0 || guard.HGet(ctx, r.ID, "pub_burl_done").Val() == "1" {
		t.Fatal("Redis failure masked")
	}
	// Retry must use the existing durable task, not read missing Redis context.
	guard.HDel(ctx, r.ID, "pub_dsp_nurl")
	ready.Store(true)
	waitPublisher(t, func() bool {
		n, _ := box.TaskCount()
		return n == 0 && guard.HGet(ctx, r.ID, "pub_burl_done").Val() == "1"
	})
	waitPublisher(t, func() bool { return downstream.Load() == 1 })
	if downstream.Load() != 1 || q.fallbackAttempts.Load() != 1 {
		t.Fatal("recovery failed", downstream.Load(), q.fallbackAttempts.Load())
	}
}

func TestPublisherFlushBeforeBURLAdmissionAndBrokenBolt(t *testing.T) {
	ctx := context.Background()
	dsp, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	r := publisherFastRecord()
	if e := seedPublisherWinner(ctx, dsp, r.ID, r.Record, time.Hour, false); e != nil {
		t.Fatal(e)
	}
	defer dsp.Del(ctx, r.ID)
	box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if e != nil {
		t.Fatal(e)
	}
	box.Close()
	q := newPublisherBURLQueue(ctx, box, dsp, nil, 1, 100, time.Second, 24*time.Hour, func(context.Context, publisherBURLInput) error { return errors.New("internal failure") })
	w := httptest.NewRecorder()
	set := "v3-adm:" + r.ID
	defer func() {
		for _, id := range stats.SMembers(ctx, set).Val() {
			stats.Del(ctx, id)
		}
		stats.Del(ctx, set)
	}()
	req := httptest.NewRequest("GET", "/adm?id="+r.ID, nil)
	req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &admRequest{GlobalId: r.ID, DspURL: "https://advertiser.test/original", Format: "0", Publisher: "1"}))
	req = req.WithContext(context.WithValue(req.Context(), publisherBurlKey{}, publisherBurlFunc(func(c context.Context, r publisherBURLInput) {
		if !w.Flushed || !strings.Contains(w.Body.String(), "advertiser.test/original") {
			t.Error("BURL admitted before complete redirect flush")
		}
		q.enqueue(c, r)
	})))
	getAdm(ctx, w, req, []*redis.Client{stats}, dsp, set, nil, "", nil, nil, nil)
	q.Close()
	if w.Code != 200 || q.lost.Load() != 1 {
		t.Fatal(w.Code, q.lost.Load())
	}
}

func TestPublisherADVIntentFailureCannotBecomeSuccess(t *testing.T) {
	ctx := context.Background()
	winner, spend, stats := callbackRedis(t, 10), callbackRedis(t, 9), callbackRedis(t, 12)
	r := publisherFastRecord()
	r.Record.WinnerType = "adv"
	r.Redis = winner
	r.Winner = billing.Winner{Format: "POP", Price: 0.25, UserID: uuid.NewString(), CampaignID: uuid.NewString()}
	winner.HSet(ctx, r.ID, "guard", "ready")
	winner.Expire(ctx, r.ID, time.Hour)
	defer winner.Del(ctx, r.ID)
	box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if e != nil {
		t.Fatal(e)
	}
	box.Close()
	store := billing.NewStore(spend, winner, time.Hour)
	e = processBurl(ctx, r, &http.Client{Timeout: 3 * time.Second}, []*redis.Client{stats}, "v3-adv:"+r.ID, nil, "", store, box, nil)
	if e == nil || winner.HGet(ctx, r.ID, "pub_burl_done").Val() == "1" || spend.Exists(ctx, "spent:user:"+r.Winner.UserID).Val() != 0 {
		t.Fatal("failed ADV write-ahead intent bypassed", e)
	}
}

func TestPublisherFullFallbackStagingIsBoundedAndNonblocking(t *testing.T) {
	// Frozen persister + occupied worker: deterministic worst-case disk stall.
	// Exercise the actual admission code, without relying on filesystem timing.
	q := &publisherBURLQueue{slots: make(chan struct{}, 1), fallback: make(chan publisherBURLInput, 1), failure: make(chan error, 1)}
	q.slots <- struct{}{}
	before := runtime.NumGoroutine()
	begin := time.Now()
	for i := 0; i < 10000; i++ {
		q.enqueue(context.Background(), publisherFastRecord())
	}
	if time.Since(begin) > time.Second || len(q.fallback) != 1 || q.lost.Load() != 9999 || runtime.NumGoroutine() > before {
		t.Fatal("overload admission blocked, grew or hid loss", q.lost.Load())
	}
	select {
	case <-q.failure:
	default:
		t.Fatal("overload not signaled")
	}
}
