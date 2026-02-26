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
	"strings"
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
	"google.golang.org/protobuf/proto"
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

	filtersAdl *filter.FiltersBox
	filtersMc  *filter.FiltersBox

	filtersCidAdl *filter.FilterCidBoxType
	filtersCidMc  *filter.FilterCidBoxType

	filterBoxChangerAdl *filter.ChangersBoxChanger
	filterBoxChangerMc  *filter.ChangersBoxChanger

	configTimeouts config.MapStringToDuration

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
	linkMap_adult *map[string]map[string]map[string]bool,
	linkMap_mainstream *map[string]map[string]map[string]bool,
	clients map[string]*http.Client,
	filtersAdl *filter.FiltersBox,
	filtersMc *filter.FiltersBox,
	filtersCidAdl *filter.FilterCidBoxType,
	filtersCidMc *filter.FilterCidBoxType,
	filterBoxChangerAdl *filter.ChangersBoxChanger,
	filterBoxChangerMc *filter.ChangersBoxChanger,
	configTimeouts config.MapStringToDuration,
) *Server {
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
		ranger:              rang,
		linkMap_adult:       linkMap_adult,
		linkMap_mainstream:  linkMap_mainstream,
		filtersAdl:          filtersAdl,
		filtersMc:           filtersMc,
		filtersCidAdl:       filtersCidAdl,
		filtersCidMc:        filtersCidMc,
		filterBoxChangerAdl: filterBoxChangerAdl,
		filterBoxChangerMc:  filterBoxChangerMc,
		configTimeouts:      configTimeouts,
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

	timeout := getSspTimeout(req.SspDomain, s.configTimeouts)

	newTmax := int32(float64(timeout.Milliseconds()) * 0.85)
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

	bg, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if err := utils.WriteJsonToRedis(bg, s.redisClient, req.GlobalId, constants.BID_REQUEST_COLUMN, jsonData, req.Logged); err != nil {
		log.Printf("failed to WriteJsonToRedis Bid Request in postBid_V2_5: %w", err)
	}

	var (
		wg sync.WaitGroup
	)

	codesCh := make(chan *dspDomainCode, len(s.dspEndpoints_adult_v_2_5))
	responsesCh := make(chan *dspDomainResp, len(s.dspEndpoints_adult_v_2_5))

	var dspList config.MapStringToString
	var linkMap map[string]map[string]map[string]bool
	var filters *filter.FiltersBox
	var filtersCid *filter.FilterCidBoxType
	//var filterBoxChanger *filter.ChangersBoxChanger
	switch req.Typic {
	case sppAdapterWeb.ADULT:
		dspList = s.dspEndpoints_adult_v_2_5
		linkMap = *s.linkMap_adult
		filters = s.filtersAdl
		filtersCid = s.filtersCidAdl
		//filterBoxChanger = s.filterBoxChangerAdl
	case sppAdapterWeb.MAINSTREAM:
		dspList = s.dspEndpoints_mainstream_v_2_5
		linkMap = *s.linkMap_mainstream
		filters = s.filtersMc
		filtersCid = s.filtersCidMc
		//filterBoxChanger = s.filterBoxChangerMc
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

		if !filters.Allowed(req.BidRequest, domain) {
			//log.Println("Gor DSP filter")
			codesCh <- &dspDomainCode{
				domain: domain,
				code:   -5,
			}
			//log.Printf("STOP SITE ID: %d, domain: %s, ssp domain: %s", req.BidRequest.Site.GetId(), domain)
			continue
		}

		/*	if req.SspDomain == "mc_clickadilla.com" && domain == "mc_dsp_dao.ad" {
			ChangeSiteId(req.BidRequest)
		}*/

		jsonDataTmp := jsonData

		if strings.HasSuffix(domain, constants.BUYMEDIA) {
			newBidRequest := proto.Clone(req.BidRequest).(*ortb_V2_5.BidRequest)
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
					}
				}
			}
		}

		/*if bidRequest, isChanged := filterBoxChanger.Change(req.BidRequest, domain); isChanged {
			jsonDataTmp, err = jsoniter.Marshal(bidRequest)
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
		}*/

		/*if DeletePrefix(domain) == "dsp_test_hilltopads.com" {
			ChangeSiteIdhilltopTest(req.BidRequest)
		}*/

		client_v_2_5 := getDspHttpClients(domain, s.clients)

		wg.Add(1)
		go func(ctx context.Context, endpoint, domain string, client_v_2_5 *http.Client, timeout time.Duration) {
			defer wg.Done()

			reqCtx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			dspResp, code, err := s.getBidsFromDSPbyHTTP_V_2_5(reqCtx, req.GlobalId, jsonDataTmp, endpoint, client_v_2_5)
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

			if code == http.StatusOK {
				if !filter.GetValueFomCidMap(dspResp, req.SspDomain, domain, *filtersCid) {
					codesCh <- &dspDomainCode{
						domain: domain,
						code:   -77,
					}
					log.Println("GOT GetValueFomCidMap")
					return
				}
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
		}(ctx, endpoint, domain, client_v_2_5, timeout)
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
	bg, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if err := utils.WriteJsonToRedis(bg, redisClient, globalId, constants.BID_RESPONSES_COLUMN, bidRespsData, logged); err != nil {
		log.Printf("failed to WriteJsonToRedis: %v", err)
	}
}
