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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ggicci/httpin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

func TestPublisherV4DSPExecutorBoundsAndShutdown(t *testing.T) {
	before := runtime.NumGoroutine()
	client := &http.Client{Transport: callbackTransport(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	e := newPublisherDSPExecutor(context.Background(), client, 2, 3)
	defer e.Close()
	e.enqueue("first", "https://dsp.test/burl")
	e.enqueue("second", "https://dsp.test/burl")
	waitPublisher(t, func() bool { return e.active.Load() == 2 })
	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	begin := time.Now()
	for i := 0; i < 10000; i++ {
		e.enqueue("overflow", "https://dsp.test/burl")
	}
	runtime.ReadMemStats(&memAfter)
	if len(e.tasks) != 3 || e.dropped.Load() != 9997 || e.active.Load() != 2 || runtime.NumGoroutine()-before > 5 {
		t.Fatal("unbounded executor", len(e.tasks), e.active.Load(), e.dropped.Load(), runtime.NumGoroutine()-before)
	}
	t.Logf("10000 admissions: elapsed=%s heap_allocation_delta=%d active=%d queue=%d drops=%d goroutine_delta=%d", time.Since(begin), memAfter.TotalAlloc-memBefore.TotalAlloc, e.active.Load(), len(e.tasks), e.dropped.Load(), runtime.NumGoroutine()-before)
	begin = time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e.Close() }()
	}
	for i := 0; i < 1000; i++ {
		e.enqueue("shutdown", "https://dsp.test/burl")
	}
	wg.Wait()
	if time.Since(begin) > time.Second || e.active.Load() != 0 {
		t.Fatal("shutdown blocked", time.Since(begin))
	}
}

