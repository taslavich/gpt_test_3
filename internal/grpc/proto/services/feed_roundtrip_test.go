package services_test

import (
	"testing"

	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	orchestratorGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	"google.golang.org/protobuf/proto"
)

func TestFeedFieldProtoRoundTrip(t *testing.T) {
	const feed = "a15c30da-6dea-4945-a5cf-40bb34b1047b"
	tests := []struct {
		name string
		in   proto.Message
		out  proto.Message
		get  func(proto.Message) string
	}{
		{
			name: "orchestrator",
			in:   &orchestratorGrpc.OrchestratorRequest_V2_5{Feed: feed},
			out:  &orchestratorGrpc.OrchestratorRequest_V2_5{},
			get:  func(value proto.Message) string { return value.(*orchestratorGrpc.OrchestratorRequest_V2_5).GetFeed() },
		},
		{
			name: "router",
			in:   &dspRouterGrpc.DspRouterRequest_V2_5{Feed: feed},
			out:  &dspRouterGrpc.DspRouterRequest_V2_5{},
			get:  func(value proto.Message) string { return value.(*dspRouterGrpc.DspRouterRequest_V2_5).GetFeed() },
		},
		{
			name: "adv",
			in:   &advGrpc.DoAuctionRequest{Feed: feed},
			out:  &advGrpc.DoAuctionRequest{},
			get:  func(value proto.Message) string { return value.(*advGrpc.DoAuctionRequest).GetFeed() },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := proto.Marshal(test.in)
			if err != nil {
				t.Fatal(err)
			}
			if err := proto.Unmarshal(encoded, test.out); err != nil {
				t.Fatal(err)
			}
			if got := test.get(test.out); got != feed {
				t.Fatalf("feed round-trip got %q want %q", got, feed)
			}
		})
	}
}
