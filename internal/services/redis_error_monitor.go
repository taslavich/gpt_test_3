package services

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const (
	RedisWriteErrorLogThresholdPerSec  = uint64(100)
	RedisWriteErrorStopThresholdPerSec = uint64(800)

	sspAdapterWorkStatusAllPath = "/work_status/all"
)

type RedisWriteErrorMonitor struct {
	name     string
	errors   atomic.Uint64
	stopFunc func(uint64)
}

func NewRedisWriteErrorMonitor(name string, stopFunc func(uint64)) *RedisWriteErrorMonitor {
	return &RedisWriteErrorMonitor{name: name, stopFunc: stopFunc}
}

func (m *RedisWriteErrorMonitor) Record(err error) {
	if err == nil || m == nil {
		return
	}
	m.errors.Add(1)
}

func (m *RedisWriteErrorMonitor) Start() {
	if m == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for range ticker.C {
			count := m.errors.Swap(0)
			if count >= RedisWriteErrorStopThresholdPerSec {
				log.Printf("❌ %s Redis write errors reached %d requests/sec, stopping all SSP adapter ORTB streams", m.name, count)
				if m.stopFunc != nil {
					m.stopFunc(count)
				}
				continue
			}

			if count >= RedisWriteErrorLogThresholdPerSec {
				log.Printf("❌ %s Redis write errors reached %d requests/sec", m.name, count)
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
