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
	"github.com/lib/pq"
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
	UserRemainingBalance        map[string]float64
	UserAntiPerekrutBlocked     map[string]bool
	PendingUserBlocks           map[string]time.Time
	CampaignSpend               map[string]SpendPoint
	CampaignAuctionAllowed      map[string]bool
	CampaignActiveIntervalStart map[string]time.Time
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
		UserSpend:                   map[string]SpendPoint{},
		UserRemainingBalance:        map[string]float64{},
		UserAntiPerekrutBlocked:     map[string]bool{},
		PendingUserBlocks:           map[string]time.Time{},
		CampaignSpend:               map[string]SpendPoint{},
		CampaignAuctionAllowed:      map[string]bool{},
		CampaignActiveIntervalStart: map[string]time.Time{},
		TrafficLimit:                map[string]uint32{},
		AppliedCampaignResetVersion: map[string]int64{},
		CampaignResetAppliedAt:      map[string]time.Time{},
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

	userIDs := make([]string, 0, len(snapshot.UserGoals))
	for userID := range snapshot.UserGoals {
		userIDs = append(userIDs, userID)
	}
	sort.Strings(userIDs)

	// The user guard is calculated once per minute here, outside the hot auction
	// path. Missing entities in the latest ClickHouse batch are explicitly treated
	// as zero spend; the MV technical row identifies the latest completed batch.
	userSpend, err := loadLatestBatchSpendPoints(
		ctx,
		m.clickhouse,
		m.database,
		"user_dsp_price_sum",
		"user_id",
		userIDs,
	)
	if err != nil {
		return fmt.Errorf("load latest user spend: %w", err)
	}
	runtimeSpent, err := m.runtime.UserSpentBatch(ctx, userIDs)
	if err != nil {
		return fmt.Errorf("load runtime user spends: %w", err)
	}
	now := time.Now().UTC()
	userRemaining, campaignAllowed, userBlocked, pendingUserBlocks, usersToPersist := calculateCampaignAuctionAllowed(
		snapshot,
		userSpend,
		runtimeSpent,
		base.PendingUserBlocks,
	)

	if len(usersToPersist) > 0 {
		if err := blockUsersInPostgres(ctx, m.db, usersToPersist); err != nil {
			m.notifyError(ctx, "postgres_block_users", err)
			log.Printf("[ADV][ANTIPEREKRUT][postgres_block_users] %v", err)

			// The database marker could not be persisted. Publish the affected users
			// as blocked locally and retain them in PendingUserBlocks so every next
			// minute retries the same idempotent bulk UPDATE. No traffic-limit growth
			// is performed in this degraded cycle.
			next := cloneAntiPerekrutState(base)
			next.UserSpend = userSpend
			next.UserRemainingBalance = userRemaining
			next.UserAntiPerekrutBlocked = userBlocked
			next.PendingUserBlocks = pendingUserBlocks
			next.CampaignAuctionAllowed = campaignAllowed
			next.LoadedAt = now
			next.Generation = baseGeneration + 1
			applyCampaignStateTransitions(next, snapshot, now)
			applyUserBlockTransitions(next, snapshot, base.UserAntiPerekrutBlocked, userBlocked, now)
			removeInactiveCampaignState(next, snapshot)

			m.writer.Lock()
			current := m.state.Load()
			if current == nil || (current.Generation == baseGeneration && current.GlobalResetGeneration == baseGlobalGeneration) {
				m.state.Store(next)
			}
			m.writer.Unlock()
			return nil
		}
		persistedAt := time.Now().UTC()
		for _, userID := range usersToPersist {
			pendingUserBlocks[userID] = persistedAt
		}
	}
	allowedCampaignIDs := make([]string, 0, len(campaignAllowed))
	for _, campaign := range snapshot.Campaigns {
		if campaign == nil || !campaignAllowed[campaign.ID] || !campaignActiveAt(campaign, now) {
			continue
		}
		allowedCampaignIDs = append(allowedCampaignIDs, campaign.ID)
	}
	sort.Strings(allowedCampaignIDs)

	// Campaign spend is intentionally not queried for blocked users. Their
	// percentages remain frozen until a later user batch makes them eligible.
	campaignSpend := make(map[string]SpendPoint, len(allowedCampaignIDs))
	if len(allowedCampaignIDs) > 0 {
		campaignSpend, err = loadLatestBatchSpendPoints(
			ctx,
			m.clickhouse,
			m.database,
			"campaign_dsp_price_sum",
			"cid",
			allowedCampaignIDs,
		)
		if err != nil {
			return fmt.Errorf("load latest allowed campaign spend: %w", err)
		}
	}

	next := cloneAntiPerekrutState(base)
	next.UserSpend = userSpend
	next.UserRemainingBalance = userRemaining
	next.UserAntiPerekrutBlocked = userBlocked
	next.PendingUserBlocks = pendingUserBlocks
	next.CampaignSpend = campaignSpend
	next.CampaignAuctionAllowed = campaignAllowed
	next.LoadedAt = now
	next.Generation = baseGeneration + 1

	resetCampaigns := applyCampaignStateTransitions(next, snapshot, now)
	applyUserBlockTransitions(next, snapshot, base.UserAntiPerekrutBlocked, userBlocked, now)
	removeInactiveCampaignState(next, snapshot)
	calculateTrafficLimits(next, snapshot, resetCampaigns, now)

	m.writer.Lock()
	defer m.writer.Unlock()
	current := m.state.Load()
	if current != nil && (current.Generation != baseGeneration || current.GlobalResetGeneration != baseGlobalGeneration) {
		return nil // a concurrent reset/newer refresh won; discard this stale calculation
	}
	m.state.Store(next)
	return nil
}

