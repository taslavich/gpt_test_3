package percenter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

const historyBucketName = "percenter_history_events"

type HistoryEvent struct {
	EventID   string    `json:"event_id"`
	EventTime time.Time `json:"event_time"`
	EventType string    `json:"event_type"`

	ExactSegmentHash             string `json:"exact_segment_hash,omitempty"`
	PreviousEffectiveSegmentHash string `json:"previous_effective_segment_hash,omitempty"`
	EffectiveSegmentHash         string `json:"effective_segment_hash,omitempty"`
	StateSegmentHash             string `json:"state_segment_hash,omitempty"`

	CampaignID      string `json:"campaign_id,omitempty"`
	CampaignVersion int64  `json:"campaign_version,omitempty"`
	TypeModel       int    `json:"type_model,omitempty"`
	ProfitModel     string `json:"profit_model,omitempty"`

	OldPointVersion uint64 `json:"old_point_version,omitempty"`
	NewPointVersion uint64 `json:"new_point_version,omitempty"`
	OldPhase        string `json:"old_phase,omitempty"`
	NewPhase        string `json:"new_phase,omitempty"`

	OldAdvertiserPrice float64 `json:"old_advertiser_price,omitempty"`
	NewAdvertiserPrice float64 `json:"new_advertiser_price,omitempty"`
	OldSSPBid          float64 `json:"old_ssp_bid,omitempty"`
	NewSSPBid          float64 `json:"new_ssp_bid,omitempty"`
	OldMargin          float64 `json:"old_margin,omitempty"`
	NewMargin          float64 `json:"new_margin,omitempty"`

	OldFallbackSegmentHash string `json:"old_fallback_segment_hash,omitempty"`
	NewFallbackSegmentHash string `json:"new_fallback_segment_hash,omitempty"`

	OriginalBid float64 `json:"original_bid,omitempty"`
	MinMargin   float64 `json:"min_margin,omitempty"`

	OldBenchmarkBuyout     float64 `json:"old_benchmark_buyout,omitempty"`
	NewBenchmarkBuyout     float64 `json:"new_benchmark_buyout,omitempty"`
	OldBaselineEfficiency  float64 `json:"old_baseline_efficiency,omitempty"`
	NewBaselineEfficiency  float64 `json:"new_baseline_efficiency,omitempty"`
	OldBestProfitPerReq    float64 `json:"old_best_profit_per_request,omitempty"`
	NewBestProfitPerReq    float64 `json:"new_best_profit_per_request,omitempty"`
	OldBestMargin          float64 `json:"old_best_margin,omitempty"`
	NewBestMargin          float64 `json:"new_best_margin,omitempty"`
	OldBestAdvertiserPrice float64 `json:"old_best_advertiser_price,omitempty"`
	NewBestAdvertiserPrice float64 `json:"new_best_advertiser_price,omitempty"`
	OldSSPLow              float64 `json:"old_ssp_low,omitempty"`
	NewSSPLow              float64 `json:"new_ssp_low,omitempty"`
	OldSSPHigh             float64 `json:"old_ssp_high,omitempty"`
	NewSSPHigh             float64 `json:"new_ssp_high,omitempty"`
	OldMarginStepIndex     int     `json:"old_margin_step_index,omitempty"`
	NewMarginStepIndex     int     `json:"new_margin_step_index,omitempty"`
	OldMarginDirection     int     `json:"old_margin_direction,omitempty"`
	NewMarginDirection     int     `json:"new_margin_direction,omitempty"`

	OldLastChangeAt         *time.Time `json:"old_last_change_at,omitempty"`
	NewLastChangeAt         *time.Time `json:"new_last_change_at,omitempty"`
	OldLastSSPReoptimizeAt  *time.Time `json:"old_last_ssp_reoptimize_at,omitempty"`
	NewLastSSPReoptimizeAt  *time.Time `json:"new_last_ssp_reoptimize_at,omitempty"`
	OldLastSimpleBaselineAt *time.Time `json:"old_last_simple_baseline_at,omitempty"`
	NewLastSimpleBaselineAt *time.Time `json:"new_last_simple_baseline_at,omitempty"`

	Requests           uint64  `json:"requests,omitempty"`
	Impressions        uint64  `json:"impressions,omitempty"`
	Clicks             uint64  `json:"clicks,omitempty"`
	AdvertiserSpend    float64 `json:"advertiser_spend,omitempty"`
	TwinBidProfit      float64 `json:"twinbid_profit,omitempty"`
	ClickTwinBidProfit float64 `json:"click_twinbid_profit,omitempty"`
	Buyout             float64 `json:"buyout,omitempty"`
	Efficiency         float64 `json:"efficiency,omitempty"`
	ProfitPerRequest   float64 `json:"profit_per_request,omitempty"`

	FallbackLevel       SegmentLevel `json:"fallback_level,omitempty"`
	FallbackImpressions uint64       `json:"fallback_impressions,omitempty"`
}

func historyTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}

func historyEventID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func StateUpdateHistoryEvent(before, after State, metrics Metrics, eventType string, now time.Time) HistoryEvent {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		eventType = "state_updated"
	}
	event := HistoryEvent{
		EventTime: now.UTC(), EventType: eventType,
		PreviousEffectiveSegmentHash: EffectiveStateHash(before), EffectiveSegmentHash: EffectiveStateHash(after), StateSegmentHash: after.SegmentHash,
		CampaignID: after.CampaignID, CampaignVersion: after.CampaignVersion, TypeModel: after.TypeModel, ProfitModel: after.ProfitModel,
		OldPointVersion: before.PointVersion, NewPointVersion: after.PointVersion, OldPhase: before.Phase, NewPhase: after.Phase,
		OldAdvertiserPrice: before.AdvertiserPrice, NewAdvertiserPrice: after.AdvertiserPrice, OldSSPBid: before.SSPBid, NewSSPBid: after.SSPBid, OldMargin: before.Margin, NewMargin: after.Margin,
		OldFallbackSegmentHash: before.FallbackSegmentHash, NewFallbackSegmentHash: after.FallbackSegmentHash,
		OriginalBid: after.OriginalBid, MinMargin: after.MinMargin,
		OldBenchmarkBuyout: before.BenchmarkBuyout, NewBenchmarkBuyout: after.BenchmarkBuyout,
		OldBaselineEfficiency: before.BaselineEfficiency, NewBaselineEfficiency: after.BaselineEfficiency,
		OldBestProfitPerReq: before.BestProfitPerReq, NewBestProfitPerReq: after.BestProfitPerReq,
		OldBestMargin: before.BestMargin, NewBestMargin: after.BestMargin,
		OldBestAdvertiserPrice: before.BestAdvertiserPrice, NewBestAdvertiserPrice: after.BestAdvertiserPrice,
		OldSSPLow: before.SSPLow, NewSSPLow: after.SSPLow, OldSSPHigh: before.SSPHigh, NewSSPHigh: after.SSPHigh,
		OldMarginStepIndex: before.MarginStepIndex, NewMarginStepIndex: after.MarginStepIndex,
		OldMarginDirection: before.MarginDirection, NewMarginDirection: after.MarginDirection,
		OldLastChangeAt: historyTime(before.LastChangeAt), NewLastChangeAt: historyTime(after.LastChangeAt),
		OldLastSSPReoptimizeAt: historyTime(before.LastSSPReoptimizeAt), NewLastSSPReoptimizeAt: historyTime(after.LastSSPReoptimizeAt),
		OldLastSimpleBaselineAt: historyTime(before.LastSimpleBaselineAt), NewLastSimpleBaselineAt: historyTime(after.LastSimpleBaselineAt),
		Requests: metrics.Requests, Impressions: metrics.Wins, Clicks: metrics.Clicks,
		AdvertiserSpend: metrics.AdvertiserSpend, TwinBidProfit: metrics.TwinBidProfit, ClickTwinBidProfit: metrics.ClickTwinBidProfit,
		Buyout: metrics.Buyout(), Efficiency: metrics.Efficiency(), ProfitPerRequest: metrics.ProfitPerRequestFor(after.ProfitModel),
	}
	event.EventID = historyEventID(event.EventType, event.StateSegmentHash, fmt.Sprint(event.NewPointVersion), fmt.Sprint(event.EventTime.UnixNano()))
	return event
}

