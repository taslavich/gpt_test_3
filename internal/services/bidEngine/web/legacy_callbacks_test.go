package bidEngineWeb

import (
	"context"
	"html"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/service"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
)

func TestLegacyCallbacksAllFormatsAndPublisherMarker(t *testing.T) {
	for _, format := range []string{"POP", "BAN", "NAT", "IPP", "VID"} {
		for _, adv := range []bool{false, true} {
			t.Run(format+map[bool]string{false: "DSP", true: "ADV"}[adv], func(t *testing.T) {
				id, imp := uuid.NewString(), "1"
				price := float32(1.5)
				adm := "https://advertiser.test/path?keep=%2F&click_id=original"
				if format == "BAN" {
					adm = `<a href="https://advertiser.test/path?keep=1&amp;x=2"><img src="https://cdn.test/a.png"></a>`
				}
				if format == "NAT" || format == "IPP" {
					adm = `{"native":{"link":{"url":"https://advertiser.test/path?keep=1"},"assets":[]}}`
				}
				if format == "VID" {
					adm = `<VAST version="3.0"><Ad><InLine><Impression>https://private.test/i</Impression><Creatives><Creative><Linear><VideoClicks><ClickThrough>https://advertiser.test/video</ClickThrough></VideoClicks><MediaFiles><MediaFile>https://cdn.test/video.mp4</MediaFile></MediaFiles></Linear></Creative></Creatives></InLine></Ad></VAST>`
				}
				nurl := "https://dsp.test/win?private=1"
				source := &ortb.Bid{Impid: &imp, Adm: &adm, Nurl: &nurl, Price: &price}
				req := &pb.BidEngineRequest_V2_5{BidRequest: &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &imp}}}, ImpIdUuid: map[string]string{imp: id}, Format: format, SspDomain: "ssp.test", Logged: true}
				if adv {
					req.ReadyBidResponse = &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{source}}}}
					req.WinnerUserIds = map[string]string{imp: "owner"}
					req.WinnerBasePrices = map[string]float64{imp: 2}
				} else {
					req.BidResponses = map[string]*ortb.BidResponse{"dsp.test": {Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{source}}}}}
				}
				server := &Server{admDomain: "callbacks.test", GetWinnerBidInternalWithRoutes_V_2_5: func(ctx context.Context, r *pb.BidEngineRequest_V2_5, p float32, m map[string]string, routes *types.FormatPercentRoutesV25, logged bool, typic, host string) (*ortb.BidResponse, clickhouse_types.UuidImpBidResponse, []string, []string) {
					return service.GetWinnerBidInternalWithRoutes_V_2_5(ctx, r, p, m, routes, logged, typic, host)
				}}
				configureCallbackServer(t, server, req)
				server.redisClients[0].HSet(context.Background(), id, "pub_user_id", "publisher", "ad_code_id", "code", "SPP_DOMAIN", "ssp.test", "FORMAT", format)
				server.redisClients[0].HSet(context.Background(), id+":pub-uuid", "state", "adapter")
				defer server.redisClients[0].Del(context.Background(), id+":pub-uuid")
				got, e := server.GetWinnerBid_V2_5(context.Background(), req)
				if e != nil || got.GetCode() != 200 {
					t.Fatal(got, e)
				}
				bid := got.GetBidResponse().GetSeatbid()[0].GetBid()[0]
				check := func(raw string) {
					t.Helper()
					u, e := url.Parse(html.UnescapeString(raw))
					if e != nil || u.Host != "callbacks.test" || u.Query().Get("id") != id || u.Query().Get("f") != constants.FormatToCodes[format] {
						t.Fatalf("invalid legacy callback: %s", raw)
					}
				}
				if bid.GetNurl() != "" {
					check(bid.GetNurl())
				}
				check(bid.GetBurl())
				re := regexp.MustCompile(`https://callbacks\.test/(?:adm|burl|nurl)\?[^"<>\s]+`)
				for _, raw := range re.FindAllString(strings.ReplaceAll(bid.GetAdm(), `\u0026`, "&"), -1) {
					check(strings.TrimRight(raw, "]"))
				}
				if adv || format == "POP" {
					if !strings.Contains(bid.GetAdm(), "callbacks.test/adm?") {
						t.Fatal("missing ADM callback", bid.GetAdm())
					}
				}
				admURL, _ := url.Parse(bid.GetAdm())
				if format == "POP" && admURL.Query().Get("p") != "1" {
					t.Fatal("Publisher POP missing p=1", bid.GetAdm())
				}
				if format != "POP" && admURL != nil && admURL.Query().Has("p") {
					t.Fatal("Publisher flag on non-POP")
				}
				for _, raw := range []string{bid.GetNurl(), bid.GetBurl()} {
					u, _ := url.Parse(raw)
					if u.Query().Has("p") {
						t.Fatal("p on non-ADM callback")
					}
				}
				for _, store := range []*redis.Client{server.redisAdmClient, server.redisBurlClient} {
					if store.HExists(context.Background(), id, "callback_context_v1").Val() {
						t.Fatal("retired context persisted")
					}
				}
				for _, store := range []*redis.Client{server.redisAdmClient, server.redisBurlClient} {
					fields := store.HGetAll(context.Background(), id).Val()
					for _, removed := range []string{"pub_user_id", "ad_code_id", "pub_winner_type", "pub_burl_nurl"} {
						if _, ok := fields[removed]; ok {
							t.Fatal("duplicated Publisher context", removed)
						}
					}
				}
				nurlValue := server.redisAdmClient.HGet(context.Background(), id, "pub_dsp_nurl").Val()
				if !adv && format == "POP" {
					if nurlValue != nurl {
						t.Fatal("DSP notification lost", nurlValue)
					}
				} else if nurlValue != "" {
					t.Fatal("extra NURL for ADV/non-POP")
				}
				if server.redisBurlClient.HExists(context.Background(), id, "pub_dsp_nurl").Val() {
					t.Fatal("NURL stored in wrong DB")
				}

			})
		}
	}
}

