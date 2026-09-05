package clickhouse_loader

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

func ProcessKafkaMessagesPercenterHistory(ctx context.Context, reader *kafka.Reader, ch clickhouse.Conn, table string, batchSize, timeoutSec, timeoutMs int) (int, error) {
	return processKafkaMessagesBatch(ctx, reader, ch, table, batchSize, timeoutSec, timeoutMs, clickhouseBatchConfig[percenter.HistoryEvent]{
		LogName: "PERCENTER_HISTORY", CommitName: "percenter_history",
		Unmarshal: func(raw []byte) (percenter.HistoryEvent, error) { return percenter.UnmarshalHistoryEvent(raw) },
		HasData:   func(e *percenter.HistoryEvent) bool { return e != nil && e.EventID != "" && !e.EventTime.IsZero() },
		Insert:    insertBatchPercenterHistory,
	})
}

func insertBatchPercenterHistory(ctx context.Context, ch clickhouse.Conn, table string, records []percenter.HistoryEvent) (clickhouseInsertStats, error) {
	var stats clickhouseInsertStats
	if len(records) == 0 {
		return stats, nil
	}
	query := fmt.Sprintf(`INSERT INTO %s (
        event_id,event_time,event_type,exact_segment_hash,previous_effective_segment_hash,effective_segment_hash,state_segment_hash,
        campaign_id,campaign_version,type_model,profit_model,old_point_version,new_point_version,old_phase,new_phase,
        old_advertiser_price,new_advertiser_price,old_ssp_bid,new_ssp_bid,old_margin,new_margin,old_fallback_segment_hash,new_fallback_segment_hash,
        original_bid,min_margin,old_benchmark_buyout,new_benchmark_buyout,old_baseline_efficiency,new_baseline_efficiency,
        old_best_profit_per_request,new_best_profit_per_request,old_best_margin,new_best_margin,old_best_advertiser_price,new_best_advertiser_price,
        old_ssp_low,new_ssp_low,old_ssp_high,new_ssp_high,old_margin_step_index,new_margin_step_index,old_margin_direction,new_margin_direction,
        old_last_change_at,new_last_change_at,old_last_ssp_reoptimize_at,new_last_ssp_reoptimize_at,old_last_simple_baseline_at,new_last_simple_baseline_at,
        requests,impressions,clicks,advertiser_spend,twinbid_profit,click_twinbid_profit,buyout,efficiency,profit_per_request,fallback_level,fallback_impressions
    )`, table)
	batch, err := ch.PrepareBatch(ctx, query)
	if err != nil {
		return stats, fmt.Errorf("prepare percenter history batch: %w", err)
	}
	for i := range records {
		e := &records[i]
		if err := batch.Append(
			e.EventID, e.EventTime.UTC(), e.EventType, e.ExactSegmentHash, e.PreviousEffectiveSegmentHash, e.EffectiveSegmentHash, e.StateSegmentHash,
			e.CampaignID, e.CampaignVersion, uint8(e.TypeModel), e.ProfitModel, e.OldPointVersion, e.NewPointVersion, e.OldPhase, e.NewPhase,
			e.OldAdvertiserPrice, e.NewAdvertiserPrice, e.OldSSPBid, e.NewSSPBid, e.OldMargin, e.NewMargin, e.OldFallbackSegmentHash, e.NewFallbackSegmentHash,
			e.OriginalBid, e.MinMargin, e.OldBenchmarkBuyout, e.NewBenchmarkBuyout, e.OldBaselineEfficiency, e.NewBaselineEfficiency,
			e.OldBestProfitPerReq, e.NewBestProfitPerReq, e.OldBestMargin, e.NewBestMargin, e.OldBestAdvertiserPrice, e.NewBestAdvertiserPrice,
			e.OldSSPLow, e.NewSSPLow, e.OldSSPHigh, e.NewSSPHigh, int32(e.OldMarginStepIndex), int32(e.NewMarginStepIndex), int8(e.OldMarginDirection), int8(e.NewMarginDirection),
			nullableHistoryTime(e.OldLastChangeAt), nullableHistoryTime(e.NewLastChangeAt), nullableHistoryTime(e.OldLastSSPReoptimizeAt), nullableHistoryTime(e.NewLastSSPReoptimizeAt), nullableHistoryTime(e.OldLastSimpleBaselineAt), nullableHistoryTime(e.NewLastSimpleBaselineAt),
			e.Requests, e.Impressions, e.Clicks, e.AdvertiserSpend, e.TwinBidProfit, e.ClickTwinBidProfit, e.Buyout, e.Efficiency, e.ProfitPerRequest, string(e.FallbackLevel), e.FallbackImpressions,
		); err != nil {
			stats.AppendErrors++
			return stats, fmt.Errorf("percenter history record %d append: %w", i, err)
		}
	}
	if err := batch.Send(); err != nil {
		return stats, fmt.Errorf("send percenter history batch: %w", err)
	}
	return stats, nil
}

func nullableHistoryTime(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	v := value.UTC()
	return &v
}