func loadLatestBatchSpendPoints(
	ctx context.Context,
	ch clickhouse.Conn,
	database, table, idColumn string,
	entityIDs []string,
) (map[string]SpendPoint, error) {
	if ch == nil {
		return nil, errors.New("ClickHouse connection is nil")
	}

	uniqueIDs := make([]string, 0, len(entityIDs))
	seen := make(map[string]struct{}, len(entityIDs))
	for _, rawID := range entityIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return map[string]SpendPoint{}, nil
	}
	sort.Strings(uniqueIDs)

	qualified := fmt.Sprintf("`%s`.`%s`", strings.TrimSpace(database), table)
	idExpression := fmt.Sprintf("toString(%s)", idColumn)

	var latestBatch time.Time
	if err := ch.QueryRow(ctx, fmt.Sprintf("SELECT max(created_at) FROM %s", qualified)).Scan(&latestBatch); err != nil {
		return nil, fmt.Errorf("load %s latest batch timestamp: %w", table, err)
	}
	latestBatch = latestBatch.UTC()
	if latestBatch.IsZero() || latestBatch.Unix() <= 0 {
		return nil, fmt.Errorf("%s latest completed batch is unavailable", table)
	}

	// Every requested entity starts with zero at the latest completed batch.
	// Actual rows from that same batch overwrite these defaults below.
	out := make(map[string]SpendPoint, len(uniqueIDs))
	for _, id := range uniqueIDs {
		out[id] = SpendPoint{Spend: 0, CreatedAt: latestBatch}
	}

	// Keep the SQL bounded when a user owns many campaigns. Only requested IDs
	// are read, so campaigns belonging to blocked users never enter this query.
	const queryChunkSize = 5_000
	for start := 0; start < len(uniqueIDs); start += queryChunkSize {
		end := start + queryChunkSize
		if end > len(uniqueIDs) {
			end = len(uniqueIDs)
		}
		chunk := uniqueIDs[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		query := fmt.Sprintf(`
SELECT
    %s AS entity_id,
    max(sum_cum_per_period) AS spend
FROM %s
WHERE
    created_at = ?
    AND %s IN (%s)
GROUP BY entity_id`,
			idExpression,
			qualified,
			idExpression,
			placeholders,
		)

		args := make([]any, 0, len(chunk)+1)
		args = append(args, latestBatch)
		for _, id := range chunk {
			args = append(args, id)
		}
		rows, err := ch.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			var spend float64
			if err := rows.Scan(&id, &spend); err != nil {
				rows.Close()
				return nil, err
			}
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if math.IsNaN(spend) || math.IsInf(spend, 0) || spend < 0 {
				rows.Close()
				return nil, fmt.Errorf("%s latest batch contains invalid spend for %s=%q: %v", table, idColumn, id, spend)
			}
			out[id] = SpendPoint{Spend: spend, CreatedAt: latestBatch}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

func calculateCampaignAuctionAllowed(
	snapshot *Snapshot,
	userSpend map[string]SpendPoint,
	runtimeSpent map[string]float64,
	pendingBlocks map[string]time.Time,
) (map[string]float64, map[string]bool, map[string]bool, map[string]time.Time, []string) {
	userRemaining := make(map[string]float64, len(snapshot.UserGoals))
	userBlocked := make(map[string]bool, len(snapshot.UserGoals))
	nextPendingBlocks := make(map[string]time.Time)
	usersToPersist := make([]string, 0)

	for userID, goal := range snapshot.UserGoals {
		remaining := goal - runtimeSpent[userID]
		if remaining < 0 {
			remaining = 0
		}
		userRemaining[userID] = remaining

		durableBlocked := snapshot.UserAntiPerekrutBlocked[userID]
		pendingAt, hasPending := pendingBlocks[userID]
		pendingBlocked := hasPending && !durableBlocked &&
			(pendingAt.IsZero() || snapshot.LoadedAt.IsZero() || !snapshot.LoadedAt.After(pendingAt))
		recent := userSpend[userID].Spend
		unsafe := recent*2 > remaining
		blocked := durableBlocked || pendingBlocked || unsafe
		userBlocked[userID] = blocked

		if blocked && !durableBlocked {
			if pendingBlocked {
				nextPendingBlocks[userID] = pendingAt
			} else {
				nextPendingBlocks[userID] = time.Time{}
			}
			if nextPendingBlocks[userID].IsZero() {
				usersToPersist = append(usersToPersist, userID)
			}
		}
	}
	sort.Strings(usersToPersist)

	campaignAllowed := make(map[string]bool, len(snapshot.Campaigns))
	for _, campaign := range snapshot.Campaigns {
		if campaign == nil || campaign.Status != CampaignStatusActive {
			continue
		}
		campaignAllowed[campaign.ID] = !userBlocked[campaign.UserID]
	}
	return userRemaining, campaignAllowed, userBlocked, nextPendingBlocks, usersToPersist
}

func blockUsersInPostgres(ctx context.Context, db *sql.DB, userIDs []string) error {
	if db == nil {
		return errors.New("postgres db is nil")
	}
	if len(userIDs) == 0 {
		return nil
	}
	result, err := db.ExecContext(ctx, `
UPDATE users
SET antiperekrut_blocked = TRUE, updated_at = NOW()
WHERE id = ANY($1::uuid[])`, pq.Array(userIDs))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != int64(len(userIDs)) {
		return fmt.Errorf("blocked %d of %d users", affected, len(userIDs))
	}
	return nil
}

func applyUserBlockTransitions(
	state *AntiPerekrutState,
	snapshot *Snapshot,
	previousBlocked map[string]bool,
	currentBlocked map[string]bool,
	now time.Time,
) {
	if state == nil || snapshot == nil {
		return
	}
	now = now.UTC()
	for _, campaign := range snapshot.Campaigns {
		if campaign == nil || !currentBlocked[campaign.UserID] {
			continue
		}
		state.TrafficLimit[campaign.ID] = TrafficLimitInitial
		if !previousBlocked[campaign.UserID] {
			state.CampaignResetAppliedAt[campaign.ID] = now
		}
	}
}

func applyCampaignStateTransitions(
	state *AntiPerekrutState,
	snapshot *Snapshot,
	now time.Time,
) map[string]struct{} {
	resetCampaigns := make(map[string]struct{})
	if state == nil || snapshot == nil {
		return resetCampaigns
	}
	if state.CampaignActiveIntervalStart == nil {
		state.CampaignActiveIntervalStart = make(map[string]time.Time)
	}
	now = now.UTC()
	for _, campaign := range snapshot.Campaigns {
		if campaign == nil {
			continue
		}
		applied := state.AppliedCampaignResetVersion[campaign.ID]
		if campaign.TrafficResetVersion > applied {
			state.TrafficLimit[campaign.ID] = TrafficLimitInitial
			state.AppliedCampaignResetVersion[campaign.ID] = campaign.TrafficResetVersion
			state.CampaignResetAppliedAt[campaign.ID] = now
			resetCampaigns[campaign.ID] = struct{}{}
		}
		if _, exists := state.TrafficLimit[campaign.ID]; !exists {
			state.TrafficLimit[campaign.ID] = TrafficLimitInitial
			if _, ok := state.CampaignResetAppliedAt[campaign.ID]; !ok {
				state.CampaignResetAppliedAt[campaign.ID] = now
			}
		}

		if len(campaign.ActiveIntervals) == 0 {
			delete(state.CampaignActiveIntervalStart, campaign.ID)
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(campaign.Status), CampaignStatusActive) {
			continue
		}
		activeInterval, activeNow := campaignActiveIntervalAt(campaign, now)
		if !activeNow {
			continue
		}
		storedStart, exists := state.CampaignActiveIntervalStart[campaign.ID]
		if exists && storedStart.Equal(activeInterval.Start) {
			continue
		}

		// A new continuous schedule block always starts from 0.01%. Unlike a
		// version/global reset, this reset may grow on the same +8s minute tick:
		// the hot path has already enforced 0.01% since the exact block start.
		state.TrafficLimit[campaign.ID] = TrafficLimitInitial
		state.CampaignActiveIntervalStart[campaign.ID] = activeInterval.Start
		if resetAt, ok := state.CampaignResetAppliedAt[campaign.ID]; !ok || resetAt.Before(activeInterval.Start) {
			state.CampaignResetAppliedAt[campaign.ID] = activeInterval.Start
		}
	}
	return resetCampaigns
}

func calculateTrafficLimits(
	state *AntiPerekrutState,
	snapshot *Snapshot,
	resetCampaigns map[string]struct{},
	now time.Time,
) {
	byUser := make(map[string][]*Campaign)
	for _, campaign := range snapshot.Campaigns {
		if campaign != nil && campaignScheduledActiveAt(campaign, now) {
			byUser[campaign.UserID] = append(byUser[campaign.UserID], campaign)
		}
	}
	for userID, campaigns := range byUser {
		if len(campaigns) == 0 || !state.CampaignAuctionAllowed[campaigns[0].ID] {
			continue
		}
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
		remaining := state.UserRemainingBalance[userID]
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
			delete(state.CampaignSpend, campaignID)
			delete(state.CampaignAuctionAllowed, campaignID)
			delete(state.CampaignActiveIntervalStart, campaignID)
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
		UserSpend:                   make(map[string]SpendPoint, len(src.UserSpend)),
		UserRemainingBalance:        make(map[string]float64, len(src.UserRemainingBalance)),
		UserAntiPerekrutBlocked:     make(map[string]bool, len(src.UserAntiPerekrutBlocked)),
		PendingUserBlocks:           make(map[string]time.Time, len(src.PendingUserBlocks)),
		CampaignSpend:               make(map[string]SpendPoint, len(src.CampaignSpend)),
		CampaignAuctionAllowed:      make(map[string]bool, len(src.CampaignAuctionAllowed)),
		CampaignActiveIntervalStart: make(map[string]time.Time, len(src.CampaignActiveIntervalStart)),
		TrafficLimit:                make(map[string]uint32, len(src.TrafficLimit)),
		AppliedCampaignResetVersion: make(map[string]int64, len(src.AppliedCampaignResetVersion)),
		CampaignResetAppliedAt:      make(map[string]time.Time, len(src.CampaignResetAppliedAt)),
		GlobalResetGeneration:       src.GlobalResetGeneration,
		LoadedAt:                    src.LoadedAt,
		Generation:                  src.Generation,
	}
	for k, v := range src.UserSpend {
		out.UserSpend[k] = v
	}
	for k, v := range src.UserRemainingBalance {
		out.UserRemainingBalance[k] = v
	}
	for k, v := range src.UserAntiPerekrutBlocked {
		out.UserAntiPerekrutBlocked[k] = v
	}
	for k, v := range src.PendingUserBlocks {
		out.PendingUserBlocks[k] = v
	}
	for k, v := range src.CampaignSpend {
		out.CampaignSpend[k] = v
	}
	for k, v := range src.CampaignAuctionAllowed {
		out.CampaignAuctionAllowed[k] = v
	}
	for k, v := range src.CampaignActiveIntervalStart {
		out.CampaignActiveIntervalStart[k] = v
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
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS antiperekrut_blocked BOOLEAN NOT NULL DEFAULT FALSE`,
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

func (m *AntiPerekrutManager) CampaignAllowed(state *AntiPerekrutState, campaign *Campaign) bool {
	if m == nil || state == nil || campaign == nil {
		return false
	}
	allowed, exists := state.CampaignAuctionAllowed[campaign.ID]
	return exists && allowed
}

func (m *AntiPerekrutManager) EffectiveTrafficLimit(
	state *AntiPerekrutState,
	campaign *Campaign,
	now time.Time,
) uint32 {
	if m == nil || campaign == nil {
		return TrafficLimitInitial
	}
	if state == nil {
		return TrafficLimitInitial
	}
	if campaign.TrafficResetVersion > state.AppliedCampaignResetVersion[campaign.ID] {
		return TrafficLimitInitial
	}
	if len(campaign.ActiveIntervals) > 0 {
		activeInterval, activeNow := campaignActiveIntervalAt(campaign, now)
		storedStart, exists := state.CampaignActiveIntervalStart[campaign.ID]
		if !activeNow || !exists || !storedStart.Equal(activeInterval.Start) {
			return TrafficLimitInitial
		}
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
