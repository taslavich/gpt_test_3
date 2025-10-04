package dspRouterWeb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
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
			log.Printf(err.Error())
			resp = nil
			funcErr = status.Errorf(codes.Internal, err.Error())
		}
	}()

	jsonData, err := jsoniter.Marshal(req.BidRequest)
	if err != nil {
		return nil, fmt.Errorf("Can not marshal in GetBids_V2_5: %w", err)
	}

	var wg sync.WaitGroup
	var sem chan struct{}

	if s.maxParallelRequests > 0 {
		sem = make(chan struct{}, 256)
	}

	responsesCh := make(chan *ortb_V2_5.BidResponse, len(s.dspEndpoints_v_2_5))
	dspMetaDataCh := make(chan *DspMetaData, len(s.dspEndpoints_v_2_5))

	// Запускаем все DSP параллельно
	for _, endpoint := range s.dspEndpoints_v_2_5 {
		wg.Add(1)
		go func(endpoint string) {
			defer wg.Done()
			if sem != nil {
				sem <- struct{}{}
				defer func() {
					<-sem
				}()
			}

			if !s.processor.ProcessRequestForDSPV25(endpoint, req.BidRequest).Allowed {
				return
			}

			t := time.Now()
			dspResp, code, errMsg := s.getBidsFromDSPbyHTTP_V_2_5_Optimized(reqCtx, bytes.NewBuffer(jsonData), endpoint)
			e := time.Since(t).Milliseconds()
			if e > 5 {
				log.Println("%v", e)
			}
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

	// Ждем завершения в отдельной горутине и закрываем каналы
	go func() {
		wg.Wait()
		close(responsesCh)
		close(dspMetaDataCh)
	}()

	// Собираем результаты параллельно с ожиданием
	responses := make([]*ortb_V2_5.BidResponse, 0, len(s.dspEndpoints_v_2_5))
	dspMetaData := make([]DspMetaData, 0, len(s.dspEndpoints_v_2_5))

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

	// Асинхронная запись в Redis
	go s.writeMetadataToRedis(ctx, req.GlobalId, dspMetaData)
	return &dspRouterGrpc.DspRouterResponse_V2_5{
		BidRequest:   req.BidRequest,
		BidResponses: responses,
		GlobalId:     req.GlobalId,
	}, nil
}

func (s *Server) getBidsFromDSPbyHTTP_V_2_5_Optimized(ctx context.Context, buf *bytes.Buffer, dspEndpoint string) (
	br *ortb_V2_5.BidResponse, code int, errMsg string) {

	req, err := http.NewRequestWithContext(ctx, "POST", dspEndpoint, buf)
	if err != nil {
		return nil, 0, fmt.Sprintf("Create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")

	resp, err := s.client_v_2_5.Do(req)
	if err != nil {
		return nil, 0, fmt.Sprintf("Request failed: %v", err)
	}
	//time.Sleep(time.Millisecond * 50)
	//resp := *s.resp
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil, resp.StatusCode, ""
	case http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, resp.StatusCode, fmt.Sprintf("Read body failed: %v", err)
		}
		var grpcResp ortb_V2_5.BidResponse
		if err := jsoniter.Unmarshal(body, &grpcResp); err != nil {
			return nil, resp.StatusCode, fmt.Sprintf("Decode failed: %v", err)
		}
		return &grpcResp, resp.StatusCode, ""
	default:
		return nil, resp.StatusCode, "NULL"
	}
}