func TestOrdinaryPOPAndPartialAttributionDoNotGetPublisherFlag(t *testing.T) {
	for _, kind := range []string{"ordinary", "advertiser-user", "owner-only", "missing-source", "unclaimed", "publisher"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			id, imp := uuid.NewString(), "1"
			adm := "http://127.0.0.1/landing?keep=1"
			price := float32(2)
			req := &pb.BidEngineRequest_V2_5{BidRequest: &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &imp}}}, ImpIdUuid: map[string]string{imp: id}, Format: "POP", SspDomain: "publisher-feed", Logged: true, BidResponses: map[string]*ortb.BidResponse{"dsp": {Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &imp, Adm: &adm, Price: &price}}}}}}}
			server := &Server{admDomain: "callbacks.test", GetWinnerBidInternalWithRoutes_V_2_5: func(ctx context.Context, r *pb.BidEngineRequest_V2_5, p float32, m map[string]string, routes *types.FormatPercentRoutesV25, logged bool, typic, host string) (*ortb.BidResponse, clickhouse_types.UuidImpBidResponse, []string, []string) {
				return service.GetWinnerBidInternalWithRoutes_V_2_5(ctx, r, p, m, routes, logged, typic, host)
			}}
			configureCallbackServer(t, server, req)
			source := server.redisClients[0]
			fields := map[string]any{"FORMAT": "POP", "SPP_DOMAIN": "signed-feed-token"}
			if kind == "advertiser-user" {
				fields["WIN_USER_ID"] = "advertiser"
			}
			if kind == "owner-only" {
				fields["pub_user_id"] = "owner"
			}
			if kind == "publisher" || kind == "missing-source" || kind == "unclaimed" {
				fields["pub_user_id"] = "owner"
				fields["ad_code_id"] = "code"
			}
			if kind == "missing-source" {
				fields["SPP_DOMAIN"] = ""
			}
			source.HSet(ctx, id, fields)
			if kind != "unclaimed" {
				source.HSet(ctx, id+":pub-uuid", "state", "adapter")
			}
			defer source.Del(ctx, id+":pub-uuid")
			got, e := server.GetWinnerBid_V2_5(ctx, req)
			if e != nil || got.GetCode() != 200 {
				t.Fatal(got, e)
			}
			u, e := url.Parse(got.GetBidResponse().GetSeatbid()[0].GetBid()[0].GetAdm())
			if e != nil {
				t.Fatal(e)
			}
			if (u.Query().Get("p") == "1") != (kind == "publisher") {
				t.Fatal(kind, u)
			}
			if u.Query().Get("url") != adm {
				t.Fatal("destination modified", u)
			}
		})
	}
}

