package percenter

import (
	"strings"
	"time"
)

const (
	FallbackWindow         = 5 * time.Minute
	FallbackMinImpressions = uint64(5)
)

// SegmentTraffic is one exact segment reconstructed from raw ORTB dimensions.
// Impressions is the number of actual impression callbacks in FallbackWindow.
type SegmentTraffic struct {
	Segment     Segment
	Impressions uint64
}

type FallbackDecision struct {
	ExactHash     string
	SelectedHash  string
	SelectedLevel SegmentLevel
	Impressions   uint64
	HasSelection  bool
}

// BuildFallbackDecisions aggregates exact-segment impression counts into the
// approved hierarchy and, for every exact segment, chooses the first (narrowest)
// level that reached minImpressions. If even campaign is below the threshold,
// HasSelection is false and the current route must be left unchanged.
func BuildFallbackDecisions(traffic []SegmentTraffic, minImpressions uint64) []FallbackDecision {
	if minImpressions == 0 {
		minImpressions = FallbackMinImpressions
	}

	counts := make(map[string]uint64)
	for _, item := range traffic {
		if strings.TrimSpace(item.Segment.CampaignID) == "" {
			continue
		}
		for _, node := range SegmentHierarchy(item.Segment) {
			counts[node.Hash] += item.Impressions
		}
	}

	decisions := make([]FallbackDecision, 0, len(traffic))
	seenExact := make(map[string]struct{}, len(traffic))
	for _, item := range traffic {
		if strings.TrimSpace(item.Segment.CampaignID) == "" {
			continue
		}
		hierarchy := SegmentHierarchy(item.Segment)
		if len(hierarchy) == 0 {
			continue
		}
		exactHash := hierarchy[0].Hash
		if _, seen := seenExact[exactHash]; seen {
			continue
		}
		seenExact[exactHash] = struct{}{}

		decision := FallbackDecision{ExactHash: exactHash}
		for _, node := range hierarchy {
			if counts[node.Hash] < minImpressions {
				continue
			}
			decision.SelectedHash = node.Hash
			decision.SelectedLevel = node.Level
			decision.Impressions = counts[node.Hash]
			decision.HasSelection = true
			break
		}
		decisions = append(decisions, decision)
	}
	return decisions
}

// SetFallbackTarget changes only the routing of an exact segment. It does not
// alter its optimizer prices. PointVersion/LastChangeAt are advanced so stale
// metrics from the old routing period cannot be reused if the exact state later
// becomes active again.
func SetFallbackTarget(state State, targetHash string, now time.Time) (State, bool) {
	targetHash = strings.TrimSpace(targetHash)
	if targetHash == state.SegmentHash {
		targetHash = ""
	}
	if strings.TrimSpace(state.FallbackSegmentHash) == targetHash {
		return state, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	state.FallbackSegmentHash = targetHash
	state.PointVersion = nextPointVersion(state.PointVersion)
	state.LastChangeAt = now
	return state, true
}

func EffectiveStateHash(state State) string {
	if target := strings.TrimSpace(state.FallbackSegmentHash); target != "" && target != state.SegmentHash {
		return target
	}
	return state.SegmentHash
}
