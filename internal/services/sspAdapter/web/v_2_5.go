package sppAdapterWeb

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/ggicci/httpin"
	"github.com/google/uuid"
	grpcRuntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/geoBadIp"
	orchestratorProto "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/ua"
	"google.golang.org/grpc/status"
)

func shouldPass(counter *uint64) bool {
	//return atomic.AddUint64(counter, 1)%100 < 1
	return false
}

func postBid_V2_5(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClients []*redis.Client,
	isBadIp func(ipStr string) (bool, error),
	getCountryISO func(ipStr string) (string, uint32, error),
	orchestratorClient orchestratorProto.OrchestratorServiceClient,
	timeout time.Duration,
	sspFeeds map[string]string,
	counter *uint64,
	typic string,
	format string,
	siteIdsAndDomains *utils.SiteIdsAndDomains,
	geoToLang geoBadIp.GeoToLang,
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
			log.Print(err.Error())
			http.Error(w, "", http.StatusInternalServerError)
		}
	}()
	input = r.Context().Value(httpin.Input).(*postBidRequest_V2_5)

	var ssp_domain string
	var ok bool

	ssp_domain, ok = sspFeeds[input.Feed]
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
		log.Printf("error: %s, feed: %s,  ipv4: %s, ipv6: %s", err.Error(), ssp_domain, device.GetIp(), device.GetIpv6())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return

	} else if device.Ip == nil && device.Ipv6 != nil {
		input.Payload.BidRequest.Device.Ip = device.Ipv6
	}

	if device.Ua == nil {
		err := fmt.Errorf(
			"There is no device ua",
		)
		log.Printf("error: %s, feed: %s,  ua: %s", err.Error(), ssp_domain, device.GetUa())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return

	}

	if input.Payload.Site == nil {
		err := fmt.Errorf(
			"There is no site object",
		)
		log.Print(err.Error(), r.RemoteAddr)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if input.Payload.Site.Id == nil {
		err := fmt.Errorf(
			"There is no site id",
		)
		log.Printf("error: %s, feed: %s,  site id: %s", err.Error(), ssp_domain, input.Payload.Site.GetId())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return

	}

	//////////////////////////////////////////////////////

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

	var at int32 = 1
	if input.Payload.BidRequest.At == nil {
		input.Payload.BidRequest.At = &at
	}

	if input.Payload.BidRequest.User == nil {
		id := uuid.New().String()
		input.Payload.BidRequest.User = &ortb_V2_5.User{
			Id: &id,
		}
	} else if input.Payload.BidRequest.User.Id == nil {
		id := uuid.New().String()
		input.Payload.BidRequest.User.Id = &id
	}

	if input.Payload.BidRequest.Site != nil {
		if input.Payload.BidRequest.Site.Id != nil {
			if input.Payload.BidRequest.Site.Domain == nil {
				domain := siteIdsAndDomains.GenerateDomain(*input.Payload.BidRequest.Site.Id)
				input.Payload.BidRequest.Site.Domain = &domain
			}
		}
	}

	if input.Payload.BidRequest.Site != nil && input.Payload.BidRequest.Site.Id != nil && input.Payload.BidRequest.Imp != nil {
		for i := range input.Payload.BidRequest.Imp {
			if input.Payload.BidRequest.Imp[i] != nil {
				input.Payload.BidRequest.Imp[i].Ext = &ortb_V2_5.Imp_Ext{
					Subid: input.Payload.BidRequest.Site.Id,
				}
			}
		}
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

	var lang string
	if lang, ok = geoToLang[input.Payload.BidRequest.Device.Geo.GetCountry()]; !ok {
		lang = geoToLang["DEFAULT"]
	}

	input.Payload.BidRequest.Device.Language = &lang

	siteId := input.Payload.Site.GetId()
	siteDomain := input.Payload.Site.GetDomain()

	logged := shouldPass(counter)

	uaFileds := ua.ParseUA(input.Payload.Device.GetUa())

	impIdUuid := make(map[string]string, len(input.Payload.Imp))

	for i := range input.Payload.Imp {
		globalId := uuid.New().String()
		impIdUuid[input.Payload.Imp[i].GetId()] = globalId
		if err := utils.WriteStatsOrtb(
			ctx,
			redisClients,
			globalId,
			logged,
			format,
			typic,
			ssp_domain,
			device.GetIp(),
			device.GetIpv6(),
			lang,
			countryISO,
			cityId,
			uaFileds,
			siteId,
			siteDomain,
			float64(input.Payload.Imp[i].GetBidfloor()),
		); err != nil {
			log.Printf("failed to WriteStats in postBid_V2_5: %v", err)
		}
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := orchestratorClient.GetWinnerBid_V2_5(
		reqCtx,
		&orchestratorProto.OrchestratorRequest_V2_5{
			BidRequest: input.Payload.BidRequest,
			SspDomain:  ssp_domain,
			Logged:     logged,
			Typic:      typic,
			Format:     format,
			ImpIdUuid:  impIdUuid,
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
		log.Print(err.Error())
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
