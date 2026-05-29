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
	"github.com/shopspring/decimal"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"google.golang.org/protobuf/proto"
)

func ProcessKafkaMessagesOrtb(
	ctx context.Context,
	reader *kafka.Reader,
	ch clickhouse.Conn,
	table string,
	batchSize int,
	timeoutSec int,
) error {
	readCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	messages := make([]kafka.Message, 0, batchSize)
	records := make([]*eventspb.OrtbEvent, 0, batchSize)

	for i := 0; i < batchSize; i++ {
		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			return err
		}

		record := &eventspb.OrtbEvent{}
		if err := proto.Unmarshal(msg.Value, record); err != nil {
			log.Printf("!!! Failed to parse Kafka ORTB protobuf message: %v", err)
			continue
		}

		if !hasDataOrtbProtoCH(record) {
			continue
		}

		records = append(records, record)
		messages = append(messages, msg)
	}

	if len(records) == 0 {
		return nil
	}

	if err := insertBatchOrtb(ctx, ch, table, records); err != nil {
		return err
	}

	if err := reader.CommitMessages(ctx, messages...); err != nil {
		log.Printf("⚠️ Failed to commit Kafka offsets: %v", err)
	} else {
		log.Printf("COMMITED ORTB %d", len(records))
	}

	return nil
}

func insertBatchOrtb(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []*eventspb.OrtbEvent,
) error {
	if len(records) == 0 {
		return nil
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
			bid_responses,
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
		return fmt.Errorf("PrepareBatch: %w", err)
	}

	for i, r := range records {
		u, err := uuid.Parse(r.Uuid)
		if err != nil {
			u = uuid.Nil
		}

		ts := time.UnixMilli(r.EventTimeMs).UTC()

		var ip *net.IP
		if r.Ip != "" {
			parsedIP := net.ParseIP(r.Ip)
			if parsedIP != nil {
				ip = &parsedIP
			}
		}

		var ipv6 *net.IP
		if r.Ipv6 != "" {
			parsedIP := net.ParseIP(r.Ipv6)
			if parsedIP != nil && parsedIP.To16() != nil {
				ipv6 = &parsedIP
			}
		}

		cityID := int32(r.CityId)

		bidFloor := decimal.NewFromFloat(r.BidFloor)

		bidResponses := r.BidResponses
		if bidResponses == nil {
			bidResponses = make(map[string]int32)
		}

		winFinalPrice := r.WinPrice
		winDspPrice := r.WinDspPrice

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
			bidFloor,
			&r.Geo,
			cityID,
			bidResponses,
			&r.WinDspDomain,
			winFinalPrice,
			winDspPrice,
			r.WinCid,
			r.WinCrid,
			r.WinUserId,
		); err != nil {
			log.Printf("Record %d: batch.Append error: %v, skipping record", i, err)
			continue
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("batch.Send: %w", err)
	}

	log.Printf("ORTB batch successfully sent to ClickHouse: %d", len(records))
	return nil
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
