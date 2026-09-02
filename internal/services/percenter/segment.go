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

type SegmentLevel string

const (
	SegmentLevelExact                SegmentLevel = "exact"
	SegmentLevelCampaignSiteGeoDevOS SegmentLevel = "campaign_site_geo_device_os"
	SegmentLevelCampaignSiteGeoDev   SegmentLevel = "campaign_site_geo_device"
	SegmentLevelCampaignSite         SegmentLevel = "campaign_site"
	SegmentLevelCampaign             SegmentLevel = "campaign"
)

type SegmentNode struct {
	Level SegmentLevel
	Hash  string
}

// HashSegment preserves the original exact-segment hash for backwards
// compatibility with states and ORTB rows that already exist in Redis/ClickHouse.
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
	return hashParts(parts)
}

// SegmentHierarchy returns the analyst-approved fallback order, from the
// narrowest segment to the broadest:
//
//	campaign + site + geo + device + os + browser + ssp_domain
//	campaign + site + geo + device + os
//	campaign + site + geo + device
//	campaign + site
//	campaign
//
// Parent hashes include an explicit hierarchy version/level so that a parent
// node cannot collide with an exact segment whose omitted dimensions happen to
// be blank strings.
func SegmentHierarchy(segment Segment) []SegmentNode {
	campaign := strings.TrimSpace(segment.CampaignID)
	site := strings.TrimSpace(segment.SiteID)
	geo := normalizeUpper(segment.Geo)
	device := normalizeLower(segment.Device)
	osName := normalizeLower(segment.OS)

	return []SegmentNode{
		{Level: SegmentLevelExact, Hash: HashSegment(segment)},
		{Level: SegmentLevelCampaignSiteGeoDevOS, Hash: hashHierarchyNode(SegmentLevelCampaignSiteGeoDevOS, campaign, site, geo, device, osName)},
		{Level: SegmentLevelCampaignSiteGeoDev, Hash: hashHierarchyNode(SegmentLevelCampaignSiteGeoDev, campaign, site, geo, device)},
		{Level: SegmentLevelCampaignSite, Hash: hashHierarchyNode(SegmentLevelCampaignSite, campaign, site)},
		{Level: SegmentLevelCampaign, Hash: hashHierarchyNode(SegmentLevelCampaign, campaign)},
	}
}

func hashHierarchyNode(level SegmentLevel, parts ...string) string {
	values := make([]string, 0, len(parts)+2)
	values = append(values, "percenter-hierarchy-v1", string(level))
	values = append(values, parts...)
	return hashParts(values)
}

func hashParts(parts []string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func normalizeLower(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
func normalizeUpper(v string) string { return strings.ToUpper(strings.TrimSpace(v)) }
