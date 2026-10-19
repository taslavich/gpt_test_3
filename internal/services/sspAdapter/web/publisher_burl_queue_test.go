package sppAdapterWeb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ggicci/httpin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/callback"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/billing"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

func waitPublisher(t *testing.T, fn func() bool) {
	t.Helper()
	end := time.Now().Add(5 * time.Second)
	for time.Now().Before(end) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("publisher queue condition timed out")
}
func TestPublisherBURLBoundedTasksRestartAndOverload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.db")
	box, e := outbox.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	var active, peak, calls atomic.Int32
	before := runtime.NumGoroutine()
	process := func(ctx context.Context, r resolvedCallback) error {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		calls.Add(1)
		<-ctx.Done()
		return ctx.Err()
	}
	q := newPublisherBURLQueue(context.Background(), box, nil, nil, 4, 100000, 100*time.Millisecond, 24*time.Hour, process)
	for i := 0; i < 400; i++ {
		q.enqueue(context.Background(), resolvedCallback{ID: uuid.NewString(), Record: callback.Record{Format: "POP", WinnerType: "dsp", PublisherID: "publisher"}})
	}
	waitPublisher(t, func() bool { return calls.Load() >= 4 })
	if peak.Load() > 4 || runtime.NumGoroutine()-before > 12 {
		t.Fatal("unbounded execution", peak.Load(), runtime.NumGoroutine()-before)
	}
	q.Close()
	if n, _ := box.TaskCount(); n != 400 {
		t.Fatal("timeout/shutdown lost tasks", n)
	}
	if e = box.Close(); e != nil {
		t.Fatal(e)
	}
	box, e = outbox.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	// Leases of canceled tasks are durable. Advance them exactly as elapsed wall
	// time would do after a hard restart, without waiting several minutes in CI.
	tasks, e := box.ClaimTasks(time.Now().Add(5*time.Minute), 400, time.Minute)
	if e != nil || len(tasks) != 400 {
		t.Fatal(len(tasks), e)
	}
	for _, task := range tasks {
		if e = box.FinishTask(task, time.Now().Add(-time.Second)); e != nil {
			t.Fatal(e)
		}
	}
	var completed atomic.Int32
	restarted := newPublisherBURLQueue(context.Background(), box, nil, nil, 4, 100000, time.Second, 24*time.Hour, func(context.Context, resolvedCallback) error { completed.Add(1); return nil })
	defer restarted.Close()
	waitPublisher(t, func() bool { n, _ := box.TaskCount(); return n == 0 })
	if completed.Load() != 400 {
		t.Fatal("restart lost or duplicated tasks", completed.Load())
	}
	t.Logf("400 durable jobs, max active=%d, restart recovered=%d", peak.Load(), completed.Load())
}
func TestPublisherManyADMAndSharedDSPGuardAcrossDBs(t *testing.T) {
	ctx := context.Background()
	adm, burl, stats := callbackRedis(t, 10), callbackRedis(t, 11), callbackRedis(t, 12)
	box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	id := uuid.NewString()
	r := callback.Record{Format: "POP", WinnerType: "dsp", PublisherID: "publisher", Destination: "https://advertiser.test/original", BURL: "https://dsp.test/slow"}
	if e = callback.Save(ctx, adm, id, r, time.Hour, false); e != nil {
		t.Fatal(e)
	}
	if e = callback.Save(ctx, burl, id, r, time.Hour, false); e != nil {
		t.Fatal(e)
	}
	clicks, imps := "many-adm:"+id, "many-imp:"+id
	defer func() {
		adm.Del(ctx, id)
		burl.Del(ctx, id)
		for _, set := range []string{clicks, imps} {
			for _, k := range stats.SMembers(ctx, set).Val() {
				stats.Del(ctx, k)
			}
			stats.Del(ctx, set)
		}
	}()
	var downstream atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	client := &http.Client{Timeout: time.Second, Transport: callbackTransport(func(req *http.Request) (*http.Response, error) {
		downstream.Add(1)
		once.Do(func() { close(started) })
		select {
		case <-release:
		case <-req.Context().Done():
		}
		return nil, errors.New("DSP timeout")
	})}
	executor := newPublisherDSPExecutor(ctx, client, 2, 16)
	defer executor.Close()
	q := newPublisherBURLQueue(ctx, box, adm, nil, 4, 100000, time.Second, 24*time.Hour, func(c context.Context, res resolvedCallback) error {
		return processBurl(c, res, client, []*redis.Client{stats}, imps, nil, "", nil, box, nil, executor)
	})
	defer q.Close()
	begin := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/adm?id="+id, nil)
			req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &admRequest{GlobalId: id}))
			req = req.WithContext(context.WithValue(req.Context(), publisherBurlKey{}, publisherBurlFunc(q.enqueue)))
			w := httptest.NewRecorder()
			getAdm(ctx, w, req, []*redis.Client{stats}, adm, clicks, nil, "", nil, box, nil)
			if w.Code != 200 || !strings.Contains(w.Body.String(), "advertiser.test/original") {
				t.Error(w.Code)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(begin)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("BURL not started")
	}
	if elapsed >= time.Second {
		t.Fatal("ADM waited for downstream", elapsed)
	}
	res, e := resolveCallback(ctx, id, burl, nil)
	if e != nil {
		t.Fatal(e)
	}
	guardCtx := context.WithValue(ctx, publisherDSPGuardKey{}, adm)
	waitPublisher(t, func() bool { return adm.HGet(ctx, id, "pub_burl_done").Val() == "1" })
	if e = processBurl(guardCtx, res, client, []*redis.Client{stats}, imps, nil, "", nil, box, nil, executor); e != nil {
		t.Fatal("completed guard waited for DSP", e)
	}
	close(release)
	waitPublisher(t, func() bool {
		n, _ := box.TaskCount()
		return n == 0 && len(q.slots) == 0 && adm.HGet(ctx, id, "pub_burl_done").Val() == "1"
	})
	if e = processBurl(guardCtx, res, client, []*redis.Client{stats}, imps, nil, "", nil, box, nil, executor); e != nil {
		t.Fatal(e)
	}
	if downstream.Load() != 1 || stats.SCard(ctx, imps).Val() != 1 {
		t.Fatal("cross-DB duplicate", downstream.Load(), stats.SCard(ctx, imps).Val())
	}
	if stats.SCard(ctx, clicks).Val() != 100 {
		t.Fatal("ADM click behavior changed")
	}
	for _, id := range stats.SMembers(ctx, clicks).Val() {
		u, _ := uuid.Parse(id)
		if u.Version() != 7 {
			t.Fatal("non-immutable time bucket UUID", id)
		}
	}
	t.Logf("100 concurrent ADM responses in %s; one BURL, no wait for slow DSP", elapsed)
}
func TestPublisherQueuedADVRestartReusesWriteAheadBilling(t *testing.T) {
	ctx := context.Background()
	adv, spend, stats := callbackRedis(t, 11), callbackRedis(t, 9), callbackRedis(t, 12)
	id, user, campaign := uuid.NewString(), uuid.NewString(), uuid.NewString()
	set := "restart-adv:" + id
	adv.HSet(ctx, id, "format", "POP", "price", "0.25", "user_id", user, "campaign_id", campaign)
	adv.Expire(ctx, id, time.Hour)
	r := callback.Record{Format: "POP", WinnerType: "adv", PublisherID: "publisher", Destination: "https://advertiser.test/"}
	if e := callback.Save(ctx, adv, id, r, time.Hour, true); e != nil {
		t.Fatal(e)
	}
	store := billing.NewStore(spend, adv, time.Hour)
	res, e := resolveCallback(ctx, id, nil, store)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "outbox.db")
	box, e := outbox.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	payload, _ := json.Marshal(publisherBURLPayload{ID: id, Record: r, Winner: res.Winner})
	if e = box.SaveTask(id, payload); e != nil {
		t.Fatal(e)
	}
	// Side effects completed but ACK was lost on adapter failure.
	if e = processBurl(ctx, res, callback.HTTPClient(), []*redis.Client{stats}, set, nil, "", store, box, nil); e != nil {
		t.Fatal(e)
	}
	box.Close()
	box, e = outbox.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	q := newPublisherBURLQueue(ctx, box, nil, store, 2, 100000, time.Second, 24*time.Hour, func(c context.Context, res resolvedCallback) error {
		return processBurl(c, res, callback.HTTPClient(), []*redis.Client{stats}, set, nil, "", store, box, nil)
	})
	defer q.Close()
	waitPublisher(t, func() bool { n, _ := box.TaskCount(); return n == 0 })
	if v := spend.Get(ctx, "spent:user:"+user).Val(); v != "0.25" {
		t.Fatal("double spend", v)
	}
	if stats.SCard(ctx, set).Val() != 1 {
		t.Fatal("double impression")
	}
	adv.Del(ctx, id)
	spend.Del(ctx, "spent:user:"+user, "spent:campaign:"+campaign, "outbox:applied:"+billing.CallbackEventID("burl", "POP", id))
	for _, k := range stats.SMembers(ctx, set).Val() {
		stats.Del(ctx, k)
	}
	stats.Del(ctx, set)
}

