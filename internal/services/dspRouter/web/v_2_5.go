package dspRouterWeb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	"github.com/redis/go-redis/v9"
	"github.com/yl2chen/cidranger"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	ruleManager *filter.RuleManager
	fileLoader  *filter.FileRuleLoader
	processor   *filter.OptimizedFilterProcessor

	// Legacy POP fields are retained for tests/backwards compatibility. New code
	// uses formatRoutes, which isolates every FORMAT x traffic-type route.
	dspEndpoints_adult_v_2_5      config.MapStringToString
	dspEndpoints_mainstream_v_2_5 config.MapStringToString
	formatRoutes                  *FormatRoutesV25

	redisClients           []*redis.Client
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor

	clients map[string]*http.Client

	timeout time.Duration

	bufferPool sync.Pool

	ranger cidranger.Ranger

	linkMap_adult      *map[string]map[string]map[string]bool
	linkMap_mainstream *map[string]map[string]map[string]bool

	filtersAdl *filter.FiltersBox
	filtersMc  *filter.FiltersBox

	filtersCidAdl *filter.FilterCidBoxType
	filtersCidMc  *filter.FilterCidBoxType

	filterBoxChangerAdl *filter.ChangersBoxChanger
	filterBoxChangerMc  *filter.ChangersBoxChanger

	configTimeouts config.MapStringToDuration

	advClient advGrpc.AdvServiceClient

	dspRouterGrpc.UnimplementedDspRouterServiceServer
}

func NewServer(
	ruleManager *filter.RuleManager,
	fileLoader *filter.FileRuleLoader,
	processor *filter.OptimizedFilterProcessor,
	formatRoutes *FormatRoutesV25,
	redisClients []*redis.Client,
	timeout time.Duration,
	clients map[string]*http.Client,
	filtersAdl *filter.FiltersBox,
	filtersMc *filter.FiltersBox,
	filtersCidAdl *filter.FilterCidBoxType,
	filtersCidMc *filter.FilterCidBoxType,
	filterBoxChangerAdl *filter.ChangersBoxChanger,
	filterBoxChangerMc *filter.ChangersBoxChanger,
	configTimeouts config.MapStringToDuration,
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
	advClient advGrpc.AdvServiceClient,
) *Server {
	rang := cidranger.NewPCTrieRanger()

	if formatRoutes != nil {
		formatRoutes.prepare(processor)
	}

	server := &Server{
		ruleManager:            ruleManager,
		fileLoader:             fileLoader,
		processor:              processor,
		formatRoutes:           formatRoutes,
		redisClients:           redisClients,
		redisWriteErrorMonitor: redisWriteErrorMonitor,
		clients:                clients,
		timeout:                timeout,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, 2048))
			},
		},
		ranger:              rang,
		filtersAdl:          filtersAdl,
		filtersMc:           filtersMc,
		filtersCidAdl:       filtersCidAdl,
		filtersCidMc:        filtersCidMc,
		filterBoxChangerAdl: filterBoxChangerAdl,
		filterBoxChangerMc:  filterBoxChangerMc,
		configTimeouts:      configTimeouts,
		advClient:           advClient,
	}
	if formatRoutes != nil {
		server.dspEndpoints_adult_v_2_5 = formatRoutes.POP.AdultEndpoints
		server.dspEndpoints_mainstream_v_2_5 = formatRoutes.POP.MainstreamEndpoints
		if formatRoutes.POP.AdultLinkMap != nil {
			legacy := map[string]map[string]map[string]bool(*formatRoutes.POP.AdultLinkMap)
			server.linkMap_adult = &legacy
		}
		if formatRoutes.POP.MainstreamLinkMap != nil {
			legacy := map[string]map[string]map[string]bool(*formatRoutes.POP.MainstreamLinkMap)
			server.linkMap_mainstream = &legacy
		}
	}
	return server
}

type dspDomainResp struct {
	domain string
	resp   *ortb_V2_5.BidResponse
}

type dspDomainCode struct {
	domain string
	code   string
}

