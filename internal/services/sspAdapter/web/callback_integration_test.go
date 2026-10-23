package sppAdapterWeb

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ggicci/httpin"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	events "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	chloader "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/clickhouse-loader"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/billing"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
	"google.golang.org/protobuf/proto"
)

type callbackTransport func(*http.Request) (*http.Response, error)

func (f callbackTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func callbackRedis(t *testing.T, db int) *redis.Client {
	t.Helper()
	addr := os.Getenv("PUB_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("PUB_TEST_REDIS_ADDR unset")
	}
	c := redis.NewClient(&redis.Options{Addr: addr, DB: db, MaxRetries: 1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() { c.Close() })
	return c
}
func seedPublisherWinner(ctx context.Context, c *redis.Client, id string, r publisherBURLRecord, ttl time.Duration, adv bool) error {
	if adv {
		return nil
	} // Existing financial winner fields are seeded by the test.
	if e := utils.WriteUUIDKeyToRedis(ctx, c, id, ttl); e != nil {
		return e
	}
	if r.BURL != "" {
		return c.HSet(ctx, id, "pub_dsp_nurl", r.BURL).Err()
	}
	return nil
}
func testPublisherInput(ctx context.Context, id string, c *redis.Client, store *billing.Store) (publisherBURLInput, error) {
	out := publisherBURLInput{ID: id, Record: publisherBURLRecord{Format: "POP", PublisherID: "1"}}
	if store != nil {
		var e error
		out.Winner, e = store.ReadWinner(ctx, id, "POP")
		out.Record.WinnerType = "adv"
		out.Redis = store.WinnerRedis()
		return out, e
	}
	fields, e := c.HGetAll(ctx, id).Result()
	if e != nil {
		return out, e
	}
	if fields["uuid"] != id {
		return out, redis.Nil
	}
	out.Record.WinnerType = "dsp"
	out.Record.BURL = fields["pub_dsp_nurl"]
	out.Redis = c
	return out, nil
}
func clearStats(ctx context.Context, c *redis.Client, sets ...string) {
	for _, set := range sets {
		for _, id := range c.SMembers(ctx, set).Val() {
			c.Del(ctx, id)
		}
		c.Del(ctx, set)
	}
}

func TestLegacyHTTPRoutesAllFormatsWithoutContext(t *testing.T) {
	ctx := context.Background()
	dsp, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	var notifications atomic.Int32
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { notifications.Add(1); w.WriteHeader(204) }))
	defer downstream.Close()
	router := chi.NewRouter()
	set := "legacy:" + uuid.NewString()
	imps := set + ":imps"
	stop := InitHttpsRoutes(ctx, router, []*redis.Client{stats}, dsp, set, time.Second, time.Second, nil, "", dsp, imps, []*redis.Client{stats}, set+":conv", []*redis.Client{stats}, set+":wins", []*redis.Client{stats}, nil, nil, nil)
	defer stop()
	defer clearStats(ctx, stats, set, imps)
	for _, format := range []string{"POP", "BAN", "NAT", "IPP", "VID"} {
		t.Run(format, func(t *testing.T) {
			id := uuid.NewString()
			if e := utils.WriteUUIDKeyToRedis(ctx, dsp, id, time.Hour); e != nil {
				t.Fatal(e)
			}
			defer dsp.Del(ctx, id)
			for _, path := range []string{"adm", "nurl", "burl"} {
				q := url.Values{"id": {id}, "f": {constants.FormatToCodes[format]}}
				if path == "adm" {
					q.Set("url", "http://127.0.0.1/landing?x=1&y=two")
				} else {
					q.Set("url", downstream.URL+"/"+path)
					if path == "nurl" {
						q.Set("s", "ordinary-ssp")
					}
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest("GET", "/"+path+"?"+q.Encode(), nil))
				want := 204
				if path == "adm" {
					want = 200
					if !strings.Contains(w.Body.String(), "127.0.0.1/landing") {
						t.Fatal(w.Body.String())
					}
				}
				if w.Code != want {
					t.Fatal(path, w.Code, w.Body.String())
				}
			}
			if dsp.HExists(ctx, id, "callback_context_v1").Val() {
				t.Fatal("new callback context written")
			}
			// p selects the Publisher path only for POP, with the existing winner guard.
			var calls atomic.Int32
			req := httptest.NewRequest("GET", "/adm", nil)
			req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &admRequest{GlobalId: id, DspURL: "http://127.0.0.1/landing", Format: constants.FormatToCodes[format], Publisher: "1"}))
			req = req.WithContext(context.WithValue(req.Context(), publisherBurlKey{}, publisherBurlFunc(func(context.Context, publisherBURLInput) { calls.Add(1) })))
			getAdm(ctx, httptest.NewRecorder(), req, []*redis.Client{stats}, dsp, set, nil, "", nil, nil, nil)
			wantCalls := int32(0)
			if format == "POP" {
				wantCalls = 1
			}
			if calls.Load() != wantCalls {
				t.Fatal("Publisher flag/format dispatch", calls.Load())
			}
		})
	}
	if notifications.Load() != 10 {
		t.Fatal("legacy local URLs filtered", notifications.Load())
	}
	if stats.SCard(ctx, imps).Val() != 5 {
		t.Fatal("ordinary POP created automatic BURL", stats.SCard(ctx, imps).Val())
	}
	// Legacy required params remain required; id alone is no longer a callback.
	for _, path := range []string{"adm", "nurl", "burl"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/"+path+"?id="+uuid.NewString(), nil))
		if w.Code != 400 && w.Code != 422 {
			t.Fatal(path, w.Code)
		}
	}
}

