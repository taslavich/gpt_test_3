package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
)

type BatchRatioManager struct {
	mu                        sync.RWMutex
	impressionsPercent        float64
	clicksPercent             float64
	defaultImpressionsPercent float64
	defaultClicksPercent      float64
	tickerEnabled             bool
	manualMode                bool
}

type LoaderControl struct {
	mu      sync.RWMutex
	running bool
	notify  chan struct{}
	onStart []func()

	version uint64
	changed chan struct{}
}

func NewLoaderControl(running bool) *LoaderControl {
	control := &LoaderControl{
		running: running,
		notify:  make(chan struct{}),
		changed: make(chan struct{}),
	}
	if running {
		close(control.notify)
	}
	return control
}

func (c *LoaderControl) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return
	}

	callbacks := append([]func(){}, c.onStart...)
	for _, callback := range callbacks {
		callback()
	}

	c.running = true
	c.version++

	close(c.notify)
	close(c.changed)
	c.changed = make(chan struct{})
}

func (c *LoaderControl) AddOnStart(callback func()) {
	if callback == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.onStart = append(c.onStart, callback)
}

func (c *LoaderControl) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}

	c.running = false
	c.version++

	c.notify = make(chan struct{})

	close(c.changed)
	c.changed = make(chan struct{})
}

func (c *LoaderControl) Running() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

func (c *LoaderControl) Status() string {
	if c.Running() {
		return "started"
	}
	return "stopped"
}

func (c *LoaderControl) Wait(ctx context.Context) error {
	for {
		c.mu.RLock()
		running := c.running
		notify := c.notify
		c.mu.RUnlock()

		if running {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-notify:
		}
	}
}

func (c *LoaderControl) state() (bool, uint64, <-chan struct{}) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.running, c.version, c.changed
}

func (c *LoaderControl) WaitIntervalAfterStart(ctx context.Context, interval time.Duration) error {
	for {
		if err := c.Wait(ctx); err != nil {
			return err
		}

		running, version, changed := c.state()
		if !running {
			continue
		}

		timer := time.NewTimer(interval)

		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()

		case <-changed:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			continue

		case <-timer.C:
			runningNow, versionNow, _ := c.state()
			if runningNow && versionNow == version {
				return nil
			}
		}
	}
}

type BatchRatioState struct {
	ImpressionsPercent float64 `json:"impressions_percent"`
	ClicksPercent      float64 `json:"clicks_percent"`
	TickerEnabled      bool    `json:"ticker_enabled"`
	ManualMode         bool    `json:"manual_mode"`
}

type BatchRatioUpdateRequest struct {
	ImpressionsPercent float64 `json:"impressions_percent"`
	ClicksPercent      float64 `json:"clicks_percent"`
}

func NewBatchRatioManager(impressionsPercent, clicksPercent float64, tickerEnabled bool) *BatchRatioManager {
	return &BatchRatioManager{
		impressionsPercent:        impressionsPercent,
		clicksPercent:             clicksPercent,
		defaultImpressionsPercent: impressionsPercent,
		defaultClicksPercent:      clicksPercent,
		tickerEnabled:             tickerEnabled,
	}
}

func (m *BatchRatioManager) State() BatchRatioState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return BatchRatioState{
		ImpressionsPercent: m.impressionsPercent,
		ClicksPercent:      m.clicksPercent,
		TickerEnabled:      m.tickerEnabled,
		ManualMode:         m.manualMode,
	}
}

func (m *BatchRatioManager) BatchSizes(ortbBatch int) (int, int) {
	state := m.State()
	return batchSizeFromPercent(ortbBatch, state.ImpressionsPercent), batchSizeFromPercent(ortbBatch, state.ClicksPercent)
}

func (m *BatchRatioManager) BatchSizesInt64(ortbBatch int64) (int64, int64) {
	state := m.State()
	return batchSizeFromPercentInt64(ortbBatch, state.ImpressionsPercent), batchSizeFromPercentInt64(ortbBatch, state.ClicksPercent)
}

func batchSizeFromPercent(ortbBatch int, percent float64) int {
	return int(math.Round(float64(ortbBatch) * percent))
}

func batchSizeFromPercentInt64(ortbBatch int64, percent float64) int64 {
	return int64(math.Round(float64(ortbBatch) * percent))
}

func (m *BatchRatioManager) SetManual(impressionsPercent, clicksPercent float64) error {
	if impressionsPercent < 0 || clicksPercent < 0 {
		return fmt.Errorf("batch ratio percents cannot be negative")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.impressionsPercent = impressionsPercent
	m.clicksPercent = clicksPercent
	m.manualMode = true
	m.tickerEnabled = false
	return nil
}

func (m *BatchRatioManager) EnableTicker() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.manualMode = false
	m.tickerEnabled = true
}

func (m *BatchRatioManager) PauseTicker() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickerEnabled = false
}

