package auction

import (
	"context"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
)

// diagnosticReason is intentionally dense: it is used as an array index on the
// auction hot path. Public numeric codes are defined separately in
// diagnosticDefinitions.
type diagnosticReason uint16

const (
	diagNone diagnosticReason = iota

	// Successful campaign outcome.
	diagBidWon

	// Campaign checks, in the same order as evaluateCampaign.
	diagCampaignFormatMismatch
	diagTrafficTypeMismatch
	diagCampaignStatusNotActive
	diagCampaignTimeWindowInvalid
	diagCampaignNotStarted
	diagCampaignEnded
	diagOutsideActiveIntervals
	diagAntiPerekrutDurableUserBlock
	diagAntiPerekrutPendingUserBlock
	diagAntiPerekrutBalanceGuardRejected
	diagAntiPerekrutCampaignStateMissing
	diagAntiPerekrutCampaignBlockedUnknown
	diagAntiPerekrutHashGateRejected
	diagQualityMismatch
	diagSiteIDQualityMismatch
	diagInvalidChargePrice
	diagCampaignSpentReadFailed
	diagCampaignBalanceInsufficient
	diagPacingCheckFailed
	diagPacingCurrentSlotReadFailed
	diagPacingCurrentSlotKeyInvalid
	diagPacingSlotSpentReadFailed
	diagPacingSlotTargetFailed
	diagPacingCurrentSlotMissing
	diagPacingTargetNonPositive
	diagPacingSlotLimitReached
	diagNoCreativesConfigured
	diagEffectivePriceNonPositive
	diagEffectivePriceBelowBidFloor

	// Campaign request filters.
	diagCountryFilterRejected
	diagLanguageFilterRejected
	diagDeviceTypeFilterRejected
	diagOSFilterRejected
	diagBrowserFilterRejected
	diagSiteIDFilterRejected
	diagIPFilterRejected

	// Creative checks. If every creative is rejected, the campaign receives the
	// reason of the last creative checked.
	diagCreativeNil
	diagCreativeIDEmpty
	diagCreativeADMURLEmpty
	diagBannerObjectMissing
	diagBannerSizesMissing
	diagBannerCreativeDimensionsInvalid
	diagBannerSizeMismatch
	diagBannerMimeMismatch
	diagNativeObjectMissing
	diagNativeRequestEmpty
	diagNativeRequestJSONInvalid
	diagNativePayloadJSONInvalid
	diagNativeAssetsEmpty
	diagNativeAssetIDInvalid
	diagNativeCampaignFormatInvalid
	diagNativeRequiredTitleMissing
	diagNativeRequiredBrandNameMissing
	diagNativeRequiredDescriptionMissing
	diagNativeRequiredDataTypeUnsupported
	diagNativeRequiredImageURLMissing
	diagNativeRequiredImageDimensionsInvalid
	diagNativeRequiredImageWidthBelowWMin
	diagNativeRequiredImageHeightBelowHMin
	diagNativeRequiredImageFormatUnsupported
	diagNativeRequiredImageNotEligible
	diagNativeRequiredAssetTypeUnsupported
	diagNativeADMBuildFailedUnknown
	diagCreativeNotMatchedUnknown

	// Candidate-pool and winner-selection outcomes.
	diagBelowWeightedTopThreshold
	diagLowerEffectivePriceThanWinner
	diagEqualTopPriceNotSelectedAfterShuffle
	diagNotSelectedByWeightedDraw
	diagWinnerSelectedBeforeAttempt
	diagNoWinnerSelected
	diagEligibleCandidateHasNoCreatives
	diagRandomCreativeNil
	diagBidBuildFailed
	diagWinnerRedisWriteFailed
	diagWeightedIndexUnavailable
	diagExcludedUnknownAuctionMode

	// Global request failures: campaign iteration did not start or the complete
	// auction could not be returned successfully.
	diagGlobalServiceDisabled
	diagGlobalBidRequestNil
	diagGlobalRequestIDEmpty
	diagGlobalImpressionsEmpty
	diagGlobalImpUUIDMapEmpty
	diagGlobalNoValidImpressions
	diagGlobalInvalidFormat
	diagGlobalInvalidTrafficType
	diagGlobalSSPDomainEmpty
	diagGlobalRuntimeStoreUnavailable
	diagGlobalWinnerStoreUnavailable
	diagGlobalPercentStoreUnavailable
	diagGlobalQualityStoreUnavailable
	diagGlobalSiteIDQualityStoreUnavailable
	diagGlobalCampaignSnapshotUnavailable
	diagGlobalAntiPerekrutManagerUnavailable
	diagGlobalAntiPerekrutStateUnavailable
	diagGlobalAuctionInfrastructureError

	// Global impression failures.
	diagGlobalImpressionNil
	diagGlobalImpressionIDEmpty
	diagGlobalImpressionIDDuplicate
	diagGlobalWinnerUUIDMissing
	diagGlobalWinnerUUIDDuplicate
	diagGlobalCampaignEntryNil
	diagGlobalCampaignIDEmpty
	diagGlobalCampaignSnapshotEmpty

	diagnosticReasonCount
)

type diagnosticScope string

const (
	diagnosticScopeCampaign         diagnosticScope = "campaign"
	diagnosticScopeGlobalRequest    diagnosticScope = "global_request"
	diagnosticScopeGlobalImpression diagnosticScope = "global_impression"
)

type diagnosticDefinition struct {
	Code        int
	Name        string
	Description string
	Scope       diagnosticScope
}

