package services

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
)

const sspAdapterWorkStatusAllPath = "/work_status/all"

type RedisWriteErrorMonitor struct {
	name                 string
	errors               atomic.Uint64
	lastURL              atomic.Value
	stopFunc             func(uint64, string)
	notifier             *utils.BotMessage
	ctx                  context.Context
	logThresholdPerTick  uint64
	stopThresholdPerTick uint64
	tickerInterval       time.Duration
}

func NewRedisWriteErrorMonitorWithSettings(name string, logThresholdPerSec uint64, stopThresholdPerSec uint64, tickerInterval time.Duration, stopFunc func(uint64, string), notifier *utils.BotMessage, ctx context.Context) *RedisWriteErrorMonitor {
	if ctx == nil {
		ctx = context.Background()
	}
	if tickerInterval <= 0 {
		tickerInterval = time.Second
	}
	if logThresholdPerSec == 0 {
		logThresholdPerSec = 1
	}
	if stopThresholdPerSec == 0 {
		stopThresholdPerSec = logThresholdPerSec
	}
	return &RedisWriteErrorMonitor{
		name:                 name,
		logThresholdPerTick:  redisWriteErrorsPerTick(logThresholdPerSec, tickerInterval),
		stopThresholdPerTick: redisWriteErrorsPerTick(stopThresholdPerSec, tickerInterval),
		tickerInterval:       tickerInterval,
		stopFunc:             stopFunc,
		notifier:             notifier,
		ctx:                  ctx,
	}
}

func redisWriteErrorsPerTick(thresholdPerSec uint64, tickerInterval time.Duration) uint64 {
	return uint64(math.Ceil(float64(thresholdPerSec) * tickerInterval.Seconds()))
}

func (m *RedisWriteErrorMonitor) Record(err error) {
	m.RecordForURL(err, "")
}

func (m *RedisWriteErrorMonitor) RecordForURL(err error, workStatusURL string) {
	if err == nil || m == nil {
		return
	}
	if strings.TrimSpace(workStatusURL) != "" {
		m.lastURL.Store(workStatusURL)
	}
	m.errors.Add(1)
}

func (m *RedisWriteErrorMonitor) Start() {
	if m == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(m.tickerInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
			}
			count := m.errors.Swap(0)
			if count >= m.stopThresholdPerTick {
				workStatusURL, _ := m.lastURL.Load().(string)
				message := fmt.Sprintf("ОСТАНОВКА service=%s Redis write errors reached %d requests per monitor tick, stopping SSP adapter ORTB streams; ticker_interval=%s; ssp_adapter_url=%s", m.name, count, m.tickerInterval, workStatusURL)
				log.Print(message)
				if m.notifier != nil {
					if err := m.notifier.SendTextMessageToBot(m.ctx, message); err != nil {
						log.Printf("❌ failed to send bot notification: %v", err)
					}
				}
				if m.stopFunc != nil {
					m.stopFunc(count, workStatusURL)
				}
				continue
			}

			if count >= m.logThresholdPerTick {
				workStatusURL, _ := m.lastURL.Load().(string)
				message := fmt.Sprintf("ПРЕДУПРЕЖДЕНИЕ service=%s Redis write errors reached %d requests per monitor tick; ticker_interval=%s; ssp_adapter_url=%s", m.name, count, m.tickerInterval, workStatusURL)
				log.Print(message)
				if m.notifier != nil {
					if err := m.notifier.SendTextMessageToBot(m.ctx, message); err != nil {
						log.Printf("❌ failed to send bot notification: %v", err)
					}
				}
			}
		}
	}()
}

func StopSspAdapterOrtbStreams(ctx context.Context, workStatusURL string) {
	endpoint := strings.TrimSpace(workStatusURL)
	if endpoint == "" {
		log.Printf("⚠️ SSP adapter stop URL is not configured; cannot stop SSP adapter ORTB streams")
		return
	}

	if err := stopSspAdapterAllStreams(ctx, endpoint); err != nil {
		log.Printf("❌ failed to stop SSP adapter ORTB streams at %s: %v", endpoint, err)
	}
}

func StopAllSspAdapterOrtbStreams(ctx context.Context, workStatusURLs []string) {
	if len(workStatusURLs) == 0 {
		log.Printf("⚠️ SSP adapter stop URLs are not configured; cannot stop SSP adapter ORTB streams")
		return
	}

	for _, rawURL := range workStatusURLs {
		StopSspAdapterOrtbStreams(ctx, rawURL)
	}
}

func stopSspAdapterAllStreams(parent context.Context, endpoint string) error {
	if parent == nil {
		parent = context.Background()
	}
	requestURL, err := buildSspAdapterStopAllURL(endpoint)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, requestURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func buildSspAdapterStopAllURL(endpoint string) (string, error) {
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if u.Path == "" || u.Path == "/" || u.Path == "/work_status" {
		u.Path = sspAdapterWorkStatusAllPath
	}
	q := u.Query()
	q.Set("work", "false")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// NotifyNow sends an immediate alert through the same notifier used by the
// RedisWriteErrorMonitor. It is used for single billing failures that cannot
// wait for the aggregate monitor tick.
func (m *RedisWriteErrorMonitor) NotifyNow(message string) error {
	if m == nil || m.notifier == nil {
		return fmt.Errorf("redis write error notifier is not configured")
	}
	return m.notifier.SendTextMessageToBot(m.ctx, message)
}

// NotifyNowForRecordedError sends one immediate notification for an error that
// was already registered through RecordForURL. It consumes that one counter so
// the same billing failure is not reported again by the aggregate ticker.
func (m *RedisWriteErrorMonitor) NotifyNowForRecordedError(message string) error {
	if m == nil {
		return fmt.Errorf("redis write error monitor is nil")
	}
	m.consumeErrors(1)
	return m.NotifyNow(message)
}

func (m *RedisWriteErrorMonitor) consumeErrors(count uint64) {
	if m == nil || count == 0 {
		return
	}
	for {
		current := m.errors.Load()
		if current == 0 {
			return
		}
		next := uint64(0)
		if current > count {
			next = current - count
		}
		if m.errors.CompareAndSwap(current, next) {
			return
		}
	}
}

// SetADVWorkStatus calls every configured ADV control endpoint and returns an
// actual HTTP status per address. Network and request failures are represented
// as 503, as required by the billing failure policy.
func SetADVWorkStatus(parent context.Context, endpoints []string, work bool) map[string]int {
	if parent == nil {
		parent = context.Background()
	}
	result := make(map[string]int)
	for _, raw := range endpoints {
		endpoint := strings.TrimSpace(raw)
		if endpoint == "" {
			continue
		}
		statusCode := http.StatusServiceUnavailable
		requestURL, err := buildADVWorkStatusURL(endpoint, work)
		if err == nil {
			ctx, cancel := context.WithTimeout(parent, 3*time.Second)
			req, reqErr := http.NewRequestWithContext(ctx, http.MethodPut, requestURL, nil)
			if reqErr == nil {
				resp, doErr := http.DefaultClient.Do(req)
				if doErr == nil {
					if resp.StatusCode == http.StatusOK {
						statusCode = http.StatusOK
					}
					_ = resp.Body.Close()
				}
			}
			cancel()
		}
		result[endpoint] = statusCode
	}
	return result
}

func buildADVWorkStatusURL(endpoint string, work bool) (string, error) {
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("ADV endpoint has no host")
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/work_status"
	}
	q := u.Query()
	q.Set("work", fmt.Sprintf("%t", work))
	u.RawQuery = q.Encode()
	return u.String(), nil
}
