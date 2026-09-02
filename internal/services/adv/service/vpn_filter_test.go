package auction

import (
	"context"
	"errors"
	"testing"
	"time"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

var (
	testVPNStart = time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	testVPNEnd   = testVPNStart.Add(24 * time.Hour)
)

type countingVPNClassifier struct {
	calls int
	gotIP string
	vpn   bool
	err   error
}

func (c *countingVPNClassifier) IsVPN(ip string) (bool, error) {
	c.calls++
	c.gotIP = ip
	return c.vpn, c.err
}

func TestClassifyVPNRequestPerformsSingleLookupWhenRequired(t *testing.T) {
	ip := "5.45.38.1"
	classifier := &countingVPNClassifier{vpn: true}
	service := &AuctionService{vpnClassifier: classifier}
	req := &ortb.BidRequest{Device: &ortb.Device{Ip: &ip}}

	got, err := service.classifyVPNRequest(req, true)
	if err != nil {
		t.Fatalf("classifyVPNRequest: %v", err)
	}
	if !got {
		t.Fatal("classifyVPNRequest returned false, want true")
	}
	if classifier.calls != 1 {
		t.Fatalf("VPN lookups=%d want 1", classifier.calls)
	}
	if classifier.gotIP != ip {
		t.Fatalf("lookup IP=%q want %q", classifier.gotIP, ip)
	}
}

func TestClassifyVPNRequestSkipsLookupWithoutBlockingCampaigns(t *testing.T) {
	classifier := &countingVPNClassifier{vpn: true}
	service := &AuctionService{vpnClassifier: classifier}

	got, err := service.classifyVPNRequest(nil, false)
	if err != nil || got {
		t.Fatalf("classifyVPNRequest(nil,false)=(%v,%v), want (false,nil)", got, err)
	}
	if classifier.calls != 0 {
		t.Fatalf("VPN lookups=%d want 0", classifier.calls)
	}
}

func TestClassifyVPNRequestFailsClosedWhenClassifierUnavailable(t *testing.T) {
	service := &AuctionService{}
	_, err := service.classifyVPNRequest(nil, true)
	if err == nil {
		t.Fatal("expected classifier-unavailable error")
	}
}

func TestClassifyVPNRequestPropagatesLookupError(t *testing.T) {
	wantErr := errors.New("lookup failed")
	classifier := &countingVPNClassifier{err: wantErr}
	service := &AuctionService{vpnClassifier: classifier}
	_, err := service.classifyVPNRequest(nil, true)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error=%v want %v", err, wantErr)
	}
	if classifier.calls != 1 {
		t.Fatalf("VPN lookups=%d want 1", classifier.calls)
	}
}

func TestSnapshotTracksPresenceOfBlockVPNCampaigns(t *testing.T) {
	snapshot := &Snapshot{
		Campaigns: []*Campaign{
			validSnapshotCampaignForVPNTest("allow-all", false),
			validSnapshotCampaignForVPNTest("block-vpn", true),
		},
		UserGoals: map[string]float64{"user": 100},
	}
	cloned, err := cloneAndValidateSnapshot(snapshot)
	if err != nil {
		t.Fatalf("cloneAndValidateSnapshot: %v", err)
	}
	if !cloned.HasBlockVPNCampaigns {
		t.Fatal("snapshot must record that block_vpn campaigns exist")
	}
}

func validSnapshotCampaignForVPNTest(id string, blockVPN bool) *Campaign {
	return &Campaign{
		ID:               id,
		UserID:           "user",
		Status:           CampaignStatusActive,
		PricingModel:     PricingModelCPM,
		Format:           "BAN",
		TrafficType:      "mainstream",
		QualitySegment:   "usual",
		BasePrice:        1,
		GoalTotalDollars: 10,
		BlockVPN:         blockVPN,
		StartTS:          testVPNStart,
		EndTS:            testVPNEnd,
		Creatives: []*Creative{{
			ID:         id + "-creative",
			CampaignID: id,
			ADMURL:     "https://example.test/ad",
			W:          300,
			H:          250,
		}},
	}
}

func TestEvaluateCampaignRejectsMatchedVPNWhenBlockEnabled(t *testing.T) {
	requestID := "request"
	impID := "imp"
	req := &ortb.BidRequest{Id: &requestID}
	imp := &ortb.Imp{Id: &impID}
	campaign := validSnapshotCampaignForVPNTest("block-vpn", true)

	_, eligible, reason, err := (&AuctionService{}).evaluateCampaign(
		context.Background(), campaign, req, imp, testVPNStart.Add(time.Hour),
		"BAN", "mainstream", "ssp", "", "hash", nil, false,
		0, true, nil, true, func(string, ...any) {},
	)
	if err != nil {
		t.Fatalf("evaluateCampaign returned error: %v", err)
	}
	if eligible {
		t.Fatal("VPN request must not be eligible when block_vpn=true")
	}
	if reason != diagVPNTrafficBlocked {
		t.Fatalf("reason=%v want %v", reason, diagVPNTrafficBlocked)
	}
}
