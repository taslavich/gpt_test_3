package rtbrules

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"google.golang.org/protobuf/proto"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Filter-copy checks use the existing unified test file.
func filterCopyStore(t *testing.T, config string) *Store {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(p, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFilterCopySiteIDToEveryTagID(t *testing.T) {
	s := filterCopyStore(t, `{"`+idA+`":{"filters":[{"field":"site.id","exists":true,"copy_to":"imp.tagid"}]}}`)
	for _, site := range []string{"actual-site-123", "", "   "} {
		t.Run("site="+site, func(t *testing.T) {
			var req ortb.BidRequest
			input := `{"site":{"id":` + fmt.Sprintf("%q", site) + `,"domain":"example.com"},"imp":[{"id":"a"},{"id":"b","tagid":"old-tag"},{"id":"c","tagid":null},{"id":"d","tagid":""}]}`
			if err := jsoniter.Unmarshal([]byte(input), &req); err != nil {
				t.Fatal(err)
			}
			before := proto.Clone(&req)
			cache := NewFilterCache(&req)
			for _, imp := range req.Imp {
				ok, why, err := s.CheckWithCache(idA, cache, imp.GetId())
				if !ok || why != "" || err != nil {
					t.Fatal(ok, why, err)
				}
			}
			// Check never writes copy results into the source or shared cache.
			root, err := cache.rootObject()
			if err != nil {
				t.Fatal(err)
			}
			if _, found := getFieldValue(root["imp"].([]any)[0].(map[string]any), "tagid"); found {
				t.Fatal("filter check mutated cache")
			}
			out, err := s.ApplyToRequest(idA, &req)
			if err != nil {
				t.Fatal(err)
			}
			var got ortb.BidRequest
			if err = jsoniter.Unmarshal(out, &got); err != nil {
				t.Fatal(err)
			}
			if got.Site.GetId() != site || got.Site.GetDomain() != "example.com" || len(got.Imp) != 4 {
				t.Fatal(string(out))
			}
			for _, imp := range got.Imp {
				if imp.Tagid == nil || imp.GetTagid() != site {
					t.Fatal("copy failed", string(out))
				}
			}
			if !proto.Equal(before, &req) {
				t.Fatal("source request mutated")
			}
			other := runRules(t, s, idB, []byte(input))
			var untouched ortb.BidRequest
			if err = jsoniter.Unmarshal(other, &untouched); err != nil || !proto.Equal(before, &untouched) {
				t.Fatal("copy leaked into other campaign", err)
			}
			if _, found := getFieldValue(root["imp"].([]any)[0].(map[string]any), "tagid"); found {
				t.Fatal("output mutated cache")
			}
		})
	}
}

func TestFilterCopyMissingSiteAndOriginalFilterOrder(t *testing.T) {
	base := `{"field":"site.id","exists":true,"copy_to":"imp.tagid"}`
	s := filterCopyStore(t, `{"`+idA+`":{"filters":[`+base+`]}}`)
	for _, raw := range []string{`{"imp":[{"id":"a"}]}`, `{"site":null,"imp":[{"id":"a"}]}`, `{"site":{},"imp":[{"id":"a"}]}`, `{"site":{"id":null},"imp":[{"id":"a"}]}`} {
		ok, why, err := s.Check(idA, []byte(raw), "a")
		if ok || why != "rtb_rule_filter_site.id" || err != nil {
			t.Fatal(raw, ok, why, err)
		}
	}
	for _, filters := range []string{base + `,{"field":"imp.tagid","exists":true}`, `{"field":"imp.tagid","exists":true},` + base} {
		s := filterCopyStore(t, `{"`+idA+`":{"filters":[`+filters+`]}}`)
		raw := []byte(`{"site":{"id":"real"},"imp":[{"id":"a"}]}`)
		ok, why, err := s.Check(idA, raw, "a")
		if ok || why != "rtb_rule_filter_imp.tagid" || err != nil {
			t.Fatal("copy must not satisfy another inbound filter", ok, why, err)
		}
	}
}

func TestFilterCopyModesAndOriginalSource(t *testing.T) {
	for _, condition := range []string{`"exists":true`, `"isWhiteList":true,"values":["original"]`, `"isWhiteList":false,"values":["blocked"]`} {
		s := filterCopyStore(t, `{"`+idA+`":{"filters":[{"field":"site.id",`+condition+`,"copy_to":"imp.tagid"}],"replace":[{"field":"site.id","mode":"set","value":"changed"}]}}`)
		raw := []byte(`{"site":{"id":"original"},"imp":[{"id":"a"}]}`)
		ok, why, err := s.Check(idA, raw, "a")
		if !ok || why != "" || err != nil {
			t.Fatal(ok, why, err)
		}
		var got ortb.BidRequest
		if err = jsoniter.Unmarshal(runRules(t, s, idA, raw), &got); err != nil {
			t.Fatal(err)
		}
		if got.Site.GetId() != "changed" || got.Imp[0].GetTagid() != "original" {
			t.Fatal("copy must precede replace", &got)
		}
	}
	s := filterCopyStore(t, `{"`+idA+`":{"filters":[{"field":"imp.secure","compare":">=","value":1,"copy_to":"imp.instl"}]}}`)
	raw := []byte(`{"imp":[{"id":"a","secure":1,"instl":0},{"id":"b","secure":2}]}`)
	if ok, _, err := s.Check(idA, raw, "a"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	var got ortb.BidRequest
	if err := jsoniter.Unmarshal(runRules(t, s, idA, raw), &got); err != nil {
		t.Fatal(err)
	}
	if got.Imp[0].GetInstl() != 1 || got.Imp[1].GetInstl() != 2 {
		t.Fatal("impression copy mixed values")
	}
	absent := filterCopyStore(t, `{"`+idA+`":{"filters":[{"field":"site.id","exists":false,"copy_to":"imp.tagid"}]}}`)
	missing := []byte(`{"site":{},"imp":[{"id":"a","tagid":"keep"}]}`)
	if ok, _, err := absent.Check(idA, missing, "a"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if out := runRules(t, absent, idA, missing); !bytes.Contains(out, []byte(`"tagid":"keep"`)) {
		t.Fatal(string(out))
	}
}

func TestFilterCopyImpressionIsolationAndNoCascade(t *testing.T) {
	s := filterCopyStore(t, `{"`+idA+`":{"filters":[{"field":"site.id","exists":true,"copy_to":"imp.tagid"},{"field":"imp.tagid","exists":true,"copy_to":"imp.ext.subid"}]}}`)
	raw := []byte(`{"site":{"id":"site"},"imp":[{"id":"a","tagid":"tag-a"},{"id":"b","tagid":"tag-b"}]}`)
	for _, id := range []string{"a", "b"} {
		if ok, _, err := s.Check(idA, raw, id); !ok || err != nil {
			t.Fatal(ok, err)
		}
	}
	var got ortb.BidRequest
	if err := jsoniter.Unmarshal(runRules(t, s, idA, raw), &got); err != nil {
		t.Fatal(err)
	}
	for i, tag := range []string{"tag-a", "tag-b"} {
		if got.Imp[i].GetTagid() != "site" || got.Imp[i].GetExt().GetSubid() != tag {
			t.Fatal("copy fed another copy or mixed impressions", &got)
		}
	}
	// Missing source never creates an impression, a site, or a fake tagid.
	for _, raw := range []string{`{}`, `{"imp":[]}`, `{"imp":[null,{},{}]}`} {
		out := runRules(t, s, idA, []byte(raw))
		if bytes.Contains(out, []byte(`"tagid"`)) || bytes.Contains(out, []byte(`"site"`)) {
			t.Fatal(string(out))
		}
	}
	// JSON-only application also tolerates a malformed impression sibling.
	m := map[string]any{"imp": []any{nil, "bad", map[string]any{"id": "a"}}}
	applyFilterCopies(m, []Filter{{Field: "imp.id", CopyTo: "imp.tagid"}})
	if m["imp"].([]any)[2].(map[string]any)["tagid"] != "a" {
		t.Fatal(m)
	}
}

func TestFilterCopyCompoundValuesAreIndependent(t *testing.T) {
	root := map[string]any{"site": map[string]any{"cat": []any{"IAB1"}}, "device": map[string]any{"geo": map[string]any{"country": "PH"}}, "imp": []any{map[string]any{"id": "a"}}}
	applyFilterCopies(root, []Filter{{Field: "site.cat", CopyTo: "bcat"}, {Field: "device.geo", CopyTo: "user.geo"}})
	root["site"].(map[string]any)["cat"].([]any)[0] = "changed"
	root["device"].(map[string]any)["geo"].(map[string]any)["country"] = "US"
	if root["bcat"].([]any)[0] != "IAB1" || root["user"].(map[string]any)["geo"].(map[string]any)["country"] != "PH" {
		t.Fatal("copy shared mutable source data")
	}
}

func TestFilterCopyConfigurationValidation(t *testing.T) {
	for _, rule := range []string{
		`{"field":"site.id","exists":true,"copy_to":null}`,
		`{"field":"site.id","exists":true,"copy_to":""}`,
		`{"field":"site.id","exists":true,"copy_to":1}`,
		`{"field":"site.id","exists":true,"copy_to":"imp.no_such_field"}`,
		`{"field":"site.id","exists":true,"copy_to":"imp.secure"}`,
		`{"field":"site.id","exists":true,"copy_to":"imp.ext.type"}`,
		`{"field":"site.id","exists":true,"copy_to":"device.devicetype"}`,
		`{"field":"site.id","exists":true,"copy_to":" imp.tagid"}`,
		`{"field":"imp.id","exists":true,"copy_to":"site.id"}`,
		`{"field":"site.id,site.domain","exists":true,"copy_to":"imp.tagid"}`,
		`{"field":"device.geo","exists":true,"copy_to":"site.publisher"}`,
		`{"field":"imp.native.request","json_path":"native.ver","exists":true,"copy_to":"imp.tagid"}`,
	} {
		if err := Validate([]byte(`{"ALL":{"filters":[` + rule + `]}}`)); err == nil {
			t.Fatal("invalid filter accepted", rule)
		}
	}
	filter := `"filters":[{"field":"site.id","exists":true,"copy_to":"imp.tagid"}]`
	for _, rest := range []string{`"add":[{"field":"site.id","value":"invented"}]`, `"add":[{"field":"site","value":{"id":"invented"}}]`, `"replace":[{"field":"site.id","mode":"set_or_add","value":"invented"}]`, `"delete":[{"field":"site"}]`, `"delete":[{"field":"site.id"}]`, `"delete":[{"field":"imp.tagid"}]`} {
		if err := Validate([]byte(`{"ALL":{` + filter + `,` + rest + `}}`)); err == nil {
			t.Fatal("unsafe overlapping source/target accepted", rest)
		}
	}
	if err := Validate([]byte(`{"ALL":{"filters":[{"field":"imp.ext","exists":true,"copy_to":"imp.ext"}],"add":[{"field":"imp.ext","postfix_field":"type","value":"pop"}]}}`)); err == nil {
		t.Fatal("postfix modifying copy source accepted")
	}
	// Conflicts must also be rejected after ALL + campaign merging.
	if err := Validate([]byte(`{"ALL":{` + filter + `},"` + idA + `":{"delete":[{"field":"imp.tagid"}]}}`)); err == nil {
		t.Fatal("merged conflict accepted")
	}
	for _, rule := range []string{`{"field":"site.cat","exists":true,"copy_to":"bcat"}`, `{"field":"device.geo","exists":true,"copy_to":"user.geo"}`} {
		if err := Validate([]byte(`{"ALL":{"filters":[` + rule + `]}}`)); err != nil {
			t.Fatal("compatible copy rejected", err)
		}
	}
}

func TestFilterCopyALLOverridePersistenceAndReload(t *testing.T) {
	config := "{\n  \"ALL\": {\"filters\":[{\"field\":\"site.id\",\"exists\":true,\"copy_to\":\"imp.tagid\"}]},\n  \"" + idB + "\": {\"filters\":[{\"field\":\"site.id\",\"exists\":true}]}\n}\n"
	s := filterCopyStore(t, `{}`)
	if err := s.UpdateJSON([]byte(config)); err != nil {
		t.Fatal(err)
	}
	if saved, err := s.SavedJSON(); err != nil || string(saved) != config {
		t.Fatal("authored JSON changed", err)
	}
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	if saved, err := s.Saved(); err != nil || saved["ALL"].Filters[0].CopyTo != "imp.tagid" {
		t.Fatal("copy_to not persisted", err)
	}
	if s.Memory()["ALL"].Filters[0].CopyTo != "imp.tagid" {
		t.Fatal("copy_to missing from debug memory")
	}
	for _, id := range []string{idA, idB, idC} {
		out := runRules(t, s, id, []byte(`{"site":{"id":"site"},"imp":[{"id":"a","tagid":"old"}]}`))
		var got ortb.BidRequest
		if err := jsoniter.Unmarshal(out, &got); err != nil {
			t.Fatal(err)
		}
		want := "site"
		if id == idB {
			want = "old"
		}
		if got.Imp[0].GetTagid() != want {
			t.Fatal(id, string(out))
		}
	}
	if err := s.UpdateJSON([]byte(`{"ALL":{"filters":[{"field":"site.id","exists":true,"copy_to":"bad"}]}}`)); err == nil {
		t.Fatal("bad update accepted")
	}
	if saved, err := s.SavedJSON(); err != nil || string(saved) != config {
		t.Fatal("bad update changed persisted file", err)
	}
}

// Tests retained from store_test.go.
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
}

// set_or_add restores the existing add-when-missing/replace-when-present
// behavior without changing the strict behavior of set or ordinary add.
func TestReplaceSetOrAddUsesProtobufAddSemantics(t *testing.T) {
	p := filepath.Join(t.TempDir(), "map.json")
	cfg := []byte(`{"` + idA + `":{"replace":[{"field":"tmax","mode":"set_or_add","value":300},{"field":"imp.bidfloor","mode":"set_or_add","value":0},{"field":"imp.secure","mode":"set_or_add","value":1},{"field":"site.domain","mode":"set_or_add","value":"example.com"}]}}`)
	if err := os.WriteFile(p, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, input string
	}{
		{"missing", `{"imp":[{"id":"a"},{"id":"b","secure":0}],"site":{"page":"/x"}}`},
		{"existing", `{"tmax":500,"imp":[{"id":"a","bidfloor":0.5,"secure":0},{"id":"b","bidfloor":3,"secure":1}],"site":{"domain":"old.example"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req ortb.BidRequest
			if err := jsoniter.Unmarshal([]byte(tc.input), &req); err != nil {
				t.Fatal(err)
			}
			before, err := jsoniter.Marshal(&req)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := s.ApplyToRequest(idA, &req)
			if err != nil {
				t.Fatal(err)
			}
			after, err := jsoniter.Marshal(&req)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("original BidRequest mutated: before=%s after=%s", before, after)
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			if m["tmax"] != float64(300) || m["site"].(map[string]any)["domain"] != "example.com" {
				t.Fatalf("set_or_add failed root field: %s", raw)
			}
			imps := m["imp"].([]any)
			for _, rawImp := range imps {
				imp := rawImp.(map[string]any)
				if imp["bidfloor"] != float64(0) || imp["secure"] != float64(1) {
					t.Fatalf("set_or_add failed impression: %s", raw)
				}
			}
		})
	}
}

// Grouped fields and ALL use the same validated per-field protobuf behavior.
func TestReplaceSetOrAddALLAndGroupedFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "map.json")
	cfg := []byte(`{"ALL":{"replace":[{"field":"imp.secure,imp.instl","mode":"set_or_add","value":1}]}}`)
	if err := os.WriteFile(p, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	raw := runRules(t, s, idA, []byte(`{"imp":[{"id":"a","secure":0},{"id":"b","instl":0}]}`))
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, item := range m["imp"].([]any) {
		imp := item.(map[string]any)
		if imp["secure"] != float64(1) || imp["instl"] != float64(1) {
			t.Fatalf("ALL/grouped set_or_add did not apply: %s", raw)
		}
	}
}

func TestReplaceSetOrAddValidation(t *testing.T) {
	for _, replacement := range []string{
		`{"field":"tmax","mode":"set_or_add"}`,
		`{"field":"tmax","mode":"set_or_add","value":"300"}`,
		`{"field":"imp.ext.type","mode":"set_or_add","value":"pop"}`,
		`{"field":"tmax","mode":"set_or_add","value":300,"postfix_field":"x"}`,
		`{"field":"device.geo.country","mode":"set_or_add","countries":{"PH":"PHL"},"value":"PHL"}`,
	} {
		cfg := []byte(`{"` + idA + `":{"replace":[` + replacement + `]}}`)
		if err := Validate(cfg); err == nil {
			t.Fatalf("unexpectedly accepted invalid set_or_add: %s", replacement)
		}
	}
	if err := Validate([]byte(`{"` + idA + `":{"replace":[{"field":"tmax","mode":"set_or_add","value":300}]}}`)); err != nil {
		t.Fatalf("valid set_or_add rejected: %v", err)
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

func TestNativeRequestJSONPathAndCachedChecks(t *testing.T) {
	config := []byte(`{"` + idA + `,` + idB + `":{"filters":[` +
		`{"field":"imp.native.request","json_path":"assets[id=100].title.len","values":[80],"isWhiteList":true},` +
		`{"field":"imp.native.request","json_path":"assets[id=101].data.type","values":[1],"isWhiteList":true},` +
		`{"field":"imp.native.request","json_path":"assets[id=102].img.type","values":[3],"isWhiteList":true},` +
		`{"field":"imp.native.request","json_path":"assets[id=102].img.wmin","compare":">=","value":200},` +
		`{"field":"imp.native.request","json_path":"assets[id=102].img.hmin","compare":">=","value":200}]}}`)
	path := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(path, config, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	native1 := `{"assets":[{"id":102,"img":{"type":3,"wmin":240,"hmin":210}},{"id":100,"title":{"len":80}},{"id":101,"data":{"type":1,"len":80}}]}`
	native2 := `{"assets":[{"id":102,"img":{"type":3,"wmin":190,"hmin":210}},{"id":100,"title":{"len":80}},{"id":101,"data":{"type":1,"len":80}}]}`
	request := map[string]any{"imp": []any{
		map[string]any{"id": "imp-good", "native": map[string]any{"request": native1}},
		map[string]any{"id": "imp-small", "native": map[string]any{"request": native2}},
	}}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	cache := newFilterCacheFromJSON(raw)
	for _, campaign := range []string{idA, idB} {
		ok, why, err := s.CheckWithCache(campaign, cache, "imp-good")
		if err != nil || !ok {
			t.Fatalf("campaign %s expected good: %s %v", campaign, why, err)
		}
	}
	if len(cache.native) != 1 {
		t.Fatalf("native parse cache should contain one impression; got %d", len(cache.native))
	}
	ok, why, err := s.CheckWithCache(idA, cache, "imp-small")
	if err != nil || ok || !strings.Contains(why, "wmin") {
		t.Fatalf("size filter %v %s %v", ok, why, err)
	}
	if len(cache.native) != 2 {
		t.Fatalf("native cache isolation by impression: %d", len(cache.native))
	}
}

func TestNativeRequestParsingAndTypeErrorsAreSeparateFromMismatch(t *testing.T) {
	cfg := []byte(`{"` + idA + `":{"filters":[{"field":"imp.native.request","json_path":"assets[id=102].img.wmin","compare":">=","value":200}]}}`)
	path := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(path, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, native        string
		wantError, wantPass bool
	}{
		{"bad JSON", `{bad`, true, false},
		{"trailing JSON", `{"assets":[]} null`, true, false},
		{"wrong type", `{"assets":[{"id":102,"img":{"wmin":"240"}}]}`, true, false},
		{"wrong assets type", `{"assets":"oops"}`, true, false},
		{"wrong asset id type", `{"assets":[{"id":"102","img":{"wmin":240}}]}`, true, false},
		{"missing asset", `{"assets":[{"id":99}]}`, false, false},
		{"valid but below limit", `{"assets":[{"id":102,"img":{"wmin":199}}]}`, false, false},
		{"valid and matches", `{"assets":[{"id":102,"img":{"wmin":200}}]}`, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"imp": []any{map[string]any{"id": "i", "native": map[string]any{"request": tc.native}}}})
			ok, _, err := s.Check(idA, raw, "i")
			if ok != tc.wantPass || errors.Is(err, ErrNativeFilter) != tc.wantError {
				t.Fatalf("passed=%t error=%v expected pass=%t error=%t", ok, err, tc.wantPass, tc.wantError)
			}
		})
	}
	raw := []byte(`{"imp":[{"id":"i"}]}`)
	ok, _, err := s.Check(idA, raw, "i")
	if err != nil || ok {
		t.Fatalf("missing native request must just fail filter: ok=%t err=%v", ok, err)
	}
}

func TestNativeRequestArrayIndexAndExists(t *testing.T) {
	cfg := []byte(`{"` + idA + `":{"filters":[{"field":"imp.native.request","json_path":"assets[0].title.len","exists":true}]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	_ = os.WriteFile(p, cfg, 0600)
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"imp":[{"id":"a","native":{"request":"{\"assets\":[{\"title\":{\"len\":80}}]}"}}]}`)
	ok, _, err := s.Check(idA, raw, "a")
	if err != nil || !ok {
		t.Fatalf("expected indexed field: %v %v", ok, err)
	}
	raw = []byte(`{"imp":[{"id":"a","native":{"request":"{\"assets\":[]}"}}]}`)
	ok, _, err = s.Check(idA, raw, "a")
	if err != nil || ok {
		t.Fatalf("missing index is mismatch, not error: %v %v", ok, err)
	}
}

func TestNativeRequestConfigValidationAndPUTRollback(t *testing.T) {
	good := []byte(`{"` + idA + `":{"filters":[{"field":"imp.native.request","json_path":"assets[id=102].img.wmin","compare":">=","value":200}]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(p, good, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`{"field":"imp.native.request","json_path":"assets[].img.wmin","compare":">=","value":200}`,
		`{"field":"imp.native.ver","json_path":"assets[0]","exists":true}`,
		`{"field":"imp.native.request","json_path":"assets[id=abc]","exists":true}`,
		`{"field":"imp.native.request","json_path":"assets[id=102].img.wmin","compare":"between","value":200}`,
		`{"field":"imp.native.request","json_path":"assets[id=102].img.wmin","compare":">=","value":"200"}`,
		`{"field":"imp.native.request","json_path":"assets[id=102].img.wmin","values":[200],"compare":">=","value":200,"isWhiteList":true}`,
		`{"field":"imp.native.request","json_path":"assets[id=102].img.wmin","values":[200,"200"],"isWhiteList":true}`,
		`{"field":"imp.native.request","json_path":"assets[id=102].img.wmin","compare":">=","value":200,"isWhiteList":true}`,
	} {
		invalid := []byte(`{"` + idA + `":{"filters":[` + bad + `]}}`)
		if err := s.UpdateJSON(invalid); err == nil {
			t.Fatalf("accepted invalid rule: %s", bad)
		}
		persisted, _ := os.ReadFile(p)
		if !bytes.Equal(persisted, good) {
			t.Fatalf("invalid PUT modified file")
		}
	}
	if len(s.Memory()[idA].Filters) != 1 {
		t.Fatal("invalid PUT changed active rules")
	}
}

func TestNativeJSONPathALLAndCampaignSpecificMerge(t *testing.T) {
	cfg := []byte(`{"ALL":{"filters":[` +
		`{"field":"imp.native.request","json_path":"assets[id=100].title.len","values":[80],"isWhiteList":true}]},` +
		`"` + idA + `":{"filters":[` +
		`{"field":"imp.native.request","json_path":"assets[id=102].img.wmin","compare":">=","value":200}]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	_ = os.WriteFile(p, cfg, 0600)
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	merged, exists := s.lookup(idA)
	if !exists || len(merged.Filters) != 2 {
		t.Fatalf("global+campaign JSON paths must be independent: %+v", merged.Filters)
	}
	global, exists := s.lookup(idB)
	if !exists || len(global.Filters) != 1 {
		t.Fatalf("ALL missing: %+v", global.Filters)
	}
}

func TestNativeFilterMalformedImpressionDoesNotPoisonOtherImpressions(t *testing.T) {
	cfg := []byte(`{"` + idA + `":{"filters":[{"field":"imp.native.request","json_path":"assets[0].title.len","values":[80],"isWhiteList":true}]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	_ = os.WriteFile(p, cfg, 0600)
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"imp": []any{
		map[string]any{"id": "bad", "native": map[string]any{"request": "{bad"}},
		map[string]any{"id": "good", "native": map[string]any{"request": `{"assets":[{"title":{"len":80}}]}`}},
	}})
	cache := newFilterCacheFromJSON(payload)
	if ok, _, err := s.CheckWithCache(idA, cache, "bad"); ok || !errors.Is(err, ErrNativeFilter) {
		t.Fatalf("bad impression must fail with native error: ok=%v err=%v", ok, err)
	}
	if ok, _, err := s.CheckWithCache(idA, cache, "good"); !ok || err != nil {
		t.Fatalf("good impression must pass despite sibling error: ok=%v err=%v", ok, err)
	}
}

// Tests retained from main_test.go.
const (
	qaCN  = "8cf66405-6013-423c-a967-a76c08c69ac4"
	qaPH  = "9e4173f7-7aee-44a6-8039-db364cb6f4f5"
	qaMX  = "1c9eeda4-3cb1-499d-aada-83c3b07e042f"
	qaVID = "11111111-1111-4111-8111-111111111111"
	qaIPP = "33333333-3333-4333-8333-333333333333"
)

func qaStore(t *testing.T, cfg map[string]any) *Store {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(raw); err != nil {
		t.Fatalf("invalid test rules: %v\n%s", err, raw)
	}
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func qaClone(t *testing.T, src map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatal(err)
	}
	var dst map[string]any
	if err := json.Unmarshal(raw, &dst); err != nil {
		t.Fatal(err)
	}
	return dst
}

func qaCheck(
	t *testing.T,
	s *Store,
	id string,
	req map[string]any,
	impID string,
	want bool,
) {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	ok, reason, err := s.CheckWithCache(
		id, newFilterCacheFromJSON(raw), impID,
	)
	if err != nil {
		t.Fatalf("unexpected filter error: %v", err)
	}
	if ok != want {
		t.Fatalf(
			"campaign=%s imp=%s accepted=%v want=%v reason=%s\n%s",
			id, impID, ok, want, reason, raw,
		)
	}
	if !ok && reason == "" {
		t.Fatal("rejected without reason")
	}
}

func qaApply(
	t *testing.T, s *Store, id string, req map[string]any,
) map[string]any {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var bid ortb.BidRequest
	if err := jsoniter.Unmarshal(raw, &bid); err != nil {
		t.Fatal(err)
	}
	before, _ := jsoniter.Marshal(&bid)

	result, err := s.ApplyToRequest(id, &bid)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := jsoniter.Marshal(&bid)
	if !bytes.Equal(before, after) {
		t.Fatalf("original request mutated:\n%s\n%s", before, after)
	}
	var out map[string]any
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func qaWhite(field string, value any) map[string]any {
	return map[string]any{
		"field":       field,
		"values":      []any{value},
		"isWhiteList": true,
	}
}

func qaNative(path string, value any) map[string]any {
	rule := qaWhite("imp.native.request", value)
	rule["json_path"] = path
	return rule
}

func qaVideoRules() []any {
	return []any{
		qaWhite("at", 1),
		qaWhite("cur", "USD"),
		map[string]any{"field": "imp.tagid", "exists": true},
		qaWhite("imp.instl", 0),
		qaWhite("imp.secure", 1),
		qaWhite("imp.bidfloorcur", "USD"),
		qaWhite("imp.video.mimes", "video/mp4"),
		qaWhite("imp.video.protocols", 3),
		qaWhite("imp.video.w", 340),
		qaWhite("imp.video.h", 192),
		qaWhite("imp.video.linearity", 1),
		qaWhite("imp.video.sequence", 1),
		qaWhite("imp.video.boxingallowed", 1),
		qaWhite("imp.video.skipmin", 0),
		qaWhite("imp.video.skipafter", 0),
	}
}

func qaVideoRequest() map[string]any {
	return map[string]any{
		"at":  1,
		"cur": []any{"USD"},
		"imp": []any{map[string]any{
			"id":          "v1",
			"tagid":       "slot",
			"instl":       0,
			"secure":      1,
			"bidfloorcur": "USD",
			"video": map[string]any{
				"mimes":         []any{"video/mp4"},
				"protocols":     []any{3},
				"w":             340,
				"h":             192,
				"linearity":     1,
				"sequence":      1,
				"boxingallowed": 1,
				"skipmin":       0,
				"skipafter":     0,
			},
		}},
	}
}

func TestQAChinaVideoFullContract(t *testing.T) {
	s := qaStore(t, map[string]any{
		qaVID: map[string]any{
			"filters": qaVideoRules(),
			"replace": []any{
				map[string]any{
					"field": "tmax",
					"mode":  "set_or_add",
					"value": 550,
				},
			},
		},
	})

	base := qaVideoRequest()
	qaCheck(t, s, qaVID, base, "v1", true)

	imp := func(req map[string]any) map[string]any {
		return req["imp"].([]any)[0].(map[string]any)
	}

	tests := []struct {
		name   string
		change func(map[string]any)
	}{
		{"wrong auction", func(r map[string]any) {
			r["at"] = 2
		}},
		{"wrong currency", func(r map[string]any) {
			r["cur"] = []any{"EUR"}
		}},
		{"missing tagid", func(r map[string]any) {
			delete(imp(r), "tagid")
		}},
		{"wrong instl", func(r map[string]any) {
			imp(r)["instl"] = 1
		}},
		{"wrong secure", func(r map[string]any) {
			imp(r)["secure"] = 0
		}},
		{"wrong bidfloorcur", func(r map[string]any) {
			imp(r)["bidfloorcur"] = "EUR"
		}},
		{"wrong mime", func(r map[string]any) {
			imp(r)["video"].(map[string]any)["mimes"] =
				[]any{"video/webm"}
		}},
		{"wrong protocol", func(r map[string]any) {
			imp(r)["video"].(map[string]any)["protocols"] =
				[]any{2}
		}},
	}

	videoFields := []struct {
		field string
		bad   any
	}{
		{"w", 320},
		{"h", 200},
		{"linearity", 2},
		{"sequence", 2},
		{"boxingallowed", 0},
		{"skipmin", 1},
		{"skipafter", 1},
	}
	for _, f := range videoFields {
		field, bad := f.field, f.bad
		tests = append(tests, struct {
			name   string
			change func(map[string]any)
		}{
			name: "video " + field,
			change: func(r map[string]any) {
				imp(r)["video"].(map[string]any)[field] = bad
			},
		})
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := qaClone(t, base)
			tc.change(req)
			qaCheck(t, s, qaVID, req, "v1", false)
		})
	}

	t.Run("arrays containing supported values", func(t *testing.T) {
		req := qaClone(t, base)
		req["cur"] = []any{"EUR", "USD"}
		video := imp(req)["video"].(map[string]any)
		video["mimes"] = []any{"video/webm", "video/mp4"}
		video["protocols"] = []any{2, 3}
		qaCheck(t, s, qaVID, req, "v1", true)
	})

	for _, current := range []any{nil, 100, 700} {
		req := qaClone(t, base)
		if current != nil {
			req["tmax"] = current
		}
		out := qaApply(t, s, qaVID, req)
		if out["tmax"] != float64(550) {
			t.Fatalf("tmax=%v with input=%v", out["tmax"], current)
		}
	}
}

func qaNativeDocument() map[string]any {
	return map[string]any{
		"native": map[string]any{
			"ver": "1.2",
			"assets": []any{
				map[string]any{
					"id": 102, "required": 1,
					"img": map[string]any{
						"type": 3,
						"w":    200, "h": 200,
						"wmin": 200, "hmin": 200,
					},
				},
				map[string]any{
					"id": 100, "required": 1,
					"title": map[string]any{"len": 80},
				},
				map[string]any{
					"id": 101, "required": 1,
					"data": map[string]any{
						"type": 1, "len": 80,
					},
				},
			},
		},
	}
}

func qaIPPRequest(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"at":  1,
		"cur": []any{"USD"},
		"imp": []any{map[string]any{
			"id":          "n1",
			"tagid":       "slot",
			"instl":       0,
			"secure":      1,
			"bidfloorcur": "USD",
			"native": map[string]any{
				"ver":     "1.2",
				"request": string(raw),
			},
		}},
	}
}

func qaIPPRules() []any {
	rules := []any{
		qaWhite("at", 1),
		qaWhite("cur", "USD"),
		map[string]any{"field": "imp.tagid", "exists": true},
		qaWhite("imp.instl", 0),
		qaWhite("imp.secure", 1),
		qaWhite("imp.bidfloorcur", "USD"),
		qaWhite("imp.native.ver", "1.2"),
		map[string]any{
			"field":  "imp.native.request",
			"exists": true,
		},
	}
	for _, item := range []struct {
		path  string
		value any
	}{
		{"native.ver", "1.2"},
		{"native.assets[id=100].required", 1},
		{"native.assets[id=100].title.len", 80},
		{"native.assets[id=101].required", 1},
		{"native.assets[id=101].data.type", 1},
		{"native.assets[id=101].data.len", 80},
		{"native.assets[id=102].required", 1},
		{"native.assets[id=102].img.type", 3},
		{"native.assets[id=102].img.w", 200},
		{"native.assets[id=102].img.h", 200},
		{"native.assets[id=102].img.wmin", 200},
		{"native.assets[id=102].img.hmin", 200},
	} {
		rules = append(rules, qaNative(item.path, item.value))
	}
	return rules
}

func qaChangeNativeAsset(
	t *testing.T, req map[string]any,
	id int, section, field string, value any,
) {
	t.Helper()
	imp := req["imp"].([]any)[0].(map[string]any)
	native := imp["native"].(map[string]any)
	var doc map[string]any
	if err := json.Unmarshal(
		[]byte(native["request"].(string)), &doc,
	); err != nil {
		t.Fatal(err)
	}
	assets := doc["native"].(map[string]any)["assets"].([]any)
	found := false
	for _, raw := range assets {
		asset := raw.(map[string]any)
		if int(asset["id"].(float64)) != id {
			continue
		}
		target := asset
		if section != "" {
			target = asset[section].(map[string]any)
		}
		if value == nil {
			delete(target, field)
		} else {
			target[field] = value
		}
		found = true
	}
	if !found {
		t.Fatalf("asset %d not found", id)
	}
	data, _ := json.Marshal(doc)
	native["request"] = string(data)
}

func TestQAChinaIPPAllAssetsAndExactSizes(t *testing.T) {
	s := qaStore(t, map[string]any{
		qaIPP: map[string]any{
			"filters": qaIPPRules(),
			"replace": []any{
				map[string]any{
					"field": "tmax",
					"mode":  "set_or_add",
					"value": 300,
				},
			},
		},
	})

	base := qaIPPRequest(t, qaNativeDocument())
	qaCheck(t, s, qaIPP, base, "n1", true)

	tests := []struct {
		name    string
		id      int
		section string
		field   string
		bad     any
	}{
		{"title required", 100, "", "required", 0},
		{"title length", 100, "title", "len", 79},
		{"data required", 101, "", "required", 0},
		{"data type", 101, "data", "type", 2},
		{"data length", 101, "data", "len", 81},
		{"image required", 102, "", "required", 0},
		{"image type", 102, "img", "type", 1},
		{"image width 199", 102, "img", "w", 199},
		{"image width 201", 102, "img", "w", 201},
		{"image height 199", 102, "img", "h", 199},
		{"image height 201", 102, "img", "h", 201},
		{"image wmin 199", 102, "img", "wmin", 199},
		{"image wmin 201", 102, "img", "wmin", 201},
		{"image hmin 199", 102, "img", "hmin", 199},
		{"image hmin 201", 102, "img", "hmin", 201},
		{"missing wmin", 102, "img", "wmin", nil},
		{"missing hmin", 102, "img", "hmin", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := qaClone(t, base)
			qaChangeNativeAsset(
				t, req, tc.id, tc.section, tc.field, tc.bad,
			)
			qaCheck(t, s, qaIPP, req, "n1", false)
		})
	}

	t.Run("unwrapped JSON rejected", func(t *testing.T) {
		req := qaIPPRequest(
			t, qaNativeDocument()["native"].(map[string]any),
		)
		qaCheck(t, s, qaIPP, req, "n1", false)
	})

	t.Run("wrong native version", func(t *testing.T) {
		req := qaClone(t, base)
		req["imp"].([]any)[0].(map[string]any)["native"].(map[string]any)["ver"] = "1.1"
		qaCheck(t, s, qaIPP, req, "n1", false)
	})

	t.Run("malformed native JSON is error", func(t *testing.T) {
		req := qaClone(t, base)
		req["imp"].([]any)[0].(map[string]any)["native"].(map[string]any)["request"] = "{bad"
		raw, _ := json.Marshal(req)
		ok, _, err := s.CheckWithCache(
			qaIPP, newFilterCacheFromJSON(raw), "n1",
		)
		if ok || !errors.Is(err, ErrNativeFilter) {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})

	for _, current := range []any{nil, 50, 900} {
		req := qaClone(t, base)
		if current != nil {
			req["tmax"] = current
		}
		out := qaApply(t, s, qaIPP, req)
		if out["tmax"] != float64(300) {
			t.Fatalf("IPP tmax=%v", out["tmax"])
		}
	}
}

func qaPOPConfig() map[string]any {
	all := qaCN + "," + qaPH + "," + qaMX
	cfg := map[string]any{
		all: map[string]any{
			"filters": []any{
				map[string]any{
					"field": "imp.tagid", "exists": true,
				},
				qaWhite("imp.secure", 1),
			},
			"add": []any{
				map[string]any{
					"field": "imp.bidfloor", "value": 0,
				},
				map[string]any{
					"field":         "imp.ext",
					"postfix_field": "type", "value": "pop",
				},
			},
			"replace": []any{
				map[string]any{
					"field": "tmax",
					"mode":  "set_or_add", "value": 500,
				},
				map[string]any{
					"field": "site.domain", "mode": "hostname",
				},
				map[string]any{
					"field": "site.page", "mode": "full_page",
				},
			},
			"delete": []any{
				map[string]any{"field": "imp.banner"},
			},
			"rename": []any{
				map[string]any{
					"field":    "device.deviceType",
					"to_field": "devicetype",
				},
			},
		},
	}
	for id, countries := range map[string]map[string]string{
		qaCN: {"CN": "CHN"},
		qaPH: {"PH": "PHL"},
		qaMX: {"MX": "MEX", "PE": "PER"},
	} {
		cfg[id] = map[string]any{
			"replace": []any{
				map[string]any{
					"field":     "device.geo.country",
					"countries": countries,
					"copy_to":   "user.geo.country",
				},
				map[string]any{
					"field":     "user.geo.country",
					"countries": countries,
				},
			},
		}
	}
	return cfg
}

func TestQAChinaPOPEndToEndRules(t *testing.T) {
	s := qaStore(t, qaPOPConfig())
	for _, tc := range []struct {
		campaign, input, expected string
	}{
		{qaCN, "CN", "CHN"},
		{qaPH, "PH", "PHL"},
		{qaMX, "MX", "MEX"},
		{qaMX, "PE", "PER"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			req := map[string]any{
				"id":   "pop",
				"tmax": 100,
				"device": map[string]any{
					"geo": map[string]any{
						"country": tc.input,
					},
				},
				"site": map[string]any{
					"domain": "https://example.com/page",
					"page":   "https://example.com/page",
				},
				"imp": []any{map[string]any{
					"id":       "p1",
					"tagid":    "slot",
					"secure":   1,
					"bidfloor": 2.5,
					"banner": map[string]any{
						"w": 300, "h": 250,
					},
				}},
			}
			qaCheck(t, s, tc.campaign, req, "p1", true)
			out := qaApply(t, s, tc.campaign, req)

			if out["tmax"] != float64(500) {
				t.Fatalf("wrong tmax: %v", out["tmax"])
			}
			device := out["device"].(map[string]any)
			if device["geo"].(map[string]any)["country"] != tc.expected {
				t.Fatalf("device GEO=%v", device)
			}
			user := out["user"].(map[string]any)
			if user["geo"].(map[string]any)["country"] != tc.expected {
				t.Fatalf("user GEO=%v", user)
			}
			imp := out["imp"].([]any)[0].(map[string]any)
			if _, present := imp["banner"]; present {
				t.Fatal("banner was not removed")
			}
			if imp["bidfloor"] != float64(2.5) {
				t.Fatalf("bidfloor overwritten: %v", imp)
			}
			if imp["ext"].(map[string]any)["type"] != "pop" {
				t.Fatalf("missing imp.ext.type: %v", imp)
			}
		})
	}

	for _, tc := range []struct {
		name   string
		tag    bool
		secure int
		want   bool
	}{
		{"valid", true, 1, true},
		{"missing tag", false, 1, false},
		{"secure zero", true, 0, false},
		{"both invalid", false, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			imp := map[string]any{"id": "p1", "secure": tc.secure}
			if tc.tag {
				imp["tagid"] = "slot"
			}
			qaCheck(t, s, qaCN,
				map[string]any{"imp": []any{imp}},
				"p1", tc.want,
			)
		})
	}
}

func TestQAAllAndCampaignRulesCombined(t *testing.T) {
	s := qaStore(t, map[string]any{
		"ALL": map[string]any{
			"filters": []any{
				qaWhite("imp.secure", 1),
			},
			"replace": []any{
				map[string]any{
					"field": "tmax",
					"mode":  "set_or_add",
					"value": 550,
				},
			},
		},
		qaCN + "," + qaPH: map[string]any{
			"filters": []any{
				map[string]any{
					"field":  "imp.tagid",
					"exists": true,
				},
			},
		},
		qaCN: map[string]any{
			"filters": []any{
				qaWhite("imp.instl", 0),
			},
			"replace": []any{
				map[string]any{
					"field": "tmax",
					"mode":  "set_or_add",
					"value": 500,
				},
			},
		},
	})

	base := map[string]any{
		"imp": []any{map[string]any{
			"id": "i", "secure": 1,
			"tagid": "slot", "instl": 0,
		}},
	}
	for _, id := range []string{qaCN, qaPH, qaMX} {
		qaCheck(t, s, id, base, "i", true)
	}

	missingTag := qaClone(t, base)
	delete(missingTag["imp"].([]any)[0].(map[string]any), "tagid")
	qaCheck(t, s, qaCN, missingTag, "i", false)
	qaCheck(t, s, qaPH, missingTag, "i", false)
	qaCheck(t, s, qaMX, missingTag, "i", true)

	wrongInstl := qaClone(t, base)
	wrongInstl["imp"].([]any)[0].(map[string]any)["instl"] = 1
	qaCheck(t, s, qaCN, wrongInstl, "i", false)
	qaCheck(t, s, qaPH, wrongInstl, "i", true)

	wrongSecure := qaClone(t, base)
	wrongSecure["imp"].([]any)[0].(map[string]any)["secure"] = 0
	for _, id := range []string{qaCN, qaPH, qaMX} {
		qaCheck(t, s, id, wrongSecure, "i", false)
	}

	for _, tc := range []struct {
		id   string
		want float64
	}{
		{qaCN, 500},
		{qaPH, 550},
		{qaMX, 550},
	} {
		out := qaApply(t, s, tc.id, base)
		if out["tmax"] != tc.want {
			t.Fatalf("%s tmax=%v want=%v",
				tc.id, out["tmax"], tc.want)
		}
	}
}

func TestQADifferentImpressionsDoNotLeak(t *testing.T) {
	s := qaStore(t, map[string]any{
		qaVID: map[string]any{
			"filters": []any{
				qaWhite("imp.video.mimes", "video/mp4"),
			},
		},
		qaIPP: map[string]any{
			"filters": []any{
				qaNative("native.ver", "1.2"),
			},
		},
		qaCN: map[string]any{
			"filters": []any{
				map[string]any{
					"field":  "imp.banner",
					"exists": true,
				},
			},
		},
	})
	native, _ := json.Marshal(qaNativeDocument())
	req := map[string]any{
		"imp": []any{
			map[string]any{
				"id": "video",
				"video": map[string]any{
					"mimes": []any{"video/mp4"},
				},
			},
			map[string]any{
				"id": "native",
				"native": map[string]any{
					"request": string(native),
				},
			},
			map[string]any{
				"id": "banner",
				"banner": map[string]any{
					"w": 300, "h": 250,
				},
			},
		},
	}
	for _, tc := range []struct {
		campaign, imp string
		pass          bool
	}{
		{qaVID, "video", true},
		{qaVID, "native", false},
		{qaVID, "banner", false},
		{qaIPP, "native", true},
		{qaIPP, "video", false},
		{qaCN, "banner", true},
		{qaCN, "video", false},
	} {
		qaCheck(t, s, tc.campaign, req, tc.imp, tc.pass)
	}
}

func TestQAInvalidChinaConfigurationsRejected(t *testing.T) {
	tests := []struct {
		name string
		rule map[string]any
	}{
		{"unknown field", map[string]any{
			"filters": []any{
				map[string]any{
					"field":  "unknown.field",
					"exists": true,
				},
			},
		}},
		{"mixed filter methods", map[string]any{
			"filters": []any{
				map[string]any{
					"field":       "imp.secure",
					"exists":      true,
					"values":      []any{1},
					"isWhiteList": true,
				},
			},
		}},
		{"wrong native selector", map[string]any{
			"filters": []any{
				map[string]any{
					"field":     "imp.native.request",
					"json_path": "native.assets[id=abc].img.w",
					"exists":    true,
				},
			},
		}},
		{"json_path on non-native field", map[string]any{
			"filters": []any{
				map[string]any{
					"field":     "imp.tagid",
					"json_path": "native.ver",
					"exists":    true,
				},
			},
		}},
		{"comparison on string field", map[string]any{
			"filters": []any{
				map[string]any{
					"field":   "imp.tagid",
					"compare": ">=",
					"value":   200,
				},
			},
		}},
		{"invalid comparison operator", map[string]any{
			"filters": []any{
				map[string]any{
					"field":   "imp.bidfloor",
					"compare": "==",
					"value":   200,
				},
			},
		}},
		{"set_or_add without value", map[string]any{
			"replace": []any{
				map[string]any{
					"field": "tmax",
					"mode":  "set_or_add",
				},
			},
		}},
		{"set_or_add wrong type", map[string]any{
			"replace": []any{
				map[string]any{
					"field": "tmax",
					"mode":  "set_or_add",
					"value": "300",
				},
			},
		}},
		{"set_or_add JSON-only field", map[string]any{
			"replace": []any{
				map[string]any{
					"field": "imp.ext.type",
					"mode":  "set_or_add",
					"value": "pop",
				},
			},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{
				qaCN: tc.rule,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(raw); err == nil {
				t.Fatalf("invalid rules accepted:\n%s", raw)
			}
		})
	}
}

// Tests retained from main2_test.go.
const (
	extraCN = "8cf66405-6013-423c-a967-a76c08c69ac4"
	extraPH = "9e4173f7-7aee-44a6-8039-db364cb6f4f5"
	extraMX = "1c9eeda4-3cb1-499d-aada-83c3b07e042f"
)

func extraStore(t *testing.T, rules map[string]any) *Store {
	t.Helper()
	raw, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(raw); err != nil {
		t.Fatalf("invalid fixture: %v\n%s", err, raw)
	}
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func extraApply(t *testing.T, s *Store, campaign string, request map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var req ortb.BidRequest
	if err := jsoniter.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	before, err := jsoniter.Marshal(&req)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.ApplyToRequest(campaign, &req)
	if err != nil {
		t.Fatal(err)
	}
	after, err := jsoniter.Marshal(&req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("input mutated\nbefore=%s\nafter=%s", before, after)
	}
	var out map[string]any
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func extraCheck(t *testing.T, s *Store, campaign string, request map[string]any, imp string, want bool) {
	t.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	ok, reason, err := s.CheckWithCache(campaign, newFilterCacheFromJSON(data), imp)
	if err != nil || ok != want {
		t.Fatalf("campaign=%s imp=%s accepted=%t want=%t reason=%q err=%v\n%s",
			campaign, imp, ok, want, reason, err, data)
	}
	if !ok && reason == "" {
		t.Fatal("filter rejection lacks reason")
	}
}

func extraImps(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	items, ok := out["imp"].([]any)
	if !ok {
		t.Fatalf("missing impressions: %v", out)
	}
	result := make([]map[string]any, len(items))
	for i, item := range items {
		var valid bool
		result[i], valid = item.(map[string]any)
		if !valid {
			t.Fatalf("imp %d has type %T", i, item)
		}
	}
	return result
}

func TestExtraPOPMissingBidfloorAndMultipleImpressions(t *testing.T) {
	s := extraStore(t, map[string]any{
		extraCN: map[string]any{
			"filters": []any{
				map[string]any{"field": "imp.tagid", "exists": true},
				map[string]any{"field": "imp.secure", "values": []any{1}, "isWhiteList": true},
			},
			"add": []any{
				map[string]any{"field": "imp.bidfloor", "value": 0},
				map[string]any{"field": "imp.ext", "postfix_field": "type", "value": "pop"},
			},
			"delete": []any{map[string]any{"field": "imp.banner"}},
			"replace": []any{
				map[string]any{"field": "tmax", "mode": "set_or_add", "value": 500},
			},
		},
	})
	request := map[string]any{"imp": []any{
		map[string]any{"id": "a", "tagid": "slot", "secure": 1,
			"bidfloor": 2.5, "banner": map[string]any{"w": 300, "h": 250}},
		map[string]any{"id": "b", "tagid": "slot", "secure": 1,
			"banner": map[string]any{"w": 300, "h": 250}},
	}}
	extraCheck(t, s, extraCN, request, "a", true)
	extraCheck(t, s, extraCN, request, "b", true)
	out := extraApply(t, s, extraCN, request)
	if out["tmax"] != float64(500) {
		t.Fatalf("missing tmax not inserted: %v", out)
	}
	imps := extraImps(t, out)
	if len(imps) != 2 || imps[0]["bidfloor"] != float64(2.5) || imps[1]["bidfloor"] != float64(0) {
		t.Fatalf("add must preserve old floor and insert missing zero: %+v", imps)
	}
	for _, imp := range imps {
		if _, ok := imp["banner"]; ok {
			t.Fatalf("banner survived deletion: %v", imp)
		}
		ext, ok := imp["ext"].(map[string]any)
		if !ok || ext["type"] != "pop" {
			t.Fatalf("missing postfix ext.type: %v", imp)
		}
	}
}

func TestExtraGEOCopyToAlreadyPopulatedUser(t *testing.T) {
	for _, tc := range []struct {
		id, original, converted string
	}{
		{extraCN, "CN", "CHN"},
		{extraPH, "PH", "PHL"},
		{extraMX, "MX", "MEX"},
		{extraMX, "PE", "PER"},
	} {
		t.Run(tc.original, func(t *testing.T) {
			s := extraStore(t, map[string]any{
				tc.id: map[string]any{
					"replace": []any{
						map[string]any{
							"field":     "device.geo.country",
							"countries": map[string]string{tc.original: tc.converted},
							"copy_to":   "user.geo.country",
						},
					},
				},
			})
			req := map[string]any{
				"imp":    []any{map[string]any{"id": "i"}},
				"device": map[string]any{"geo": map[string]any{"country": tc.original}},
				"user":   map[string]any{"geo": map[string]any{"country": "USA"}},
			}
			out := extraApply(t, s, tc.id, req)
			device := out["device"].(map[string]any)["geo"].(map[string]any)
			user := out["user"].(map[string]any)["geo"].(map[string]any)
			if device["country"] != tc.converted || user["country"] != tc.converted {
				t.Fatalf("copy_to must use transformed country: device=%v user=%v", device, user)
			}
		})
	}
}

func TestExtraFilterRunsBeforeReplacement(t *testing.T) {
	s := extraStore(t, map[string]any{
		extraCN: map[string]any{
			"filters": []any{
				map[string]any{"field": "imp.secure", "values": []any{1}, "isWhiteList": true},
			},
			"replace": []any{
				map[string]any{"field": "imp.secure", "mode": "set_or_add", "value": 1},
			},
		},
	})
	req := map[string]any{"imp": []any{map[string]any{"id": "i", "secure": 0}}}
	extraCheck(t, s, extraCN, req, "i", false)
	out := extraApply(t, s, extraCN, req)
	if extraImps(t, out)[0]["secure"] != float64(1) {
		t.Fatalf("replacement did not set secure: %v", out)
	}
	extraCheck(t, s, extraCN, out, "i", true)
}

func TestExtraNativeExactAndThresholdSemantics(t *testing.T) {
	path := "native.assets[id=102].img.wmin"
	exact := extraStore(t, map[string]any{
		extraCN: map[string]any{"filters": []any{
			map[string]any{"field": "imp.native.request", "json_path": path,
				"values": []any{200}, "isWhiteList": true},
		}},
	})
	minimum := extraStore(t, map[string]any{
		extraPH: map[string]any{"filters": []any{
			map[string]any{"field": "imp.native.request", "json_path": path,
				"compare": ">=", "value": 200},
		}},
	})
	for _, tc := range []struct {
		n              int
		exact, minimum bool
	}{
		{199, false, false},
		{200, true, true},
		{201, false, true},
		{250, false, true},
	} {
		doc, err := json.Marshal(map[string]any{
			"native": map[string]any{"assets": []any{
				map[string]any{"id": 100, "title": map[string]any{"len": 80}},
				map[string]any{"id": 102, "img": map[string]any{"wmin": tc.n}},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		req := map[string]any{"imp": []any{
			map[string]any{"id": "native-1", "native": map[string]any{"request": string(doc)}},
		}}
		extraCheck(t, exact, extraCN, req, "native-1", tc.exact)
		extraCheck(t, minimum, extraPH, req, "native-1", tc.minimum)
	}
}

func TestExtraBlacklistAbsentAndEmpty(t *testing.T) {
	s := extraStore(t, map[string]any{
		extraCN: map[string]any{"filters": []any{
			map[string]any{"field": "device.make", "values": []any{"blocked"}, "isWhiteList": false},
			map[string]any{"field": "imp.tagid", "exists": true},
		}},
	})
	for _, tc := range []struct {
		name   string
		device map[string]any
		tag    any
		want   bool
	}{
		{"missing blacklist field", nil, "x", true},
		{"different make", map[string]any{"make": "allowed"}, "x", true},
		{"blacklisted make", map[string]any{"make": "blocked"}, "x", false},
		{"empty tag exists", nil, "", true},
		{"missing tag", nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			imp := map[string]any{"id": "i"}
			if tc.tag != nil {
				imp["tagid"] = tc.tag
			}
			req := map[string]any{"imp": []any{imp}}
			if tc.device != nil {
				req["device"] = tc.device
			}
			extraCheck(t, s, extraCN, req, "i", tc.want)
		})
	}
}

func TestExtraValidationProhibitedCombinations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rules map[string]any
	}{
		{"empty whitelist", map[string]any{"filters": []any{
			map[string]any{"field": "imp.secure", "values": []any{}, "isWhiteList": true},
		}}},
		{"mixed native scalar types", map[string]any{"filters": []any{
			map[string]any{"field": "imp.native.request", "json_path": "native.ver",
				"values": []any{1, "1"}, "isWhiteList": true},
		}}},
		{"invalid native array selector", map[string]any{"filters": []any{
			map[string]any{"field": "imp.native.request", "json_path": "native.assets[-1].id", "exists": true},
		}}},
		{"unrecognized comparison", map[string]any{"filters": []any{
			map[string]any{"field": "imp.bidfloor", "compare": "!=", "value": 200},
		}}},
		{"filter type property forbidden", map[string]any{"filters": []any{
			map[string]any{"field": "imp.native.request", "json_path": "native.ver",
				"type": "value", "values": []any{"1.2"}, "isWhiteList": true},
		}}},
		{"wrong add type", map[string]any{"add": []any{
			map[string]any{"field": "tmax", "value": "500"},
		}}},
		{"delete array not allowed", map[string]any{"delete": []any{
			map[string]any{"field": "cur"},
		}}},
		{"replacement value forbidden with hostname", map[string]any{"replace": []any{
			map[string]any{"field": "site.domain", "mode": "hostname", "value": "example.com"},
		}}},
		{"countries plus set forbidden", map[string]any{"replace": []any{
			map[string]any{"field": "device.geo.country", "mode": "set",
				"value": "PHL", "countries": map[string]string{"PH": "PHL"}},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{extraCN: tc.rules})
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(raw); err == nil {
				t.Fatalf("accepted invalid configuration: %s", raw)
			}
		})
	}
}

func TestExtraInvalidUpdatePreservesRulesAndFile(t *testing.T) {
	initial := map[string]any{
		extraCN: map[string]any{"filters": []any{
			map[string]any{"field": "imp.secure", "values": []any{1}, "isWhiteList": true},
		}},
	}
	data, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	bad := []byte("{\"ALL\":{\"filters\":[{\"field\":\"imp.secure\",\"values\":[]}]}}")
	if err := s.UpdateJSON(bad); err == nil {
		t.Fatal("invalid update accepted")
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(persisted, data) {
		t.Fatalf("invalid update changed stored JSON: %s", persisted)
	}
	req := map[string]any{"imp": []any{map[string]any{"id": "i", "secure": 1}}}
	extraCheck(t, s, extraCN, req, "i", true)
	req["imp"].([]any)[0].(map[string]any)["secure"] = 0
	extraCheck(t, s, extraCN, req, "i", false)
}

func TestExtraNativeMalformedDoesNotPoisonOtherCampaign(t *testing.T) {
	s := extraStore(t, map[string]any{
		extraCN: map[string]any{"filters": []any{
			map[string]any{"field": "imp.native.request",
				"json_path": "native.assets[id=102].img.wmin",
				"values":    []any{200}, "isWhiteList": true},
		}},
		extraPH: map[string]any{"filters": []any{
			map[string]any{"field": "imp.tagid", "exists": true},
		}},
	})
	req := map[string]any{"imp": []any{
		map[string]any{"id": "i", "tagid": "slot",
			"native": map[string]any{"request": "{broken"}},
	}}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	cache := newFilterCacheFromJSON(raw)
	ok, _, err := s.CheckWithCache(extraCN, cache, "i")
	if ok || !errors.Is(err, ErrNativeFilter) {
		t.Fatalf("malformed native must fail with native error: ok=%t err=%v", ok, err)
	}
	ok, why, err := s.CheckWithCache(extraPH, cache, "i")
	if !ok || err != nil {
		t.Fatalf("unrelated campaign poisoned: ok=%t reason=%s err=%v", ok, why, err)
	}
}

// Additional contract and defensive-boundary tests. All original tests above
// are retained; these cases exercise the same existing five rule modules.
func coverageStore(rs RuleSet) *Store {
	s := &Store{}
	s.active.Store(&snapshot{byCampaign: map[string]RuleSet{idA: rs}})
	return s
}
func coverageRaw(s string) json.RawMessage { return json.RawMessage(s) }
func coverageBool(v bool) *bool            { return &v }

func TestAllNumericOperatorsAndBoundaries(t *testing.T) {
	for _, op := range []string{">", ">=", "<", "<="} {
		for _, number := range []int{199, 200, 201} {
			t.Run(fmt.Sprintf("%s/%d", op, number), func(t *testing.T) {
				s := extraStore(t, map[string]any{idA: map[string]any{"filters": []any{map[string]any{"field": "imp.bidfloor", "compare": op, "value": 200}}}})
				raw := []byte(fmt.Sprintf(`{"imp":[{"id":"one","bidfloor":%d}]}`, number))
				want := map[string]bool{">": number > 200, ">=": number >= 200, "<": number < 200, "<=": number <= 200}[op]
				got, reason, err := s.Check(idA, raw, "one")
				if err != nil || got != want || (!got && reason != "rtb_rule_filter_imp.bidfloor") {
					t.Fatal(got, reason, err)
				}
				absent, _, err := s.Check(idA, []byte(`{"imp":[{"id":"one"}]}`), "one")
				if err != nil || absent {
					t.Fatal("missing is not a numeric match", absent, err)
				}
			})
		}
	}
	for _, tc := range []struct {
		name       string
		left       any
		op, target string
		wantErr    bool
	}{
		{"wrong-type", "200", ">", "200", true},
		{"left-overflow", json.Number("1e9999"), ">", "200", true},
		{"target-invalid", json.Number("200"), ">", "{", true},
		{"target-overflow", json.Number("200"), ">", "1e9999", true},
		{"unknown-op", json.Number("200"), "==", "200", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := checkNumericComparison(tc.left, tc.op, coverageRaw(tc.target))
			if err == nil {
				t.Fatal("invalid comparison accepted")
			}
		})
	}
}

func TestProtobufFilterCacheProductionEntry(t *testing.T) {
	cfg := map[string]any{"ALL": map[string]any{"filters": []any{map[string]any{"field": "imp.native.request", "json_path": "assets[id=7].title.len", "compare": ">=", "value": 20}}}, idA: map[string]any{"filters": []any{map[string]any{"field": "device.os", "values": []string{"Android"}, "isWhiteList": true}}}}
	s := extraStore(t, cfg)
	impID, embedded, osName := "one", `{"assets":[{"id":7,"title":{"len":20}}]}`, "Android"
	req := &ortb.BidRequest{Device: &ortb.Device{Os: &osName}, Imp: []*ortb.Imp{{Id: &impID, Native: &ortb.Native{Request: &embedded}}}}
	cache := NewFilterCache(req)
	if !s.HasFilters(idA) || !s.HasFilters(idB) {
		t.Fatal("ALL/campaign filters not detected")
	}
	for _, campaign := range []string{idA, idB, idA} {
		ok, reason, err := s.CheckWithCache(campaign, cache, impID)
		if err != nil || !ok || reason != "" {
			t.Fatal(ok, reason, err)
		}
	}
	if len(cache.native) != 1 || !cache.decoded {
		t.Fatal("native cache not reused")
	}
	empty := extraStore(t, map[string]any{idA: map[string]any{"replace": []any{map[string]any{"field": "tmax", "mode": "set", "value": 20}}}})
	if empty.HasFilters(idA) || empty.HasFilters(idB) {
		t.Fatal("transform-only/unknown campaign reported filters")
	}
	if ok, _, err := empty.Check(idA, []byte("invalid JSON not inspected without filters"), ""); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, _, err := empty.Check(idB, nil, ""); !ok || err != nil {
		t.Fatal(ok, err)
	}
	var nilCache *FilterCache
	if _, err := nilCache.rootObject(); err == nil {
		t.Fatal("nil cache accepted")
	}
	if ok, _, err := s.CheckWithCache(idA, nilCache, ""); ok || err == nil {
		t.Fatal("nil cache must fail with filters")
	}
	for _, raw := range []string{"{", "null", "[]"} {
		if _, err := decodeJSON([]byte(raw)); err == nil {
			t.Fatal("nonobject request accepted", raw)
		}
	}
	nan := float32(math.NaN())
	bad := NewFilterCache(&ortb.BidRequest{Device: &ortb.Device{Geo: &ortb.Geo{Lat: &nan}}})
	if _, err := bad.rootObject(); err == nil {
		t.Fatal("marshal error lost")
	}
	if _, err := bad.rootObject(); err == nil {
		t.Fatal("marshal error not cached")
	}
}

func TestNativeJSONPathDefensiveBoundaries(t *testing.T) {
	for _, path := range []string{"", " assets[0]", "assets[999999999999999999999999999999999]", "assets[id=999999999999999999999999999999]"} {
		if _, err := parseNativeJSONPath(path); err == nil {
			t.Fatal("invalid path accepted", path)
		}
	}
	for _, tc := range []struct{ raw, typ string }{{`"x"`, "string"}, {`1`, "number"}, {`true`, "bool"}, {`{}`, ""}, {`null`, ""}} {
		if got := jsonScalarType(coverageRaw(tc.raw)); got != tc.typ {
			t.Fatal(tc, got)
		}
	}
	for _, tc := range []struct {
		value any
		typ   string
	}{{true, "bool"}, {map[string]any{}, "object"}, {[]any{}, "array"}, {nil, "null"}, {int(1), "unknown"}} {
		if got := nativeJSONType(tc.value); got != tc.typ {
			t.Fatal(got, tc.typ)
		}
	}
	for _, tc := range []struct {
		name, path, raw string
		exists, wantErr bool
	}{
		{"non-object-root", "assets", `[]`, false, true},
		{"non-object-parent", "title.len", `{"title":3}`, false, true},
		{"scalar-array-element", "assets[id=7].title", `{"assets":[3]}`, false, true},
		{"missing-asset-ids", "assets[id=7].title", `{"assets":[{}, {"id":null}]}`, false, false},
		{"null-indexed-item", "assets[0].title", `{"assets":[null]}`, false, false},
		{"index-out-of-range", "assets[1].title", `{"assets":[]}`, false, false},
		{"missing-key", "title.len", `{}`, false, false},
		{"null-value", "title.len", `{"title":null}`, false, false},
		{"correct-object", "title.len", `{"title":{"len":20}}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var doc any
			d := json.NewDecoder(strings.NewReader(tc.raw))
			d.UseNumber()
			if err := d.Decode(&doc); err != nil {
				t.Fatal(err)
			}
			steps, err := parseNativeJSONPath(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			_, exists, err := readNativeJSONPath(doc, steps)
			if exists != tc.exists || (err != nil) != tc.wantErr {
				t.Fatal(exists, err)
			}
		})
	}
	for _, raw := range []string{"null", "[]", "1", `"string"`} {
		if _, err := parseNativeDocument(raw); !errors.Is(err, ErrNativeFilter) {
			t.Fatal("nonobject native accepted", raw, err)
		}
	}
	for _, tc := range []struct{ raw, path string }{
		{"{", "x"}, {`{}`, ""}, {`{"imp":[{"native":{"request":3}}]}`, "x"},
	} {
		cache := newFilterCacheFromJSON([]byte(tc.raw))
		if _, _, err := cache.nativeValues("", tc.path); err == nil {
			t.Fatal(tc)
		}
	}
	s := extraStore(t, map[string]any{idA: map[string]any{"filters": []any{map[string]any{"field": "imp.native.request", "json_path": "flag", "isWhiteList": true, "values": []bool{true}}}}})
	for _, tc := range []struct {
		flag    string
		want    bool
		wantErr bool
	}{{`true`, true, false}, {`false`, false, false}, {`"true"`, false, true}, {`{}`, false, true}, {`[]`, false, false}} {
		native := fmt.Sprintf(`{"flag":%s}`, tc.flag)
		request, _ := json.Marshal(map[string]any{"imp": []any{map[string]any{"id": "one", "native": map[string]any{"request": native}}}})
		ok, _, err := s.Check(idA, request, "one")
		if ok != tc.want || (err != nil) != tc.wantErr {
			t.Fatal(tc, ok, err)
		}
	}
}

func TestPointerHelpersAbsentParentsAndInvalidPaths(t *testing.T) {
	req := &ortb.BidRequest{}
	if err := addProtobufField(req, "device.geo.country", coverageRaw(`"PH"`)); err != nil || req.GetDevice().GetGeo().GetCountry() != "PH" {
		t.Fatal(req, err)
	}
	if err := addProtobufField(req, "device.geo.country", coverageRaw(`"MX"`)); err != nil || req.GetDevice().GetGeo().GetCountry() != "PH" {
		t.Fatal("add replaced present field", err)
	}
	for _, tc := range []struct {
		req         *ortb.BidRequest
		path, value string
	}{
		{&ortb.BidRequest{}, "unknown", `1`},
		{&ortb.BidRequest{}, "tmax", `"bad"`},
		{&ortb.BidRequest{}, "device.ip.child", `1`},
		{&ortb.BidRequest{Imp: []*ortb.Imp{{}}}, "imp.secure", `"bad"`},
	} {
		if err := addProtobufField(tc.req, tc.path, coverageRaw(tc.value)); err == nil {
			t.Fatal(tc.path)
		}
	}
	ip := "1.2.3.4"
	if err := addProtobufField(&ortb.BidRequest{Device: &ortb.Device{Ip: &ip}}, "device.ip.child", coverageRaw(`1`)); err == nil {
		t.Fatal("scalar parent accepted")
	}
	if _, _, ok := protoMember(reflect.TypeOf(1), "x"); ok {
		t.Fatal("scalar has protobuf member")
	}
	clearPointerValue(reflect.Value{}, []string{"x"})
	clearPointerValue(reflect.ValueOf(req), nil)
	var nilReq *ortb.BidRequest
	clearPointerPath(nilReq, "device")
	clearPointerValue(reflect.ValueOf(1), []string{"x"})
	clearPointerPath(req, "missing")
	clearPointerPath(&ortb.BidRequest{}, "device.geo.country")
	clearPointerPath(req, "device.geo.country")
	if req.Device.Geo.Country != nil {
		t.Fatal("pointer not cleared")
	}
}

func TestJSONTransformBoundaryCases(t *testing.T) {
	root := map[string]any{"x": json.Number("0"), "imp": []any{nil, map[string]any{"id": "one"}}, "site": map[string]any{"domain": "abc"}}
	if len(fieldTargets(root, "x", "", false)) != 1 {
		t.Fatal("root target")
	}
	if fieldTargets(map[string]any{}, "imp.tagid", "", false) != nil {
		t.Fatal("absent imp")
	}
	if len(fieldTargets(root, "imp.tagid", "one", false)) != 1 {
		t.Fatal("nonobject impression was not skipped")
	}
	if descend(map[string]any{"a": 1}, []string{"a", "b"}, true) != nil {
		t.Fatal("scalar parent overwritten")
	}
	for _, key := range []string{"missing", "nil"} {
		values, exists := readField(map[string]any{"nil": nil}, key, "")
		if exists || values != nil {
			t.Fatal(key)
		}
	}
	if err := addPostfixField(map[string]any{"site": "scalar"}, "site", "x", 1); err == nil {
		t.Fatal("base scalar overwritten")
	}
	if err := addPostfixField(map[string]any{"site": map[string]any{"x": 1}}, "site", "x.y", 1); err == nil {
		t.Fatal("postfix scalar overwritten")
	}
	copyTo(root, "imp.tagid", "imp.bidfloorcur")
	if v, ok := getFieldValue(root, "x"); !ok || v != json.Number("0") {
		t.Fatal(v, ok)
	}
	compound := map[string]any{"a": []any{map[string]any{"b": "original"}}}
	copied := copyValue(compound).(map[string]any)
	copied["a"].([]any)[0].(map[string]any)["b"] = "changed"
	if compound["a"].([]any)[0].(map[string]any)["b"] != "original" {
		t.Fatal("shared object after copy")
	}
	setField(root, "new", 1)
	if root["new"] != 1 {
		t.Fatal(root)
	}
	if err := replaceField(root, Replace{Field: "x", Mode: "set", Value: coverageRaw("{")}); err == nil {
		t.Fatal("invalid replacement JSON accepted")
	}
	if err := replaceField(root, Replace{Field: "x", Mode: "unknown"}); err == nil {
		t.Fatal("unknown mode accepted")
	}
	if got := hostname("http://[bad"); got != "" {
		t.Fatal(got)
	}
	for _, tc := range []struct{ page, host, want string }{{"//example.com/x", "", "https://example.com/x"}, {"example.com/x", "", "https://example.com/x"}, {"ftp://example.com/x", "", "ftp://example.com/x"}, {"", "", ""}, {"/x", "example.com", "https://example.com/x"}} {
		if got := fullPageURL(tc.page, tc.host); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestStoreLifecycleErrorsAndSavedBytes(t *testing.T) {
	var absent *Store
	if _, err := absent.SavedJSON(); err == nil {
		t.Fatal("nil SavedJSON")
	}
	if _, err := absent.Saved(); err == nil {
		t.Fatal("nil Saved")
	}
	if absent.UpdateJSON([]byte(`{}`)) == nil || len(absent.Memory()) != 0 || absent.Has(idA) || absent.HasFilters(idA) {
		t.Fatal("nil store behavior")
	}
	if len((&Store{}).Memory()) != 0 || (&Store{}).Has(idA) {
		t.Fatal("uninitialized store behavior")
	}
	if _, err := NewStore(" "); err == nil {
		t.Fatal("empty path accepted")
	}
	missing := filepath.Join(t.TempDir(), "missing.json")
	if _, err := NewStore(missing); err == nil {
		t.Fatal("missing file accepted")
	}
	raw := []byte("{\n  \"" + idA + "," + idB + "\": {\"filters\":[{\"field\":\"device.os\",\"exists\":true}]}\n}\n")
	p := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(p, raw, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.SavedJSON()
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("authored JSON changed", string(got), err)
	}
	for _, bad := range []string{"{", `{"invalid":{}}`} {
		if err = os.WriteFile(p, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if s.Reload() == nil {
			t.Fatal("bad reload accepted")
		}
		if _, err = s.Saved(); err == nil {
			t.Fatal("bad Saved accepted")
		}
		if !s.HasFilters(idA) {
			t.Fatal("bad reload changed active snapshot")
		}
	}
	if err = os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Saved(); err == nil {
		t.Fatal("read error hidden")
	}
	if _, err = s.SavedJSON(); err == nil {
		t.Fatal("raw read error hidden")
	}
	// A real rename failure must retain the last valid in-memory snapshot.
	if err = os.Mkdir(p, 0700); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateJSON([]byte(`{}`)); err == nil || !s.Has(idA) {
		t.Fatal("failed persistence altered memory", err)
	}
	pending, err := filepath.Glob(filepath.Join(filepath.Dir(p), ".adv-rtb-rules-*"))
	if err != nil || len(pending) != 0 {
		t.Fatal("failed rename leaked temp file", pending, err)
	}
}

func TestSchemaScalarNestedAndConflictBoundaries(t *testing.T) {
	for _, tc := range []struct {
		raw, typ string
		want     bool
	}{
		{"{", "number", false}, {`true`, "bool", true}, {`1`, "bool", false}, {`1`, "unknown", false},
		{`1`, "strings", false}, {`["x",1]`, "strings", false}, {`1`, "numbers", false}, {`[1,2]`, "numbers", true},
	} {
		if got := validType(coverageRaw(tc.raw), tc.typ); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	if _, err := validateField(" device.os"); err == nil {
		t.Fatal("padded field accepted")
	}
	for _, tc := range []struct {
		path, raw string
		want      bool
	}{
		{"device", "{", false}, {"device", "null", false}, {"device", `{"notknown":1}`, false}, {"device", `{"os":1}`, false},
		{"device", `{"geo":{"notknown":1}}`, false}, {"device", `{"geo":{"country":"PH"},"os":"Android"}`, true},
	} {
		if got := validNestedObject(tc.path, coverageRaw(tc.raw)); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	for _, path := range []string{"", " x"} {
		if validPostfix(path) {
			t.Fatal("invalid postfix", path)
		}
	}
	if !postfixConflictsWithProtobuf("device", "geo.country.x") || !postfixConflictsWithProtobuf("device", "os.x") {
		t.Fatal("scalar postfix overlap not rejected")
	}
	if compatibleCopyPaths("unknown", "user.geo") {
		t.Fatal("unknown copy path accepted")
	}
	if renamedPath("id", "newid") != "newid" || parentPath("id") != "" {
		t.Fatal("root path helpers")
	}
	invalid := []RuleSet{
		{Filters: []Filter{{Field: "device.os"}}},
		{Filters: []Filter{{Field: "imp.native.request", JSONPath: "x", IsWhiteList: coverageBool(true), Values: []json.RawMessage{coverageRaw(`{}`)}}}},
		{Add: []Add{{Field: "unknown", Value: coverageRaw(`1`)}}},
		{Add: []Add{{Field: "imp.ext", PostfixField: "x", Value: coverageRaw(`null`)}}},
		{Add: []Add{{Field: "imp.ext", PostfixField: "x", Value: coverageRaw(`{`)}}},
		{Replace: []Replace{{Field: "device.os", Mode: "full_page"}}},
		{Replace: []Replace{{Field: "device.os", Mode: "unknown"}}},
		{Replace: []Replace{{Field: "device.geo.country", Countries: map[string]string{"ph": "PHL"}}}},
		{Delete: []Delete{{Field: "device"}}, Replace: []Replace{{Field: "device.os", Mode: "set", Value: coverageRaw(`"x"`)}}},
		{Delete: []Delete{{Field: "user.geo"}}, Replace: []Replace{{Field: "device.geo", Mode: "set", Value: coverageRaw(`{}`), CopyTo: "user.geo"}}},
		{Delete: []Delete{{Field: "device"}}, Rename: []Rename{{Field: "device.deviceType", ToField: "devicetype"}}},
		{Rename: []Rename{{Field: "device.os", ToField: "notknown"}}},
	}
	for i, rs := range invalid {
		if err := validateSet(rs); err == nil {
			t.Fatal("invalid rule set accepted", i, rs)
		}
	}
	rs := RuleSet{Add: []Add{{Field: "imp.ext", PostfixField: "extra", Value: coverageRaw(`1`)}, {Field: "device.os", Value: coverageRaw(`"x"`)}}}
	if err := validateSet(rs); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		fn   func([]byte) error
		raw  string
	}{
		{"filter-trailing", new(Filter).UnmarshalJSON, `{"field":"device.os","exists":true} true`},
		{"filter-stray-value", new(Filter).UnmarshalJSON, `{"field":"device.os","exists":true,"value":1}`},
		{"filter-empty-path", new(Filter).UnmarshalJSON, `{"field":"device.os","exists":true,"json_path":""}`},
		{"add-trailing", new(Add).UnmarshalJSON, `{"field":"device.os","value":"x"} true`},
		{"replace-trailing", new(Replace).UnmarshalJSON, `{"field":"device.os","mode":"set","value":"x"} true`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn([]byte(tc.raw)); err == nil {
				t.Fatal("invalid JSON accepted")
			}
		})
	}
}

func TestConfigurationSyntaxAndMergedConflicts(t *testing.T) {
	for _, raw := range []string{
		`[]`, `null`, `1`, `{"` + idA + `":null}`, `{"not-a-uuid":{}}`,
		`{"` + idA + `":{"filters":null}}`, `{"` + idA + `":{"add":null}}`, `{"` + idA + `":{"replace":null}}`, `{"` + idA + `":{"delete":null}}`, `{"` + idA + `":{"rename":null}}`,
		`{"` + idA + `":{"add":[{"field":"device.os,device.os","value":"x"}]}}`,
		`{"` + idA + `":{"replace":[{"field":"device.os,","mode":"set","value":"x"}]}}`,
		`{"` + idA + `":{"delete":[{"field":"device,,user"}]}}`,
		`{"ALL":{"delete":[{"field":"device"}]},"` + idA + `":{"add":[{"field":"device.os","value":"x"}]}}`,
	} {
		if _, err := parse([]byte(raw)); err == nil {
			t.Fatal("invalid configuration accepted", raw)
		}
	}
	for _, raw := range []string{"", `{"a"`, `{"a":`, `[{`, `{} {}`} {
		if rejectDuplicateKeys([]byte(raw)) == nil {
			t.Fatal("invalid syntax accepted", raw)
		}
	}
	// Merge only matching module/path keys; nonconflicting entries survive.
	prev := RuleSet{Add: []Add{{Field: "device.os", Value: coverageRaw(`"old"`)}}, Delete: []Delete{{Field: "imp.banner"}}, Rename: []Rename{{Field: "device.deviceType", ToField: "devicetype"}}}
	next := RuleSet{Add: []Add{{Field: "device.os", Value: coverageRaw(`"new"`)}}, Delete: []Delete{{Field: "imp.video"}}, Rename: []Rename{{Field: "device.deviceType", ToField: "devicetype"}}}
	merged := mergeRuleSet(prev, next)
	if len(merged.Add) != 1 || string(merged.Add[0].Value) != `"new"` || len(merged.Delete) != 2 || len(merged.Rename) != 1 {
		t.Fatal(merged)
	}
}

func TestApplyRequestDefensiveFailuresAndNoRules(t *testing.T) {
	if _, err := (&Store{}).ApplyToRequest(idA, nil); err == nil {
		t.Fatal("nil request accepted")
	}
	id := "request"
	req := &ortb.BidRequest{Id: &id}
	raw, err := (&Store{}).ApplyToRequest(idA, req)
	if err != nil || !bytes.Contains(raw, []byte(`"id":"request"`)) {
		t.Fatal(string(raw), err)
	}
	if _, err = coverageStore(RuleSet{}).ApplyToRequest(idA, req); err != nil {
		t.Fatal(err)
	}
	nan := float32(math.NaN())
	if _, err = coverageStore(RuleSet{}).ApplyToRequest(idA, &ortb.BidRequest{Device: &ortb.Device{Geo: &ortb.Geo{Lat: &nan}}}); err == nil {
		t.Fatal("marshal failure hidden")
	}
	// Directly constructed invalid snapshots exercise defensive runtime errors;
	// public NewStore/UpdateJSON reject these configurations before activation.
	for _, rs := range []RuleSet{
		{Add: []Add{{Field: "tmax", Value: coverageRaw(`"wrong"`)}}},
		{Replace: []Replace{{Field: "tmax", Mode: "set_or_add", Value: coverageRaw(`"wrong"`)}}},
		{Add: []Add{{Field: "site", PostfixField: "extra", Value: coverageRaw(`{`)}}},
		{Add: []Add{{Field: "site", PostfixField: "id.child", Value: coverageRaw(`1`)}}},
		{Replace: []Replace{{Field: "id", Mode: "unknown"}}},
	} {
		if _, err = coverageStore(rs).ApplyToRequest(idA, &ortb.BidRequest{Site: &ortb.Site{Id: &id}}); err == nil {
			t.Fatal("runtime error lost", rs)
		}
	}
}

func TestEncodedRulePhaseRejectsInvalidJSON(t *testing.T) {
	for _, raw := range []string{"{", "null", "[]"} {
		if _, err := applyEncodedRules([]byte(raw), RuleSet{}); err == nil {
			t.Fatal("invalid encoded input accepted", raw)
		}
	}
}

type failingRulesTemp struct {
	*os.File
	stage                 string
	failure               error
	writes, syncs, closes int
}

func (f *failingRulesTemp) Write(b []byte) (int, error) {
	f.writes++
	if f.stage == "write" {
		return 0, f.failure
	}
	return f.File.Write(b)
}
func (f *failingRulesTemp) Sync() error {
	f.syncs++
	if f.stage == "sync" {
		return f.failure
	}
	return f.File.Sync()
}
func (f *failingRulesTemp) Close() error {
	f.closes++
	err := f.File.Close()
	if f.stage == "close" {
		return f.failure
	}
	return err
}

func TestAtomicWriteAllFileFailureStages(t *testing.T) {
	for _, stage := range []string{"create", "write", "sync", "close"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "rules.json")
			old := []byte(`{"ALL":{}}`)
			if err := os.WriteFile(target, old, 0600); err != nil {
				t.Fatal(err)
			}
			injected := fmt.Errorf("injected %s failure", stage)
			var file *failingRulesTemp
			err := atomicWriteWithTemp(target, []byte(`{}`), func(dir, pattern string) (rulesTempFile, error) {
				if stage == "create" {
					return nil, injected
				}
				real, e := os.CreateTemp(dir, pattern)
				if e != nil {
					return nil, e
				}
				file = &failingRulesTemp{File: real, stage: stage, failure: injected}
				return file, nil
			})
			if !errors.Is(err, injected) {
				t.Fatal("original I/O error not returned", err)
			}
			got, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(got, old) {
				t.Fatal("failed update changed existing file", string(got), err)
			}
			files, err := filepath.Glob(filepath.Join(dir, ".adv-rtb-rules-*"))
			if err != nil || len(files) != 0 {
				t.Fatal("temporary file leaked", files, err)
			}
			if file != nil && (file.closes != 1 || file.writes != 1) {
				t.Fatal("temp file not closed exactly once", file.closes, file.writes)
			}
			if stage == "write" && file.syncs != 0 {
				t.Fatal("sync after failed write")
			}
			if (stage == "sync" || stage == "close") && file.syncs != 1 {
				t.Fatal("sync count", file.syncs)
			}
		})
	}
	dir := t.TempDir()
	blocker := filepath.Join(dir, "regular-file")
	if err := os.WriteFile(blocker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	err := atomicWriteWithTemp(filepath.Join(blocker, "rules.json"), nil, func(string, string) (rulesTempFile, error) {
		called = true
		return nil, errors.New("unexpected create")
	})
	if err == nil || called {
		t.Fatal("mkdir failure not returned before create", err, called)
	}
	for _, raw := range []string{`{"a":1,7:2}`, `{"a":1, true}`} {
		if err = rejectDuplicateKeys([]byte(raw)); err == nil {
			t.Fatal("non-string JSON key accepted", raw)
		}
	}
}

func TestGroupedConflictsFollowAuthoredOrder(t *testing.T) {
	first := `"` + idA + `,` + idB + `":{"replace":[{"field":"device.os","mode":"set","value":"first"}],"add":[{"field":"site.id","value":"preserved"}]}`
	last := `"` + idA + `":{"replace":[{"field":"device.os","mode":"set","value":"last"}]}`
	for _, tc := range []struct{ body, want string }{{first + "," + last, `"last"`}, {last + "," + first, `"first"`}} {
		raw := []byte("{" + tc.body + "}")
		p := filepath.Join(t.TempDir(), "rules.json")
		if err := os.WriteFile(p, raw, 0600); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 64; i++ {
			s, err := NewStore(p)
			if err != nil {
				t.Fatal(err)
			}
			for _, reload := range []func() error{s.Reload, func() error { return s.UpdateJSON(raw) }} {
				if err = reload(); err != nil {
					t.Fatal(err)
				}
				rules := s.Memory()[idA]
				if len(rules.Replace) != 1 || string(rules.Replace[0].Value) != tc.want || len(rules.Add) != 1 {
					t.Fatalf("authored-order precedence lost after load/update %d: %+v; want %s", i, rules, tc.want)
				}
			}
			saved, err := s.SavedJSON()
			if err != nil || !bytes.Equal(saved, raw) {
				t.Fatal("authored JSON rewritten", err)
			}
		}
	}
}

func TestConcurrentRuntimeChecksTransformsAndSnapshotIsolation(t *testing.T) {
	raw := []byte(`{"` + idA + `":{"filters":[{"field":"device.os","exists":true}],"replace":[{"field":"tmax","mode":"set_or_add","value":1}],"delete":[{"field":"imp.banner"}]}}`)
	p := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(p, raw, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	osName, impID := "Android", "one"
	req := &ortb.BidRequest{Device: &ortb.Device{Os: &osName}, Imp: []*ortb.Imp{{Id: &impID, Banner: &ortb.Banner{}}}}
	before := proto.Clone(req)
	changed := s.Memory()
	changed[idA].Replace[0].Value[0] = '9'
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				ok, reason, e := s.CheckWithCache(idA, NewFilterCache(req), impID)
				if e != nil || !ok || reason != "" {
					t.Error(ok, reason, e)
					return
				}
				out, e := s.ApplyToRequest(idA, req)
				if e != nil {
					t.Error(e)
					return
				}
				var decoded struct {
					Tmax int
					Imp  []struct{ Banner any }
				}
				if e = json.Unmarshal(out, &decoded); e != nil || decoded.Tmax != 1 || len(decoded.Imp) != 1 || decoded.Imp[0].Banner != nil {
					t.Error("inconsistent runtime snapshot", string(out), e)
					return
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 25; i++ {
			if err := s.UpdateJSON(raw); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	wg.Wait()
	if !proto.Equal(before, req) {
		t.Fatal("shared inbound request mutated")
	}
	if got := string(s.Memory()[idA].Replace[0].Value); got != "1" {
		t.Fatal("Memory caller corrupted active rules", got)
	}
}

// Bounded fuzz runs use the same public validation/persistence contracts.
// These seeds also execute in an ordinary go test run.
func FuzzRulesJSONPreservesLastValidSnapshot(f *testing.F) {
	for _, seed := range []string{`{}`, `{"ALL":{"filters":[{"field":"device.os","exists":true}]}}`, `{"` + idA + `,` + idB + `":{"replace":[{"field":"tmax","mode":"set_or_add","value":250}]}}`, `{"ALL":{"filters":[{"field":"imp.native.request","json_path":"assets[id=7].title.len","compare":">=","value":20}]}}`, `null`, `{"ALL":{"filters":null}}`, `{"ALL":{},"ALL":{}}`, `{`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		initial := []byte(`{"` + idA + `":{}}`)
		p := filepath.Join(t.TempDir(), "rules.json")
		if err := os.WriteFile(p, initial, 0600); err != nil {
			t.Fatal(err)
		}
		s, err := NewStore(p)
		if err != nil {
			t.Fatal(err)
		}
		validErr := Validate([]byte(input))
		updateErr := s.UpdateJSON([]byte(input))
		if (validErr == nil) != (updateErr == nil) {
			t.Fatal("validation/update disagree", validErr, updateErr)
		}
		saved, err := s.SavedJSON()
		if err != nil {
			t.Fatal(err)
		}
		if validErr != nil {
			if !bytes.Equal(saved, initial) || !s.Has(idA) {
				t.Fatal("rejected input altered file/snapshot")
			}
			return
		}
		if !bytes.Equal(saved, []byte(input)) {
			t.Fatal("authored JSON changed")
		}
		reloaded, err := NewStore(p)
		if err != nil {
			t.Fatal("accepted input cannot reload", err)
		}
		if !reflect.DeepEqual(s.Memory(), reloaded.Memory()) {
			t.Fatal("reload changed merged rules")
		}
	})
}

func TestALLPriorityIndependentOfAuthoredPosition(t *testing.T) {
	global := `"ALL":{"replace":[{"field":"tmax","mode":"set_or_add","value":100}],"add":[{"field":"device.os","value":"Android"}]}`
	specific := `"` + idA + `":{"replace":[{"field":"tmax","mode":"set_or_add","value":200}]}`
	for _, body := range []string{global + "," + specific, specific + "," + global} {
		p := filepath.Join(t.TempDir(), "rules.json")
		raw := []byte("{" + body + "}")
		if err := os.WriteFile(p, raw, 0600); err != nil {
			t.Fatal(err)
		}
		s, err := NewStore(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			id   string
			want int
		}{{idA, 200}, {idB, 100}} {
			got, err := s.ApplyToRequest(tc.id, &ortb.BidRequest{})
			if err != nil {
				t.Fatal(err)
			}
			var value struct {
				Tmax   int
				Device struct {
					OS string `json:"os"`
				}
			}
			if err = json.Unmarshal(got, &value); err != nil || value.Tmax != tc.want || value.Device.OS != "Android" {
				t.Fatal(tc, string(got), err)
			}
		}
	}
}

// 54Ads requirements supplied by the user after the original coverage patch.
// The literals below are independent of production/helper-generated profiles.
// Video/IPP IDs are test-only placeholders, never active campaign IDs.
const chinaPartnerProfilesJSON = `{
  "8cf66405-6013-423c-a967-a76c08c69ac4,9e4173f7-7aee-44a6-8039-db364cb6f4f5,1c9eeda4-3cb1-499d-aada-83c3b07e042f": {
    "add": [{"field":"imp.bidfloor","value":0},{"field":"imp.ext","postfix_field":"type","value":"pop"}],
    "replace": [{"field":"tmax","mode":"set_or_add","value":500},{"field":"site.domain","mode":"hostname"},{"field":"site.page","mode":"full_page"}],
    "delete": [{"field":"imp.banner"}],
    "rename": [{"field":"device.deviceType","to_field":"devicetype"}],
    "filters": [{"field":"imp.tagid","exists":true},{"field":"imp.secure","isWhiteList":true,"values":[1]}]
  },
  "8cf66405-6013-423c-a967-a76c08c69ac4": {"replace":[{"field":"device.geo.country","countries":{"CN":"CHN"},"copy_to":"user.geo.country"},{"field":"user.geo.country","countries":{"CN":"CHN"}}]},
  "9e4173f7-7aee-44a6-8039-db364cb6f4f5": {"replace":[{"field":"device.geo.country","countries":{"PH":"PHL"},"copy_to":"user.geo.country"},{"field":"user.geo.country","countries":{"PH":"PHL"}}]},
  "1c9eeda4-3cb1-499d-aada-83c3b07e042f": {"replace":[{"field":"device.geo.country","countries":{"MX":"MEX","PE":"PER"},"copy_to":"user.geo.country"},{"field":"user.geo.country","countries":{"MX":"MEX","PE":"PER"}}]},
  "11111111-1111-4111-8111-111111111111": {
    "filters": [
      {"field":"at","isWhiteList":true,"values":[1]},
      {"field":"cur","isWhiteList":true,"values":["USD"]},
      {"field":"imp.tagid","exists":true},
      {"field":"imp.instl","isWhiteList":true,"values":[0]},
      {"field":"imp.secure","isWhiteList":true,"values":[1]},
      {"field":"imp.bidfloorcur","isWhiteList":true,"values":["USD"]},
      {"field":"imp.video.mimes","isWhiteList":true,"values":["video/mp4"]},
      {"field":"imp.video.protocols","isWhiteList":true,"values":[3]},
      {"field":"imp.video.w","isWhiteList":true,"values":[340]},
      {"field":"imp.video.h","isWhiteList":true,"values":[192]},
      {"field":"imp.video.linearity","isWhiteList":true,"values":[1]},
      {"field":"imp.video.sequence","isWhiteList":true,"values":[1]},
      {"field":"imp.video.boxingallowed","isWhiteList":true,"values":[1]},
      {"field":"imp.video.skipmin","isWhiteList":true,"values":[0]},
      {"field":"imp.video.skipafter","isWhiteList":true,"values":[0]}
    ],
    "replace":[{"field":"tmax","mode":"set_or_add","value":550}]
  },
  "33333333-3333-4333-8333-333333333333": {
    "filters":[
      {"field":"at","isWhiteList":true,"values":[1]},
      {"field":"cur","isWhiteList":true,"values":["USD"]},
      {"field":"imp.tagid","exists":true},
      {"field":"imp.instl","isWhiteList":true,"values":[0]},
      {"field":"imp.secure","isWhiteList":true,"values":[1]},
      {"field":"imp.bidfloorcur","isWhiteList":true,"values":["USD"]},
      {"field":"imp.native.ver","isWhiteList":true,"values":["1.2"]},
      {"field":"imp.native.request","exists":true},
      {"field":"imp.native.request","json_path":"native.ver","isWhiteList":true,"values":["1.2"]},
      {"field":"imp.native.request","json_path":"native.assets[id=100].required","isWhiteList":true,"values":[1]},
      {"field":"imp.native.request","json_path":"native.assets[id=100].title.len","isWhiteList":true,"values":[80]},
      {"field":"imp.native.request","json_path":"native.assets[id=101].required","isWhiteList":true,"values":[1]},
      {"field":"imp.native.request","json_path":"native.assets[id=101].data.type","isWhiteList":true,"values":[1]},
      {"field":"imp.native.request","json_path":"native.assets[id=101].data.len","isWhiteList":true,"values":[80]},
      {"field":"imp.native.request","json_path":"native.assets[id=102].required","isWhiteList":true,"values":[1]},
      {"field":"imp.native.request","json_path":"native.assets[id=102].img.type","isWhiteList":true,"values":[3]},
      {"field":"imp.native.request","json_path":"native.assets[id=102].img.w","isWhiteList":true,"values":[200]},
      {"field":"imp.native.request","json_path":"native.assets[id=102].img.h","isWhiteList":true,"values":[200]},
      {"field":"imp.native.request","json_path":"native.assets[id=102].img.wmin","isWhiteList":true,"values":[200]},
      {"field":"imp.native.request","json_path":"native.assets[id=102].img.hmin","isWhiteList":true,"values":[200]}
    ],
    "replace":[{"field":"tmax","mode":"set_or_add","value":300}]
  }
}
`

const chinaVideoSampleJSON = `{
 "id":"video-test-request-001","test":1,"at":1,"tmax":550,"cur":["USD"],
 "imp":[{"id":"video-test-imp-001","instl":0,"tagid":"example-site-video","bidfloor":0.00001,"bidfloorcur":"USD","secure":1,
 "video":{"mimes":["video/mp4"],"protocols":[3],"w":340,"h":192,"linearity":1,"sequence":1,"boxingallowed":1,"skipmin":0,"skipafter":0}}],
 "site":{"id":"test-site-001","name":"Example Site","domain":"example.com","page":"https://example.com/video","publisher":{"id":"test-publisher-001","domain":"example.com"}},
 "device":{"ip":"192.0.2.1","ua":"Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1","devicetype":4,"make":"Apple","model":"iPhone","os":"iOS","osv":"18.7","js":1,"language":"en"},
 "user":{"id":"test-user-001"}
}
`

const chinaIPPSampleJSON = `{
 "id":"push-test-request-001","test":1,"at":1,"tmax":300,"cur":["USD"],
 "imp":[{"id":"push-test-imp-001","instl":0,"tagid":"example-site-push","bidfloor":0.01,"bidfloorcur":"USD","secure":1,
 "native":{"ver":"1.2","request":"{\"native\":{\"ver\":\"1.2\",\"assets\":[{\"id\":100,\"required\":1,\"title\":{\"len\":80}},{\"id\":101,\"required\":1,\"data\":{\"type\":1,\"len\":80}},{\"id\":102,\"required\":1,\"img\":{\"type\":3,\"w\":200,\"h\":200,\"wmin\":200,\"hmin\":200}}]}}"}}],
 "site":{"id":"test-site-001","name":"Example Site","domain":"example.com","page":"https://example.com/test-page","publisher":{"id":"test-publisher-001"}},
 "device":{"ip":"192.0.2.1","ua":"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Mobile Safari/537.36","devicetype":4,"os":"Android","osv":"10","js":1,"language":"en"},
 "user":{"id":"test-user-001"}
}
`

// Requirement ledger for the supplied 54Ads messages (not inferred from coverage):
// POP: preserve real tagid; absent minimum -> 0; retain a supplied minimum;
// secure=1 for this HTTPS-only profile; PHL and tmax=500; no fabricated metadata.
// BAN: ISO alpha-3 for every supported country, both existing geo country paths.
// VID: the literal VAST 3.0 request, all 15 configured checks and tmax=550.
// IPP: the literal Native 1.2 JSON string, wrapper, assets 100/101/102, tmax=300.
// The existing posted profile specifies exact dimensions/lengths, not >=/<=.
// Old behavior is checked against a frozen pre-rtbrules oracle below; approved
// copy_to/default-floor differences have separate explicit assertions.
// The literal posted profile remains a historical regression fixture. The
// later agreed site.id copy and ALL rename are checked by CurrentContract tests.
const chinaBannerTestID = "55555555-5555-4555-8555-555555555555"

func chinaObject(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func chinaCopy(t *testing.T, in map[string]any) map[string]any {
	t.Helper()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	return chinaObject(t, string(b))
}
func chinaProfiles(t *testing.T) map[string]any {
	t.Helper()
	return chinaObject(t, chinaPartnerProfilesJSON)
}
func chinaStore(t *testing.T, config map[string]any) *Store {
	t.Helper()
	b, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "54ads.json")
	if err = os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func chinaSample(t *testing.T, format string) map[string]any {
	t.Helper()
	if format == "IPP" {
		return chinaObject(t, chinaIPPSampleJSON)
	}
	r := chinaObject(t, chinaVideoSampleJSON)
	if format == "POP" {
		r["id"] = "pop-captured-request"
		r["tmax"] = 500
		imp := r["imp"].([]any)[0].(map[string]any)
		imp["id"] = "pop-imp-one"
		imp["tagid"] = "placement-from-input-77"
		imp["bidfloor"] = 2.5
		delete(imp, "video")
		imp["banner"] = map[string]any{"w": 300, "h": 250}
		imp["ext"] = map[string]any{"subid": "publisher-subid"}
		r["device"].(map[string]any)["geo"] = map[string]any{"country": "PH"}
		r["user"].(map[string]any)["geo"] = map[string]any{"country": "US", "city": "Paris"}
	}
	return r
}

// Paths here are fixture locations (imp.0...), not a second rule implementation.
func chinaChange(t *testing.T, root any, path string, value any, remove bool) {
	t.Helper()
	parts := strings.Split(path, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		switch node := current.(type) {
		case map[string]any:
			current = node[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node) {
				t.Fatal("bad fixture index", path)
			}
			current = node[index]
		default:
			t.Fatal("bad fixture parent", path)
		}
	}
	last := parts[len(parts)-1]
	switch node := current.(type) {
	case map[string]any:
		if remove {
			delete(node, last)
		} else {
			node[last] = value
		}
	case []any:
		index, err := strconv.Atoi(last)
		if err != nil || index < 0 || index >= len(node) {
			t.Fatal("bad fixture index", path)
		}
		if remove {
			t.Fatal("use parent-array mutation instead")
		}
		node[index] = value
	default:
		t.Fatal("bad fixture leaf", path)
	}
}
func chinaEvaluate(t *testing.T, s *Store, id string, input map[string]any) (bool, string, error, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var req ortb.BidRequest
	if err = jsoniter.Unmarshal(raw, &req); err != nil {
		return false, "protobuf_decode", err, nil
	}
	before := proto.Clone(&req)
	impID := ""
	if len(req.GetImp()) > 0 {
		impID = req.GetImp()[0].GetId()
	}
	accepted, reason, err := s.CheckWithCache(id, NewFilterCache(&req), impID)
	if !proto.Equal(before, &req) {
		t.Fatal("filter mutated source request")
	}
	if err != nil || !accepted {
		return accepted, reason, err, nil
	}
	out, err := s.ApplyToRequest(id, &req)
	if err != nil {
		return false, "apply", err, nil
	}
	if !proto.Equal(before, &req) {
		t.Fatal("transform mutated source request")
	}
	return accepted, reason, nil, chinaObject(t, string(out))
}
func chinaAccepted(t *testing.T, s *Store, id string, in map[string]any) map[string]any {
	t.Helper()
	ok, reason, err, out := chinaEvaluate(t, s, id, in)
	if err != nil || !ok {
		t.Fatalf("required request rejected: accepted=%t reason=%s err=%v", ok, reason, err)
	}
	return out
}
func chinaRejected(t *testing.T, s *Store, id string, in map[string]any) {
	t.Helper()
	ok, reason, err, out := chinaEvaluate(t, s, id, in)
	if ok || out != nil {
		t.Fatal("invalid request reached output", reason, err)
	}
	if err == nil && reason == "" {
		t.Fatal("rejection has no reason")
	}
}

func TestChinaSuppliedProfileIsTheTestedContract(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	// Independently enumerate the posted filter counts, rather than constructing
	// the contract with the same helpers whose behavior these tests exercise.
	for _, tc := range []struct {
		id      string
		filters int
		timeout int
	}{{qaCN, 2, 500}, {qaPH, 2, 500}, {qaMX, 2, 500}, {qaVID, 15, 550}, {qaIPP, 20, 300}} {
		rules := s.Memory()[tc.id]
		if len(rules.Filters) != tc.filters {
			t.Fatal(tc.id, "missing/extra supplied filters", len(rules.Filters))
		}
		format := "POP"
		if tc.id == qaVID {
			format = "VID"
		}
		if tc.id == qaIPP {
			format = "IPP"
		}
		out := chinaAccepted(t, s, tc.id, chinaSample(t, format))
		if out["tmax"] != float64(tc.timeout) {
			t.Fatal(tc.id, "incorrect timeout", out["tmax"])
		}
	}
}

func TestChinaVideoEveryFieldBoundary(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	sample := chinaSample(t, "VID")
	numeric := []struct {
		path  string
		value float64
	}{
		{"at", 1}, {"imp.0.instl", 0}, {"imp.0.secure", 1}, {"imp.0.video.w", 340}, {"imp.0.video.h", 192}, {"imp.0.video.linearity", 1}, {"imp.0.video.sequence", 1}, {"imp.0.video.boxingallowed", 1}, {"imp.0.video.skipmin", 0}, {"imp.0.video.skipafter", 0},
	}
	for _, field := range numeric {
		t.Run(field.path, func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				value  any
				remove bool
			}{{"missing", nil, true}, {"null", nil, false}, {"below", field.value - 1, false}, {"above", field.value + 1, false}, {"string", "1", false}, {"bool", true, false}, {"object", map[string]any{}, false}, {"array", []any{}, false}} {
				t.Run(tc.name, func(t *testing.T) {
					in := chinaCopy(t, sample)
					chinaChange(t, in, field.path, tc.value, tc.remove)
					chinaRejected(t, s, qaVID, in)
				})
			}
			in := chinaCopy(t, sample)
			chinaChange(t, in, field.path, field.value, false)
			chinaAccepted(t, s, qaVID, in)
		})
	}
	for _, field := range []struct {
		path           string
		allowed, wrong any
	}{{"cur", "USD", "EUR"}, {"imp.0.video.mimes", "video/mp4", "video/webm"}, {"imp.0.video.protocols", float64(3), float64(2)}} {
		t.Run(field.path, func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				value  any
				remove bool
			}{{"missing", nil, true}, {"null", nil, false}, {"empty", []any{}, false}, {"unsupported", []any{field.wrong}, false}, {"wrong_shape", field.allowed, false}, {"wrong_item", []any{true}, false}} {
				t.Run(tc.name, func(t *testing.T) {
					in := chinaCopy(t, sample)
					chinaChange(t, in, field.path, tc.value, tc.remove)
					chinaRejected(t, s, qaVID, in)
				})
			}
			in := chinaCopy(t, sample)
			mixed := []any{field.wrong, field.allowed}
			chinaChange(t, in, field.path, mixed, false)
			out := chinaAccepted(t, s, qaVID, in)
			// isWhiteList on an array means membership, not destructive replacement.
			if !reflect.DeepEqual(chinaFixtureValue(t, out, field.path), mixed) {
				t.Fatal("advertised capabilities were rewritten")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		path   string
		value  any
		remove bool
	}{{"tag_missing", "imp.0.tagid", nil, true}, {"tag_null", "imp.0.tagid", nil, false}, {"tag_number", "imp.0.tagid", 123, false}, {"currency_missing", "imp.0.bidfloorcur", nil, true}, {"currency_null", "imp.0.bidfloorcur", nil, false}, {"currency_empty", "imp.0.bidfloorcur", "", false}, {"currency_wrong", "imp.0.bidfloorcur", "EUR", false}} {
		t.Run(tc.name, func(t *testing.T) {
			in := chinaCopy(t, sample)
			chinaChange(t, in, tc.path, tc.value, tc.remove)
			chinaRejected(t, s, qaVID, in)
		})
	}
}

func chinaFixtureValue(t *testing.T, root any, path string) any {
	t.Helper()
	current := root
	for _, part := range strings.Split(path, ".") {
		switch node := current.(type) {
		case map[string]any:
			current = node[part]
		case []any:
			i, e := strconv.Atoi(part)
			if e != nil || i < 0 || i >= len(node) {
				t.Fatal(path)
			}
			current = node[i]
		default:
			t.Fatal("bad fixture path", path)
		}
	}
	return current
}
func chinaNativeMutation(t *testing.T, in map[string]any, change func(map[string]any)) {
	t.Helper()
	native := chinaFixtureValue(t, in, "imp.0.native").(map[string]any)
	document := chinaObject(t, native["request"].(string))
	change(document)
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	native["request"] = string(encoded)
}

func TestChinaIPPEveryOuterFieldBoundary(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	sample := chinaSample(t, "IPP")
	for _, field := range []struct {
		path     string
		expected any
	}{{"at", float64(1)}, {"imp.0.instl", float64(0)}, {"imp.0.secure", float64(1)}, {"imp.0.bidfloorcur", "USD"}, {"imp.0.native.ver", "1.2"}} {
		t.Run(field.path, func(t *testing.T) {
			wrong := any("wrong")
			if n, ok := field.expected.(float64); ok {
				wrong = n + 1
			}
			for _, tc := range []struct {
				name   string
				value  any
				remove bool
			}{{"missing", nil, true}, {"null", nil, false}, {"wrong", wrong, false}, {"empty", "", false}, {"bool", false, false}, {"array", []any{}, false}, {"object", map[string]any{}, false}} {
				t.Run(tc.name, func(t *testing.T) {
					in := chinaCopy(t, sample)
					chinaChange(t, in, field.path, tc.value, tc.remove)
					chinaRejected(t, s, qaIPP, in)
				})
			}
		})
	}
	for _, value := range []any{nil, []any{}, []any{"EUR"}, "USD", []any{1}} {
		in := chinaCopy(t, sample)
		in["cur"] = value
		chinaRejected(t, s, qaIPP, in)
	}
	in := chinaCopy(t, sample)
	delete(in, "cur")
	chinaRejected(t, s, qaIPP, in)
	for _, path := range []string{"imp.0.tagid", "imp.0.native.request"} {
		for _, tc := range []struct {
			name   string
			value  any
			remove bool
		}{{"missing", nil, true}, {"null", nil, false}, {"number", 1, false}, {"object", map[string]any{}, false}, {"array", []any{}, false}} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				in := chinaCopy(t, sample)
				chinaChange(t, in, path, tc.value, tc.remove)
				chinaRejected(t, s, qaIPP, in)
			})
		}
	}
}

func TestChinaIPPEveryAssetFieldBoundary(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	sample := chinaSample(t, "IPP")
	for _, field := range []struct {
		path  string
		value float64
	}{
		{"native.assets.0.id", 100}, {"native.assets.0.required", 1}, {"native.assets.0.title.len", 80},
		{"native.assets.1.id", 101}, {"native.assets.1.required", 1}, {"native.assets.1.data.type", 1}, {"native.assets.1.data.len", 80},
		{"native.assets.2.id", 102}, {"native.assets.2.required", 1}, {"native.assets.2.img.type", 3}, {"native.assets.2.img.w", 200}, {"native.assets.2.img.h", 200}, {"native.assets.2.img.wmin", 200}, {"native.assets.2.img.hmin", 200},
	} {
		t.Run(field.path, func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				value  any
				remove bool
			}{{"missing", nil, true}, {"null", nil, false}, {"below", field.value - 1, false}, {"above", field.value + 1, false}, {"zero", float64(0), false}, {"string", "200", false}, {"bool", true, false}, {"array", []any{}, false}, {"object", map[string]any{}, false}} {
				t.Run(tc.name, func(t *testing.T) {
					in := chinaCopy(t, sample)
					chinaNativeMutation(t, in, func(doc map[string]any) { chinaChange(t, doc, field.path, tc.value, tc.remove) })
					chinaRejected(t, s, qaIPP, in)
				})
			}
		})
	}
	for _, tc := range []struct {
		name   string
		path   string
		value  any
		remove bool
	}{
		{"version_missing", "native.ver", nil, true}, {"version_null", "native.ver", nil, false}, {"version_1_1", "native.ver", "1.1", false}, {"version_number", "native.ver", 1.2, false}, {"assets_missing", "native.assets", nil, true}, {"assets_null", "native.assets", nil, false}, {"assets_empty", "native.assets", []any{}, false}, {"assets_object", "native.assets", map[string]any{}, false},
		{"title_object_missing", "native.assets.0.title", nil, true}, {"title_wrong_type", "native.assets.0.title", "bad", false}, {"data_missing", "native.assets.1.data", nil, true}, {"image_missing", "native.assets.2.img", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := chinaCopy(t, sample)
			chinaNativeMutation(t, in, func(doc map[string]any) { chinaChange(t, doc, tc.path, tc.value, tc.remove) })
			chinaRejected(t, s, qaIPP, in)
		})
	}
}

func TestChinaIPPEncodingWrapperAndStableAssetIDs(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	sample := chinaSample(t, "IPP")
	for i, raw := range []string{"", `{`, `null`, `[]`, `"double encoded"`, `{"native":{}}`, `{"native":{"ver":"1.2"}}`, chinaObject(t, chinaIPPSampleJSON)["imp"].([]any)[0].(map[string]any)["native"].(map[string]any)["request"].(string) + ` {}`} {
		t.Run(fmt.Sprintf("invalid_document_%d", i), func(t *testing.T) {
			in := chinaCopy(t, sample)
			chinaChange(t, in, "imp.0.native.request", raw, false)
			chinaRejected(t, s, qaIPP, in)
		})
	}
	in := chinaCopy(t, sample)
	chinaNativeMutation(t, in, func(doc map[string]any) {
		inner := doc["native"].(map[string]any)
		for key, value := range inner {
			doc[key] = value
		}
		delete(doc, "native")
	})
	chinaRejected(t, s, qaIPP, in)
	for _, permutation := range [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		t.Run(fmt.Sprint(permutation), func(t *testing.T) {
			in := chinaCopy(t, sample)
			chinaNativeMutation(t, in, func(doc map[string]any) {
				obj := doc["native"].(map[string]any)
				assets := obj["assets"].([]any)
				obj["assets"] = []any{assets[permutation[0]], assets[permutation[1]], assets[permutation[2]], map[string]any{"id": 777, "title": map[string]any{"len": 10}}}
			})
			native := chinaFixtureValue(t, in, "imp.0.native").(map[string]any)
			native["request"] = " \n" + native["request"].(string) + "\n "
			out := chinaAccepted(t, s, qaIPP, in)
			if chinaFixtureValue(t, out, "imp.0.native.request") != native["request"] {
				t.Fatal("embedded JSON string was rewritten")
			}
		})
	}
}

func TestChinaRequiredParentObjectsAndTimeouts(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	for _, tc := range []struct {
		format, id, parent string
		timeout            float64
	}{{"POP", qaPH, "imp", 500}, {"VID", qaVID, "imp.0.video", 550}, {"IPP", qaIPP, "imp.0.native", 300}} {
		t.Run(tc.format, func(t *testing.T) {
			base := chinaSample(t, tc.format)
			for _, v := range []any{nil, []any{}, "wrong", float64(1), map[string]any{}} {
				in := chinaCopy(t, base)
				chinaChange(t, in, tc.parent, v, false)
				chinaRejected(t, s, tc.id, in)
			}
			in := chinaCopy(t, base)
			chinaChange(t, in, tc.parent, nil, true)
			chinaRejected(t, s, tc.id, in)
			for _, v := range []any{nil, float64(0), float64(1), float64(100), tc.timeout, float64(1000)} {
				in := chinaCopy(t, base)
				in["tmax"] = v
				out := chinaAccepted(t, s, tc.id, in)
				if out["tmax"] != tc.timeout {
					t.Fatal("incorrect timeout", v, out["tmax"])
				}
			}
			in = chinaCopy(t, base)
			delete(in, "tmax")
			if out := chinaAccepted(t, s, tc.id, in); out["tmax"] != tc.timeout {
				t.Fatal(out)
			}
		})
	}
}

func TestChinaPOPFloorHTTPSAndPlacementRequirements(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	for _, campaign := range []string{qaCN, qaPH, qaMX} {
		t.Run(campaign, func(t *testing.T) {
			sample := chinaSample(t, "POP")
			for _, floor := range []any{nil, float64(0), float64(0.01), float64(2.5)} {
				in := chinaCopy(t, sample)
				chinaChange(t, in, "imp.0.bidfloor", floor, false)
				want := floor
				if floor == nil {
					want = float64(0)
				}
				out := chinaAccepted(t, s, campaign, in)
				if chinaFixtureValue(t, out, "imp.0.bidfloor") != want {
					t.Fatal("supplied minimum was overwritten or default absent", floor, out)
				}
				if chinaFixtureValue(t, out, "imp.0.tagid") != "placement-from-input-77" || chinaFixtureValue(t, out, "imp.0.secure") != float64(1) {
					t.Fatal("placement/HTTPS metadata changed")
				}
			}
			in := chinaCopy(t, sample)
			chinaChange(t, in, "imp.0.bidfloor", nil, true)
			out := chinaAccepted(t, s, campaign, in)
			if chinaFixtureValue(t, out, "imp.0.bidfloor") != float64(0) {
				t.Fatal("unset minimum was not explicit zero")
			}
			for _, tc := range []struct {
				name, path string
				value      any
				remove     bool
			}{{"missing_tag", "imp.0.tagid", nil, true}, {"null_tag", "imp.0.tagid", nil, false}, {"missing_secure", "imp.0.secure", nil, true}, {"null_secure", "imp.0.secure", nil, false}, {"not_https", "imp.0.secure", float64(0), false}, {"invalid_secure", "imp.0.secure", float64(2), false}} {
				t.Run(tc.name, func(t *testing.T) {
					in := chinaCopy(t, sample)
					chinaChange(t, in, tc.path, tc.value, tc.remove)
					chinaRejected(t, s, campaign, in)
				})
			}
		})
	}
	// A zero HTTPS flag is not upgraded or invented by the transformation stage.
	// Filters reject it first; a profile for some other campaign is unaffected.
	in := chinaSample(t, "POP")
	chinaChange(t, in, "imp.0.secure", float64(0), false)
	raw, _ := json.Marshal(in)
	var req ortb.BidRequest
	if err := jsoniter.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	out, err := s.ApplyToRequest(qaPH, &req)
	if err != nil {
		t.Fatal(err)
	}
	if chinaFixtureValue(t, chinaObject(t, string(out)), "imp.0.secure") != float64(0) {
		t.Fatal("HTTPS support fabricated")
	}
	delete(in["imp"].([]any)[0].(map[string]any), "tagid")
	raw, _ = json.Marshal(in)
	req.Reset()
	if err = jsoniter.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	out, err = s.ApplyToRequest(qaPH, &req)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := chinaFixtureValue(t, chinaObject(t, string(out)), "imp.0").(map[string]any)["tagid"]; exists {
		t.Fatal("missing placement id was invented")
	}
}

func TestChinaVideoIPPBidfloorMetadataAndTestFlagAreNotFabricated(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	for _, tc := range []struct{ format, id string }{{"VID", qaVID}, {"IPP", qaIPP}} {
		t.Run(tc.format, func(t *testing.T) {
			base := chinaSample(t, tc.format)
			for _, floor := range []any{nil, float64(0), float64(0.01), float64(2.5)} {
				in := chinaCopy(t, base)
				chinaChange(t, in, "imp.0.bidfloor", floor, false)
				out := chinaAccepted(t, s, tc.id, in)
				got, exists := chinaFixtureValue(t, out, "imp.0").(map[string]any)["bidfloor"]
				if floor == nil {
					if exists {
						t.Fatal("example floor invented", got)
					}
				} else if got != floor {
					t.Fatal("actual minimum changed", got, floor)
				}
			}
			in := chinaCopy(t, base)
			delete(in, "test")
			chinaChange(t, in, "imp.0.bidfloor", nil, true)
			out := chinaAccepted(t, s, tc.id, in)
			if _, present := out["test"]; present {
				t.Fatal("production request was forced into test mode")
			}
			if _, present := chinaFixtureValue(t, out, "imp.0").(map[string]any)["bidfloor"]; present {
				t.Fatal("example floor forced")
			}
			if out["id"] != in["id"] || chinaFixtureValue(t, out, "device.ip") != "192.0.2.1" || chinaFixtureValue(t, out, "user.id") != "test-user-001" || chinaFixtureValue(t, out, "site.id") != "test-site-001" {
				t.Fatal("request identity changed")
			}
			for _, key := range []string{"timestamp", "event_time", "created_at"} {
				if _, present := out[key]; present {
					t.Fatal("request timestamp fabricated", key)
				}
			}
		})
	}
}

// Only this future-format fixture carries forward the already existing legacy
// deviceType -> devicetype rule. The supplied JSON literal stays unchanged;
// strict tests below separately check that literal for missing requirements.
func chinaWireProfile(t *testing.T) map[string]any {
	t.Helper()
	cfg := chinaProfiles(t)
	for _, id := range []string{qaVID, qaIPP} {
		cfg[id].(map[string]any)["rename"] = []any{map[string]any{"field": "device.deviceType", "to_field": "devicetype"}}
	}
	return cfg
}
func chinaSerializedRequest(t *testing.T, s *Store, id string, input map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var req ortb.BidRequest
	if err = jsoniter.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	before := proto.Clone(&req)
	ok, reason, err := s.CheckWithCache(id, NewFilterCache(&req), req.GetImp()[0].GetId())
	if err != nil || !ok {
		t.Fatal(ok, reason, err)
	}
	wire, err := s.ApplyToRequest(id, &req)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, &req) {
		t.Fatal("source request changed")
	}
	return wire
}
func TestChinaFullVideoIPPSamplesOnSerializedWire(t *testing.T) {
	s := chinaStore(t, chinaWireProfile(t))
	for _, tc := range []struct{ format, id string }{{"VID", qaVID}, {"IPP", qaIPP}} {
		t.Run(tc.format, func(t *testing.T) {
			in := chinaSample(t, tc.format)
			wire := chinaSerializedRequest(t, s, tc.id, in)
			out := chinaObject(t, string(wire))
			if !reflect.DeepEqual(in, out) {
				t.Fatalf("partner sample changed on wire:\nexpected=%s\nactual=%s", mustChinaJSON(t, in), wire)
			}
			if tc.format == "IPP" {
				if _, ok := chinaFixtureValue(t, out, "imp.0.native.request").(string); !ok {
					t.Fatal("native.request is not a JSON-encoded string")
				}
			}
			received := make(chan []byte, 1)
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				received <- body
				w.WriteHeader(204)
			}))
			defer endpoint.Close()
			response, err := endpoint.Client().Post(endpoint.URL, "application/json", bytes.NewReader(wire))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 204 {
				t.Fatal(response.StatusCode)
			}
			got := <-received
			if !bytes.Equal(got, wire) {
				t.Fatal("wire body altered in HTTP transport")
			}
		})
	}
}
func mustChinaJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestChinaCampaignAndImpressionIsolation(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	unknown := "99999999-9999-4999-8999-999999999999"
	input := chinaSample(t, "POP")
	raw := mustChinaJSON(t, input)
	var req ortb.BidRequest
	if err := jsoniter.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	expected, err := jsoniter.Marshal(&req)
	if err != nil {
		t.Fatal(err)
	}
	out := chinaAccepted(t, s, unknown, input)
	if !reflect.DeepEqual(out, chinaObject(t, string(expected))) {
		t.Fatal("54Ads rules leaked into an unrelated campaign")
	}
	// Combined request retains distinct per-impression video/native capabilities.
	video, ipp := chinaSample(t, "VID"), chinaSample(t, "IPP")
	mixed := chinaCopy(t, video)
	mixed["imp"] = append(mixed["imp"].([]any), ipp["imp"].([]any)[0])
	raw = mustChinaJSON(t, mixed)
	req.Reset()
	if err = jsoniter.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	before := proto.Clone(&req)
	cache := NewFilterCache(&req)
	for _, tc := range []struct {
		id, imp string
		want    bool
	}{{qaVID, "video-test-imp-001", true}, {qaVID, "push-test-imp-001", false}, {qaIPP, "push-test-imp-001", true}, {qaIPP, "video-test-imp-001", false}} {
		ok, _, err := s.CheckWithCache(tc.id, cache, tc.imp)
		if err != nil || ok != tc.want {
			t.Fatal(tc, ok, err)
		}
	}
	if !proto.Equal(before, &req) {
		t.Fatal("shared request mutated")
	}
	// A failed first impression cannot poison the valid sibling, or vice versa.
	for _, firstBad := range []bool{false, true} {
		in := chinaSample(t, "IPP")
		bad := chinaCopy(t, in)["imp"].([]any)[0].(map[string]any)
		bad["id"] = "bad"
		bad["native"].(map[string]any)["request"] = "{bad"
		good := in["imp"].([]any)[0]
		if firstBad {
			in["imp"] = []any{bad, good}
		} else {
			in["imp"] = []any{good, bad}
		}
		raw = mustChinaJSON(t, in)
		var r ortb.BidRequest
		if err = jsoniter.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		cache := NewFilterCache(&r)
		for _, impID := range []string{"bad", "push-test-imp-001", "bad", "push-test-imp-001"} {
			ok, _, err := s.CheckWithCache(qaIPP, cache, impID)
			if impID == "bad" {
				if ok || !errors.Is(err, ErrNativeFilter) {
					t.Fatal(ok, err)
				}
			} else if !ok || err != nil {
				t.Fatal(ok, err)
			}
		}
	}
}

// Frozen test-only pre-rtbrules oracle from the removed proj251 implementation.
// Function names alone are prefixed; its behavior is not adapted to the new rules.
func chinaLegacyNormalize(payload map[string]any, pop bool, countryReplacements map[string]string) {
	if payload == nil {
		return
	}
	payload["tmax"] = 500

	deviceCountry := ""
	if rawDevice, ok := payload["device"]; ok {
		if device, ok := rawDevice.(map[string]any); ok {
			if deviceType, exists := device["deviceType"]; exists {
				delete(device, "deviceType")
				device["devicetype"] = deviceType
			}
			deviceCountry = chinaLegacyGeo(device["geo"], countryReplacements)
		}
	}

	user, _ := payload["user"].(map[string]any)
	if user != nil {
		chinaLegacyGeo(user["geo"], countryReplacements)
	}
	// Only a country actually replaced for this campaign may fill a missing
	// user country. Preserve existing user/geo fields and a nonempty country.
	if deviceCountry != "" {
		if user == nil {
			user = make(map[string]any)
			payload["user"] = user
		}
		geo, _ := user["geo"].(map[string]any)
		if geo == nil {
			geo = make(map[string]any)
			user["geo"] = geo
		}
		country, _ := geo["country"].(string)
		if strings.TrimSpace(country) == "" {
			geo["country"] = deviceCountry
		}
	}

	if rawSite, ok := payload["site"]; ok {
		if site, ok := rawSite.(map[string]any); ok {
			domain, _ := site["domain"].(string)
			page, _ := site["page"].(string)
			host := chinaLegacyHostname(domain)
			if host == "" {
				host = chinaLegacyHostname(page)
			}
			if host != "" {
				site["domain"] = host
			}
			if fullPage := chinaLegacyFullPage(page, host); fullPage != "" {
				site["page"] = fullPage
			}
		}
	}

	if rawImps, ok := payload["imp"].([]any); ok {
		for _, rawImp := range rawImps {
			imp, ok := rawImp.(map[string]any)
			if !ok {
				continue
			}
			ext, _ := imp["ext"].(map[string]any)
			if ext == nil {
				ext = make(map[string]any)
			}
			ext["type"] = "pop"
			imp["ext"] = ext
			if pop {
				delete(imp, "banner")
			}
		}
	}
}

// Return the replacement only when this campaign's rule matched. Already
// normalized and unrelated countries must not trigger the user-country fallback.
func chinaLegacyGeo(rawGeo any, countryReplacements map[string]string) string {
	geo, ok := rawGeo.(map[string]any)
	if !ok {
		return ""
	}
	country, _ := geo["country"].(string)
	replacement, ok := countryReplacements[strings.ToUpper(strings.TrimSpace(country))]
	if !ok {
		return ""
	}
	geo["country"] = replacement
	return replacement
}

func chinaLegacyHostname(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	candidate := raw
	if !strings.Contains(candidate, "://") {
		candidate = "//" + strings.TrimPrefix(candidate, "//")
	}
	u, err := url.Parse(candidate)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(u.Hostname())
}

func chinaLegacyFullPage(rawPage, host string) string {
	page := strings.TrimSpace(rawPage)
	if page == "" {
		if host == "" {
			return ""
		}
		return "https://" + host + "/"
	}
	if u, err := url.Parse(page); err == nil && u.IsAbs() && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return page
	}
	if strings.HasPrefix(page, "//") {
		return "https:" + page
	}
	if strings.HasPrefix(page, "/") && host != "" {
		return "https://" + host + page
	}
	if !strings.Contains(page, "://") {
		return "https://" + strings.TrimPrefix(page, "//")
	}
	return page
}

// Independent ISO-3166-1 corpus from pycountry databases/iso3166-1.json,
// SHA256 f01b812b57fba9f31ff621bf33e7c7570a01964dbeb5be2167e94decf538c89f.
// Frozen test data; not generated from isoAlpha3 and requires no network at test time.
const chinaISOReference = `
AD AND
AE ARE
AF AFG
AG ATG
AI AIA
AL ALB
AM ARM
AO AGO
AQ ATA
AR ARG
AS ASM
AT AUT
AU AUS
AW ABW
AX ALA
AZ AZE
BA BIH
BB BRB
BD BGD
BE BEL
BF BFA
BG BGR
BH BHR
BI BDI
BJ BEN
BL BLM
BM BMU
BN BRN
BO BOL
BQ BES
BR BRA
BS BHS
BT BTN
BV BVT
BW BWA
BY BLR
BZ BLZ
CA CAN
CC CCK
CD COD
CF CAF
CG COG
CH CHE
CI CIV
CK COK
CL CHL
CM CMR
CN CHN
CO COL
CR CRI
CU CUB
CV CPV
CW CUW
CX CXR
CY CYP
CZ CZE
DE DEU
DJ DJI
DK DNK
DM DMA
DO DOM
DZ DZA
EC ECU
EE EST
EG EGY
EH ESH
ER ERI
ES ESP
ET ETH
FI FIN
FJ FJI
FK FLK
FM FSM
FO FRO
FR FRA
GA GAB
GB GBR
GD GRD
GE GEO
GF GUF
GG GGY
GH GHA
GI GIB
GL GRL
GM GMB
GN GIN
GP GLP
GQ GNQ
GR GRC
GS SGS
GT GTM
GU GUM
GW GNB
GY GUY
HK HKG
HM HMD
HN HND
HR HRV
HT HTI
HU HUN
ID IDN
IE IRL
IL ISR
IM IMN
IN IND
IO IOT
IQ IRQ
IR IRN
IS ISL
IT ITA
JE JEY
JM JAM
JO JOR
JP JPN
KE KEN
KG KGZ
KH KHM
KI KIR
KM COM
KN KNA
KP PRK
KR KOR
KW KWT
KY CYM
KZ KAZ
LA LAO
LB LBN
LC LCA
LI LIE
LK LKA
LR LBR
LS LSO
LT LTU
LU LUX
LV LVA
LY LBY
MA MAR
MC MCO
MD MDA
ME MNE
MF MAF
MG MDG
MH MHL
MK MKD
ML MLI
MM MMR
MN MNG
MO MAC
MP MNP
MQ MTQ
MR MRT
MS MSR
MT MLT
MU MUS
MV MDV
MW MWI
MX MEX
MY MYS
MZ MOZ
NA NAM
NC NCL
NE NER
NF NFK
NG NGA
NI NIC
NL NLD
NO NOR
NP NPL
NR NRU
NU NIU
NZ NZL
OM OMN
PA PAN
PE PER
PF PYF
PG PNG
PH PHL
PK PAK
PL POL
PM SPM
PN PCN
PR PRI
PS PSE
PT PRT
PW PLW
PY PRY
QA QAT
RE REU
RO ROU
RS SRB
RU RUS
RW RWA
SA SAU
SB SLB
SC SYC
SD SDN
SE SWE
SG SGP
SH SHN
SI SVN
SJ SJM
SK SVK
SL SLE
SM SMR
SN SEN
SO SOM
SR SUR
SS SSD
ST STP
SV SLV
SX SXM
SY SYR
SZ SWZ
TC TCA
TD TCD
TF ATF
TG TGO
TH THA
TJ TJK
TK TKL
TL TLS
TM TKM
TN TUN
TO TON
TR TUR
TT TTO
TV TUV
TW TWN
TZ TZA
UA UKR
UG UGA
UM UMI
US USA
UY URY
UZ UZB
VA VAT
VC VCT
VE VEN
VG VGB
VI VIR
VN VNM
VU VUT
WF WLF
WS WSM
YE YEM
YT MYT
ZA ZAF
ZM ZMB
ZW ZWE
`

func TestChinaBannerAllISOCountriesAndOwnerIsolation(t *testing.T) {
	cfg := chinaProfiles(t)
	// Test-only campaign: user did not supply the real Banner campaign UUID.
	// ISO mode is explicit and does not leak into POP/campaign country maps.
	cfg[chinaBannerTestID] = map[string]any{"replace": []any{map[string]any{"field": "device.geo.country", "mode": "iso"}, map[string]any{"field": "user.geo.country", "mode": "iso"}}}
	s := chinaStore(t, cfg)
	entries := strings.Fields(chinaISOReference)
	if len(entries) != 249*2 {
		t.Fatal("independent ISO corpus incomplete", len(entries))
	}
	if len(isoAlpha3) != 249 {
		t.Fatal("runtime ISO table incomplete or has unexpected entries", len(isoAlpha3))
	}
	for i := 0; i < len(entries); i += 2 {
		from, to := entries[i], entries[i+1]
		t.Run(from, func(t *testing.T) {
			for _, raw := range []string{from, strings.ToLower(from), "  " + strings.ToLower(from) + "  "} {
				input := chinaSample(t, "POP")
				chinaChange(t, input, "device.geo.country", raw, false)
				chinaChange(t, input, "user.geo.country", raw, false)
				out := chinaAccepted(t, s, chinaBannerTestID, input)
				if chinaFixtureValue(t, out, "device.geo.country") != to || chinaFixtureValue(t, out, "user.geo.country") != to {
					t.Fatal("ISO mapping incorrect", from, to, out)
				}
				if chinaFixtureValue(t, out, "user.geo.city") != "Paris" || chinaFixtureValue(t, out, "user.id") != "test-user-001" || chinaFixtureValue(t, out, "imp.0.tagid") != "placement-from-input-77" {
					t.Fatal("unrelated fields changed")
				}
			}
		})
	}
	// Explicit customer examples plus different independent device/user countries.
	in := chinaSample(t, "POP")
	chinaChange(t, in, "device.geo.country", "JM", false)
	chinaChange(t, in, "user.geo.country", "JO", false)
	out := chinaAccepted(t, s, chinaBannerTestID, in)
	if chinaFixtureValue(t, out, "device.geo.country") != "JAM" || chinaFixtureValue(t, out, "user.geo.country") != "JOR" {
		t.Fatal("JM/JO customer examples failed")
	}
	for _, country := range []string{"JAM", "JOR", "ZZ", "", "  "} {
		in := chinaSample(t, "POP")
		chinaChange(t, in, "device.geo.country", country, false)
		chinaChange(t, in, "user.geo.country", country, false)
		out := chinaAccepted(t, s, chinaBannerTestID, in)
		if chinaFixtureValue(t, out, "device.geo.country") != country || chinaFixtureValue(t, out, "user.geo.country") != country {
			t.Fatal("unmapped value invented/modified", country)
		}
	}
	for _, path := range []string{"device.geo.country", "user.geo.country", "device.geo", "user.geo", "device", "user"} {
		for _, remove := range []bool{true, false} {
			in := chinaSample(t, "POP")
			chinaChange(t, in, path, nil, remove)
			out := chinaAccepted(t, s, chinaBannerTestID, in)
			// ISO replacement is existing-field-only, never a source of fake countries.
			if parent, ok := out[strings.Split(path, ".")[0]].(map[string]any); ok {
				if geo, ok := parent["geo"].(map[string]any); ok {
					if value, present := geo["country"]; present && value != nil && strings.HasPrefix(path, strings.Split(path, ".")[0]) {
						t.Fatal("absent ISO country created", path, value)
					}
				}
			}
		}
	}
	// The POP country map does not acquire all-country ISO conversion globally.
	out = chinaAccepted(t, s, qaCN, func() map[string]any {
		r := chinaSample(t, "POP")
		chinaChange(t, r, "device.geo.country", "JM", false)
		return r
	}())
	if chinaFixtureValue(t, out, "device.geo.country") != "JM" {
		t.Fatal("banner ISO mode leaked into POP")
	}
}

func chinaLegacyExpected(t *testing.T, input map[string]any, countries map[string]string, pop bool) map[string]any {
	t.Helper()
	var req ortb.BidRequest
	if err := jsoniter.Unmarshal(mustChinaJSON(t, input), &req); err != nil {
		t.Fatal(err)
	}
	raw, err := jsoniter.Marshal(&req)
	if err != nil {
		t.Fatal(err)
	}
	expected := chinaObject(t, string(raw))
	chinaLegacyNormalize(expected, pop, countries)
	return expected
}
func TestChinaLegacyExactGoldenCorpus(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	campaigns := []struct {
		id, from, to string
		countries    map[string]string
	}{
		{qaCN, "CN", "CHN", map[string]string{"CN": "CHN"}},
		{qaPH, "PH", "PHL", map[string]string{"PH": "PHL"}},
		{qaMX, "MX", "MEX", map[string]string{"MX": "MEX", "PE": "PER"}},
		{qaMX, "PE", "PER", map[string]string{"MX": "MEX", "PE": "PER"}},
	}
	sites := []struct{ domain, page string }{{"https://example.com/path", "/landing"}, {"example.com", "https://example.com/video"}, {"", "https://example.com/path?q=1#part"}, {"example.com", ""}, {"example.com:8443", "/landing"}, {"//example.com/path", "//example.com/landing"}, {"", ""}, {"http://[bad", "https://fallback.example/x"}, {"example.com", "ftp://example.com/x"}}
	for _, campaign := range campaigns {
		for _, country := range []string{campaign.from, strings.ToLower(campaign.from), "  " + strings.ToLower(campaign.from) + "  "} {
			for _, userState := range []string{"missing", "missing_geo", "missing_country", "empty_country", "whitespace_country", "already_normalized"} {
				for i, site := range sites {
					t.Run(campaign.id+"/"+country+"/"+userState+"/site"+strconv.Itoa(i), func(t *testing.T) {
						in := chinaSample(t, "POP")
						chinaChange(t, in, "device.geo.country", country, false)
						in["tmax"] = 100
						chinaChange(t, in, "site.domain", site.domain, false)
						chinaChange(t, in, "site.page", site.page, false)
						switch userState {
						case "missing":
							delete(in, "user")
						case "missing_geo":
							delete(in["user"].(map[string]any), "geo")
						case "missing_country":
							delete(in["user"].(map[string]any)["geo"].(map[string]any), "country")
						case "empty_country":
							chinaChange(t, in, "user.geo.country", "", false)
						case "whitespace_country":
							chinaChange(t, in, "user.geo.country", "  ", false)
						case "already_normalized":
							chinaChange(t, in, "user.geo.country", campaign.to, false)
						}
						expected := chinaLegacyExpected(t, in, campaign.countries, true)
						out := chinaAccepted(t, s, campaign.id, in)
						if !bytes.Equal(mustChinaJSON(t, expected), mustChinaJSON(t, out)) {
							t.Fatalf("legacy behavior changed:\nlegacy=%s\nrules=%s", mustChinaJSON(t, expected), mustChinaJSON(t, out))
						}
					})
				}
			}
		}
	}
	// Missing/empty device parents do not prevent independent user GEO conversion.
	for _, tc := range campaigns {
		for _, path := range []string{"device", "device.geo", "device.geo.country"} {
			in := chinaSample(t, "POP")
			chinaChange(t, in, path, nil, true)
			chinaChange(t, in, "user.geo.country", tc.from, false)
			expected := chinaLegacyExpected(t, in, tc.countries, true)
			out := chinaAccepted(t, s, tc.id, in)
			if !bytes.Equal(mustChinaJSON(t, expected), mustChinaJSON(t, out)) {
				t.Fatal("missing-device legacy behavior changed", tc, path)
			}
		}
	}
}

func TestChinaLegacyIntentionalCopyAndFloorDifferencesAreExplicit(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	for _, tc := range []struct {
		name, device, user, oldUser, newUser string
		userPresent                          bool
	}{
		{"populated_target", "PH", "US", "US", "PHL", true},
		{"unmatched_source", "US", "US", "US", "US", false},
		{"already_alpha3", "PHL", "US", "US", "PHL", true},
		{"empty_source", "", "US", "US", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := chinaSample(t, "POP")
			chinaChange(t, in, "device.geo.country", tc.device, false)
			if tc.userPresent {
				chinaChange(t, in, "user.geo.country", tc.user, false)
			} else {
				delete(in["user"].(map[string]any), "geo")
			}
			legacy := chinaLegacyExpected(t, in, map[string]string{"PH": "PHL"}, true)
			out := chinaAccepted(t, s, qaPH, in)
			if tc.userPresent {
				if chinaFixtureValue(t, legacy, "user.geo.country") != tc.oldUser {
					t.Fatal("frozen legacy oracle changed")
				}
			} else if _, present := legacy["user"].(map[string]any)["geo"]; present {
				t.Fatal("legacy did not preserve absent GEO")
			}
			if chinaFixtureValue(t, out, "user.geo.country") != tc.newUser {
				t.Fatal("current copy_to contract changed", out)
			}
		})
	}
	in := chinaSample(t, "POP")
	chinaChange(t, in, "imp.0.bidfloor", nil, true)
	legacy := chinaLegacyExpected(t, in, map[string]string{"PH": "PHL"}, true)
	if _, present := chinaFixtureValue(t, legacy, "imp.0").(map[string]any)["bidfloor"]; present {
		t.Fatal("legacy oracle fabricated default floor")
	}
	out := chinaAccepted(t, s, qaPH, in)
	if chinaFixtureValue(t, out, "imp.0.bidfloor") != float64(0) {
		t.Fatal("new explicit unset floor missing")
	}
	// An old missing-tag request could be normalized, but the new contract rejects it.
	chinaChange(t, in, "imp.0.tagid", nil, true)
	chinaRejected(t, s, qaPH, in)
}

func TestChinaVideoIPPRealSourceMetadataIsPreserved(t *testing.T) {
	s := chinaStore(t, chinaWireProfile(t))
	for _, tc := range []struct{ format, id string }{{"VID", qaVID}, {"IPP", qaIPP}} {
		t.Run(tc.format, func(t *testing.T) {
			base := chinaSample(t, tc.format)
			for _, field := range []struct {
				path  string
				value any
			}{
				{"id", "actual-source-request-987"}, {"test", float64(0)}, {"imp.0.id", "source-impression-432"}, {"imp.0.tagid", "actual-source-placement-55"},
				{"site.id", "actual-source-site"}, {"site.name", "Actual Site"}, {"site.domain", "source.example"}, {"site.page", "https://source.example/page?q=keep#part"}, {"site.publisher.id", "source-publisher"},
				{"device.ip", "198.51.100.27"}, {"device.ua", "actual original user agent"}, {"device.devicetype", float64(1)}, {"device.os", "original-OS"}, {"device.osv", "original-version"}, {"device.js", float64(0)}, {"device.language", "zh"}, {"user.id", "actual-source-user"},
			} {
				t.Run(field.path, func(t *testing.T) {
					in := chinaCopy(t, base)
					chinaChange(t, in, field.path, field.value, false)
					out := chinaAccepted(t, s, tc.id, in)
					if !bytes.Equal(mustChinaJSON(t, in), mustChinaJSON(t, out)) {
						t.Fatal("a placeholder or some other field replaced actual source data", field.path, string(mustChinaJSON(t, out)))
					}
				})
			}
		})
	}
}

// The literal historical profile above remains unchanged for its regression
// corpus. These tests use the later agreed configuration: copy the existing
// site.id in the filter, and put the device-key rename in ALL. No production
// configuration is edited by this test fixture.
func chinaCurrentProfiles(t *testing.T) map[string]any {
	t.Helper()
	config := chinaProfiles(t)
	config["ALL"] = map[string]any{"rename": []any{map[string]any{"field": "device.deviceType", "to_field": "devicetype"}}}
	for _, block := range config {
		rules := block.(map[string]any)
		filters, _ := rules["filters"].([]any)
		for i, entry := range filters {
			filter := entry.(map[string]any)
			if filter["field"] == "imp.tagid" {
				filters[i] = map[string]any{"field": "site.id", "exists": true, "copy_to": "imp.tagid"}
			}
		}
	}
	// The rename is shared through ALL, rather than duplicated by campaign.
	delete(config[qaCN+","+qaPH+","+qaMX].(map[string]any), "rename")
	return config
}

func TestChinaCurrentContractRetainsOtherPartnerConditions(t *testing.T) {
	historical := chinaStore(t, chinaProfiles(t)).Memory()
	current := chinaStore(t, chinaCurrentProfiles(t)).Memory()
	for _, tc := range []struct {
		id      string
		filters int
	}{{qaCN, 2}, {qaPH, 2}, {qaMX, 2}, {qaVID, 15}, {qaIPP, 20}} {
		old, got := historical[tc.id], current[tc.id]
		if len(got.Filters) != tc.filters || len(old.Filters) != tc.filters {
			t.Fatal("missing or added partner conditions", tc.id)
		}
		copied := 0
		for i, expected := range old.Filters {
			if expected.Field == "imp.tagid" {
				expected.Field = "site.id"
				expected.CopyTo = "imp.tagid"
				copied++
			}
			if !reflect.DeepEqual(got.Filters[i], expected) {
				t.Fatal("another partner filter changed", tc.id, i, got.Filters[i], expected)
			}
		}
		if copied != 1 || !reflect.DeepEqual(got.Add, old.Add) || !reflect.DeepEqual(got.Replace, old.Replace) || !reflect.DeepEqual(got.Delete, old.Delete) {
			t.Fatal("partner transforms changed", tc.id)
		}
		if !reflect.DeepEqual(got.Rename, []Rename{{Field: "device.deviceType", ToField: "devicetype"}}) {
			t.Fatal("global rename missing or duplicated", tc.id, got.Rename)
		}
	}
}

func TestChinaCurrentContractCopiesSiteIDToPlacement(t *testing.T) {
	s := chinaStore(t, chinaCurrentProfiles(t))
	for _, tc := range []struct{ format, id string }{{"POP", qaCN}, {"POP", qaPH}, {"POP", qaMX}, {"VID", qaVID}, {"IPP", qaIPP}} {
		for _, tag := range []struct {
			name   string
			value  any
			remove bool
		}{{"empty", "", false}, {"whitespace", "   ", false}, {"null", nil, false}, {"missing", nil, true}, {"existing", "old-placement", false}} {
			t.Run(tc.id+"/"+tag.name, func(t *testing.T) {
				in := chinaSample(t, tc.format)
				chinaChange(t, in, "site.id", "actual-site-id-from-input", false)
				chinaChange(t, in, "imp.0.tagid", tag.value, tag.remove)
				second := chinaCopy(t, in["imp"].([]any)[0].(map[string]any))
				second["id"] = "second-impression"
				second["tagid"] = "another-existing-placement"
				in["imp"] = append(in["imp"].([]any), second)
				out := chinaAccepted(t, s, tc.id, in)
				if chinaFixtureValue(t, out, "site.id") != "actual-site-id-from-input" {
					t.Fatal("copy changed source site.id")
				}
				imps := out["imp"].([]any)
				if len(imps) != 2 {
					t.Fatal("copy changed impression count")
				}
				for _, entry := range imps {
					if entry.(map[string]any)["tagid"] != "actual-site-id-from-input" {
						t.Fatal("tagid was not copied from the existing site.id", entry)
					}
				}
			})
		}
		for _, missing := range []struct {
			name, path string
			remove     bool
		}{{"missing_id", "site.id", true}, {"null_id", "site.id", false}, {"missing_site", "site", true}, {"null_site", "site", false}} {
			t.Run(tc.id+"/"+missing.name, func(t *testing.T) {
				in := chinaSample(t, tc.format)
				chinaChange(t, in, missing.path, nil, missing.remove)
				accepted, reason, err, out := chinaEvaluate(t, s, tc.id, in)
				if accepted || out != nil || err != nil || reason != "rtb_rule_filter_site.id" {
					t.Fatal("missing source must fail the site.id filter", accepted, reason, err, out)
				}
			})
		}
	}
}

func TestChinaCurrentContractUsesGlobalPartnerDeviceKey(t *testing.T) {
	s := chinaStore(t, chinaCurrentProfiles(t))
	for _, tc := range []struct{ format, id string }{{"POP", qaCN}, {"POP", qaPH}, {"POP", qaMX}, {"VID", qaVID}, {"IPP", qaIPP}} {
		t.Run(tc.id, func(t *testing.T) {
			out := chinaAccepted(t, s, tc.id, chinaSample(t, tc.format))
			device := out["device"].(map[string]any)
			if device["devicetype"] != float64(4) {
				t.Errorf("ALL rename did not preserve device type for %s: %v", tc.format, device)
			}
			if _, present := device["deviceType"]; present {
				t.Error("camelCase deviceType still present on partner wire")
			}
		})
	}
}

func TestChinaCurrentContractPreservesExistsSemantics(t *testing.T) {
	s := chinaStore(t, chinaCurrentProfiles(t))
	for _, tc := range []struct{ format, id string }{{"POP", qaCN}, {"POP", qaPH}, {"POP", qaMX}, {"VID", qaVID}, {"IPP", qaIPP}} {
		for _, siteID := range []string{"", "   "} {
			in := chinaSample(t, tc.format)
			chinaChange(t, in, "site.id", siteID, false)
			out := chinaAccepted(t, s, tc.id, in)
			if chinaFixtureValue(t, out, "imp.0.tagid") != siteID {
				t.Fatal("exists/copy_to must not trim, invent or validate the source ID", tc.id, siteID, out)
			}
		}
	}
}

func TestChinaNativeWrapperAndCrossCampaignErrorIsolation(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	base := chinaSample(t, "IPP")
	for _, version := range []any{"", "1.2 ", false, []any{}, map[string]any{}} {
		in := chinaCopy(t, base)
		chinaNativeMutation(t, in, func(doc map[string]any) { chinaChange(t, doc, "native.ver", version, false) })
		chinaRejected(t, s, qaIPP, in)
	}
	for _, wrapper := range []any{nil, []any{}, "1.2", 1} {
		in := chinaCopy(t, base)
		chinaNativeMutation(t, in, func(doc map[string]any) { doc["native"] = wrapper })
		chinaRejected(t, s, qaIPP, in)
	}
	in := chinaCopy(t, base)
	original := chinaFixtureValue(t, in, "imp.0.native.request").(string)
	double, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	chinaChange(t, in, "imp.0.native.request", string(double), false)
	chinaRejected(t, s, qaIPP, in)
	// One source impression advertises both formats; bad native JSON must not
	// invalidate the same impression's otherwise valid VIDEO campaign check.
	in = chinaSample(t, "VID")
	chinaChange(t, in, "imp.0.native", map[string]any{"ver": "1.2", "request": "{bad"}, false)
	var req ortb.BidRequest
	if err = jsoniter.Unmarshal(mustChinaJSON(t, in), &req); err != nil {
		t.Fatal(err)
	}
	cache := NewFilterCache(&req)
	impID := req.GetImp()[0].GetId()
	for _, id := range []string{qaIPP, qaVID, qaIPP, qaVID} {
		ok, _, err := s.CheckWithCache(id, cache, impID)
		if id == qaIPP {
			if ok || !errors.Is(err, ErrNativeFilter) {
				t.Fatal("malformed Native not distinguished", ok, err)
			}
		} else if !ok || err != nil {
			t.Fatal("Native error poisoned VIDEO", ok, err)
		}
	}
}

func TestChinaSiblingValuesCannotSatisfyAnotherImpression(t *testing.T) {
	s := chinaStore(t, chinaProfiles(t))
	for _, tc := range []struct {
		format, id, path string
		wrong            any
	}{{"VID", qaVID, "imp.0.video.w", float64(339)}, {"VID", qaVID, "imp.0.video.skipmin", float64(1)}, {"IPP", qaIPP, "imp.0.native.ver", "1.1"}} {
		in := chinaSample(t, tc.format)
		sibling := chinaCopy(t, in)["imp"].([]any)[0].(map[string]any)
		sibling["id"] = "valid-sibling"
		chinaChange(t, in, tc.path, tc.wrong, false)
		in["imp"] = append(in["imp"].([]any), sibling)
		var req ortb.BidRequest
		if err := jsoniter.Unmarshal(mustChinaJSON(t, in), &req); err != nil {
			t.Fatal(err)
		}
		cache := NewFilterCache(&req)
		if ok, _, err := s.CheckWithCache(tc.id, cache, req.GetImp()[0].GetId()); ok || err != nil {
			t.Fatal("sibling value satisfied a failing impression", tc, ok, err)
		}
		if ok, _, err := s.CheckWithCache(tc.id, cache, "valid-sibling"); !ok || err != nil {
			t.Fatal("invalid sibling rejected valid impression", tc, ok, err)
		}
	}
}

func FuzzChinaNativeLiteralContractAndCache(f *testing.F) {
	cfgPath := filepath.Join(f.TempDir(), "54ads.json")
	if err := os.WriteFile(cfgPath, []byte(chinaPartnerProfilesJSON), 0600); err != nil {
		f.Fatal(err)
	}
	store, err := NewStore(cfgPath)
	if err != nil {
		f.Fatal(err)
	}
	var sample map[string]any
	if err = json.Unmarshal([]byte(chinaIPPSampleJSON), &sample); err != nil {
		f.Fatal(err)
	}
	valid := sample["imp"].([]any)[0].(map[string]any)["native"].(map[string]any)["request"].(string)
	for _, seed := range []string{valid, `{`, `null`, `[]`, `{"native":{"ver":"1.2","assets":[]}}`, `{"native":{"ver":"1.2","assets":[{"id":"100"}]}}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, embedded string) {
		in := chinaSample(t, "IPP")
		chinaChange(t, in, "imp.0.native.request", embedded, false)
		var req ortb.BidRequest
		if err := jsoniter.Unmarshal(mustChinaJSON(t, in), &req); err != nil {
			t.Fatal(err)
		}
		before := proto.Clone(&req)
		cache := NewFilterCache(&req)
		imp := req.GetImp()[0].GetId()
		ok, reason, err := store.CheckWithCache(qaIPP, cache, imp)
		for _, again := range []*FilterCache{cache, NewFilterCache(&req)} {
			same, sameReason, sameErr := store.CheckWithCache(qaIPP, again, imp)
			if same != ok || sameReason != reason || (sameErr == nil) != (err == nil) || errors.Is(sameErr, ErrNativeFilter) != errors.Is(err, ErrNativeFilter) {
				t.Fatal("cache and fresh native parsing disagree", same, sameReason, sameErr, ok, reason, err)
			}
		}
		if err != nil && !errors.Is(err, ErrNativeFilter) {
			t.Fatal("embedded Native error not classified", err)
		}
		if !ok && err == nil && reason == "" {
			t.Fatal("mismatch has no reason")
		}
		if ok {
			out, err := store.ApplyToRequest(qaIPP, &req)
			if err != nil {
				t.Fatal(err)
			}
			if chinaFixtureValue(t, chinaObject(t, string(out)), "imp.0.native.request") != embedded {
				t.Fatal("valid embedded request altered")
			}
		}
		if !proto.Equal(before, &req) {
			t.Fatal("filter/application mutated the source")
		}
	})
}