var diagnosticDefinitions = [diagnosticReasonCount]diagnosticDefinition{
	diagNone: {Code: 0, Name: "none", Description: "internal non-terminal marker", Scope: diagnosticScopeCampaign},

	diagBidWon: {Code: 200, Name: "bid_won", Description: "campaign won, bid was built and winner record was written", Scope: diagnosticScopeCampaign},

	diagCampaignFormatMismatch:             {Code: 300, Name: "campaign_format_mismatch", Description: "campaign format does not match requested format", Scope: diagnosticScopeCampaign},
	diagTrafficTypeMismatch:                {Code: 301, Name: "traffic_type_mismatch", Description: "campaign traffic type does not match request traffic type", Scope: diagnosticScopeCampaign},
	diagCampaignStatusNotActive:            {Code: 302, Name: "campaign_status_not_active", Description: "campaign status is not active", Scope: diagnosticScopeCampaign},
	diagCampaignTimeWindowInvalid:          {Code: 303, Name: "campaign_time_window_invalid", Description: "campaign start/end window is missing or invalid", Scope: diagnosticScopeCampaign},
	diagCampaignNotStarted:                 {Code: 304, Name: "campaign_not_started", Description: "current time is before campaign start", Scope: diagnosticScopeCampaign},
	diagCampaignEnded:                      {Code: 305, Name: "campaign_ended", Description: "current time is at or after campaign end", Scope: diagnosticScopeCampaign},
	diagOutsideActiveIntervals:             {Code: 306, Name: "outside_active_intervals", Description: "current time is outside configured active intervals", Scope: diagnosticScopeCampaign},
	diagAntiPerekrutDurableUserBlock:       {Code: 307, Name: "antiperekrut_durable_user_block", Description: "campaign user has a durable antiperekrut_blocked marker in the campaign snapshot", Scope: diagnosticScopeCampaign},
	diagAntiPerekrutPendingUserBlock:       {Code: 308, Name: "antiperekrut_pending_user_block", Description: "campaign user is blocked locally while the durable block is pending snapshot confirmation", Scope: diagnosticScopeCampaign},
	diagAntiPerekrutBalanceGuardRejected:   {Code: 324, Name: "antiperekrut_balance_guard_rejected", Description: "user spend from the last complete minute multiplied by two exceeds remaining user balance", Scope: diagnosticScopeCampaign},
	diagAntiPerekrutCampaignStateMissing:   {Code: 325, Name: "antiperekrut_campaign_state_missing", Description: "AntiPerekrut state has no CampaignAuctionAllowed entry for the campaign", Scope: diagnosticScopeCampaign},
	diagAntiPerekrutCampaignBlockedUnknown: {Code: 326, Name: "antiperekrut_campaign_blocked_unknown", Description: "AntiPerekrut rejected the campaign but the published state does not expose a more specific reason", Scope: diagnosticScopeCampaign},
	diagAntiPerekrutHashGateRejected:       {Code: 327, Name: "antiperekrut_hash_gate_rejected", Description: "campaign did not pass the AntiPerekrut traffic-percent hash gate", Scope: diagnosticScopeCampaign},
	diagQualityMismatch:                    {Code: 309, Name: "quality_mismatch", Description: "SSP domain is absent from the campaign quality segment", Scope: diagnosticScopeCampaign},
	diagSiteIDQualityMismatch:              {Code: 328, Name: "site_id_quality_mismatch", Description: "request site.id failed the second quality-map stage for the campaign quality segment", Scope: diagnosticScopeCampaign},
	diagInvalidChargePrice:                 {Code: 310, Name: "invalid_charge_price", Description: "calculated charge price is non-positive or non-finite", Scope: diagnosticScopeCampaign},
	diagCampaignSpentReadFailed:            {Code: 311, Name: "campaign_spent_read_failed", Description: "campaign spend could not be read from runtime Redis", Scope: diagnosticScopeCampaign},
	diagCampaignBalanceInsufficient:        {Code: 312, Name: "campaign_balance_insufficient", Description: "campaign remaining budget is smaller than charge price", Scope: diagnosticScopeCampaign},
	diagPacingCheckFailed:                  {Code: 313, Name: "pacing_check_failed", Description: "pacing eligibility check returned an unclassified infrastructure or data error", Scope: diagnosticScopeCampaign},
	diagPacingCurrentSlotReadFailed:        {Code: 320, Name: "pacing_current_slot_read_failed", Description: "current pacing slot key could not be read from runtime Redis", Scope: diagnosticScopeCampaign},
	diagPacingCurrentSlotKeyInvalid:        {Code: 321, Name: "pacing_current_slot_key_invalid", Description: "current pacing slot points to an invalid slot-spend key", Scope: diagnosticScopeCampaign},
	diagPacingSlotSpentReadFailed:          {Code: 322, Name: "pacing_slot_spent_read_failed", Description: "current pacing slot spend could not be read or parsed", Scope: diagnosticScopeCampaign},
	diagPacingSlotTargetFailed:             {Code: 323, Name: "pacing_slot_target_failed", Description: "target for the current pacing slot could not be calculated", Scope: diagnosticScopeCampaign},
	diagPacingCurrentSlotMissing:           {Code: 314, Name: "pacing_current_slot_missing", Description: "current pacing slot key is absent", Scope: diagnosticScopeCampaign},
	diagPacingTargetNonPositive:            {Code: 315, Name: "pacing_target_non_positive", Description: "calculated target for the current pacing slot is non-positive", Scope: diagnosticScopeCampaign},
	diagPacingSlotLimitReached:             {Code: 316, Name: "pacing_slot_limit_reached", Description: "current slot spend reached its pacing target", Scope: diagnosticScopeCampaign},
	diagNoCreativesConfigured:              {Code: 317, Name: "no_creatives_configured", Description: "campaign has no creatives", Scope: diagnosticScopeCampaign},
	diagEffectivePriceNonPositive:          {Code: 318, Name: "effective_price_non_positive", Description: "effective auction price after deduction is non-positive or non-finite", Scope: diagnosticScopeCampaign},
	diagEffectivePriceBelowBidFloor:        {Code: 319, Name: "effective_price_below_bidfloor", Description: "effective auction price is below impression bid floor", Scope: diagnosticScopeCampaign},
	diagCountryFilterRejected:              {Code: 330, Name: "country_filter_rejected", Description: "request failed campaign country filter", Scope: diagnosticScopeCampaign},
	diagLanguageFilterRejected:             {Code: 331, Name: "language_filter_rejected", Description: "request failed campaign language filter", Scope: diagnosticScopeCampaign},
	diagDeviceTypeFilterRejected:           {Code: 332, Name: "device_type_filter_rejected", Description: "request failed campaign device-type filter", Scope: diagnosticScopeCampaign},
	diagOSFilterRejected:                   {Code: 333, Name: "os_filter_rejected", Description: "request failed campaign OS filter", Scope: diagnosticScopeCampaign},
	diagBrowserFilterRejected:              {Code: 334, Name: "browser_filter_rejected", Description: "request failed campaign browser filter", Scope: diagnosticScopeCampaign},
	diagSiteIDFilterRejected:               {Code: 335, Name: "site_id_filter_rejected", Description: "request failed campaign site_id filter", Scope: diagnosticScopeCampaign},
	diagIPFilterRejected:                   {Code: 336, Name: "ip_filter_rejected", Description: "request failed campaign IP filter", Scope: diagnosticScopeCampaign},

	diagCreativeNil:                          {Code: 400, Name: "creative_nil", Description: "last checked creative is nil", Scope: diagnosticScopeCampaign},
	diagCreativeIDEmpty:                      {Code: 401, Name: "creative_id_empty", Description: "last checked creative has an empty ID", Scope: diagnosticScopeCampaign},
	diagCreativeADMURLEmpty:                  {Code: 402, Name: "creative_adm_url_empty", Description: "last checked creative has an empty ADM URL", Scope: diagnosticScopeCampaign},
	diagBannerObjectMissing:                  {Code: 403, Name: "banner_object_missing", Description: "BAN request impression has no banner object", Scope: diagnosticScopeCampaign},
	diagBannerSizesMissing:                   {Code: 404, Name: "banner_sizes_missing", Description: "BAN request contains no valid banner size", Scope: diagnosticScopeCampaign},
	diagBannerCreativeDimensionsInvalid:      {Code: 405, Name: "banner_creative_dimensions_invalid", Description: "last checked banner creative has invalid dimensions", Scope: diagnosticScopeCampaign},
	diagBannerSizeMismatch:                   {Code: 406, Name: "banner_size_mismatch", Description: "last checked banner creative size is not requested", Scope: diagnosticScopeCampaign},
	diagBannerMimeMismatch:                   {Code: 407, Name: "banner_mime_mismatch", Description: "last checked banner creative MIME type is not accepted", Scope: diagnosticScopeCampaign},
	diagNativeObjectMissing:                  {Code: 410, Name: "native_object_missing", Description: "NAT/IPP impression has no native object", Scope: diagnosticScopeCampaign},
	diagNativeRequestEmpty:                   {Code: 411, Name: "native_request_empty", Description: "native request payload is empty", Scope: diagnosticScopeCampaign},
	diagNativeRequestJSONInvalid:             {Code: 412, Name: "native_request_json_invalid", Description: "outer native request JSON is invalid", Scope: diagnosticScopeCampaign},
	diagNativePayloadJSONInvalid:             {Code: 413, Name: "native_payload_json_invalid", Description: "inner native payload JSON is invalid", Scope: diagnosticScopeCampaign},
	diagNativeAssetsEmpty:                    {Code: 414, Name: "native_assets_empty", Description: "native request has no assets", Scope: diagnosticScopeCampaign},
	diagNativeAssetIDInvalid:                 {Code: 415, Name: "native_asset_id_invalid", Description: "native request contains an invalid asset ID", Scope: diagnosticScopeCampaign},
	diagNativeCampaignFormatInvalid:          {Code: 416, Name: "native_campaign_format_invalid", Description: "campaign format is not NAT or IPP during native creative validation", Scope: diagnosticScopeCampaign},
	diagNativeRequiredTitleMissing:           {Code: 417, Name: "native_required_title_missing", Description: "last checked creative has no required native title", Scope: diagnosticScopeCampaign},
	diagNativeRequiredBrandNameMissing:       {Code: 418, Name: "native_required_brand_name_missing", Description: "campaign has no required native brand name", Scope: diagnosticScopeCampaign},
	diagNativeRequiredDescriptionMissing:     {Code: 419, Name: "native_required_description_missing", Description: "last checked creative has no required native description", Scope: diagnosticScopeCampaign},
	diagNativeRequiredDataTypeUnsupported:    {Code: 420, Name: "native_required_data_type_unsupported", Description: "required native data asset type is unsupported", Scope: diagnosticScopeCampaign},
	diagNativeRequiredImageURLMissing:        {Code: 421, Name: "native_required_image_url_missing", Description: "last checked creative has no required image URL", Scope: diagnosticScopeCampaign},
	diagNativeRequiredImageDimensionsInvalid: {Code: 422, Name: "native_required_image_dimensions_invalid", Description: "last checked native image has invalid dimensions", Scope: diagnosticScopeCampaign},
	diagNativeRequiredImageWidthBelowWMin:    {Code: 423, Name: "native_required_image_width_below_wmin", Description: "last checked native image width is below wmin", Scope: diagnosticScopeCampaign},
	diagNativeRequiredImageHeightBelowHMin:   {Code: 424, Name: "native_required_image_height_below_hmin", Description: "last checked native image height is below hmin", Scope: diagnosticScopeCampaign},
	diagNativeRequiredImageFormatUnsupported: {Code: 425, Name: "native_required_image_format_unsupported", Description: "last checked native image format is unsupported", Scope: diagnosticScopeCampaign},
	diagNativeRequiredImageNotEligible:       {Code: 426, Name: "native_required_image_not_eligible", Description: "required native image is not eligible for an unclassified reason", Scope: diagnosticScopeCampaign},
	diagNativeRequiredAssetTypeUnsupported:   {Code: 427, Name: "native_required_asset_type_unsupported", Description: "required native asset kind is unsupported", Scope: diagnosticScopeCampaign},
	diagNativeADMBuildFailedUnknown:          {Code: 428, Name: "native_adm_build_failed_unknown", Description: "native ADM build failed for an unclassified reason", Scope: diagnosticScopeCampaign},
	diagCreativeNotMatchedUnknown:            {Code: 429, Name: "creative_not_matched_unknown", Description: "creative did not match for an unclassified format-specific reason", Scope: diagnosticScopeCampaign},

	diagBelowWeightedTopThreshold:            {Code: 500, Name: "below_weighted_top_threshold", Description: "eligible campaign was excluded from weighted-top pool", Scope: diagnosticScopeCampaign},
	diagLowerEffectivePriceThanWinner:        {Code: 501, Name: "lower_effective_price_than_winner", Description: "eligible campaign had a lower effective price than the max-bid winner", Scope: diagnosticScopeCampaign},
	diagEqualTopPriceNotSelectedAfterShuffle: {Code: 502, Name: "equal_top_price_not_selected_after_shuffle", Description: "equal top-price campaign was not first after tie shuffle", Scope: diagnosticScopeCampaign},
	diagNotSelectedByWeightedDraw:            {Code: 503, Name: "not_selected_by_weighted_draw", Description: "eligible campaign was not selected before the weighted-draw winner", Scope: diagnosticScopeCampaign},
	diagWinnerSelectedBeforeAttempt:          {Code: 504, Name: "winner_selected_before_attempt", Description: "another campaign won before this eligible campaign was attempted", Scope: diagnosticScopeCampaign},
	diagNoWinnerSelected:                     {Code: 505, Name: "no_winner_selected", Description: "eligible campaign remained unattempted because no winner was selected", Scope: diagnosticScopeCampaign},
	diagEligibleCandidateHasNoCreatives:      {Code: 510, Name: "eligible_candidate_has_no_creatives", Description: "defensive check: eligible candidate unexpectedly has no creatives", Scope: diagnosticScopeCampaign},
	diagRandomCreativeNil:                    {Code: 511, Name: "random_creative_nil", Description: "randomly selected creative is nil", Scope: diagnosticScopeCampaign},
	diagBidBuildFailed:                       {Code: 512, Name: "bid_build_failed", Description: "bid could not be built from the selected creative", Scope: diagnosticScopeCampaign},
	diagWinnerRedisWriteFailed:               {Code: 513, Name: "winner_redis_write_failed", Description: "winner record could not be written to winner Redis", Scope: diagnosticScopeCampaign},
	diagWeightedIndexUnavailable:             {Code: 514, Name: "weighted_index_unavailable", Description: "weighted selection could not produce a candidate index", Scope: diagnosticScopeCampaign},
	diagExcludedUnknownAuctionMode:           {Code: 515, Name: "excluded_unknown_auction_mode", Description: "candidate pool excluded the campaign due to an unknown auction mode", Scope: diagnosticScopeCampaign},

	diagGlobalServiceDisabled:                {Code: 899, Name: "service_disabled", Description: "ADV work controller rejected the request before auction", Scope: diagnosticScopeGlobalRequest},
	diagGlobalBidRequestNil:                  {Code: 900, Name: "bid_request_nil", Description: "OpenRTB bid request is nil", Scope: diagnosticScopeGlobalRequest},
	diagGlobalRequestIDEmpty:                 {Code: 901, Name: "request_id_empty", Description: "OpenRTB request ID is empty", Scope: diagnosticScopeGlobalRequest},
	diagGlobalImpressionsEmpty:               {Code: 902, Name: "impressions_empty", Description: "OpenRTB request contains no impressions", Scope: diagnosticScopeGlobalRequest},
	diagGlobalImpUUIDMapEmpty:                {Code: 903, Name: "imp_uuid_map_empty", Description: "impression-to-winner-UUID map is empty", Scope: diagnosticScopeGlobalRequest},
	diagGlobalNoValidImpressions:             {Code: 904, Name: "no_valid_impressions", Description: "request contains no non-nil impression with a usable ID", Scope: diagnosticScopeGlobalRequest},
	diagGlobalInvalidFormat:                  {Code: 905, Name: "invalid_format", Description: "request format is empty or unsupported", Scope: diagnosticScopeGlobalRequest},
	diagGlobalInvalidTrafficType:             {Code: 906, Name: "invalid_traffic_type", Description: "request traffic type is empty or unsupported", Scope: diagnosticScopeGlobalRequest},
	diagGlobalSSPDomainEmpty:                 {Code: 907, Name: "ssp_domain_empty", Description: "normalized SSP domain is empty", Scope: diagnosticScopeGlobalRequest},
	diagGlobalRuntimeStoreUnavailable:        {Code: 908, Name: "runtime_store_unavailable", Description: "ADV runtime store is not initialized", Scope: diagnosticScopeGlobalRequest},
	diagGlobalWinnerStoreUnavailable:         {Code: 909, Name: "winner_store_unavailable", Description: "ADV winner store is not initialized", Scope: diagnosticScopeGlobalRequest},
	diagGlobalPercentStoreUnavailable:        {Code: 910, Name: "percent_store_unavailable", Description: "ADV percent store is not initialized", Scope: diagnosticScopeGlobalRequest},
	diagGlobalQualityStoreUnavailable:        {Code: 911, Name: "quality_store_unavailable", Description: "ADV quality store is not initialized", Scope: diagnosticScopeGlobalRequest},
	diagGlobalSiteIDQualityStoreUnavailable:  {Code: 916, Name: "site_id_quality_store_unavailable", Description: "ADV site ID quality store is not initialized", Scope: diagnosticScopeGlobalRequest},
	diagGlobalCampaignSnapshotUnavailable:    {Code: 912, Name: "campaign_snapshot_unavailable", Description: "campaign snapshot is unavailable", Scope: diagnosticScopeGlobalRequest},
	diagGlobalAntiPerekrutManagerUnavailable: {Code: 913, Name: "antiperekrut_manager_unavailable", Description: "AntiPerekrut manager is unavailable while enabled", Scope: diagnosticScopeGlobalRequest},
	diagGlobalAntiPerekrutStateUnavailable:   {Code: 914, Name: "antiperekrut_state_unavailable", Description: "AntiPerekrut state is unavailable or not loaded", Scope: diagnosticScopeGlobalRequest},
	diagGlobalAuctionInfrastructureError:     {Code: 915, Name: "auction_infrastructure_error", Description: "auction returned an infrastructure error after campaign checks", Scope: diagnosticScopeGlobalRequest},
	diagGlobalImpressionNil:                  {Code: 930, Name: "impression_nil", Description: "impression entry is nil", Scope: diagnosticScopeGlobalImpression},
	diagGlobalImpressionIDEmpty:              {Code: 931, Name: "impression_id_empty", Description: "impression ID is empty", Scope: diagnosticScopeGlobalImpression},
	diagGlobalImpressionIDDuplicate:          {Code: 932, Name: "impression_id_duplicate", Description: "impression ID is duplicated in the request", Scope: diagnosticScopeGlobalImpression},
	diagGlobalWinnerUUIDMissing:              {Code: 933, Name: "winner_uuid_missing", Description: "winner UUID is missing for the impression", Scope: diagnosticScopeGlobalImpression},
	diagGlobalWinnerUUIDDuplicate:            {Code: 934, Name: "winner_uuid_duplicate", Description: "winner UUID is reused by multiple impressions", Scope: diagnosticScopeGlobalImpression},
	diagGlobalCampaignEntryNil:               {Code: 935, Name: "campaign_entry_nil", Description: "campaign snapshot contains a nil campaign entry", Scope: diagnosticScopeGlobalImpression},
	diagGlobalCampaignIDEmpty:                {Code: 936, Name: "campaign_id_empty", Description: "campaign snapshot entry has an empty campaign ID", Scope: diagnosticScopeGlobalImpression},
	diagGlobalCampaignSnapshotEmpty:          {Code: 937, Name: "campaign_snapshot_empty", Description: "campaign snapshot contains no campaigns for this impression", Scope: diagnosticScopeGlobalImpression},
}