// Use actual Redis effects and the /adm handler with the V4 lifecycle.
// All 1000 unique winners arrive concurrently; any Bolt overflow remains V3's
// finite internal admission policy, while the DSP queue cannot delay Redis work.
func TestPublisherV4ThousandConcurrentADM(t *testing.T) {
	ctx := context.Background()
	guard, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	root := uuid.NewString()
	clicks, imps := "v4-clicks:"+root, "v4-imps:"+root
	records := make([]publisherBURLInput, 1000)
	for i := range records {
		r := publisherFastRecord()
		r.Redis = guard
		r.Record.BURL = "https://dsp.test/delegated-nurl?original=%2F"

		if err := seedPublisherWinner(ctx, guard, r.ID, r.Record, time.Hour, false); err != nil {
			t.Fatal(err)
		}
		records[i] = r
	}
	t.Cleanup(func() {
		for _, r := range records {
			guard.Del(ctx, r.ID)
		}
		for _, set := range []string{clicks, imps} {
			for _, id := range stats.SMembers(ctx, set).Val() {
				stats.Del(ctx, id)
			}
			stats.Del(ctx, set)
		}
	})
	box, err := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	var calls atomic.Int32
	client := &http.Client{Transport: callbackTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	executor := newPublisherDSPExecutor(ctx, client, 2, 8)
	defer executor.Close()
	q := newPublisherBURLQueue(ctx, box, guard, nil, 8, 100000, 20*time.Second, 24*time.Hour, func(c context.Context, r publisherBURLInput) error {
		return processBurl(c, r, client, []*redis.Client{stats}, imps, nil, "", nil, box, nil, executor)
	})
	defer q.Close()
	begin := time.Now()
	var wg sync.WaitGroup
	for _, r := range records {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/adm?id="+id, nil)
			req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &admRequest{GlobalId: id, DspURL: "https://advertiser.test/original", Format: "0", Publisher: "1"}))
			req = req.WithContext(context.WithValue(req.Context(), publisherBurlKey{}, publisherBurlFunc(q.enqueue)))
			getAdm(ctx, w, req, []*redis.Client{stats}, guard, clicks, nil, "", nil, box, nil)
			if w.Code != 200 || !strings.Contains(w.Body.String(), "advertiser.test/original") {
				t.Errorf("ADM status=%d", w.Code)
			}
		}(r.ID)
	}
	wg.Wait()
	redirectDuration := time.Since(begin)
	deadline := time.Now().Add(15 * time.Second)
	for {
		n, _ := box.TaskCount()
		if stats.SCard(ctx, imps).Val() == 1000 && n == 0 && len(q.slots) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("internal work stalled impressions=%d backlog=%d", stats.SCard(ctx, imps).Val(), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, r := range records {
		if guard.HGet(ctx, r.ID, "pub_burl_done").Val() != "1" {
			t.Fatal("uncommitted guard", r.ID)
		}
	}
	if stats.SCard(ctx, clicks).Val() != 1000 {
		t.Fatal("ADM clicks changed")
	}
	if len(executor.tasks) > 8 || executor.active.Load() > 2 || q.lost.Load() != 0 {
		t.Fatal("unbounded/lost internal tasks")
	}
	t.Logf("1000 concurrent /adm: redirects=%s total_internal=%s internal_fallback=%d DSP_active=%d DSP_queue=%d DSP_dropped=%d", redirectDuration, time.Since(begin), q.fallbackAttempts.Load(), executor.active.Load(), len(executor.tasks), executor.dropped.Load())
}

func TestPublisherV4SlowDSPReleasesMainWorkers(t *testing.T) {
	ctx := context.Background()
	guard, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	records := make([]publisherBURLInput, 1000)
	set := "v4-internal:" + uuid.NewString()
	for i := range records {
		r := publisherFastRecord()
		r.Redis = guard
		r.Record.BURL = "https://dsp.test/delegated-nurl?keep=%2F"
		records[i] = r
		if err := seedPublisherWinner(ctx, guard, r.ID, r.Record, time.Hour, false); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, r := range records {
			guard.Del(ctx, r.ID)
		}
		for _, id := range stats.SMembers(ctx, set).Val() {
			stats.Del(ctx, id)
		}
		stats.Del(ctx, set)
	})
	var calls atomic.Int32
	client := &http.Client{Transport: callbackTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.URL.String() != "https://dsp.test/delegated-nurl?keep=%2F" {
			t.Error("changed callback purpose", req.URL)
		}
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	executor := newPublisherDSPExecutor(ctx, client, 2, 3)
	defer executor.Close()
	box, err := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	q := newPublisherBURLQueue(ctx, box, guard, nil, 8, 100000, 20*time.Second, 24*time.Hour, func(c context.Context, r publisherBURLInput) error {
		return processBurl(c, r, client, []*redis.Client{stats}, set, nil, "", nil, box, nil, executor)
	})
	defer q.Close()
	before := runtime.NumGoroutine()
	begin := time.Now()
	// Saturate external HTTP first, then feed eight internal workers at a time.
	// This isolates DSP-induced fallback from an instantaneous 1000-job RAM burst.
	for i := 0; i < len(records); i += 8 {
		end := i + 8
		if end > len(records) {
			end = len(records)
		}
		for j := i; j < end; j++ {
			q.enqueue(ctx, records[j])
		}
		waitPublisher(t, func() bool { return len(q.slots) == 0 })
	}
	if q.fallbackAttempts.Load() != 0 || executor.dropped.Load() == 0 || stats.SCard(ctx, set).Val() != 1000 || len(executor.tasks) > 3 || executor.active.Load() > 2 {
		t.Fatal("DSP delayed internal BURL", q.fallbackAttempts.Load(), executor.dropped.Load(), stats.SCard(ctx, set).Val())
	}
	t.Logf("1000 internal BURLs under blocked DSP: elapsed=%s internal_fallback=%d DSP_active=%d DSP_queue=%d DSP_dropped=%d goroutine_delta=%d", time.Since(begin), q.fallbackAttempts.Load(), executor.active.Load(), len(executor.tasks), executor.dropped.Load(), runtime.NumGoroutine()-before)
	// Full and stopped DSP executors must still leave the successful guard intact.
	executor.Close()
	for _, r := range records[:10] {
		if err := processBurl(ctx, r, client, []*redis.Client{stats}, set, nil, "", nil, box, nil, executor); err != nil {
			t.Fatal(err)
		}
	}
	if stats.SCard(ctx, set).Val() != 1000 {
		t.Fatal("repeat after overflow rebilled")
	}
	r := publisherFastRecord()
	r.Redis = guard
	r.Record.BURL = "https://dsp.test/burl"
	defer guard.Del(ctx, r.ID)
	if err := processBurl(ctx, r, client, []*redis.Client{stats}, set, nil, "", nil, box, nil, executor); err != nil || guard.HGet(ctx, r.ID, "pub_burl_done").Val() != "1" {
		t.Fatal("stopped executor broke internal success", err)
	}
}

func TestPublisherV4ThreeSecondDSPTimeoutAndGuard(t *testing.T) {
	ctx := context.Background()
	guard, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	r := publisherFastRecord()
	r.Redis = guard
	r.Record.BURL = "https://dsp.test/callback"
	defer guard.Del(ctx, r.ID)
	set := "v4-timeout:" + r.ID
	defer func() {
		for _, id := range stats.SMembers(ctx, set).Val() {
			stats.Del(ctx, id)
		}
		stats.Del(ctx, set)
	}()
	started, finished := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	client := &http.Client{Transport: callbackTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		<-req.Context().Done()
		close(finished)
		return nil, req.Context().Err()
	})}
	executor := newPublisherDSPExecutor(ctx, client, 1, 1)
	defer executor.Close()
	begin := time.Now()
	if err := processBurl(ctx, r, client, []*redis.Client{stats}, set, nil, "", nil, nil, nil, executor); err != nil {
		t.Fatal(err)
	}
	internal := time.Since(begin)
	if internal > time.Second || guard.HGet(ctx, r.ID, "pub_burl_done").Val() != "1" || guard.HExists(ctx, r.ID, "pub_burl_lease").Val() {
		t.Fatal("HTTP retained guard", internal)
	}
	<-started
	for i := 0; i < 20; i++ {
		if err := processBurl(ctx, r, client, []*redis.Client{stats}, set, nil, "", nil, nil, nil, executor); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-finished:
		t.Fatal("DSP already completed")
	default:
	}
	select {
	case <-finished:
	case <-time.After(4 * time.Second):
		t.Fatal("3s timeout not enforced")
	}
	if calls.Load() != 1 || stats.SCard(ctx, set).Val() != 1 {
		t.Fatal("duplicate callback or impression")
	}
	t.Logf("internal=%s DSP_timeout=%s", internal, time.Since(begin))
}

