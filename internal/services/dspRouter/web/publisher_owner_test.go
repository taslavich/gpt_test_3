package dspRouterWeb

import (
	"context"
	"testing"
	"time"

	adv "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"google.golang.org/grpc"
)

type publisherOwnerADVClient struct {
	adv.AdvServiceClient
	owner string
}

func (c *publisherOwnerADVClient) DoAuction(_ context.Context, r *adv.DoAuctionRequest, _ ...grpc.CallOption) (*adv.DoAuctionResponse, error) {
	c.owner = r.GetPublisherUserId()
	return &adv.DoAuctionResponse{}, nil
}

func TestPublisherOwnerForwardedToADVOnlyForPublisherTraffic(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		client := &publisherOwnerADVClient{}
		s := &Server{advClient: client}
		id, imp, spoof := "req", "i", "untrusted-ortb-owner"
		request := &pb.DspRouterRequest_V2_5{IsPublisher: enabled, PublisherUserId: "authenticated-owner", Format: "POP", BidRequest: &ortb.BidRequest{Id: &id, Imp: []*ortb.Imp{{Id: &imp}}, Site: &ortb.Site{Publisher: &ortb.Publisher{Id: &spoof}}, User: &ortb.User{Id: &spoof}}}
		if _, err := s.doAdvAuction(context.Background(), request, time.Second); err != nil {
			t.Fatal(err)
		}
		want := ""
		if enabled {
			want = "authenticated-owner"
		}
		if client.owner != want {
			t.Fatal("owner/traffic isolation failed", client.owner, want)
		}
	}
}