func (m *BatchRatioManager) updateFromTicker(impressionsPercent, clicksPercent float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.tickerEnabled || m.manualMode {
		return
	}
	if impressionsPercent == 0 && clicksPercent == 0 {
		m.impressionsPercent = m.defaultImpressionsPercent
		m.clicksPercent = m.defaultClicksPercent
		return
	}
	m.impressionsPercent = impressionsPercent
	m.clicksPercent = clicksPercent
}

func (m *BatchRatioManager) TickerEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tickerEnabled && !m.manualMode
}

func (m *BatchRatioManager) StartClickHouseTicker(ctx context.Context, ch clickhouse.Conn, cfg config.BatchRatioConfig) {
	interval := time.Duration(cfg.TickerIntervalSec) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !m.TickerEnabled() {
					continue
				}

				impressionsPercent, clicksPercent, err := FetchBatchRatioWithRetries(ctx, ch, cfg)
				if err != nil {
					log.Printf("⚠️ failed to fetch batch ratio from ClickHouse after retries, keeping previous value: %v", err)
					continue
				}

				m.updateFromTicker(impressionsPercent, clicksPercent)
				log.Printf("✅ batch ratio updated from ClickHouse: impressions=%.4f clicks=%.4f", impressionsPercent, clicksPercent)
			}
		}
	}()
}

func FetchBatchRatioWithRetries(ctx context.Context, ch clickhouse.Conn, cfg config.BatchRatioConfig) (float64, float64, error) {
	attempts := cfg.TickerRetryAttempts
	if attempts <= 0 {
		attempts = 3
	}

	requestTimeout := time.Duration(cfg.TickerRequestTimeoutMS) * time.Millisecond
	if requestTimeout <= 0 {
		requestTimeout = 2 * time.Second
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		select {
		case <-ctx.Done():
			return 0, 0, ctx.Err()
		default:
		}

		requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		impressionsPercent, clicksPercent, err := FetchBatchRatio(requestCtx, ch, cfg)
		cancel()
		if err == nil {
			return impressionsPercent, clicksPercent, nil
		}

		lastErr = err
		log.Printf("⚠️ failed to fetch batch ratio from ClickHouse attempt %d/%d: %v", attempt, attempts, err)
	}

	return 0, 0, lastErr
}

func FetchBatchRatio(ctx context.Context, ch clickhouse.Conn, cfg config.BatchRatioConfig) (float64, float64, error) {
	if strings.TrimSpace(cfg.Table) == "" {
		return 0, 0, fmt.Errorf("BATCH_RATIO_TABLE is empty")
	}

	query := fmt.Sprintf(
		"SELECT %s, %s FROM %s ORDER BY %s DESC LIMIT 1",
		quoteIdentifier(cfg.ImpressionsPercentColumn),
		quoteIdentifier(cfg.ClicksPercentColumn),
		quoteTable(cfg.Table),
		quoteIdentifier(cfg.OrderColumn),
	)

	var impressionsPercent float64
	var clicksPercent float64
	if err := ch.QueryRow(ctx, query).Scan(&impressionsPercent, &clicksPercent); err != nil {
		return 0, 0, err
	}
	if impressionsPercent < 0 || clicksPercent < 0 {
		return 0, 0, fmt.Errorf("batch ratio percents cannot be negative")
	}
	return impressionsPercent, clicksPercent, nil
}

func (m *BatchRatioManager) StartHTTPServer(ctx context.Context, cfg config.BatchRatioConfig, loaderControl *LoaderControl) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/batch-ratio", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, m.State())
		case http.MethodPost:
			var req BatchRatioUpdateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := m.SetManual(req.ImpressionsPercent, req.ClicksPercent); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, m.State())
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/batch-ratio/ticker/pause", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		m.PauseTicker()
		writeJSON(w, m.State())
	})
	mux.HandleFunc("/batch-ratio/ticker/resume", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		m.EnableTicker()
		writeJSON(w, m.State())
	})
	mux.HandleFunc("/loader/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if loaderControl != nil {
			loaderControl.Start()
		}
		writeJSON(w, map[string]string{"status": "started"})
	})
	mux.HandleFunc("/loader/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if loaderControl != nil {
			loaderControl.Stop()
		}
		writeJSON(w, map[string]string{"status": "stopped"})
	})
	mux.HandleFunc("/loader/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if loaderControl == nil {
			writeJSON(w, map[string]any{"status": "unknown", "running": false})
			return
		}
		writeJSON(w, map[string]any{
			"status":  loaderControl.Status(),
			"running": loaderControl.Running(),
		})
	})

	server := &http.Server{Addr: net.JoinHostPort(cfg.HTTPHost, fmt.Sprint(cfg.HTTPPort)), Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		log.Printf("✅ batch ratio HTTP server listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ batch ratio HTTP server error: %v", err)
		}
	}()
	return server
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("⚠️ failed to write JSON response: %v", err)
	}
}

var identifierRegexp = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func quoteTable(table string) string {
	parts := strings.Split(table, ".")
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, quoteIdentifier(part))
	}
	return strings.Join(quoted, ".")
}

func quoteIdentifier(identifier string) string {
	identifier = strings.TrimSpace(identifier)
	if identifierRegexp.MatchString(identifier) {
		return identifier
	}
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}