func TestPublisherBURLBacklogRaisesExistingFailureSignal(t *testing.T) {
	box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	for i := 0; i < 3; i++ {
		id := uuid.NewString()
		payload, _ := json.Marshal(publisherBURLPayload{ID: id, Record: callback.Record{Format: "POP", PublisherID: "publisher", WinnerType: "dsp"}})
		if e = box.SaveTask(id, payload); e != nil {
			t.Fatal(e)
		}
	}
	signaled := make(chan error, 1)
	q := newPublisherBURLQueue(context.Background(), box, nil, nil, 1, 2, time.Second, 24*time.Hour, func(ctx context.Context, _ resolvedCallback) error { <-ctx.Done(); return ctx.Err() }, func(err error) {
		select {
		case signaled <- err:
		default:
		}
	})
	defer q.Close()
	select {
	case err := <-signaled:
		if !strings.Contains(err.Error(), "high water") {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("backlog overflow not reported")
	}
	if n, _ := box.TaskCount(); n != 3 {
		t.Fatal("backpressure discarded durable tasks", n)
	}
}

func TestPublisherBURLExpiredRetryHorizonHoldsIntent(t *testing.T) {
	box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	id := uuid.NewString()
	payload, _ := json.Marshal(publisherBURLPayload{ID: id, Record: callback.Record{Format: "POP", PublisherID: "publisher", WinnerType: "adv"}})
	if e = box.SaveTask(id, payload); e != nil {
		t.Fatal(e)
	}
	signaled := make(chan error, 1)
	q := newPublisherBURLQueue(context.Background(), box, nil, nil, 1, 100, time.Second, time.Nanosecond, func(context.Context, resolvedCallback) error {
		t.Error("old task retried outside billing dedup retention")
		return nil
	}, func(e error) {
		select {
		case signaled <- e:
		default:
		}
	})
	defer q.Close()
	select {
	case e := <-signaled:
		if !strings.Contains(e.Error(), "horizon") {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("expired task not reported")
	}
	waitPublisher(t, func() bool { n, _ := box.TaskCount(); return n == 1 })
}
