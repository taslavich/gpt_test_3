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
	AlertPolicy
	AlertDeliveryContract
	alertKindCount
)

var alertExplanations = [alertKindCount]string{
	AlertRedisSafety:      "Redis safety check failed",
	AlertLastRequest:      "Last-request timestamps not saved",
	AlertConfig:           "Publisher config unavailable or Redis unverified",
	AlertRedisRead:        "Redis read failed",
	AlertOverload:         "Publisher request limit reached",
	AlertReservation:      "Delivery reservation or cache failed",
	AlertDelivery:         "Ad delivery preparation failed",
	AlertConfirm:          "Delivery confirmation not saved",
	AlertInfrastructure:   "Delivery host or settings unavailable",
	AlertRuntime:          "Publisher script or client config unavailable",
	AlertOutboxWrite:      "Failed request telemetry not saved",
	AlertOutboxFlush:      "Telemetry backlog not sent to Redis",
	AlertPolicy:           "Site policy unavailable or invalid",
	AlertDeliveryContract: "Delivery contract unverified; auctions blocked",
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

// Bound the digest so the RTB section leaves room for the other counters.
const maxRTBAlertGroups = 64
const maxRTBAlertLinesBytes = 2600

type rtbAlertGroup struct {
	reason, path string
	status       int
	count        uint64
}

type rtbAlertBatch struct {
	count, other uint64
	groups       []rtbAlertGroup
}

func (b *rtbAlertBatch) add(g rtbAlertGroup) {
	for i, previous := range b.groups {
		if previous.reason == g.reason && previous.status == g.status {
			b.groups[i].count += g.count
			if previous.path != g.path {
				b.groups[i].path = "" // The count covers multiple or unknown endpoints.
			}
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

var shortRTBReasons = map[string]string{
	"ssp_panic":                    "SSP Adapter panic",
	"ssp_request_invalid":          "SSP could not read the bid request",
	"ssp_dependencies":             "SSP dependencies not ready",
	"ssp_feed_unknown":             "Feed missing from SSP config",
	"ssp_uuid_rejected":            "SSP rejected the signed request ID",
	"ssp_device_missing":           "Device missing from bid request",
	"ssp_ip_missing":               "Visitor IP missing from bid request",
	"ssp_ip_limited":               "Visitor IP rate-limited",
	"ssp_ua_missing":               "User-Agent missing",
	"ssp_ua_short":                 "User-Agent too short",
	"ssp_site_missing":             "Site missing from bid request",
	"ssp_site_id_missing":          "Site ID missing",
	"ssp_ip_check_failed":          "IP reputation check failed",
	"ssp_ip_rejected":              "IP rejected by reputation check",
	"ssp_ip_invalid":               "Invalid visitor IP",
	"ssp_geo_lookup_failed":        "Geo lookup failed",
	"ssp_impressions_missing":      "No impressions in bid request",
	"ssp_impression_id_missing":    "Impression ID missing",
	"ssp_impression_id_duplicate":  "Duplicate impression ID",
	"ssp_video_missing":            "Video object missing",
	"ssp_video_mimes_missing":      "Video MIME types missing",
	"ssp_publisher_owner":          "SSP could not read publisher owner from Redis",
	"ssp_orchestrator_timeout":     "Orchestrator timed out",
	"ssp_orchestrator_unavailable": "Orchestrator unreachable",
	"ssp_orchestrator_canceled":    "Orchestrator call canceled",
	"ssp_orchestrator_rejected":    "Orchestrator rejected bid request",
	"ssp_orchestrator_error":       "Orchestrator call failed",
	"ssp_orchestrator_empty":       "Orchestrator returned no response",
	"ssp_site_blocked":             "Site ID blocked",
	"ssp_ortb_write_failed":        "SSP could not save auction stats in Redis",
	"transport_canceled":           "Request canceled",
	"transport_timeout":            "Request timed out",
	"transport_dns":                "DNS lookup failed",
	"transport_refused":            "Connection refused",
	"transport_reset":              "Connection reset",
	"transport_tls":                "TLS handshake failed",
	"transport_failed":             "Network request failed",
	"traffic_type_invalid":         "No RTB route for this traffic type",
	"endpoint_invalid":             "Invalid RTB endpoint URL",
	"request_build_failed":         "Could not build RTB HTTP request",
	"request_encode_failed":        "Could not encode bid request",
	"ortb_prepare_failed":          "Could not prepare auction in Redis",
	"response_read_failed":         "Could not read bid response",
	"response_decode_failed":       "Could not decode bid response",
	"unknown_failure":              "Unknown failure; see pub logs",
}

func (g rtbAlertGroup) line() string {
	reason := shortRTBReasons[g.reason]
	if reason == "" {
		reason = "Unexpected response; see pub logs"
		switch g.status {
		case 400:
			reason = "Invalid request"
		case 401, 403:
			reason = "Request refused"
		case 404:
			reason = "Endpoint not found"
		case 408:
			reason = "Request timed out"
		case 429:
			reason = "Request rate limited"
		case 500:
			reason = "Upstream internal error"
		case 502:
			reason = "Gateway got no valid response"
		case 503:
			reason = "Service unavailable"
		case 504:
			reason = "Gateway timed out"
		}
	}
	prefix := "RTB"
	if g.status != 0 {
		prefix += fmt.Sprintf(" HTTP %d", g.status)
	}
	line := fmt.Sprintf("%s: %s ×%d", prefix, reason, g.count)
	if g.path != "" {
		line += " (" + g.path + ")"
	}
	return line
}

func alertRTBPath(upstream string) string {
	// The stored upstream is host[:port] + path. No host, query or fragment
	// is needed in the digest; the short path distinguishes routes.
	upstream, _, _ = strings.Cut(upstream, "?")
	upstream, _, _ = strings.Cut(upstream, "#")
	if i := strings.IndexByte(upstream, '/'); i >= 0 {
		return boundedAlertText(upstream[i:], 32)
	}
	return ""
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
	a.rtb.add(rtbAlertGroup{reason: failure.reason, status: failure.status, path: failure.path, count: 1})
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
		remaining, other := maxRTBAlertLinesBytes, rtb.other
		// In a busy digest show throttling before spending the text budget.
		for _, rateLimited := range []bool{true, false} {
			for _, group := range rtb.groups {
				if (group.status == 429) != rateLimited {
					continue
				}
				line := group.line()
				if len(line)+1 > remaining {
					other += group.count
					continue
				}
				remaining -= len(line) + 1
				lines = append(lines, line)
			}
		}
		if other != 0 {
			lines = append(lines, fmt.Sprintf("Other RTB errors: %d (see pub logs)", other))
		}
	}
	for i := range batch {
		batch[i] = a.counts[i].Swap(0)
		if batch[i] != 0 {
			lines = append(lines, fmt.Sprintf("%s: %d", alertExplanations[i], batch[i]))
		}
	}
	if len(lines) == 0 {
		return false
	}
	if err := a.send(ctx, fmt.Sprintf("[pub] %s\n%s", boundedAlertText(a.instance, 128), strings.Join(lines, "\n"))); err != nil {
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
