package sppAdapterWeb

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/callback"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	events "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	chloader "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/clickhouse-loader"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/billing"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
	"google.golang.org/protobuf/proto"
	"math"
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
func TestPublisherADMDoesNotWaitForInternalBURL(t *testing.T) {
	ctx := context.Background()
	dsp, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	id := uuid.NewString()
	set := "callbacks-test:" + id
	impSet := set + ":imp"
	t.Cleanup(func() {
		dsp.Del(ctx, id)
		for _, s := range []string{set, impSet} {
			ids := stats.SMembers(ctx, s).Val()
			for _, id := range ids {
				stats.Del(ctx, id)
			}
			stats.Del(ctx, s)
		}
	})
	record := callback.Record{Format: "POP", Destination: "https://advertiser.test/landing?click_id=original&x=%2F", NURL: "https://dsp.test/win?keep=%2F", BURL: "https://dsp.test/win?keep=%2F", SSP: "publisher", PublisherID: "publisher", WinnerType: "dsp"}
	if e := callback.Save(ctx, dsp, id, record, time.Hour, false); e != nil {
		t.Fatal(e)
	}
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	client := &http.Client{Transport: callbackTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "dsp.test" {
			t.Errorf("self HTTP or wrong URL: %s", r.URL)
		}
		calls.Add(1)
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
		return nil, errors.New("DSP offline")
	})}
	box, err := outbox.Open(filepath.Join(t.TempDir(), "publisher.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	executor := newPublisherDSPExecutor(ctx, client, 2, 16)
	defer executor.Close()
	queue := newPublisherBURLQueue(ctx, box, dsp, nil, 2, 100000, 20*time.Second, 24*time.Hour, func(c context.Context, res resolvedCallback) error {
		defer close(finished)
		return processBurl(c, res, client, []*redis.Client{stats}, impSet, nil, "", nil, box, nil, executor)
	})
	defer queue.Close()
	task := publisherBurlFunc(queue.enqueue)
	reqCtx, cancel := context.WithCancel(ctx)
	req := httptest.NewRequest("GET", "/adm?id="+id+"&url=https://attacker.test&f=bogus", nil).WithContext(reqCtx)
	req = req.WithContext(context.WithValue(req.Context(), httpin.Input, &admRequest{GlobalId: id}))
	req = req.WithContext(context.WithValue(req.Context(), publisherBurlKey{}, task))
	w := httptest.NewRecorder()
	returned := make(chan struct{})
	go func() { getAdm(ctx, w, req, []*redis.Client{stats}, dsp, set, nil, "", nil, nil, nil); close(returned) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("ADM waited for downstream BURL")
	}
	cancel()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("no internal BURL")
	}
	if w.Code != 200 || !strings.Contains(w.Body.String(), "advertiser.test") || strings.Contains(w.Body.String(), "attacker.test") {
		t.Fatal(w.Code, w.Body.String())
	}
	if stats.SCard(ctx, set).Val() != 1 {
		t.Fatal("not one click")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("BURL did not finish")
	}
	resolved, e := resolveCallback(ctx, id, dsp, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = processBurl(ctx, resolved, client, []*redis.Client{stats}, impSet, nil, "", nil, nil, nil, executor); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 1 || stats.SCard(ctx, impSet).Val() != 1 {
		t.Fatal("duplicate internal/external BURL", calls.Load())
	}
	if dsp.HExists(ctx, id, "confirm").Val() {
		t.Fatal("unexpected confirm")
	}
	t.Run("realClickHouseRevenue", func(t *testing.T) { assertADMClickIncome(t, stats, set, impSet, id) })
}
func TestCallbackRestoresADVAndUsesExistingBillingIntent(t *testing.T) {
	ctx := context.Background()
	dsp, adv, runtime, stats := callbackRedis(t, 10), callbackRedis(t, 11), callbackRedis(t, 9), callbackRedis(t, 12)
	id, user, campaign := uuid.NewString(), uuid.NewString(), uuid.NewString()
	set := "adv-burl:" + id
	adv.HSet(ctx, id, "price", ".25", "user_id", user, "campaign_id", campaign, "format", "POP", "type_model", 2, "click_id_param", "cid")
	adv.Expire(ctx, id, 2*time.Hour)
	record := callback.Record{Format: "POP", Destination: "https://advertiser.test/landing?keep=1", SSP: "ssp", WinnerType: "adv"}
	if e := callback.Save(ctx, adv, id, record, time.Hour, true); e != nil {
		t.Fatal(e)
	}
	box, e := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	store := billing.NewStore(runtime, adv, time.Hour)
	resolved, e := resolveCallback(ctx, id, dsp, store)
	if e != nil {
		t.Fatal(e)
	}
	if resolved.Winner.TypeModel != 2 || resolved.Record.Destination != record.Destination {
		t.Fatal(resolved)
	}
	client := &http.Client{Transport: callbackTransport(func(r *http.Request) (*http.Response, error) {
		t.Error("local ADV has no downstream URL")
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	for i := 0; i < 2; i++ {
		if e = processBurl(ctx, resolved, client, []*redis.Client{stats}, set, nil, "", store, box, nil); e != nil {
			t.Fatal(e)
		}
	}
	if value := runtime.Get(ctx, "spent:user:"+user).Val(); value != "0.25" {
		t.Fatal("billing not idempotent", value)
	}
	// A new Store represents another adapter / a restarted process; no local cache.
	// A separate DSP outage does not erase a valid ADV winner.
	dsp.Close()
	if _, e = resolveCallback(ctx, id, dsp, store); e != nil {
		t.Fatal("DSP outage broke ADV callback", e)
	}
	restarted := billing.NewStore(runtime, adv, time.Hour)
	if _, e = resolveCallback(ctx, id, dsp, restarted); e != nil {
		t.Fatal(e)
	}
	dsp = callbackRedis(t, 10)
	adv.PExpire(ctx, id, time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	if _, e = resolveCallback(ctx, id, dsp, restarted); !errors.Is(e, redis.Nil) {
		t.Fatal("expired UUID accepted", e)
	}
	t.Cleanup(func() {
		adv.Del(ctx, id)
		for _, key := range []string{"spent:user:" + user, "spent:campaign:" + campaign, "outbox:applied:" + billing.CallbackEventID("burl", "POP", id)} {
			runtime.Del(ctx, key)
		}
		for _, event := range stats.SMembers(ctx, set).Val() {
			stats.Del(ctx, event)
		}
		stats.Del(ctx, set)
	})
}

func TestUUIDOnlyHTTPRoutesAllFormatsAndStoredAttribution(t *testing.T) {
	ctx := context.Background()
	dsp, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	router := chi.NewRouter()
	set := "routes:" + uuid.NewString()
	imps := set + ":impressions"
	stop := InitHttpsRoutes(ctx, router, []*redis.Client{stats}, dsp, set, time.Second, time.Second, nil, "", dsp, imps, []*redis.Client{stats}, set+":conversions", []*redis.Client{stats}, set+":wins", []*redis.Client{stats}, nil, nil, nil)
	defer stop()
	for _, format := range []string{"POP", "BAN", "NAT", "IPP", "VID"} {
		t.Run(format, func(t *testing.T) {
			id := uuid.NewString()
			record := callback.Record{Format: format, Destination: "https://advertiser.test/landing?keep=%2F", WinnerType: "dsp", SSP: "ordinary-ssp"}
			if e := callback.Save(ctx, dsp, id, record, time.Hour, false); e != nil {
				t.Fatal(e)
			}
			defer dsp.Del(ctx, id)
			for _, path := range []string{"adm", "nurl", "burl"} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest("GET", "/"+path+"?id="+id+"&url=http://127.0.0.1/private&f=999&s=adl_pb.com", nil))
				want := 204
				if path == "adm" {
					want = 200
					if !strings.Contains(w.Body.String(), "advertiser.test") || strings.Contains(w.Body.String(), "127.0.0.1") {
						t.Fatal(w.Body.String())
					}
				}
				if w.Code != want {
					t.Fatal(path, w.Code, w.Body.String())
				}
			}
			// Exactly one BURL impression; forged s= must not make NURL an impression.
			for _, key := range stats.SMembers(ctx, imps).Val() {
				if stats.HGet(ctx, key, constants.ORTB_UUID).Val() == id && stats.HGet(ctx, key, constants.FORMAT_COLUMN).Val() != format {
					t.Fatal("query format overrode Redis")
				}
			}
		})
	}
	if stats.SCard(ctx, set).Val() != 5 || stats.SCard(ctx, imps).Val() != 5 {
		t.Fatal("unexpected callback counts", stats.SCard(ctx, set).Val(), stats.SCard(ctx, imps).Val())
	}
	for _, path := range []string{"adm", "nurl", "burl"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/"+path+"?id="+uuid.NewString(), nil))
		if w.Code != 410 {
			t.Fatal(path, w.Code)
		}
	}
	for _, s := range []string{set, imps} {
		for _, id := range stats.SMembers(ctx, s).Val() {
			stats.Del(ctx, id)
		}
		stats.Del(ctx, s)
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

func TestCallbackUsesExistingDSPBackupOnlyOnFailure(t *testing.T) {
	ctx := context.Background()
	primary, backup := callbackRedis(t, 10), callbackRedis(t, 11)
	id := uuid.NewString()
	r := callback.Record{Format: "POP", WinnerType: "dsp", Destination: "https://advertiser.test/unchanged"}
	if e := callback.Save(ctx, backup, id, r, time.Hour, false); e != nil {
		t.Fatal(e)
	}
	defer backup.Del(ctx, id)
	// Evicted DB1 context can still be served from the trusted existing DB0 copy.
	got, e := resolveCallback(ctx, id, primary, nil, backup)
	if e != nil || got.Record != r {
		t.Fatal(got, e)
	}
	primary.Close()
	// Connection outage is different from a known missing UUID, but the replica
	// copy can still authorize the same UUID without trusting client parameters.
	got, e = resolveCallback(ctx, id, primary, nil, backup)
	if e != nil || got.Record != r {
		t.Fatal(got, e)
	}
	backup.Del(ctx, id)
	if _, e = resolveCallback(ctx, id, primary, nil, backup); e == nil || errors.Is(e, redis.Nil) {
		t.Fatal("outage misclassified as a known miss", e)
	}
}
