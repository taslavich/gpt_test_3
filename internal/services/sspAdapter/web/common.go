package sppAdapterWeb

import (
	"context"
	"log"
	"net/http"
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
	redisClient *redis.Client,
	timeout time.Duration,
) {
	input := r.Context().Value(httpin.Input).(*admNurlRequest)

	if err := utils.WriteStringToRedis(ctx, redisClient, input.GlobalId, constants.ADM_COLUMN, constants.TRUE); err != nil {
		log.Printf("failed to WriteStringToRedis ADM in getAdm: %w", err)
	}

	if err := utils.WriteStringToRedis(ctx, redisClient, input.GlobalId, constants.ADM_IP_COLUMN, r.RemoteAddr); err != nil {
		log.Printf("failed to WriteStringToRedis ADM_IP in getAdm: %w", err)
	}

	decodedURL, err := utils.DecodeWrappedURL(utils.ADM, input.DspURL)
	if err != nil {
		log.Printf("in getAdm Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, decodedURL, http.StatusFound)
}

func getNurl(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClient *redis.Client,
	timeout time.Duration,
) {
	input := r.Context().Value(httpin.Input).(*admNurlRequest)

	decodedURL, err := utils.DecodeWrappedURL(utils.NURL, input.DspURL)
	if err != nil {
		log.Printf("in getNurl Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, decodedURL, http.StatusFound)
}

func getHealth(
	w http.ResponseWriter,
) {
	w.WriteHeader(http.StatusNoContent)
}