func TestPublisherADMDoesNotWaitForInternalBURL(t *testing.T) {
	ctx := context.Background()
	dsp, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	id := uuid.NewString()
	clicks, imps := "pub-click:"+id, "pub-imp:"+id
	defer dsp.Del(ctx, id)
	defer clearStats(ctx, stats, clicks, imps)
	record := publisherBURLRecord{Format: "POP", WinnerType: "dsp", PublisherID: "publisher", BURL: "http://127.0.0.1/downstream"}
	if e := seedPublisherWinner(ctx, dsp, id, record, time.Hour, false); e != nil {
		t.Fatal(e)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	client := &http.Client{Transport: callbackTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
		return nil, errors.New("DSP offline")
	})}
	box, e := outbox.Open(filepath.Join(t.TempDir(), "publisher.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	executor := newPublisherDSPExecutor(ctx, client, 2, 16)
	defer executor.Close()
	queue := newPublisherBURLQueue(ctx, box, dsp, nil, 2, 100000, 20*time.Second, 24*time.Hour, func(c context.Context, res publisherBURLInput) error {
		return processBurl(c, res, client, []*redis.Client{stats}, imps, nil, "", nil, box, nil, executor)
	})
	defer queue.Close()
	// Attribution survives the raw ORTB hash being consumed/deleted by Loader.
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req := httptest.NewRequest("GET", "/adm", nil).WithContext(reqCtx)
	req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &admRequest{GlobalId: id, Format: "0", Publisher: "1", DspURL: url.QueryEscape("https://advertiser.test/landing?click_id=original&x=%2F")}))
	req = req.WithContext(context.WithValue(req.Context(), publisherBurlKey{}, publisherBurlFunc(queue.enqueue)))
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { getAdm(ctx, w, req, []*redis.Client{stats}, dsp, clicks, nil, "", nil, box, nil); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ADM waited for DSP")
	}
	cancel()
	if w.Code != 200 || !w.Flushed || !strings.Contains(w.Body.String(), "advertiser.test/landing") {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("internal BURL not started")
	}
	close(release)
	waitPublisher(t, func() bool { return dsp.HGet(ctx, id, "pub_burl_done").Val() == "1" })
	resolved, e := testPublisherInput(ctx, id, dsp, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = processBurl(ctx, resolved, client, []*redis.Client{stats}, imps, nil, "", nil, box, nil, executor); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 1 || stats.SCard(ctx, imps).Val() != 1 || stats.SCard(ctx, clicks).Val() != 1 {
		t.Fatal("duplicate effects", calls.Load())
	}
	t.Run("realClickHouseRevenue", func(t *testing.T) { assertADMClickIncome(t, stats, clicks, imps, id) })
}

