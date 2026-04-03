package sppAdapterWeb

import (
	"context"
	"net/http"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/geoBadIp"
	orchestratorProto "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"

	"github.com/ggicci/httpin"
	"github.com/ggicci/httpin/integration"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"github.com/unrolled/render"
)

const (
	ADULT      = "ADULT"
	MAINSTREAM = "MAINSTREAM"
)

var rnr = render.New(render.Options{
	StreamingJSON: true,
	UnEscapeHTML:  true,
})

const (
	PostBid_POP_ADL_V_2_5_URL = "/bid_v_2_5"
	PostBid_POP_MC_V_2_5_URL  = "/bid_v_2_5_mc"

	PostBid_BAN_ADL_V_2_5_URL = "/bid_v_2_5_ban_adl"
	PostBid_BAN_MC_V_2_5_URL  = "/bid_v_2_5_ban_mc"

	PostBid_NAT_ADL_V_2_5_URL = "/bid_v_2_5_nat_adl"
	PostBid_NAT_MC_V_2_5_URL  = "/bid_v_2_5_nat_mc"

	PostBid_IPP_ADL_V_2_5_URL = "/bid_v_2_5_ipp_adl"
	PostBid_IPP_MC_V_2_5_URL  = "/bid_v_2_5_ipp_mc"

	GetAdmUrl  = "/adm"
	GetNurlUrl = "/nurl"

	GetWorkStatusUrl = "/work_status"
	PutWorkStatusUrl = "/work_status"
)

type postBidRequest_V2_5 struct {
	Feed    string `in:"query=feed" required:"true"`
	Payload *struct {
		*ortb_V2_5.BidRequest
	} `in:"body=json"`
}

type postBidResponse_V2_5 struct {
	*ortb_V2_5.BidResponse
}

type admNurlRequest struct {
	GlobalId string `in:"query=id" required:"true"`
	DspURL   string `in:"query=url" required:"true"`
}

type putWorkStatusRequest struct {
	Work  bool   `in:"query=work" required:"true"`
	Typic string `in:"query=typic" required:"true"`
}

type getWorkStatusResponse struct {
	WorkAdl bool `json:"workAdl"`
	WorkMc  bool `json:"workMc"`
}

func InitHttpRoutes(
	ctx context.Context,
	httpRouter *chi.Mux,
	redisClient *redis.Client,
	isBadIp func(ipStr string) (bool, error),
	getCountryISO func(ipStr string) (string, uint32, error),
	orchestratorClient orchestratorProto.OrchestratorServiceClient,
	bidRequestTimeout time.Duration,
	sspFeeds map[string]string,
	sspMainstreamFeeds map[string]string,
	workAdl,
	workMc *bool,
	siteIdsAndDomains *utils.SiteIdsAndDomains,
	geoToLang geoBadIp.GeoToLang,
) {
	var counter uint64 = 0
	integration.UseGochiURLParam("path", chi.URLParam)

	httpRouter.With(
		httpin.NewInput(postBidRequest_V2_5{}),
	).Post(PostBid_POP_ADL_V_2_5_URL, func(w http.ResponseWriter, r *http.Request) {
		postBid_V2_5(ctx, w, r, redisClient, isBadIp, getCountryISO, orchestratorClient, bidRequestTimeout, sspFeeds, sspMainstreamFeeds, &counter, ADULT, siteIdsAndDomains, geoToLang)
	})

	httpRouter.With(
		httpin.NewInput(postBidRequest_V2_5{}),
	).Post(PostBid_POP_MC_V_2_5_URL, func(w http.ResponseWriter, r *http.Request) {
		postBid_V2_5(ctx, w, r, redisClient, isBadIp, getCountryISO, orchestratorClient, bidRequestTimeout, sspFeeds, sspMainstreamFeeds, &counter, MAINSTREAM, siteIdsAndDomains, geoToLang)
	})

	httpRouter.With(
		httpin.NewInput(putWorkStatusRequest{}),
	).Put(PutWorkStatusUrl, func(w http.ResponseWriter, r *http.Request) {
		putWorkStatus(w, r, workAdl, workMc)
	})

	httpRouter.With(
		httpin.NewInput(getWorkStatusResponse{}),
	).Get(GetWorkStatusUrl, func(w http.ResponseWriter, r *http.Request) {
		getWorkStatus(w, workAdl, workMc)
	})
}

func InitHttpsRoutes(
	ctx context.Context,
	httpRouter *chi.Mux,
	redisClient *redis.Client,
	admTimeout,
	nurlTimeout time.Duration,
) {
	integration.UseGochiURLParam("path", chi.URLParam)

	httpRouter.With(
		httpin.NewInput(admNurlRequest{}),
	).Get(GetAdmUrl, func(w http.ResponseWriter, r *http.Request) {
		getAdm(ctx, w, r, redisClient)
	})

	httpRouter.With(
		httpin.NewInput(admNurlRequest{}),
	).Get(GetNurlUrl, func(w http.ResponseWriter, r *http.Request) {
		getNurl(w, r)
	})
}
