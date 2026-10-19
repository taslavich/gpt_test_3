package bidEngineWeb

import (
	"context"
	"errors"
	"fmt"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/callback"
	shard "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/redis/go-redis/v9"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	ProfitPercent          float32
	redisClients           []*redis.Client
	redisAdmClient         *redis.Client
	callbackADV            *redis.Client
	redisBurlClient        *redis.Client
	redisUUIDKeyTTL        time.Duration
	redisSetOrtb           string
	timeout                time.Duration
	percentRoutes          *types.FormatPercentRoutesV25
	admDomain              string
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor

	// Legacy shared-map function is retained so existing in-process tests/callers
	// can keep constructing Server directly. Production uses the route-aware
	// function below.
	GetWinnerBidInternal_V_2_5 func(
		ctx context.Context,
		req *bidEngineGrpc.BidEngineRequest_V2_5,
		profitPercent float32,
		ImpIdUuid map[string]string,
		percentMapAdult *map[string]map[string]map[string]*types.PercentAndBidfloor,
		percentMapMainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,
		logged bool,
		typic string,
		admDomain string,
	) (*ortb_V2_5.BidResponse, clickhouse_types.UuidImpBidResponse, []string, []string)

	GetWinnerBidInternalWithRoutes_V_2_5 func(
		ctx context.Context,
		req *bidEngineGrpc.BidEngineRequest_V2_5,
		profitPercent float32,
		ImpIdUuid map[string]string,
		percentRoutes *types.FormatPercentRoutesV25,
		logged bool,
		typic string,
		admDomain string,
	) (*ortb_V2_5.BidResponse, clickhouse_types.UuidImpBidResponse, []string, []string)

	pb.BidEngineServiceServer
}

func NewServer(
	ProfitPercent float32,
	redisClients []*redis.Client,
	redisAdmClient *redis.Client,
	redisBurlClient *redis.Client,
	redisUUIDKeyTTL time.Duration,
	redisSetOrtb string,
	GetWinnerBidInternalWithRoutes_V_2_5 func(
		ctx context.Context,
		req *bidEngineGrpc.BidEngineRequest_V2_5,
		profitPercent float32,
		ImpIdUuid map[string]string,
		percentRoutes *types.FormatPercentRoutesV25,
		logged bool,
		typic string,
		admDomain string,
	) (*ortb_V2_5.BidResponse, clickhouse_types.UuidImpBidResponse, []string, []string),
	percentRoutes *types.FormatPercentRoutesV25,

	admDomain string,
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
	callbackADV *redis.Client,
) *Server {
	return &Server{
		ProfitPercent:                        ProfitPercent,
		callbackADV:                          callbackADV,
		redisClients:                         redisClients,
		redisAdmClient:                       redisAdmClient,
		redisBurlClient:                      redisBurlClient,
		redisUUIDKeyTTL:                      redisUUIDKeyTTL,
		redisSetOrtb:                         redisSetOrtb,
		GetWinnerBidInternalWithRoutes_V_2_5: GetWinnerBidInternalWithRoutes_V_2_5,
		percentRoutes:                        percentRoutes,
		admDomain:                            admDomain,
		redisWriteErrorMonitor:               redisWriteErrorMonitor,
	}
}