const (
	diagnosticShardCount          = 32
	diagnosticCampaignBlockSize   = 64
	diagnosticCampaignReasonCount = int(diagGlobalServiceDisabled)
	invalidDiagnosticIndex        = ^uint32(0)
)

type diagnosticCampaignMeta struct {
	CampaignID string
	UserID     string
}

type diagnosticRegistrySnapshot struct {
	Campaigns []diagnosticCampaignMeta
}

type diagnosticCampaignCounter struct {
	Counts [diagnosticShardCount][diagnosticCampaignReasonCount]atomic.Uint64
}

type diagnosticCampaignBlock [diagnosticCampaignBlockSize]diagnosticCampaignCounter
type diagnosticCampaignBlocks []*diagnosticCampaignBlock

type diagnosticGlobalShard struct {
	RequestTotal     atomic.Uint64
	ImpressionTotal  atomic.Uint64
	RequestCounts    [diagnosticReasonCount]atomic.Uint64
	ImpressionCounts [diagnosticReasonCount]atomic.Uint64
}

type diagnosticBuffer struct {
	windowStart time.Time
	windowEnd   time.Time
	partial     bool
	writers     atomic.Int64
	blocks      atomic.Pointer[diagnosticCampaignBlocks]
	globals     [diagnosticShardCount]diagnosticGlobalShard
	resizeMu    sync.Mutex
}

