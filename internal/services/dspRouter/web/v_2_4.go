package dspRouterWeb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_4"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type DspMetaData struct {
	DspEndpoint string
	Code        int
	ErrMsg      string
}

type Server struct {
	ruleManager *filter.RuleManager
	fileLoader  *filter.FileRuleLoader
	processor   *filter.OptimizedFilterProcessor

	dspConfigPath string
	sppConfigPath string

	dspEndpoints_v_2_4 []string
	dspEndpoints_v_2_5 []string

	redisClient *redis.Client

	client_v_2_4 *http.Client
	client_v_2_5 *http.Client
	timeout      time.Duration

	maxParallelRequests int
	debug               bool
	slowLogThreshold    time.Duration

	// Пулы для снижения аллокаций
	bufferPool sync.Pool
	metaPool   sync.Pool
	resp       *http.Response

	dspRouterGrpc.UnimplementedDspRouterServiceServer
}

func newHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   300 * time.Millisecond,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        1024,
		MaxIdleConnsPerHost: 256,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 500 * time.Millisecond,
		DisableCompression:  true,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

func NewServer(
	ruleManager *filter.RuleManager,
	fileLoader *filter.FileRuleLoader,
	processor *filter.OptimizedFilterProcessor,
	dspConfigPath string,
	sppConfigPath string,
	dspEndpoints_v_2_4,
	dspEndpoints_v_2_5 []string,
	redisClient *redis.Client,
	timeout time.Duration,
	maxParallelRequests int,
	debug bool,
	resp *http.Response,
) *Server {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	if maxParallelRequests <= 0 {
		maxParallelRequests = 64
	}

	client_v_2_4 := newHTTPClient(timeout)
	client_v_2_5 := newHTTPClient(timeout)

	return &Server{
		ruleManager:         ruleManager,
		fileLoader:          fileLoader,
		processor:           processor,
		dspConfigPath:       dspConfigPath,
		sppConfigPath:       sppConfigPath,
		dspEndpoints_v_2_4:  dspEndpoints_v_2_4,
		dspEndpoints_v_2_5:  dspEndpoints_v_2_5,
		redisClient:         redisClient,
		client_v_2_4:        client_v_2_4,
		client_v_2_5:        client_v_2_5,
		timeout:             timeout,
		maxParallelRequests: maxParallelRequests,
		debug:               debug,
		slowLogThreshold:    50 * time.Millisecond,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, 2048))
			},
		},
		metaPool: sync.Pool{
			New: func() interface{} {
				return &DspMetaData{}
			},
		},
		resp: resp,
	}
}

func (s *Server) shouldLog(duration time.Duration) bool {
	return s.debug || duration >= s.slowLogThreshold
}

func (s *Server) logDuration(label string, start time.Time) {
	duration := time.Since(start)
	if s.shouldLog(duration) {
		log.Printf("%s in %v", label, duration)
	}
}

func (s *Server) logDurationForEndpoint(prefix, endpoint string, start time.Time) {
	duration := time.Since(start)
	if s.shouldLog(duration) {
		log.Printf("%s %s in %v", prefix, endpoint, duration)
	}
}

