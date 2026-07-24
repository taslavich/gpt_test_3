package auction

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
)

const (
	TrafficLimitInitial uint32 = 100
	TrafficLimitFull    uint32 = 1_000_000
	trafficMultiplier          = 3.0
	trafficLateFactor          = 1.40
)

type SpendPoint struct {
	Spend     float64
	CreatedAt time.Time
}

type AntiPerekrutState struct {
	UserSpend                   map[string]SpendPoint
	CampaignSpend               map[string]SpendPoint
	TrafficLimit                map[string]uint32
	AppliedCampaignResetVersion map[string]int64
	CampaignResetAppliedAt      map[string]time.Time
	GlobalResetGeneration       int64
	LoadedAt                    time.Time
	Generation                  uint64
}

type AntiPerekrutNotifier func(context.Context, string) error

type AntiPerekrutManager struct {
	db         *sql.DB
	clickhouse clickhouse.Conn
	database   string
	runtime    *RuntimeStore
	snapshot   func() *Snapshot
	notify     AntiPerekrutNotifier
	tickOffset time.Duration

	notifyMu    sync.Mutex
	notifyQueue []string
	notifyWake  chan struct{}

	state  atomic.Pointer[AntiPerekrutState]
	writer sync.Mutex
}

func NewAntiPerekrutManager(
	db *sql.DB,
	ch clickhouse.Conn,
	database string,
	runtime *RuntimeStore,
	snapshot func() *Snapshot,
	tickOffset time.Duration,
	notify AntiPerekrutNotifier,
) (*AntiPerekrutManager, error) {
	if db == nil {
		return nil, errors.New("antiperekrut postgres db is nil")
	}
	if ch == nil {
		return nil, errors.New("antiperekrut ClickHouse connection is nil")
	}
	if runtime == nil {
		return nil, errors.New("antiperekrut runtime store is nil")
	}
	if snapshot == nil {
		return nil, errors.New("antiperekrut snapshot getter is nil")
	}
	if tickOffset < 0 || tickOffset >= time.Minute {
		return nil, fmt.Errorf("ANTIPEREKRUT_TICK_OFFSET must be in [0,1m): %s", tickOffset)
	}
	database = strings.TrimSpace(database)
	if database == "" {
		return nil, errors.New("antiperekrut ClickHouse database is empty")
	}
	manager := &AntiPerekrutManager{
		db: db, clickhouse: ch, database: database, runtime: runtime,
		snapshot: snapshot, notify: notify, notifyWake: make(chan struct{}, 1), tickOffset: tickOffset,
	}
	manager.state.Store(newEmptyAntiPerekrutState())
	return manager, nil
}

func newEmptyAntiPerekrutState() *AntiPerekrutState {
	return &AntiPerekrutState{
		UserSpend: map[string]SpendPoint{}, CampaignSpend: map[string]SpendPoint{},
		TrafficLimit: map[string]uint32{}, AppliedCampaignResetVersion: map[string]int64{},
		CampaignResetAppliedAt: map[string]time.Time{},
	}
}

func (m *AntiPerekrutManager) State() *AntiPerekrutState {
	if m == nil {
		return nil
	}
	return m.state.Load()
}

func (m *AntiPerekrutManager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	go m.runNotifier(ctx)
	go func() {
		for {
			now := time.Now().UTC()
			next := now.Truncate(time.Minute).Add(time.Minute).Add(m.tickOffset)
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				if err := m.Refresh(ctx); err != nil {
					m.notifyError(ctx, "minute_refresh", err)
				}
			}
		}
	}()
}

