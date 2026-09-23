package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

const maxTransactionRetries = 5

type Winner struct {
	Price        float64
	UserID       string
	CampaignID   string
	TypeModel    int
	Format       string
	ClickIDParam string
}

type Store struct {
	runtime   *redis.Client
	winners   *redis.Client
	postgres  *sql.DB
	markerTTL time.Duration
}

// NewStore keeps the previous call shape source-compatible. Production passes
// the PostgreSQL connection as the optional fourth argument so percenter promo
// spend is decremented durably together with ADV billing.
func NewStore(runtime, winners *redis.Client, markerTTL time.Duration, postgres ...*sql.DB) *Store {
	if markerTTL <= 0 {
		markerTTL = 720 * time.Hour
	}
	var db *sql.DB
	if len(postgres) > 0 {
		db = postgres[0]
	}
	return &Store{runtime: runtime, winners: winners, postgres: db, markerTTL: markerTTL}
}

func (s *Store) ReadWinner(ctx context.Context, winnerUUID, expectedFormat string) (Winner, error) {
	if s == nil || s.winners == nil {
		return Winner{}, errors.New("ADV winner Redis client is nil")
	}
	winnerUUID = strings.TrimSpace(winnerUUID)
	if winnerUUID == "" {
		return Winner{}, errors.New("winner UUID is empty")
	}
	values, err := s.winners.HGetAll(ctx, winnerUUID).Result()
	if err != nil {
		return Winner{}, fmt.Errorf("read ADV winner: %w", err)
	}
	if len(values) == 0 {
		return Winner{}, redis.Nil
	}
	price, err := strconv.ParseFloat(strings.TrimSpace(values["price"]), 64)
	if err != nil || price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return Winner{}, errors.New("ADV winner has invalid price")
	}
	format := normalizeFormat(values["format"])
	if format == "" || format != normalizeFormat(expectedFormat) {
		return Winner{}, fmt.Errorf("ADV winner format mismatch: got %s expected %s", format, expectedFormat)
	}
	typeModel, err := strconv.Atoi(strings.TrimSpace(values["type_model"]))
	if err != nil || typeModel < 1 || typeModel > 3 {
		// Winner records produced before stage 01 did not carry type_model and
		// therefore can only be treated as legacy Simple records.
		typeModel = 1
	}
	winner := Winner{
		Price:        price,
		UserID:       strings.TrimSpace(values["user_id"]),
		CampaignID:   strings.TrimSpace(values["campaign_id"]),
		TypeModel:    typeModel,
		Format:       format,
		ClickIDParam: strings.TrimSpace(values[constants.ADVWinnerClickIDParamField]),
	}
	if winner.UserID == "" || winner.CampaignID == "" {
		return Winner{}, errors.New("ADV winner has empty user_id or campaign_id")
	}
	return winner, nil
}

func (s *Store) Apply(ctx context.Context, record outbox.Record) error {
	if s == nil || s.runtime == nil {
		return errors.New("ADV runtime Redis client is nil")
	}
	if strings.TrimSpace(record.EventID) == "" || strings.TrimSpace(record.UserID) == "" || strings.TrimSpace(record.CampaignID) == "" || record.Price <= 0 || math.IsNaN(record.Price) || math.IsInf(record.Price, 0) {
		return errors.New("invalid ADV billing event")
	}
	if err := s.applyRuntimeSpend(ctx, record); err != nil {
		return err
	}
	if promoSpendAppliesToTypeModel(record.TypeModel) {
		if s.postgres == nil {
			return errors.New("PostgreSQL is required for percenter promo billing")
		}
		if err := s.applyPromoSpend(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func promoSpendAppliesToTypeModel(typeModel int) bool {
	// Promo is a user-level spend-window counter, not a discount tied to one
	// optimizer mode. Every valid campaign mode consumes it while traffic is
	// billed, including map-only and RTB fallback campaigns. Legacy records
	// without type_model are treated as Simple, as in ReadWinner.
	if typeModel == 0 {
		typeModel = 1
	}
	return typeModel == 1 || typeModel == 2 || typeModel == 3
}

func (s *Store) applyRuntimeSpend(ctx context.Context, record outbox.Record) error {
	markerKey := "outbox:applied:" + record.EventID
	currentKey := "pacing:current:" + record.CampaignID
	watchKeys := []string{markerKey, currentKey}
	var lastErr error
	for attempt := 0; attempt < maxTransactionRetries; attempt++ {
		err := s.runtime.Watch(ctx, func(tx *redis.Tx) error {
			applied, err := tx.Exists(ctx, markerKey).Result()
			if err != nil {
				return err
			}
			if applied > 0 {
				return nil
			}
			slotKey, err := tx.Get(ctx, currentKey).Result()
			if errors.Is(err, redis.Nil) {
				slotKey = ""
			} else if err != nil {
				return err
			}
			if slotKey != "" && !strings.HasPrefix(slotKey, "pacing:spent:"+record.CampaignID+":") {
				return fmt.Errorf("invalid pacing current value %q", slotKey)
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.IncrByFloat(ctx, "spent:user:"+record.UserID, record.Price)
				pipe.IncrByFloat(ctx, "spent:campaign:"+record.CampaignID, record.Price)
				if slotKey != "" {
					pipe.IncrByFloat(ctx, slotKey, record.Price)
				}
				pipe.Set(ctx, markerKey, "1", s.markerTTL)
				return nil
			})
			return err
		}, watchKeys...)
		if err == nil {
			return nil
		}
		lastErr = err
		if !errors.Is(err, redis.TxFailedErr) {
			return fmt.Errorf("ADV billing transaction: %w", err)
		}
	}
	return fmt.Errorf("ADV billing transaction conflicted after retries: %w", lastErr)
}

// applyPromoSpend is deliberately independent from the Redis marker. A crash
// after one store commits but before the other one does is recovered by the
// durable billing outbox: Redis has its marker and PostgreSQL has this marker,
// so either side can be retried without double charging promo balance.
func (s *Store) applyPromoSpend(ctx context.Context, record outbox.Record) error {
	tx, err := s.postgres.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin percenter promo transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO adv_promo_spend_events(event_id, user_id, campaign_id, spend_delta)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (event_id) DO NOTHING
	`, record.EventID, record.UserID, record.CampaignID, record.Price)
	if err != nil {
		return fmt.Errorf("insert percenter promo billing marker: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read percenter promo marker result: %w", err)
	}
	if inserted == 0 {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit existing percenter promo marker: %w", err)
		}
		return nil
	}

	result, err = tx.ExecContext(ctx, `
		UPDATE users
		SET promo_spend_remaining = GREATEST(0, promo_spend_remaining - $2)
		WHERE id = $1::uuid
	`, record.UserID, record.Price)
	if err != nil {
		return fmt.Errorf("decrement percenter promo_spend_remaining: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read percenter promo update result: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("percenter promo user %q not found", record.UserID)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit percenter promo transaction: %w", err)
	}
	return nil
}

func normalizeFormat(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if _, ok := constants.FormatToCodes[value]; ok {
		return value
	}
	return ""
}