func newDiagnosticBuffer(start, end time.Time, partial bool, campaignCount int) *diagnosticBuffer {
	buffer := &diagnosticBuffer{
		windowStart: start.UTC(),
		windowEnd:   end.UTC(),
		partial:     partial,
	}
	buffer.ensureCampaignCapacity(campaignCount)
	return buffer
}

func (buffer *diagnosticBuffer) ensureCampaignCapacity(campaignCount int) {
	if buffer == nil || campaignCount <= 0 {
		return
	}
	requiredBlocks := (campaignCount + diagnosticCampaignBlockSize - 1) / diagnosticCampaignBlockSize
	current := buffer.blocks.Load()
	if current != nil && len(*current) >= requiredBlocks {
		return
	}

	buffer.resizeMu.Lock()
	defer buffer.resizeMu.Unlock()

	current = buffer.blocks.Load()
	if current != nil && len(*current) >= requiredBlocks {
		return
	}
	next := make(diagnosticCampaignBlocks, requiredBlocks)
	if current != nil {
		copy(next, *current)
	}
	for index := range next {
		if next[index] == nil {
			next[index] = &diagnosticCampaignBlock{}
		}
	}
	buffer.blocks.Store(&next)
}

func recordDiagnosticCampaign(
	blocks *diagnosticCampaignBlocks,
	shard uint32,
	campaignIndex uint32,
	reason diagnosticReason,
) bool {
	if blocks == nil || shard >= diagnosticShardCount ||
		campaignIndex == invalidDiagnosticIndex ||
		!validReasonForScope(reason, diagnosticScopeCampaign) ||
		reason == diagNone {
		return false
	}
	blockIndex := int(campaignIndex) / diagnosticCampaignBlockSize
	entryIndex := int(campaignIndex) % diagnosticCampaignBlockSize
	if blockIndex < 0 || blockIndex >= len(*blocks) || (*blocks)[blockIndex] == nil {
		return false
	}
	(*blocks)[blockIndex][entryIndex].Counts[shard][reason].Add(1)
	return true
}

