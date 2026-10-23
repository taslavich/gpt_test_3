package sppAdapterWeb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
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

func videoImpressionNoop(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
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
	publisherPOP := format == constants.POP && input.Publisher == "1"
	clickUUID := uuid.NewString()
	if publisherPOP {
		id, e := uuid.NewV7()
		if e != nil {
			http.Error(w, "click ID unavailable", 503)
			return
		}
		clickUUID = id.String()
	}
	// The queue payload is in memory/durable Outbox only. This compatibility
	// discriminator means p=1; owner/billing identity is never taken from it.
	publisherInput := publisherBURLInput{ID: input.GlobalId, Record: publisherBURLRecord{Format: format, PublisherID: "1"}}
	publisherReady := false
	record := outbox.Record{
		Kind:       outbox.KindADM,
		EventID:    "adm:" + clickUUID,
		GlobalID:   strings.TrimSpace(input.GlobalId),
		ClickID:    clickUUID,
		WinnerType: outbox.WinnerUnknown,
		Format:     format,
		Source:     "adm",
		CreatedAt:  time.Now().UTC(),
		Attempts:   1,
	}
	redirectURL := decodedURL
	var processingErr error
	stopADV := false

	var dspWinner bool
	if publisherPOP {
		var fields map[string]string
		if redisAdmClient == nil {
			err = errors.New("redis client is nil")
		} else {
			fields, err = redisAdmClient.HGetAll(ctx, input.GlobalId).Result()
		}
		if err == nil && fields["uuid"] != "" && fields["uuid"] == input.GlobalId {
			dspWinner = true
			publisherInput.Record.WinnerType = "dsp"
			publisherInput.Record.BURL = fields["pub_dsp_nurl"]
			publisherInput.Redis = redisAdmClient
			publisherReady = true
			if publisherInput.Record.BURL == "" {
				missing := fmt.Errorf("Publisher DSP NURL missing uuid=%s; internal BURL continues", input.GlobalId)
				log.Print(missing)
				recordRedisError(redisWriteErrorMonitor, missing, sspAdapterWorkStatusURL)
			}
		}
	} else {
		// Ordinary RTB keeps the exact proj212 UUID lookup.
		dspWinner, err = utils.UUIDKeyExistsInRedis(ctx, redisAdmClient, input.GlobalId)
	}
	if err != nil {
		processingErr = fmt.Errorf("resolve DSP ADM winner: %w", err)
	} else if dspWinner {
		record.WinnerType = outbox.WinnerDSP
	} else if advBillingStore == nil {
		processingErr = errors.New("ADV billing store is not initialized")
	} else {
		winner, winnerErr := advBillingStore.ReadWinner(ctx, input.GlobalId, format)
		if winnerErr != nil {
			processingErr = fmt.Errorf("resolve ADV ADM winner: %w", winnerErr)
			if !errors.Is(winnerErr, redis.Nil) {
				stopADV = true
			}
		} else {
			record.WinnerType = outbox.WinnerADV
			record.UserID = winner.UserID
			record.CampaignID = winner.CampaignID
			record.TypeModel = winner.TypeModel
			record.Price = winner.Price
			redirectURL = appendClickIDParameter(decodedURL, winner.ClickIDParam, clickUUID)
			if publisherPOP {
				publisherInput.Record.WinnerType = "adv"
				publisherInput.Winner = winner
				publisherInput.Redis = advBillingStore.WinnerRedis()
				publisherReady = true
			}
		}
	}

	if processingErr == nil {
		if err := utils.WriteClickStats(ctx, redisClients, clickUUID, input.GlobalId, format, true); err != nil {
			processingErr = fmt.Errorf("write ADM click stats: %w", err)
		} else if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetClicks, clickUUID, true); err != nil {
			processingErr = fmt.Errorf("add ADM click to Redis set: %w", err)
		}
	}

	if processingErr != nil {
		record.LastError = processingErr.Error()
		record.LastAttemptAt = time.Now().UTC()
		persistADMFailure(
			processingErr,
			record,
			advOutbox,
			advControlURLs,
			sspAdapterWorkStatusURL,
			redisWriteErrorMonitor,
			stopADV,
		)
	}

	// The SSP request has already been paid for. Infrastructure and billing
	// failures are retried through the durable outbox and must never block the
	// user's redirect. The browser page records whether this click started in an
	// iframe, then immediately performs the configured redirect attempts without
	// waiting for the stats report.
	writeADMRedirectPage(w, redirectURL, clickUUID)
	if publisherPOP && publisherReady {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if task, ok := r.Context().Value(publisherBurlKey{}).(publisherBurlFunc); ok {
			task(context.Background(), publisherInput)
		}
	}
}

