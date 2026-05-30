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
<<<<<<< HEAD
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
=======
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"google.golang.org/protobuf/proto"
>>>>>>> opt
)

type ortbInsertStats struct {
	badUUIDCount int
	badIPCount   int
}

func ProcessKafkaMessagesOrtb(
	ctx context.Context,
	reader *kafka.Reader,
	ch clickhouse.Conn,
	table string,
	batchSize int,
	timeoutSec int,
<<<<<<< HEAD
	timeoutMs int,
) error {
	if batchSize <= 0 {
		return nil
	}

	start := time.Now()
	readCtx, cancel := context.WithTimeout(ctx, batchTimeout(timeoutSec, timeoutMs))
	defer cancel()

	records := make([]types.Ortb, 0, batchSize)
	commitMap := make(map[int]kafka.Message, 64)

	var (
		readCount     int
		badJSONCount  int
		emptyCount    int
		commitOnlyCnt int
=======
	timeoutMS int,
) error {
	start := time.Now()
	readCtx, cancel := context.WithTimeout(ctx, batchTimeout(timeoutSec, timeoutMS))
	defer cancel()

	commitMap := make(map[int]kafka.Message, 64)
	records := make([]eventspb.OrtbEvent, 0, batchSize)

	var (
		readCount     int
		badProtoCount int
		emptyCount    int
>>>>>>> opt
	)

	for len(records) < batchSize {
		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
<<<<<<< HEAD
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
=======
			if errors.Is(err, context.DeadlineExceeded) {
>>>>>>> opt
				break
			}
			return err
		}

		readCount++
<<<<<<< HEAD

		var record types.Ortb
		if err := json.Unmarshal(msg.Value, &record); err != nil {
			badJSONCount++
			commitOnlyCnt++
			rememberCommitMessage(commitMap, msg)
			continue
		}

		if !types.HasDataOrtb(record) {
			emptyCount++
			commitOnlyCnt++
			rememberCommitMessage(commitMap, msg)
=======
		rememberCommitMessage(commitMap, msg)

		var record eventspb.OrtbEvent
		if err := proto.Unmarshal(msg.Value, &record); err != nil {
			badProtoCount++
			continue
		}

		if !hasDataOrtbProtoCH(&record) {
			emptyCount++
>>>>>>> opt
			continue
		}

		records = append(records, record)
<<<<<<< HEAD
		rememberCommitMessage(commitMap, msg)
	}

	if len(records) > 0 {
		stats, err := insertBatchOrtb(ctx, ch, table, records)
		if err != nil {
			return err
		}

		commitMessages := compactCommitMessages(commitMap)
		if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
			return fmt.Errorf("commit ORTB offsets after successful insert: %w", err)
		}

		log.Printf(
			"ORTB batch: read=%d inserted=%d offsets=%d bad_json=%d empty=%d commit_only=%d bad_uuid=%d bad_ip=%d duration=%s",
			readCount,
			len(records),
			len(commitMessages),
			badJSONCount,
			emptyCount,
			commitOnlyCnt,
			stats.badUUIDCount,
			stats.badIPCount,
			time.Since(start),
		)
		return nil
	}

	commitMessages := compactCommitMessages(commitMap)
	if len(commitMessages) > 0 {
		if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
			return fmt.Errorf("commit ORTB skipped offsets: %w", err)
		}
		log.Printf(
			"ORTB batch: read=%d inserted=0 offsets=%d bad_json=%d empty=%d commit_only=%d duration=%s",
			readCount,
			len(commitMessages),
			badJSONCount,
			emptyCount,
			commitOnlyCnt,
			time.Since(start),
		)
=======
	}

	if len(records) == 0 {
		if len(commitMap) > 0 {
			commitMessages := compactCommitMessages(commitMap)
			if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
				return fmt.Errorf("commit skipped ORTB offsets: %w", err)
			}
		}

		return nil
	}

	stats, err := insertBatchOrtb(ctx, ch, table, records)
	if err != nil {
		return err
	}

	commitMessages := compactCommitMessages(commitMap)
	if err := reader.CommitMessages(ctx, commitMessages...); err != nil {
		return fmt.Errorf("commit ORTB offsets after insert: %w", err)
>>>>>>> opt
	}

	log.Printf("COMMITED ORTB records=%d offsets=%d", len(records), len(commitMessages))

	log.Printf(
		"ORTB batch: read=%d inserted=%d bad_proto=%d empty=%d bad_uuid=%d bad_ip=%d append_errors=%d duration=%s",
		readCount,
		len(records),
		badProtoCount,
		emptyCount,
		stats.badUUIDCount,
		stats.badIPCount,
		stats.appendErrors,
		time.Since(start),
	)

	return nil
}

