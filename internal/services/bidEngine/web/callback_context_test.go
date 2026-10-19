package bidEngineWeb

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/callback"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/service"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
)

func TestUUIDOnlyCallbacksPersistContextAllFormatsAndWinnerTypes(t *testing.T) {
	t.Setenv("CALLBACK_UUID_ONLY", "true")
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
				server.redisClients[0].HSet(context.Background(), id, "pub_user_id", "publisher")
				got, e := server.GetWinnerBid_V2_5(context.Background(), req)
				if e != nil || got.GetCode() != 200 {
					t.Fatal(got, e)
				}
				bid := got.GetBidResponse().GetSeatbid()[0].GetBid()[0]
				check := func(raw string) {
					t.Helper()
					u, e := url.Parse(html.UnescapeString(raw))
					if e != nil || u.Host != "callbacks.test" || len(u.Query()) != 1 || u.Query().Get("id") != id {
						t.Fatalf("non UUID-only callback: %s", raw)
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
				store := server.redisAdmClient
				if adv {
					store = server.callbackADV
				}
				record, e := callback.Decode(store.HGetAll(context.Background(), id).Val())
				if e != nil {
					t.Fatal(e)
				}
				if record.Format != format || record.PublisherID != "publisher" || record.SSP != "ssp.test" {
					t.Fatal(record)
				}
				if !adv && record.NURL != nurl {
					t.Fatal("downstream parameters lost", record)
				}
				if record.Destination != "" && strings.Contains(record.Destination, "callbacks.test") {
					t.Fatal("recursive destination", record)
				}
				if store.PTTL(context.Background(), id).Val() < 59*time.Minute {
					t.Fatal("short retention")
				}
				b, _ := json.Marshal(record)
				if strings.Contains(bid.GetNurl(), string(b)) {
					t.Fatal("context exposed")
				}
			})
		}
	}
}

func TestNoBidCanExposeUnstoredCallbackContext(t *testing.T) {
	for _, collect := range []bool{false, true} {
		t.Run(fmt.Sprint(collect), func(t *testing.T) {
			id, imp := uuid.NewString(), "1"
			adm := "https://callbacks.test/adm?id=" + id
			req := &pb.BidEngineRequest_V2_5{BidRequest: &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &imp}}}, ImpIdUuid: map[string]string{imp: id}, Format: "POP", Logged: true}
			s := &Server{GetWinnerBidInternalWithRoutes_V_2_5: func(ctx context.Context, _ *pb.BidEngineRequest_V2_5, _ float32, _ map[string]string, _ *types.FormatPercentRoutesV25, _ bool, _, _ string) (*ortb.BidResponse, clickhouse_types.UuidImpBidResponse, []string, []string) {
				if collect {
					callback.Collect(ctx, id, callback.Record{Format: "POP", Destination: "https://advertiser.test/", WinnerType: "dsp"})
				}
				return &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &imp, Adm: &adm}}}}}, nil, nil, nil
			}}
			got, e := s.GetWinnerBid_V2_5(context.Background(), req)
			if e != nil || got.GetCode() != 204 || got.GetBidResponse() != nil {
				t.Fatal("unstored UUID link escaped", got, e)
			}
		})
	}
}
