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
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/billing"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/emergency"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

type CallbackDeps struct {
	Winner    *billing.WinnerStore
	Billing   *billing.Store
	Outbox    *outbox.Store
	Emergency *emergency.Controller
}

func getAdm(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClients []*redis.Client,
	redisAdmClient *redis.Client,
	redisSetClicks string,
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
	sspAdapterWorkStatusURL string,
	deps *CallbackDeps,
) {
	input := r.Context().Value(httpin.Input).(*admRequest)
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
	own := false
	if deps != nil && deps.Winner != nil {
		winner, found, err := deps.Winner.Lookup(ctx, input.GlobalId)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if found {
			own = true
			if format != constants.IPP || winner.Format != format {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			eventID := uuid.NewString()
			_, err := deps.Billing.Apply(ctx, billing.Event{EventID: eventID, UserID: winner.UserID, CampaignID: winner.CampaignID, Format: winner.Format, Price: winner.Price})
			if err != nil {
				rec := outbox.Record{EventID: eventID, UserID: winner.UserID, CampaignID: winner.CampaignID, Format: winner.Format, Price: winner.Price, Source: "adm"}
				saveErr := deps.Outbox.Save(rec)
				if deps.Emergency != nil {
					_ = deps.Emergency.StopAndNotify(ctx, "ADM billing failed: "+err.Error())
				}
				if saveErr != nil {
					log.Printf("critical: failed to save billing outbox: %v", saveErr)
				}
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
	}

	if !own {
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
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	nurlClient *http.Client,
	redisClients []*redis.Client,
	redisNurlClient *redis.Client,
	redisSetImpressions string,
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
	sspAdapterWorkStatusURL string,
) {
	input := r.Context().Value(httpin.Input).(*nurlRequest)

	if input.Ssp_Domain == "adl_pb.com" {
		format, ok := constants.CodeToFormat[input.Format]
		if !ok {
			log.Printf("in getNurl invalid format code: %q", input.Format)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		exists, err := utils.UUIDKeyExistsInRedis(ctx, redisNurlClient, input.GlobalId)
		if err != nil {
			log.Printf("failed to check NURL UUID key %s in url %s in getNurl: %v", input.GlobalId, r.URL.String(), err)
			redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !exists {
			log.Printf("NURL UUID key %s does not exist in url %s in getNurl", input.GlobalId, r.URL.String())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		impressionsUuid := uuid.New().String()
		if err := utils.WriteImpressionStats(ctx, redisClients, impressionsUuid, input.GlobalId, format, true); err != nil {
			log.Printf("failed to WriteImpressionStats in getNurl: %v", err)
			redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetImpressions, impressionsUuid, true); err != nil {
			log.Printf("failed to add impression UUID to Redis set in getNurl: %v", err)
			redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	}

	decodedURL, err := url.QueryUnescape(input.DspURL)
	if err != nil {
		log.Printf("in getNurl Failed to decode original URL: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	resp, err := nurlClient.Get(decodedURL)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
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
	deps *CallbackDeps,
) {
	input := r.Context().Value(httpin.Input).(*burlRequest)
	format, ok := constants.CodeToFormat[input.Format]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	externalChecked := false
	if deps != nil && deps.Winner != nil {
		winner, found, err := deps.Winner.Lookup(ctx, input.GlobalId)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if found {
			if !(format == constants.NAT || format == constants.BAN || format == constants.POP) || winner.Format != format {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			eventID := uuid.NewString()
			if _, err := deps.Billing.Apply(ctx, billing.Event{EventID: eventID, UserID: winner.UserID, CampaignID: winner.CampaignID, Format: winner.Format, Price: winner.Price}); err != nil {
				rec := outbox.Record{EventID: eventID, UserID: winner.UserID, CampaignID: winner.CampaignID, Format: winner.Format, Price: winner.Price, Source: "burl"}
				saveErr := deps.Outbox.Save(rec)
				if deps.Emergency != nil {
					_ = deps.Emergency.StopAndNotify(ctx, "BURL billing failed: "+err.Error())
				}
				if saveErr != nil {
					log.Printf("critical: failed to save billing outbox: %v", saveErr)
				}
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		} else {
			externalChecked = true
			exists, err := utils.UUIDKeyExistsInRedis(ctx, redisNurlClient, input.GlobalId)
			if err != nil {
				redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if !exists {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
	}
	if !externalChecked && (deps == nil || deps.Winner == nil) {
		exists, err := utils.UUIDKeyExistsInRedis(ctx, redisNurlClient, input.GlobalId)
		if err != nil {
			redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !exists {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	impressionsUuid := uuid.New().String()
	if err := utils.WriteImpressionStats(ctx, redisClients, impressionsUuid, input.GlobalId, format, true); err != nil {
		redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetImpressions, impressionsUuid, true); err != nil {
		redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func getCurl(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClients []*redis.Client,
	redisSetConversions string,
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
	sspAdapterWorkStatusURL string,
) {
	input := r.Context().Value(httpin.Input).(*curlRequest)

	if err := utils.WriteConversionStats(ctx, redisClients, input.ClickUuid, input.Payout, true); err != nil {
		log.Printf("failed to WriteConversionStats in getCurl: %v", err)
		redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetConversions, input.ClickUuid, true); err != nil {
		log.Printf("failed to add conversion UUID to Redis set in getCurl: %v", err)
		redisWriteErrorMonitor.RecordForURL(err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
