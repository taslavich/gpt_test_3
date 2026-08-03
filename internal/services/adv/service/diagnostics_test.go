package auction

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	filterV2 "gitlab.com/twinbid-exchange/RTB-exchange/internal/filterV2"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestAuctionDiagnosticsShardedDoubleBufferAndPercentages(t *testing.T) {
	start := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	diagnostics := NewAuctionDiagnostics(start)
	campaign := &Campaign{ID: "campaign-a", UserID: "user-a"}
	diagnostics.registerCampaigns([]*Campaign{campaign})
	diagnostics.SetEnabled(true, start)

	session := diagnostics.active.Load()
	if session == nil {
		t.Fatal("diagnostics session was not enabled")
	}
	recorder, ok := session.begin("request-1")
	if !ok {
		t.Fatal("diagnostics recorder was not created")
	}
	recorder.RecordRequestStart(4)
	recorder.RecordGlobalImpression(diagGlobalWinnerUUIDMissing)
	recorder.RecordCampaign(campaign.diagnosticIndex, diagBidWon)
	for i := 0; i < 3; i++ {
		recorder.RecordCampaign(campaign.diagnosticIndex, diagQualityMismatch)
	}
	recorder.Close()

	session.rotate(diagnostics, start.Add(time.Minute))
	snapshot := diagnostics.Snapshot()
	if snapshot == nil || !snapshot.Ready {
		t.Fatal("closed-minute diagnostics snapshot was not published")
	}
	if !snapshot.WindowStart.Equal(start) || !snapshot.WindowEnd.Equal(start.Add(time.Minute)) {
		t.Fatalf("unexpected window: %s - %s", snapshot.WindowStart, snapshot.WindowEnd)
	}

	campaignSnapshot := snapshot.Campaigns["campaign-a"]
	if campaignSnapshot.Total != 4 {
		t.Fatalf("unexpected campaign total: got %d want 4", campaignSnapshot.Total)
	}
	if got := campaignSnapshot.Codes["200"]; got.Count != 1 || got.Percent != 25 {
		t.Fatalf("unexpected winner bucket: %+v", got)
	}
	if got := campaignSnapshot.Codes["309"]; got.Count != 3 || got.Percent != 75 {
		t.Fatalf("unexpected quality bucket: %+v", got)
	}
	if snapshot.Global.Requests.Total != 1 || snapshot.Global.Impressions.Total != 4 {
		t.Fatalf("unexpected global totals: %+v", snapshot.Global)
	}
	if got := snapshot.Global.Impressions.Codes["933"]; got.Count != 1 || got.Percent != 25 {
		t.Fatalf("unexpected global impression bucket: %+v", got)
	}

	// A write after rotation must land only in the fresh current buffer.
	recorder, ok = session.begin("request-2")
	if !ok {
		t.Fatal("second recorder was not created")
	}
	recorder.RecordCampaign(campaign.diagnosticIndex, diagSiteIDFilterRejected)
	recorder.Close()
	publishedAgain := diagnostics.Snapshot()
	if publishedAgain.Campaigns["campaign-a"].Total != 4 {
		t.Fatal("published snapshot changed after a write to the new buffer")
	}
}

func TestDiagnosticsDisabledHasNoActiveSession(t *testing.T) {
	diagnostics := NewAuctionDiagnostics(time.Now().UTC())
	if diagnostics.active.Load() != nil {
		t.Fatal("diagnostics must be disabled by default")
	}
	diagnostics.SetEnabled(true, time.Now().UTC())
	if diagnostics.active.Load() == nil {
		t.Fatal("diagnostics were not enabled")
	}
	diagnostics.SetEnabled(false, time.Now().UTC())
	if diagnostics.active.Load() != nil {
		t.Fatal("diagnostics were not disabled")
	}
}

func TestDiagnosticsRotationLoopCanRaceSafelyWithDisable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	diagnostics := NewAuctionDiagnostics(time.Now().UTC())
	diagnostics.Start(ctx)
	diagnostics.SetEnabled(true, time.Now().UTC())
	diagnostics.SetEnabled(false, time.Now().UTC())
}

