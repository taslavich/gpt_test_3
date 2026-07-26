package sppAdapterWeb

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/ggicci/httpin"
	"github.com/google/uuid"
	grpcRuntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/geoBadIp"
	orchestratorProto "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/ua"
	"google.golang.org/grpc/status"
)

func shouldPass(counter *uint64) bool {
	//return atomic.AddUint64(counter, 1)%100 < 100
	//return false
	return true
}

func writeStatsOrtbAndAddToSet(
	ctx context.Context,
	redisClients []*redis.Client,
	redisSetOrtb string,
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
	sspAdapterWorkStatusURL string,
	globalId string,
	logged bool,
	format string,
	typic string,
	sspDomain string,
	ip string,
	ipv6 string,
	lang string,
	countryISO string,
	cityId uint32,
	code int32,
	uaFields ua.UAFields,
	siteId string,
	siteDomain string,
	bidFloor float64,
) bool {
	if err := utils.WriteStatsOrtb(
		ctx,
		redisClients,
		globalId,
		logged,
		format,
		typic,
		sspDomain,
		ip,
		ipv6,
		lang,
		countryISO,
		cityId,
		code,
		uaFields,
		siteId,
		siteDomain,
		bidFloor,
	); err != nil {
		log.Printf("failed to WriteStats in postBid_V2_5: %v", err)
		recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
		return false
	}

	if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetOrtb, globalId, logged); err != nil {
		log.Printf("failed to add ORTB UUID to Redis set in postBid_V2_5: %v", err)
		recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
		return false
	}

	return true
}

func removeFailedImpBidsFromResponse(bidResponse *ortb_V2_5.BidResponse, failedImpIds []string) bool {
	if bidResponse == nil || len(failedImpIds) == 0 {
		return false
	}

	failed := make(map[string]struct{}, len(failedImpIds))
	for _, impID := range failedImpIds {
		failed[impID] = struct{}{}
	}

	removed := false
	for _, seatBid := range bidResponse.Seatbid {
		if seatBid == nil {
			continue
		}

		bids := seatBid.Bid[:0]
		for _, bid := range seatBid.Bid {
			if bid == nil {
				continue
			}
			if _, ok := failed[bid.GetImpid()]; ok {
				removed = true
				continue
			}
			bids = append(bids, bid)
		}
		seatBid.Bid = bids
	}

	return removed
}

