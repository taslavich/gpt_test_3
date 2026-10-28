package auction

import (
	"encoding/json"
	jsoniter "github.com/json-iterator/go"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"reflect"
	"sync"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/rtbrules"
)

// Legacy test IDs are intentionally test-only. Production behavior is driven
// exclusively by the rules map loaded at ADV startup.
const (
	rtb54AdsChinaCampaignID       = "8cf66405-6013-423c-a967-a76c08c69ac4"
	rtb54AdsPhilippinesCampaignID = "9e4173f7-7aee-44a6-8039-db364cb6f4f5"
	rtb54AdsLatAmCampaignID       = "1c9eeda4-3cb1-499d-aada-83c3b07e042f"
)

var rtb54AdsCountryReplacements = map[string]map[string]string{
	rtb54AdsChinaCampaignID:       {"CN": "CHN"},
	rtb54AdsPhilippinesCampaignID: {"PH": "PHL"},
	rtb54AdsLatAmCampaignID:       {"MX": "MEX", "PE": "PER"},
}
var fixtureStore struct {
	once  sync.Once
	store *rtbrules.Store
	err   error
}

func legacyRuleStore(t *testing.T) *rtbrules.Store {
	t.Helper()
	fixtureStore.once.Do(func() {
		fixtureStore.store, fixtureStore.err = rtbrules.NewStore("../../../../cmd/adv/adv_rtb_rules_map.json")
	})
	if fixtureStore.err != nil {
		t.Fatal(fixtureStore.err)
	}
	return fixtureStore.store
}

// Keep old country-regression test cases but run them against the new rule
// engine and the migrated JSON file, never against production 54Ads hardcode.
func normalize54AdsRTBRequest(payload map[string]any, pop bool, countries map[string]string) {
	var campaign string
	for id, m := range rtb54AdsCountryReplacements {
		if reflect.DeepEqual(m, countries) {
			campaign = id
			break
		}
	}
	if campaign == "" {
		panic("unknown test countries")
	}
	// Pure test helper: load fixture and copy output back into the original map.
	b, _ := json.Marshal(payload)
	s, e := rtbrules.NewStore("../../../../cmd/adv/adv_rtb_rules_map.json")
	if e != nil {
		panic(e)
	}
	_ = pop // Campaign format is fixed, so the map has no "formats" branch.
	var req ortb.BidRequest
	if e = jsoniter.Unmarshal(b, &req); e != nil {
		panic(e)
	}
	b, e = s.ApplyToRequest(campaign, &req)
	if e != nil {
		panic(e)
	}
	for k := range payload {
		delete(payload, k)
	}
	if e = json.Unmarshal(b, &payload); e != nil {
		panic(e)
	}
}
