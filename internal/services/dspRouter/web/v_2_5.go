package dspRouterWeb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/coder"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_4"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	ruleManager *filter.RuleManager
	fileLoader  *filter.FileRuleLoader
	processor   *filter.OptimizedFilterProcessor

	dspEndpoints_v_2_4 []string
	dspEndpoints_v_2_5 []string

	redisClient *redis.Client

	client_v_2_5 *http.Client
	timeout      time.Duration
	// Пулы для снижения аллокаций
	bufferPool sync.Pool

	dspRouterGrpc.UnimplementedDspRouterServiceServer
}

func NewFastHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   100 * time.Millisecond, // TCP соединение за 20ms
			KeepAlive: 30 * time.Second,       // Keep-alive
			DualStack: true,
		}).DialContext,

		// Оптимально для 50ms RTT
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     30 * time.Second,

		// Агрессивные таймауты для 50ms
		TLSHandshakeTimeout:   40 * time.Millisecond,
		ExpectContinueTimeout: 40 * time.Millisecond,
		ResponseHeaderTimeout: 80 * time.Millisecond, // Получение headers за 40ms

		// Оптимизации для скорости
		DisableCompression: true,  // Быстрее без сжатия при 50ms
		ForceAttemptHTTP2:  false, // HTTP/1.1 стабильнее для низких latency
	}

	return &http.Client{
		Transport: transport,
		Timeout:   150 * time.Millisecond, // Общий таймаут чуть больше RTT
	}
}

func NewServer(
	ruleManager *filter.RuleManager,
	fileLoader *filter.FileRuleLoader,
	processor *filter.OptimizedFilterProcessor,
	dspEndpoints_v_2_4,
	dspEndpoints_v_2_5 []string,
	redisClient *redis.Client,
	timeout time.Duration,
	maxParallelRequests int,
) *Server {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if maxParallelRequests <= 0 {
		maxParallelRequests = 64
	}

	client_v_2_5 := NewFastHTTPClient()

	// Глобальный лимит исходящих: консервативно отталкиваемся от
	// количества DSP и локального лимита; при желании — вынеси в конфиг.
	outboundLimit := maxParallelRequests * 2
	if l := len(dspEndpoints_v_2_4); l > 0 && l*maxParallelRequests > outboundLimit {
		outboundLimit = l * maxParallelRequests
	}
	if l := len(dspEndpoints_v_2_5); l > 0 && l*maxParallelRequests > outboundLimit {
		outboundLimit = l * maxParallelRequests
	}
	if outboundLimit < 256 {
		outboundLimit = 256
	}

	return &Server{
		ruleManager:        ruleManager,
		fileLoader:         fileLoader,
		processor:          processor,
		dspEndpoints_v_2_4: dspEndpoints_v_2_4,
		dspEndpoints_v_2_5: dspEndpoints_v_2_5,
		redisClient:        redisClient,
		client_v_2_5:       client_v_2_5,
		timeout:            timeout,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, 2048))
			},
		},
	}
}

