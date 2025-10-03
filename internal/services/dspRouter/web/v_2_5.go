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

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) GetBids_V2_5(
	ctx context.Context,
	req *dspRouterGrpc.DspRouterRequest_V2_5,
) (
	resp *dspRouterGrpc.DspRouterResponse_V2_5,
	funcErr error,
) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		fmt.Printf("Execution time in ms: %d ms\n", elapsed.Milliseconds())

		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in GetBids_V2_5: %v", r)
			log.Printf(err.Error())

			grpcCode := codes.Internal

			resp = nil
			funcErr = status.Errorf(grpcCode, err.Error())
		}
	}()
	originReq := req

	var bdmu sync.Mutex
	var wg sync.WaitGroup

	dspEndpointLen := len(s.dspEndpoints_v_2_5)
	responsesCh := make(chan *ortb_V2_5.BidResponse, dspEndpointLen)
	dspMetaDataCh := make(chan *DspMetaData, dspEndpointLen)

	jsonData, err := json.Marshal(req.BidRequest)
	if err != nil {
		return nil, fmt.Errorf("Can not marshal in GetBids_V2_5: %w", err)
	}

	for i := range s.dspEndpoints_v_2_5 {
		wg.Add(1)
		endpoint := s.dspEndpoints_v_2_5[i]
		go func(
			mu *sync.Mutex,
			req *dspRouterGrpc.DspRouterRequest_V2_5,
			endpoint string,
		) {
			defer wg.Done()
			dspFilterStart := time.Now()
			s.requestMutex.RLock()
			filterResult := s.processor.ProcessRequestForDSPV25(endpoint, req.BidRequest)
			s.requestMutex.RUnlock()
			dspFilterEnd := time.Since(dspFilterStart)
			fmt.Printf("Dsp filter time in ms: %d ms\n", dspFilterEnd.Milliseconds())
			if !filterResult.Allowed {
				return
			}

			reqStart := time.Now()
			resp, code, errMsg := s.getBidsFromDSPbyHTTP_V_2_5(ctx, jsonData, endpoint)
			reqEnd := time.Since(reqStart)
			fmt.Printf("Request time in ms: %d ms\n", reqEnd.Milliseconds())

			dspMetaDataCh <- &DspMetaData{
				DspEndpoint: endpoint,
				Code:        code,
				ErrMsg:      errMsg,
			}

			sppFilterStart := time.Now()
			s.requestMutex.RLock()
			filterRes := s.processor.ProcessResponseForSPPV25(req.SppEndpoint, resp)
			s.requestMutex.RUnlock()
			if !filterRes.Allowed {
				return
			}
			sppFilterEnd := time.Since(sppFilterStart)
			fmt.Printf("Spp filter time in ms: %d ms\n", sppFilterEnd.Milliseconds())

			if resp != nil {
				responsesCh <- resp
			}
		}(
			&bdmu,
			req,
			endpoint,
		)
	}

	wg.Wait()
	close(responsesCh)
	close(dspMetaDataCh)

	massiveStart := time.Now()
	dspMetaData := make([]*DspMetaData, 0)
	for d := range dspMetaDataCh {
		dspMetaData = append(dspMetaData, d)
	}

	bidRespsData, err := json.Marshal(dspMetaData)
	if err != nil {
		fmt.Printf("failed to marshal slice in GetBids_V2_5: %w", err)
	}

	if err := utils.WriteJsonToRedis(ctx, s.redisClient, req.GlobalId, constants.BID_RESPONSES_COLUMN, bidRespsData); err != nil {
		fmt.Printf("failed to WriteJsonToRedis Bid Responses in GetBids_V2_5: %v", err)
	}
	massiveEnd := time.Since(massiveStart)
	fmt.Printf("Massive proccessing time in ms: %d ms\n", massiveEnd.Milliseconds())

	return &dspRouterGrpc.DspRouterResponse_V2_5{
		BidRequest: originReq.BidRequest,
		BidResponses: func() []*ortb_V2_5.BidResponse {
			responses := make([]*ortb_V2_5.BidResponse, 0)
			for resp := range responsesCh {
				responses = append(responses, resp)
			}
			return responses
		}(),
		GlobalId: req.GlobalId,
	}, nil
}

func (s *Server) getBidsFromDSPbyHTTP_V_2_5(ctx context.Context, jsonData []byte, dspEndpoint string) (
	br *ortb_V2_5.BidResponse,
	code int,
	errMsg string,
) {
	req, err := http.NewRequestWithContext(ctx, "POST", dspEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, fmt.Sprintf("Create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Sprintf("Can not post req to dsps in GetBids_V2_5: %w", err)
	}
	defer func() {
		if retErr := resp.Body.Close(); retErr != nil {
			errMsg = fmt.Sprintf(
				"Cannot close resp in GetBids_V2_5: %w",
				retErr,
			)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Sprintf("Read body failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return nil, resp.StatusCode, string(body)
	}

	var grpcResp ortb_V2_5.BidResponse
	if err := json.Unmarshal(body, &grpcResp); err != nil {
		return nil, resp.StatusCode, fmt.Sprintf("Unmarshal failed: %v", err)
	}

	return &grpcResp, resp.StatusCode, ""
}
