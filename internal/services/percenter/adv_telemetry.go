package percenter

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// ADVTelemetrySource is the minimal contract required by the percenter
// telemetry publisher. The interface deliberately lives in percenter so this
// package does not import the ADV service package back and create an import
// cycle. *auction.AuctionService satisfies this interface implicitly.
type ADVTelemetrySource interface {
	PercenterTelemetryTotals() TelemetryTotals
	RecordPercenterTelemetryPublishFailure()
	RecordPercenterTelemetryPublishRecovered()
}

// TelemetryNotifier keeps the telemetry package independent from the concrete
// Telegram/bot implementation used by cmd/adv.
type TelemetryNotifier func(context.Context, string) error

func StartADVPercenterTelemetryPublisher(
	ctx context.Context,
	client *redis.Client,
	service ADVTelemetrySource,
	notify TelemetryNotifier,
) {
	if service == nil || client == nil {
		return
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	startedAt := time.Now().UTC()
	instanceID := fmt.Sprintf("%d-%d", os.Getpid(), startedAt.UnixNano())

	go RunTelemetryPublisher(
		ctx,
		client,
		func(now time.Time) TelemetrySnapshot {
			return TelemetrySnapshot{
				Version:    1,
				Source:     "adv",
				Hostname:   hostname,
				InstanceID: instanceID,
				StartedAt:  startedAt,
				UpdatedAt:  now,
				Totals:     service.PercenterTelemetryTotals(),
			}
		},
		func(attempt uint64, publishErr error) {
			service.RecordPercenterTelemetryPublishFailure()
			log.Printf(
				"[ADV][PERCENTER_TELEMETRY_UPLOAD_RETRY] host=%s attempt=%d next_retry=%s error=%v",
				hostname,
				attempt,
				TelemetryPublishInterval,
				publishErr,
			)
		},
		func(attempt uint64, publishErr error) {
			message := fmt.Sprintf(
				"[ADV][PERCENTER_TELEMETRY_REDIS_UNAVAILABLE] host=%s failed_attempts=%d retry_interval=%s error=%v",
				hostname,
				attempt,
				TelemetryPublishInterval,
				publishErr,
			)
			log.Print(message)
			if notify != nil {
				if sendErr := notify(ctx, message); sendErr != nil {
					log.Printf("[ADV][PERCENTER_TELEMETRY_TELEGRAM_ERROR] %v", sendErr)
				}
			}
		},
		func(failures uint64) {
			service.RecordPercenterTelemetryPublishRecovered()
			log.Printf("[ADV][PERCENTER_TELEMETRY_UPLOAD_RECOVERED] host=%s failed_attempts=%d", hostname, failures)
			if failures < TelemetryFailureAlertAfter || notify == nil {
				return
			}
			message := fmt.Sprintf(
				"[ADV][PERCENTER_TELEMETRY_REDIS_RECOVERED] host=%s after_failed_attempts=%d telemetry uploads are healthy",
				hostname,
				failures,
			)
			if sendErr := notify(ctx, message); sendErr != nil {
				log.Printf("[ADV][PERCENTER_TELEMETRY_TELEGRAM_ERROR] %v", sendErr)
			}
		},
		nil,
	)
	log.Printf(
		"[ADV][PERCENTER_TELEMETRY] publisher_started host=%s interval=%s alert_after_failures=%d",
		hostname,
		TelemetryPublishInterval,
		TelemetryFailureAlertAfter,
	)
}
