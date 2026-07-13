package orchestrator

import (
	"log"

	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewOrchestrator(
	addressOfBidEngine string,
	addressOfDspRouter string,
) *TOrchestrator {
	return &TOrchestrator{
		addressOfBidEngine: addressOfBidEngine,
		addressOfDspRouter: addressOfDspRouter,
	}
}

func (o *TOrchestrator) GetGrpClients() (*GrpcClients, func()) {
	bidEngineConn, err := grpc.NewClient(
		o.addressOfBidEngine,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("did not connect to bidEngine: %v", err)
	}

	dspRouterConn, err := grpc.NewClient(
		o.addressOfDspRouter,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("did not connect to dspRouter: %v", err)
	}

	return &GrpcClients{
			BidEngineGrpcClient: bidEngineGrpc.NewBidEngineServiceClient(bidEngineConn),
			DspRouterGrpcClient: dspRouterGrpc.NewDspRouterServiceClient(dspRouterConn),
		},
		func() {
			if err := bidEngineConn.Close(); err != nil {
				log.Printf("Cannot close bidEngine connection: %v", err)
			}

			if err := dspRouterConn.Close(); err != nil {
				log.Printf("Cannot close dspRouter connection: %v", err)
			}
		}
}

/*r := manual.NewBuilderWithScheme("static")

	r.InitialState(resolver.State{
		Addresses: []resolver.Address{
			{Addr: "localhost:8051"},
			{Addr: "localhost:8052"},
		},
	})

	dspRouterConn, err := grpc.Dial(
		r.Scheme()+":///ignored",
		grpc.WithResolvers(r),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{
        "loadBalancingConfig": [{"round_robin":{}}]
    }`),
	)
	if err != nil {
		log.Fatalf("did not connect to dspRouter: %v", err)
	}*/
