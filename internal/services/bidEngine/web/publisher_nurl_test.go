package bidEngineWeb

import (
	"context"
	"net/url"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	stats "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
)

// Nil Redis clients deliberately prove that Publisher attribution/notification
// needs neither an ORTB hash/marker nor an ADM hash, even for multiple winners.
func TestPublisherADMQueryNURLWithoutRedis(t *testing.T) {
	for _, tc := range []struct {
		name, format, kind, nurl string
		publisher                bool
	}{
		{"DSP", constants.POP, "dsp", "https://dsp.test/win?x=one%2Ftwo&y=a=b&raw=%25#fragment?keep&equals=", true},
		{"DSP-no-NURL", constants.POP, "dsp", "", true},
		{"ADV", constants.POP, "adv", "https://unused.test/win", true},
		{"ordinary-POP", constants.POP, "dsp", "https://dsp.test/win", false},
		{"BAN", constants.BAN, "dsp", "https://dsp.test/win", true},
		{"NAT", constants.NAT, "dsp", "https://dsp.test/win", true},
		{"IPP", constants.IPP, "dsp", "https://dsp.test/win", true},
		{"VID", constants.VID, "dsp", "https://dsp.test/win", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &pb.BidEngineRequest_V2_5{IsPublisher: tc.publisher, Format: tc.format, ImpIdUuid: map[string]string{"one": "uuid-one", "two": "uuid-two"}}
			wins := stats.UuidImpBidResponse{}
			response := &ortb.BidResponse{}
			destination := "https://advertiser.test/landing?keep=%2F&equals=a=b#fragment"
			for _, imp := range []string{"one", "two"} {
				id := req.ImpIdUuid[imp]
				adm := utils.WrapURL("adm.test", destination, id, tc.format)
				burl := utils.WrapBurlNurlURL("adm.test", tc.nurl, id, tc.format)
				nurl := utils.WrapNurlURL("adm.test", tc.nurl, id, "ssp.test", tc.format)
				response.Seatbid = append(response.Seatbid, &ortb.SeatBid{Bid: []*ortb.Bid{{Impid: &imp, Adm: &adm, Burl: &burl, Nurl: &nurl}}})
				kind := tc.kind
				wins[id] = &stats.Bid{WinDspDomain: &kind}
			}
			before := []string{}
			for _, s := range response.Seatbid {
				b := s.Bid[0]
				before = append(before, b.GetAdm(), b.GetBurl(), b.GetNurl())
			}
			(&Server{}).markPublisherPOP(context.Background(), req, response, wins)
			for i, s := range response.Seatbid {
				b := s.Bid[0]
				u, e := url.Parse(b.GetAdm())
				if e != nil {
					t.Fatal(e)
				}
				wantPublisher := tc.publisher && tc.format == constants.POP
				if (u.Query().Get("p") == "1") != wantPublisher {
					t.Fatal("wrong p", u)
				}
				wantNURL := ""
				if wantPublisher && tc.kind == "dsp" {
					wantNURL = tc.nurl
				}
				if u.Query().Get("n") != wantNURL || (wantNURL == "" && u.Query().Has("n")) {
					t.Fatal("nested URL damaged/unexpected", u, wantNURL)
				}
				if u.Query().Get("url") != destination {
					t.Fatal("destination modified", u)
				}
				if b.GetBurl() != before[i*3+1] || b.GetNurl() != before[i*3+2] {
					t.Fatal("BURL/NURL modified")
				}
				if !wantPublisher && b.GetAdm() != before[i*3] {
					t.Fatal("ordinary/non-POP ADM modified")
				}
			}
		})
	}
}
