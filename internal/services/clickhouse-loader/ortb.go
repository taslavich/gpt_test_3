package clickhouse_loader

import (
	"context"
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

func ProcessKafkaMessagesOrtb(
	ctx context.Context,
	reader *kafka.Reader,
	ch clickhouse.Conn,
	table string,
	batchSize int,
	timeoutSec int,
	timeoutMs int,
) (int, error) {
	return processKafkaMessagesBatch(
		ctx,
		reader,
		ch,
		table,
		batchSize,
		timeoutSec,
		timeoutMs,
		clickhouseBatchConfig[*eventspb.OrtbEvent]{
			LogName:    "ORTB",
			CommitName: "ORTB",
			Unmarshal:  unmarshalOrtbEvent,
			HasData:    hasDataOrtbProtoCH,
			Insert:     insertBatchOrtb,
		},
	)
}

func unmarshalOrtbEvent(value []byte) (*eventspb.OrtbEvent, error) {
	var record eventspb.OrtbEvent
	if err := proto.Unmarshal(value, &record); err != nil {
		return nil, err
	}

	return &record, nil
}

func insertBatchOrtb(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []*eventspb.OrtbEvent,
) (clickhouseInsertStats, error) {
	var stats clickhouseInsertStats

	if len(records) == 0 {
		return stats, nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			uuid,
			event_time,
			code,
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
		r := records[i]

		u, err := uuid.Parse(r.Uuid)
		if err != nil {
			stats.BadUUIDCount++
			u = uuid.Nil
		}

		ts := time.Now().UTC()
		if r.EventTimeMs != 0 {
			ts = time.UnixMilli(r.EventTimeMs).UTC()
		}

		var ip *net.IP
		if r.Ip != "" {
			parsedIP := net.ParseIP(r.Ip)
			if parsedIP != nil && parsedIP.To4() != nil {
				ip = &parsedIP
			} else {
				stats.BadIPCount++
			}
		}

		var ipv6 *net.IP
		if r.Ipv6 != "" {
			parsedIP := net.ParseIP(r.Ipv6)
			if parsedIP != nil && parsedIP.To16() != nil {
				ipv6 = &parsedIP
			} else {
				stats.BadIPCount++
			}
		}

		cityID := int32(r.CityId)
		code := uint16(r.Code)
		bidResponsesRaw := encodeBidResponsesRaw(r.BidResponses)

		if err := batch.Append(
			u,
			ts,
			code,
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
			stats.AppendErrors++
			return stats, fmt.Errorf("Record %d: batch.Append: %w", i, err)
		}
	}

	if err := batch.Send(); err != nil {
		return stats, fmt.Errorf("batch.Send: %w", err)
	}

	return stats, nil
}

func encodeBidResponsesRaw(items map[string]string) string {
	if len(items) == 0 {
		//log.Printf("encodeBidResponsesRaw: empty items map, returning empty string")
		return ""
	}

	payload, err := proto.Marshal(&eventspb.BidResponses{Items: items})
	if err != nil {
		log.Printf("encodeBidResponsesRaw: failed to marshal BidResponses protobuf: %v", err)
		return ""
	}

	return string(payload)
}

func hasDataOrtbProtoCH(record *eventspb.OrtbEvent) bool {
	if record == nil {
		return false
	}

	return record.Uuid != ""
}