func (m *AntiPerekrutManager) Refresh(ctx context.Context) error {
	if m == nil {
		return errors.New("antiperekrut manager is nil")
	}
	snapshot := m.snapshot()
	if snapshot == nil {
		return errors.New("campaign snapshot is unavailable")
	}
	base := m.state.Load()
	if base == nil {
		base = newEmptyAntiPerekrutState()
	}

	// A durable global reset is applied before ClickHouse/Redis reads so a missed
	// direct HTTP call cannot leave stale high percentages active. The same
	// Refresh call must still complete all data loads and set LoadedAt before ADV
	// is allowed to serve auctions. Post-reset spend timestamps prevent growth in
	// this cycle even though the fresh maps are published.
	globalGeneration, err := loadGlobalResetGeneration(ctx, m.db)
	if err != nil {
		return fmt.Errorf("load global reset generation: %w", err)
	}
	if globalGeneration > base.GlobalResetGeneration {
		m.applyGlobalReset(globalGeneration, time.Now().UTC())
		base = m.state.Load()
		if base == nil {
			return errors.New("antiperekrut state is unavailable after global reset")
		}
	}

	baseGeneration := base.Generation
	baseGlobalGeneration := base.GlobalResetGeneration
	userSpend, err := loadSpendPoints(ctx, m.clickhouse, m.database, "user_dsp_price_sum", "user_id")
	if err != nil {
		return fmt.Errorf("load user spend: %w", err)
	}
	campaignSpend, err := loadSpendPoints(ctx, m.clickhouse, m.database, "campaign_dsp_price_sum", "cid")
	if err != nil {
		return fmt.Errorf("load campaign spend: %w", err)
	}
	userIDs := make([]string, 0, len(snapshot.UserGoals))
	for userID := range snapshot.UserGoals {
		userIDs = append(userIDs, userID)
	}
	runtimeSpent, err := m.runtime.UserSpentBatch(ctx, userIDs)
	if err != nil {
		return fmt.Errorf("load runtime user spends: %w", err)
	}

	now := time.Now().UTC()
	next := cloneAntiPerekrutState(base)
	next.UserSpend = userSpend
	next.CampaignSpend = campaignSpend
	next.LoadedAt = now
	next.Generation = baseGeneration + 1

	resetCampaigns := make(map[string]struct{})
	for _, campaign := range snapshot.Campaigns {
		if campaign == nil {
			continue
		}
		applied := next.AppliedCampaignResetVersion[campaign.ID]
		if campaign.TrafficResetVersion > applied {
			next.TrafficLimit[campaign.ID] = TrafficLimitInitial
			next.AppliedCampaignResetVersion[campaign.ID] = campaign.TrafficResetVersion
			next.CampaignResetAppliedAt[campaign.ID] = now
			resetCampaigns[campaign.ID] = struct{}{}
		}
		if _, exists := next.TrafficLimit[campaign.ID]; !exists {
			next.TrafficLimit[campaign.ID] = TrafficLimitInitial
			if _, ok := next.CampaignResetAppliedAt[campaign.ID]; !ok {
				next.CampaignResetAppliedAt[campaign.ID] = now
			}
		}
	}
	removeInactiveCampaignState(next, snapshot)
	calculateTrafficLimits(next, snapshot, runtimeSpent, resetCampaigns)

	m.writer.Lock()
	defer m.writer.Unlock()
	current := m.state.Load()
	if current != nil && (current.Generation != baseGeneration || current.GlobalResetGeneration != baseGlobalGeneration) {
		return nil // a concurrent reset/newer refresh won; discard this stale calculation
	}
	m.state.Store(next)
	return nil
}