func (s *Server) GetBids_V2_4(
	ctx context.Context,
	req *dspRouterGrpc.DspRouterRequest_V2_4,
) (resp *dspRouterGrpc.DspRouterResponse_V2_4, funcErr error) {

	reqCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	startTime := time.Now()
	defer func() {
		s.logDuration("GetBids_V2_4", startTime)
	}()

	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in GetBids_V2_4: %v", r)
			log.Printf(err.Error())
			resp = nil
			funcErr = status.Errorf(codes.Internal, err.Error())
		}
	}()

	// Предварительная сериализация JSON
	jsonData, err := json.Marshal(req.BidRequest)
	if err != nil {
		return nil, fmt.Errorf("Can not marshal in GetBids_V2_4: %w", err)
	}

	var (
		wg  sync.WaitGroup
		sem chan struct{}
	)

	if s.maxParallelRequests > 0 {
		sem = make(chan struct{}, s.maxParallelRequests)
	}

	responsesCh := make(chan *ortb_V2_4.BidResponse, len(s.dspEndpoints_v_2_4))
	dspMetaDataCh := make(chan *DspMetaData, len(s.dspEndpoints_v_2_4))

	// Запускаем все DSP параллельно
	for _, endpoint := range s.dspEndpoints_v_2_4 {
		wg.Add(1)
		go func(endpoint string) {
			defer wg.Done()

			if sem != nil {
				sem <- struct{}{}
				defer func() {
					<-sem
				}()
			}

			// Быстрая фильтрация DSP
			if !s.processor.ProcessRequestForDSPV24(endpoint, req.BidRequest).Allowed {
				return
			}

			// HTTP запрос к DSP
			dspResp, code, errMsg := s.getBidsFromDSPbyHTTP_V_2_4_Optimized(reqCtx, jsonData, endpoint)

			// Отправляем метаданные
			meta := s.metaPool.Get().(*DspMetaData)
			meta.DspEndpoint = endpoint
			meta.Code = code
			meta.ErrMsg = errMsg
			dspMetaDataCh <- meta

			// Фильтрация ответа SPP
			if dspResp != nil && s.processor.ProcessResponseForSPPV24(req.SppEndpoint, dspResp).Allowed {
				responsesCh <- dspResp
			}
		}(endpoint)
	}

	// Ждем завершения в отдельной горутине и закрываем каналы
	go func() {
		wg.Wait()
		close(responsesCh)
		close(dspMetaDataCh)
	}()

	// Собираем результаты параллельно с ожиданием
	responses := make([]*ortb_V2_4.BidResponse, 0, len(s.dspEndpoints_v_2_4))
	dspMetaData := make([]DspMetaData, 0, len(s.dspEndpoints_v_2_4))

	// Используем select для параллельного сбора результатов
	for responsesCh != nil || dspMetaDataCh != nil {
		select {
		case resp, ok := <-responsesCh:
			if !ok {
				responsesCh = nil
			} else {
				responses = append(responses, resp)
			}
		case meta, ok := <-dspMetaDataCh:
			if !ok {
				dspMetaDataCh = nil
			} else {
				dspMetaData = append(dspMetaData, DspMetaData{
					DspEndpoint: meta.DspEndpoint,
					Code:        meta.Code,
					ErrMsg:      meta.ErrMsg,
				})
				s.metaPool.Put(meta)
			}
		}
	}

	// Асинхронная запись в Redis чтобы не блокировать ответ
	go s.writeMetadataToRedis(ctx, req.GlobalId, dspMetaData)

	return &dspRouterGrpc.DspRouterResponse_V2_4{
		BidRequest:   req.BidRequest,
		BidResponses: responses,
		GlobalId:     req.GlobalId,
	}, nil
}

func (s *Server) getBidsFromDSPbyHTTP_V_2_4_Optimized(ctx context.Context, jsonData []byte, dspEndpoint string) (
	br *ortb_V2_4.BidResponse, code int, errMsg string) {

	// Используем пул буферов
	buf := s.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Write(jsonData)
	defer s.bufferPool.Put(buf)

	req, err := http.NewRequestWithContext(ctx, "POST", dspEndpoint, buf)
	if err != nil {
		return nil, 0, fmt.Sprintf("Create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")

	httpStart := time.Now()
	resp, err := s.client_v_2_4.Do(req)
	if err != nil {
		return nil, 0, fmt.Sprintf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	s.logDuration("DSP HTTP request v2_4", httpStart)

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil, resp.StatusCode, ""
	case http.StatusOK:
		var grpcResp ortb_V2_4.BidResponse
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&grpcResp); err != nil {
			return nil, resp.StatusCode, fmt.Sprintf("Decode failed: %v", err)
		}
		return &grpcResp, resp.StatusCode, ""
	default:
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		if err != nil {
			return nil, resp.StatusCode, fmt.Sprintf("Read failed: %v", err)
		}
		return nil, resp.StatusCode, string(body)
	}
}

func (s *Server) writeMetadataToRedis(ctx context.Context, globalId string, metadata []DspMetaData) {
	if len(metadata) == 0 {
		return
	}

	bidRespsData, err := json.Marshal(metadata)
	if err != nil {
		log.Printf("failed to marshal metadata: %v", err)
		return
	}

	// Используем background context чтобы не зависеть от основного
	bgCtx := context.Background()
	if err := utils.WriteJsonToRedis(bgCtx, s.redisClient, globalId, constants.BID_RESPONSES_COLUMN, bidRespsData); err != nil {
		log.Printf("failed to WriteJsonToRedis: %v", err)
	}
}
