package dspRouterWeb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
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
	"google.golang.org/protobuf/types/known/emptypb"
)

type Server struct {
	ruleManager *filter.RuleManager
	fileLoader  *filter.FileRuleLoader
	processor   *filter.OptimizedFilterProcessor

	dspEndpoints_v_2_5            config.MapStringToString
	dspEndpoints_mainstream_v_2_5 config.MapStringToString

	redisClient *redis.Client

	client_v_2_5 *http.Client
	timeout      time.Duration
	// Пулы для снижения аллокаций
	bufferPool sync.Pool

	ranger cidranger.Ranger

	linkFilename_adult      string
	linkFilename_mainstream string

	linkMap_adult      map[string]map[string]map[string]bool
	linkMap_mainstream map[string]map[string]map[string]bool

	dspRouterGrpc.UnimplementedDspRouterServiceServer
}

func NewFastHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second, // Уменьшаем с 30s до 5s
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,

		// Пул соединений - ОБЯЗАТЕЛЬНО оставить!
		MaxIdleConns:        500,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     200,
		IdleConnTimeout:     30 * time.Second, // ОБЯЗАТЕЛЬНО оставить!

		DisableCompression: true,
		ForceAttemptHTTP2:  false,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   150 * time.Millisecond, // Главный таймаут
	}
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
	linkFilename_adult string,
	linkFilename_mainstream string,
	linkMap_adult map[string]map[string]map[string]bool,
	linkMap_mainstream map[string]map[string]map[string]bool,
) *Server {
	if timeout <= 0 {
		timeout = 150 * time.Millisecond
	}
	if maxParallelRequests <= 0 {
		maxParallelRequests = 64
	}

	client_v_2_5 := NewFastHTTPClient()

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
		dspEndpoints_v_2_5:            dspEndpoints_v_2_5,
		dspEndpoints_mainstream_v_2_5: dspEndpoints_mainstream_v_2_5,
		redisClient:                   redisClient,
		client_v_2_5:                  client_v_2_5,
		timeout:                       timeout,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, 2048))
			},
		},
		ranger:                  rang,
		linkFilename_adult:      linkFilename_adult,
		linkFilename_mainstream: linkFilename_mainstream,
		linkMap_adult:           linkMap_adult,
		linkMap_mainstream:      linkMap_mainstream,
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

	reqCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer func() {
		cancel()
		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in GetBids_V2_5: %v, %s", r, string(debug.Stack()))
			resp = nil
			funcErr = status.Error(codes.Internal, err.Error())
		}
	}()

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

	var (
		wg sync.WaitGroup
	)

	codesCh := make(chan *dspDomainCode, len(s.dspEndpoints_v_2_5))
	responsesCh := make(chan *dspDomainResp, len(s.dspEndpoints_v_2_5))

	var dspList config.MapStringToString
	switch req.Typic {
	case sppAdapterWeb.ADULT:
		dspList = s.dspEndpoints_v_2_5
	case sppAdapterWeb.MAINSTREAM:
		dspList = s.dspEndpoints_mainstream_v_2_5
	}

	for endpoint, domain := range dspList {
		endpoint := endpoint
		domain := domain

		switch req.Typic {
		case sppAdapterWeb.ADULT:
			if !utils.GetValueFomSspGeoDspMap(req.SspDomain, req.BidRequest.Device.Geo.GetCountry(), domain, s.linkMap_adult, true) {
				codesCh <- &dspDomainCode{
					domain: domain,
					code:   -2,
				}
				continue
			}
		case sppAdapterWeb.MAINSTREAM:
			if !utils.GetValueFomSspGeoDspMap(req.SspDomain, req.BidRequest.Device.Geo.GetCountry(), domain, s.linkMap_mainstream, true) {
				codesCh <- &dspDomainCode{
					domain: domain,
					code:   -2,
				}
				continue
			}
		}

		if !s.processor.ProcessRequestForDSPV25(endpoint, req.BidRequest).Allowed {
			//log.Println("Gor DSP filter")
			codesCh <- &dspDomainCode{
				domain: domain,
				code:   -2,
			}
			continue
		}

		/*if !Allowed(endpoint, req.BidRequest, s.ranger) {
			//log.Println("Gor DSP filter")
			codesCh <- &dspDomainCode{
				domain: domain,
				code:   -1,
			}
			continue
		}*/

		wg.Add(1)
		go func(endpoint string) {
			defer wg.Done()

			dspResp, code, err := s.getBidsFromDSPbyHTTP_V_2_5(reqCtx, jsonData, endpoint)
			if err != nil {
				log.Printf(
					"Cannot getBidsFromDSPbyHTTP_V_2_5, bid request id: %s,ssp_domain: %s, dsp_domain: %s, error: %v",
					req.BidRequest.GetId(),
					req.SspDomain,
					endpoint,
					err,
				)
			}

			codesCh <- &dspDomainCode{
				domain: domain,
				code:   code,
			}

			// Фильтрация ответа SPP
			if !s.processor.ProcessResponseForSPPV25(req.SspDomain, dspResp).Allowed {
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
		}(endpoint)
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

func (s *Server) getBidsFromDSPbyHTTP_V_2_5(ctx context.Context, jsonData []byte, dspEndpoint string) (
	ddr *ortb_V2_5.BidResponse, code int, err error) {
	// Пул буферов — как в v2.4
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
	if dspEndpoint == HILLTOP_DSP {
		req.Header.Set("X-Openrtb-Version", "2.5")
	}

	resp, err := s.client_v_2_5.Do(req)
	if err != nil {
		return nil, 1, fmt.Errorf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var grpcResp ortb_V2_5.BidResponse
		dec := jsoniter.NewDecoder(resp.Body) // без лишних аллокаций
		if err := dec.Decode(&grpcResp); err != nil {
			return nil, 3, fmt.Errorf("decode: %v", err)
		}
		return &grpcResp, resp.StatusCode, nil
	}
	return nil, resp.StatusCode, nil
}

func (s *Server) GetSspGeoLinksMap(ctx context.Context, req *dspRouterGrpc.GetSspGeoDspLinksRequest_V2_5) (*dspRouterGrpc.GetSspGeoDspLinksResponse_V2_5, error) {
	var data []byte
	var err error

	var typic string
	err = json.Unmarshal([]byte(req.Typic), &typic)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid JSON format: %v", err)
	}

	switch typic {
	case sppAdapterWeb.ADULT:
		data, err = os.ReadFile(s.linkFilename_adult)
	case sppAdapterWeb.MAINSTREAM:
		data, err = os.ReadFile(s.linkFilename_mainstream)
	default:
		return nil, status.Error(codes.Internal, "Typic is no here! failed to read file")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read file %s: %w", s.linkFilename_adult, err)
	}

	return &dspRouterGrpc.GetSspGeoDspLinksResponse_V2_5{
		JsonData: string(data),
	}, nil
}

func (s *Server) SetSspGeoLinksMap(ctx context.Context, req *dspRouterGrpc.SspGeoDspLinksRequest_V2_5) (*emptypb.Empty, error) {
	var err error

	var typic string
	err = json.Unmarshal([]byte(req.Typic), &typic)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid JSON format: %v", err)
	}

	switch typic {
	case sppAdapterWeb.ADULT:
		s.linkMap_adult, err = utils.RewriteSspGeoDspFile[bool](
			req.JsonData,
			s.linkFilename_adult,
		)
	case sppAdapterWeb.MAINSTREAM:
		s.linkMap_mainstream, err = utils.RewriteSspGeoDspFile[bool](
			req.JsonData,
			s.linkFilename_mainstream,
		)
	default:
		return nil, status.Error(codes.Internal, "Typic is no here! failed to read file")
	}
	if err != nil {
		return nil, err
	}

	log.Printf("Successfully updated Links SSP entries")

	return nil, nil
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