func loadSpendPoints(ctx context.Context, ch clickhouse.Conn, database, table, idColumn string) (map[string]SpendPoint, error) {
	if ch == nil {
		return nil, errors.New("ClickHouse connection is nil")
	}
	qualified := fmt.Sprintf("`%s`.`%s`", strings.TrimSpace(database), table)
	idExpression := fmt.Sprintf("toString(%s)", idColumn)
	query := fmt.Sprintf(`
SELECT
    %s AS entity_id,
    argMax(sum_cum_per_period, created_at) AS spend,
    max(created_at) AS spend_created_at
FROM %s
WHERE sum_cum_per_period != 0 AND notEmpty(trimBoth(%s))
GROUP BY %s`, idExpression, qualified, idExpression, idColumn)
	rows, err := ch.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]SpendPoint)
	for rows.Next() {
		var id string
		var spend float64
		var createdAt time.Time
		if err := rows.Scan(&id, &spend, &createdAt); err != nil {
			return nil, err
		}
		id = strings.TrimSpace(id)
		if id == "" || math.IsNaN(spend) || math.IsInf(spend, 0) || spend < 0 {
			continue
		}
		out[id] = SpendPoint{Spend: spend, CreatedAt: createdAt.UTC()}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func calculateTrafficLimits(state *AntiPerekrutState, snapshot *Snapshot, runtimeSpent map[string]float64, resetCampaigns map[string]struct{}) {
	byUser := make(map[string][]*Campaign)
	for _, campaign := range snapshot.Campaigns {
		if campaign != nil && campaign.Status == CampaignStatusActive {
			byUser[campaign.UserID] = append(byUser[campaign.UserID], campaign)
		}
	}
	for userID, campaigns := range byUser {
		complete := true
		for _, campaign := range campaigns {
			if _, resetNow := resetCampaigns[campaign.ID]; resetNow {
				complete = false
				break
			}
			spend, ok := state.CampaignSpend[campaign.ID]
			if !ok || !spend.CreatedAt.After(state.CampaignResetAppliedAt[campaign.ID]) {
				complete = false
				break
			}
		}
		if !complete {
			continue
		}
		sort.Slice(campaigns, func(i, j int) bool {
			li, lj := state.TrafficLimit[campaigns[i].ID], state.TrafficLimit[campaigns[j].ID]
			if li == lj {
				return campaigns[i].ID < campaigns[j].ID
			}
			return li < lj
		})
		remaining := snapshot.UserGoals[userID] - runtimeSpent[userID]
		if remaining < 0 {
			remaining = 0
		}
		accumulated := 0.0
		for i, campaign := range campaigns {
			point := state.CampaignSpend[campaign.ID]
			forecast := point.Spend * trafficLateFactor
			if state.TrafficLimit[campaign.ID] < TrafficLimitFull {
				forecast *= trafficMultiplier
			}
			temporary := 0.0
			for _, rest := range campaigns[i+1:] {
				temporary += state.CampaignSpend[rest.ID].Spend * trafficLateFactor
			}
			candidateS := accumulated + forecast
			if candidateS+temporary >= remaining {
				break
			}
			accumulated = candidateS
			if state.TrafficLimit[campaign.ID] < TrafficLimitFull {
				next := uint64(state.TrafficLimit[campaign.ID]) * 3
				if next > uint64(TrafficLimitFull) {
					next = uint64(TrafficLimitFull)
				}
				state.TrafficLimit[campaign.ID] = uint32(next)
			}
		}
	}
}

func (m *AntiPerekrutManager) RegisterStartupEvent(ctx context.Context, eventID, sourceService, sourceInstance string) (generation int64, retErr error) {
	defer func() {
		if retErr != nil {
			m.notifyError(ctx, "register_startup_event", retErr)
		}
	}()
	if m == nil || m.db == nil {
		return 0, errors.New("antiperekrut manager is unavailable")
	}
	if _, err := uuid.Parse(strings.TrimSpace(eventID)); err != nil {
		return 0, fmt.Errorf("invalid event_id: %w", err)
	}
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
INSERT INTO antiperekrut_restart_events(event_id, source_service, source_instance, reason)
VALUES ($1::uuid, $2, $3, 'startup')
ON CONFLICT (event_id) DO NOTHING`, eventID, strings.TrimSpace(sourceService), strings.TrimSpace(sourceInstance))
	if err != nil {
		return 0, err
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if inserted > 0 {
		err = tx.QueryRowContext(ctx, `
UPDATE antiperekrut_control_state
SET global_reset_generation = global_reset_generation + 1, updated_at = NOW()
WHERE id = 1
RETURNING global_reset_generation`).Scan(&generation)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT global_reset_generation FROM antiperekrut_control_state WHERE id = 1`).Scan(&generation)
	}
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	m.applyGlobalReset(generation, time.Now().UTC())
	return generation, nil
}

