package rtbrules

import (
	"bytes"
	"encoding/json"
	jsoniter "github.com/json-iterator/go"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const idA = "8cf66405-6013-423c-a967-a76c08c69ac4"
const idB = "9e4173f7-7aee-44a6-8039-db364cb6f4f5"
const idC = "1c9eeda4-3cb1-499d-aada-83c3b07e042f"

func runRules(t *testing.T, s *Store, campaign string, data []byte) []byte {
	t.Helper()
	var req ortb.BidRequest
	if err := jsoniter.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	out, err := s.ApplyToRequest(campaign, &req)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func load54(t *testing.T) *Store {
	t.Helper()
	p := "../../../../cmd/adv/adv_rtb_rules_map.json"
	s, e := NewStore(p)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func Test54AdsMapParity(t *testing.T) {
	s := load54(t)
	for _, tc := range []struct{ id, from, to string }{{idA, "CN", "CHN"}, {idB, "PH", "PHL"}, {idC, "MX", "MEX"}, {idC, "PE", "PER"}} {
		input := `{"tmax":100,"device":{"deviceType":4,"geo":{"country":"` + tc.from + `"}},"site":{"domain":"https://example.com/p","page":"/landing"},"imp":[{"id":"a","banner":{"w":300},"ext":{"subid":"sub"}}]}`
		out := runRules(t, s, tc.id, []byte(input))
		var m map[string]any
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatal(err)
		}
		if m["tmax"] != float64(500) {
			t.Fatalf("tmax %+v", m)
		}
		dev := m["device"].(map[string]any)
		if dev["devicetype"] != float64(4) || dev["deviceType"] != nil {
			t.Fatalf("device %+v", dev)
		}
		if dev["geo"].(map[string]any)["country"] != tc.to {
			t.Fatalf("country %+v", dev)
		}
		user := m["user"].(map[string]any)
		if user["geo"].(map[string]any)["country"] != tc.to {
			t.Fatalf("fallback %+v", user)
		}
		site := m["site"].(map[string]any)
		if site["domain"] != "example.com" || site["page"] != "https://example.com/landing" {
			t.Fatalf("site %+v", site)
		}
		imp := m["imp"].([]any)[0].(map[string]any)
		if imp["banner"] != nil || imp["ext"].(map[string]any)["subid"] != "sub" || imp["ext"].(map[string]any)["type"] != "pop" {
			t.Fatalf("imp %+v", imp)
		}
		// Deletion is campaign-only. No per-rule format branches exist.
		if len(ruleSetForTest(t, s, tc.id).Delete) != 1 {
			t.Fatal("missing pointer delete rule")
		}

	}
	if s.Has("f9d2679a-dad3-4be5-8e4e-4511bc977fff") {
		t.Fatal("random campaign inherited 54Ads rules")
	}
}
func Test54AdsExistingUserAndMismatch(t *testing.T) {
	s := load54(t)
	a := []byte(`{"device":{"geo":{"country":"PH"}},"user":{"id":"u","geo":{"country":"US","city":"Paris"}}}`)
	b := runRules(t, s, idB, a)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	geo := m["user"].(map[string]any)["geo"].(map[string]any)
	if geo["country"] != "PHL" || geo["city"] != "Paris" {
		t.Fatalf("copy_to did not overwrite user geo country while preserving city %+v", geo)
	}
	b = runRules(t, s, idA, a)
	var other map[string]any
	_ = json.Unmarshal(b, &other)
	if other["device"].(map[string]any)["geo"].(map[string]any)["country"] != "PH" {
		t.Fatal("country escaped campaign rules")
	}
	if other["user"].(map[string]any)["geo"].(map[string]any)["country"] != "PH" {
		t.Fatal("other campaign mutated user geo")
	}
}
func TestManualGroupedAndDynamicRules(t *testing.T) {
	cfg := `{"` + idA + `,` + idB + `":{"filters":[{"field":"imp.video.protocols","isWhiteList":true,"values":[3]},{"field":"device.os","exists":true}],"add":[{"field":"imp.ext","postfix_field":"type","value":"pop"}],"replace":[{"field":"device.geo.country","mode":"iso"}],"delete":[{"field":"imp.banner"}],"rename":[{"field":"device.deviceType","to_field":"devicetype"}]}}`
	path := filepath.Join(t.TempDir(), "rtb.json")
	if e := os.WriteFile(path, []byte(cfg), 0600); e != nil {
		t.Fatal(e)
	}
	s, e := NewStore(path)
	if e != nil {
		t.Fatal(e)
	}
	if !s.Has(idA) || !s.Has(idB) || len(s.Memory()) != 2 {
		t.Fatal("grouped campaign IDs not expanded")
	}
	saved, e := s.Saved()
	if e != nil {
		t.Fatal(e)
	}
	if len(saved) != 1 {
		t.Fatal("GET must preserve grouped representation")
	}
	req := []byte(`{"device":{"os":"Android","geo":{"country":"PH"}},"imp":[{"id":"a","video":{"protocols":[2,3]},"banner":{"w":1}},{"id":"b","video":{"protocols":[2]}}]}`)
	ok, _, e := s.Check(idA, req, "a")
	if e != nil || !ok {
		t.Fatalf("expected pass %v %v", ok, e)
	}
	ok, reason, e := s.Check(idA, req, "b")
	if e != nil || ok || !strings.Contains(reason, "protocols") {
		t.Fatalf("expected reject %v %s %v", ok, reason, e)
	}
	got := runRules(t, s, idA, req)
	var m map[string]any
	_ = json.Unmarshal(got, &m)
	if m["device"].(map[string]any)["geo"].(map[string]any)["country"] != "PHL" {
		t.Fatal("iso failed")
	}
	for _, raw := range m["imp"].([]any) {
		imp := raw.(map[string]any)
		if imp["banner"] != nil || imp["ext"].(map[string]any)["type"] != "pop" {
			t.Fatalf("imp changed incorrectly: %+v", imp)
		}
	}
	if !bytes.Equal(req, []byte(`{"device":{"os":"Android","geo":{"country":"PH"}},"imp":[{"id":"a","video":{"protocols":[2,3]},"banner":{"w":1}},{"id":"b","video":{"protocols":[2]}}]}`)) {
		t.Fatal("source modified")
	}
}
func ruleSetForTest(t *testing.T, s *Store, id string) RuleSet {
	t.Helper()
	r, ok := s.lookup(id)
	if !ok {
		t.Fatal("missing campaign")
	}
	return r
}
func TestRejectInvalidAndKeepLastValid(t *testing.T) {
	good := []byte(`{"` + idA + `":{"add":[{"field":"imp.ext","postfix_field":"type","value":"pop"}]}}`)
	path := filepath.Join(t.TempDir(), "map.json")
	if e := os.WriteFile(path, good, 0600); e != nil {
		t.Fatal(e)
	}
	s, e := NewStore(path)
	if e != nil {
		t.Fatal(e)
	}
	invalid := []string{
		`{"` + idA + `":{"filters":[{"field":"device.os","type":"exists","exists":true}]}}`,
		`{"` + idA + `":{"filters":[{"field":"device.os"}]}}`,
		`{"` + idA + `":{"filters":[{"field":"device.os","values":["Android"],"isWhiteList":true,"exists":true}]}}`,
		`{"` + idA + `":{"filters":[{"field":"device.os","values":[],"isWhiteList":true,"exists":false}]}}`,
		`{"` + idA + `":{"filters":[{"field":"device.os","values":null,"exists":true}]}}`,
		`{"` + idA + `":{"filters":[{"field":"device.os","exists":null}]}}`,
		`{"` + idA + `":{"filters":[{"field":"device.os","values":null,"isWhiteList":true}]}}`,
		`{"` + idA + `":{"filters":[{"field":"device.os","values":[],"isWhiteList":true}]}}`,
		`{"` + idA + `":{"delete":[{"field":"random.field"}]}}`,
		`{"` + idA + `":{"add":[{"field":"imp.video.w","value":"wrong"}]}}`,
		`{"` + idA + `":{"filters":[{"field":"imp.video.protocols","isWhiteList":true,"values":["3"]}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.geo.country","mode":"iso","countries":{"PH":"PHL"}}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.geo.country","mode":"iso","countries":{}}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.geo.country","mode":"iso","countries":null}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.geo.country","mode":"set","value":"PHL","countries":{"PH":"PHL"}}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.geo.country","mode":"countries","countries":{"PH":"PHL"}}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.geo.country","countries":{}}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.geo.country","countries":null}]}}`,
		`{"` + idA + `":{"rename":[{"field":"device.deviceType","to_field":"device.devicetype"}]}}`,
		`{"` + idA + `":{"rename":[{"field":"device.deviceType","to_field":"deviceType"}]}}`,
		`{"` + idA + `":{"rename":[{"field":"device.deviceType","to_field":"ip"}]}}`,
		`{"` + idA + `":{"wat":1}}`,
		`{"` + idA + `":{"add":[]},"` + idA + `":{"delete":[]}}`,
		`{"` + idA + `,` + idA + `":{}}`,
		`{"` + idA + `":{"filters":[{"field":"device.os","exists":true,"isWhiteList":false}]}}`,
		`{"` + idA + `":{"add":[{"field":"imp.video.protocols","value":["3"]}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.os","mode":"iso"}]}}`,
		`{"` + idA + `":{"delete":[{"field":"imp.video.protocols"}]}}`,
		`{"` + idA + `":{"delete":[{"field":"device.devicetype"}]}}`,
		`{"` + idA + `":{"filters":[{"field":"device.devicetype","exists":true}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.devicetype","mode":"set","value":4}]}}`,
		`{"` + idA + `":{"delete":[{"field":"not.a.proto"}]}}`,
		`{"` + idA + `":{"delete":[{"field":"imp.banner"}],"add":[{"field":"imp.banner.w","value":300}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.deviceType","mode":"rename","to_field":"devicetype"}]}}`,
		`{"` + idA + `":{"add":[{"field":"imp.ext","postfix_field":"type","value":"pop","formats":["POP"]}]}}`,
		`{"` + idA + `":{"rename":[{"field":"device.deviceType","to_field":"device.ip"}]}}`,
	}
	for i, bad := range invalid {
		if e := s.UpdateJSON([]byte(bad)); e == nil {
			t.Errorf("invalid case %d accepted", i)
		}
		if !s.Has(idA) {
			t.Fatalf("active changed at %d", i)
		}
		onDisk, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(onDisk, good) {
			t.Fatalf("disk changed on rejected update %d", i)
		}
	}
	newValid := []byte(`{"` + idB + `":{"filters":[{"field":"device.os","exists":false}]}}`)
	if e := s.UpdateJSON(newValid); e != nil {
		t.Fatal(e)
	}
	if s.Has(idA) || !s.Has(idB) {
		t.Fatal("update did not atomically swap snapshot")
	}
	got, e := s.Saved()
	if e != nil {
		t.Fatal(e)
	}
	if _, ok := got[idB]; !ok {
		t.Fatal("GET did not read persisted map")
	}
}
func TestReadOnlyAndConcurrentUpdate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "map.json")
	b := []byte(`{"` + idA + `":{}}`)
	_ = os.WriteFile(p, b, 0600)
	s, _ := NewStore(p)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = s.Has(idA)
				_ = s.Has(idB)
				_ = s.Memory()
				if i == 0 {
					_ = s.UpdateJSON(b)
				}
			}
		}(i)
	}
	wg.Wait()
	rules, _ := s.lookup(idA)
	if !reflect.DeepEqual(rules, RuleSet{}) {
		t.Fatal("unexpected rules")
	}
}

