package services_test

import (
	"testing"

	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
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
	if fields.ByName("winnerBasePrices") == nil {
		t.Fatal("ADV response must expose winnerBasePrices")
	}
}

func TestADVReadyBidResponseRoundTrip(t *testing.T) {
	responseID := "adv-response"
	input := &advGrpc.DoAuctionResponse{
		BidResponse: &ortb.BidResponse{Id: &responseID},
		WinnerUserIds: map[string]string{
			"imp-1": "user-1",
		},
		WinnerBasePrices: map[string]float64{
			"imp-1": 2.5,
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
	if got := output.GetWinnerBasePrices()["imp-1"]; got != 2.5 {
		t.Fatalf("winner base price got %v want %v", got, 2.5)
	}
}

func TestWinnerBasePriceMapsRoundTripAcrossADVChain(t *testing.T) {
	tests := []struct {
		name    string
		input   proto.Message
		output  proto.Message
		readMap func(proto.Message) map[string]float64
	}{
		{
			name:   "router response",
			input:  &dspRouterGrpc.DspRouterResponse_V2_5{WinnerBasePrices: map[string]float64{"imp-1": 3.5}},
			output: &dspRouterGrpc.DspRouterResponse_V2_5{},
			readMap: func(message proto.Message) map[string]float64 {
				return message.(*dspRouterGrpc.DspRouterResponse_V2_5).GetWinnerBasePrices()
			},
		},
		{
			name:   "bid engine request",
			input:  &bidEngineGrpc.BidEngineRequest_V2_5{WinnerBasePrices: map[string]float64{"imp-1": 3.5}},
			output: &bidEngineGrpc.BidEngineRequest_V2_5{},
			readMap: func(message proto.Message) map[string]float64 {
				return message.(*bidEngineGrpc.BidEngineRequest_V2_5).GetWinnerBasePrices()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := proto.Marshal(test.input)
			if err != nil {
				t.Fatal(err)
			}
			if err := proto.Unmarshal(encoded, test.output); err != nil {
				t.Fatal(err)
			}
			if got := test.readMap(test.output)["imp-1"]; got != 3.5 {
				t.Fatalf("winner base price got %v want %v", got, 3.5)
			}
		})
	}
}
