package antiperekrut

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type NotifyFunc func(context.Context, string) error

type ClientConfig struct {
	Enabled        bool
	URLs           []string
	RequestTimeout time.Duration
	RetryInitial   time.Duration
	RetryMax       time.Duration
}

type StartupEvent struct {
	EventID        string `json:"event_id"`
	SourceService  string `json:"source_service"`
	SourceInstance string `json:"source_instance"`
	Reason         string `json:"reason"`
}

type restartACK struct {
	Generation int64 `json:"generation"`
}

type deliveryResult struct {
	url        string
	generation int64
	err        error
}

func NewStartupEvent(service, instance string) StartupEvent {
	service = strings.TrimSpace(service)
	instance = strings.TrimSpace(instance)
	if instance == "" {
		instance = service + "-unknown"
	}
	return StartupEvent{
		EventID: uuid.NewString(), SourceService: service,
		SourceInstance: instance, Reason: "startup",
	}
}

// FanoutStartupEvent performs one parallel delivery to every configured ADV
// replica. At least one durable ACK is required. Failed URLs continue retrying
// independently in the background until the process context is cancelled.
func FanoutStartupEvent(ctx context.Context, cfg ClientConfig, event StartupEvent, notify NotifyFunc) error {
	if !cfg.Enabled {
		return nil
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("invalid startup event id: %w", err)
	}
	urls, err := normalizeURLs(cfg.URLs)
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		return errors.New("ADV_SERVICE_CONTROL_URLS is empty while startup reset is enabled")
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 3 * time.Second
	}
	if cfg.RetryInitial <= 0 {
		cfg.RetryInitial = time.Second
	}
	if cfg.RetryMax <= 0 {
		cfg.RetryMax = time.Minute
	}

	pending := append([]string(nil), urls...)
	backoff := cfg.RetryInitial
	for {
		results := deliverParallel(ctx, cfg, event, pending)
		successes := 0
		failed := make([]string, 0, len(results))
		for _, result := range results {
			if result.err == nil {
				successes++
				continue
			}
			failed = append(failed, result.url)
			notifyFailure(ctx, notify, event, result.url, result.err)
		}
		if successes > 0 {
			for _, target := range failed {
				target := target
				go retryURL(ctx, cfg, event, target, notify)
			}
			return nil
		}

		// No durable ACK means the startup service must remain unready. Retry the
		// same event_id instead of exiting and creating an event storm on restart.
		if len(failed) == 0 {
			return errors.New("startup reset delivery produced no results")
		}
		pending = failed
		jitter := time.Duration(rand.Int63n(int64(maxDuration(backoff/4, time.Millisecond))))
		timer := time.NewTimer(backoff + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("startup reset was not durably acknowledged: %w", ctx.Err())
		case <-timer.C:
		}
		backoff *= 2
		if backoff > cfg.RetryMax {
			backoff = cfg.RetryMax
		}
	}
}

func deliverParallel(ctx context.Context, cfg ClientConfig, event StartupEvent, urls []string) []deliveryResult {
	results := make([]deliveryResult, 0, len(urls))
	ch := make(chan deliveryResult, len(urls))
	var wg sync.WaitGroup
	for _, target := range urls {
		target := target
		wg.Add(1)
		go func() {
			defer wg.Done()
			generation, err := deliverOnce(ctx, cfg, event, target)
			ch <- deliveryResult{url: target, generation: generation, err: err}
		}()
	}
	wg.Wait()
	close(ch)
	for result := range ch {
		results = append(results, result)
	}
	return results
}

func retryURL(ctx context.Context, cfg ClientConfig, event StartupEvent, target string, notify NotifyFunc) {
	backoff := cfg.RetryInitial
	for {
		jitter := time.Duration(rand.Int63n(int64(maxDuration(backoff/4, time.Millisecond))))
		timer := time.NewTimer(backoff + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if _, err := deliverOnce(ctx, cfg, event, target); err == nil {
			return
		} else {
			notifyFailure(ctx, notify, event, target, err)
		}
		backoff *= 2
		if backoff > cfg.RetryMax {
			backoff = cfg.RetryMax
		}
	}
}

func deliverOnce(parent context.Context, cfg ClientConfig, event StartupEvent, target string) (int64, error) {
	body, err := json.Marshal(event)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(parent, cfg.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(target, "/")+"/internal/antiperekrut/restart", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: cfg.RequestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 64<<10)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(limited)
		return 0, fmt.Errorf("ADV returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	var ack restartACK
	if err := json.NewDecoder(limited).Decode(&ack); err != nil {
		return 0, fmt.Errorf("decode ADV restart ACK: %w", err)
	}
	return ack.Generation, nil
}

func normalizeURLs(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		raw = strings.TrimRight(strings.TrimSpace(raw), "/")
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, fmt.Errorf("invalid ADV control URL %q", raw)
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	return out, nil
}

func notifyFailure(ctx context.Context, notify NotifyFunc, event StartupEvent, target string, err error) {
	if notify == nil || err == nil {
		return
	}
	_ = notify(ctx, fmt.Sprintf("[%s][ANTIPEREKRUT_STARTUP_RESET_ERROR] instance=%s event_id=%s adv_url=%s error=%v", event.SourceService, event.SourceInstance, event.EventID, target, err))
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
