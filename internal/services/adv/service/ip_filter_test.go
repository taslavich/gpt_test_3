package auction

import (
	"errors"
	"testing"
)

func TestParseIPv4CampaignFilterAndMatch(t *testing.T) {
	filter, prefixes, err := parseIPv4CampaignFilter([]byte(`{"isWhiteList":true,"objects":["1.2.3.4","192.168.1.123/24","10.0.0.0/8"]}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !filter.Apply || !filter.IsWhiteList {
		t.Fatalf("unexpected filter: %+v", filter)
	}
	if !filter.Objects["1.2.3.4"] {
		t.Fatalf("exact IPv4 was not compiled: %+v", filter.Objects)
	}
	if len(prefixes) != 2 || prefixes[0].String() != "192.168.1.0/24" || prefixes[1].String() != "10.0.0.0/8" {
		t.Fatalf("prefixes=%v", prefixes)
	}

	for _, value := range []string{"1.2.3.4", "192.168.1.200", "10.50.60.70"} {
		value := value
		allowed, matched := ipv4CampaignFilterAllowed(filter, prefixes, &value)
		if !allowed || !matched {
			t.Fatalf("value=%s allowed=%t matched=%t", value, allowed, matched)
		}
	}
	outside := "8.8.8.8"
	if allowed, matched := ipv4CampaignFilterAllowed(filter, prefixes, &outside); allowed || matched {
		t.Fatalf("outside whitelist: allowed=%t matched=%t", allowed, matched)
	}
}

func TestIPv4CampaignFilterBlacklistAndMissingIP(t *testing.T) {
	filter, prefixes, err := parseIPv4CampaignFilter([]byte(`{"isWhiteList":false,"objects":["203.0.113.0/24"]}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	blocked := "203.0.113.50"
	if allowed, matched := ipv4CampaignFilterAllowed(filter, prefixes, &blocked); allowed || !matched {
		t.Fatalf("blocked CIDR: allowed=%t matched=%t", allowed, matched)
	}
	outside := "198.51.100.7"
	if allowed, matched := ipv4CampaignFilterAllowed(filter, prefixes, &outside); !allowed || matched {
		t.Fatalf("outside blacklist: allowed=%t matched=%t", allowed, matched)
	}
	if allowed, matched := ipv4CampaignFilterAllowed(filter, prefixes, nil); !allowed || matched {
		t.Fatalf("missing IP in blacklist: allowed=%t matched=%t", allowed, matched)
	}
}

func TestParseIPv4CampaignFilterRejectsInvalidAndIPv6(t *testing.T) {
	cases := []string{
		`{"isWhiteList":true,"objects":["999.10.1.1"]}`,
		`{"isWhiteList":true,"objects":["192.168.1.0/35"]}`,
		`{"isWhiteList":true,"objects":["2001:db8::1"]}`,
		`{"isWhiteList":true,"objects":["2001:db8::/32"]}`,
		`{"isWhiteList":true,"objects":[""]}`,
	}
	for _, raw := range cases {
		_, _, err := parseIPv4CampaignFilter([]byte(raw))
		if err == nil {
			t.Fatalf("expected error for %s", raw)
		}
		var target *InvalidCampaignIPFilterError
		if !errors.As(err, &target) {
			t.Fatalf("error type=%T want InvalidCampaignIPFilterError", err)
		}
	}
}
