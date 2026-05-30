package sppAdapterWeb

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/ggicci/httpin"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
)

func getAdm(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClients []*redis.Client,
	redisSetClicks string,
) {
	input := r.Context().Value(httpin.Input).(*admNurlRequest)

	decodedURL, err := url.QueryUnescape(input.DspURL)
	if err != nil {
		log.Printf("in getAdm Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := utils.WriteStringToRedis(ctx, redisClients, input.GlobalId, constants.EVENT_TIME_CLICKS_COLUMN, time.Now().UTC().Format("2006-01-02 15:04:05.000"), true); err != nil {
		log.Printf("failed to WriteStringToRedis EVENT_TIME_CLICKS_COLUMN in getAdm: %v", err)
	} else if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetClicks, input.GlobalId, true); err != nil {
		log.Printf("failed to add click UUID to Redis set in getAdm: %v", err)
	}

	http.Redirect(w, r, decodedURL, http.StatusFound)
}

func getNurl(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClients []*redis.Client,
	redisSetImpressions string,
) {
	input := r.Context().Value(httpin.Input).(*admNurlRequest)

	decodedURL, err := url.QueryUnescape(input.DspURL)
	if err != nil {
		log.Printf("in getNurl Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := utils.WriteStringToRedis(ctx, redisClients, input.GlobalId, constants.EVENT_TIME_IMPRESSIONS_COLUMN, time.Now().UTC().Format("2006-01-02 15:04:05.000"), true); err != nil {
		log.Printf("failed to WriteStringToRedis EVENT_TIME_IMPRESSIONS_COLUMN in getAdm: %v", err)
	} else if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetImpressions, input.GlobalId, true); err != nil {
		log.Printf("failed to add impression UUID to Redis set in getNurl: %v", err)
	}

	http.Redirect(w, r, decodedURL, http.StatusFound)
}