func TestPublisherV4OrdinaryBURLStillWaitsForHTTP(t *testing.T) {
	ctx := context.Background()
	stats := callbackRedis(t, 12)
	r := publisherFastRecord()
	r.Record.PublisherID = ""
	r.Record.BURL = "https://dsp.test/ordinary"
	set := "v4-ordinary:" + r.ID
	defer func() {
		for _, id := range stats.SMembers(ctx, set).Val() {
			stats.Del(ctx, id)
		}
		stats.Del(ctx, set)
	}()
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	defer close(release)
	client := &http.Client{Transport: callbackTransport(func(req *http.Request) (*http.Response, error) {
		close(started)
		select {
		case <-release:
		case <-req.Context().Done():
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})}
	callCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	go func() {
		finished <- processBurl(callCtx, r, client, []*redis.Client{stats}, set, nil, "", nil, nil, nil)
	}()
	select {
	case <-started:
	case err := <-finished:
		t.Fatal("ordinary internal BURL failed", err)
	case <-time.After(time.Second):
		t.Fatal("HTTP not started")
	}
	select {
	case err := <-finished:
		t.Fatal("ordinary callback unexpectedly async", err)
	default:
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("ordinary callback stuck")
	}
}

func TestPublisherV4NetworkFailureDoesNotRetry(t *testing.T) {
	ctx := context.Background()
	guard, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	r := publisherFastRecord()
	r.Redis = guard
	r.Record.BURL = "https://dsp.test/network"
	defer guard.Del(ctx, r.ID)
	set := "v4-network:" + r.ID
	defer func() {
		for _, id := range stats.SMembers(ctx, set).Val() {
			stats.Del(ctx, id)
		}
		stats.Del(ctx, set)
	}()
	var calls atomic.Int32
	client := &http.Client{Transport: callbackTransport(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("network unavailable")
	})}
	executor := newPublisherDSPExecutor(ctx, client, 1, 1)
	defer executor.Close()
	for i := 0; i < 2; i++ {
		if err := processBurl(ctx, r, client, []*redis.Client{stats}, set, nil, "", nil, nil, nil, executor); err != nil {
			t.Fatal(err)
		}
	}
	waitPublisher(t, func() bool { return calls.Load() == 1 && executor.active.Load() == 0 })
	if guard.HGet(ctx, r.ID, "pub_burl_done").Val() != "1" || stats.SCard(ctx, set).Val() != 1 {
		t.Fatal("network error undid Redis success")
	}
}