func TestLegacyADVWinnerWithNoCallbackContextBillsOnce(t *testing.T) {
	ctx := context.Background()
	dsp, adv, runtime, stats := callbackRedis(t, 10), callbackRedis(t, 11), callbackRedis(t, 9), callbackRedis(t, 12)
	id, user, campaign := uuid.NewString(), uuid.NewString(), uuid.NewString()
	set := "adv-burl:" + id
	adv.HSet(ctx, id, "price", ".25", "user_id", user, "campaign_id", campaign, "format", "POP", "type_model", 2, "click_id_param", "cid")
	adv.Expire(ctx, id, time.Hour)
	// A corrupt retired field must be irrelevant to the restored winner reader.
	adv.HSet(ctx, id, "callback_context_v1", "not-json")
	defer adv.Del(ctx, id)
	defer clearStats(ctx, stats, set)
	defer runtime.Del(ctx, "spent:user:"+user, "spent:campaign:"+campaign, "outbox:applied:"+billing.CallbackEventID("burl", "POP", id))
	box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	store := billing.NewStore(runtime, adv, time.Hour)
	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 20 * time.Millisecond})
	defer broken.Close()
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/burl", nil)
		req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &burlRequest{GlobalId: id, Format: "0"}))
		w := httptest.NewRecorder()
		winnerDB := dsp
		if i == 1 {
			winnerDB = broken
		}
		getBurl(ctx, w, req, &http.Client{}, []*redis.Client{stats}, winnerDB, set, nil, "", store, box, nil)
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if runtime.Get(ctx, "spent:user:"+user).Val() != "0.25" {
		t.Fatal("duplicate ADV spend")
	}
	winner, e := store.ReadWinner(ctx, id, "POP")
	if e != nil || winner.TypeModel != 2 {
		t.Fatal(winner, e)
	}
}

