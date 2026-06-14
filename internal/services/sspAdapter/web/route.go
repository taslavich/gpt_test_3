package sppAdapterWeb

import (
	"context"
	"net/http"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/geoBadIp"
	orchestratorProto "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"

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

	PutWorkStatusAllUrl           = "/work_status/all"
	PutWorkStatusPopAdultUrl      = "/work_status/pop_adl"
	PutWorkStatusPopMainstreamUrl = "/work_status/pop_mc"
	PutWorkStatusIppAdultUrl      = "/work_status/ipp_adl"
	PutWorkStatusIppMainstreamUrl = "/work_status/ipp_mc"
	PutWorkStatusBanAdultUrl      = "/work_status/ban_adl"
	PutWorkStatusBanMainstreamUrl = "/work_status/ban_mc"
	PutWorkStatusNatAdultUrl      = "/work_status/nat_adl"
	PutWorkStatusNatMainstreamUrl = "/work_status/nat_mc"
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
	Format   string `in:"query=f" required:"true"`
}

type putWorkStatusRequest struct {
	Work bool `in:"query=work" required:"true"`
}

type getWorkStatusResponse struct {
	PopAdult      bool `json:"popAdult"`
	PopMainstream bool `json:"popMainstream"`
	IppAdult      bool `json:"ippAdult"`
	IppMainstream bool `json:"ippMainstream"`
	BanAdult      bool `json:"banAdult"`
	BanMainstream bool `json:"banMainstream"`
	NatAdult      bool `json:"natAdult"`
	NatMainstream bool `json:"natMainstream"`
}