func (s *Server) GetWinnerBid_V2_5(
	ctx context.Context,
	req *bidEngineGrpc.BidEngineRequest_V2_5,
) (
	resp *bidEngineGrpc.BidEngineResponse_V2_5,
	funcErr error,
) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in GetWinnerBid_V2_5: %v, %s", r, string(debug.Stack()))
			log.Printf("Error: %v", err.Error())
			resp = nil
			funcErr = status.Error(codes.Internal, err.Error())
		}
	}()
	if s == nil {
		return nil, status.Error(codes.Unavailable, "bid engine service is unavailable")
	}
	if req == nil || req.GetBidRequest() == nil {
		return nil, status.Error(codes.InvalidArgument, "bid engine request is empty")
	}
	if s.GetWinnerBidInternalWithRoutes_V_2_5 == nil && s.GetWinnerBidInternal_V_2_5 == nil {
		return nil, status.Error(codes.Internal, "bid engine auction function is nil")
	}

	ctx, callbackRecords := callback.Collecting(ctx)

	// ADV and DSP winners are finalized by one per-impression path. Production
	// selects FORMAT x traffic percent maps; the legacy branch keeps the old shared maps.
	var bidResponse *ortb_V2_5.BidResponse
	var clickhouseBid clickhouse_types.UuidImpBidResponse
	var burlUUIDs, admUUIDs []string
	if s.GetWinnerBidInternalWithRoutes_V_2_5 != nil {
		bidResponse, clickhouseBid, burlUUIDs, admUUIDs = s.GetWinnerBidInternalWithRoutes_V_2_5(
			ctx, req, s.ProfitPercent, req.ImpIdUuid, s.percentRoutes, req.Logged, req.Typic, s.admDomain,
		)
	} else {
		bidResponse, clickhouseBid, burlUUIDs, admUUIDs = s.GetWinnerBidInternal_V_2_5(
			ctx, req, s.ProfitPercent, req.ImpIdUuid, nil, nil, req.Logged, req.Typic, s.admDomain,
		)
	}

	// UUID authorization and private callback context are committed together below.
	_ = burlUUIDs
	_ = admUUIDs
	callbackFailed := map[string]bool{}
	if bidResponse != nil {
		for _, seat := range bidResponse.GetSeatbid() {
			if seat == nil {
				continue
			}
			kept := seat.Bid[:0]
			for _, bid := range seat.GetBid() {
				if bid == nil {
					continue
				}
				id := req.GetImpIdUuid()[bid.GetImpid()]
				record, ok := callbackRecords[id]
				var err error
				if !ok {
					err = errors.New("callback context was not collected")
				} else {
					// The existing ORTB hash is the trusted publisher attribution authority.
					var c *redis.Client
					c, _, err = shard.SelectShard(s.redisClients, id)
					if err == nil {
						var owner string
						owner, err = c.HGet(ctx, id, "pub_user_id").Result()
						if errors.Is(err, redis.Nil) {
							err = nil
						}
						record.PublisherID = owner
					}
					target := s.redisAdmClient
					stats := clickhouseBid[id]
					adv := stats != nil && stats.WinDspDomain != nil && *stats.WinDspDomain == "adv"
					if adv {
						record.WinnerType = "adv"
						target = s.callbackADV
					}
					if err == nil {
						err = callback.Save(ctx, target, id, record, s.redisUUIDKeyTTL, adv)
					}
					if err == nil && !adv && s.redisBurlClient != s.redisAdmClient {
						err = callback.Save(ctx, s.redisBurlClient, id, record, s.redisUUIDKeyTTL, false)
					}
				}
				if err != nil {
					callbackFailed[bid.GetImpid()] = true
					log.Printf("callback context persistence failed uuid=%s: %v", id, err)
					continue
				}
				kept = append(kept, bid)
			}
			seat.Bid = kept
		}
	}

	advCandidateImpIDs := bidResponseImpIDSet(req.GetReadyBidResponse())
	finalImpIDs := bidResponseImpIDSet(bidResponse)
	finalADVImpIDs := make(map[string]struct{})
	for impID, uuid := range req.GetImpIdUuid() {
		if _, finalized := finalImpIDs[impID]; !finalized {
			continue
		}
		winnerStats := clickhouseBid[uuid]
		if winnerStats == nil || winnerStats.WinDspDomain == nil || *winnerStats.WinDspDomain != "adv" {
			continue
		}
		finalADVImpIDs[impID] = struct{}{}
	}
	allADV := allRequestImpressionsInSet(req.GetBidRequest(), finalADVImpIDs)

	impIdUuidClone := make(map[string]string, len(req.ImpIdUuid))
	winnerUsers := make(map[string]string, len(finalADVImpIDs))
	failedImpIds := make([]string, 0)
	failedSet := make(map[string]struct{})
	appendFailed := func(impID string) {
		if impID == "" {
			return
		}
		if _, exists := failedSet[impID]; exists {
			return
		}
		failedSet[impID] = struct{}{}
		failedImpIds = append(failedImpIds, impID)
	}

	// Legacy ADV handling marked a selected ADV impression as failed when its
	// UUID mapping was missing. Keep that behavior even though the common stats
	// loop is keyed by ImpIdUuid.
	for impID := range advCandidateImpIDs {
		if _, exists := req.GetImpIdUuid()[impID]; !exists {
			appendFailed(impID)
		}
	}

	for impID, uuid := range req.GetImpIdUuid() {
		if callbackFailed[impID] {
			appendFailed(impID)
			continue
		}
		_, isADV := finalADVImpIDs[impID]
		if allADV && !isADV {
			continue
		}
		if isADV {
			// Preserve legacy ADV behavior: only a successfully finalized ADV bid
			// gets winner stats and enters ImpIdUuidClone. Invalid ADV winners are
			// failed rather than silently converted into a DSP winner here.
			if _, finalized := finalImpIDs[impID]; !finalized {
				appendFailed(impID)
				continue
			}
			if err := utils.WriteWinStats(ctx, s.redisClients, uuid, clickhouseBid[uuid], req.Logged); err != nil {
				log.Printf("failed to WriteWinStats for ADV bid: %v", err)
				if s.redisWriteErrorMonitor != nil {
					s.redisWriteErrorMonitor.RecordForURL(err, req.SspUrl)
				}
				appendFailed(impID)
				continue
			}
			impIdUuidClone[impID] = uuid
			winnerUsers[impID] = req.GetWinnerUserIds()[impID]
			continue
		}

		// Preserve the legacy DSP path: winner stats (including the empty winner
		// record when no DSP bid survives) are written for every fallback imp.
		if err := utils.WriteWinStats(ctx, s.redisClients, uuid, clickhouseBid[uuid], req.Logged); err != nil {
			log.Printf("failed to WriteJsonToRedis Bid BID_RESPONSE_WINNER in GetWinnerBidInternal: %v", err)
			if s.redisWriteErrorMonitor != nil {
				s.redisWriteErrorMonitor.RecordForURL(err, req.SspUrl)
			}
			appendFailed(impID)
			continue
		}
		impIdUuidClone[impID] = uuid
	}

	if len(impIdUuidClone) == 0 {
		response := &bidEngineGrpc.BidEngineResponse_V2_5{
			BidResponse:   nil,
			Code:          http.StatusNoContent,
			Rekl:          allADV,
			WinnerUserIds: winnerUsers,
		}
		if allADV {
			response.FailedImpIds = failedImpIds
		}
		return response, nil
	}

	if !bidResponseHasBids(bidResponse) {
		return &bidEngineGrpc.BidEngineResponse_V2_5{
			BidResponse:   nil,
			Code:          http.StatusNoContent,
			Rekl:          allADV,
			WinnerUserIds: winnerUsers,
		}, nil
	}

	return &bidEngineGrpc.BidEngineResponse_V2_5{
		BidResponse:    bidResponse,
		Code:           http.StatusOK,
		FailedImpIds:   failedImpIds,
		ImpIdUuidClone: impIdUuidClone,
		Rekl:           allADV,
		WinnerUserIds:  winnerUsers,
	}, nil
}

func bidResponseImpIDSet(response *ortb_V2_5.BidResponse) map[string]struct{} {
	result := make(map[string]struct{})
	if response == nil {
		return result
	}
	for _, seat := range response.GetSeatbid() {
		if seat == nil {
			continue
		}
		for _, bid := range seat.GetBid() {
			if bid == nil || bid.GetImpid() == "" {
				continue
			}
			result[bid.GetImpid()] = struct{}{}
		}
	}
	return result
}

func allRequestImpressionsInSet(request *ortb_V2_5.BidRequest, impIDs map[string]struct{}) bool {
	if request == nil || len(request.GetImp()) == 0 {
		return false
	}
	seen := 0
	for _, imp := range request.GetImp() {
		if imp == nil || imp.GetId() == "" {
			continue
		}
		seen++
		if _, exists := impIDs[imp.GetId()]; !exists {
			return false
		}
	}
	return seen > 0
}

func bidResponseHasBids(response *ortb_V2_5.BidResponse) bool {
	if response == nil {
		return false
	}
	for _, seat := range response.GetSeatbid() {
		if seat != nil && len(seat.GetBid()) > 0 {
			return true
		}
	}
	return false
}