type diagnosticSession struct {
	startedAt time.Time
	current   atomic.Pointer[diagnosticBuffer]
	rotateMu  sync.Mutex
}

func newDiagnosticSession(now time.Time, campaignCount int) *diagnosticSession {
	now = now.UTC()
	minuteStart := now.Truncate(time.Minute)
	partial := !now.Equal(minuteStart)
	end := minuteStart.Add(time.Minute)
	if !partial {
		end = now.Add(time.Minute)
	}
	session := &diagnosticSession{startedAt: now}
	session.current.Store(newDiagnosticBuffer(now, end, partial, campaignCount))
	return session
}

type AuctionDiagnostics struct {
	replica string

	active    atomic.Pointer[diagnosticSession]
	published atomic.Pointer[AuctionDiagnosticsSnapshot]
	registry  atomic.Pointer[diagnosticRegistrySnapshot]

	registryMu sync.Mutex
	indexByID  map[string]uint32

	controlMu sync.Mutex
	startOnce sync.Once
	wake      chan struct{}
}

func NewAuctionDiagnostics(now time.Time) *AuctionDiagnostics {
	replica, _ := os.Hostname()
	d := &AuctionDiagnostics{
		replica:   strings.TrimSpace(replica),
		indexByID: make(map[string]uint32),
		wake:      make(chan struct{}, 1),
	}
	d.registry.Store(&diagnosticRegistrySnapshot{})
	d.published.Store(emptyAuctionDiagnosticsSnapshot(d.replica, now.UTC(), now.UTC(), false))
	return d
}

func (d *AuctionDiagnostics) Start(ctx context.Context) {
	if d == nil {
		return
	}
	d.startOnce.Do(func() {
		go d.rotationLoop(ctx)
	})
}

func (d *AuctionDiagnostics) rotationLoop(ctx context.Context) {
	for {
		session := d.active.Load()
		if session == nil {
			select {
			case <-ctx.Done():
				return
			case <-d.wake:
				continue
			}
		}
		buffer := session.current.Load()
		if buffer == nil {
			select {
			case <-ctx.Done():
				return
			case <-d.wake:
				continue
			}
		}
		delay := time.Until(buffer.windowEnd)
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-d.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case boundary := <-timer.C:
			session.rotate(d, boundary.UTC())
		}
	}
}

