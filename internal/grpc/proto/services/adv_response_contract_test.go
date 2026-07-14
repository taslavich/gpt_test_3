package services_test

import (
	"testing"

	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestADVResponseContractDoesNotExposeSelectedOrCode(t *testing.T) {
	fields := (&advGrpc.DoAuctionResponse{}).ProtoReflect().Descriptor().Fields()
	for _, name := range []protoreflect.Name{"selected", "code"} {
		if fields.ByName(name) != nil {
			t.Fatalf("deprecated ADV response field %q is still present", name)
		}
	}
	if fields.ByName("bid_response") == nil {
		t.Fatal("ADV response must expose bid_response")
	}
	if fields.ByName("winnerUserIds") == nil {
		t.Fatal("ADV response must expose winnerUserIds")
	}
}

func TestADVReadyBidResponseRoundTrip(t *testing.T) {
	responseID := "adv-response"
	input := &advGrpc.DoAuctionResponse{
		BidResponse: &ortb.BidResponse{Id: &responseID},
		WinnerUserIds: map[string]string{
			"imp-1": "user-1",
		},
	}

	encoded, err := proto.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	output := &advGrpc.DoAuctionResponse{}
	if err := proto.Unmarshal(encoded, output); err != nil {
		t.Fatal(err)
	}
	if output.GetBidResponse() == nil || output.GetBidResponse().GetId() != responseID {
		t.Fatalf("unexpected bid response after round-trip: %#v", output.GetBidResponse())
	}
	if got := output.GetWinnerUserIds()["imp-1"]; got != "user-1" {
		t.Fatalf("winner user id got %q want %q", got, "user-1")
	}
}