func writeADMRedirectPage(w http.ResponseWriter, redirectURL, clickUUID string) {
	redirectJSON, _ := json.Marshal(redirectURL)
	reportBase := GetAdmIframeReportURL + "?click_id=" + url.QueryEscape(clickUUID)
	reportJSON, _ := json.Marshal(reportBase)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title></title>
<noscript><meta http-equiv="refresh" content="0;url=%s"></noscript>
</head>
<body>
<script>
(function () {
  const advertiserUrl = %s;
  const reportBase = %s;
  const isIframe = window.self !== window.top;
  const reportUrl = reportBase + "&is_iframe=" + (isIframe ? "1" : "0");

  try {
    let queued = false;
    if (navigator.sendBeacon) {
      queued = navigator.sendBeacon(reportUrl);
    }
    if (!queued && window.fetch) {
      void fetch(reportUrl, {method: "POST", keepalive: true, credentials: "omit"}).catch(function () {});
    }
  } catch (_) {}

  if (isIframe) {
    try { window.open(advertiserUrl, "_blank"); } catch (_) {}
    try { window.top.location = advertiserUrl; } catch (_) {}
  }

  window.location.replace(advertiserUrl);
})();
</script>
</body>
</html>`, html.EscapeString(redirectURL), redirectJSON, reportJSON)
}

func postAdmIframeReport(
	w http.ResponseWriter,
	r *http.Request,
	redisClients []*redis.Client,
) {
	clickID := strings.TrimSpace(r.URL.Query().Get("click_id"))
	if _, err := uuid.Parse(clickID); err != nil {
		http.Error(w, "invalid click_id", http.StatusBadRequest)
		return
	}

	rawIframe := strings.TrimSpace(r.URL.Query().Get("is_iframe"))
	var isIframe bool
	switch rawIframe {
	case constants.FALSE:
		isIframe = false
	case constants.TRUE:
		isIframe = true
	default:
		http.Error(w, "invalid is_iframe", http.StatusBadRequest)
		return
	}

	updated, err := utils.WriteClickIframeStatus(r.Context(), redisClients, clickID, isIframe)
	if err != nil {
		log.Printf("ADM iframe report failed: click_id=%s is_iframe=%s error=%v", clickID, rawIframe, err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !updated {
		log.Printf("ADM iframe report ignored for missing click: click_id=%s is_iframe=%s", clickID, rawIframe)
	}

	w.WriteHeader(http.StatusNoContent)
}

var clickIDParameterNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.~-]+$`)