func TestDiagnosticRegistryIsStableWhileEnabledAndCompactsWhileDisabled(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Minute)
	diagnostics := NewAuctionDiagnostics(start)
	a := &Campaign{ID: "a", UserID: "u1"}
	b := &Campaign{ID: "b", UserID: "u2"}
	diagnostics.registerCampaigns([]*Campaign{a, b})
	if a.diagnosticIndex != 0 || b.diagnosticIndex != 1 {
		t.Fatalf("unexpected initial indexes: a=%d b=%d", a.diagnosticIndex, b.diagnosticIndex)
	}

	diagnostics.SetEnabled(true, start)
	b2 := &Campaign{ID: "b", UserID: "u2-new"}
	c := &Campaign{ID: "c", UserID: "u3"}
	diagnostics.registerCampaigns([]*Campaign{b2, c})
	if b2.diagnosticIndex != 1 || c.diagnosticIndex != 2 {
		t.Fatalf("active-session indexes were not stable: b=%d c=%d", b2.diagnosticIndex, c.diagnosticIndex)
	}

	diagnostics.SetEnabled(false, start.Add(10*time.Second))
	b3 := &Campaign{ID: "b", UserID: "u2-final"}
	c2 := &Campaign{ID: "c", UserID: "u3"}
	diagnostics.registerCampaigns([]*Campaign{b3, c2})
	if b3.diagnosticIndex != 0 || c2.diagnosticIndex != 1 {
		t.Fatalf("disabled registry was not compacted: b=%d c=%d", b3.diagnosticIndex, c2.diagnosticIndex)
	}
}

func TestRecorderRefreshesCampaignBlocksOnlyAfterSnapshotGrowth(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Minute)
	diagnostics := NewAuctionDiagnostics(start)
	a := &Campaign{ID: "a", UserID: "u1"}
	diagnostics.registerCampaigns([]*Campaign{a})
	diagnostics.SetEnabled(true, start)
	session := diagnostics.active.Load()
	recorder, ok := session.begin("request-before-growth")
	if !ok {
		t.Fatal("recorder was not created")
	}

	// Grow past one complete 64-campaign block after Begin. RecordCampaign must
	// lazily refresh the block pointer rather than drop the new campaign result.
	campaigns := make([]*Campaign, 0, diagnosticCampaignBlockSize+1)
	campaigns = append(campaigns, a)
	for index := 1; index <= diagnosticCampaignBlockSize; index++ {
		campaigns = append(campaigns, &Campaign{ID: fmt.Sprintf("campaign-%d", index), UserID: "u"})
	}
	diagnostics.registerCampaigns(campaigns)
	last := campaigns[len(campaigns)-1]
	recorder.RecordCampaign(last.diagnosticIndex, diagQualityMismatch)
	recorder.Close()
	session.rotate(diagnostics, start.Add(time.Minute))

	snapshot := diagnostics.Snapshot()
	if got := snapshot.Campaigns[last.ID].Codes["309"].Count; got != 1 {
		t.Fatalf("new campaign result was lost after capacity growth: got %d", got)
	}
}

func TestDiagnosticsConcurrentToggleAndRecord(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Minute)
	diagnostics := NewAuctionDiagnostics(start)
	campaign := &Campaign{ID: "campaign-a", UserID: "user-a"}
	diagnostics.registerCampaigns([]*Campaign{campaign})
	diagnostics.SetEnabled(true, start)

	var stop atomic.Bool
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for !stop.Load() {
				session := diagnostics.active.Load()
				if session == nil {
					continue
				}
				recorder, ok := session.begin(fmt.Sprintf("request-%d", worker))
				if !ok {
					continue
				}
				recorder.RecordRequestStart(1)
				recorder.RecordCampaign(campaign.diagnosticIndex, diagQualityMismatch)
				recorder.Close()
			}
		}(worker)
	}

	for iteration := 0; iteration < 20; iteration++ {
		now := start.Add(time.Duration(iteration+1) * time.Millisecond)
		diagnostics.SetEnabled(false, now)
		diagnostics.registerCampaigns([]*Campaign{campaign})
		diagnostics.SetEnabled(true, now)
	}
	stop.Store(true)
	workers.Wait()
	diagnostics.SetEnabled(false, start.Add(time.Second))
}

func TestMatchingCreativesUsesLastCreativeRejection(t *testing.T) {
	impID := "imp-1"
	width := int32(300)
	height := int32(250)
	imp := &ortb.Imp{
		Id: &impID,
		Banner: &ortb.Banner{
			W: &width,
			H: &height,
		},
	}
	campaign := &Campaign{
		Format: "BAN",
		Creatives: []*Creative{
			{ID: "creative-1", ADMURL: "https://example.test/1", W: 0, H: 0},
			{ID: "creative-2", ADMURL: "https://example.test/2", W: 728, H: 90},
			{ID: "creative-3", ADMURL: "", W: 300, H: 250},
		},
	}

	matched, reason := matchingCreativesWithLastRejection(campaign, imp, "BAN")
	if len(matched) != 0 {
		t.Fatalf("unexpected matching creatives: %d", len(matched))
	}
	if reason != diagCreativeADMURLEmpty {
		t.Fatalf("unexpected last-creative reason: got %s want %s", diagnosticReasonName(reason), diagnosticReasonName(diagCreativeADMURLEmpty))
	}
}

