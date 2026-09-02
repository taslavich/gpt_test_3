package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	goredis "github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

type apiError struct {
	Error string `json:"error"`
}

type metricView struct {
	SegmentHash        string  `json:"segment_hash"`
	PointVersion       uint64  `json:"point_version"`
	Requests           uint64  `json:"requests"`
	Wins               uint64  `json:"wins"`
	Clicks             uint64  `json:"clicks"`
	AdvertiserSpend    float64 `json:"advertiser_spend"`
	TwinBidProfit      float64 `json:"twinbid_profit"`
	ClickTwinBidProfit float64 `json:"click_twinbid_profit"`
	WinRate            float64 `json:"win_rate"`
	Efficiency         float64 `json:"efficiency"`
	ProfitPerRequest   float64 `json:"profit_per_request"`
}

type timingView struct {
	NextEvaluationAt  *time.Time `json:"next_evaluation_at,omitempty"`
	NextRebenchmarkAt *time.Time `json:"next_rebenchmark_at,omitempty"`
}

type decisionView struct {
	WouldChange bool                   `json:"would_change"`
	Reason      string                 `json:"reason"`
	Guards      map[string]interface{} `json:"guards,omitempty"`
	NextState   *percenter.State       `json:"next_state,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, apiError{Error: message})
}

func (a *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	redisOK := false
	clickhouseOK := false
	redisError := ""
	clickhouseError := ""
	if a != nil && a.redis != nil {
		if err := a.redis.Ping(ctx).Err(); err != nil {
			redisError = err.Error()
		} else {
			redisOK = true
		}
	} else {
		redisError = "redis client is nil"
	}
	if a != nil && a.clickhouse != nil {
		if err := a.clickhouse.Ping(ctx); err != nil {
			clickhouseError = err.Error()
		} else {
			clickhouseOK = true
		}
	} else {
		clickhouseError = "clickhouse connection is nil"
	}

	status := http.StatusOK
	serviceStatus := "ok"
	if !redisOK || !clickhouseOK {
		status = http.StatusServiceUnavailable
		serviceStatus = "degraded"
	}
	writeJSON(w, status, map[string]interface{}{
		"status":       serviceStatus,
		"started_at":   a.startedAt,
		"current_time": time.Now().UTC(),
		"last_tick":    a.tickStatus(),
		"redis": map[string]interface{}{
			"ok":    redisOK,
			"error": redisError,
		},
		"clickhouse": map[string]interface{}{
			"ok":    clickhouseOK,
			"error": clickhouseError,
		},
	})
}

func (a *Server) handleSegmentState(w http.ResponseWriter, r *http.Request) {
	state, err := a.loadStateFromRequest(r)
	if err != nil {
		handleStateLoadError(w, err)
		return
	}
	ttl, err := a.redis.TTL(r.Context(), percenter.SegmentKey(state.SegmentHash)).Result()
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("redis TTL: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"state":       state,
		"timing":      a.stateTiming(state),
		"ttl_seconds": int64(ttl / time.Second),
	})
}

func (a *Server) handleSegmentMetrics(w http.ResponseWriter, r *http.Request) {
	state, err := a.loadStateFromRequest(r)
	if err != nil {
		handleStateLoadError(w, err)
		return
	}
	window, err := parseWindow(r, 3*a.policy.MarginOptimizeInterval)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	metric, found, err := a.loadCurrentMetrics(r.Context(), state, window)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("load metrics: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"found":   found,
		"window":  window.String(),
		"state":   state,
		"metrics": newMetricView(metric, state),
	})
}

func (a *Server) handleSegmentExplain(w http.ResponseWriter, r *http.Request) {
	state, err := a.loadStateFromRequest(r)
	if err != nil {
		handleStateLoadError(w, err)
		return
	}
	window, err := parseWindow(r, 3*a.policy.MarginOptimizeInterval)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	metric, found, err := a.loadCurrentMetrics(r.Context(), state, window)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("load metrics: %v", err))
		return
	}
	now := time.Now().UTC()
	decision := a.explainDecision(state, metric, found, now)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"state":    state,
		"timing":   a.stateTiming(state),
		"window":   window.String(),
		"found":    found,
		"metrics":  newMetricView(metric, state),
		"decision": decision,
	})
}

func (a *Server) handleSegmentEvaluate(w http.ResponseWriter, r *http.Request) {
	state, err := a.loadStateFromRequest(r)
	if err != nil {
		handleStateLoadError(w, err)
		return
	}
	window, err := parseWindow(r, 3*a.policy.MarginOptimizeInterval)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	metric, found, err := a.loadCurrentMetrics(r.Context(), state, window)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("load metrics: %v", err))
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"applied": false,
			"changed": false,
			"reason":  "no_metrics_for_current_point",
			"state":   state,
			"window":  window.String(),
			"metrics": newMetricView(metric, state),
		})
		return
	}

	now := time.Now().UTC()
	next, changed := percenter.Advance(state, metric, a.policy, now)
	dryRun := queryBool(r, "dry_run", false)
	response := map[string]interface{}{
		"applied":  false,
		"dry_run":  dryRun,
		"changed":  changed,
		"state":    state,
		"window":   window.String(),
		"metrics":  newMetricView(metric, state),
		"decision": a.explainDecision(state, metric, found, now),
	}
	if !changed || dryRun {
		if changed {
			response["next_state"] = next
		}
		log.Printf("[PERCENTER][HTTP][EVALUATE] segment_hash=%s dry_run=%t changed=%t applied=false", state.SegmentHash, dryRun, changed)
		writeJSON(w, http.StatusOK, response)
		return
	}

	saved, err := a.store.SaveIfCurrent(r.Context(), state, next)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("save state: %v", err))
		return
	}
	if !saved {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"applied": false,
			"changed": true,
			"reason":  "state_changed_concurrently_retry",
		})
		return
	}
	response["applied"] = true
	response["next_state"] = next
	log.Printf("[PERCENTER][HTTP][EVALUATE] segment_hash=%s dry_run=false changed=true applied=true point_version=%d", state.SegmentHash, next.PointVersion)
	writeJSON(w, http.StatusOK, response)
}

func (a *Server) handleSegmentRebenchmark(w http.ResponseWriter, r *http.Request) {
	state, err := a.loadStateFromRequest(r)
	if err != nil {
		handleStateLoadError(w, err)
		return
	}
	next := percenter.RebenchmarkState(state, time.Now().UTC())
	dryRun := queryBool(r, "dry_run", false)
	if dryRun {
		log.Printf("[PERCENTER][HTTP][REBENCHMARK] segment_hash=%s dry_run=true applied=false", state.SegmentHash)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"applied":    false,
			"dry_run":    true,
			"state":      state,
			"next_state": next,
		})
		return
	}
	saved, err := a.store.SaveIfCurrent(r.Context(), state, next)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("save state: %v", err))
		return
	}
	if !saved {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"applied": false,
			"reason":  "state_changed_concurrently_retry",
		})
		return
	}
	log.Printf("[PERCENTER][HTTP][REBENCHMARK] segment_hash=%s dry_run=false applied=true point_version=%d", state.SegmentHash, next.PointVersion)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"applied":    true,
		"state":      state,
		"next_state": next,
	})
}

func (a *Server) handleCampaignSegments(w http.ResponseWriter, r *http.Request) {
	campaignID := strings.TrimSpace(chi.URLParam(r, "campaignID"))
	if campaignID == "" {
		writeAPIError(w, http.StatusBadRequest, "campaignID is required")
		return
	}
	cursor, err := parseUintQuery(r, "cursor", 0)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := parseIntQuery(r, "limit", 200, 1, 500)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	states := make([]percenter.State, 0, limit)
	scanned := 0
	nextCursor := cursor
	for scanned < 10000 && len(states) < limit {
		keys, next, err := a.redis.Scan(r.Context(), nextCursor, "percenter:segment:*", int64(minInt(200, limit))).Result()
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("redis scan: %v", err))
			return
		}
		nextCursor = next
		scanned += len(keys)
		if len(keys) > 0 {
			values, err := a.redis.MGet(r.Context(), keys...).Result()
			if err != nil {
				writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("redis mget: %v", err))
				return
			}
			for _, value := range values {
				if value == nil {
					continue
				}
				var state percenter.State
				if err := json.Unmarshal([]byte(fmt.Sprint(value)), &state); err != nil {
					continue
				}
				if state.CampaignID == campaignID {
					states = append(states, state)
				}
			}
		}
		if nextCursor == 0 {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"campaign_id": campaignID,
		"segments":    states,
		"returned":    len(states),
		"scanned":     scanned,
		"next_cursor": strconv.FormatUint(nextCursor, 10),
		"truncated":   nextCursor != 0,
	})
}

func (a *Server) handleRedisKey(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if err := validatePercenterRedisKey(key); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	typeName, err := a.redis.Type(r.Context(), key).Result()
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("redis TYPE: %v", err))
		return
	}
	if typeName == "none" {
		writeAPIError(w, http.StatusNotFound, "redis key not found")
		return
	}
	ttl, err := a.redis.PTTL(r.Context(), key).Result()
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("redis PTTL: %v", err))
		return
	}
	response := map[string]interface{}{
		"key":    key,
		"type":   typeName,
		"ttl_ms": int64(ttl / time.Millisecond),
	}
	switch typeName {
	case "string":
		value, err := a.redis.Get(r.Context(), key).Result()
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("redis GET: %v", err))
			return
		}
		var decoded interface{}
		if json.Unmarshal([]byte(value), &decoded) == nil {
			response["value"] = decoded
		} else {
			response["value"] = value
		}
	case "hash":
		value, err := a.redis.HGetAll(r.Context(), key).Result()
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("redis HGETALL: %v", err))
			return
		}
		response["value"] = value
	default:
		response["value"] = nil
		response["note"] = "read-only proxy currently decodes string and hash values"
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *Server) handleRedisScan(w http.ResponseWriter, r *http.Request) {
	pattern := strings.TrimSpace(r.URL.Query().Get("pattern"))
	if pattern == "" {
		pattern = "percenter:*"
	}
	if !strings.HasPrefix(pattern, "percenter:") {
		writeAPIError(w, http.StatusBadRequest, "pattern must start with percenter:")
		return
	}
	cursor, err := parseUintQuery(r, "cursor", 0)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	count, err := parseIntQuery(r, "count", 100, 1, 500)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	keys, nextCursor, err := a.redis.Scan(r.Context(), cursor, pattern, int64(count)).Result()
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("redis scan: %v", err))
		return
	}
	type item struct {
		Key   string `json:"key"`
		Type  string `json:"type"`
		TTLMS int64  `json:"ttl_ms"`
	}
	items := make([]item, 0, len(keys))
	pipe := a.redis.Pipeline()
	typeCmds := make([]*goredis.StatusCmd, 0, len(keys))
	ttlCmds := make([]*goredis.DurationCmd, 0, len(keys))
	for _, key := range keys {
		typeCmds = append(typeCmds, pipe.Type(r.Context(), key))
		ttlCmds = append(ttlCmds, pipe.PTTL(r.Context(), key))
	}
	if _, err := pipe.Exec(r.Context()); err != nil && !errors.Is(err, goredis.Nil) {
		writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("redis pipeline: %v", err))
		return
	}
	for i, key := range keys {
		items = append(items, item{
			Key:   key,
			Type:  typeCmds[i].Val(),
			TTLMS: int64(ttlCmds[i].Val() / time.Millisecond),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pattern":     pattern,
		"cursor":      strconv.FormatUint(cursor, 10),
		"next_cursor": strconv.FormatUint(nextCursor, 10),
		"keys":        items,
	})
}

func (a *Server) loadStateFromRequest(r *http.Request) (percenter.State, error) {
	segmentHash := strings.TrimSpace(chi.URLParam(r, "segmentHash"))
	if segmentHash == "" {
		return percenter.State{}, fmt.Errorf("segment hash is required")
	}
	return a.store.Load(r.Context(), segmentHash)
}

func handleStateLoadError(w http.ResponseWriter, err error) {
	if errors.Is(err, goredis.Nil) {
		writeAPIError(w, http.StatusNotFound, "segment state not found")
		return
	}
	writeAPIError(w, http.StatusBadGateway, fmt.Sprintf("load segment state: %v", err))
}

func (a *Server) loadCurrentMetrics(ctx context.Context, state percenter.State, window time.Duration) (percenter.Metrics, bool, error) {
	return percenter.LoadSegmentWindowMetrics(
		ctx,
		a.clickhouse,
		a.cfg.Database,
		a.cfg.TableOrtb,
		a.cfg.TableImpressions,
		a.cfg.TableClicks,
		state.SegmentHash,
		state.PointVersion,
		window,
	)
}

func newMetricView(metric percenter.Metrics, state percenter.State) metricView {
	return metricView{
		SegmentHash:        metric.SegmentHash,
		PointVersion:       metric.PointVersion,
		Requests:           metric.Requests,
		Wins:               metric.Wins,
		Clicks:             metric.Clicks,
		AdvertiserSpend:    metric.AdvertiserSpend,
		TwinBidProfit:      metric.TwinBidProfit,
		ClickTwinBidProfit: metric.ClickTwinBidProfit,
		WinRate:            metric.Buyout(),
		Efficiency:         metric.Efficiency(),
		ProfitPerRequest:   metric.ProfitPerRequestFor(state.ProfitModel),
	}
}

func (a *Server) stateTiming(state percenter.State) timingView {
	result := timingView{}
	if !state.LastChangeAt.IsZero() {
		next := state.LastChangeAt.Add(a.policy.MarginOptimizeInterval)
		result.NextEvaluationAt = &next
	}
	if state.TypeModel == percenter.TypeModelSimple {
		if !state.LastSimpleBaselineAt.IsZero() {
			next := state.LastSimpleBaselineAt.Add(a.policy.SimpleBaselineReoptimizeInterval)
			result.NextRebenchmarkAt = &next
		}
	} else if !state.LastSSPReoptimizeAt.IsZero() {
		next := state.LastSSPReoptimizeAt.Add(a.policy.SSPReoptimizeInterval)
		result.NextRebenchmarkAt = &next
	}
	return result
}

func (a *Server) explainDecision(state percenter.State, metric percenter.Metrics, found bool, now time.Time) decisionView {
	decision := decisionView{Reason: "no_state_change_for_current_metrics"}
	if !found || metric.Requests == 0 {
		decision.Reason = "no_metrics_for_current_point"
		return decision
	}
	if !state.LastChangeAt.IsZero() && now.Sub(state.LastChangeAt) < a.policy.MarginOptimizeInterval {
		decision.Reason = "waiting_for_full_evaluation_interval"
		decision.Guards = map[string]interface{}{
			"eligible_at": state.LastChangeAt.Add(a.policy.MarginOptimizeInterval),
		}
		return decision
	}
	if state.TypeModel == percenter.TypeModelSimple && !state.LastSimpleBaselineAt.IsZero() && !now.Before(state.LastSimpleBaselineAt.Add(a.policy.SimpleBaselineReoptimizeInterval)) {
		next, changed := percenter.Advance(state, metric, a.policy, now)
		decision.WouldChange = changed
		decision.Reason = "simple_baseline_rebenchmark_due"
		if changed {
			decision.NextState = &next
		}
		return decision
	}
	if state.TypeModel != percenter.TypeModelSimple && !state.LastSSPReoptimizeAt.IsZero() && !now.Before(state.LastSSPReoptimizeAt.Add(a.policy.SSPReoptimizeInterval)) {
		next, changed := percenter.Advance(state, metric, a.policy, now)
		decision.WouldChange = changed
		decision.Reason = "ssp_rebenchmark_due"
		if changed {
			decision.NextState = &next
		}
		return decision
	}

	next, changed := percenter.Advance(state, metric, a.policy, now)
	decision.WouldChange = changed
	if changed {
		decision.NextState = &next
	}

	buyout := metric.Buyout()
	efficiency := metric.Efficiency()
	profit := metric.ProfitPerRequestFor(state.ProfitModel)
	switch state.Phase {
	case percenter.PhaseSimpleBaseline:
		if buyout <= 0 {
			decision.Reason = "baseline_win_rate_unavailable"
		} else {
			decision.Reason = "simple_baseline_ready"
		}
	case percenter.PhaseSimpleMarginSearch:
		retention := 0.0
		if state.BenchmarkBuyout > 0 {
			retention = buyout / state.BenchmarkBuyout
		}
		valid := state.BenchmarkBuyout > 0 && retention >= a.policy.SimpleWinRateRetention
		improved := valid && profit > state.BestProfitPerReq
		decision.Guards = map[string]interface{}{
			"win_rate_retention":          retention,
			"required_win_rate_retention": a.policy.SimpleWinRateRetention,
			"win_rate_guard_passed":       valid,
			"profit_per_request":          profit,
			"best_profit_per_request":     state.BestProfitPerReq,
			"profit_improved":             improved,
		}
		if !valid {
			decision.Reason = "win_rate_retention_below_threshold"
		} else if improved {
			decision.Reason = "profit_improved"
		} else {
			decision.Reason = "profit_not_improved"
		}
	case percenter.PhaseBenchmark, "":
		if buyout <= 0 {
			decision.Reason = "benchmark_buyout_unavailable"
		} else {
			decision.Reason = "benchmark_ready"
		}
	case percenter.PhaseSSPSearch:
		retention := 0.0
		if state.BenchmarkBuyout > 0 {
			retention = buyout / state.BenchmarkBuyout
		}
		passed := state.BenchmarkBuyout > 0 && retention >= a.policy.BuyoutRetention
		decision.Guards = map[string]interface{}{
			"buyout_retention":          retention,
			"required_buyout_retention": a.policy.BuyoutRetention,
			"buyout_guard_passed":       passed,
			"ssp_low":                   state.SSPLow,
			"ssp_high":                  state.SSPHigh,
			"ssp_search_precision":      a.policy.SSPSearchPrecision,
		}
		if passed {
			decision.Reason = "buyout_guard_passed"
		} else {
			decision.Reason = "buyout_guard_failed"
		}
	case percenter.PhaseMarginBaseline:
		if efficiency <= 0 {
			decision.Reason = "efficiency_baseline_unavailable"
		} else {
			decision.Reason = "efficiency_baseline_ready"
		}
	case percenter.PhaseMarginSearch:
		retention := 0.0
		if state.BaselineEfficiency > 0 {
			retention = efficiency / state.BaselineEfficiency
		}
		valid := state.BaselineEfficiency > 0 && retention >= a.policy.EfficiencyRetention
		improved := valid && profit > state.BestProfitPerReq
		decision.Guards = map[string]interface{}{
			"efficiency_retention":          retention,
			"required_efficiency_retention": a.policy.EfficiencyRetention,
			"efficiency_guard_passed":       valid,
			"profit_per_request":            profit,
			"best_profit_per_request":       state.BestProfitPerReq,
			"profit_improved":               improved,
		}
		if !valid {
			decision.Reason = "efficiency_retention_below_threshold"
		} else if improved {
			decision.Reason = "profit_improved"
		} else {
			decision.Reason = "profit_not_improved"
		}
	default:
		decision.Reason = "unknown_phase_will_reset"
	}
	return decision
}

func parseWindow(r *http.Request, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("window"))
	if raw == "" {
		return fallback, nil
	}
	window, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid window %q: %w", raw, err)
	}
	if window < time.Second || window > 24*time.Hour {
		return 0, fmt.Errorf("window must be between 1s and 24h")
	}
	return window, nil
}

func queryBool(r *http.Request, name string, fallback bool) bool {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func parseUintQuery(r *http.Request, name string, fallback uint64) (uint64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return value, nil
}

func parseIntQuery(r *http.Request, name string, fallback, minValue, maxValue int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	if value < minValue || value > maxValue {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minValue, maxValue)
	}
	return value, nil
}

func validatePercenterRedisKey(key string) error {
	if key == "" {
		return fmt.Errorf("key is required")
	}
	if !strings.HasPrefix(key, "percenter:") {
		return fmt.Errorf("only percenter:* keys can be read")
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
