package sppAdapterWeb

import (
	"context"
	"net/http"
	"time"

	orchestratorProto "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"

	"github.com/ggicci/httpin"
	"github.com/ggicci/httpin/integration"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"github.com/unrolled/render"
)

var rnr = render.New(render.Options{
	StreamingJSON: true,
	UnEscapeHTML:  true,
})

const (
	PostBid_V_2_5_URL = "/bid_v_2_5"

	GetNurlUrl = "/nurl"
	GetBurlUrl = "/burl"

	GetHealthUrl = "/health"
)

type postBidRequest_V2_5 struct {
	Payload *struct {
		BidRequest *ortb_V2_5.BidRequest `json:"bid_request" required:"true"`
		SppDomain  string                `json:"ssp_domain" required:"true"`
	} `in:"body=json"`
}

type postBidResponse_V2_5 struct {
	*ortb_V2_5.BidResponse
}

type nurlRequest struct {
	GlobalId string `in:"query=id" required:"true"`
	DspURL   string `in:"query=url" required:"true"`
}

type burlRequest struct {
	GlobalId string `in:"query=id" required:"true"`
	DspURL   string `in:"query=url" required:"true"`
}

func InitRoutes(
	ctx context.Context,
	httpRouter *chi.Mux,
	redisClient *redis.Client,
	isBadIp func(ipStr string) (bool, error),
	getCountryISO func(ipStr string) (string, error),
	orchestratorClient orchestratorProto.OrchestratorServiceClient,
	bidRequestTimeout,
	nurlTimeout,
	burlTimeout time.Duration,
) {
	integration.UseGochiURLParam("path", chi.URLParam)

	httpRouter.With(
		httpin.NewInput(postBidRequest_V2_5{}),
	).Post(PostBid_V_2_5_URL, func(w http.ResponseWriter, r *http.Request) {
		postBid_V2_5(ctx, w, r, redisClient, isBadIp, getCountryISO, orchestratorClient, bidRequestTimeout)
	})

	httpRouter.With(
		httpin.NewInput(nurlRequest{}),
	).Get(GetNurlUrl, func(w http.ResponseWriter, r *http.Request) {
		getNurl(ctx, w, r, redisClient, nurlTimeout)
	})

	httpRouter.With(
		httpin.NewInput(burlRequest{}),
	).Get(GetBurlUrl, func(w http.ResponseWriter, r *http.Request) {
		getBurl(ctx, w, r, redisClient, burlTimeout)
	})

	httpRouter.Get(GetHealthUrl, func(w http.ResponseWriter, r *http.Request) {
		getHealth(w)
	})
}
