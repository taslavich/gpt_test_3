package sppAdapterWeb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ggicci/httpin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/billing"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
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
	advBillingStore *billing.Store,
	advOutbox *outbox.Store,
	advControlURLs []string,
) {
	input, ok := r.Context().Value(httpin.Input).(*admRequest)
	if !ok || input == nil {
		http.Error(w, "invalid ADM request", http.StatusBadRequest)
		return
	}
	format, ok := constants.CodeToFormat[input.Format]
	if !ok {
		http.Error(w, "invalid format", http.StatusBadRequest)
		return
	}
	decodedURL, err := url.QueryUnescape(input.DspURL)
	if err != nil || strings.TrimSpace(decodedURL) == "" {
		http.Error(w, "invalid redirect URL", http.StatusBadRequest)
		return
	}
	isADV, err := handleADVCallback(
		r.Context(),
		input.GlobalId,
		format,
		"adm",
		format == constants.IPP,
		advBillingStore,
		advOutbox,
		advControlURLs,
		sspAdapterWorkStatusURL,
		redisWriteErrorMonitor,
	)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if !isADV {
		exists, err := utils.UUIDKeyExistsInRedis(ctx, redisAdmClient, input.GlobalId)
		if err != nil {
			recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if !exists {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	clickUUID := uuid.NewString()
	if err := utils.WriteClickStats(ctx, redisClients, clickUUID, input.GlobalId, format, true); err != nil {
		recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetClicks, clickUUID, true); err != nil {
		recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	redirectURL := decodedURL
	if isADV {
		redirectURL = appendClickID(decodedURL, clickUUID)
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func appendClickID(rawURL, clickID string) string {
	rawURL = strings.TrimSpace(rawURL)
	clickID = strings.TrimSpace(clickID)

	if rawURL == "" || clickID == "" {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		// Не получилось разобрать ссылку — проксируем исходную.
		return rawURL
	}

	clickIDParameter := "click_id=" + url.QueryEscape(clickID)

	if parsed.RawQuery == "" {
		parsed.RawQuery = clickIDParameter
	} else {
		parsed.RawQuery += "&" + clickIDParameter
	}

	return parsed.String()
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
	advBillingStore *billing.Store,
	advOutbox *outbox.Store,
	advControlURLs []string,
) {
	input, ok := r.Context().Value(httpin.Input).(*nurlRequest)
	if !ok || input == nil {
		http.Error(w, "invalid NURL request", http.StatusBadRequest)
		return
	}
	format, ok := constants.CodeToFormat[input.Format]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// ADV has no downstream DSP NURL. Detect a preselected ADV winner by UUID
	// in DB 6 and terminate the callback locally without requiring or calling
	// an embedded url= target.
	isADV, err := handleADVCallback(
		r.Context(),
		input.GlobalId,
		format,
		"nurl",
		false,
		advBillingStore,
		advOutbox,
		advControlURLs,
		sspAdapterWorkStatusURL,
		redisWriteErrorMonitor,
	)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if isADV {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if input.Ssp_Domain == "adl_pb.com" {
		exists, err := utils.UUIDKeyExistsInRedis(ctx, redisNurlClient, input.GlobalId)
		if err != nil {
			recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if !exists {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		impressionUUID := uuid.NewString()
		if err := utils.WriteImpressionStats(ctx, redisClients, impressionUUID, input.GlobalId, format, true); err != nil {
			recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetImpressions, impressionUUID, true); err != nil {
			recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	}
	decodedURL, err := url.QueryUnescape(input.DspURL)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(decodedURL) == "" {
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
	advBillingStore *billing.Store,
	advOutbox *outbox.Store,
	advControlURLs []string,
) {
	input, ok := r.Context().Value(httpin.Input).(*burlRequest)
	if !ok || input == nil {
		http.Error(w, "invalid BURL request", http.StatusBadRequest)
		return
	}
	format, ok := constants.CodeToFormat[input.Format]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	billable := format == constants.NAT || format == constants.BAN || format == constants.POP
	isADV, err := handleADVCallback(
		r.Context(),
		input.GlobalId,
		format,
		"burl",
		billable,
		advBillingStore,
		advOutbox,
		advControlURLs,
		sspAdapterWorkStatusURL,
		redisWriteErrorMonitor,
	)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if !isADV {
		exists, err := utils.UUIDKeyExistsInRedis(ctx, redisNurlClient, input.GlobalId)
		if err != nil {
			recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if !exists {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	impressionUUID := uuid.NewString()
	if err := utils.WriteImpressionStats(ctx, redisClients, impressionUUID, input.GlobalId, format, true); err != nil {
		recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetImpressions, impressionUUID, true); err != nil {
		recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleADVCallback(
	ctx context.Context,
	winnerUUID, format, source string,
	billable bool,
	store *billing.Store,
	outboxStore *outbox.Store,
	controlURLs []string,
	sspAdapterWorkStatusURL string,
	monitor *services.RedisWriteErrorMonitor,
) (bool, error) {
	if store == nil {
		return false, nil
	}
	winner, err := store.ReadWinner(ctx, winnerUUID, format)
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		recordRedisError(monitor, err, sspAdapterWorkStatusURL)
		return false, err
	}
	if !billable {
		return true, nil
	}
	record := outbox.Record{
		EventID: uuid.NewString(), UserID: winner.UserID, CampaignID: winner.CampaignID,
		Price: winner.Price, Format: winner.Format, Source: source, CreatedAt: time.Now().UTC(), Attempts: 1,
	}
	if err := store.Apply(ctx, record); err != nil {
		record.LastError = err.Error()
		record.LastAttemptAt = time.Now().UTC()
		handleADVWriteFailure(err, record, outboxStore, controlURLs, sspAdapterWorkStatusURL, monitor)
		return true, err
	}
	return true, nil
}

func handleADVWriteFailure(redisErr error, record outbox.Record, outboxStore *outbox.Store, controlURLs []string, sspAdapterWorkStatusURL string, monitor *services.RedisWriteErrorMonitor) {
	outboxErr := errors.New("ADV outbox is not initialized")
	if outboxStore != nil {
		outboxErr = outboxStore.Save(record)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	statuses := services.SetADVWorkStatus(stopCtx, controlURLs, false)
	cancel()
	if monitor != nil {
		monitor.RecordForURL(redisErr, sspAdapterWorkStatusURL)
	}
	statusJSON, _ := json.Marshal(statuses)
	severity := "ADV billing Redis failure"
	if outboxErr != nil {
		severity = "CRITICAL ADV billing Redis and outbox failure"
	}
	message := fmt.Sprintf("%s: redis_error=%v outbox_error=%v event_id=%s adv_statuses=%s", severity, redisErr, outboxErr, record.EventID, statusJSON)
	log.Print(message)
	if monitor != nil {
		if err := monitor.NotifyNowForRecordedError(message); err != nil {
			log.Printf("ADV billing immediate notification failed: %v", err)
		}
	}
}

func recordRedisError(monitor *services.RedisWriteErrorMonitor, err error, url string) {
	if monitor != nil {
		monitor.RecordForURL(err, url)
	}
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
	input, ok := r.Context().Value(httpin.Input).(*curlRequest)
	if !ok || input == nil {
		http.Error(w, "invalid conversion callback request", http.StatusBadRequest)
		return
	}
	status := strings.TrimSpace(input.Status)
	conversionEventTime := time.Now().UTC()

	conversionsUuid := uuid.NewString()

	if err := utils.WriteConversionStats(
		ctx,
		redisClients,
		conversionsUuid,
		input.ClickUuid,
		input.Payout,
		status,
		conversionEventTime,
		true,
	); err != nil {
		recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetConversions, conversionsUuid, true); err != nil {
		recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
