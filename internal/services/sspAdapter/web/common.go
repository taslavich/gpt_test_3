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
	redisClient *redis.Client,
	timeout time.Duration,
) {
	input := r.Context().Value(httpin.Input).(*admNurlBurlRequest)

	if err := utils.WriteStringToRedis(ctx, redisClient, input.GlobalId, constants.ADM_COLUMN, constants.TRUE); err != nil {
		log.Printf("failed to WriteStringToRedis ADM in getAdm: %w", err)
	}

	decodedURL, err := url.QueryUnescape(input.DspURL)
	if err != nil {
		log.Printf("in getAdm Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(decodedURL)
	if err != nil {
		log.Printf("in getAdm Failed to proxy win notice to DSP %s, globalID: %s, error: %w",
			decodedURL,
			input.GlobalId,
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		log.Printf("in getAdm DSP %s returned error for win notice: %d", decodedURL, resp.StatusCode)
		w.WriteHeader(resp.StatusCode)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getNurl(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClient *redis.Client,
	timeout time.Duration,
) {
	input := r.Context().Value(httpin.Input).(*admNurlBurlRequest)

	if err := utils.WriteStringToRedis(ctx, redisClient, input.GlobalId, constants.NURL_COLUMN, constants.TRUE); err != nil {
		log.Printf("failed to WriteStringToRedis SUCCESS in getNurl: %w", err)
	}

	decodedURL, err := url.QueryUnescape(input.DspURL)
	if err != nil {
		log.Printf("in getNurl Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(decodedURL)
	if err != nil {
		log.Printf("in getNurl Failed to proxy win notice to DSP %s, globalID: %s, error: %w",
			decodedURL,
			input.GlobalId,
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		log.Printf("in getNurl DSP %s returned error for win notice: %d", decodedURL, resp.StatusCode)
		w.WriteHeader(resp.StatusCode)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getBurl(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	timeout time.Duration,
) {
	input := r.Context().Value(httpin.Input).(*admNurlBurlRequest)

	decodedURL, err := url.QueryUnescape(input.DspURL)
	if err != nil {
		log.Printf("in getBurl Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(decodedURL)
	if err != nil {
		log.Printf(
			"in getBurl Failed to proxy billable event to DSP %s, globalID: %s, error: %w",
			decodedURL,
			input.GlobalId,
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		log.Printf("in getBurl DSP %s returned error for billable event: %d", decodedURL, resp.StatusCode)
		w.WriteHeader(resp.StatusCode)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getHealth(
	w http.ResponseWriter,
) {
	w.WriteHeader(http.StatusNoContent)
}
