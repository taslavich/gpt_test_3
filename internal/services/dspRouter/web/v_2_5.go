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
	siteIDDSPLinkStore            *SiteIDDSPLinkStore

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
	siteIDDSPLinkStores ...*SiteIDDSPLinkStore,
) *Server {
	rang := cidranger.NewPCTrieRanger()

	if formatRoutes != nil {
		formatRoutes.prepare(processor)
	}

	var siteIDDSPLinkStore *SiteIDDSPLinkStore
	if len(siteIDDSPLinkStores) > 0 {
		siteIDDSPLinkStore = siteIDDSPLinkStores[0]
	}

	server := &Server{
		ruleManager:            ruleManager,
		fileLoader:             fileLoader,
		processor:              processor,
		formatRoutes:           formatRoutes,
		siteIDDSPLinkStore:     siteIDDSPLinkStore,
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
	timeout := getSspTimeout(req.GetSspDomain(), s.configTimeouts)
	if timeout <= 0 {
		timeout = time.Second
	}
	newTmax := int32(float64(timeout.Milliseconds()) * 0.85)
	req.BidRequest.Tmax = &newTmax
	if req.BidRequest.Device != nil && req.BidRequest.Device.Ip == nil && req.BidRequest.Device.Ipv6 != nil {
		req.BidRequest.Device.Ip = req.BidRequest.Device.Ipv6
	}

	trafficType := trafficTypeFromDspRouterRequest(req)
	requestFormat := normalizeDSPFormat(req.GetFormat())
	if traceRequest {
		log.Printf("[ROUTER][REQUEST_RECEIVED] request_id=%q ssp_domain=%q format=%q traffic_type=%q impressions=%d imp_uuid_count=%d timeout_ms=%d",
			req.GetBidRequest().GetId(), req.GetSspDomain(), req.GetFormat(), trafficType,
			len(req.GetBidRequest().GetImp()), len(req.GetImpIdUuid()), timeout.Milliseconds())
	}

	var dspList []DSPEndpointV25
	var linkMap GeoDspLinkMap
	var nativeMask filter.NativeFieldMask
	if s.formatRoutes != nil {
		dspList, linkMap, nativeMask = s.formatRoutes.selectRuntime(requestFormat, trafficType)
	} else if requestFormat == constants.POP {
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

	var filters *filter.FiltersBox
	var filtersCid *filter.FilterCidBoxType
	var filterBoxChanger *filter.ChangersBoxChanger
	switch trafficType {
	case sppAdapterWeb.ADULT:
		filters, filtersCid, filterBoxChanger = s.filtersAdl, s.filtersCidAdl, s.filterBoxChangerAdl
	case sppAdapterWeb.MAINSTREAM:
		filters, filtersCid, filterBoxChanger = s.filtersMc, s.filtersCidMc, s.filterBoxChangerMc
	}
	if filters != nil && !filters.Allowed(req.BidRequest, "", true) {
		return &dspRouterGrpc.DspRouterResponse_V2_5{
			BidRequest: req.BidRequest, BidResponses: map[string]*ortb_V2_5.BidResponse{},
			Code: 703, Rekl: false, ImpIdUuid: cloneStringMap(req.GetImpIdUuid()),
		}, nil
	}

	// ADV and ordinary DSP are intentionally started in parallel. DSP receives
	// the complete incoming BidRequest; it no longer waits for ADV and no
	// impressions are removed based on the ADV result.
	fanoutCtx, cancelFanout := context.WithTimeout(ctx, timeout)
	defer cancelFanout()
	type advResult struct {
		response *advGrpc.DoAuctionResponse
		err      error
	}
	advCh := make(chan advResult, 1)
	go func() {
		response, err := s.doAdvAuction(fanoutCtx, req, timeout)
		advCh <- advResult{response: response, err: err}
	}()

	dspBidRequest, ok := proto.Clone(req.GetBidRequest()).(*ortb_V2_5.BidRequest)
	if !ok || dspBidRequest == nil {
		return nil, status.Error(codes.Internal, "cannot clone DSP bid request")
	}
	dspJSON, err := jsoniter.Marshal(dspBidRequest)
	if err != nil {
		return nil, status.Errorf(codes.Unknown, "cannot marshal DSP request: %v", err)
	}
	globalUUID := ""
	for _, value := range req.GetImpIdUuid() {
		globalUUID = value
		break
	}
	var filterCtx *filter.V25RequestContext
	if s.processor != nil {
		filterCtx = filter.NewV25RequestContext(dspBidRequest, requestFormat, nativeMask)
	}

	type demandJob struct {
		endpoint string
		domain   string
		jsonData []byte
		client   *http.Client
	}
	jobs := make([]demandJob, 0, len(dspList))
	preCodes := make(map[string]string)
	siteID := req.GetBidRequest().GetSite().GetId()
	for _, demandEndpoint := range dspList {
		endpoint, domain := demandEndpoint.Endpoint, demandEndpoint.Domain
		country := ""
		if req.GetBidRequest().GetDevice() != nil && req.GetBidRequest().GetDevice().GetGeo() != nil {
			country = req.GetBidRequest().GetDevice().GetGeo().GetCountry()
		}
		if !shouldRouteDSPByMaps(siteID, domain, req.SspDomain, country, s.siteIDDSPLinkStore, linkMap) {
			preCodes[domain] = "-2"
			continue
		}
		if s.processor != nil && !s.processor.ProcessRequestContextForDSPV25(DeletePrefix(domain), filterCtx).Allowed {
			preCodes[domain] = "-3"
			continue
		}
		if !Allowed(domain, dspBidRequest, s.ranger) {
			preCodes[domain] = "-1"
			continue
		}
		if filters != nil && !filters.Allowed(dspBidRequest, domain, false) {
			preCodes[domain] = "-5"
			continue
		}

		jsonData := dspJSON
		mainRequest := dspBidRequest
		if strings.HasSuffix(domain, constants.BUYMEDIA) {
			changed := proto.Clone(dspBidRequest).(*ortb_V2_5.BidRequest)
			var mockBidfloor float32
			var mockSecure int32 = 1
			mockBidfloorcur := "USD"
			for _, imp := range changed.GetImp() {
				if imp == nil {
					continue
				}
				imp.Bidfloor = &mockBidfloor
				imp.Secure = &mockSecure
				imp.Bidfloorcur = &mockBidfloorcur
				imp.Banner = &ortb_V2_5.Banner{}
			}
			mainRequest = changed
			jsonData, err = jsoniter.Marshal(changed)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "marshal BUYMEDIA request: %v", err)
			}
		}
		if filterBoxChanger != nil {
			if changedRequest, isChanged := filterBoxChanger.Change(mainRequest, domain); isChanged {
				jsonData, err = jsoniter.Marshal(changedRequest)
				if err != nil {
					return nil, status.Errorf(codes.Internal, "marshal changed DSP request: %v", err)
				}
			}
		}
		jobs = append(jobs, demandJob{endpoint: endpoint, domain: domain, jsonData: jsonData, client: getDspHttpClients(domain, s.clients)})
	}

	codesCh := make(chan *dspDomainCode, len(jobs))
	responsesCh := make(chan *dspDomainResp, len(jobs))
	var wg sync.WaitGroup
	for _, job := range jobs {
		job := job
		wg.Add(1)
		go func() {
			defer wg.Done()
			reqCtx, cancel := context.WithTimeout(fanoutCtx, timeout)
			defer cancel()
			demandResp, code, _ := s.getBidsFromDSPbyHTTP_V_2_5(reqCtx, globalUUID, job.jsonData, job.endpoint, job.client)
			if code == http.StatusOK && filtersCid != nil && !filter.GetValueFomCidMap(demandResp, req.SspDomain, job.domain, *filtersCid) {
				codesCh <- &dspDomainCode{domain: job.domain, code: "-77"}
				return
			}
			codesCh <- &dspDomainCode{domain: job.domain, code: fmt.Sprintf("%d", code)}
			if demandResp == nil {
				return
			}
			if s.processor != nil && !s.processor.ProcessResponseForSPPV25(DeletePrefix(req.SspDomain), demandResp).Allowed {
				return
			}
			if strings.HasSuffix(req.SspDomain, "kadam.net") {
				for _, seat := range demandResp.GetSeatbid() {
					for _, bid := range seat.GetBid() {
						if bid == nil {
							continue
						}
						adid := uuid.New().String()[:10]
						bid.Adid = &adid
					}
				}
			} else {
				for _, seat := range demandResp.GetSeatbid() {
					for _, bid := range seat.GetBid() {
						if bid != nil {
							bid.Adid = nil
						}
					}
				}
			}
			responsesCh <- &dspDomainResp{domain: job.domain, resp: demandResp}
		}()
	}
	wg.Wait()
	close(codesCh)
	close(responsesCh)

	dspCodes := preCodes
	for item := range codesCh {
		dspCodes[item.domain] = item.code
	}
	for _, requestUUID := range req.GetImpIdUuid() {
		if len(dspCodes) == 0 {
			continue
		}
		if err := writeBidResponsesToRedis(s.redisClients, requestUUID, dspCodes, req.Logged); err != nil {
			log.Printf("failed to write bid responses to Redis: %v", err)
			if s.redisWriteErrorMonitor != nil {
				s.redisWriteErrorMonitor.RecordForURL(err, req.SspUrl)
			}
		}
	}
	responses := make(map[string]*ortb_V2_5.BidResponse)
	for item := range responsesCh {
		responses[item.domain] = item.resp
	}

	adv := <-advCh
	readyADV := successfulADVBidResponse(adv.response, adv.err)
	winnerUserIDs := map[string]string{}
	winnerBasePrices := map[string]float64{}
	if adv.err != nil {
		log.Printf("ADV auction failed; DSP results remain available: %v", adv.err)
		readyADV = nil
	} else if readyADV != nil {
		winnerUserIDs = cloneStringMap(adv.response.GetWinnerUserIds())
		winnerBasePrices = cloneFloat64Map(adv.response.GetWinnerBasePrices())
	}

	return &dspRouterGrpc.DspRouterResponse_V2_5{
		BidRequest: req.GetBidRequest(), BidResponses: responses, SspDomain: req.GetSspDomain(),
		Rekl: false, ReadyBidResponse: readyADV, WinnerUserIds: winnerUserIDs,
		ImpIdUuid: cloneStringMap(req.GetImpIdUuid()), WinnerBasePrices: winnerBasePrices,
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