func trafficTypeFromDspRouterRequest(req *dspRouterGrpc.DspRouterRequest_V2_5) string {
	if req == nil {
		return ""
	}
	if req.GetTrafficType() != "" {
		return req.GetTrafficType()
	}
	return req.GetTypic()
}

// successfulADVBidResponse returns a ready ADV bid only when the gRPC call
// completed without an error. An error always wins over any response object so
// the caller falls back to DSP even if a transport implementation returned both.
func successfulADVBidResponse(response *advGrpc.DoAuctionResponse, callErr error) *ortb_V2_5.BidResponse {
	if callErr != nil || response == nil {
		return nil
	}
	return response.GetBidResponse()
}

func (s *Server) doAdvAuction(
	ctx context.Context,
	req *dspRouterGrpc.DspRouterRequest_V2_5,
	timeout time.Duration,
) (*advGrpc.DoAuctionResponse, error) {
	if s == nil || s.advClient == nil {
		return nil, fmt.Errorf("adv client is not configured")
	}
	if req == nil || req.GetBidRequest() == nil {
		return nil, fmt.Errorf("ADV request is empty")
	}
	advCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	advBidRequest, err := buildADVAuctionBidRequest(req.GetBidRequest(), req.GetFormat())
	if err != nil {
		return nil, err
	}
	return s.advClient.DoAuction(advCtx, &advGrpc.DoAuctionRequest{
		BidRequest:  advBidRequest,
		Format:      req.GetFormat(),
		TrafficType: trafficTypeFromDspRouterRequest(req),
		SspDomain:   req.GetSspDomain(),
		ImpIdUuid:   cloneStringMap(req.GetImpIdUuid()),
	})
}

