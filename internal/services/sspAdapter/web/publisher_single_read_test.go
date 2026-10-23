package sppAdapterWeb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ggicci/httpin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/billing"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

// Count actual Redis commands on the winner key, including pipeline commands.
// Financial lease EVAL and click statistics are independent of winner lookup.
type winnerReadHook struct {
	id    string
	mu    sync.Mutex
	reads map[string]int
}

func (h *winnerReadHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *winnerReadHook) count(cmd redis.Cmder) {
	args := cmd.Args()
	if len(args) > 1 && args[1] == h.id {
		switch cmd.Name() {
		case "hgetall", "hget", "hmget", "exists":
			h.mu.Lock()
			h.reads[cmd.Name()]++
			h.mu.Unlock()
		}
	}
}
func (h *winnerReadHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error { h.count(cmd); return next(ctx, cmd) }
}
func (h *winnerReadHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			h.count(cmd)
		}
		return next(ctx, cmds)
	}
}
func (h *winnerReadHook) assert(t *testing.T, wantAll, wantExists int) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.reads["hgetall"] != wantAll || h.reads["exists"] != wantExists || h.reads["hget"] != 0 || h.reads["hmget"] != 0 {
		t.Fatal("winner reads", h.reads)
	}
}

func TestPublisherADMUsesLegacyUUIDCheckAndQueryNURL(t *testing.T) {
	for _, kind := range []string{"dsp", "dsp-no-nurl", "adv", "ordinary"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dsp, adv, spend, stats := callbackRedis(t, 10), callbackRedis(t, 11), callbackRedis(t, 9), callbackRedis(t, 12)
			id, user, campaign := uuid.NewString(), uuid.NewString(), uuid.NewString()
			clicks, imps := "one-read-click:"+id, "one-read-imp:"+id
			defer dsp.Del(context.Background(), id)
			defer adv.Del(context.Background(), id)
			defer clearStats(context.Background(), stats, clicks, imps)
			defer spend.Del(context.Background(), "spent:user:"+user, "spent:campaign:"+campaign, "outbox:applied:"+billing.CallbackEventID("burl", "POP", id))
			var downstreamCalls atomic.Int32
			downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { downstreamCalls.Add(1); w.WriteHeader(204) }))
			defer downstream.Close()
			if kind == "adv" {
				adv.HSet(ctx, id, "format", "POP", "price", "0.25", "user_id", user, "campaign_id", campaign)
				adv.Expire(ctx, id, time.Hour)
			} else {
				if e := utils.WriteUUIDKeyToRedis(ctx, dsp, id, time.Hour); e != nil {
					t.Fatal(e)
				}

			}
			dspHook := &winnerReadHook{id: id, reads: map[string]int{}}
			dsp.AddHook(dspHook)
			advHook := &winnerReadHook{id: id, reads: map[string]int{}}
			adv.AddHook(advHook)
			box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
			if e != nil {
				t.Fatal(e)
			}
			defer box.Close()
			store := billing.NewStore(spend, adv, time.Hour)
			httpClient := &http.Client{Timeout: time.Second}
			executor := newPublisherDSPExecutor(ctx, httpClient, 2, 8)
			defer executor.Close()
			queue := newPublisherBURLQueue(ctx, box, dsp, store, 2, 100, time.Second, 24*time.Hour, func(c context.Context, r publisherBURLInput) error {
				return processBurl(c, r, httpClient, []*redis.Client{stats}, imps, nil, "", store, box, nil, executor)
			})
			defer queue.Close()
			nurl := ""
			if kind == "dsp" {
				nurl = downstream.URL + "/dsp-notification"
			}
			for i := 0; i < 2; i++ {
				flag := "1"
				if kind == "ordinary" {
					flag = ""
				}
				req := httptest.NewRequest("GET", "/adm", nil)
				req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &admRequest{GlobalId: id, Format: "0", Publisher: flag, NURL: nurl, DspURL: "https://advertiser.test/landing"}))
				req = req.WithContext(context.WithValue(req.Context(), publisherBurlKey{}, publisherBurlFunc(queue.enqueue)))
				w := httptest.NewRecorder()
				getAdm(ctx, w, req, []*redis.Client{stats}, dsp, clicks, nil, "test-status", store, box, nil)
				if w.Code != 200 || !strings.Contains(w.Body.String(), "advertiser.test/landing") {
					t.Fatal("redirect lost", w.Code)
				}
			}
			dspHook.assert(t, 0, 2)
			wantADV := 0
			if kind == "adv" {
				wantADV = 2
			}
			advHook.assert(t, wantADV, 0)
			if kind == "ordinary" {
				if stats.SCard(ctx, imps).Val() != 0 {
					t.Fatal("ordinary POP auto BURL")
				}
				return
			}
			guard := dsp
			if kind == "adv" {
				guard = adv
			}
			waitPublisher(t, func() bool { return guard.HGet(ctx, id, "pub_burl_done").Val() == "1" })
			if stats.SCard(ctx, imps).Val() != 1 {
				t.Fatal("missing/duplicate internal impression")
			}
			if kind == "dsp" {
				waitPublisher(t, func() bool { return downstreamCalls.Load() == 1 })
			} else if downstreamCalls.Load() != 0 {
				t.Fatal("HTTP request without DSP NURL")
			}
			if kind == "adv" && spend.Get(ctx, "spent:user:"+user).Val() != "0.25" {
				t.Fatal("duplicate ADV charge")
			}

		})
	}
}

func TestPublisherADMUnavailableRedisUsesExistingOutboxAndRedirect(t *testing.T) {
	ctx := context.Background()
	id := uuid.NewString()
	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 20 * time.Millisecond})
	defer broken.Close()
	box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	req := httptest.NewRequest("GET", "/adm", nil)
	req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &admRequest{GlobalId: id, Format: "0", Publisher: "1", DspURL: "https://advertiser.test/original"}))
	var calls atomic.Int32
	req = req.WithContext(context.WithValue(req.Context(), publisherBurlKey{}, publisherBurlFunc(func(context.Context, publisherBURLInput) { calls.Add(1) })))
	w := httptest.NewRecorder()
	getAdm(ctx, w, req, nil, broken, "clicks", nil, "", nil, box, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "advertiser.test/original") || calls.Load() != 0 {
		t.Fatal("failed winner lookup changed redirect/authorized BURL", w.Code)
	}
	records, e := box.List()
	if e != nil || len(records) != 1 || records[0].Kind != outbox.KindADM || records[0].WinnerType != outbox.WinnerUnknown {
		t.Fatal("existing ADM outbox not used", records, e)
	}
}
