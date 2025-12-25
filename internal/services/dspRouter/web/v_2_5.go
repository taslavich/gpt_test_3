package dspRouterWeb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/redis/go-redis/v9"
	"github.com/yl2chen/cidranger"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/coder"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	ruleManager *filter.RuleManager
	fileLoader  *filter.FileRuleLoader
	processor   *filter.OptimizedFilterProcessor

	dspEndpoints_adult_v_2_5      config.MapStringToString
	dspEndpoints_mainstream_v_2_5 config.MapStringToString

	redisClient *redis.Client

	clients map[string]*http.Client

	timeout time.Duration

	bufferPool sync.Pool

	ranger cidranger.Ranger

	linkMap_adult      *map[string]map[string]map[string]bool
	linkMap_mainstream *map[string]map[string]map[string]bool

	filters *filter.FiltersBox

	dspRouterGrpc.UnimplementedDspRouterServiceServer
}

func NewServer(
	ruleManager *filter.RuleManager,
	fileLoader *filter.FileRuleLoader,
	processor *filter.OptimizedFilterProcessor,
	dspEndpoints_v_2_5,
	dspEndpoints_mainstream_v_2_5 config.MapStringToString,
	redisClient *redis.Client,
	timeout time.Duration,
	maxParallelRequests int,
	linkMap_adult *map[string]map[string]map[string]bool,
	linkMap_mainstream *map[string]map[string]map[string]bool,
	clients map[string]*http.Client,
	filters *filter.FiltersBox,
) *Server {
	if maxParallelRequests <= 0 {
		maxParallelRequests = 64
	}

	// Глобальный лимит исходящих: консервативно отталкиваемся от
	// количества DSP и локального лимита; при желании — вынеси в конфиг.
	outboundLimit := maxParallelRequests * 2

	if l := len(dspEndpoints_v_2_5); l > 0 && l*maxParallelRequests > outboundLimit {
		outboundLimit = l * maxParallelRequests
	}
	if outboundLimit < 256 {
		outboundLimit = 256
	}

	rang := cidranger.NewPCTrieRanger()

	return &Server{
		ruleManager:                   ruleManager,
		fileLoader:                    fileLoader,
		processor:                     processor,
		dspEndpoints_adult_v_2_5:      dspEndpoints_v_2_5,
		dspEndpoints_mainstream_v_2_5: dspEndpoints_mainstream_v_2_5,
		redisClient:                   redisClient,
		clients:                       clients,
		timeout:                       timeout,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, 2048))
			},
		},
		ranger:             rang,
		linkMap_adult:      linkMap_adult,
		linkMap_mainstream: linkMap_mainstream,
		filters:            filters,
	}
}

type dspDomainResp struct {
	domain string
	resp   *ortb_V2_5.BidResponse
}

