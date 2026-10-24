package sppAdapterWeb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ggicci/httpin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

// Opt-in latency experiment. Same fixture runs unchanged against v2 and v3:
// real local HTTP, Redis and fsynced Bolt, 1000 unique winners, concurrency 32.
// It measures response receipt, not external internet/DSP latency or target RPS.
func TestPublisherADMLatencySample(t *testing.T) {
	label := os.Getenv("PUBLISHER_ADM_BENCH_LABEL")
	if label == "" {
		t.Skip("set PUBLISHER_ADM_BENCH_LABEL on disposable Redis")
	}
	for _, publisher := range []bool{true, false} {
		t.Run(fmt.Sprint(publisher), func(t *testing.T) {
			const count = 1000
			ctx := context.Background()
			dsp, stats := callbackRedis(t, 10), callbackRedis(t, 12)
			ids := make([]string, count)
			set := "latency:" + uuid.NewString()
			imps := set + ":imps"
			defer func() {
				dsp.Del(ctx, ids...)
				for _, set := range []string{set, imps} {
					for _, id := range stats.SMembers(ctx, set).Val() {
						stats.Del(ctx, id)
					}
					stats.Del(ctx, set)
				}
			}()
			for i := range ids {
				ids[i] = uuid.NewString()
				r := publisherBURLRecord{Format: "POP", WinnerType: "dsp"}
				if publisher {
					r.PublisherID = "publisher"
				}
				if e := seedPublisherWinner(ctx, dsp, ids[i], r, time.Hour, false); e != nil {
					t.Fatal(e)
				}
			}
			box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
			if e != nil {
				t.Fatal(e)
			}
			defer box.Close()
			q := newPublisherBURLQueue(ctx, box, dsp, nil, 8, 100000, 20*time.Second, 24*time.Hour, func(c context.Context, r publisherBURLInput) error {
				return processBurl(c, r, &http.Client{Timeout: 3 * time.Second}, []*redis.Client{stats}, imps, nil, "", nil, box, nil)
			})
			defer q.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r = r.WithContext(context.WithValue(r.Context(), httpin.Input, &admRequest{GlobalId: r.URL.Query().Get("id"), DspURL: "https://advertiser.test/landing", Format: "0", Publisher: r.URL.Query().Get("p")}))
				r = r.WithContext(context.WithValue(r.Context(), publisherBurlKey{}, publisherBurlFunc(q.enqueue)))
				getAdm(ctx, w, r, []*redis.Client{stats}, dsp, set, nil, "", nil, box, nil)
			}))
			defer server.Close()
			client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{MaxIdleConns: 32, MaxIdleConnsPerHost: 32}}
			defer client.CloseIdleConnections()
			durations := make([]time.Duration, count)
			jobs := make(chan int)
			var wg sync.WaitGroup
			begin := time.Now()
			for i := 0; i < 32; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for index := range jobs {
						start := time.Now()
						response, e := client.Get(server.URL + "/adm?id=" + ids[index] + map[bool]string{true: "&p=1", false: ""}[publisher])
						if e != nil {
							t.Error(e)
							continue
						}
						_, e = io.Copy(io.Discard, response.Body)
						response.Body.Close()
						durations[index] = time.Since(start)
						if e != nil || response.StatusCode != 200 {
							t.Error(response.StatusCode, e)
						}
					}
				}()
			}
			for i := 0; i < count; i++ {
				jobs <- i
			}
			close(jobs)
			wg.Wait()
			elapsed := time.Since(begin)
			sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
			t.Logf("LATENCY label=%s publisher=%t n=%d concurrency=32 p50=%s p95=%s p99=%s elapsed=%s", label, publisher, count, durations[499], durations[949], durations[989], elapsed)
		})
	}
}
