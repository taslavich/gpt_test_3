package clickhouse_loader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
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

	var (
		messages []kafka.Message
		records  []types.Ortb
	)

	for i := 0; i < batchSize; i++ {

		msg, err := reader.FetchMessage(readCtx)
		if err != nil {
			// таймаут — значит новых сообщений пока нет
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			return err
		}

		var record types.Ortb
		if err := json.Unmarshal(msg.Value, &record); err != nil {
			log.Printf("!!! Failed to parse Kafka message: %v", err)
			continue
		}

		if !types.HasDataOrtb(record) {
			continue
		}

		records = append(records, record)
		messages = append(messages, msg)
	}

	if len(records) == 0 {
		time.Sleep(1 * time.Second)
		return nil
	}

	// native batch insert в ClickHouse
	if err := insertBatchOrtb(ctx, ch, table, records); err != nil {
		return err
	}

	// коммит offsets ТОЛЬКО после успешной вставки
	if err := reader.CommitMessages(ctx, messages...); err != nil {
		log.Printf("⚠️ Failed to commit Kafka offsets: %v", err)
	} else {
		log.Println("COMMITED %d", len(records))
	}

	return nil
}

func insertBatchOrtb(
	ctx context.Context,
	ch clickhouse.Conn,
	table string,
	records []types.Ortb,
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
			win_flag,
			win_cid,
			win_crid,
			win_user_id
		)
	`, table)

	batch, err := ch.PrepareBatch(ctx, query)
	if err != nil {
		log.Printf("PrepareBatch error: %v", err)
		return fmt.Errorf("PrepareBatch: %w", err)
	}

	for i, r := range records {
		// UUID (только это поле парсим как UUID)
		u, err := uuid.Parse(r.UUID)
		if err != nil {
			log.Printf("Record %d: Cannot parse UUID '%s': %v, using nil UUID", i, r.UUID, err)
			u = uuid.Nil
		}

		// EVENT_TIME
		ts := parseTimestampUTC(r.EVENT_TIME)
		if ts.IsZero() && r.EVENT_TIME != "" {
			log.Printf("Record %d: Failed to parse EVENT_TIME '%s'", i, r.EVENT_TIME)
		}

		// TYPIC (string to int16)
		var typic int16
		if r.TYPIC != "" {
			typic64, err := strconv.ParseInt(r.TYPIC, 10, 16)
			if err != nil {
				log.Printf("Record %d: Cannot parse TYPIC '%s': %v, using 0", i, r.TYPIC, err)
				typic = 0
			} else {
				typic = int16(typic64)
			}
		}

		// IP (string to IPv4)
		var ip *net.IP
		if r.IP != "" {
			parsedIP := net.ParseIP(r.IP)
			if parsedIP != nil && parsedIP.To4() != nil {
				ip = &parsedIP
			} else {
				log.Printf("Record %d: Cannot parse IP '%s', using nil", i, r.IP)
			}
		}

		// IPV6 (string to IPv6)
		var ipv6 *net.IP
		if r.IPV6 != "" {
			parsedIP := net.ParseIP(r.IPV6)
			if parsedIP != nil && parsedIP.To16() != nil {
				ipv6 = &parsedIP
			} else {
				log.Printf("Record %d: Cannot parse IPV6 '%s', using nil", i, r.IPV6)
			}
		}

		// CITY_ID (string to uint32)
		var cityID uint32
		if r.CITY_ID != "" {
			id64, err := strconv.ParseUint(r.CITY_ID, 10, 32)
			if err != nil {
				log.Printf("Record %d: Cannot parse CITY_ID '%s': %v, using 0", i, r.CITY_ID, err)
				cityID = 0
			} else {
				cityID = uint32(id64)
			}
		}

		// BID_FLOOR (string to decimal)
		var bidFloor decimal.Decimal
		if r.BID_FLOOR != "" {
			bidFloor, err = decimal.NewFromString(r.BID_FLOOR)
			if err != nil {
				log.Printf("Record %d: Cannot parse BID_FLOOR '%s': %v, using 0", i, r.BID_FLOOR, err)
				bidFloor = decimal.NewFromInt(0)
			}
		} else {
			bidFloor = decimal.NewFromInt(0)
		}

		// BID_RESPONSES (string JSON to Map)
		var bidResponses map[string]int32
		if r.BID_RESPONSES != "" {
			if err := json.Unmarshal([]byte(r.BID_RESPONSES), &bidResponses); err != nil {
				log.Printf("Record %d: Cannot parse BID_RESPONSES '%s': %v, using empty map", i, r.BID_RESPONSES, err)
				bidResponses = make(map[string]int32)
			}
		} else {
			bidResponses = make(map[string]int32)
		}

		// WIN_PRICE (string to float64)
		var winFinalPrice float64
		if r.WIN_PRICE != "" {
			parsed, err := strconv.ParseFloat(r.WIN_PRICE, 64)
			if err != nil {
				log.Printf("Record %d: Cannot parse WIN_PRICE '%s': %v, using 0", i, r.WIN_PRICE, err)
				winFinalPrice = 0
			} else {
				winFinalPrice = parsed
			}
		}

		// WIN_DSP_PRICE (string to float64)
		var winDspPrice float64
		if r.WIN_DSP_PRICE != "" {
			parsed, err := strconv.ParseFloat(r.WIN_DSP_PRICE, 64)
			if err != nil {
				log.Printf("Record %d: Cannot parse WIN_DSP_PRICE '%s': %v, using 0", i, r.WIN_DSP_PRICE, err)
				winDspPrice = 0
			} else {
				winDspPrice = parsed
			}
		}

		// WIN_FLAG (string to bool)
		var winFlag bool
		if r.WIN_FLAG != "" {
			parsed, err := strconv.ParseBool(r.WIN_FLAG)
			if err != nil {
				log.Printf("Record %d: Cannot parse WIN_FLAG '%s': %v, using false", i, r.WIN_FLAG, err)
				winFlag = false
			} else {
				winFlag = parsed
			}
		}

		// CID (простая строка, не UUID)
		winCid := r.WIN_CID

		// CRID (простая строка, не UUID)
		winCrid := r.WIN_CRID

		// USER_ID (простая строка, не UUID)
		winUserID := r.WIN_USER_ID

		if err := batch.Append(
			u,                  // uuid (UUID)
			ts,                 // event_time
			r.FORMAT,           // format
			typic,              // typic
			&r.SPP_DOMAIN,      // spp_domain (nullable)
			ip,                 // ip (nullable)
			ipv6,               // ipv6 (nullable)
			&r.LANG,            // lang (nullable)
			&r.BROWSER,         // browser (nullable)
			&r.BROWSER_VERSION, // browser_version (nullable)
			&r.OS,              // os (nullable)
			&r.OS_VERSION,      // os_version (nullable)
			&r.DEVICE,          // device (nullable)
			&r.SITE_ID,         // site_id (nullable)
			&r.SITE_DOMAIN,     // site_domain (nullable)
			bidFloor,           // bid_floor
			&r.GEO,             // geo (nullable)
			cityID,             // city_id (nullable)
			bidResponses,       // bid_responses
			&r.WIN_DSP_DOMAIN,  // win_dsp_domain (nullable)
			winFinalPrice,      // win_final_price
			winDspPrice,        // win_dsp_price
			winFlag,            // win_flag
			winCid,             // cid (строка)
			winCrid,            // crid (строка)
			winUserID,          // user_id (строка)
		); err != nil {
			log.Printf("Record %d: batch.Append error: %v, skipping record", i, err)
			continue
		}
	}

	if err := batch.Send(); err != nil {
		log.Printf("batch.Send error: %v", err)
		return fmt.Errorf("batch.Send: %w", err)
	}

	log.Printf("Batch successfully sent to ClickHouse")
	return nil
}

func parseTimestampUTC(s string) time.Time {
	// fallback: начало Unix-времени
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
