package clickhouse_loader

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
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
		clickhouseBatchConfig[eventspb.OrtbEvent]{
			LogName:    "ORTB",
			CommitName: "ORTB",
			Unmarshal:  unmarshalOrtbEvent,
			HasData:    hasDataOrtbProtoCH,
			Insert:     insertBatchOrtb,
		},
	)
}

func unmarshalOrtbEvent(value []byte) (eventspb.OrtbEvent, error) {
	var record eventspb.OrtbEvent
	if err := proto.Unmarshal(value, &record); err != nil {
		return eventspb.OrtbEvent{}, err
	}

	return record, nil
}

func insertBatchOrtb(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []eventspb.OrtbEvent,
) (clickhouseInsertStats, error) {
	var stats clickhouseInsertStats

	if len(records) == 0 {
		return stats, nil
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			uuid,
			logical_event_id,
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
			adv_rtb_responses_raw,
			win_dsp_domain,
			win_final_price,
			win_dsp_price,
			win_cid,
			win_crid,
			win_user_id,
			exact_segment_hash,
			segment_hash,
			percenter_point_version
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
		normalResponses, advRTBResponses, exactSegmentHash, segmentHash, pointVersion := splitOrtbBidResponses(r.BidResponses)
		bidResponsesRaw := encodeBidResponsesRaw(normalResponses)
		advRTBResponsesRaw := encodeBidResponsesRaw(advRTBResponses)

		logicalEventID := logicalOrtbEventID(r.Uuid, u)

		if err := batch.Append(
			u,
			logicalEventID,
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
			advRTBResponsesRaw,
			&r.WinDspDomain,
			r.WinPrice,
			r.WinDspPrice,
			r.WinCid,
			r.WinCrid,
			r.WinUserId,
			exactSegmentHash,
			segmentHash,
			pointVersion,
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

func logicalOrtbEventID(raw string, parsed uuid.UUID) string {
	id := strings.TrimSpace(raw)
	if id == "" {
		return parsed.String()
	}
	// Existing rows populated by ALTER use toString(uuid), i.e. canonical UUID
	// text. Canonicalize valid producer UUIDs as well so replay identity matches
	// both newly inserted and pre-existing rows. Preserve malformed raw IDs
	// instead of collapsing every bad UUID to uuid.Nil.
	if _, err := uuid.Parse(id); err == nil {
		return parsed.String()
	}
	return id
}

func splitOrtbBidResponses(items map[string]string) (map[string]string, map[string]string, string, string, uint64) {
	normalResponses := make(map[string]string)
	advRTBResponses := make(map[string]string)
	exactSegmentHash := ""
	segmentHash := ""
	var pointVersion uint64
	for key, value := range items {
		switch key {
		case constants.PercenterExactSegmentHashTransportKey:
			exactSegmentHash = strings.TrimSpace(value)
			continue
		case constants.PercenterSegmentHashTransportKey:
			segmentHash = strings.TrimSpace(value)
			continue
		case constants.PercenterPointVersionTransportKey:
			parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
			if err == nil {
				pointVersion = parsed
			}
			continue
		}
		if campaignID, ok := strings.CutPrefix(key, constants.ADVRTBResponseStatsPrefix); ok {
			advRTBResponses[campaignID] = value
			continue
		}
		normalResponses[key] = value
	}
	return normalResponses, advRTBResponses, exactSegmentHash, segmentHash, pointVersion
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