func (d *AuctionDiagnostics) signalWake() {
	if d == nil {
		return
	}
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *AuctionDiagnostics) registerCampaigns(campaigns []*Campaign) {
	if d == nil {
		return
	}
	d.registryMu.Lock()
	// While diagnostics are disabled, indexes can be compacted to the current
	// campaign snapshot because no auction writes diagnostic counters. While a
	// session is active, indexes stay stable and new campaigns are appended so
	// in-flight auctions holding an older snapshot cannot be misattributed.
	active := d.active.Load() != nil
	current := d.registry.Load()
	metas := make([]diagnosticCampaignMeta, 0)
	if active && current != nil {
		metas = append(metas, current.Campaigns...)
	} else {
		d.indexByID = make(map[string]uint32, len(campaigns))
	}
	for _, campaign := range campaigns {
		if campaign == nil {
			continue
		}
		campaignID := strings.TrimSpace(campaign.ID)
		if campaignID == "" {
			campaign.diagnosticIndex = invalidDiagnosticIndex
			continue
		}
		index, exists := d.indexByID[campaignID]
		if !exists {
			index = uint32(len(metas))
			d.indexByID[campaignID] = index
			metas = append(metas, diagnosticCampaignMeta{
				CampaignID: campaignID,
				UserID:     strings.TrimSpace(campaign.UserID),
			})
		} else if int(index) < len(metas) {
			metas[index] = diagnosticCampaignMeta{
				CampaignID: campaignID,
				UserID:     strings.TrimSpace(campaign.UserID),
			}
		}
		campaign.diagnosticIndex = index
	}
	next := &diagnosticRegistrySnapshot{Campaigns: metas}
	d.registry.Store(next)
	d.registryMu.Unlock()

	if session := d.active.Load(); session != nil {
		if buffer := session.current.Load(); buffer != nil {
			buffer.ensureCampaignCapacity(len(metas))
		}
	}
}

func (d *AuctionDiagnostics) SetEnabled(enabled bool, now time.Time) AuctionDiagnosticsStatus {
	if d == nil {
		return AuctionDiagnosticsStatus{}
	}
	d.controlMu.Lock()
	defer d.controlMu.Unlock()

	if enabled {
		if d.active.Load() == nil {
			// Serialize session creation with disabled-mode registry compaction.
			// Once active is published, registerCampaigns preserves all indexes.
			d.registryMu.Lock()
			registry := d.registry.Load()
			campaignCount := 0
			if registry != nil {
				campaignCount = len(registry.Campaigns)
			}
			session := newDiagnosticSession(now, campaignCount)
			d.active.Store(session)
			d.registryMu.Unlock()

			buffer := session.current.Load()
			if buffer != nil {
				d.published.Store(emptyAuctionDiagnosticsSnapshot(
					d.replica,
					buffer.windowStart,
					buffer.windowEnd,
					true,
				))
			}
			d.signalWake()
		}
		return d.Status()
	}

	session := d.active.Swap(nil)
	d.signalWake()
	if session != nil {
		session.finish(d, now.UTC())
	}
	return d.Status()
}

func (d *AuctionDiagnostics) Status() AuctionDiagnosticsStatus {
	status := AuctionDiagnosticsStatus{CoveragePercent: 100}
	if d == nil {
		return status
	}
	session := d.active.Load()
	status.Enabled = session != nil
	if session != nil {
		status.StartedAt = session.startedAt
		if buffer := session.current.Load(); buffer != nil {
			status.CurrentWindowStart = buffer.windowStart
			status.CurrentWindowEnd = buffer.windowEnd
			status.Partial = buffer.partial
		}
	}
	if snapshot := d.published.Load(); snapshot != nil {
		status.LastPublishedStart = snapshot.WindowStart
		status.LastPublishedEnd = snapshot.WindowEnd
		status.Ready = snapshot.Ready
	}
	return status
}

func (session *diagnosticSession) rotate(manager *AuctionDiagnostics, now time.Time) {
	if session == nil || manager == nil {
		return
	}
	session.rotateMu.Lock()
	defer session.rotateMu.Unlock()
	if manager.active.Load() != session {
		return
	}
	old := session.current.Load()
	if old == nil || now.Before(old.windowEnd) {
		return
	}
	registry := manager.registry.Load()
	campaignCount := 0
	if registry != nil {
		campaignCount = len(registry.Campaigns)
	}
	start := old.windowEnd
	fresh := newDiagnosticBuffer(start, start.Add(time.Minute), false, campaignCount)
	session.current.Store(fresh)
	waitDiagnosticWriters(old)
	manager.published.Store(old.snapshot(manager.replica, registry, true))
}

func (session *diagnosticSession) finish(manager *AuctionDiagnostics, now time.Time) {
	if session == nil || manager == nil {
		return
	}
	session.rotateMu.Lock()
	defer session.rotateMu.Unlock()
	old := session.current.Swap(nil)
	if old == nil {
		return
	}
	waitDiagnosticWriters(old)
	windowEnd := old.windowEnd
	if now.After(old.windowStart) && now.Before(old.windowEnd) {
		windowEnd = now
	}
	registry := manager.registry.Load()
	manager.published.Store(old.snapshotWithBounds(manager.replica, registry, false, windowEnd, true))
}

func waitDiagnosticWriters(buffer *diagnosticBuffer) {
	if buffer == nil {
		return
	}
	for buffer.writers.Load() != 0 {
		runtime.Gosched()
	}
}

type auctionDiagnosticRecorder struct {
	buffer *diagnosticBuffer
	blocks *diagnosticCampaignBlocks
	shard  uint32
	closed bool
}

func (session *diagnosticSession) begin(requestID string) (auctionDiagnosticRecorder, bool) {
	if session == nil {
		return auctionDiagnosticRecorder{}, false
	}
	for {
		buffer := session.current.Load()
		if buffer == nil {
			return auctionDiagnosticRecorder{}, false
		}
		buffer.writers.Add(1)
		if session.current.Load() != buffer {
			buffer.writers.Add(-1)
			continue
		}
		shard := uint32(xxhash.Sum64String(strings.TrimSpace(requestID)) & (diagnosticShardCount - 1))
		return auctionDiagnosticRecorder{
			buffer: buffer,
			blocks: buffer.blocks.Load(),
			shard:  shard,
		}, true
	}
}

func (recorder *auctionDiagnosticRecorder) Close() {
	if recorder == nil || recorder.closed {
		return
	}
	recorder.closed = true
	recorder.buffer.writers.Add(-1)
}

func (recorder *auctionDiagnosticRecorder) RecordRequestStart(impressions int) {
	if recorder == nil || recorder.buffer == nil {
		return
	}
	global := &recorder.buffer.globals[recorder.shard]
	global.RequestTotal.Add(1)
	if impressions > 0 {
		global.ImpressionTotal.Add(uint64(impressions))
	}
}

func (recorder *auctionDiagnosticRecorder) RecordGlobalRequest(reason diagnosticReason) {
	if recorder == nil || recorder.buffer == nil ||
		!validReasonForScope(reason, diagnosticScopeGlobalRequest) {
		return
	}
	recorder.buffer.globals[recorder.shard].RequestCounts[reason].Add(1)
}