func buildADVAuctionBidRequest(source *ortb_V2_5.BidRequest, requestedFormat string) (*ortb_V2_5.BidRequest, error) {
	if source == nil {
		return nil, fmt.Errorf("ADV bid request is nil")
	}
	cloned, ok := proto.Clone(source).(*ortb_V2_5.BidRequest)
	if !ok || cloned == nil {
		return nil, fmt.Errorf("cannot clone ADV bid request")
	}
	format := strings.ToUpper(strings.TrimSpace(requestedFormat))
	if format != constants.BAN && format != constants.IPP {
		return cloned, nil
	}
	marker := constants.ADVImpressionFormatMarkerPrefix + format
	for _, imp := range cloned.GetImp() {
		if imp == nil || imp.GetBanner() == nil || imp.GetNative() != nil {
			continue
		}
		ext := imp.Banner.GetExt()[:0]
		for _, value := range imp.Banner.GetExt() {
			if !strings.HasPrefix(strings.TrimSpace(value), constants.ADVImpressionFormatMarkerPrefix) {
				ext = append(ext, value)
			}
		}
		imp.Banner.Ext = append(ext, marker)
	}
	return cloned, nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneFloat64Map(input map[string]float64) map[string]float64 {
	if len(input) == 0 {
		return map[string]float64{}
	}
	output := make(map[string]float64, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func bidResponseImpIDs(response *ortb_V2_5.BidResponse) map[string]struct{} {
	result := make(map[string]struct{})
	if response == nil {
		return result
	}
	for _, seat := range response.GetSeatbid() {
		if seat == nil {
			continue
		}
		for _, bid := range seat.GetBid() {
			if bid == nil || strings.TrimSpace(bid.GetImpid()) == "" {
				continue
			}
			result[bid.GetImpid()] = struct{}{}
		}
	}
	return result
}

// buildDSPFallbackBidRequest keeps the full request context but removes every
// impression already won by ADV. The returned UUID map is narrowed to exactly
// the impressions that are really sent downstream.
func buildDSPFallbackBidRequest(
	source *ortb_V2_5.BidRequest,
	impIDUUID map[string]string,
	readyADVResponse *ortb_V2_5.BidResponse,
) (*ortb_V2_5.BidRequest, map[string]string, error) {
	if source == nil {
		return nil, nil, fmt.Errorf("DSP fallback bid request is nil")
	}
	cloned, ok := proto.Clone(source).(*ortb_V2_5.BidRequest)
	if !ok || cloned == nil {
		return nil, nil, fmt.Errorf("cannot clone DSP fallback bid request")
	}

	resolvedByADV := bidResponseImpIDs(readyADVResponse)
	fallbackImps := make([]*ortb_V2_5.Imp, 0, len(cloned.GetImp()))
	fallbackUUIDs := make(map[string]string, len(impIDUUID))
	for _, imp := range cloned.GetImp() {
		// Preserve malformed/empty impressions on the DSP fallback path exactly
		// as the legacy full-request flow did; only a concrete ADV-winning imp ID
		// is removed.
		if imp == nil || strings.TrimSpace(imp.GetId()) == "" {
			fallbackImps = append(fallbackImps, imp)
			continue
		}
		if _, resolved := resolvedByADV[imp.GetId()]; resolved {
			continue
		}
		fallbackImps = append(fallbackImps, imp)
		if uuid, exists := impIDUUID[imp.GetId()]; exists {
			fallbackUUIDs[imp.GetId()] = uuid
		}
	}
	cloned.Imp = fallbackImps
	return cloned, fallbackUUIDs, nil
}

func (s *Server) GetBids_V2_5(
	ctx context.Context,
	req *dspRouterGrpc.DspRouterRequest_V2_5,
) (resp *dspRouterGrpc.DspRouterResponse_V2_5, funcErr error) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in GetBids_V2_5: %v, %s", r, string(debug.Stack()))
			resp = nil
			funcErr = status.Error(codes.Internal, err.Error())
		}
	}()

	if s == nil {
		return nil, status.Error(codes.Unavailable, "router service is nil")
	}
	if req == nil || req.GetBidRequest() == nil {
		return nil, status.Error(codes.InvalidArgument, "bid request is nil")
	}

	traceRequest := utils.ShouldTraceSSPDomain(req.GetSspDomain())
	if traceRequest {
		log.Printf(
			"[ROUTER][REQUEST_RECEIVED] request_id=%q ssp_domain=%q format=%q traffic_type=%q impressions=%d imp_uuid_count=%d",
			req.GetBidRequest().GetId(),
			req.GetSspDomain(),
			req.GetFormat(),
			trafficTypeFromDspRouterRequest(req),
			len(req.GetBidRequest().GetImp()),
			len(req.GetImpIdUuid()),
		)
	}

	timeout := getSspTimeout(req.GetSspDomain(), s.configTimeouts)
	if timeout <= 0 {
		timeout = time.Second
	}
	newTmax := int32(float64(timeout.Milliseconds()) * 0.85)
	req.BidRequest.Tmax = &newTmax
	if req.BidRequest.Device != nil && req.BidRequest.Device.Ip == nil && req.BidRequest.Device.Ipv6 != nil {
		req.BidRequest.Device.Ip = req.BidRequest.Device.Ipv6
	}

	var wg sync.WaitGroup

	trafficType := trafficTypeFromDspRouterRequest(req)
	requestFormat := normalizeDSPFormat(req.GetFormat())

	var dspList []DSPEndpointV25
	var linkMap GeoDspLinkMap
	var nativeMask filter.NativeFieldMask
	if s.formatRoutes != nil {
		dspList, linkMap, nativeMask = s.formatRoutes.selectRuntime(requestFormat, trafficType)
	} else if requestFormat == constants.POP {
		// Backwards-compatible fallback used by older tests/constructors. Sorting
		// here is test/legacy-only; production routes are precompiled at startup.
		switch trafficType {
		case sppAdapterWeb.ADULT:
			dspList = orderedEndpoints(s.dspEndpoints_adult_v_2_5)
			if s.linkMap_adult != nil {
				linkMap = *s.linkMap_adult
			}
		case sppAdapterWeb.MAINSTREAM:
			dspList = orderedEndpoints(s.dspEndpoints_mainstream_v_2_5)
			if s.linkMap_mainstream != nil {
				linkMap = *s.linkMap_mainstream
			}
		}
	}

	codesCh := make(chan *dspDomainCode, len(dspList))
	responsesCh := make(chan *dspDomainResp, len(dspList))

	var filters *filter.FiltersBox
	var filtersCid *filter.FilterCidBoxType
	var filterBoxChanger *filter.ChangersBoxChanger
	switch trafficType {
	case sppAdapterWeb.ADULT:
		filters = s.filtersAdl
		filtersCid = s.filtersCidAdl
		filterBoxChanger = s.filterBoxChangerAdl
	case sppAdapterWeb.MAINSTREAM:
		filters = s.filtersMc
		filtersCid = s.filtersCidMc
		filterBoxChanger = s.filterBoxChangerMc
	}

	if filters != nil && !filters.Allowed(req.BidRequest, "", true) {
		if traceRequest {
			log.Printf(
				"[ROUTER][REQUEST_REJECT] request_id=%q ssp_domain=%q format=%q reason=ssp_filter_rejected",
				req.GetBidRequest().GetId(),
				req.GetSspDomain(),
				req.GetFormat(),
			)
		}
		return &dspRouterGrpc.DspRouterResponse_V2_5{
			BidRequest:   req.BidRequest,
			BidResponses: map[string]*ortb_V2_5.BidResponse{},
			Code:         703,
			Rekl:         false,
			ImpIdUuid:    cloneStringMap(req.GetImpIdUuid()),
		}, nil
	}

	globalUuid := ""
	for _, value := range req.GetImpIdUuid() {
		globalUuid = value
		break
	}

	if traceRequest {
		log.Printf(
			"[ROUTER][ADV_CALL_START] request_id=%q ssp_domain=%q format=%q traffic_type=%q timeout_ms=%d impressions=%d",
			req.GetBidRequest().GetId(),
			req.GetSspDomain(),
			req.GetFormat(),
			trafficTypeFromDspRouterRequest(req),
			timeout.Milliseconds(),
			len(req.GetBidRequest().GetImp()),
		)
	}
	advStartedAt := time.Now()
	advResponse, advErr := s.doAdvAuction(ctx, req, timeout)
	readyADVResponse := successfulADVBidResponse(advResponse, advErr)
	if traceRequest {
		if advErr != nil {
			log.Printf(
				"[ROUTER][ADV_CALL_ERROR] request_id=%q ssp_domain=%q format=%q duration_ms=%d error=%v",
				req.GetBidRequest().GetId(),
				req.GetSspDomain(),
				req.GetFormat(),
				time.Since(advStartedAt).Milliseconds(),
				advErr,
			)
		} else {
			log.Printf(
				"[ROUTER][ADV_CALL_DONE] request_id=%q ssp_domain=%q format=%q duration_ms=%d response_nil=%t has_bid_response=%t winner_user_ids=%d winner_base_prices=%d",
				req.GetBidRequest().GetId(),
				req.GetSspDomain(),
				req.GetFormat(),
				time.Since(advStartedAt).Milliseconds(),
				advResponse == nil,
				readyADVResponse != nil,
				len(advResponse.GetWinnerUserIds()),
				len(advResponse.GetWinnerBasePrices()),
			)
		}
	}
	var readyADVForBidEngine *ortb_V2_5.BidResponse
	winnerUserIDs := map[string]string{}
	winnerBasePrices := map[string]float64{}
	if advErr != nil {
		// ADV errors always fall through with the full request. Ignore any
		// response object returned together with the error, preserving the
		// previous error semantics.
		log.Printf("ADV auction failed, falling back to DSP: %v", advErr)
		readyADVResponse = nil
	} else if len(bidResponseImpIDs(readyADVResponse)) > 0 {
		readyADVForBidEngine = readyADVResponse
		winnerUserIDs = cloneStringMap(advResponse.GetWinnerUserIds())
		winnerBasePrices = cloneFloat64Map(advResponse.GetWinnerBasePrices())
	}

	dspBidRequest, dspImpIdUuid, err := buildDSPFallbackBidRequest(
		req.GetBidRequest(),
		req.GetImpIdUuid(),
		readyADVForBidEngine,
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if len(dspBidRequest.GetImp()) == 0 && readyADVForBidEngine != nil {
		if traceRequest {
			log.Printf(
				"[ROUTER][ADV_SELECTED] request_id=%q ssp_domain=%q format=%q winner_user_ids=%d resolved_impressions=%d",
				req.GetBidRequest().GetId(),
				req.GetSspDomain(),
				req.GetFormat(),
				len(winnerUserIDs),
				len(bidResponseImpIDs(readyADVForBidEngine)),
			)
		}
		return &dspRouterGrpc.DspRouterResponse_V2_5{
			BidRequest:       req.GetBidRequest(),
			BidResponses:     map[string]*ortb_V2_5.BidResponse{},
			SspDomain:        req.GetSspDomain(),
			Code:             http.StatusOK,
			Rekl:             true,
			ReadyBidResponse: readyADVForBidEngine,
			WinnerUserIds:    winnerUserIDs,
			ImpIdUuid:        cloneStringMap(req.GetImpIdUuid()),
			WinnerBasePrices: winnerBasePrices,
		}, nil
	}

	if traceRequest {
		if readyADVForBidEngine != nil {
			log.Printf(
				"[ROUTER][ADV_PARTIAL_FALLBACK_DSP] request_id=%q ssp_domain=%q format=%q adv_resolved=%d dsp_impressions=%d dsp_endpoints=%d",
				req.GetBidRequest().GetId(),
				req.GetSspDomain(),
				req.GetFormat(),
				len(bidResponseImpIDs(readyADVForBidEngine)),
				len(dspBidRequest.GetImp()),
				len(dspList),
			)
		} else {
			log.Printf(
				"[ROUTER][ADV_NO_BID_FALLBACK_DSP] request_id=%q ssp_domain=%q format=%q dsp_endpoints=%d",
				req.GetBidRequest().GetId(),
				req.GetSspDomain(),
				req.GetFormat(),
				len(dspList),
			)
		}
	}

	jsonData, err := jsoniter.Marshal(dspBidRequest)
	if err != nil {
		newErr := fmt.Errorf("Can not marshal in GetBids_V_2_5 because got uknown error: %v", err)
		grpcCode := codes.Unknown
		if st, ok := status.FromError(err); ok {
			grpcCode = st.Code()
			newErr = fmt.Errorf("Can not marshal in GetBids_V_2_5 because got error: %v", st.Err())
		}
		return nil, status.Error(grpcCode, newErr.Error())
	}

	globalUuid = ""
	for _, value := range dspImpIdUuid {
		globalUuid = value
		break
	}

	var dspFilterCtx *filter.V25RequestContext
	if s.processor != nil {
		dspFilterCtx = filter.NewV25RequestContext(dspBidRequest, requestFormat, nativeMask)
	}

	for _, dspEndpoint := range dspList {
		endpoint := dspEndpoint.Endpoint
		domain := dspEndpoint.Domain

		country := ""
		if req.GetBidRequest().GetDevice() != nil && req.GetBidRequest().GetDevice().GetGeo() != nil {
			country = req.GetBidRequest().GetDevice().GetGeo().GetCountry()
		}
		if linkMap != nil && !utils.GetValueFomSspGeoDspMap(req.SspDomain, country, domain, linkMap, false) {
			codesCh <- &dspDomainCode{
				domain: domain,
				code:   "-2",
			}
			continue
		}

		if s.processor != nil && !s.processor.ProcessRequestContextForDSPV25(DeletePrefix(domain), dspFilterCtx).Allowed {
			codesCh <- &dspDomainCode{
				domain: domain,
				code:   "-3",
			}
			continue
		}

		if !Allowed(domain, dspBidRequest, s.ranger) {
			//log.Println("Gor DSP filter")
			codesCh <- &dspDomainCode{
				domain: domain,
				code:   "-1",
			}
			continue
		}

		if filters != nil && !filters.Allowed(dspBidRequest, domain, false) {
			//log.Println("Gor DSP filter")
			codesCh <- &dspDomainCode{
				domain: domain,
				code:   "-5",
			}
			//log.Printf("STOP SITE ID: %d, domain: %s, ssp domain: %s", req.BidRequest.Site.GetId(), domain)
			continue
		}

		/*	if req.SspDomain == "mc_clickadilla.com" && domain == "mc_dsp_dao.ad" {
			ChangeSiteId(req.BidRequest)
		}*/

		jsonDataTmp := jsonData
		mainRequest := dspBidRequest

		if strings.HasSuffix(domain, constants.BUYMEDIA) {
			newBidRequest := proto.Clone(dspBidRequest).(*ortb_V2_5.BidRequest)
			var mockBidfloor float32 = 0
			var mockSecure int32 = 1
			var mockBidfloorcur string = "USD"
			if newBidRequest.Imp != nil {
				for i := range newBidRequest.Imp {
					imp := newBidRequest.Imp[i]
					if imp != nil {
						newBidRequest.Imp[i].Bidfloor = &mockBidfloor
						newBidRequest.Imp[i].Secure = &mockSecure
						newBidRequest.Imp[i].Bidfloorcur = &mockBidfloorcur
						newBidRequest.Imp[i].Banner = &ortb_V2_5.Banner{}
					}
				}
			}

			mainRequest = newBidRequest

			jsonDataTmp, err = jsoniter.Marshal(newBidRequest)
			if err != nil {
				newErr := fmt.Errorf("Can not marshal in GetBids_V_2_5 because got uknown error: %v", err)

				grpcCode := codes.Unknown

				if st, ok := status.FromError(err); ok {
					grpcCode = st.Code()
					newErr = fmt.Errorf("Can not marshal in GetBids_V_2_5 because got error: %v", st.Err())
				}

				return nil, status.Error(grpcCode, newErr.Error())
			}
		}

		if filterBoxChanger != nil {
			if bidRequest, isChanged := filterBoxChanger.Change(mainRequest, domain); isChanged {
				//fmt.Println(bidRequest.Site.GetId())
				jsonDataTmp, err = jsoniter.Marshal(bidRequest)
				if err != nil {
					newErr := fmt.Errorf("Can not marshal in GetBids_V_2_5 because got uknown error: %v", err)

					grpcCode := codes.Unknown

					if st, ok := status.FromError(err); ok {
						grpcCode = st.Code()
						newErr = fmt.Errorf("Can not marshal in GetBids_V_2_5 because got error: %v", st.Err())
					}

					return nil, status.Error(grpcCode, newErr.Error())
				}
			}
		}

		/*if DeletePrefix(domain) == "dsp_test_hilltopads.com" {
			ChangeSiteIdhilltopTest(req.BidRequest)
		}*/

		client_v_2_5 := getDspHttpClients(domain, s.clients)

		wg.Add(1)
		go func(ctx context.Context, endpoint, domain string, client_v_2_5 *http.Client, timeout time.Duration) {
			defer wg.Done()

			reqCtx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			dspResp, code, err := s.getBidsFromDSPbyHTTP_V_2_5(reqCtx, globalUuid, jsonDataTmp, endpoint, client_v_2_5)
			if err != nil {
				/*log.Printf(
					"Cannot getBidsFromDSPbyHTTP_V_2_5, uuid: %s,ssp_domain: %s, dsp_domain: %s, timeout max %d ms, tmax: %d, error: %v",
					req.GlobalId,
					req.SspDomain,
					domain,
					client_v_2_5.Timeout.Milliseconds(),
					newTmax,
					err,
				)*/
			}

			if code == http.StatusOK && filtersCid != nil {
				if !filter.GetValueFomCidMap(dspResp, req.SspDomain, domain, *filtersCid) {
					codesCh <- &dspDomainCode{
						domain: domain,
						code:   "-77",
					}
					return
				}
			}

			codesCh <- &dspDomainCode{
				domain: domain,
				code:   fmt.Sprintf("%d", code),
			}
			if dspResp == nil {
				return
			}

			if !s.processor.ProcessResponseForSPPV25(DeletePrefix(req.SspDomain), dspResp).Allowed {
				//log.Printf("Gor SSP filter, domain %s, resp: %w", endpoint, dspResp)
				return
			}

			if strings.HasSuffix(req.SspDomain, "kadam.net") {
				for i := range dspResp.Seatbid {
					if dspResp.Seatbid[i] == nil {
						continue
					}
					for j := range dspResp.Seatbid[i].Bid {
						if dspResp.Seatbid[i].Bid[j] == nil {
							continue
						}
						adid := uuid.New().String()[:10]
						dspResp.Seatbid[i].Bid[j].Adid = &adid
					}
				}
			} else {
				for i := range dspResp.Seatbid {
					if dspResp.Seatbid[i] == nil {
						continue
					}
					for j := range dspResp.Seatbid[i].Bid {
						if dspResp.Seatbid[i].Bid[j] == nil {
							continue
						}
						dspResp.Seatbid[i].Bid[j].Adid = nil
					}
				}
			}

			if dspResp != nil {
				responsesCh <- &dspDomainResp{
					domain: domain,
					resp:   dspResp,
				}
			}
		}(ctx, endpoint, domain, client_v_2_5, timeout)
	}

	go func() {
		wg.Wait()
		close(codesCh)
		close(responsesCh)
	}()

	clickResponses := make(map[string]string)
	for c := range codesCh {
		clickResponses[c.domain] = c.code
	}

	for _, uuid := range dspImpIdUuid {
		if err := writeBidResponsesToRedis(s.redisClients, uuid, clickResponses, req.Logged); err != nil {
			log.Printf("failed to write bid responses to Redis: %v", err)
			if s.redisWriteErrorMonitor != nil {
				s.redisWriteErrorMonitor.RecordForURL(err, req.SspUrl)
			}
		}
	}

	responses := make(map[string]*ortb_V2_5.BidResponse)
	for r := range responsesCh {
		responses[r.domain] = r.resp
	}

	return &dspRouterGrpc.DspRouterResponse_V2_5{
		BidRequest:       req.GetBidRequest(),
		BidResponses:     responses,
		SspDomain:        req.GetSspDomain(),
		Rekl:             false,
		ReadyBidResponse: readyADVForBidEngine,
		WinnerUserIds:    winnerUserIDs,
		ImpIdUuid:        cloneStringMap(req.GetImpIdUuid()),
		WinnerBasePrices: winnerBasePrices,
	}, nil
}

func (s *Server) getBidsFromDSPbyHTTP_V_2_5(ctx context.Context, uuid string, jsonData []byte, dspEndpoint string, client_v_2_5 *http.Client) (
	ddr *ortb_V2_5.BidResponse, code int32, err error) {
	/*if dspEndpoint == "none" {
		return nil, http.StatusNoContent, nil
	}*/

	buf := s.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Write(jsonData)
	defer s.bufferPool.Put(buf)

	req, err := http.NewRequestWithContext(ctx, "POST", dspEndpoint, buf)
	if err != nil {
		//log.Println("Create request failed: %v", err)
		return nil, 55, fmt.Errorf("Create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("X-Openrtb-Version", "2.5")
	networkStart := time.Now()
	resp, err := client_v_2_5.Do(req)
	networkDuration := time.Since(networkStart)

	if err != nil {
		return nil, 1, fmt.Errorf("Timeout: %d ms, Request failed: %v", networkDuration.Milliseconds(), err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 4, fmt.Errorf("read body failed: %v", err)
	}

	if resp.StatusCode == http.StatusOK {
		var grpcResp ortb_V2_5.BidResponse
		if err := jsoniter.Unmarshal(body, &grpcResp); err != nil {
			log.Printf("uuid: %s, body: %s", uuid, string(body))
			return nil, 3, fmt.Errorf("decode: %v, body: %s", err, string(body))
		}
		return &grpcResp, int32(resp.StatusCode), nil
	}
	return nil, int32(resp.StatusCode), nil
}

func writeBidResponsesToRedis(
	redisClients []*redis.Client,
	uuid string,
	data map[string]string,
	logged bool,
) error {
	if !logged {
		return nil
	}

	bg, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	payload, err := proto.Marshal(&eventspb.BidResponses{
		Items: data,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal bid responses protobuf: %w", err)
	}

	return utils.WriteBytesToRedis(bg, redisClients, uuid, constants.BID_RESPONSES_COLUMN, payload, logged)
}