type dspDomainResp[T ortb_V2_5.BidResponse | ortb_V2_4.BidResponse] struct {
	endpoint string
	resp     *T
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
			funcErr = status.Errorf(codes.Internal, err.Error())
		}
	}()

	sspDomain := req.BidRequest.GetSspDomain()
	req.BidRequest.SspDomain = nil

	jsonData, err := jsoniter.Marshal(req.BidRequest)
	if err != nil {
		newErr := fmt.Errorf("Can not marshal in GetBids_V_2_5 because got uknown error: %w", err)

		grpcCode := codes.Unknown

		st, ok := status.FromError(err)
		if !ok {
			grpcCode = st.Code()
			newErr = fmt.Errorf("Can not marshal in GetBids_V_2_5 because got error: %w", st.Err())
		}

		return nil, status.Errorf(grpcCode, newErr.Error())
	}

	var (
		wg sync.WaitGroup
	)

	responsesCh := make(chan *dspDomainResp[ortb_V2_5.BidResponse], len(s.dspEndpoints_v_2_4))

	// Запускаем все DSP параллельно
	for _, endpoint := range s.dspEndpoints_v_2_5 {
		if !s.processor.ProcessRequestForDSPV25(endpoint, req.BidRequest).Allowed {
			log.Println("Gor DSP filter")
			continue
		}
		wg.Add(1)
		go func(endpoint string) {
			defer wg.Done()

			dspResp, err := s.getBidsFromDSPbyHTTP_V_2_5_Optimized(reqCtx, jsonData, endpoint)
			if err != nil {
				log.Printf("Cannot getBidsFromDSPbyHTTP_V_2_5_Optimized: %w", err)
			}

			// Фильтрация ответа SPP
			if !s.processor.ProcessResponseForSPPV25(sspDomain, dspResp).Allowed {
				log.Printf("Gor SSP filter, domain %s", endpoint)
				return
			}

			for i := range dspResp.Seatbid {
				if dspResp.Seatbid[i] != nil {
					for j := range dspResp.Seatbid[i].Bid {
						if dspResp.Seatbid[i].Bid[j].Adid == nil {
							adid := coder.AdmToAdidCompact(*dspResp.Seatbid[i].Bid[j].Adm)
							dspResp.Seatbid[i].Bid[j].Adid = &adid
						}
					}
				}
			}

			if dspResp != nil {
				responsesCh <- &dspDomainResp[ortb_V2_5.BidResponse]{
					endpoint: endpoint,
					resp:     dspResp,
				}
			}
		}(endpoint)
	}

	// Ждем завершения и закрываем каналы
	go func() {
		wg.Wait()
		close(responsesCh)
	}()

	// Собираем результаты
	responses := make(map[string]*ortb_V2_5.BidResponse, len(s.dspEndpoints_v_2_5))

	for responsesCh != nil {
		select {
		case r, ok := <-responsesCh:
			if !ok {
				responsesCh = nil
			} else {
				responses[r.endpoint] = r.resp
			}
		}
	}

	writeMetadataToRedis(ctx, s.redisClient, req.GlobalId, responses)

	return &dspRouterGrpc.DspRouterResponse_V2_5{
		BidRequest:   req.BidRequest,
		BidResponses: responses,
		GlobalId:     req.GlobalId,
	}, nil
}

func (s *Server) getBidsFromDSPbyHTTP_V_2_5_Optimized(ctx context.Context, jsonData []byte, dspEndpoint string) (
	ddr *ortb_V2_5.BidResponse, err error) {
	// Пул буферов — как в v2.4
	buf := s.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Write(jsonData)
	defer s.bufferPool.Put(buf)

	req, err := http.NewRequestWithContext(ctx, "POST", dspEndpoint, buf)
	if err != nil {
		return nil, fmt.Errorf("Create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")

	log.Println("GOT req inner")
	t := time.Now()
	resp, err := s.client_v_2_5.Do(req)
	log.Println("%v", time.Since(t))
	if err != nil {
		return nil, fmt.Errorf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		var grpcResp ortb_V2_5.BidResponse
		dec := jsoniter.NewDecoder(resp.Body) // без лишних аллокаций
		if err := dec.Decode(&grpcResp); err != nil {
			return nil, fmt.Errorf("decode: %v", err)
		}
		return &grpcResp, nil
	}
	return nil, nil
}

func writeMetadataToRedis[T ortb_V2_5.BidResponse | ortb_V2_4.BidResponse](ctx context.Context, redisClient *redis.Client, globalId string, data map[string]*T) {
	if len(data) == 0 {
		return
	}
	bidRespsData, err := json.Marshal(data)
	if err != nil {
		log.Printf("failed to marshal data: %v", err)
		return
	}
	bgCtx := context.Background()
	if err := utils.WriteJsonToRedis(bgCtx, redisClient, globalId, constants.BID_RESPONSES_COLUMN, bidRespsData); err != nil {
		log.Printf("failed to WriteJsonToRedis: %v", err)
	}
}
