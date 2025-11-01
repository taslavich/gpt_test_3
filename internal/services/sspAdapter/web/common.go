package sppAdapterWeb

import (
	"context"
	"encoding/base64"
	"log"
	"net/http"
	"net/url"

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
) {
	input := r.Context().Value(httpin.Input).(*admNurlRequest)

	decodedURL, err := base64.RawURLEncoding.DecodeString(input.DspURL)
	if err != nil {
		log.Printf("in getAdm Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	endpoint := string(decodedURL)

	if _, err := url.ParseRequestURI(endpoint); err != nil {
		log.Printf("in getAdm Invalid redirect URL: --%s-- , %v", endpoint, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := utils.WriteStringToRedis(ctx, redisClient, input.GlobalId, constants.ADM_COLUMN, constants.TRUE); err != nil {
		log.Printf("failed to WriteStringToRedis ADM in getAdm: %w", err)
	}

	if err := utils.WriteStringToRedis(ctx, redisClient, input.GlobalId, constants.ADM_IP_COLUMN, r.RemoteAddr); err != nil {
		log.Printf("failed to WriteStringToRedis ADM_IP in getAdm: %w", err)
	}

	// Так надежнее - покрываем все возможные случаи
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	http.Redirect(w, r, endpoint, http.StatusTemporaryRedirect)
}

func getNurl(
	w http.ResponseWriter,
	r *http.Request,
) {
	input := r.Context().Value(httpin.Input).(*admNurlRequest)

	decodedURL, err := url.QueryUnescape(input.DspURL)
	if err != nil {
		log.Printf("in getNurl Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, decodedURL, http.StatusFound)
}