// Actual /adm Redis click -> Kafka protobuf boundary -> actual clicks_in / fact
// refreshes / financial aggregate. Only the Kafka broker transport is omitted.
func assertADMClickIncome(t *testing.T, stats *redis.Client, clickSet, impSet, auction string) {
	t.Helper()
	addr := os.Getenv("PUBLISHER_TEST_CLICKHOUSE_ADDR")
	if addr == "" {
		t.Skip("PUBLISHER_TEST_CLICKHOUSE_ADDR unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ch, e := clickhouse.Open(&clickhouse.Options{Addr: []string{addr}, Auth: clickhouse.Auth{Username: "default"}})
	if e != nil {
		t.Fatal(e)
	}
	defer ch.Close()
	db := "adm_income_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	defer ch.Exec(context.Background(), "DROP DATABASE IF EXISTS "+db+" SYNC")
	if e = chloader.CreateDB(ctx, ch, db); e != nil {
		t.Fatal(e)
	}
	must := func(sql string, args ...any) {
		t.Helper()
		if e := ch.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	refresh := func(name string) {
		t.Helper()
		must("SYSTEM REFRESH VIEW " + db + "." + name)
		must("SYSTEM WAIT VIEW " + db + "." + name)
	}
	must("INSERT INTO "+db+".ortb(uuid,event_time,code,format,typic,pub_user_id,ad_code_id,zone_id,site_id,win_final_price,win_dsp_price) VALUES(?,now64(3),200,'POP','MAINSTREAM','publisher','code','zone','site',8,10)", auction)
	clickIDs := stats.SMembers(ctx, clickSet).Val()
	if len(clickIDs) != 1 {
		t.Fatal(clickIDs)
	}
	f, e := stats.HGetAll(ctx, clickIDs[0]).Result()
	if e != nil {
		t.Fatal(e)
	}
	stamp, e := time.ParseInLocation("2006-01-02 15:04:05.000", f[constants.EVENT_TIME_CLICKS_COLUMN], time.UTC)
	if e != nil {
		t.Fatal(e)
	}
	message := &events.ClickEvent{ClicksUuid: clickIDs[0], OrtbUuid: f[constants.ORTB_UUID], Format: f[constants.FORMAT_COLUMN], EventTimeClicksMs: stamp.UnixMilli()}
	payload, e := proto.Marshal(message)
	if e != nil {
		t.Fatal(e)
	}
	var decoded events.ClickEvent
	if e = proto.Unmarshal(payload, &decoded); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		must("INSERT INTO "+db+".clicks_in(clicks_uuid,uuid,event_time_clicks,ad_format) VALUES(?,?,?,?)", decoded.ClicksUuid, decoded.OrtbUuid, time.UnixMilli(decoded.EventTimeClicksMs), decoded.Format)
		refresh("mv_clicks_to_fact")
	}
	refresh("mv_publisher_pop_revenue_to_agg_stats")
	check := func() {
		t.Helper()
		var revenue float64
		var clicks uint64
		if e = ch.QueryRow(ctx, "SELECT sum(ssp_revenue_views),sum(clicks) FROM "+db+".agg_stats WHERE pub_user_id='publisher'").Scan(&revenue, &clicks); e != nil || math.Abs(revenue-.008) > 1e-10 || clicks != 1 {
			t.Fatal("one ADM not one credit", revenue, clicks, e)
		}
	}
	check()
	for _, impID := range stats.SMembers(ctx, impSet).Val() {
		must("INSERT INTO "+db+".impressions_in(impressions_uuid,uuid,event_time_impressions,ad_format) VALUES(?,?,now64(3),'POP')", impID, auction)
	}
	refresh("mv_impressions_to_fact")
	refresh("mv_publisher_pop_revenue_to_agg_stats")
	check()
	var impressions uint64
	var advertiser float64
	if e = ch.QueryRow(ctx, "SELECT sum(impressions),sum(spend_views_table) FROM "+db+".agg_stats").Scan(&impressions, &advertiser); e != nil || impressions != 1 || math.Abs(advertiser-.010) > 1e-10 {
		t.Fatal("BURL advertiser accounting", impressions, advertiser, e)
	}
}

func TestPublisherAutomaticBURLRequiresPOPAndExplicitFlag(t *testing.T) {
	ctx := context.Background()
	dsp, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	id := uuid.NewString()
	set := "flag:" + id
	if e := seedPublisherWinner(ctx, dsp, id, publisherBURLRecord{Format: "POP", WinnerType: "dsp", PublisherID: "owner"}, time.Hour, false); e != nil {
		t.Fatal(e)
	}
	defer dsp.Del(ctx, id)
	defer clearStats(ctx, stats, set)
	for _, pair := range []struct{ f, p string }{{"0", ""}, {"0", "0"}, {"1", "1"}, {"2", "1"}, {"3", "1"}, {"4", "1"}} {
		var calls atomic.Int32
		req := httptest.NewRequest("GET", "/adm", nil)
		req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &admRequest{GlobalId: id, Format: pair.f, Publisher: pair.p, DspURL: "https://advertiser.test/original"}))
		req = req.WithContext(context.WithValue(req.Context(), publisherBurlKey{}, publisherBurlFunc(func(context.Context, publisherBURLInput) { calls.Add(1) })))
		w := httptest.NewRecorder()
		getAdm(ctx, w, req, []*redis.Client{stats}, dsp, set, nil, "", nil, nil, nil)
		if w.Code != 200 || calls.Load() != 0 {
			t.Fatal(pair, w.Code, calls.Load())
		}
	}
}

func TestPublisherADMNeedsWinnerBeforeBURL(t *testing.T) {
	ctx := context.Background()
	dsp, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	id := uuid.NewString()
	set := "missing-winner:" + id
	defer clearStats(ctx, stats, set)
	req := httptest.NewRequest("GET", "/adm", nil)
	req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &admRequest{GlobalId: id, Format: "0", Publisher: "1", DspURL: "https://advertiser.test/"}))
	var calls atomic.Int32
	req = req.WithContext(context.WithValue(req.Context(), publisherBurlKey{}, publisherBurlFunc(func(context.Context, publisherBURLInput) { calls.Add(1) })))
	w := httptest.NewRecorder()
	getAdm(ctx, w, req, []*redis.Client{stats}, dsp, set, nil, "", nil, nil, nil)
	if w.Code != 200 || calls.Load() != 0 || stats.SCard(ctx, set).Val() != 0 {
		t.Fatal("p bypassed missing winner", w.Code)
	}
}
