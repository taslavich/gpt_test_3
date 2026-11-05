package pub

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// AlertKind enumerates failures reported as counters. RTB errors use a separate
// digest so the operator also sees the reason.
type AlertKind uint8

const (
	AlertRedisSafety AlertKind = iota
	AlertLastRequest
	AlertConfig
	AlertRedisRead
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
	"overload", "reservation", "delivery", "confirm",
	"infrastructure", "runtime", "telemetry_outbox_write", "telemetry_outbox_flush",
}

type SystemAlerts struct {
	instance string
	send     func(context.Context, string) error
	counts   [alertKindCount]atomic.Uint64
	wake     chan struct{}
	rtbMu    sync.Mutex
	rtb      rtbAlertBatch
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

const maxRTBAlertSamples = 3
const maxRTBAlertDetailBytes = 1024

type rtbAlertBatch struct {
	count   uint64
	details []string
	omitted bool
}

func (b *rtbAlertBatch) add(detail string) {
	for _, previous := range b.details {
		if detail == previous {
			return
		}
	}
	if len(b.details) == maxRTBAlertSamples {
		b.omitted = true
		return
	}
	b.details = append(b.details, detail)
}

func (a *SystemAlerts) recordRTBFailure(err error) {
	if a == nil || err == nil {
		return
	}
	detail := strings.TrimSpace(strings.ToValidUTF8(err.Error(), "�"))
	if len(detail) > maxRTBAlertDetailBytes {
		end := maxRTBAlertDetailBytes - len("…")
		for !utf8.RuneStart(detail[end]) {
			end--
		}
		detail = detail[:end] + "…"
	}
	a.rtbMu.Lock()
	a.rtb.count++
	a.rtb.add(detail)
	a.rtbMu.Unlock()
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *SystemAlerts) flush(ctx context.Context) bool {
	var batch [alertKindCount]uint64
	var lines []string
	a.rtbMu.Lock()
	rtb := a.rtb
	a.rtb = rtbAlertBatch{}
	a.rtbMu.Unlock()
	if rtb.count != 0 {
		lines = append(lines, fmt.Sprintf("RTB errors: %d", rtb.count))
		for _, detail := range rtb.details {
			lines = append(lines, "- "+detail)
		}
		if rtb.omitted {
			lines = append(lines, "Other RTB errors are in the pub journal.")
		}
	}
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
		if rtb.count != 0 {
			a.rtbMu.Lock()
			rtb.count += a.rtb.count
			rtb.omitted = rtb.omitted || a.rtb.omitted
			for _, detail := range a.rtb.details {
				rtb.add(detail)
			}
			a.rtb = rtb
			a.rtbMu.Unlock()
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
