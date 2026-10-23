package bidEngineWeb

import (
	"context"
	"log"
	"net/url"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	stats "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
)

// Publisher is authenticated once by SSP Adapter and carried by the gRPC chain.
// Mark only finalized POP bids; no Publisher Redis lookup/storage is needed.
func (s *Server) markPublisherPOP(ctx context.Context, req *pb.BidEngineRequest_V2_5, response *ortb.BidResponse, wins stats.UuidImpBidResponse) {
	if !req.GetIsPublisher() || req.GetFormat() != constants.POP {
		return
	}
	for _, seat := range response.GetSeatbid() {
		for _, bid := range seat.GetBid() {
			if bid == nil {
				continue
			}
			id := req.GetImpIdUuid()[bid.GetImpid()]
			adm, err := url.Parse(bid.GetAdm())
			if err != nil {
				log.Printf("Publisher POP ADM uuid=%s: %v", id, err)
				continue
			}
			q := adm.Query()
			q.Set("p", "1")
			adv := false
			if win := wins[id]; win != nil && win.WinDspDomain != nil && *win.WinDspDomain == "adv" {
				adv = true
			}
			if !adv {
				if burl, err := url.Parse(bid.GetBurl()); err == nil {
					if notification := burl.Query().Get("url"); notification != "" {
						q.Set("n", notification)
					}
				}
			}
			adm.RawQuery = q.Encode()
			marked := adm.String()
			bid.Adm = &marked
		}
	}
}
