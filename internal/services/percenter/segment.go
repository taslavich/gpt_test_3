package percenter

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type Segment struct {
	SSPDomain  string
	Geo        string
	Browser    string
	Device     string
	OS         string
	SiteID     string
	CampaignID string
}

func HashSegment(segment Segment) string {
	parts := []string{
		normalizeLower(segment.SSPDomain),
		normalizeUpper(segment.Geo),
		normalizeLower(segment.Browser),
		normalizeLower(segment.Device),
		normalizeLower(segment.OS),
		strings.TrimSpace(segment.SiteID),
		strings.TrimSpace(segment.CampaignID),
	}
	// Unit separator preserves empty positions without making a blank value
	// indistinguishable from an omitted field.
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func normalizeLower(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
func normalizeUpper(v string) string { return strings.ToUpper(strings.TrimSpace(v)) }
