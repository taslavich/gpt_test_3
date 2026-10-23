package pub

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
	be "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/service"
	beweb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/web"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/billing"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
	adapter "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	stats "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
)

// Real Publisher request/delivery, real auction service, real ADM HTTP route,
// real Redis and durable queue. Only the SSP/orchestrator gRPC transport is
// bridged in-process; its existing signed UUID claim is executed verbatim.
func TestLegacyPublisherPOPDeliveryToInternalBURL(t *testing.T) {
	for _, kind := range []string{"dsp", "adv"} {
		t.Run(kind, func(t *testing.T) {
			s, code := fixture(t)
			ctx := context.Background()
			addr := s.ORTB[0].Options().Addr
			client := func(db int) *redis.Client {
				c := redis.NewClient(&redis.Options{Addr: addr, DB: db})
				t.Cleanup(func() { c.Close() })
				return c
			}
			admRedis, burlRedis, events, advRedis, runtime := client(10), client(11), client(12), client(8), client(9)
			var notifications atomic.Int32
			downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { notifications.Add(1); w.WriteHeader(204) }))
			defer downstream.Close()
			nurl := downstream.URL + "/dsp-nurl?keep=one%2Ftwo"
			root := uuid.NewString()
			clicks, imps := "flow-clicks:"+root, "flow-imps:"+root
			box, e := outbox.Open(filepath.Join(t.TempDir(), "queue.db"))
			if e != nil {
				t.Fatal(e)
			}
			defer box.Close()
			store := billing.NewStore(runtime, advRedis, time.Hour)
			router := chi.NewRouter()
			stop := adapter.InitHttpsRoutes(ctx, router, []*redis.Client{events}, admRedis, clicks, time.Second, time.Second, nil, "", burlRedis, imps, []*redis.Client{events}, "flow-conv", []*redis.Client{events}, "flow-wins", []*redis.Client{events}, store, box, nil)
			defer stop()
			engine := beweb.NewServer(.1, s.ORTB, admRedis, burlRedis, time.Hour, "ortb:ready", func(ctx context.Context, r *pb.BidEngineRequest_V2_5, p float32, ids map[string]string, routes *types.FormatPercentRoutesV25, logged bool, typic, host string) (*ortb.BidResponse, stats.UuidImpBidResponse, []string, []string) {
				return be.GetWinnerBidInternalWithRoutes_V_2_5(ctx, r, p, ids, routes, logged, typic, host)
			}, nil, "adm.example.test", nil)
			destination := "https://advertiser.test/original?keep=one&x=two"
			var auctionID, originalADM string
			s.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
				var input ortb.BidRequest
				if e := json.NewDecoder(r.Body).Decode(&input); e != nil {
					return nil, e
				}
				auctionID = r.Header.Get(pubuuid.Header)
				if e := pubuuid.Validate(s.Config.InternalSecret, auctionID, r.URL.Query().Get("feed"), r.Header.Get(pubuuid.TimeHeader), r.Header.Get(pubuuid.SignatureHeader), &input, time.Now()); e != nil {
					return nil, e
				}
				if e := pubuuid.Claim(ctx, s.ORTB, auctionID, r.Header.Get(pubuuid.SignatureHeader)); e != nil {
					return nil, e
				}
				imp := input.GetImp()[0].GetId()
				price := float32(1.5)
				source := &ortb.Bid{Impid: &imp, Adm: &destination, Price: &price}
				req := &pb.BidEngineRequest_V2_5{BidRequest: &input, ImpIdUuid: map[string]string{imp: auctionID}, Format: "POP", SspDomain: "mapped-ssp-domain.test", Logged: true, Typic: "MAINSTREAM", IsPublisher: true}
				if kind == "adv" {
					advRedis.HSet(ctx, auctionID, "format", "POP", "price", "0.25", "user_id", code.UserID, "campaign_id", root, "type_model", 1, "click_id_param", "cid")
					advRedis.Expire(ctx, auctionID, time.Hour)
					req.ReadyBidResponse = &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{source}}}}
					req.WinnerUserIds = map[string]string{imp: code.UserID}
					req.WinnerBasePrices = map[string]float64{imp: 2}
				} else {
					source.Nurl = &nurl
					req.BidResponses = map[string]*ortb.BidResponse{"dsp.test": {Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{source}}}}}
				}
				response, e := engine.GetWinnerBid_V2_5(ctx, req)
				if e != nil {
					return nil, e
				}
				if response.GetCode() != 200 {
					t.Fatalf("auction failed: %v", response)
				}
				originalADM = response.GetBidResponse().GetSeatbid()[0].GetBid()[0].GetAdm()
				b, _ := json.Marshal(response.GetBidResponse())
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
			})
			t.Cleanup(func() {
				for _, c := range []*redis.Client{admRedis, burlRedis, advRedis} {
					c.Del(ctx, auctionID)
				}
				for _, set := range []string{clicks, imps} {
					for _, id := range events.SMembers(ctx, set).Val() {
						events.Del(ctx, id)
					}
					events.Del(ctx, set)
				}
				runtime.Del(ctx, "spent:user:"+code.UserID, "spent:campaign:"+root, "outbox:applied:"+billing.CallbackEventID("burl", "POP", auctionID))
			})
			result := send(s, input(code))
			if result.Code != 200 {
				t.Fatal(result.Code, result.Body.String())
			}
			var fill Result
			if e = json.Unmarshal(result.Body.Bytes(), &fill); e != nil || fill.Status != "fill" {
				t.Fatal(fill, e)
			}
			u, e := url.Parse(originalADM)
			if e != nil || u.Query().Get("p") != "1" || u.Query().Get("f") != "0" || u.Query().Get("url") != destination {
				t.Fatal("legacy Publisher ADM", originalADM, e)
			}
			token := strings.TrimPrefix(fill.OpenURL, "/delivery/")
			if strings.Contains(token, "://") {
				v, _ := url.Parse(fill.OpenURL)
				token = strings.TrimPrefix(v.Path, "/delivery/")
			}
			delivery, e := s.loadDelivery(ctx, token)
			if e != nil || delivery.Destination != originalADM {
				t.Fatal("delivery rewrote ADM", delivery, e)
			}
			// Publisher NURL is carried in ADM, never added to the UUID hash.
			for _, r := range []*redis.Client{admRedis, burlRedis} {
				fields := r.HGetAll(ctx, auctionID).Val()
				for _, key := range []string{"pub_user_id", "ad_code_id", "pub_winner_type", "pub_burl_nurl", "pub_dsp_nurl"} {
					if _, ok := fields[key]; ok {
						t.Fatal("duplicated context", key)
					}
				}
			}
			if kind == "dsp" && u.Query().Get("n") != nurl {
				t.Fatal("DSP NURL missing from ADM")
			}
			if kind == "adv" && admRedis.Exists(ctx, auctionID).Val() != 0 {
				t.Fatal("extra ADV ADM context")
			}
			// A consumed ORTB hash must not be required when the browser opens the ad.
			s.ORTB[0].Del(ctx, auctionID)
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest("GET", originalADM, nil))
				if w.Code != 200 || !strings.Contains(w.Body.String(), "advertiser.test/original") {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			guard := admRedis
			if kind == "adv" {
				guard = advRedis
			}
			deadline := time.Now().Add(5 * time.Second)
			for guard.HGet(ctx, auctionID, "pub_burl_done").Val() != "1" && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			if guard.HGet(ctx, auctionID, "pub_burl_done").Val() != "1" || events.SCard(ctx, imps).Val() != 1 || events.SCard(ctx, clicks).Val() != 2 {
				t.Fatal("internal BURL missing or duplicate", events.SCard(ctx, imps).Val())
			}
			if kind == "dsp" {
				until := time.Now().Add(time.Second)
				for notifications.Load() == 0 && time.Now().Before(until) {
					time.Sleep(time.Millisecond)
				}
				if notifications.Load() != 1 {
					t.Fatal("DSP notification count", notifications.Load())
				}
			}
			if kind == "adv" && runtime.Get(ctx, "spent:user:"+code.UserID).Val() != "0.25" {
				t.Fatal("ADV BURL spend", runtime.Get(ctx, "spent:user:"+code.UserID).Val())
			}
		})
	}
}