func bidResponseHasBids(bidResponse *ortb_V2_5.BidResponse) bool {
	if bidResponse == nil {
		return false
	}
	for _, seatBid := range bidResponse.Seatbid {
		if seatBid != nil && len(seatBid.Bid) > 0 {
			return true
		}
	}
	return false
}
func postBid_V2_5(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClients []*redis.Client,
	redisSetOrtb string,
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
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
	sspAdapterWorkStatusURL string,
	ipLimitStore *IPLimitStore,
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
	rawInput := r.Context().Value(httpin.Input)
	parsedInput, ok := rawInput.(*postBidRequest_V2_5)
	if !ok || parsedInput == nil {
		err := fmt.Errorf("invalid request: parsed input is missing")
		log.Print(err.Error(), r.RemoteAddr)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	input = parsedInput

	if input.Payload == nil || input.Payload.BidRequest == nil {
		err := fmt.Errorf("invalid request: bid request payload is nil or missing")
		log.Print(err.Error(), r.RemoteAddr)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if orchestratorClient == nil || isBadIp == nil || getCountryISO == nil || siteIdsAndDomains == nil || ipLimitStore == nil {
		log.Print("SSP adapter request dependencies are not initialized")
		http.Error(w, "service dependencies are unavailable", http.StatusServiceUnavailable)
		return
	}

	ssp_domain, ok := sspFeeds[input.Feed]
	if !ok {
		err := fmt.Errorf("Busy")
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	traceRequest := utils.ShouldTraceSSPDomain(ssp_domain)
	if traceRequest {
		log.Printf(
			"[SPP][BID_RECEIVED] request_id=%q path=%q method=%q feed=%q ssp_domain=%q format=%q traffic_type=%q impressions=%d remote_addr=%q",
			input.Payload.BidRequest.GetId(),
			r.URL.Path,
			r.Method,
			input.Feed,
			ssp_domain,
			format,
			typic,
			len(input.Payload.BidRequest.GetImp()),
			r.RemoteAddr,
		)
		log.Printf(
			"[SPP][FEED_RESOLVED] request_id=%q feed=%q ssp_domain=%q format=%q traffic_type=%q",
			input.Payload.BidRequest.GetId(),
			input.Feed,
			ssp_domain,
			format,
			typic,
		)
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

	}

	if device.Ip != nil && ipLimitStore.ContainsIPv4(device.GetIp()) {
		err := fmt.Errorf("Ip is limited: %s", device.GetIp())
		log.Printf("error: %s, feed: %s", err.Error(), ssp_domain)

		logged := shouldPass(counter)

		globalId := uuid.New().String()
		if !writeStatsOrtbAndAddToSet(
			ctx,
			redisClients,
			redisSetOrtb,
			redisWriteErrorMonitor,
			sspAdapterWorkStatusURL,
			globalId,
			logged,
			format,
			typic,
			ssp_domain,
			device.GetIp(),
			device.GetIpv6(),
			"",
			"",
			0,
			701,
			ua.UAFields{},
			"",
			"",
			0,
		) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if device.Ipv6 != nil && ipLimitStore.ContainsIPv6(device.GetIpv6()) {
		err := fmt.Errorf("Ipv6 is limited: %s", device.GetIpv6())
		log.Printf("error: %s, feed: %s", err.Error(), ssp_domain)

		logged := shouldPass(counter)

		globalId := uuid.New().String()
		if !writeStatsOrtbAndAddToSet(
			ctx,
			redisClients,
			redisSetOrtb,
			redisWriteErrorMonitor,
			sspAdapterWorkStatusURL,
			globalId,
			logged,
			format,
			typic,
			ssp_domain,
			device.GetIp(),
			device.GetIpv6(),
			"",
			"",
			0,
			701,
			ua.UAFields{},
			"",
			"",
			0,
		) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if device.Ua == nil {
		err := fmt.Errorf(
			"There is no device ua",
		)
		log.Printf("error: %s, feed: %s,  ua: %s", err.Error(), ssp_domain, device.GetUa())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return

	}

	if len(device.GetUa()) < 20 {
		err := fmt.Errorf("User-Agent is too short: %s", device.GetUa())
		log.Printf("error: %s, feed: %s", err.Error(), ssp_domain)

		logged := shouldPass(counter)

		globalId := uuid.New().String()
		if !writeStatsOrtbAndAddToSet(
			ctx,
			redisClients,
			redisSetOrtb,
			redisWriteErrorMonitor,
			sspAdapterWorkStatusURL,
			globalId,
			logged,
			format,
			typic,
			ssp_domain,
			device.GetIp(),
			device.GetIpv6(),
			"",
			"",
			0,
			702,
			ua.UAFields{},
			"",
			"",
			0,
		) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		http.Error(w, err.Error(), http.StatusForbidden)
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

	testIp := device.GetIp()
	if device.Ip == nil {
		testIp = device.GetIpv6()
	}
	bad, err := isBadIp(testIp)
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

	countryISO, cityId, err := getCountryISO(testIp)
	if errors.Is(err, geoBadIp.BadIpFormatError) {
		err := fmt.Errorf(
			"Bad format: %w",
			err,
		)
		log.Print(err.Error(), r.RemoteAddr)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	} else if errors.Is(err, geoBadIp.InnerLookupIpError) {
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

	lang := strings.ToLower(strings.TrimSpace(input.Payload.BidRequest.Device.GetLanguage()))
	if lang == "" {
		countryForLanguage := ""
		if input.Payload.BidRequest.Device.Geo != nil {
			countryForLanguage = input.Payload.BidRequest.Device.Geo.GetCountry()
		}
		mappedLang, found := geoToLang[countryForLanguage]
		if !found {
			mappedLang = geoToLang["DEFAULT"]
		}
		lang = mappedLang
		lang = strings.ToLower(strings.TrimSpace(lang))
	}

	input.Payload.BidRequest.Device.Language = &lang

	siteId := input.Payload.Site.GetId()
	siteDomain := input.Payload.Site.GetDomain()

	logged := shouldPass(counter)

	uaFileds := ua.ParseUA(input.Payload.Device.GetUa())

	if len(input.Payload.Imp) == 0 {
		err := fmt.Errorf("invalid request: impressions are missing")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	impIdUuid := make(map[string]string, len(input.Payload.Imp))
	uuidBidFloor := make(map[string]float32, len(input.Payload.Imp))

	for _, imp := range input.Payload.Imp {
		if imp == nil || imp.GetId() == "" {
			err := fmt.Errorf("invalid request: impression id is missing")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, duplicate := impIdUuid[imp.GetId()]; duplicate {
			err := fmt.Errorf("invalid request: duplicate impression id %q", imp.GetId())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		globalId := uuid.New().String()
		impIdUuid[imp.GetId()] = globalId
		uuidBidFloor[globalId] = imp.GetBidfloor()
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if traceRequest {
		log.Printf(
			"[SPP][ORCHESTRATOR_CALL_START] request_id=%q ssp_domain=%q format=%q traffic_type=%q timeout_ms=%d impressions=%d",
			input.Payload.BidRequest.GetId(),
			ssp_domain,
			format,
			typic,
			timeout.Milliseconds(),
			len(input.Payload.BidRequest.GetImp()),
		)
	}
	orchestratorStartedAt := time.Now()
	res, err := orchestratorClient.GetWinnerBid_V2_5(
		reqCtx,
		&orchestratorProto.OrchestratorRequest_V2_5{
			BidRequest:  input.Payload.BidRequest,
			SspDomain:   ssp_domain,
			Logged:      logged,
			Typic:       typic,
			Format:      format,
			TrafficType: typic,
			ImpIdUuid:   impIdUuid,
			SspUrl:      sspAdapterWorkStatusURL,
		},
	)
	if err != nil {
		if traceRequest {
			log.Printf(
				"[SPP][ORCHESTRATOR_CALL_ERROR] request_id=%q ssp_domain=%q format=%q duration_ms=%d error=%v",
				input.Payload.BidRequest.GetId(),
				ssp_domain,
				format,
				time.Since(orchestratorStartedAt).Milliseconds(),
				err,
			)
		}
		httpErr := fmt.Errorf("Cannot GetWinnerBid because got error")

		httpCode := http.StatusInternalServerError

		if st, ok := status.FromError(err); ok {
			httpCode = grpcRuntime.HTTPStatusFromCode(st.Code())
		}

		http.Error(w, httpErr.Error(), httpCode)
		log.Print(err.Error())
		return
	}
	if res == nil {
		if traceRequest {
			log.Printf(
				"[SPP][ORCHESTRATOR_CALL_DONE] request_id=%q ssp_domain=%q format=%q duration_ms=%d response_nil=true",
				input.Payload.BidRequest.GetId(),
				ssp_domain,
				format,
				time.Since(orchestratorStartedAt).Milliseconds(),
			)
		}
		log.Print("orchestrator returned a nil response")
		http.Error(w, "orchestrator returned an empty response", http.StatusServiceUnavailable)
		return
	}

	if traceRequest {
		log.Printf(
			"[SPP][ORCHESTRATOR_CALL_DONE] request_id=%q ssp_domain=%q format=%q duration_ms=%d response_nil=false code=%d rekl=%t has_bid_response=%t failed_imp_ids=%d",
			input.Payload.BidRequest.GetId(),
			ssp_domain,
			format,
			time.Since(orchestratorStartedAt).Milliseconds(),
			res.GetCode(),
			res.GetRekl(),
			res.GetBidResponse() != nil,
			len(res.GetFailedImpIds()),
		)
	}

	if res.Code == 703 {
		err := fmt.Errorf("Site id blocked all")
		log.Printf("error: %s, feed: %s", err.Error(), ssp_domain)

		for _, uuid := range impIdUuid {
			if !writeStatsOrtbAndAddToSet(
				ctx,
				redisClients,
				redisSetOrtb,
				redisWriteErrorMonitor,
				sspAdapterWorkStatusURL,
				uuid,
				logged,
				format,
				typic,
				ssp_domain,
				device.GetIp(),
				device.GetIpv6(),
				lang,
				countryISO,
				cityId,
				703,
				uaFileds,
				siteId,
				siteDomain,
				float64(uuidBidFloor[uuid]),
			) {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}

		w.WriteHeader(http.StatusForbidden)
		return
	}

	if res.Code == http.StatusNoContent {
		for _, uuid := range impIdUuid {
			if !writeStatsOrtbAndAddToSet(
				ctx,
				redisClients,
				redisSetOrtb,
				redisWriteErrorMonitor,
				sspAdapterWorkStatusURL,
				uuid,
				logged,
				format,
				typic,
				ssp_domain,
				device.GetIp(),
				device.GetIpv6(),
				lang,
				countryISO,
				cityId,
				204,
				uaFileds,
				siteId,
				siteDomain,
				float64(uuidBidFloor[uuid]),
			) {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}

		w.WriteHeader(http.StatusNoContent)
		return
	}

	failedImpIds := append([]string(nil), res.GetFailedImpIds()...)
	for impID, uuid := range res.GetImpIdUuidClone() {
		if !writeStatsOrtbAndAddToSet(
			ctx,
			redisClients,
			redisSetOrtb,
			redisWriteErrorMonitor,
			sspAdapterWorkStatusURL,
			uuid,
			logged,
			format,
			typic,
			ssp_domain,
			device.GetIp(),
			device.GetIpv6(),
			lang,
			countryISO,
			cityId,
			int32(http.StatusOK),
			uaFileds,
			siteId,
			siteDomain,
			float64(uuidBidFloor[uuid]),
		) {
			failedImpIds = append(failedImpIds, impID)
		}
	}

	removeFailedImpBidsFromResponse(res.BidResponse, failedImpIds)
	if !bidResponseHasBids(res.BidResponse) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err = rnr.JSON(w, http.StatusOK, postBidResponse_V2_5{
		BidResponse: res.BidResponse,
	}); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func getWorkStatus(
	w http.ResponseWriter,
	workStatus *WorkStatus,
) {
	if workStatus == nil {
		http.Error(w, "work status is unavailable", http.StatusServiceUnavailable)
		return
	}
	popAdult, _ := workStatus.Get(PostBid_POP_ADL_V_2_5_URL)
	popMainstream, _ := workStatus.Get(PostBid_POP_MC_V_2_5_URL)
	ippAdult, _ := workStatus.Get(PostBid_IPP_ADL_V_2_5_URL)
	ippMainstream, _ := workStatus.Get(PostBid_IPP_MC_V_2_5_URL)
	banAdult, _ := workStatus.Get(PostBid_BAN_ADL_V_2_5_URL)
	banMainstream, _ := workStatus.Get(PostBid_BAN_MC_V_2_5_URL)
	natAdult, _ := workStatus.Get(PostBid_NAT_ADL_V_2_5_URL)
	natMainstream, _ := workStatus.Get(PostBid_NAT_MC_V_2_5_URL)

	if err := rnr.JSON(w, http.StatusOK, getWorkStatusResponse{
		PopAdult:      popAdult,
		PopMainstream: popMainstream,
		IppAdult:      ippAdult,
		IppMainstream: ippMainstream,
		BanAdult:      banAdult,
		BanMainstream: banMainstream,
		NatAdult:      natAdult,
		NatMainstream: natMainstream,
	}); err != nil {
		log.Printf("Cannot make HTTP response back in getWorkStatus: %v\n", err)
	}
}

func putWorkStatus(
	w http.ResponseWriter,
	r *http.Request,
	workStatus *WorkStatus,
	streamURL string,
) {
	input, ok := r.Context().Value(httpin.Input).(*putWorkStatusRequest)
	if !ok || input == nil || workStatus == nil {
		http.Error(w, "invalid work status request", http.StatusBadRequest)
		return
	}

	if err := workStatus.Set(streamURL, input.Work); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	if err := rnr.Text(w, http.StatusOK, fmt.Sprintf("Changed %s to %v", streamURL, input.Work)); err != nil {
		log.Printf("Cannot make HTTP response back in putWorkStatus: %v\n", err)
	}
}

func putAllWorkStatus(
	w http.ResponseWriter,
	r *http.Request,
	workStatus *WorkStatus,
) {
	input, ok := r.Context().Value(httpin.Input).(*putWorkStatusRequest)
	if !ok || input == nil || workStatus == nil {
		http.Error(w, "invalid work status request", http.StatusBadRequest)
		return
	}
	workStatus.SetAll(input.Work)

	if err := rnr.Text(w, http.StatusOK, fmt.Sprintf("Changed all ORTB streams to %v", input.Work)); err != nil {
		log.Printf("Cannot make HTTP response back in putAllWorkStatus: %v\n", err)
	}
}