func TestLegacyWinnerIsNotDroppedWhenNoCallbackContextWasCollected(t *testing.T) {
	ctx := context.Background()
	id, imp := uuid.NewString(), "1"
	adm := utils.WrapURL("callbacks.test", "http://127.0.0.1/private", id, "POP")
	req := &pb.BidEngineRequest_V2_5{BidRequest: &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &imp}}}, ImpIdUuid: map[string]string{imp: id}, Format: "POP", Logged: false}
	server := &Server{GetWinnerBidInternalWithRoutes_V_2_5: func(context.Context, *pb.BidEngineRequest_V2_5, float32, map[string]string, *types.FormatPercentRoutesV25, bool, string, string) (*ortb.BidResponse, clickhouse_types.UuidImpBidResponse, []string, []string) {
		return &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &imp, Adm: &adm}}}}}, nil, []string{id}, []string{id}
	}}
	configureCallbackServer(t, server, req)
	got, e := server.GetWinnerBid_V2_5(ctx, req)
	if e != nil || got.GetCode() != 200 || got.GetBidResponse() == nil {
		t.Fatal(got, e)
	}
	if server.redisAdmClient.HGet(ctx, id, "uuid").Val() != id {
		t.Fatal("legacy UUID authorization missing")
	}
}

func TestPublisherDSPNURLWriteFailureDoesNotDropWinningBid(t *testing.T) {
	ctx := context.Background()
	id, imp := uuid.NewString(), "1"
	destination := "http://127.0.0.1/landing"
	price := float32(2)
	nurl := "https://dsp.test/win"
	req := &pb.BidEngineRequest_V2_5{BidRequest: &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &imp}}}, ImpIdUuid: map[string]string{imp: id}, Format: "POP", Logged: true, SspDomain: "mapped-domain", BidResponses: map[string]*ortb.BidResponse{"dsp": {Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &imp, Adm: &destination, Nurl: &nurl, Price: &price}}}}}}}
	server := &Server{admDomain: "callbacks.test", GetWinnerBidInternalWithRoutes_V_2_5: func(ctx context.Context, r *pb.BidEngineRequest_V2_5, p float32, ids map[string]string, routes *types.FormatPercentRoutesV25, logged bool, typic, host string) (*ortb.BidResponse, clickhouse_types.UuidImpBidResponse, []string, []string) {
		return service.GetWinnerBidInternalWithRoutes_V_2_5(ctx, r, p, ids, routes, logged, typic, host)
	}}
	configureCallbackServer(t, server, req)
	server.redisClients[0].HSet(ctx, id, "pub_user_id", "owner", "ad_code_id", "code", "FORMAT", "POP", "SPP_DOMAIN", "signed-feed")
	server.redisClients[0].HSet(ctx, id+":pub-uuid", "state", "adapter")
	defer server.redisClients[0].Del(ctx, id+":pub-uuid")
	server.redisAdmClient.AddHook(failDSPNURLWrite{})
	got, e := server.GetWinnerBid_V2_5(ctx, req)
	if e != nil || got.GetCode() != 200 || got.GetBidResponse() == nil {
		t.Fatal("metadata failure filtered winner", got, e)
	}
	u, _ := url.Parse(got.GetBidResponse().GetSeatbid()[0].GetBid()[0].GetAdm())
	if server.redisAdmClient.HGet(ctx, id, "uuid").Val() != id || server.redisAdmClient.HExists(ctx, id, "pub_dsp_nurl").Val() {
		t.Fatal("UUID authorization lost on NURL write failure")
	}
	if u.Query().Get("p") != "1" {
		t.Fatal(u)
	}
}
