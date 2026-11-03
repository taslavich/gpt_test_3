package pub

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"
)

// AlertKind is a fixed set of server-side failures. Request data and raw errors
// stay in local logs, never in the bot payload.
type AlertKind uint8

const (
	AlertRedisSafety AlertKind = iota
	AlertLastRequest
	AlertConfig
	AlertRedisRead
	AlertRTB
	AlertOverload
	AlertReservation
	AlertDelivery
	AlertConfirm
	AlertInfrastructure
	AlertRuntime
	AlertOutboxWrite
	AlertOutboxFlush
	alertKindCount
)

var alertNames = [alertKindCount]string{
	"redis_safety", "last_request_flush", "config_unready", "redis_read",
	"rtb_failed", "overload", "reservation", "delivery", "confirm",
	"infrastructure", "runtime", "telemetry_outbox_write", "telemetry_outbox_flush",
}

type SystemAlerts struct {
	instance string
	send     func(context.Context, string) error
	counts   [alertKindCount]atomic.Uint64
	wake     chan struct{}
}

func NewSystemAlerts(instance string, send func(context.Context, string) error) *SystemAlerts {
	if send == nil {
		return nil
	}
	return &SystemAlerts{instance: instance, send: send, wake: make(chan struct{}, 1)}
}

// Record performs no I/O. One worker sends a digest even under heavy traffic.
func (a *SystemAlerts) Record(kind AlertKind) {
	if a == nil || kind >= alertKindCount {
		return
	}
	a.counts[kind].Add(1)
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *SystemAlerts) flush(ctx context.Context) bool {
	var batch [alertKindCount]uint64
	var lines []string
	for i := range batch {
		batch[i] = a.counts[i].Swap(0)
		if batch[i] != 0 {
			lines = append(lines, fmt.Sprintf("%s=%d", alertNames[i], batch[i]))
		}
	}
	if len(lines) == 0 {
		return false
	}
	if err := a.send(ctx, fmt.Sprintf("[pub] host=%s\n%s", a.instance, strings.Join(lines, "\n"))); err != nil {
		// Retain the digest and any counts arriving during the send.
		for i, n := range batch {
			if n != 0 {
				a.counts[i].Add(n)
			}
		}
		if ctx.Err() == nil {
			log.Printf("pub system notification failed; will retry: %v", err)
		}
	}
	return true
}

func (a *SystemAlerts) Run(ctx context.Context) {
	a.run(ctx, time.Minute)
}

func (a *SystemAlerts) run(ctx context.Context, interval time.Duration) {
	if a == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.wake:
		}
		for ctx.Err() == nil && a.flush(ctx) {
			// Wait even after a send failure; a broken bot must not cause a retry storm.
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}