// Exercise the public handler too: only Publisher POP takes the executor path.
func TestPublisherV4InternalBURLUsesExecutor(t *testing.T) {
	ctx := context.Background()
	guard, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	r := publisherFastRecord()
	r.Redis = guard
	r.Record.BURL = "https://dsp.test/public"
	if err := seedPublisherWinner(ctx, guard, r.ID, r.Record, time.Hour, false); err != nil {
		t.Fatal(err)
	}
	defer guard.Del(ctx, r.ID)
	set := "v4-public:" + r.ID
	defer func() {
		for _, id := range stats.SMembers(ctx, set).Val() {
			stats.Del(ctx, id)
		}
		stats.Del(ctx, set)
	}()
	client := &http.Client{Transport: callbackTransport(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	executor := newPublisherDSPExecutor(ctx, client, 1, 1)
	defer executor.Close()
	begin := time.Now()
	err := processBurl(ctx, r, client, []*redis.Client{stats}, set, nil, "", nil, nil, nil, executor)
	if err != nil || time.Since(begin) > time.Second {
		t.Fatal("internal Publisher BURL waited", err, time.Since(begin))
	}
}

type publisherV4CompletionHook struct{ fail atomic.Bool }

func (h *publisherV4CompletionHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *publisherV4CompletionHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *publisherV4CompletionHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.fail.Load() && cmd.Name() == "eval" && strings.Contains(cmd.Args()[1].(string), "HDEL") {
			return errors.New("injected Redis guard completion failure")
		}
		return next(ctx, cmd)
	}
}

func TestPublisherV4GuardCompletionFailureUsesBoltWithoutDSP(t *testing.T) {
	ctx := context.Background()
	guard, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	hook := &publisherV4CompletionHook{}
	guard.AddHook(hook)
	r := publisherFastRecord()
	r.Redis = guard
	r.Record.BURL = "https://dsp.test/burl"
	defer guard.Del(ctx, r.ID)
	set := "v4-completion:" + r.ID
	defer func() {
		for _, id := range stats.SMembers(ctx, set).Val() {
			stats.Del(ctx, id)
		}
		stats.Del(ctx, set)
	}()
	var calls atomic.Int32
	client := &http.Client{Transport: callbackTransport(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})}
	executor := newPublisherDSPExecutor(ctx, client, 1, 1)
	defer executor.Close()
	box, err := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	hook.fail.Store(true)
	q := newPublisherBURLQueue(ctx, box, guard, nil, 1, 1000, time.Second, 24*time.Hour, func(c context.Context, r publisherBURLInput) error {
		return processBurl(c, r, client, []*redis.Client{stats}, set, nil, "", nil, box, nil, executor)
	})
	defer q.Close()
	q.enqueue(ctx, r)
	waitPublisher(t, func() bool { n, _ := box.TaskCount(); return n == 1 && len(q.slots) == 0 })
	if calls.Load() != 0 || guard.HGet(ctx, r.ID, "pub_burl_done").Val() == "1" || q.fallbackAttempts.Load() != 1 {
		t.Fatal("completion failure reported success")
	}
	// Simulate the existing lease expiry so the durable retry can take ownership.
	hook.fail.Store(false)
	guard.HSet(ctx, r.ID, "pub_burl_until", 0)
	waitPublisher(t, func() bool { n, _ := box.TaskCount(); return n == 0 && calls.Load() == 1 })
	if stats.SCard(ctx, set).Val() != 1 || guard.HGet(ctx, r.ID, "pub_burl_done").Val() != "1" {
		t.Fatal("completion retry not idempotent")
	}
}