type dspDomainCode struct {
	domain string
	code   int
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

	client_v_2_5 := getSspHttpClients(req.SspDomain, s.clients)
	newTmax := int32(float64(client_v_2_5.Timeout.Milliseconds()) * 0.85)
	req.BidRequest.Tmax = &newTmax

	jsonData, err := jsoniter.Marshal(req.BidRequest)
	if err != nil {
		newErr := fmt.Errorf("Can not marshal in GetBids_V_2_5 because got uknown error: %v", err)

		grpcCode := codes.Unknown

		st, ok := status.FromError(err)
		if !ok {
			grpcCode = st.Code()
			newErr = fmt.Errorf("Can not marshal in GetBids_V_2_5 because got error: %v", st.Err())
		}

		return nil, status.Error(grpcCode, newErr.Error())
	}

	if err := utils.WriteJsonToRedis(ctx, s.redisClient, req.GlobalId, constants.BID_REQUEST_COLUMN, jsonData, req.Logged); err != nil {
		log.Printf("failed to WriteJsonToRedis Bid Request in postBid_V2_5: %w", err)
	}

	var (
		wg sync.WaitGroup
	)

	codesCh := make(chan *dspDomainCode, len(s.dspEndpoints_adult_v_2_5))
	responsesCh := make(chan *dspDomainResp, len(s.dspEndpoints_adult_v_2_5))

	var dspList config.MapStringToString
	var linkMap map[string]map[string]map[string]bool
	switch req.Typic {
	case sppAdapterWeb.ADULT:
		dspList = s.dspEndpoints_adult_v_2_5
		linkMap = *s.linkMap_adult
	case sppAdapterWeb.MAINSTREAM:
		dspList = s.dspEndpoints_mainstream_v_2_5
		linkMap = *s.linkMap_mainstream
	}

	for endpoint, domain := range dspList {
		endpoint := endpoint
		domain := domain

		if !utils.GetValueFomSspGeoDspMap(req.SspDomain, req.BidRequest.Device.Geo.GetCountry(), domain, linkMap, false) {
			codesCh <- &dspDomainCode{
				domain: domain,
				code:   -2,
			}
			continue
		}

		if !s.processor.ProcessRequestForDSPV25(DeletePrefix(domain), req.BidRequest).Allowed {
			//log.Println("Gor DSP filter")
			codesCh <- &dspDomainCode{
				domain: domain,
				code:   -3,
			}
			continue
		}

		if !Allowed(domain, req.BidRequest, s.ranger) {
			//log.Println("Gor DSP filter")
			codesCh <- &dspDomainCode{
				domain: domain,
				code:   -1,
			}
			continue
		}

		if !s.filters.Allowed(req.BidRequest, domain) {
			//log.Println("Gor DSP filter")
			codesCh <- &dspDomainCode{
				domain: domain,
				code:   -5,
			}
			continue
		}

		if req.SspDomain == "mc_clickadilla.com" && domain == "mc_dsp_dao.ad" {
			ChangeSiteId(req.BidRequest)
		}

		if DeletePrefix(domain) == "dsp_test_hilltopads.com" {
			ChangeSiteIdhilltopTest(req.BidRequest)
		}

		wg.Add(1)
		go func(ctx context.Context, endpoint, domain string, client_v_2_5 *http.Client) {
			defer wg.Done()

			reqCtx, cancel := context.WithTimeout(context.Background(), client_v_2_5.Timeout)
			defer cancel()

			dspResp, code, err := s.getBidsFromDSPbyHTTP_V_2_5(reqCtx, req.GlobalId, jsonData, endpoint, client_v_2_5)
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

			codesCh <- &dspDomainCode{
				domain: domain,
				code:   code,
			}

			if !s.processor.ProcessResponseForSPPV25(DeletePrefix(req.SspDomain), dspResp).Allowed {
				//log.Printf("Gor SSP filter, domain %s, resp: %w", endpoint, dspResp)
				return
			}

			for i := range dspResp.Seatbid {
				if dspResp.Seatbid[i] != nil {
					for j := range dspResp.Seatbid[i].Bid {
						if dspResp.Seatbid[i].Bid[j].Adm != nil {
							adid := coder.AdmToAdidCompact(*dspResp.Seatbid[i].Bid[j].Adm)
							dspResp.Seatbid[i].Bid[j].Adid = &adid
						}
					}
				}
			}

			if dspResp != nil {
				responsesCh <- &dspDomainResp{
					domain: domain,
					resp:   dspResp,
				}
			}
		}(ctx, endpoint, domain, client_v_2_5)
	}

	go func() {
		wg.Wait()
		close(codesCh)
		close(responsesCh)
	}()

	clickResponses := make(map[string]int)
	for c := range codesCh {
		clickResponses[c.domain] = c.code
	}

	writeMetadataToRedis(ctx, s.redisClient, req.GlobalId, clickResponses, req.Logged)

	responses := make(map[string]*ortb_V2_5.BidResponse)
	for r := range responsesCh {
		responses[r.domain] = r.resp
	}

	return &dspRouterGrpc.DspRouterResponse_V2_5{
		BidRequest:   req.BidRequest,
		BidResponses: responses,
		GlobalId:     req.GlobalId,
		SspDomain:    req.SspDomain,
	}, nil
}

func (s *Server) getBidsFromDSPbyHTTP_V_2_5(ctx context.Context, uuid string, jsonData []byte, dspEndpoint string, client_v_2_5 *http.Client) (
	ddr *ortb_V2_5.BidResponse, code int, err error) {
	buf := s.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Write(jsonData)
	defer s.bufferPool.Put(buf)

	req, err := http.NewRequestWithContext(ctx, "POST", dspEndpoint, buf)
	if err != nil {
		return nil, 0, fmt.Errorf("Create request failed: %v", err)
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
	defer resp.Body.Close()

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
		return &grpcResp, resp.StatusCode, nil
	}
	return nil, resp.StatusCode, nil
}

func writeMetadataToRedis(ctx context.Context, redisClient *redis.Client, globalId string, data map[string]int, logged bool) {
	bidRespsData, err := json.Marshal(data)
	if err != nil {
		log.Printf("failed to marshal data: %v", err)
		return
	}
	bgCtx := context.Background()
	if err := utils.WriteJsonToRedis(bgCtx, redisClient, globalId, constants.BID_RESPONSES_COLUMN, bidRespsData, logged); err != nil {
		log.Printf("failed to WriteJsonToRedis: %v", err)
	}
}