func TestPointerOnlyDeletionAndRenameLast(t *testing.T) {
	cfg := `{"` + idA + `":{"add":[{"field":"imp.ext","postfix_field":"type","value":"pop"}],"replace":[{"field":"device.deviceType","mode":"set","value":6}],"delete":[{"field":"imp.banner"}],"rename":[{"field":"device.deviceType","to_field":"devicetype"}]}}`
	path := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(path, []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"device":{"deviceType":4},"imp":[{"id":"a","banner":{"w":300}},{"id":"b"}]}`)
	var req ortb.BidRequest
	if err := jsoniter.Unmarshal(input, &req); err != nil {
		t.Fatal(err)
	}
	raw, err := store.ApplyToRequest(idA, &req)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	dev := m["device"].(map[string]any)
	if dev["devicetype"] != float64(6) {
		t.Fatalf("rename not last %+v", dev)
	}
	if _, old := dev["deviceType"]; old {
		t.Fatalf("old key remains %+v", dev)
	}
	for _, v := range m["imp"].([]any) {
		imp := v.(map[string]any)
		if _, ok := imp["banner"]; ok {
			t.Fatalf("banner remains %+v", imp)
		}
	}
	// Deleting an absent pointer is also a no-op, and the caller is untouched.
	if req.Imp[0].Banner == nil {
		t.Fatal("source pointer mutated")
	}
	if req.Device.GetDeviceType() != 4 {
		t.Fatal("source scalar mutated")
	}
}
func TestDeletePointerStructureValidation(t *testing.T) {
	for _, field := range []string{"imp.banner", "imp.video", "device.geo", "user.geo", "site.publisher", "tmax"} {
		if err := validatePointerPath(field); err != nil {
			t.Errorf("pointer %s: %v", field, err)
		}
	}
	for _, field := range []string{"imp.video.protocols", "device.devicetype", "cur", "imp.banner.btype"} {
		if err := validatePointerPath(field); err == nil {
			t.Errorf("accepted non-pointer %s", field)
		}
	}
}

func TestFilterInferredModesAndIndependentConditions(t *testing.T) {
	conf := []byte(`{"` + idA + `":{"filters":[{"field":"device.os","exists":true},{"field":"device.os","values":["Android"],"isWhiteList":true}]}}`)
	path := filepath.Join(t.TempDir(), "rtb.json")
	if err := os.WriteFile(path, conf, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{`{"device":{"os":"Android"}}`, true},
		{`{"device":{"os":"iOS"}}`, false},
		{`{"device":{}}`, false},
	} {
		ok, _, err := s.Check(idA, []byte(tc.in), "")
		if err != nil || ok != tc.want {
			t.Fatalf("Check(%s) = %v, %v; want %v", tc.in, ok, err, tc.want)
		}
	}
	saved, err := s.Saved()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(saved)
	if err != nil || bytes.Contains(b, []byte(`"type"`)) {
		t.Fatalf("obsolete filter type in persisted representation: %s (%v)", b, err)
	}
	for _, f := range s.Memory()[idA].Filters {
		if f.Exists == nil && len(f.Values) == 0 {
			t.Fatal("filter lost its mode")
		}
	}
}

func TestCountriesWithoutModeAndRenameLeaf(t *testing.T) {
	input := []byte(`{"` + idA + `":{"replace":[{"field":"device.geo.country","countries":{"PH":"PHL"},"copy_to":"user.geo.country"}],"rename":[{"field":"device.deviceType","to_field":"devicetype"}]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(p, input, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	output := runRules(t, s, idA, []byte(`{"device":{"deviceType":4,"geo":{"country":"PH"}}}`))
	var obj map[string]any
	if err := json.Unmarshal(output, &obj); err != nil {
		t.Fatal(err)
	}
	device := obj["device"].(map[string]any)
	if device["devicetype"] != float64(4) || device["deviceType"] != nil {
		t.Fatalf("wrong rename: %v", device)
	}
	if device["geo"].(map[string]any)["country"] != "PHL" {
		t.Fatalf("wrong countries substitution: %v", device)
	}
	if obj["user"].(map[string]any)["geo"].(map[string]any)["country"] != "PHL" {
		t.Fatal("user.geo.country copy missing")
	}
	for _, source := range []any{s.Memory(), mustSaved(t, s)} {
		b, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte(`"mode":"countries"`)) || bytes.Contains(b, []byte(`"to_field":"device.devicetype"`)) {
			t.Fatalf("old syntax leaked into map: %s", b)
		}
	}
	if err := s.UpdateJSON(input); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(`"mode": "countries"`)) {
		t.Fatalf("persisted outdated syntax: %s", b)
	}
	if err := Validate(b); err != nil {
		t.Fatal(err)
	}
}
func mustSaved(t *testing.T, s *Store) map[string]RuleSet {
	t.Helper()
	rules, err := s.Saved()
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

// set changes present protobuf values only; add fills missing ones.
func TestReplaceSetAndAdd(t *testing.T) {
	cfg := []byte(`{"` + idA + `":{"add":[{"field":"tmax","value":500}],"replace":[{"field":"tmax","mode":"set","value":500},{"field":"site.domain","mode":"set","value":"changed.example"}]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(p, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ input string }{{`{"imp":[{"id":"x"}]}`}, {`{"tmax":100,"site":{"domain":"original.example"}}`}} {
		raw := runRules(t, s, idA, []byte(tc.input))
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if m["tmax"] != float64(500) {
			t.Fatalf("tmax not forced: %s", raw)
		}
		if _, hasSite := m["site"]; hasSite && !strings.Contains(tc.input, "domain") {
			t.Fatalf("set created missing site: %s", raw)
		}
	}
	if err := Validate([]byte(`{"` + idA + `":{"replace":[{"field":"tmax","mode":"set_or_add","value":500}]}}`)); err == nil {
		t.Fatal("set_or_add unexpectedly accepted")
	}
}

func TestCopyToOverwritesMissingAndPopulatedFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "map.json")
	cfg := []byte(`{"` + idA + `":{"replace":[{"field":"device.geo.country","countries":{"PH":"PHL"},"copy_to":"user.geo.country"}]}}`)
	if err := os.WriteFile(p, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ source, want string }{
		{`{"device":{"geo":{"country":"PH"}},"user":{"geo":{"country":"US","city":"Paris"}}}`, "PHL"},
		{`{"device":{"geo":{"country":"PH"}}}`, "PHL"},
		// A missing country mapping still copies the final *existing* source.
		{`{"device":{"geo":{"country":"GB"}},"user":{"geo":{"country":"US"}}}`, "GB"},
		{`{"device":{"geo":{"country":""}},"user":{"geo":{"country":"US"}}}`, ""},
	} {
		raw := runRules(t, s, idA, []byte(tc.source))
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		geo := m["user"].(map[string]any)["geo"].(map[string]any)
		if geo["country"] != tc.want {
			t.Errorf("copy_to country for %s: got %v, expected %q", tc.source, geo, tc.want)
		}
		if strings.Contains(tc.source, "Paris") && geo["city"] != "Paris" {
			t.Fatal("copy_to deleted unrelated city")
		}
	}
	raw := runRules(t, s, idA, []byte(`{"device":{},"user":{"geo":{"country":"US"}}}`))
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if m["user"].(map[string]any)["geo"].(map[string]any)["country"] != "US" {
		t.Fatal("missing source must not copy")
	}
}

func TestCopyToCompatibleTypesValidationAndPUT(t *testing.T) {
	good := []byte(`{"` + idA + `":{"replace":[{"field":"device.geo.country","mode":"iso","copy_to":"user.geo.country"}]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(p, good, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	tests := []string{
		`{"` + idA + `":{"replace":[{"field":"device.geo.country","mode":"iso","copy_to":"tmax"}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.geo","mode":"set","value":{},"copy_to":"site.publisher"}]}}`,
		`{"` + idA + `":{"replace":[{"field":"imp.video.w","mode":"set","value":340,"copy_to":"device.w"}]}}`,
		`{"` + idA + `":{"replace":[{"field":"imp.video.w","mode":"set","value":340,"copy_to":"bad.key"}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.geo.country","mode":"iso","copy_to_if_empty":"user.geo.country"}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.geo.country","mode":"iso","copy_to":null}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.geo.country","mode":"iso","copy_to":""}]}}`,
	}
	for _, bad := range tests {
		if err := s.UpdateJSON([]byte(bad)); err == nil {
			t.Errorf("invalid map accepted: %s", bad)
		}
		disk, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(disk, good) {
			t.Fatal("invalid PUT changed disk")
		}
	}
	raw := runRules(t, s, idA, []byte(`{"device":{"geo":{"country":"PH"}},"user":{"geo":{"country":"US"}}}`))
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if m["user"].(map[string]any)["geo"].(map[string]any)["country"] != "PHL" {
		t.Fatal("valid ISO copy_to failed")
	}
	// Scalar same-type paths are accepted across root/device.
	good2 := []byte(`{"` + idA + `":{"replace":[{"field":"device.os","mode":"set","value":"Android","copy_to":"device.model"}]}}`)
	if err := s.UpdateJSON(good2); err != nil {
		t.Fatal(err)
	}
	raw = runRules(t, s, idA, []byte(`{"device":{"os":"iOS","model":"iPhone"}}`))
	_ = json.Unmarshal(raw, &m)
	if m["device"].(map[string]any)["model"] != "Android" {
		t.Fatal("set copy_to failed")
	}
}

func TestCopyToPerImpressionNotCrossing(t *testing.T) {
	cfg := []byte(`{"` + idA + `":{"replace":[{"field":"imp.video.mimes","mode":"set","value":["video/mp4"],"copy_to":"imp.banner.mimes"}]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(p, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	raw := runRules(t, s, idA, []byte(`{"imp":[{"id":"a","video":{"mimes":["video/webm"]},"banner":{"mimes":["image/png"]}},{"id":"b","banner":{"mimes":["image/jpeg"]}}]}`))
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	imps := m["imp"].([]any)
	for i, it := range imps {
		imp := it.(map[string]any)
		if i == 0 {
			if imp["banner"].(map[string]any)["mimes"].([]any)[0] != "video/mp4" {
				t.Fatal("first impression copy missing")
			}
		} else {
			if imp["banner"].(map[string]any)["mimes"].([]any)[0] != "image/jpeg" {
				t.Fatal("cross impression contamination")
			}
		}
	}
}

func TestAddKnownMissingOnlyAndPostfixSharedParents(t *testing.T) {
	cfg := []byte(`{"` + idA + `":{"add":[
		{"field":"tmax","value":500},
		{"field":"imp.ext","postfix_field":"custom.deep.type","value":"pop"},
		{"field":"imp.ext","postfix_field":"custom.deep.name","value":"display"}
	]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(p, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		source string
		want   float64
	}{
		{`{"imp":[{"id":"a"},{"id":"b","ext":{"subid":"s"}}]}`, 500},
		{`{"tmax":0,"imp":[{"id":"a"}]}`, 0},
		{`{"tmax":100,"imp":[{"id":"a"}]}`, 100},
	} {
		out := runRules(t, s, idA, []byte(tc.source))
		var obj map[string]any
		if err := json.Unmarshal(out, &obj); err != nil {
			t.Fatal(err)
		}
		if obj["tmax"] != tc.want {
			t.Fatalf("existing protobuf value was overwritten: %s", out)
		}
		for _, im := range obj["imp"].([]any) {
			imp := im.(map[string]any)
			custom := imp["ext"].(map[string]any)["custom"].(map[string]any)
			deep := custom["deep"].(map[string]any)
			if deep["type"] != "pop" || deep["name"] != "display" {
				t.Fatalf("shared JSON parent lost a child: %s", out)
			}
		}
	}
	// Unknown JSON keys are not put into or read back through protobuf.
	var req ortb.BidRequest
	if err := jsoniter.Unmarshal([]byte(`{"imp":[{"id":"a"}]}`), &req); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyToRequest(idA, &req); err != nil {
		t.Fatal(err)
	}
	if req.GetTmax() != 0 || req.GetImp()[0].GetExt() != nil {
		t.Fatal("caller protobuf modified")
	}
}

func TestAllAndOverlappingGroupsAndCombinedFields(t *testing.T) {
	// JSON groups are retained on disk; expanded runtime has independent rules.
	cfg := []byte(`{"ALL":{"replace":[{"field":"site.domain","mode":"hostname"}]},"` + idA + `,` + idB + `":{"replace":[{"field":"device.geo.country,user.geo.country","countries":{"PH":"PHL"}}],"delete":[{"field":"imp.banner"}]},"` + idB + `":{"replace":[{"field":"tmax","mode":"set","value":300}],"add":[{"field":"site.id,device.os","value":"test"}]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(p, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Has(idC) {
		t.Fatal("ALL not active")
	}
	b := s.Memory()[idB]
	if len(b.Replace) != 4 || len(b.Delete) != 1 || len(b.Add) != 2 {
		t.Fatalf("lost merged rules %+v", b)
	}
	if s.Memory()[idA].Replace[0].Field == "device.geo.country,user.geo.country" {
		t.Fatal("grouped field not expanded")
	}
	if err := s.UpdateJSON(cfg); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(persisted, cfg) {
		t.Fatal("PUT rewrote user grouping")
	}
	out := runRules(t, s, idC, []byte(`{"site":{"page":"https://example.com/abc"}}`))
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["site"].(map[string]any)["domain"] != "example.com" {
		t.Fatalf("ALL not applied to other campaigns: %s", out)
	}
}

func TestGroupedFieldsAndPostfixValidation(t *testing.T) {
	invalid := []string{
		`{"add":[{"field":"site.domain","postfix_field":"x","value":1}]}`,
		`{"add":[{"field":"imp.ext","postfix_field":"subid","value":"x"}]}`,
		`{"add":[{"field":"imp.ext","postfix_field":"nested..field","value":"x"}]}`,
		`{"add":[{"field":"imp.ext","postfix_field":"","value":"x"}]}`,
		`{"add":[{"field":"imp.ext","mode":"field_and_value","value":"x"}]}`,
		`{"replace":[{"field":"device.geo.country,tmax","countries":{"PH":"PHL"}}]}`,
		`{"replace":[{"field":"site.domain,site.page","mode":"hostname"}]}`,
		`{"replace":[{"field":"device.geo.country,user.geo.country","mode":"iso","copy_to":"user.geo.country"}]}`,
		`{"filters":[{"field":"device.geo.country,user.geo.country","exists":true}]}`,
		`{"rename":[{"field":"device.deviceType,device.js","to_field":"devicetype"}]}`,
		`{"delete":[{"field":"imp.banner,imp.video.protocols"}]}`,
		`{"add":[{"field":"device.os,tmax","value":"wrong-type"}]}`,
	}
	for i, body := range invalid {
		cfg := []byte(`{"` + idA + `":` + body + `}`)
		if err := Validate(cfg); err == nil {
			t.Errorf("invalid grouped case %d accepted: %s", i, body)
		}
	}
}

func TestHostnameAndFullPageCrossRecovery(t *testing.T) {
	cfg := []byte(`{"` + idA + `":{"replace":[{"field":"site.domain","mode":"hostname"},{"field":"site.page","mode":"full_page"}]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(p, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ input, domain, page string }{
		{`{"site":{"page":"https://example.org/landing"}}`, "example.org", "https://example.org/landing"},
		{`{"site":{"domain":"example.org"}}`, "example.org", "https://example.org/"},
		{`{"site":{}}`, "", ""},
	} {
		out := runRules(t, s, idA, []byte(tc.input))
		var obj map[string]any
		if err := json.Unmarshal(out, &obj); err != nil {
			t.Fatal(err)
		}
		site := obj["site"].(map[string]any)
		if tc.domain == "" {
			if site["domain"] != nil || site["page"] != nil {
				t.Fatalf("created both missing fields: %s", out)
			}
		} else if site["domain"] != tc.domain || site["page"] != tc.page {
			t.Fatalf("site recovery failed: %s", out)
		}
	}
}

func TestCrossGroupPostfixMergeKeepsSeparateJSONKeys(t *testing.T) {
	cfg := []byte(`{"` + idA + `,` + idB + `":{"add":[{"field":"imp.ext","postfix_field":"custom.name","value":"main"}]},"` + idB + `":{"add":[{"field":"imp.ext","postfix_field":"custom.type","value":"pop"}]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(p, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	b := s.Memory()[idB]
	if len(b.Add) != 2 {
		t.Fatalf("postfix rules with same protobuf parent got lost: %+v", b.Add)
	}
	out := runRules(t, s, idB, []byte(`{"imp":[{"id":"a"}]}`))
	var data map[string]any
	if err := json.Unmarshal(out, &data); err != nil {
		t.Fatal(err)
	}
	custom := data["imp"].([]any)[0].(map[string]any)["ext"].(map[string]any)["custom"].(map[string]any)
	if custom["type"] != "pop" || custom["name"] != "main" {
		t.Fatalf("lost nested fields: %s", out)
	}
	bad := []byte(`{"` + idA + `":{"add":[{"field":"imp.ext","postfix_field":"custom","value":"hello"},{"field":"imp.ext","postfix_field":"custom.type","value":"pop"}]}}`)
	if err := Validate(bad); err == nil {
		t.Fatal("accepted scalar parent collision")
	}
}

func TestPostfixDeleteSiblingAndRenameConflict(t *testing.T) {
	allowed := []byte(`{"` + idA + `":{"add":[{"field":"site","postfix_field":"custom","value":"yes"}],"delete":[{"field":"site.publisher"}]}}`)
	if err := Validate(allowed); err != nil {
		t.Fatalf("separate JSON extension and protobuf sibling deletion should coexist: %v", err)
	}
	conflict := []byte(`{"` + idA + `":{"add":[{"field":"device","postfix_field":"devicetype","value":9}],"rename":[{"field":"device.deviceType","to_field":"devicetype"}]}}`)
	if err := Validate(conflict); err == nil {
		t.Fatal("postfix collides with rename destination")
	}
}
