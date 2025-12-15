package sppAdapterWeb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/ggicci/httpin"
	"github.com/google/uuid"
	grpcRuntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/geoBadIp"
	orchestratorProto "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"google.golang.org/grpc/status"
)

func checkRegion(countryISO string) error {
	allowed := []string{"RU", "US", "AR", "CN", "VN", "BD", "AZ", "IN", "ZA", "DE", "TH", "UA", "GB", "MY", "PK", "TR", "KZ", "UZ"}
	for _, region := range allowed {
		if countryISO == region {
			return nil
		}
	}

	return fmt.Errorf("region %s not allowed", countryISO)
}

func shouldPass(counter *uint64) bool {
	return atomic.AddUint64(counter, 1)%100 < 5
}

func postBid_V2_5(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClient *redis.Client,
	isBadIp func(ipStr string) (bool, error),
	getCountryISO func(ipStr string) (string, uint32, error),
	orchestratorClient orchestratorProto.OrchestratorServiceClient,
	timeout time.Duration,
	sspFeeds map[string]string,
	sspMainstreamFeeds map[string]string,
	counter *uint64,
	typic string,
) {
	var input *postBidRequest_V2_5
	defer func() {
		if r := recover(); r != nil {
			var payloadInfo string
			if input != nil && input.Payload != nil {
				payloadInfo = fmt.Sprintf("%+v", input.Payload)
			} else {
				payloadInfo = "nil payload"
			}
			err := fmt.Errorf("Recovered from panic in postBid_V2_5: %v, req: %s, stack %s", r, payloadInfo, string(debug.Stack()))
			log.Printf(err.Error())
			http.Error(w, "", http.StatusInternalServerError)
		}
	}()
	input = r.Context().Value(httpin.Input).(*postBidRequest_V2_5)

	var ssp_domain string
	var ok bool
	switch typic {
	case ADULT:
		ssp_domain, ok = sspFeeds[input.Feed]
	case MAINSTREAM:
		ssp_domain, ok = sspMainstreamFeeds[input.Feed]
	}
	if !ok {
		err := fmt.Errorf("Busy")
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if input == nil || input.Payload == nil {
		err := fmt.Errorf("Invalid request: payload is nil or missing")
		log.Print(err.Error(), r.RemoteAddr)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if input.Payload.Device == nil {
		err := fmt.Errorf(
			"There is no device object",
		)
		log.Print(err.Error(), r.RemoteAddr)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	device := input.Payload.BidRequest.Device

	if device.Ip == nil && device.Ipv6 == nil {
		err := fmt.Errorf(
			"There is no device ip",
		)
		log.Printf("error: %s, feed: %s,  ipv4: %s, ipv6: %s", err.Error(), input.Feed, device.GetIp(), device.GetIpv6())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return

	} else if device.Ip == nil && device.Ipv6 != nil {
		input.Payload.BidRequest.Device.Ip = device.Ipv6
	}

	bad, err := isBadIp(input.Payload.BidRequest.Device.GetIp())
	if err != nil && bad == false {
		err := fmt.Errorf(
			"There an server error while isBadIp: %w",
			err,
		)
		log.Print(err.Error(), r.RemoteAddr)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if err != nil && bad == true {
		err := fmt.Errorf(
			"Ip is bad: %w",
			err,
		)
		log.Print(err.Error(), r.RemoteAddr)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	countryISO, cityId, err := getCountryISO(input.Payload.BidRequest.Device.GetIp())
	if errors.As(err, geoBadIp.BadIpFormatError) {
		err := fmt.Errorf(
			"Bad format: %w",
			err,
		)
		log.Print(err.Error(), r.RemoteAddr)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	} else if errors.As(err, geoBadIp.InnerLookupIpError) {
		err := fmt.Errorf(
			"There an server error while getCountryISO: %w",
			err,
		)
		log.Print(err.Error(), r.RemoteAddr)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	/*if err := checkRegion(countryISO); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}*/

	if input.Payload.BidRequest.Cur == nil {
		input.Payload.BidRequest.Cur = append(input.Payload.BidRequest.Cur, "USD")
	}

	logged := shouldPass(counter)

	globalId := uuid.New().String()

	bidReqData, err := json.Marshal(input.Payload)
	if err != nil {
		log.Printf("failed to marshal JSON in postBid_V2_5: %w", err)
	}

	if err := utils.WriteStringToRedis(ctx, redisClient, globalId, constants.TYPIC_COLUMN, typic, logged); err != nil {
		log.Printf("failed to WriteStringToRedis Domain in postBid_V2_5: %w", err)
	}

	if err := utils.WriteStringToRedis(ctx, redisClient, globalId, constants.SPP_DOMAIN_COLUMN, ssp_domain, logged); err != nil {
		log.Printf("failed to WriteStringToRedis Domain in postBid_V2_5: %w", err)
	}

	if err := utils.WriteJsonToRedis(ctx, redisClient, globalId, constants.BID_REQUEST_COLUMN, bidReqData, logged); err != nil {
		log.Printf("failed to WriteJsonToRedis Bid Request in postBid_V2_5: %w", err)
	}

	if err := utils.WriteStringToRedis(ctx, redisClient, globalId, constants.GEO_COLUMN, countryISO, logged); err != nil {
		log.Printf("failed to WriteStringToRedis Geo in postBid_V2_5: %w", err)
	}

	if err := utils.WriteUint32ToRedis(ctx, redisClient, globalId, constants.CITY_ID_COLUMN, cityId, logged); err != nil {
		log.Printf("failed to WriteStringToRedis CITY_ID_COLUMN in postBid_V2_5: %w", err)
	}

	if err := utils.WriteStringToRedis(ctx, redisClient, globalId, constants.ADM_COLUMN, constants.FALSE, logged); err != nil {
		log.Printf("failed to WriteStringToRedis ADM in postBid_V2_5: %w", err)
	}

	if err := utils.WriteStringToRedis(ctx, redisClient, globalId, constants.TIMESTAMP_COLUMN, time.Now().UTC().Format("2006-01-02 15:04:05.000"), logged); err != nil {
		log.Printf("failed to WriteJsonToRedis TimeStamp in postBid_V2_5: %w", err)
	}

	if countryISO != "" {
		if input.Payload.BidRequest.Device.Geo == nil {
			input.Payload.BidRequest.Device.Geo = &ortb_V2_5.Geo{
				Country: &countryISO,
			}
		} else {
			input.Payload.BidRequest.Device.Geo.Country = &countryISO
		}
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := orchestratorClient.GetWinnerBid_V2_5(
		reqCtx,
		&orchestratorProto.OrchestratorRequest_V2_5{
			BidRequest: input.Payload.BidRequest,
			GlobalId:   globalId,
			SspDomain:  ssp_domain,
			Logged:     logged,
			Typic:      typic,
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
	statusCode := http.StatusOK
	if len(res.BidResponse.Seatbid[0].Bid) == 0 {
		w.WriteHeader(http.StatusNoContent) // ← ПРАВИЛЬНО
		return
	}

	if err = rnr.JSON(w, statusCode, postBidResponse_V2_5{
		BidResponse: res.BidResponse,
	}); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func getWorkStatus(
	w http.ResponseWriter,
	workAdl,
	workMc *bool,
) {
	if err := rnr.JSON(w, http.StatusOK, getWorkStatusResponse{
		WorkAdl: *workAdl,
		WorkMc:  *workMc,
	}); err != nil {
		log.Printf("Cannot make HTTP response back in getWorkStatus: %v\n", err)
	}
}

func putWorkStatus(
	w http.ResponseWriter,
	r *http.Request,
	workAdl,
	workMc *bool,
) {
	input := r.Context().Value(httpin.Input).(*putWorkStatusRequest)

	var work bool
	switch input.Typic {
	case ADULT:
		*workAdl = input.Work
		work = *workAdl
	case MAINSTREAM:
		*workMc = input.Work
		work = *workMc
	default:
		http.Error(w, "Typic is no here", http.StatusBadRequest)
	}

	if err := rnr.Text(w, http.StatusOK, fmt.Sprintf("Changed to %v", work)); err != nil {
		log.Printf("Cannot make HTTP response back in putWorkStatus: %v\n", err)
	}
}