func (recorder *auctionDiagnosticRecorder) RecordGlobalImpression(reason diagnosticReason) {
	if recorder == nil || recorder.buffer == nil ||
		!validReasonForScope(reason, diagnosticScopeGlobalImpression) {
		return
	}
	recorder.buffer.globals[recorder.shard].ImpressionCounts[reason].Add(1)
}

func (recorder *auctionDiagnosticRecorder) RecordCampaign(campaignIndex uint32, reason diagnosticReason) {
	if recorder == nil || recorder.buffer == nil {
		return
	}
	if recordDiagnosticCampaign(recorder.blocks, recorder.shard, campaignIndex, reason) {
		return
	}
	// A campaign snapshot may be published after Begin but before Auction loads
	// that snapshot. Capacity is grown before snapshot publication, so one lazy
	// refresh covers this rare boundary race without an atomic load per result.
	recorder.blocks = recorder.buffer.blocks.Load()
	_ = recordDiagnosticCampaign(recorder.blocks, recorder.shard, campaignIndex, reason)
}

func validReasonForScope(reason diagnosticReason, scope diagnosticScope) bool {
	return reason > diagNone && reason < diagnosticReasonCount && diagnosticDefinitions[reason].Scope == scope
}

type DiagnosticCodeSnapshot struct {
	Name    string  `json:"name"`
	Count   uint64  `json:"count"`
	Percent float64 `json:"percent"`
}

type DiagnosticBucketSnapshot struct {
	Total uint64                            `json:"total"`
	Codes map[string]DiagnosticCodeSnapshot `json:"codes"`
}

type CampaignDiagnosticSnapshot struct {
	UserID string                            `json:"user_id,omitempty"`
	Total  uint64                            `json:"total"`
	Codes  map[string]DiagnosticCodeSnapshot `json:"codes"`
}

type GlobalDiagnosticsSnapshot struct {
	Requests    DiagnosticBucketSnapshot `json:"requests"`
	Impressions DiagnosticBucketSnapshot `json:"impressions"`
}

type DiagnosticCodeDefinitionSnapshot struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
}

type AuctionDiagnosticsSnapshot struct {
	Replica         string                                      `json:"replica,omitempty"`
	Enabled         bool                                        `json:"enabled"`
	CoveragePercent int                                         `json:"coverage_percent"`
	Ready           bool                                        `json:"ready"`
	Partial         bool                                        `json:"partial"`
	WindowStart     time.Time                                   `json:"window_start"`
	WindowEnd       time.Time                                   `json:"window_end"`
	GeneratedAt     time.Time                                   `json:"generated_at"`
	Global          GlobalDiagnosticsSnapshot                   `json:"global"`
	Campaigns       map[string]CampaignDiagnosticSnapshot       `json:"campaigns"`
	Codebook        map[string]DiagnosticCodeDefinitionSnapshot `json:"codebook"`
}

type AuctionDiagnosticsStatus struct {
	Enabled            bool      `json:"enabled"`
	CoveragePercent    int       `json:"coverage_percent"`
	StartedAt          time.Time `json:"started_at,omitempty"`
	CurrentWindowStart time.Time `json:"current_window_start,omitempty"`
	CurrentWindowEnd   time.Time `json:"current_window_end,omitempty"`
	Partial            bool      `json:"partial"`
	Ready              bool      `json:"ready"`
	LastPublishedStart time.Time `json:"last_published_start,omitempty"`
	LastPublishedEnd   time.Time `json:"last_published_end,omitempty"`
}

func emptyAuctionDiagnosticsSnapshot(replica string, start, end time.Time, enabled bool) *AuctionDiagnosticsSnapshot {
	return &AuctionDiagnosticsSnapshot{
		Replica:         replica,
		Enabled:         enabled,
		CoveragePercent: 100,
		Ready:           false,
		Partial:         true,
		WindowStart:     start.UTC(),
		WindowEnd:       end.UTC(),
		GeneratedAt:     time.Now().UTC(),
		Global: GlobalDiagnosticsSnapshot{
			Requests:    DiagnosticBucketSnapshot{Codes: map[string]DiagnosticCodeSnapshot{}},
			Impressions: DiagnosticBucketSnapshot{Codes: map[string]DiagnosticCodeSnapshot{}},
		},
		Campaigns: map[string]CampaignDiagnosticSnapshot{},
		Codebook:  diagnosticCodebook(),
	}
}

func (buffer *diagnosticBuffer) snapshot(replica string, registry *diagnosticRegistrySnapshot, enabled bool) *AuctionDiagnosticsSnapshot {
	return buffer.snapshotWithBounds(replica, registry, enabled, buffer.windowEnd, buffer.partial)
}

func (buffer *diagnosticBuffer) snapshotWithBounds(
	replica string,
	registry *diagnosticRegistrySnapshot,
	enabled bool,
	windowEnd time.Time,
	partial bool,
) *AuctionDiagnosticsSnapshot {
	result := &AuctionDiagnosticsSnapshot{
		Replica:         replica,
		Enabled:         enabled,
		CoveragePercent: 100,
		Ready:           true,
		Partial:         partial,
		WindowStart:     buffer.windowStart,
		WindowEnd:       windowEnd.UTC(),
		GeneratedAt:     time.Now().UTC(),
		Global: GlobalDiagnosticsSnapshot{
			Requests:    buffer.snapshotGlobalRequests(),
			Impressions: buffer.snapshotGlobalImpressions(),
		},
		Campaigns: make(map[string]CampaignDiagnosticSnapshot),
		Codebook:  diagnosticCodebook(),
	}
	if registry == nil {
		return result
	}
	blocks := buffer.blocks.Load()
	if blocks == nil {
		return result
	}
	for campaignIndex, meta := range registry.Campaigns {
		blockIndex := campaignIndex / diagnosticCampaignBlockSize
		entryIndex := campaignIndex % diagnosticCampaignBlockSize
		if blockIndex >= len(*blocks) || (*blocks)[blockIndex] == nil {
			continue
		}
		counter := &(*blocks)[blockIndex][entryIndex]
		bucket := snapshotCampaignCounter(counter)
		if bucket.Total == 0 {
			continue
		}
		result.Campaigns[meta.CampaignID] = CampaignDiagnosticSnapshot{
			UserID: meta.UserID,
			Total:  bucket.Total,
			Codes:  bucket.Codes,
		}
	}
	return result
}

