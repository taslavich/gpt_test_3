package web

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestPublisherPercentThroughRealADVGRPCAndWinnerStore(t *testing.T) {
	addr := os.Getenv("PUB_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("PUB_TEST_REDIS_ADDR unset")
	}
	r := redis.NewClient(&redis.Options{Addr: addr, DB: 14})
	defer r.Close()
	dir := t.TempDir()
	write := func(name, data string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	percents, err := auction.NewPercentStore(write("adv.json", `{"ALL":0.6,"ALL_RTB":0.7}`))
	if err != nil {
		t.Fatal(err)
	}
	quality, err := auction.NewQualityStore(write("quality.json", `{"usual":{"ssp.test":true},"high":{},"ultra":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	sites, err := auction.NewSiteIDQualityStore(write("sites.json", `{"usual":{"isWhiteList":false,"siteIds":[]},"high":{"isWhiteList":false,"siteIds":[]},"ultra":{"isWhiteList":false,"siteIds":[]}}`))
	if err != nil {
		t.Fatal(err)
	}
	owners, err := auction.NewPublisherPercentStore(filepath.Join(dir, "publisher.json"))
	if err != nil {
		t.Fatal(err)
	}
	owner := uuid.NewString()
	if err = owners.Update(auction.PublisherPercentMap{owner: .1}); err != nil {
		t.Fatal(err)
	}
	s := auction.NewAuctionService(auction.NewRuntimeStore(r, time.Minute, time.Hour), auction.NewWinnerStore(r, time.Minute), percents, quality, sites)
	s.SetAntiPerekrutEnabled(false)
	s.ConfigurePublisherPercents(owners)
	now := time.Now().UTC()
	c := &auction.Campaign{ID: uuid.NewString(), UserID: uuid.NewString(), TypeModel: auction.TypeModelMapOnly, BasePrice: 10, GoalTotalDollars: 1000, PricingModel: auction.PricingModelCPM, Format: "POP", TrafficType: auction.TrafficMainstream, QualitySegment: "usual", Status: auction.CampaignStatusActive, StartTS: now.Add(-time.Hour), EndTS: now.Add(time.Hour), Creatives: []*auction.Creative{{ID: uuid.NewString(), ADMURL: "https://advertiser.example"}}}
	if err = s.PublishSnapshot(&auction.Snapshot{Campaigns: []*auction.Campaign{c}, UserGoals: map[string]float64{c.UserID: 1000}}); err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()
	server := grpc.NewServer()
	defer server.Stop()
	pb.RegisterAdvServiceServer(server, NewServer(s, NewWorkController()))
	go server.Serve(listener)
	conn, err := grpc.NewClient("passthrough:///publisher-pricing", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewAdvServiceClient(conn)
	for _, test := range []struct {
		owner string
		price float32
	}{{owner, 9}, {"", 4}, {uuid.NewString(), 4}} {
		id, imp, winner := uuid.NewString(), "i", uuid.NewString()
		defer r.Del(context.Background(), winner)
		request := &pb.DoAuctionRequest{PublisherUserId: test.owner, Format: "POP", TrafficType: auction.TrafficMainstream, SspDomain: "ssp.test", ImpIdUuid: map[string]string{imp: winner}, BidRequest: &ortb.BidRequest{Id: &id, Imp: []*ortb.Imp{{Id: &imp}}}}
		out, err := client.DoAuction(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if out.GetBidResponse().GetSeatbid()[0].GetBid()[0].GetPrice() != test.price || out.GetWinnerBasePrices()[imp] != 10 {
			t.Fatal("gRPC owner pricing lost", out)
		}
		if price := r.HGet(context.Background(), winner, "price").Val(); price != "0.01" {
			t.Fatal("wrong advertiser billing", price)
		}
	}
}
