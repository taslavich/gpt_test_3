package dspRouterWeb

import (
	"context"
	"fmt"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
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
	defer func() {
		cancel()
		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in GetBids_V2_5: %v", r)
			resp = nil
			funcErr = status.Errorf(codes.Internal, err.Error())
		}
	}()

	jsonData, err := jsoniter.Marshal(req.BidRequest)
	if err != nil {
		return nil, fmt.Errorf("Can not marshal in GetBids_V_2_5: %w", err)
	}

	var (
		wg sync.WaitGroup
	)

	responsesCh := make(chan *ortb_V2_5.BidResponse, len(s.dspEndpoints_v_2_5))
	dspMetaDataCh := make(chan *DspMetaData, len(s.dspEndpoints_v_2_5))

	// Запускаем все DSP параллельно
	for _, endpoint := range s.dspEndpoints_v_2_5 {
		if !s.processor.ProcessRequestForDSPV25(endpoint, req.BidRequest).Allowed {
			continue
		}
		wg.Add(1)
		go func(endpoint string) {
			defer wg.Done()

			dspResp, code, errMsg := s.getBidsFromDSPbyHTTP_V_2_5_Optimized(reqCtx, jsonData, endpoint)

			// Отправляем метаданные
			meta := s.metaPool.Get().(*DspMetaData)
			meta.DspEndpoint = endpoint
			meta.Code = code
			meta.ErrMsg = errMsg
			dspMetaDataCh <- meta

			// Фильтрация ответа SPP
			if dspResp != nil && s.processor.ProcessResponseForSPPV25(req.SppEndpoint, dspResp).Allowed {
				responsesCh <- dspResp
			}
		}(endpoint)
	}

	// Ждем завершения и закрываем каналы
	go func() {
		wg.Wait()
		close(responsesCh)
		close(dspMetaDataCh)
	}()

	// Собираем результаты
	responses := make([]*ortb_V2_5.BidResponse, 0, len(s.dspEndpoints_v_2_5))
	dspMetaData := make([]DspMetaData, 0, len(s.dspEndpoints_v_2_5))

	for responsesCh != nil || dspMetaDataCh != nil {
		select {
		case r, ok := <-responsesCh:
			if !ok {
				responsesCh = nil
			} else {
				responses = append(responses, r)
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
	/*
		// Пул буферов — как в v2.4
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

		t := time.Now()
		resp, err := s.client_v_2_5.Do(req)
		log.Println("%v", time.Since(t))
		if err != nil {
			return nil, 0, fmt.Errorf("Request failed: %v", err).Error()
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusNoContent:
			return nil, resp.StatusCode, ""
		case http.StatusOK:
			var grpcResp ortb_V2_5.BidResponse
			dec := jsoniter.NewDecoder(resp.Body) // без лишних аллокаций
			if err := dec.Decode(&grpcResp); err != nil {
				return nil, resp.StatusCode, fmt.Sprintf("decode: %v", err)
			}
			return &grpcResp, resp.StatusCode, ""
		default:
			return nil, resp.StatusCode, "NULL"
		}*/

	time.Sleep(20 * time.Millisecond)
	return &ortb_V2_5.BidResponse{}, 0, ""
}