func appendClickIDParameter(rawURL, parameterName, clickID string) string {
	rawURL = strings.TrimSpace(rawURL)
	parameterName = strings.TrimSpace(parameterName)
	clickID = strings.TrimSpace(clickID)
	if rawURL == "" || clickID == "" || !clickIDParameterNamePattern.MatchString(parameterName) {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	query.Set(parameterName, clickID)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func persistADMFailure(
	processingErr error,
	record outbox.Record,
	outboxStore *outbox.Store,
	controlURLs []string,
	sspAdapterWorkStatusURL string,
	monitor *services.RedisWriteErrorMonitor,
	stopADV bool,
) {
	// Malformed callback with an empty winner ID is not an ADV
	// infrastructure/billing failure and must never disable ADV.
	if strings.TrimSpace(record.GlobalID) == "" {
		stopADV = false
	}

	requiresADVRecovery := stopADV
	record.RequiresADVRecovery = &requiresADVRecovery

	outboxErr := errors.New("ADM outbox is not initialized")
	if outboxStore != nil {
		outboxErr = outboxStore.Save(record)
	}
	if monitor != nil {
		monitor.RecordForURL(processingErr, sspAdapterWorkStatusURL)
	}

	statuses := map[string]int{}
	if stopADV {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		statuses = services.SetADVWorkStatus(stopCtx, controlURLs, false)
		cancel()
	}

	severity := "ADM callback deferred to durable outbox"
	if outboxErr != nil {
		severity = "CRITICAL ADM callback processing and outbox failure"
	}
	statusJSON, _ := json.Marshal(statuses)
	message := fmt.Sprintf(
		"%s: error=%v outbox_error=%v event_id=%s global_id=%s winner_type=%s adv_statuses=%s",
		severity,
		processingErr,
		outboxErr,
		record.EventID,
		record.GlobalID,
		record.WinnerType,
		statusJSON,
	)
	log.Print(message)

	if outboxErr != nil && monitor != nil {
		go func() {
			if err := monitor.NotifyNowForRecordedError(message); err != nil {
				log.Printf("ADM outbox immediate notification failed: %v", err)
			}
		}()
	}
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

	// Detect a preselected ADV winner by UUID in DB 6. Local ADV winners have
	// no downstream NURL, while external RTB winners may carry one in url=.
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
	if isADV && strings.TrimSpace(input.DspURL) == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if !isADV && input.Ssp_Domain == "adl_pb.com" {
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
	billable := format == constants.NAT || format == constants.BAN || format == constants.POP || format == constants.VID
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
	if strings.TrimSpace(input.DspURL) != "" {
		decodedURL, err := url.QueryUnescape(input.DspURL)
		if err != nil || strings.TrimSpace(decodedURL) == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		resp, err := nurlClient.Get(decodedURL)
		if err == nil {
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Shared business function, with no HTTP writer and no self-request to /burl.
func processBurl(ctx context.Context, resolved publisherBURLInput, nurlClient *http.Client, redisClients []*redis.Client, redisSetImpressions string, monitor *services.RedisWriteErrorMonitor, statusURL string, store *billing.Store, box *outbox.Store, controls []string, dspExecutor ...*publisherDSPExecutor) (result error) {
	ctx = context.WithValue(ctx, publisherWinnerKey{}, resolved)
	format := resolved.Record.Format
	publisherPOP := format == constants.POP && resolved.Record.PublisherID != ""
	if publisherPOP {
		// DSP ADM uses DB1 and external BURL uses DB0. The lease must always live
		// in the ADM winner hash; two copies of the context are not two billing locks.
		if resolved.Record.WinnerType == "dsp" {
			if canonical, ok := ctx.Value(publisherDSPGuardKey{}).(*redis.Client); ok {
				resolved.Redis = canonical
			}
		}
		// Serialize internal and external BURL for this winner. Retry on actual
		// failure is safe: billing event ID and impression UUID stay unchanged.
		lease := uuid.NewString()
		acquired, e := resolved.Redis.Eval(ctx, `local state=redis.call('HGET',KEYS[1],'pub_burl_done');if state=='1' then return 0 end
   local untilms=tonumber(redis.call('HGET',KEYS[1],'pub_burl_until') or '0');if untilms>tonumber(ARGV[1]) then return 2 end
   redis.call('HSET',KEYS[1],'pub_burl_lease',ARGV[3],'pub_burl_until',ARGV[2]);if redis.call('PTTL',KEYS[1])<0 then redis.call('PEXPIRE',KEYS[1],86400000) end;return 1`, []string{resolved.ID}, time.Now().UnixMilli(), time.Now().Add(2*time.Minute).UnixMilli(), lease).Int()
		if e != nil {
			return e
		}
		if acquired == 0 {
			return nil
		}
		if acquired == 2 {
			return errors.New("publisher BURL lease busy")
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			defer cancel()
			done := "0"
			if result == nil {
				done = "1"
			}
			committed, e := resolved.Redis.Eval(cleanup, `if redis.call('HGET',KEYS[1],'pub_burl_lease')==ARGV[1] then redis.call('HDEL',KEYS[1],'pub_burl_lease','pub_burl_until');if ARGV[2]=='1' then redis.call('HSET',KEYS[1],'pub_burl_done','1') end;return 1 end;return 0`, []string{resolved.ID}, lease, done).Int()
			if e == nil && committed != 1 {
				e = errors.New("publisher BURL lease no longer owned")
			}
			if e != nil {
				log.Printf("publisher BURL completion uuid=%s: %v", resolved.ID, e)
				if result == nil {
					result = e
				}
			}
			if result == nil {
				// Only the guard owner submits, after successful Redis completion.
				// Admission is bounded/nonblocking and never changes internal success.
				var executor *publisherDSPExecutor
				if len(dspExecutor) > 0 {
					executor = dspExecutor[0]
				}
				executor.enqueue(resolved.ID, resolved.Record.BURL)
			}
		}()
	}
	billable := format == constants.NAT || format == constants.BAN || format == constants.POP || format == constants.VID
	if _, e := handleADVCallback(ctx, resolved.ID, format, "burl", billable, store, box, controls, statusURL, monitor); e != nil {
		return e
	}
	impressionUUID := uuid.NewString()
	if publisherPOP {
		impressionUUID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("publisher-burl:"+resolved.ID)).String()
	}
	if e := utils.WriteImpressionStats(ctx, redisClients, impressionUUID, resolved.ID, format, true); e != nil {
		recordRedisError(monitor, e, statusURL)
		return e
	}
	if e := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetImpressions, impressionUUID, true); e != nil {
		recordRedisError(monitor, e, statusURL)
		return e
	}
	if publisherPOP {
		return nil // Guard completion and DSP admission run in the defer above.
	}
	return sendBURLCallback(ctx, resolved.ID, resolved.Record.BURL, nurlClient, false)
}

// Keep ordinary RTB callbacks synchronous with their existing HTTP semantics.
func sendBURLCallback(ctx context.Context, id, target string, nurlClient *http.Client, publisherPOP bool) error {
	if target != "" {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if e != nil {
			log.Printf("downstream BURL request invalid uuid=%s", id)
			if !publisherPOP {
				return e
			}
			return nil // Internal effects completed; external URL failure is diagnostic.
		}
		resp, e := nurlClient.Do(req)
		if e == nil {
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				log.Printf("downstream BURL uuid=%s status=%d", id, resp.StatusCode)
			}
			if _, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 4096)); readErr != nil {
				log.Printf("downstream BURL response unreadable uuid=%s", id)
			}
		} else {
			if uerr, ok := e.(*url.Error); ok {
				e = uerr.Err
			}
			log.Printf("downstream BURL uuid=%s: %v", id, e)
		}
	}
	return nil
}

func getClicksWins(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	redisClients []*redis.Client,
	redisSetClicksWins string,
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
	sspAdapterWorkStatusURL string,
	advBillingStore *billing.Store,
	advOutbox *outbox.Store,
	advControlURLs []string,
) {
	input, ok := r.Context().Value(httpin.Input).(*clicksWinsRequest)
	if !ok || input == nil {
		http.Error(w, "invalid ClicksWins request", http.StatusBadRequest)
		return
	}

	clicksWinsUUID := uuid.NewString()
	if advBillingStore != nil {
		winner, err := advBillingStore.ReadWinner(ctx, input.GlobalId, constants.IPP)
		if err != nil && !errors.Is(err, redis.Nil) {
			recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if err == nil {
			requiresRecovery := true
			billingRecord := outbox.Record{
				Kind:                outbox.KindBilling,
				RequiresADVRecovery: &requiresRecovery,
				EventID:             billing.CallbackEventID("clicks_wins", constants.IPP, clicksWinsUUID),
				GlobalID:            strings.TrimSpace(input.GlobalId),
				UserID:              winner.UserID,
				CampaignID:          winner.CampaignID,
				TypeModel:           winner.TypeModel,
				Price:               winner.Price,
				Format:              winner.Format,
				Source:              "clicks_wins",
				CreatedAt:           time.Now().UTC(),
				Attempts:            1,
			}
			if err := billing.ApplyDurableCallbackIntent(ctx, advOutbox, advBillingStore, billingRecord); err != nil {
				billingRecord.LastError = err.Error()
				billingRecord.LastAttemptAt = time.Now().UTC()
				handleADVWriteFailure(err, billingRecord, advOutbox, advControlURLs, sspAdapterWorkStatusURL, redisWriteErrorMonitor)
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
	}

	if err := utils.WriteClicksWinStats(ctx, redisClients, clicksWinsUUID, input.GlobalId, true); err != nil {
		recordRedisError(redisWriteErrorMonitor, err, sspAdapterWorkStatusURL)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err := utils.AddUUIDToRedisSet(ctx, redisClients, redisSetClicksWins, clicksWinsUUID, true); err != nil {
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
	var winner billing.Winner
	if resolved, ok := ctx.Value(publisherWinnerKey{}).(publisherBURLInput); ok && resolved.ID == winnerUUID {
		if resolved.Record.WinnerType != "adv" {
			return false, nil
		}
		winner = resolved.Winner
	} else {
		if store == nil {
			return false, nil
		}
		var err error
		winner, err = store.ReadWinner(ctx, winnerUUID, format)
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		if err != nil {
			recordRedisError(monitor, err, sspAdapterWorkStatusURL)
			return false, err
		}
	}
	if !billable {
		return true, nil
	}
	requiresRecovery := true
	record := outbox.Record{
		Kind: outbox.KindBilling, RequiresADVRecovery: &requiresRecovery,
		EventID: billing.CallbackEventID(source, winner.Format, winnerUUID),
		UserID:  winner.UserID, CampaignID: winner.CampaignID, TypeModel: winner.TypeModel,
		Price: winner.Price, Format: winner.Format, Source: source, CreatedAt: time.Now().UTC(), Attempts: 1,
	}
	if err := billing.ApplyDurableCallbackIntent(ctx, outboxStore, store, record); err != nil {
		record.LastError = err.Error()
		record.LastAttemptAt = time.Now().UTC()
		handleADVWriteFailure(err, record, outboxStore, controlURLs, sspAdapterWorkStatusURL, monitor)
		return true, err
	}
	return true, nil
}

func handleADVWriteFailure(redisErr error, record outbox.Record, outboxStore *outbox.Store, controlURLs []string, sspAdapterWorkStatusURL string, monitor *services.RedisWriteErrorMonitor) {
	requiresADVRecovery := true
	record.RequiresADVRecovery = &requiresADVRecovery

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