func InitHttpRoutes(
	ctx context.Context,
	httpRouter *chi.Mux,
	redisClients []*redis.Client,
	isBadIp func(ipStr string) (bool, error),
	getCountryISO func(ipStr string) (string, uint32, error),
	orchestratorClient orchestratorProto.OrchestratorServiceClient,
	bidRequestTimeout time.Duration,
	sspFeedsPopAdl map[string]string, // ADULT + POP
	sspFeedsPopMc map[string]string, // MAINSTREAM + POP
	sspFeedsIppAdl map[string]string, // ADULT + POP
	sspFeedsIppMc map[string]string, // MAINSTREAM + POP
	sspFeedsBanAdl map[string]string, // ADULT + BAN
	sspFeedsBanMc map[string]string, // MAINSTREAM + BAN
	sspFeedsNatAdl map[string]string, // ADULT + NAT
	sspFeedsNatMc map[string]string, // MAINSTREAM + NAT
	workStatus *WorkStatus,
	siteIdsAndDomains *utils.SiteIdsAndDomains,
	geoToLang geoBadIp.GeoToLang,
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
	sspAdapterWorkStatusURL string,
) {
	var counter uint64 = 0
	integration.UseGochiURLParam("path", chi.URLParam)

	httpRouter.With(
		httpin.NewInput(postBidRequest_V2_5{}),
	).Post(PostBid_POP_ADL_V_2_5_URL, func(w http.ResponseWriter, r *http.Request) {
		postBid_V2_5(ctx, w, r, redisClients, isBadIp, getCountryISO, orchestratorClient, bidRequestTimeout, sspFeedsPopAdl, &counter, ADULT, constants.POP, siteIdsAndDomains, geoToLang, redisWriteErrorMonitor, sspAdapterWorkStatusURL)
	})

	httpRouter.With(
		httpin.NewInput(postBidRequest_V2_5{}),
	).Post(PostBid_POP_MC_V_2_5_URL, func(w http.ResponseWriter, r *http.Request) {
		postBid_V2_5(ctx, w, r, redisClients, isBadIp, getCountryISO, orchestratorClient, bidRequestTimeout, sspFeedsPopMc, &counter, MAINSTREAM, constants.POP, siteIdsAndDomains, geoToLang, redisWriteErrorMonitor, sspAdapterWorkStatusURL)
	})

	httpRouter.With(
		httpin.NewInput(putWorkStatusRequest{}),
	).Put(PutWorkStatusAllUrl, func(w http.ResponseWriter, r *http.Request) {
		putAllWorkStatus(w, r, workStatus)
	})
	httpRouter.With(
		httpin.NewInput(putWorkStatusRequest{}),
	).Put(PutWorkStatusPopAdultUrl, func(w http.ResponseWriter, r *http.Request) {
		putWorkStatus(w, r, workStatus, PostBid_POP_ADL_V_2_5_URL)
	})
	httpRouter.With(
		httpin.NewInput(putWorkStatusRequest{}),
	).Put(PutWorkStatusPopMainstreamUrl, func(w http.ResponseWriter, r *http.Request) {
		putWorkStatus(w, r, workStatus, PostBid_POP_MC_V_2_5_URL)
	})
	httpRouter.With(
		httpin.NewInput(putWorkStatusRequest{}),
	).Put(PutWorkStatusIppAdultUrl, func(w http.ResponseWriter, r *http.Request) {
		putWorkStatus(w, r, workStatus, PostBid_IPP_ADL_V_2_5_URL)
	})
	httpRouter.With(
		httpin.NewInput(putWorkStatusRequest{}),
	).Put(PutWorkStatusIppMainstreamUrl, func(w http.ResponseWriter, r *http.Request) {
		putWorkStatus(w, r, workStatus, PostBid_IPP_MC_V_2_5_URL)
	})
	httpRouter.With(
		httpin.NewInput(putWorkStatusRequest{}),
	).Put(PutWorkStatusBanAdultUrl, func(w http.ResponseWriter, r *http.Request) {
		putWorkStatus(w, r, workStatus, PostBid_BAN_ADL_V_2_5_URL)
	})
	httpRouter.With(
		httpin.NewInput(putWorkStatusRequest{}),
	).Put(PutWorkStatusBanMainstreamUrl, func(w http.ResponseWriter, r *http.Request) {
		putWorkStatus(w, r, workStatus, PostBid_BAN_MC_V_2_5_URL)
	})
	httpRouter.With(
		httpin.NewInput(putWorkStatusRequest{}),
	).Put(PutWorkStatusNatAdultUrl, func(w http.ResponseWriter, r *http.Request) {
		putWorkStatus(w, r, workStatus, PostBid_NAT_ADL_V_2_5_URL)
	})
	httpRouter.With(
		httpin.NewInput(putWorkStatusRequest{}),
	).Put(PutWorkStatusNatMainstreamUrl, func(w http.ResponseWriter, r *http.Request) {
		putWorkStatus(w, r, workStatus, PostBid_NAT_MC_V_2_5_URL)
	})

	httpRouter.With(
		httpin.NewInput(getWorkStatusResponse{}),
	).Get(GetWorkStatusUrl, func(w http.ResponseWriter, r *http.Request) {
		getWorkStatus(w, workStatus)
	})

	//---------------------------------------------------------------

	httpRouter.With(
		httpin.NewInput(postBidRequest_V2_5{}),
	).Post(PostBid_IPP_ADL_V_2_5_URL, func(w http.ResponseWriter, r *http.Request) {
		postBid_V2_5(ctx, w, r, redisClients, isBadIp, getCountryISO, orchestratorClient, bidRequestTimeout, sspFeedsIppAdl, &counter, ADULT, constants.IPP, siteIdsAndDomains, geoToLang, redisWriteErrorMonitor, sspAdapterWorkStatusURL)
	})

	httpRouter.With(
		httpin.NewInput(postBidRequest_V2_5{}),
	).Post(PostBid_IPP_MC_V_2_5_URL, func(w http.ResponseWriter, r *http.Request) {
		postBid_V2_5(ctx, w, r, redisClients, isBadIp, getCountryISO, orchestratorClient, bidRequestTimeout, sspFeedsIppMc, &counter, MAINSTREAM, constants.IPP, siteIdsAndDomains, geoToLang, redisWriteErrorMonitor, sspAdapterWorkStatusURL)
	})

	//---------------------------------------------------------------

	httpRouter.With(
		httpin.NewInput(postBidRequest_V2_5{}),
	).Post(PostBid_BAN_ADL_V_2_5_URL, func(w http.ResponseWriter, r *http.Request) {
		postBid_V2_5(ctx, w, r, redisClients, isBadIp, getCountryISO, orchestratorClient, bidRequestTimeout, sspFeedsBanAdl, &counter, ADULT, constants.BAN, siteIdsAndDomains, geoToLang, redisWriteErrorMonitor, sspAdapterWorkStatusURL)
	})

	httpRouter.With(
		httpin.NewInput(postBidRequest_V2_5{}),
	).Post(PostBid_BAN_MC_V_2_5_URL, func(w http.ResponseWriter, r *http.Request) {
		postBid_V2_5(ctx, w, r, redisClients, isBadIp, getCountryISO, orchestratorClient, bidRequestTimeout, sspFeedsBanMc, &counter, MAINSTREAM, constants.BAN, siteIdsAndDomains, geoToLang, redisWriteErrorMonitor, sspAdapterWorkStatusURL)
	})

	//---------------------------------------------------------------

	httpRouter.With(
		httpin.NewInput(postBidRequest_V2_5{}),
	).Post(PostBid_NAT_ADL_V_2_5_URL, func(w http.ResponseWriter, r *http.Request) {
		postBid_V2_5(ctx, w, r, redisClients, isBadIp, getCountryISO, orchestratorClient, bidRequestTimeout, sspFeedsNatAdl, &counter, ADULT, constants.NAT, siteIdsAndDomains, geoToLang, redisWriteErrorMonitor, sspAdapterWorkStatusURL)
	})

	httpRouter.With(
		httpin.NewInput(postBidRequest_V2_5{}),
	).Post(PostBid_NAT_MC_V_2_5_URL, func(w http.ResponseWriter, r *http.Request) {
		postBid_V2_5(ctx, w, r, redisClients, isBadIp, getCountryISO, orchestratorClient, bidRequestTimeout, sspFeedsNatMc, &counter, MAINSTREAM, constants.NAT, siteIdsAndDomains, geoToLang, redisWriteErrorMonitor, sspAdapterWorkStatusURL)
	})
}

func InitHttpsRoutes(
	ctx context.Context,
	httpRouter *chi.Mux,
	redisClientsImp []*redis.Client,
	redisClientsClicks []*redis.Client,
	redisAdmClient *redis.Client,
	redisNurlClient *redis.Client,
	redisSetImpressions string,
	redisSetClicks string,
	admTimeout,
	nurlTimeout time.Duration,
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
	sspAdapterWorkStatusURL string,
) {
	integration.UseGochiURLParam("path", chi.URLParam)

	httpRouter.With(
		httpin.NewInput(admNurlRequest{}),
	).Get(GetAdmUrl, func(w http.ResponseWriter, r *http.Request) {
		getAdm(ctx, w, r, redisClientsClicks, redisAdmClient, redisSetClicks, redisWriteErrorMonitor, sspAdapterWorkStatusURL)
	})

	httpRouter.With(
		httpin.NewInput(admNurlRequest{}),
	).Get(GetNurlUrl, func(w http.ResponseWriter, r *http.Request) {
		getNurl(ctx, w, r, redisClientsImp, redisNurlClient, redisSetImpressions, redisWriteErrorMonitor, sspAdapterWorkStatusURL)
	})
}
