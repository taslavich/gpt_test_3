package dspRouterWeb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) GetBids_V2_5(
	ctx context.Context,
	req *dspRouterGrpc.DspRouterRequest_V2_5,
) (resp *dspRouterGrpc.DspRouterResponse_V2_5, funcErr error) {

	reqCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	startTime := time.Now()
	defer func() {
		processingTime := time.Since(startTime)
		log.Printf("Request processed in %v", processingTime)
	}()

	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in GetBids_V2_5: %v", r)
			log.Printf(err.Error())
			resp = nil
			funcErr = status.Errorf(codes.Internal, err.Error())
		}
	}()

	// Предварительная сериализация JSON
	jsonData, err := json.Marshal(req.BidRequest)
	if err != nil {
		return nil, fmt.Errorf("Can not marshal in GetBids_V2_5: %w", err)
	}

	var wg sync.WaitGroup
	responsesCh := make(chan *ortb_V2_5.BidResponse, len(s.dspEndpoints_v_2_5))
	dspMetaDataCh := make(chan *DspMetaData, len(s.dspEndpoints_v_2_5))

	// Запускаем все DSP параллельно
	for _, endpoint := range s.dspEndpoints_v_2_5 {
		wg.Add(1)
		go func(endpoint string) {
			defer wg.Done()

			dspFilterStartTime := time.Now()
			// Быстрая фильтрация DSP
			if !s.processor.ProcessRequestForDSPV25(endpoint, req.BidRequest).Allowed {
				return
			}
			dspFilterEndTime := time.Since(dspFilterStartTime)
			log.Printf("Dsp filter processed in %v", dspFilterEndTime)

			reqStartTime := time.Now()
			// HTTP запрос к DSP
			dspResp, code, errMsg := s.getBidsFromDSPbyHTTP_V_2_5_Optimized(reqCtx, jsonData, endpoint)

			reqEndTime := time.Since(reqStartTime)
			log.Printf("Http Request processed in %v", reqEndTime)

			// Отправляем метаданные
			meta := s.metaPool.Get().(*DspMetaData)
			meta.DspEndpoint = endpoint
			meta.Code = code
			meta.ErrMsg = errMsg
			dspMetaDataCh <- meta

			sppFilterStartTime := time.Now()
			// Фильтрация ответа SPP
			if dspResp != nil && s.processor.ProcessResponseForSPPV25(req.SppEndpoint, dspResp).Allowed {
				responsesCh <- dspResp
			}
			sppFilterEndTime := time.Since(sppFilterStartTime)
			log.Printf("Spp filter processed in %v", sppFilterEndTime)
		}(endpoint)
	}

	// Ждем завершения в отдельной горутине и закрываем каналы
	go func() {
		wg.Wait()
		close(responsesCh)
		close(dspMetaDataCh)
	}()

	// Собираем результаты параллельно с ожиданием
	responses := make([]*ortb_V2_5.BidResponse, 0, len(s.dspEndpoints_v_2_5))
	dspMetaData := make([]*DspMetaData, 0, len(s.dspEndpoints_v_2_5))

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

	// Асинхронная запись в Redis
	go s.writeMetadataToRedis(ctx, req.GlobalId, dspMetaData)

	return &dspRouterGrpc.DspRouterResponse_V2_5{
		BidRequest:   req.BidRequest,
		BidResponses: responses,
		GlobalId:     req.GlobalId,
	}, nil
}

func (s *Server) getBidsFromDSPbyHTTP_V_2_5_Optimized(ctx context.Context, jsonData []byte, dspEndpoint string) (
	br *ortb_V2_5.BidResponse, code int, errMsg string) {

	// Используем пул буферов
	buf := s.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Write(jsonData)
	defer s.bufferPool.Put(buf)

	httpReqInnerStartTime := time.Now()
	req, err := http.NewRequestWithContext(ctx, "POST", dspEndpoint, buf)
	if err != nil {
		return nil, 0, fmt.Sprintf("Create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client_v_2_5.Do(req)
	if err != nil {
		return nil, 0, fmt.Sprintf("Request failed: %v", err)
	}
	defer resp.Body.Close()
	httpReqInnerEndTime := time.Since(httpReqInnerStartTime)
	log.Printf("Http Inner processed in %v", httpReqInnerEndTime)

	readingStartTime := time.Now()
	// Быстрое чтение тела
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Sprintf("Read failed: %v", err)
	}

	// Парсим только при успешном статусе
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		grpcResp := &ortb_V2_5.BidResponse{}
		if err := json.Unmarshal(body, grpcResp); err != nil {
			return nil, resp.StatusCode, fmt.Sprintf("Unmarshal failed: %v", err)
		}
		return grpcResp, resp.StatusCode, ""
	}
	readingEndTime := time.Since(readingStartTime)
	log.Printf("Reading processed in %v", readingEndTime)

	return nil, resp.StatusCode, string(body)
}
