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

	// Пулы для снижения аллокаций
	bufferPool sync.Pool
	metaPool   sync.Pool

	dspRouterGrpc.UnimplementedDspRouterServiceServer
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
) *Server {
	// Оптимизированный транспорт с балансом между скоростью и надежностью
	transport := &http.Transport{
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   20,
		MaxConnsPerHost:       10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   3 * time.Millisecond,
		ExpectContinueTimeout: 1 * time.Millisecond,
		ResponseHeaderTimeout: 10 * time.Millisecond,
		DisableCompression:    true,
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Millisecond,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		ForceAttemptHTTP2:      false,
		MaxResponseHeaderBytes: 2048, // Уменьшено
		ReadBufferSize:         4096,
		WriteBufferSize:        4096,
	}

	client_v_2_4 := &http.Client{
		Transport: transport,
		Timeout:   20 * time.Millisecond, // Уменьшено
	}

	client_v_2_5 := &http.Client{
		Transport: transport,
		Timeout:   20 * time.Millisecond,
	}

	return &Server{
		ruleManager:        ruleManager,
		fileLoader:         fileLoader,
		processor:          processor,
		dspConfigPath:      dspConfigPath,
		sppConfigPath:      sppConfigPath,
		dspEndpoints_v_2_4: dspEndpoints_v_2_4,
		dspEndpoints_v_2_5: dspEndpoints_v_2_5,
		redisClient:        redisClient,
		client_v_2_4:       client_v_2_4,
		client_v_2_5:       client_v_2_5,
		timeout:            timeout,
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
	}
}

func (s *Server) GetBids_V2_4(
	ctx context.Context,
	req *dspRouterGrpc.DspRouterRequest_V2_4,
) (resp *dspRouterGrpc.DspRouterResponse_V2_4, funcErr error) {

	reqCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

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

	var wg sync.WaitGroup
	responsesCh := make(chan *ortb_V2_4.BidResponse, len(s.dspEndpoints_v_2_4))
	dspMetaDataCh := make(chan *DspMetaData, len(s.dspEndpoints_v_2_4))

	// Запускаем все DSP параллельно
	for _, endpoint := range s.dspEndpoints_v_2_4 {
		wg.Add(1)
		go func(endpoint string) {
			defer wg.Done()

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
	dspMetaData := make([]*DspMetaData, 0, len(s.dspEndpoints_v_2_4))

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
				dspMetaData = append(dspMetaData, meta)
				// Возвращаем в пул после использования
				defer s.metaPool.Put(meta)
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

	resp, err := s.client_v_2_4.Do(req)
	if err != nil {
		return nil, 0, fmt.Sprintf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Быстрое чтение тела
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Sprintf("Read failed: %v", err)
	}

	// Парсим только при успешном статусе
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		grpcResp := &ortb_V2_4.BidResponse{}
		if err := json.Unmarshal(body, grpcResp); err != nil {
			return nil, resp.StatusCode, fmt.Sprintf("Unmarshal failed: %v", err)
		}
		return grpcResp, resp.StatusCode, ""
	}

	return nil, resp.StatusCode, string(body)
}

func (s *Server) writeMetadataToRedis(ctx context.Context, globalId string, metadata []*DspMetaData) {
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
