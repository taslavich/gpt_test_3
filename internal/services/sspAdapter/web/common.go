package sppAdapterWeb

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/ggicci/httpin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
)

func getAdm(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClients []*redis.Client,
	redisAdmClient *redis.Client,
	redisSetClicks string,
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
	sspAdapterWorkStatusURL string,
) {
	input := r.Context().Value(httpin.Input).(*admNurlBurlRequest)
	format, ok := constants.CodeToFormat[input.Format]
	if !ok {
		log.Printf("in getAdm invalid format code: %q", input.Format)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	decodedURL, err := url.QueryUnescape(input.DspURL)
	if err != nil {
		log.Printf("in getAdm Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	exists, err := utils.UUIDKeyExistsInRedis(ctx, redisAdmClient, input.GlobalId)
	if err != nil {
		log.Printf("failed to check ADM UUID key %s in url %s in getAdm: %v", input.GlobalId, r.URL.String(), err)
		redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !exists {
		log.Printf("ADM UUID key %s does not exist in url %s in getAdm", input.GlobalId, r.URL.String())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	clickUuid := uuid.New().String()

	if err := utils.WriteClickStats(ctx, redisClients, clickUuid, input.GlobalId, format, true); err != nil {
		log.Printf("failed to WriteClickStats in getAdm: %v", err)
		redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetClicks, clickUuid, true); err != nil {
		log.Printf("failed to add click UUID to Redis set in getAdm: %v", err)
		redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	http.Redirect(w, r, decodedURL, http.StatusFound)
}

func getNurl(
	w http.ResponseWriter,
	r *http.Request,
	nurlClient *http.Client,
) {
	input := r.Context().Value(httpin.Input).(*admNurlBurlRequest)

	decodedURL, err := url.QueryUnescape(input.DspURL)
	if err != nil {
		log.Printf("in getNurl Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	resp, err := nurlClient.Get(decodedURL)
	if err != nil {
		log.Printf("failed to call nurl target: %v", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	defer resp.Body.Close()

	_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if err != nil {
		log.Printf("failed to read nurl target response: %v", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func getBurl(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClients []*redis.Client,
	redisNurlClient *redis.Client,
	redisSetImpressions string,
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
	sspAdapterWorkStatusURL string,
	nurlClient *http.Client,
) {
	input := r.Context().Value(httpin.Input).(*admNurlBurlRequest)
	format, ok := constants.CodeToFormat[input.Format]
	if !ok {
		log.Printf("in getBurl invalid format code: %q", input.Format)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	decodedURL, err := url.QueryUnescape(input.DspURL)
	if err != nil {
		log.Printf("in getBurl Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	exists, err := utils.UUIDKeyExistsInRedis(ctx, redisNurlClient, input.GlobalId)
	if err != nil {
		log.Printf("failed to check BURL UUID key %s in url %s in getBurl: %v", input.GlobalId, r.URL.String(), err)
		redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !exists {
		log.Printf("BURL UUID key %s does not exist in url %s in getBurl", input.GlobalId, r.URL.String())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	impressionsUuid := uuid.New().String()

	if err := utils.WriteImpressionStats(ctx, redisClients, impressionsUuid, input.GlobalId, format, true); err != nil {
		log.Printf("failed to WriteImpressionStats in getBurl: %v", err)
		redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetImpressions, impressionsUuid, true); err != nil {
		log.Printf("failed to add impression UUID to Redis set in getBurl: %v", err)
		redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	resp, err := nurlClient.Get(decodedURL)
	if err != nil {
		log.Printf("failed to call burl target: %v", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	defer resp.Body.Close()

	_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if err != nil {
		log.Printf("failed to read burl target response: %v", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