type ortbInsertStats struct {
	badUUIDCount int
	badIPCount   int
	appendErrors int
}

func insertBatchOrtb(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
<<<<<<< HEAD
	records []types.Ortb,
) (ortbInsertStats, error) {
	var stats ortbInsertStats
=======
	records []eventspb.OrtbEvent,
) (ortbInsertStats, error) {
	stats := ortbInsertStats{}

>>>>>>> opt
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

	for _, r := range records {
<<<<<<< HEAD
		u, err := uuid.Parse(r.UUID)
=======
		u, err := uuid.Parse(r.Uuid)
>>>>>>> opt
		if err != nil {
			stats.badUUIDCount++
			u = uuid.Nil
		}

<<<<<<< HEAD
		ts := parseTimestampUTC(r.EVENT_TIME)
=======
		ts := time.UnixMilli(r.EventTimeMs).UTC()
>>>>>>> opt

		var ip *net.IP
		if r.Ip != "" {
			parsedIP := net.ParseIP(r.Ip)
			if parsedIP != nil {
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

<<<<<<< HEAD
		var cityID int32
		if r.CITY_ID != "" {
			id64, err := strconv.ParseInt(r.CITY_ID, 10, 32)
			if err == nil {
				cityID = int32(id64)
			}
		}

		var bidFloor float64
		if r.BID_FLOOR != "" {
			parsed, err := strconv.ParseFloat(r.BID_FLOOR, 64)
			if err == nil {
				bidFloor = parsed
			}
		}

		var winFinalPrice float64
		if r.WIN_PRICE != "" {
			parsed, err := strconv.ParseFloat(r.WIN_PRICE, 64)
			if err == nil {
				winFinalPrice = parsed
			}
		}

		var winDspPrice float64
		if r.WIN_DSP_PRICE != "" {
			parsed, err := strconv.ParseFloat(r.WIN_DSP_PRICE, 64)
			if err == nil {
				winDspPrice = parsed
			}
		}
=======
		cityID := int32(r.CityId)
		bidResponsesRaw := marshalBidResponsesRaw(r.BidResponses)
>>>>>>> opt

		if err := batch.Append(
			u,
			ts,
<<<<<<< HEAD
			r.FORMAT,
			r.TYPIC,
			&r.SPP_DOMAIN,
			ip,
			ipv6,
			&r.LANG,
			&r.BROWSER,
			&r.BROWSER_VERSION,
			&r.OS,
			&r.OS_VERSION,
			&r.DEVICE,
			&r.SITE_ID,
			&r.SITE_DOMAIN,
			bidFloor,
			&r.GEO,
			cityID,
			r.BID_RESPONSES,
			&r.WIN_DSP_DOMAIN,
			winFinalPrice,
			winDspPrice,
			r.WIN_CID,
			r.WIN_CRID,
			r.WIN_USER_ID,
		); err != nil {
=======
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
>>>>>>> opt
			return stats, fmt.Errorf("batch.Append: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return stats, fmt.Errorf("batch.Send: %w", err)
	}

	return stats, nil
<<<<<<< HEAD
=======
}

func hasDataOrtbProtoCH(record *eventspb.OrtbEvent) bool {
	if record == nil {
		return false
	}

	return record.Uuid != "" && record.EventTimeMs != 0
}

func marshalBidResponsesRaw(items map[string]int32) string {
	if len(items) == 0 {
		return ""
	}

	payload, err := proto.Marshal(&eventspb.BidResponses{Items: items})
	if err != nil {
		return ""
	}

	return string(payload)
>>>>>>> opt
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
