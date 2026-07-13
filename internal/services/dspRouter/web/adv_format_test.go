package dspRouterWeb

import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestBuildADVAuctionBidRequestMarksOnlyClone(t *testing.T) {
	impID := "imp-1"
	source := &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &impID, Banner: &ortb.Banner{Ext: []string{"publisher-ext"}}}}}

	cloned, err := buildADVAuctionBidRequest(source, constants.IPP)
	if err != nil {
		t.Fatal(err)
	}
	if cloned == source || cloned.GetImp()[0] == source.GetImp()[0] {
		t.Fatal("ADV request must be a deep clone")
	}
	if len(source.GetImp()[0].GetBanner().GetExt()) != 1 {
		t.Fatal("source request was mutated")
	}
	want := constants.ADVImpressionFormatMarkerPrefix + constants.IPP
	found := false
	for _, value := range cloned.GetImp()[0].GetBanner().GetExt() {
		if value == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("ADV format marker %q was not added", want)
	}
}

func TestBuildADVAuctionBidRequestReplacesStaleMarker(t *testing.T) {
	impID := "imp-1"
	stale := constants.ADVImpressionFormatMarkerPrefix + constants.BAN
	source := &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &impID, Banner: &ortb.Banner{Ext: []string{stale}}}}}

	cloned, err := buildADVAuctionBidRequest(source, constants.IPP)
	if err != nil {
		t.Fatal(err)
	}
	values := cloned.GetImp()[0].GetBanner().GetExt()
	if len(values) != 1 || values[0] != constants.ADVImpressionFormatMarkerPrefix+constants.IPP {
		t.Fatalf("unexpected markers: %#v", values)
	}
}

func TestBuildADVAuctionBidRequestDoesNotMarkMixedImpression(t *testing.T) {
	impID := "mixed"
	source := &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &impID, Banner: &ortb.Banner{}, Native: &ortb.Native{}}}}
	cloned, err := buildADVAuctionBidRequest(source, constants.BAN)
	if err != nil {
		t.Fatal(err)
	}
	if got := cloned.GetImp()[0].GetBanner().GetExt(); len(got) != 0 {
		t.Fatalf("mixed impression must not be marked as BAN: %#v", got)
	}
}
