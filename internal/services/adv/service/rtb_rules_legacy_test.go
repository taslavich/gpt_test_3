package auction

import (
	"context"
	"encoding/json"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"google.golang.org/protobuf/proto"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestRTBRulesActualPreflightAndOutboundIsolation(t *testing.T) {
	const a = "11111111-1111-1111-1111-111111111111"
	const b = "22222222-2222-2222-2222-222222222222"
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	config := []byte(`{"` + a + `":{"filters":[{"field":"imp.native.request","json_path":"assets[id=7].title.len","compare":">=","value":20}],"replace":[{"field":"imp.native.ver","mode":"set_or_add","value":"1.2"}]}}`)
	if err := os.WriteFile(path, config, 0600); err != nil {
		t.Fatal(err)
	}
	rules, err := rtbrules.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan struct {
		path    string
		request *ortb.BidRequest
	}, 2)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request ortb.BidRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		received <- struct {
			path    string
			request *ortb.BidRequest
		}{r.URL.Path, &request}
		w.WriteHeader(204)
	}))
	defer endpoint.Close()
	now := time.Now().UTC()
	campaign := func(id string) *Campaign {
		return &Campaign{ID: id, RTB: true, DSPLink: endpoint.URL + "/" + id, Format: "IPP", TrafficType: TrafficMainstream, Status: CampaignStatusActive, StartTS: now.Add(-time.Hour), EndTS: now.Add(time.Hour), QualitySegment: "usual"}
	}
	quality := &QualityStore{}
	quality.value.Store(&qualitySnapshot{bySegment: QualityMaps{"usual": {"ssp.example": true}}})
	service := &AuctionService{rtbRules: rules, rtbHTTPClient: endpoint.Client(), quality: quality}
	good, bad, small := "good", "bad", "small"
	valid, malformed, below := `{"assets":[{"id":7,"title":{"len":20}}]}`, `{`, `{"assets":[{"id":7,"title":{"len":19}}]}`
	version := "1.1"
	source := &ortb.BidRequest{Imp: []*ortb.Imp{{Id: &good, Native: &ortb.Native{Request: &valid, Ver: &version}}, {Id: &bad, Native: &ortb.Native{Request: &malformed, Ver: &version}}, {Id: &small, Native: &ortb.Native{Request: &below, Ver: &version}}}}
	before := proto.Clone(source)
	var mu sync.Mutex
	var traces []string
	logf := func(format string, args ...any) {
		mu.Lock()
		traces = append(traces, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	out := service.fetchRTBCampaignBids(context.Background(), source, &Snapshot{Campaigns: []*Campaign{campaign(a), campaign(b)}}, now, AuctionRequestOptions{}, "IPP", TrafficMainstream, "ssp.example", "", nil, false, nil, logf)
	if len(out) != 0 {
		t.Fatal("204 endpoints created bids", out)
	}
	got := map[string]*ortb.BidRequest{}
	for i := 0; i < 2; i++ {
		select {
		case r := <-received:
			got[r.path] = r.request
		case <-time.After(time.Second):
			t.Fatal("expected campaign not called")
		}
	}
	filtered := got["/"+a]
	plain := got["/"+b]
	if len(filtered.GetImp()) != 1 || filtered.GetImp()[0].GetId() != good || filtered.GetImp()[0].GetNative().GetVer() != "1.2" {
		t.Fatal("native preflight/outbound rules incorrect", filtered)
	}
	if len(plain.GetImp()) != 3 || plain.GetImp()[0].GetNative().GetVer() != "1.1" {
		t.Fatal("rules leaked to other campaign", plain)
	}
	if !proto.Equal(before, source) {
		t.Fatal("original request mutated")
	}
	mu.Lock()
	joined := strings.Join(traces, "\n")
	mu.Unlock()
	if !strings.Contains(joined, "rtb_rules_native_filter_error") || !strings.Contains(joined, "rtb_rule_filter_imp.native.request.assets[id=7].title.len") {
		t.Fatal("malformed native and normal mismatch not distinguished", joined)
	}
}
