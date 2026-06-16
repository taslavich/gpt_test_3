package clickhouse_loader

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"google.golang.org/protobuf/proto"
)

type ortbInsertStats struct {
	badUUIDCount int
	badIPCount   int
	appendErrors int
}

func ProcessKafkaMessagesOrtb(
	ctx context.Context,
	reader *kafka.Reader,
	ch clickhouse.Conn,
	table string,
	batchSize int,
	timeoutSec int,
	timeoutMs int,
) (int, error) {
	if batchSize <= 0 {
		return 0, nil
	}

	start := time.Now()
	readCtx, cancel := context.WithTimeout(ctx, batchTimeout(timeoutSec, timeoutMs))
	defer cancel()

	records := make([]eventspb.OrtbEvent, 0, batchSize)
	commitMap := make(map[int]kafka.Message, 64)

	var (
		readCount     int
		badProtoCount int
		emptyCount    int
		commitOnlyCnt int
	)

	for len(records) < batchSize {
		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				break
			}
			return 0, err
		}

		readCount++

		var record eventspb.OrtbEvent
		if err := proto.Unmarshal(msg.Value, &record); err != nil {
			badProtoCount++
			commitOnlyCnt++
			rememberCommitMessage(commitMap, msg)
			continue
		}

		if !hasDataOrtbProtoCH(&record) {
			emptyCount++
			commitOnlyCnt++
			rememberCommitMessage(commitMap, msg)
			continue
		}

		records = append(records, record)
		rememberCommitMessage(commitMap, msg)
	}

	if len(records) > 0 {
		stats, err := insertBatchOrtb(ctx, ch, table, records)
		if err != nil {
			return 0, err
		}

		commitMessages := compactCommitMessages(commitMap)
		if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
			return 0, fmt.Errorf("commit ORTB offsets after successful insert failed: %w", err)
		}

		log.Printf("COMMITED ORTB records=%d offsets=%d", len(records), len(commitMessages))

		log.Printf(
			"ORTB batch: read=%d inserted=%d offsets=%d bad_proto=%d empty=%d commit_only=%d bad_uuid=%d bad_ip=%d append_errors=%d duration=%s",
			readCount,
			len(records),
			len(commitMessages),
			badProtoCount,
			emptyCount,
			commitOnlyCnt,
			stats.badUUIDCount,
			stats.badIPCount,
			stats.appendErrors,
			time.Since(start),
		)
		return len(records), nil
	}

	commitMessages := compactCommitMessages(commitMap)
	if len(commitMessages) > 0 {
		if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
			log.Printf("⚠️ Failed to commit skipped ORTB offsets: %v", err)
		}
	}

	if readCount > 0 {
		log.Printf(
			"ORTB batch: read=%d inserted=0 offsets=%d bad_proto=%d empty=%d commit_only=%d duration=%s",
			readCount,
			len(commitMessages),
			badProtoCount,
			emptyCount,
			commitOnlyCnt,
			time.Since(start),
		)
	}

	return 0, nil
}

func insertBatchOrtb(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []eventspb.OrtbEvent,
) (ortbInsertStats, error) {
	var stats ortbInsertStats
	if len(records) == 0 {
		return stats, nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			uuid,
			event_time,
			format,
			typic,
			spp_domain,
			ip,
			ipv6,
			lang,
			browser,
			browser_version,
			os,
			os_version,
			device,
			site_id,
			site_domain,
			bid_floor,
			geo,
			city_id,
			bid_responses_raw,
			win_dsp_domain,
			win_final_price,
			win_dsp_price,
			win_cid,
			win_crid,
			win_user_id
		)
	`, table)

	batch, err := ch.PrepareBatch(ctx, query)
	if err != nil {
		return stats, fmt.Errorf("PrepareBatch: %w", err)
	}

	for i := range records {
		r := &records[i]

		u, err := uuid.Parse(r.Uuid)
		if err != nil {
			stats.badUUIDCount++
			u = uuid.Nil
		}

		ts := time.UnixMilli(r.EventTimeMs).UTC()

		var ip *net.IP
		if r.Ip != "" {
			parsedIP := net.ParseIP(r.Ip)
			if parsedIP != nil && parsedIP.To4() != nil {
				ip = &parsedIP
			} else {
				stats.badIPCount++
			}
		}

		var ipv6 *net.IP
		if r.Ipv6 != "" {
			parsedIP := net.ParseIP(r.Ipv6)
			if parsedIP != nil && parsedIP.To16() != nil {
				ipv6 = &parsedIP
			} else {
				stats.badIPCount++
			}
		}

		cityID := int32(r.CityId)
		bidResponsesRaw := encodeBidResponsesRaw(r.BidResponses)

		if err := batch.Append(
			u,
			ts,
			r.Format,
			r.Typic,
			&r.SppDomain,
			ip,
			ipv6,
			&r.Lang,
			&r.Browser,
			&r.BrowserVersion,
			&r.Os,
			&r.OsVersion,
			&r.Device,
			&r.SiteId,
			&r.SiteDomain,
			r.BidFloor,
			&r.Geo,
			cityID,
			bidResponsesRaw,
			&r.WinDspDomain,
			r.WinPrice,
			r.WinDspPrice,
			r.WinCid,
			r.WinCrid,
			r.WinUserId,
		); err != nil {
			stats.appendErrors++
			log.Printf("Record %d: batch.Append error: %v, skipping record", i, err)
			continue
		}
	}

	if err := batch.Send(); err != nil {
		return stats, fmt.Errorf("batch.Send: %w", err)
	}

	return stats, nil
}

func encodeBidResponsesRaw(items map[string]int32) string {
	if len(items) == 0 {
		return ""
	}

	payload, err := proto.Marshal(&eventspb.BidResponses{Items: items})
	if err != nil {
		return ""
	}

	return string(payload)
}

func hasDataOrtbProtoCH(record *eventspb.OrtbEvent) bool {
	if record == nil {
		return false
	}

	return record.Uuid != "" && record.EventTimeMs != 0
}

func parseTimestampUTC(s string) time.Time {
	fallback := time.Unix(0, 0).UTC()

	if s == "" {
		return fallback
	}

	layouts := []string{
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
	}

	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t
		}
	}

	return fallback
}