func FallbackHistoryEvent(before, after State, decision FallbackDecision, previousEffectiveHash string, now time.Time) HistoryEvent {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	event := HistoryEvent{
		EventTime: now.UTC(), EventType: "fallback_route_changed",
		ExactSegmentHash: decision.ExactHash, PreviousEffectiveSegmentHash: strings.TrimSpace(previousEffectiveHash), EffectiveSegmentHash: strings.TrimSpace(decision.SelectedHash), StateSegmentHash: after.SegmentHash,
		CampaignID: after.CampaignID, CampaignVersion: after.CampaignVersion, TypeModel: after.TypeModel, ProfitModel: after.ProfitModel,
		OldPointVersion: before.PointVersion, NewPointVersion: after.PointVersion, OldPhase: before.Phase, NewPhase: after.Phase,
		OldAdvertiserPrice: before.AdvertiserPrice, NewAdvertiserPrice: after.AdvertiserPrice, OldSSPBid: before.SSPBid, NewSSPBid: after.SSPBid, OldMargin: before.Margin, NewMargin: after.Margin,
		OldFallbackSegmentHash: before.FallbackSegmentHash, NewFallbackSegmentHash: after.FallbackSegmentHash,
		OriginalBid: after.OriginalBid, MinMargin: after.MinMargin,
		OldBenchmarkBuyout: before.BenchmarkBuyout, NewBenchmarkBuyout: after.BenchmarkBuyout,
		OldBaselineEfficiency: before.BaselineEfficiency, NewBaselineEfficiency: after.BaselineEfficiency,
		OldBestProfitPerReq: before.BestProfitPerReq, NewBestProfitPerReq: after.BestProfitPerReq,
		OldBestMargin: before.BestMargin, NewBestMargin: after.BestMargin,
		OldBestAdvertiserPrice: before.BestAdvertiserPrice, NewBestAdvertiserPrice: after.BestAdvertiserPrice,
		OldSSPLow: before.SSPLow, NewSSPLow: after.SSPLow, OldSSPHigh: before.SSPHigh, NewSSPHigh: after.SSPHigh,
		OldMarginStepIndex: before.MarginStepIndex, NewMarginStepIndex: after.MarginStepIndex,
		OldMarginDirection: before.MarginDirection, NewMarginDirection: after.MarginDirection,
		OldLastChangeAt: historyTime(before.LastChangeAt), NewLastChangeAt: historyTime(after.LastChangeAt),
		OldLastSSPReoptimizeAt: historyTime(before.LastSSPReoptimizeAt), NewLastSSPReoptimizeAt: historyTime(after.LastSSPReoptimizeAt),
		OldLastSimpleBaselineAt: historyTime(before.LastSimpleBaselineAt), NewLastSimpleBaselineAt: historyTime(after.LastSimpleBaselineAt),
		FallbackLevel: decision.SelectedLevel, FallbackImpressions: decision.Impressions,
	}
	event.EventID = historyEventID(event.EventType, event.ExactSegmentHash, event.PreviousEffectiveSegmentHash, event.EffectiveSegmentHash, fmt.Sprint(event.NewPointVersion), fmt.Sprint(event.EventTime.UnixNano()))
	return event
}

func ValidateHistoryEvent(event HistoryEvent) error {
	if strings.TrimSpace(event.EventID) == "" {
		return fmt.Errorf("event_id is empty")
	}
	if event.EventTime.IsZero() {
		return fmt.Errorf("event_time is zero")
	}
	if strings.TrimSpace(event.EventType) == "" {
		return fmt.Errorf("event_type is empty")
	}
	if strings.TrimSpace(event.StateSegmentHash) == "" {
		return fmt.Errorf("state_segment_hash is empty")
	}
	return nil
}

func MarshalHistoryEvent(event HistoryEvent) ([]byte, error) {
	if err := ValidateHistoryEvent(event); err != nil {
		return nil, err
	}
	return json.Marshal(event)
}

func UnmarshalHistoryEvent(raw []byte) (HistoryEvent, error) {
	var event HistoryEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return HistoryEvent{}, err
	}
	if err := ValidateHistoryEvent(event); err != nil {
		return HistoryEvent{}, err
	}
	event.EventTime = event.EventTime.UTC()
	return event, nil
}

type HistoryOutbox struct{ db *bolt.DB }

func OpenHistoryOutbox(path string) (*HistoryOutbox, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("history outbox path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create history outbox directory: %w", err)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open history outbox: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error { _, err := tx.CreateBucketIfNotExists([]byte(historyBucketName)); return err }); err != nil {
		db.Close()
		return nil, err
	}
	return &HistoryOutbox{db: db}, nil
}
func (o *HistoryOutbox) Close() error {
	if o == nil || o.db == nil {
		return nil
	}
	return o.db.Close()
}
func (o *HistoryOutbox) Save(event HistoryEvent) error {
	if o == nil || o.db == nil {
		return fmt.Errorf("history outbox is unavailable")
	}
	raw, err := MarshalHistoryEvent(event)
	if err != nil {
		return err
	}
	return o.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(historyBucketName))
		key := []byte(event.EventID)
		if old := b.Get(key); old != nil {
			if string(old) != string(raw) {
				return fmt.Errorf("history event_id conflict: %s", event.EventID)
			}
			return nil
		}
		return b.Put(key, raw)
	})
}
func (o *HistoryOutbox) Delete(eventID string) error {
	if o == nil || o.db == nil {
		return fmt.Errorf("history outbox is unavailable")
	}
	return o.db.Update(func(tx *bolt.Tx) error { return tx.Bucket([]byte(historyBucketName)).Delete([]byte(eventID)) })
}
func (o *HistoryOutbox) List(limit int) ([]HistoryEvent, error) {
	if o == nil || o.db == nil {
		return nil, fmt.Errorf("history outbox is unavailable")
	}
	if limit <= 0 {
		limit = 100
	}
	out := make([]HistoryEvent, 0, limit)
	err := o.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte(historyBucketName)).Cursor()
		for k, v := c.First(); k != nil && len(out) < limit; k, v = c.Next() {
			e, err := UnmarshalHistoryEvent(append([]byte(nil), v...))
			if err != nil {
				return err
			}
			out = append(out, e)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].EventTime.Before(out[j].EventTime) })
	return out, err
}
