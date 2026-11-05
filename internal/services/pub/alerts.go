package pub

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
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
	AlertRTBCircuit
	AlertPolicy
	AlertDeliveryContract
	alertKindCount
)

var alertNames = [alertKindCount]string{
	"redis_safety", "last_request_flush", "config_unready", "redis_read",
	"overload", "reservation", "delivery", "confirm",
	"infrastructure", "runtime", "telemetry_outbox_write", "telemetry_outbox_flush", "rtb_circuit_open", "policy_unready", "delivery_contract_unverified",
}

var alertExplanations = [alertKindCount]string{
	"Publisher could not verify Redis safety settings",
	"Publisher could not publish the last-request timestamps",
	"Publisher configuration in Redis is unavailable or Redis safety has not been verified",
	"Publisher failed to read required data from Redis",
	"Publisher hit its concurrent request limit",
	"Publisher could not reserve, release or cache a delivery",
	"Publisher could not prepare or validate the ad delivery",
	"Publisher could not record the delivery confirmation",
	"Publisher infrastructure settings or delivery host were unavailable",
	"Publisher runtime or client configuration could not be served",
	"Publisher could not save a failed request to Redis or the local outbox",
	"Publisher could not flush the telemetry outbox to Redis",
	"Publisher temporarily skipped auctions after consecutive RTB failures",
	"Publisher could not load or verify the current site policy",
	"Publisher cannot run auctions because the delivery contract is not verified",
}

type SystemAlerts struct {
	instance string
	send     func(context.Context, string) error
	counts   [alertKindCount]atomic.Uint64
	rtbMu    sync.Mutex
	rtb      rtbAlertBatch
}

func NewSystemAlerts(instance string, send func(context.Context, string) error) *SystemAlerts {
	if send == nil {
		return nil
	}
	return &SystemAlerts{instance: instance, send: send}
}

// Record performs no I/O. One worker sends a digest even under heavy traffic.
func (a *SystemAlerts) Record(kind AlertKind) {
	if a == nil || kind >= alertKindCount {
		return
	}
	a.counts[kind].Add(1)
}

// Keep one representative request for each failure group. Excess categories
// are counted and can be recovered from the service logs.
const maxRTBAlertGroups = 64
const maxRTBAlertSamples = 8

type rtbAlertGroup struct {
	sample rtbFailure
	count  uint64
}

type rtbAlertBatch struct {
	count, other uint64
	groups       []rtbAlertGroup
}

func (b *rtbAlertBatch) add(g rtbAlertGroup) {
	for i, previous := range b.groups {
		if previous.sample.reason == g.sample.reason && previous.sample.status == g.sample.status {
			b.groups[i].count += g.count
			return
		}
	}
	if len(b.groups) == maxRTBAlertGroups {
		b.other += g.count
		return
	}
	b.groups = append(b.groups, g)
}

func boundedAlertText(text string, max int) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(text, ""))
	if len(text) <= max {
		return text
	}
	end := max - len("…")
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "…"
}

func (g rtbAlertGroup) line(withSample bool) string {
	f := g.sample
	line := fmt.Sprintf("- %d x [%s] %s", g.count, f.reason, f.explanation)
	if f.status != 0 {
		line += fmt.Sprintf(" (HTTP %d)", f.status)
	}
	if !withSample {
		return line
	}
	if f.upstream != "" {
		line += " | upstream=" + f.upstream
	}
	if f.requestID != "" {
		line += " | request=" + f.requestID
	}
	if f.ortbID != "" {
		line += " | ortb=" + f.ortbID
	}
	return line
}

func (a *SystemAlerts) recordRTBFailure(err error) {
	if a == nil || err == nil {
		return
	}
	var failure rtbFailure
	if !errors.As(err, &failure) {
		if strings.Contains(err.Error(), " HTTP 200: read response:") {
			failure = rtbFailure{reason: "response_read_failed", explanation: "Publisher could not read the successful HTTP response from RTB", status: 200}
		} else {
			failure = rtbFailure{reason: "unknown_failure", explanation: "Publisher hit an unclassified RTB failure; see the pub log"}
		}
	}
	a.rtbMu.Lock()
	a.rtb.count++
	a.rtb.add(rtbAlertGroup{sample: failure, count: 1})
	a.rtbMu.Unlock()
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
		for i, group := range rtb.groups {
			lines = append(lines, group.line(i < maxRTBAlertSamples || group.sample.status == 429))
		}
		if rtb.other != 0 {
			lines = append(lines, fmt.Sprintf("Other RTB errors: %d; see the pub log.", rtb.other))
		}
	}
	for i := range batch {
		batch[i] = a.counts[i].Swap(0)
		if batch[i] != 0 {
			lines = append(lines, fmt.Sprintf("%s=%d: %s", alertNames[i], batch[i], alertExplanations[i]))
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
			rtb.other += a.rtb.other
			for _, group := range a.rtb.groups {
				rtb.add(group)
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

const publisherAlertBatchInterval = 10 * time.Minute

func (a *SystemAlerts) Run(ctx context.Context) {
	a.run(ctx, publisherAlertBatchInterval)
}

func (a *SystemAlerts) run(ctx context.Context, interval time.Duration) {
	if a == nil {
		return
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if ctx.Err() != nil {
				return
			}
			a.flush(ctx)
			// Wait a full interval after each attempt, including failed sends.
			// flush retains a failed digest to merge with the next batch.
			timer.Reset(interval)
		}
	}
}