func TestCampaignFilterReturnsFirstRejection(t *testing.T) {
	country := "DE"
	siteID := "site-1"
	req := &ortb.BidRequest{
		Device: &ortb.Device{Geo: &ortb.Geo{Country: &country}},
		Site:   &ortb.Site{Id: &siteID},
	}
	campaign := &Campaign{
		ID:            "campaign-a",
		CountryFilter: filterV2.NewFilters(true, true, []string{"US"}),
		SiteIDFilter:  filterV2.NewFilters(true, true, []string{"site-2"}),
	}

	reason := campaignFilterRejectionWithDebug(campaign, req, "request-1", "imp-1", func(string, ...any) {})
	if reason != diagCountryFilterRejected {
		t.Fatalf("unexpected first filter rejection: got %s want %s", diagnosticReasonName(reason), diagnosticReasonName(diagCountryFilterRejected))
	}
}

func TestAntiPerekrutEligibilityHasSpecificDiagnosticCodes(t *testing.T) {
	manager := &AntiPerekrutManager{}
	campaign := &Campaign{ID: "campaign-a", UserID: "user-a"}
	state := newEmptyAntiPerekrutState()
	state.CampaignAuctionAllowed[campaign.ID] = false
	state.UserRemainingBalance[campaign.UserID] = 10
	state.UserSpend[campaign.UserID] = SpendPoint{Spend: 6}

	if got := diagnosticReasonForAntiPerekrutEligibility(manager.CampaignEligibility(state, campaign, false)); got != diagAntiPerekrutBalanceGuardRejected {
		t.Fatalf("unexpected balance-guard diagnostic: %s", diagnosticReasonName(got))
	}
	if got := diagnosticReasonForAntiPerekrutEligibility(manager.CampaignEligibility(state, campaign, true)); got != diagAntiPerekrutDurableUserBlock {
		t.Fatalf("unexpected durable-block diagnostic: %s", diagnosticReasonName(got))
	}
	delete(state.UserSpend, campaign.UserID)
	state.PendingUserBlocks[campaign.UserID] = time.Now().UTC()
	if got := diagnosticReasonForAntiPerekrutEligibility(manager.CampaignEligibility(state, campaign, false)); got != diagAntiPerekrutPendingUserBlock {
		t.Fatalf("unexpected pending-block diagnostic: %s", diagnosticReasonName(got))
	}
	delete(state.CampaignAuctionAllowed, campaign.ID)
	if got := diagnosticReasonForAntiPerekrutEligibility(manager.CampaignEligibility(state, campaign, false)); got != diagAntiPerekrutCampaignStateMissing {
		t.Fatalf("unexpected missing-state diagnostic: %s", diagnosticReasonName(got))
	}
}

func TestDiagnosticDefinitionsAreCompleteAndCodesUnique(t *testing.T) {
	seen := make(map[int]diagnosticReason, diagnosticReasonCount-1)
	for reason := diagnosticReason(1); reason < diagnosticReasonCount; reason++ {
		definition := diagnosticDefinitions[reason]
		if definition.Code <= 0 {
			t.Fatalf("reason %d has invalid public code %d", reason, definition.Code)
		}
		if definition.Name == "" || definition.Description == "" {
			t.Fatalf("reason %d has incomplete definition: %+v", reason, definition)
		}
		switch definition.Scope {
		case diagnosticScopeCampaign, diagnosticScopeGlobalRequest, diagnosticScopeGlobalImpression:
		default:
			t.Fatalf("reason %d has invalid scope %q", reason, definition.Scope)
		}
		if previous, duplicate := seen[definition.Code]; duplicate {
			t.Fatalf("duplicate public code %d for reasons %d and %d", definition.Code, previous, reason)
		}
		seen[definition.Code] = reason
	}
}

func TestPacingEligibilityReasonsHaveSpecificDiagnosticCodes(t *testing.T) {
	tests := []struct {
		input PacingEligibilityReason
		want  diagnosticReason
	}{
		{PacingEligibilityCurrentSlotMissing, diagPacingCurrentSlotMissing},
		{PacingEligibilityCurrentSlotReadFailed, diagPacingCurrentSlotReadFailed},
		{PacingEligibilityCurrentSlotKeyInvalid, diagPacingCurrentSlotKeyInvalid},
		{PacingEligibilitySlotSpentReadFailed, diagPacingSlotSpentReadFailed},
		{PacingEligibilitySlotTargetFailed, diagPacingSlotTargetFailed},
		{PacingEligibilityTargetNonPositive, diagPacingTargetNonPositive},
		{PacingEligibilitySlotLimitReached, diagPacingSlotLimitReached},
	}
	for _, test := range tests {
		if got := diagnosticReasonForPacingEligibility(test.input); got != test.want {
			t.Fatalf("unexpected pacing diagnostic for %d: got %s want %s", test.input, diagnosticReasonName(got), diagnosticReasonName(test.want))
		}
	}
}
