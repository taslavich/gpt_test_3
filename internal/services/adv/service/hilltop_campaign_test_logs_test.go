package auction

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestHilltopCampaignTestDomainsAreOnlyTheTwoMappedFeeds(t *testing.T) {
	for _, domain := range []string{"mc_hilltopads.com", "adl_hilltopads.com"} {
		if !isHilltopCampaignTestDomain(domain) {
			t.Errorf("expected test logging for mapped SSP domain %q", domain)
		}
	}
	for _, domain := range []string{
		"mc_hilltopads.com_other", "mc_other.com", "adl_other.com",
		"mc_test", "adl_test", "", "mc_publisher.twinbidexchange.com",
	} {
		if isHilltopCampaignTestDomain(domain) {
			t.Errorf("unexpected extra logging for SSP domain %q", domain)
		}
	}
}

func TestHilltopCampaignTestLogsKeepOnlyFourCombinationsAndOriginalFormat(t *testing.T) {
	const firstCampaign = "1c9eeda4-3cb1-499d-aada-83c3b07e042f"
	const secondCampaign = "9e4173f7-7aee-44a6-8039-db364cb6f4f5"
	const otherCampaign = "b9d20ed5-654b-4c90-81e2-81d44a5b4caa"
	var output bytes.Buffer
	originalWriter := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(originalWriter)

	for _, domain := range []string{"mc_hilltopads.com", "adl_hilltopads.com"} {
		if !isHilltopCampaignTestDomain(domain) {
			t.Fatalf("test domain %q not enabled", domain)
		}
		// These are actual, unmodified ADV test log message formats.
		for _, campaignID := range []string{firstCampaign, secondCampaign} {
			logHilltopCampaignTestLine("[ADV][RTB_CAMPAIGN_SEEN] request_id=%q campaign_id=%q user_id=%q dsp_link=%q request_imps=%d", domain, campaignID, "user", "https://example.com/bid", 1)
			logHilltopCampaignTestLine("[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q format=%q campaign_id=%q user_id=%q reason=quality_mismatch campaign_quality=%q ssp_domain=%q", domain, "imp1", "POP", campaignID, "user", "high", domain)
		}
		logHilltopCampaignTestLine("[ADV][RTB_HTTP_RESULT] request_id=%q campaign_id=%q url=%q imp_ids=%v status=%d duration=%s", domain, otherCampaign, "https://example.com", []string{"imp1"}, 204, "10ms")
		logHilltopCampaignTestLine("[ADV][AUCTION_START] request_id=%q format=%q", domain, "POP")
		logHilltopCampaignTestLine("[ADV][RTB_DISCOVERY] request_id=%q rtb_campaigns=%d", domain, 3)
	}

	got := output.String()
	for _, domain := range []string{"mc_hilltopads.com", "adl_hilltopads.com"} {
		for _, campaignID := range []string{firstCampaign, secondCampaign} {
			if !strings.Contains(got, "[RTB_CAMPAIGN_SEEN] request_id=\""+domain+"\" campaign_id=\""+campaignID+"\"") {
				t.Errorf("missing original RTB line for domain %q and campaign %q: %s", domain, campaignID, got)
			}
			if !strings.Contains(got, "[CAMPAIGN_REJECT] request_id=\""+domain+"\" imp_id=\"imp1\" format=\"POP\" campaign_id=\""+campaignID+"\"") {
				t.Errorf("missing original rejection line for domain %q and campaign %q: %s", domain, campaignID, got)
			}
		}
	}
	if strings.Contains(got, otherCampaign) || strings.Contains(got, "AUCTION_START") || strings.Contains(got, "RTB_DISCOVERY") {
		t.Fatalf("unrelated campaign/request-wide line leaked into logs: %s", got)
	}
	if strings.Count(got, "[ADV]") != 8 { // 2 domains * 2 campaigns * 2 original messages
		t.Fatalf("got %d ADV log lines, want 8: %s", strings.Count(got, "[ADV]"), got)
	}
}
