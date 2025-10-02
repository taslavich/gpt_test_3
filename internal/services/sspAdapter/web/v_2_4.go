package sppAdapterWeb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ggicci/httpin"
	"github.com/google/uuid"
	grpcRuntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/redis/go-redis/v9"
	"github.com/unrolled/render"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/geoBadIp"
	orchestratorProto "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_4"
	"google.golang.org/grpc/status"
)

var rnr = render.New(render.Options{
	StreamingJSON: true,
})

func postBid_V2_4(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClient *redis.Client,
	isBadIp func(ipStr string) (bool, error),
	getCountryISO func(ipStr string) (string, error),
	orchestratorClient orchestratorProto.OrchestratorServiceClient,
	timeout time.Duration,
) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in postBid_V2_4: %v", r)
			log.Printf(err.Error())
			http.Error(w, "", http.StatusInternalServerError)
		}
	}()
	input := r.Context().Value(httpin.Input).(*postBidRequest_V2_4)

	if input.Payload.Device == nil {
		err := fmt.Errorf(
			"There is no device object",
		)
		log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	deviceIp := input.Payload.Device.Ip
	if deviceIp == nil {
		err := fmt.Errorf(
			"There is no device ip",
		)
		log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	bad, err := isBadIp(*deviceIp)
	if err != nil && bad == false {
		err := fmt.Errorf(
			"There an server error while isBadIp: %w",
			err,
		)
		log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if err != nil && bad == true {
		err := fmt.Errorf(
			"Ip is bad: %w",
			err,
		)
		log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	countryISO, err := getCountryISO(*deviceIp)
	if errors.As(err, geoBadIp.BadIpFormatError) {
		err := fmt.Errorf(
			"Bad format: %w",
			err,
		)
		log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	} else if errors.As(err, geoBadIp.InnerLookupIpError) {
		err := fmt.Errorf(
			"There an server error while getCountryISO: %w",
			err,
		)
		log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	globalId := uuid.New().String()

	bidReqData, err := json.Marshal(input.Payload)
	if err != nil {
		fmt.Printf("failed to marshal JSON in postBid_V2_4: %w", err)
	}

	if input.Payload.Device.Geo == nil {
		input.Payload.Device.Geo = &ortb_V2_4.Geo{
			Country: &countryISO,
		}
	} else {
		input.Payload.Device.Geo.Country = &countryISO
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := orchestratorClient.GetWinnerBid_V2_4(
		reqCtx,
		&orchestratorProto.OrchestratorRequest_V2_4{
			BidRequest:  input.Payload,
			SppEndpoint: r.Host,
			GlobalId:    globalId,
		},
	)
	if err != nil {
		httpErr := fmt.Errorf("Cannot GetWinnerBid because got error:")

		httpCode := http.StatusInternalServerError

		st, ok := status.FromError(err)
		if !ok {
			httpCode = grpcRuntime.HTTPStatusFromCode(st.Code())
		}

		http.Error(w, httpErr.Error(), httpCode)
		log.Printf(err.Error())
		return
	}
	asyncWriteBidDataToRedis(ctx, timeout, redisClient, globalId, bidReqData, countryISO)

	statusCode := http.StatusOK
	if len(res.BidResponse.Seatbid.Bid) == 0 {
		statusCode = http.StatusNoContent
	}

	if err = rnr.JSON(w, statusCode, postBidResponse_V2_4{
		BidResponse: res.BidResponse,
	}); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}