func (buffer *diagnosticBuffer) snapshotGlobalRequests() DiagnosticBucketSnapshot {
	result := DiagnosticBucketSnapshot{Codes: make(map[string]DiagnosticCodeSnapshot)}
	for shard := range buffer.globals {
		result.Total += buffer.globals[shard].RequestTotal.Load()
	}
	for reason := diagnosticReason(1); reason < diagnosticReasonCount; reason++ {
		if diagnosticDefinitions[reason].Scope != diagnosticScopeGlobalRequest {
			continue
		}
		var count uint64
		for shard := range buffer.globals {
			count += buffer.globals[shard].RequestCounts[reason].Load()
		}
		addDiagnosticSnapshotCode(&result, reason, count)
	}
	return result
}

func (buffer *diagnosticBuffer) snapshotGlobalImpressions() DiagnosticBucketSnapshot {
	result := DiagnosticBucketSnapshot{Codes: make(map[string]DiagnosticCodeSnapshot)}
	for shard := range buffer.globals {
		result.Total += buffer.globals[shard].ImpressionTotal.Load()
	}
	for reason := diagnosticReason(1); reason < diagnosticReasonCount; reason++ {
		if diagnosticDefinitions[reason].Scope != diagnosticScopeGlobalImpression {
			continue
		}
		var count uint64
		for shard := range buffer.globals {
			count += buffer.globals[shard].ImpressionCounts[reason].Load()
		}
		addDiagnosticSnapshotCode(&result, reason, count)
	}
	return result
}

func snapshotCampaignCounter(counter *diagnosticCampaignCounter) DiagnosticBucketSnapshot {
	result := DiagnosticBucketSnapshot{Codes: make(map[string]DiagnosticCodeSnapshot)}
	if counter == nil {
		return result
	}
	for reason := diagnosticReason(1); reason < diagnosticReasonCount; reason++ {
		if diagnosticDefinitions[reason].Scope != diagnosticScopeCampaign {
			continue
		}
		var count uint64
		for shard := 0; shard < diagnosticShardCount; shard++ {
			count += counter.Counts[shard][reason].Load()
		}
		if count > 0 {
			result.Total += count
		}
	}
	if result.Total == 0 {
		return result
	}
	for reason := diagnosticReason(1); reason < diagnosticReasonCount; reason++ {
		if diagnosticDefinitions[reason].Scope != diagnosticScopeCampaign {
			continue
		}
		var count uint64
		for shard := 0; shard < diagnosticShardCount; shard++ {
			count += counter.Counts[shard][reason].Load()
		}
		addDiagnosticSnapshotCode(&result, reason, count)
	}
	return result
}

func addDiagnosticSnapshotCode(bucket *DiagnosticBucketSnapshot, reason diagnosticReason, count uint64) {
	if bucket == nil || count == 0 {
		return
	}
	definition := diagnosticDefinitions[reason]
	percent := 0.0
	if bucket.Total > 0 {
		percent = roundDiagnosticPercent(float64(count) * 100 / float64(bucket.Total))
	}
	bucket.Codes[strconv.Itoa(definition.Code)] = DiagnosticCodeSnapshot{
		Name:    definition.Name,
		Count:   count,
		Percent: percent,
	}
}

func roundDiagnosticPercent(value float64) float64 {
	return math.Round(value*1_000_000) / 1_000_000
}

func diagnosticCodebook() map[string]DiagnosticCodeDefinitionSnapshot {
	codes := make([]int, 0, diagnosticReasonCount-1)
	byCode := make(map[int]diagnosticDefinition, diagnosticReasonCount-1)
	for reason := diagnosticReason(1); reason < diagnosticReasonCount; reason++ {
		definition := diagnosticDefinitions[reason]
		codes = append(codes, definition.Code)
		byCode[definition.Code] = definition
	}
	sort.Ints(codes)
	result := make(map[string]DiagnosticCodeDefinitionSnapshot, len(codes))
	for _, code := range codes {
		definition := byCode[code]
		result[strconv.Itoa(code)] = DiagnosticCodeDefinitionSnapshot{
			Name:        definition.Name,
			Description: definition.Description,
			Scope:       string(definition.Scope),
		}
	}
	return result
}

func (d *AuctionDiagnostics) Snapshot() *AuctionDiagnosticsSnapshot {
	if d == nil {
		return nil
	}
	published := d.published.Load()
	if published == nil {
		return nil
	}
	copy := *published
	copy.Enabled = d.active.Load() != nil
	return &copy
}

func diagnosticReasonName(reason diagnosticReason) string {
	if reason <= diagNone || reason >= diagnosticReasonCount {
		return "unknown"
	}
	return diagnosticDefinitions[reason].Name
}

func (s *AuctionService) StartDiagnostics(ctx context.Context) {
	if s != nil && s.diagnostics != nil {
		s.diagnostics.Start(ctx)
	}
}

func (s *AuctionService) SetDiagnosticsEnabled(enabled bool) AuctionDiagnosticsStatus {
	if s == nil || s.diagnostics == nil {
		return AuctionDiagnosticsStatus{}
	}
	status := s.diagnostics.SetEnabled(enabled, time.Now().UTC())
	if !enabled {
		// With no active writers, compact the stable ID registry back to the
		// campaigns in the current snapshot so deleted campaign IDs do not retain
		// counter capacity for the lifetime of the process.
		if snapshot := s.currentSnapshot(); snapshot != nil {
			s.diagnostics.registerCampaigns(snapshot.Campaigns)
		}
	}
	return status
}

func (s *AuctionService) DiagnosticsStatus() AuctionDiagnosticsStatus {
	if s == nil || s.diagnostics == nil {
		return AuctionDiagnosticsStatus{}
	}
	return s.diagnostics.Status()
}

func (s *AuctionService) DiagnosticsSnapshot() *AuctionDiagnosticsSnapshot {
	if s == nil || s.diagnostics == nil {
		return nil
	}
	return s.diagnostics.Snapshot()
}

// RecordServiceDisabledRequest accounts for a gRPC request rejected by the work
// controller before Auction can be entered. It has no cost while diagnostics are
// disabled beyond one active-session pointer load.
func (s *AuctionService) RecordServiceDisabledRequest(requestID string, impressions int) {
	if s == nil || s.diagnostics == nil {
		return
	}
	session := s.diagnostics.active.Load()
	if session == nil {
		return
	}
	recorder, ok := session.begin(requestID)
	if !ok {
		return
	}
	defer recorder.Close()
	recorder.RecordRequestStart(impressions)
	recorder.RecordGlobalRequest(diagGlobalServiceDisabled)
}
