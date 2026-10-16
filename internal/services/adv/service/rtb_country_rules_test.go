package auction

import (
	"encoding/json"
	"reflect"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"google.golang.org/protobuf/proto"
)

func countryTestRequest(country string) *ortb.BidRequest {
	impID, subID := "imp-1", "publisher-subid"
	deviceType, width, tmax := int32(4), int32(300), int32(100)
	userID := "source-user"
	domain, page := "https://example.com/path", "/landing"
	return &ortb.BidRequest{
		Tmax:   &tmax,
		Device: &ortb.Device{DeviceType: &deviceType, Geo: &ortb.Geo{Country: &country}},
		User:   &ortb.User{Id: &userID},
		Site:   &ortb.Site{Domain: &domain, Page: &page},
		Imp:    []*ortb.Imp{{Id: &impID, Banner: &ortb.Banner{W: &width}, Ext: &ortb.Imp_Ext{Subid: &subID}}},
	}
}

func countryTestPayload(t *testing.T, req *ortb.BidRequest, campaign *Campaign, format string) map[string]any {
	t.Helper()
	before := proto.Clone(req)
	body, err := marshalRTBRequestForCampaign(req, campaign, format)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, req) {
		t.Fatal("source request mutated")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func Test54AdsCountryRulesAreCampaignSpecific(t *testing.T) {
	campaigns := []struct {
		id        string
		countries map[string]string
	}{
		{rtb54AdsChinaCampaignID, map[string]string{"CN": "CHN"}},
		{rtb54AdsPhilippinesCampaignID, map[string]string{"PH": "PHL"}},
		{rtb54AdsLatAmCampaignID, map[string]string{"MX": "MEX", "PE": "PER"}},
	}
	for _, campaign := range campaigns {
		for _, country := range []string{"CN", "PH", "MX", "PE", "US", "CHN", "PHL", "MEX", "PER"} {
			t.Run(campaign.id+"/"+country, func(t *testing.T) {
				req := countryTestRequest(country)
				payload := countryTestPayload(t, req, &Campaign{ID: campaign.id, UserID: "unrelated-owner", RTB: true}, constants.POP)
				want := country
				if replacement, ok := campaign.countries[country]; ok {
					want = replacement
				}
				device := payload["device"].(map[string]any)
				user := payload["user"].(map[string]any)
				if got := device["geo"].(map[string]any)["country"]; got != want {
					t.Fatalf("device country=%v want=%s", got, want)
				}
				if want != country {
					if got := user["geo"].(map[string]any)["country"]; got != want {
						t.Fatalf("user fallback country=%v want=%s", got, want)
					}
				} else if _, exists := user["geo"]; exists {
					t.Fatal("unmatched device country populated user geo")
				}
				if user["id"] != "source-user" {
					t.Fatal("existing user id lost")
				}
				if payload["tmax"] != float64(500) || device["devicetype"] != float64(4) {
					t.Fatal("existing request normalization lost")
				}
				if _, exists := device["deviceType"]; exists {
					t.Fatal("old deviceType field remains")
				}
				site := payload["site"].(map[string]any)
				if site["domain"] != "example.com" || site["page"] != "https://example.com/landing" {
					t.Fatalf("site normalization lost: %+v", site)
				}
				imp := payload["imp"].([]any)[0].(map[string]any)
				if _, exists := imp["banner"]; exists {
					t.Fatal("POP still has banner")
				}
				ext := imp["ext"].(map[string]any)
				if ext["type"] != "pop" || ext["subid"] != "publisher-subid" {
					t.Fatalf("imp ext changed: %+v", ext)
				}
			})
		}
	}
}

func Test54AdsCountryFallbackOnlyAfterCampaignSpecificDeviceReplacement(t *testing.T) {
	rules := []struct{ id, from, to string }{
		{rtb54AdsChinaCampaignID, "CN", "CHN"},
		{rtb54AdsPhilippinesCampaignID, "PH", "PHL"},
		{rtb54AdsLatAmCampaignID, "MX", "MEX"},
		{rtb54AdsLatAmCampaignID, "PE", "PER"},
	}
	for _, rule := range rules {
		for _, userState := range []string{"missing user", "missing geo", "missing country", "empty country", "whitespace country", "existing country"} {
			t.Run(rule.id+"/"+rule.from+"/"+userState, func(t *testing.T) {
				payload := map[string]any{"device": map[string]any{"geo": map[string]any{"country": rule.from}}}
				switch userState {
				case "missing user":
				case "missing geo":
					payload["user"] = map[string]any{}
				case "missing country":
					payload["user"] = map[string]any{"geo": map[string]any{}}
				case "empty country":
					payload["user"] = map[string]any{"geo": map[string]any{"country": ""}}
				case "whitespace country":
					payload["user"] = map[string]any{"geo": map[string]any{"country": "  "}}
				case "existing country":
					payload["user"] = map[string]any{"geo": map[string]any{"country": "US"}}
				}
				normalize54AdsRTBRequest(payload, true, rtb54AdsCountryReplacements[rule.id])
				user := payload["user"].(map[string]any)
				want := rule.to
				if userState == "existing country" {
					want = "US"
				}
				if got := user["geo"].(map[string]any)["country"]; got != want {
					t.Fatalf("user country=%v want=%s", got, want)
				}
			})
		}
		for _, country := range []string{"US", rule.to} {
			t.Run(rule.id+"/no-fallback/"+country, func(t *testing.T) {
				req := countryTestRequest(country)
				req.User = nil
				payload := countryTestPayload(t, req, &Campaign{ID: rule.id, RTB: true}, constants.POP)
				if _, exists := payload["user"]; exists {
					t.Fatal("no matching device replacement, but user was created")
				}
			})
		}
	}
}

func Test54AdsCountryFallbackDoesNotLeakOtherCampaignCountries(t *testing.T) {
	for _, pair := range []struct{ id, country string }{
		{rtb54AdsChinaCampaignID, "PH"}, {rtb54AdsChinaCampaignID, "MX"}, {rtb54AdsChinaCampaignID, "PE"},
		{rtb54AdsPhilippinesCampaignID, "CN"}, {rtb54AdsPhilippinesCampaignID, "MX"}, {rtb54AdsPhilippinesCampaignID, "PE"},
		{rtb54AdsLatAmCampaignID, "CN"}, {rtb54AdsLatAmCampaignID, "PH"},
	} {
		t.Run(pair.id+"/"+pair.country, func(t *testing.T) {
			req := countryTestRequest(pair.country)
			req.User = nil
			payload := countryTestPayload(t, req, &Campaign{ID: pair.id, RTB: true}, constants.POP)
			if _, exists := payload["user"]; exists {
				t.Fatal("country rule leaked from a different campaign")
			}
			if got := payload["device"].(map[string]any)["geo"].(map[string]any)["country"]; got != pair.country {
				t.Fatalf("unrelated country changed: %v", got)
			}
		})
	}
}

func Test54AdsNormalizationNoLongerSelectedByOwner(t *testing.T) {
	for _, campaign := range []*Campaign{
		{ID: "other-campaign", UserID: "94416e97-6a74-42e5-8902-65e6cf7ac13c", RTB: true},
		{ID: "other-campaign", UserID: "other-owner", RTB: true},
		nil,
	} {
		req := countryTestRequest("CN")
		payload := countryTestPayload(t, req, campaign, constants.POP)
		body, err := jsoniter.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		var original map[string]any
		if err := json.Unmarshal(body, &original); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(payload, original) {
			t.Fatalf("unrelated campaign request changed: %+v", payload)
		}
	}
}

func Test54AdsFallbackPreservesUserAndGeoFields(t *testing.T) {
	payload := map[string]any{
		"device": map[string]any{"geo": map[string]any{"country": " ph "}},
		"user":   map[string]any{"id": "user-1", "geo": map[string]any{"city": "Manila", "lat": 14.6}},
	}
	normalize54AdsRTBRequest(payload, true, rtb54AdsCountryReplacements[rtb54AdsPhilippinesCampaignID])
	user := payload["user"].(map[string]any)
	geo := user["geo"].(map[string]any)
	if user["id"] != "user-1" || geo["city"] != "Manila" || geo["lat"] != 14.6 || geo["country"] != "PHL" {
		t.Fatalf("fields lost: %+v", payload)
	}
}

func Test54AdsExistingUserCountryNormalizesIndependently(t *testing.T) {
	for _, tt := range []struct{ device, user, wantDevice, wantUser string }{
		{"MX", "PE", "MEX", "PER"},
		{"PE", "MX", "PER", "MEX"},
		{"CN", "PE", "CN", "PER"},
	} {
		payload := map[string]any{
			"device": map[string]any{"geo": map[string]any{"country": tt.device}},
			"user":   map[string]any{"geo": map[string]any{"country": tt.user}},
		}
		normalize54AdsRTBRequest(payload, true, rtb54AdsCountryReplacements[rtb54AdsLatAmCampaignID])
		device := payload["device"].(map[string]any)["geo"].(map[string]any)["country"]
		user := payload["user"].(map[string]any)["geo"].(map[string]any)["country"]
		if device != tt.wantDevice || user != tt.wantUser {
			t.Fatalf("device=%v user=%v", device, user)
		}
	}
	// Missing device must not prevent normalization of an existing user country.
	payload := map[string]any{"user": map[string]any{"geo": map[string]any{"country": "PH"}}}
	normalize54AdsRTBRequest(payload, true, rtb54AdsCountryReplacements[rtb54AdsPhilippinesCampaignID])
	if got := payload["user"].(map[string]any)["geo"].(map[string]any)["country"]; got != "PHL" {
		t.Fatalf("user country=%v", got)
	}
}

func Test54AdsAllCampaignsKeepBannerOutsidePOP(t *testing.T) {
	for _, campaignID := range []string{rtb54AdsChinaCampaignID, rtb54AdsPhilippinesCampaignID, rtb54AdsLatAmCampaignID} {
		payload := countryTestPayload(t, countryTestRequest("PH"), &Campaign{ID: campaignID, RTB: true}, constants.BAN)
		imp := payload["imp"].([]any)[0].(map[string]any)
		if _, exists := imp["banner"]; !exists {
			t.Fatal("BAN banner removed")
		}
		if imp["ext"].(map[string]any)["type"] != "pop" {
			t.Fatal("existing imp.ext.type normalization changed")
		}
	}
}