func (m *AntiPerekrutManager) applyGlobalReset(generation int64, now time.Time) {
	m.writer.Lock()
	defer m.writer.Unlock()
	current := m.state.Load()
	if current == nil {
		current = newEmptyAntiPerekrutState()
	}
	if generation <= current.GlobalResetGeneration {
		return
	}
	next := cloneAntiPerekrutState(current)
	next.Generation = current.Generation + 1
	next.GlobalResetGeneration = generation
	resetAllActiveCampaigns(next, m.snapshot(), now.UTC())
	m.state.Store(next)
}

func resetAllActiveCampaigns(state *AntiPerekrutState, snapshot *Snapshot, now time.Time) {
	if state == nil || snapshot == nil {
		return
	}
	for _, campaign := range snapshot.Campaigns {
		if campaign == nil || campaign.Status != CampaignStatusActive {
			continue
		}
		state.TrafficLimit[campaign.ID] = TrafficLimitInitial
		state.CampaignResetAppliedAt[campaign.ID] = now
		if campaign.TrafficResetVersion > state.AppliedCampaignResetVersion[campaign.ID] {
			state.AppliedCampaignResetVersion[campaign.ID] = campaign.TrafficResetVersion
		}
	}
}

func removeInactiveCampaignState(state *AntiPerekrutState, snapshot *Snapshot) {
	active := make(map[string]struct{}, len(snapshot.Campaigns))
	for _, campaign := range snapshot.Campaigns {
		if campaign != nil {
			active[campaign.ID] = struct{}{}
		}
	}
	for campaignID := range state.TrafficLimit {
		if _, ok := active[campaignID]; !ok {
			delete(state.TrafficLimit, campaignID)
			delete(state.AppliedCampaignResetVersion, campaignID)
			delete(state.CampaignResetAppliedAt, campaignID)
		}
	}
}

func cloneAntiPerekrutState(src *AntiPerekrutState) *AntiPerekrutState {
	if src == nil {
		return newEmptyAntiPerekrutState()
	}
	out := &AntiPerekrutState{
		UserSpend: make(map[string]SpendPoint, len(src.UserSpend)), CampaignSpend: make(map[string]SpendPoint, len(src.CampaignSpend)),
		TrafficLimit:                make(map[string]uint32, len(src.TrafficLimit)),
		AppliedCampaignResetVersion: make(map[string]int64, len(src.AppliedCampaignResetVersion)),
		CampaignResetAppliedAt:      make(map[string]time.Time, len(src.CampaignResetAppliedAt)),
		GlobalResetGeneration:       src.GlobalResetGeneration, LoadedAt: src.LoadedAt, Generation: src.Generation,
	}
	for k, v := range src.UserSpend {
		out.UserSpend[k] = v
	}
	for k, v := range src.CampaignSpend {
		out.CampaignSpend[k] = v
	}
	for k, v := range src.TrafficLimit {
		out.TrafficLimit[k] = v
	}
	for k, v := range src.AppliedCampaignResetVersion {
		out.AppliedCampaignResetVersion[k] = v
	}
	for k, v := range src.CampaignResetAppliedAt {
		out.CampaignResetAppliedAt[k] = v
	}
	return out
}

func loadGlobalResetGeneration(ctx context.Context, db *sql.DB) (int64, error) {
	var generation int64
	err := db.QueryRowContext(ctx, `SELECT global_reset_generation FROM antiperekrut_control_state WHERE id = 1`).Scan(&generation)
	return generation, err
}

func EnsureAntiPerekrutSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("postgres db is nil")
	}
	queries := []string{
		`ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS traffic_reset_version BIGINT NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS antiperekrut_control_state (
			id SMALLINT PRIMARY KEY,
			global_reset_generation BIGINT NOT NULL DEFAULT 0,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT antiperekrut_control_state_singleton CHECK (id = 1)
		)`,
		`INSERT INTO antiperekrut_control_state(id, global_reset_generation) VALUES (1, 0) ON CONFLICT (id) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS antiperekrut_restart_events (
			event_id UUID PRIMARY KEY,
			source_service TEXT NOT NULL,
			source_instance TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT 'startup',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_antiperekrut_restart_events_created_at ON antiperekrut_restart_events(created_at DESC)`,
	}
	for _, query := range queries {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("antiperekrut schema migration: %w", err)
		}
	}
	return nil
}

func (m *AntiPerekrutManager) EffectiveTrafficLimit(state *AntiPerekrutState, campaign *Campaign) uint32 {
	if m == nil || campaign == nil {
		return TrafficLimitInitial
	}
	if state == nil {
		return TrafficLimitInitial
	}
	if campaign.TrafficResetVersion > state.AppliedCampaignResetVersion[campaign.ID] {
		return TrafficLimitInitial
	}
	limit, ok := state.TrafficLimit[campaign.ID]
	if !ok || limit < TrafficLimitInitial {
		return TrafficLimitInitial
	}
	if limit > TrafficLimitFull {
		return TrafficLimitFull
	}
	return limit
}

func trafficHashPass(hashID, campaignID string, limit uint32) bool {
	if limit >= TrafficLimitFull {
		return true
	}
	if limit == 0 || strings.TrimSpace(hashID) == "" || strings.TrimSpace(campaignID) == "" {
		return false
	}
	value := xxhash.Sum64String(strings.TrimSpace(hashID)+"\x00"+strings.TrimSpace(campaignID)) % uint64(TrafficLimitFull)
	return value < uint64(limit)
}

func (m *AntiPerekrutManager) NotifyAuctionError(stage string, err error) {
	if m == nil || err == nil || m.notify == nil {
		return
	}
	m.enqueueNotification(fmt.Sprintf("[ADV][ANTIPEREKRUT][%s] %v", stage, err))
}

func (m *AntiPerekrutManager) enqueueNotification(message string) {
	if m == nil || m.notify == nil || strings.TrimSpace(message) == "" {
		return
	}
	m.notifyMu.Lock()
	m.notifyQueue = append(m.notifyQueue, message)
	m.notifyMu.Unlock()
	select {
	case m.notifyWake <- struct{}{}:
	default:
	}
}

func (m *AntiPerekrutManager) dequeueNotification() (string, bool) {
	m.notifyMu.Lock()
	defer m.notifyMu.Unlock()
	if len(m.notifyQueue) == 0 {
		return "", false
	}
	message := m.notifyQueue[0]
	if len(m.notifyQueue) == 1 {
		m.notifyQueue = nil
	} else {
		m.notifyQueue[0] = ""
		m.notifyQueue = m.notifyQueue[1:]
	}
	return message, true
}

func (m *AntiPerekrutManager) runNotifier(ctx context.Context) {
	if m == nil || m.notify == nil {
		return
	}
	for {
		message, ok := m.dequeueNotification()
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-m.notifyWake:
			}
			continue
		}

		backoff := time.Second
		for {
			notifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := m.notify(notifyCtx, message)
			cancel()
			if err == nil {
				break
			}
			log.Printf("[ADV][ANTIPEREKRUT][TELEGRAM_RETRY] error=%v", err)
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if backoff < time.Minute {
				backoff *= 2
				if backoff > time.Minute {
					backoff = time.Minute
				}
			}
		}
	}
}

func (m *AntiPerekrutManager) notifyError(_ context.Context, stage string, err error) {
	if err == nil || m == nil || m.notify == nil {
		return
	}
	m.enqueueNotification(fmt.Sprintf("[ADV][ANTIPEREKRUT][%s] %v", stage, err))
}
