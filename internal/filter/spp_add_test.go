package filter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/uuid"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"google.golang.org/protobuf/proto"
)

func TestSPPCIDAddAndRandom(t *testing.T) {
	const domain = "ssp.example"
	const rules = `{"spps":{"static.example":{"add":[{"field":"bid.cid","value":"fixed"}]},
	"ssp.example":{"add_random":[{"field":"bid.cid"}]}}}`
	path := filepath.Join(t.TempDir(), "rules.json")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(rules)
	rm := NewRuleManager()
	loader := NewFileRuleLoader(rm, "", path)
	if err := loader.LoadSPPRules(); err != nil {
		t.Fatal(err)
	}
	p := NewOptimizedFilterProcessor(rm)
	bids := []*ortb.Bid{{Cid: proto.String("real")}, {}, {Cid: proto.String("")}, {Cid: proto.String("  ")}, nil}
	r := &ortb.BidResponse{Seatbid: []*ortb.SeatBid{nil, {Bid: bids[:2]}, {Bid: bids[2:]}}}
	p.addMissingSPPFieldsV25("other.example", r)
	if bids[1].Cid != nil {
		t.Fatal("unconfigured domain was changed")
	}
	p.addMissingSPPFieldsV25(domain, r)
	seen := map[string]bool{}
	for _, b := range bids[1:4] {
		id, err := uuid.Parse(b.GetCid())
		if err != nil || id.Version() != 4 || seen[b.GetCid()] {
			t.Fatalf("expected a distinct random UUID v4, got %q", b.GetCid())
		}
		seen[b.GetCid()] = true
	}
	if bids[0].GetCid() != "real" {
		t.Fatal("real CID was overwritten")
	}
	static := &ortb.Bid{}
	p.addMissingSPPFieldsV25("static.example", &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{static}}}})
	if static.GetCid() != "fixed" {
		t.Fatalf("add returned %q", static.GetCid())
	}
	for _, invalid := range []string{
		`{"spps":{"feed":{"add_random":[{"field":"bid.price"}]}}}`,
		`{"spps":{"feed":{"add":[{"field":"bid.cid","value":12}]}}}`,
		`{"spps":{"feed":{"add":[{"field":"bid.cid","value":" "}]}}}`,
		`{"spps":{"feed":{"add":[{"field":"bid.cid","value":"fixed"}],"add_random":[{"field":"bid.cid"}]}}}`,
	} {
		write(invalid)
		if err := loader.LoadSPPRules(); err == nil {
			t.Fatalf("accepted invalid configuration %s", invalid)
		}
		if !rm.sppDomainAdds[domain].random {
			t.Fatal("invalid reload replaced the previous rules")
		}
	}
	write(`{"spps":{}}`)
	if err := loader.LoadSPPRules(); err != nil || len(rm.sppDomainAdds) != 0 {
		t.Fatal("removed domain rule was retained", err)
	}
}

func TestSPPCIDPoolHas50StableIDsAndPicksOnlyMembers(t *testing.T) {
	const domain = "ssp.example"
	const json = `{"spps":{"` + domain + `":{"add_random":[{"field":"bid.cid","pool_size":50}]}}}`
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, []byte(json), 0600); err != nil {
		t.Fatal(err)
	}
	load := func() *RuleManager {
		t.Helper()
		rm := NewRuleManager()
		if err := NewFileRuleLoader(rm, "", path).LoadSPPRules(); err != nil {
			t.Fatal(err)
		}
		return rm
	}
	rm, restarted := load(), load()
	pool := rm.sppDomainAdds[domain].pool
	if len(pool) != 50 || !slices.Equal(pool, restarted.sppDomainAdds[domain].pool) {
		t.Fatal("expected the same pool of 50 IDs on an independent load")
	}
	valid := make(map[string]bool, len(pool))
	for _, cid := range pool {
		if _, err := uuid.Parse(cid); err != nil || valid[cid] {
			t.Fatalf("invalid or duplicate pooled ID %q", cid)
		}
		valid[cid] = true
	}
	otherSize := 50
	other, err := compileSPPCIDAdd(VersionedSPPSettings{
		AddRandom: []sppAddRandom{{Field: "bid.cid", PoolSize: &otherSize}},
	}, "domain:other")
	if err != nil {
		t.Fatal(err)
	}
	for _, cid := range other.pool {
		if valid[cid] {
			t.Fatalf("another domain reused pooled ID %q", cid)
		}
	}
	p := NewOptimizedFilterProcessor(rm)
	seen := make(map[string]bool)
	for i := 0; i < 2000; i++ {
		bid := &ortb.Bid{}
		p.addMissingSPPFieldsV25(domain, &ortb.BidResponse{
			Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{bid}}},
		})
		if !valid[bid.GetCid()] {
			t.Fatalf("selected CID outside the pool: %q", bid.GetCid())
		}
		seen[bid.GetCid()] = true
	}
	if len(seen) != 50 {
		t.Fatalf("selection reached %d distinct IDs; expected 50", len(seen))
	}
	for _, size := range []int{0, -1, maxSPPCIDPoolSize + 1} {
		if _, err := compileSPPCIDAdd(VersionedSPPSettings{
			AddRandom: []sppAddRandom{{Field: "bid.cid", PoolSize: &size}},
		}, "domain:test"); err == nil {
			t.Fatalf("accepted invalid pool size %d", size)
		}
	}
}

func TestSPPCIDAddPoolsAreScopedByRuleID(t *testing.T) {
	const scope = "domain:ssp.example"
	compile := func(id string) []string {
		t.Helper()
		var cfg VersionedSPPSettings
		data := fmt.Sprintf(`{"add":[{"field":"bid.cid","pool_size":50,"rule_id":%q}]}`, id)
		if err := json.Unmarshal([]byte(data), &cfg); err != nil {
			t.Fatal(err)
		}
		add, err := compileSPPCIDAdd(cfg, scope)
		if err != nil || !add.random || len(add.pool) != 50 {
			t.Fatalf("bad rule pool: len=%d err=%v", len(add.pool), err)
		}
		return add.pool
	}
	first := compile("first")
	if !slices.Equal(first, compile("first")) {
		t.Fatal("rule pool changed after reload")
	}
	for _, cid := range compile("second") {
		if slices.Contains(first, cid) {
			t.Fatalf("separate rule reused CID %q", cid)
		}
	}
	var old VersionedSPPSettings
	if err := json.Unmarshal([]byte(`{"add_random":[{"field":"bid.cid","pool_size":50}]}`), &old); err != nil {
		t.Fatal(err)
	}
	legacy, err := compileSPPCIDAdd(old, scope)
	if err != nil || !slices.Equal(legacy.pool, compile("")) {
		t.Fatal("switch to add without rule_id rotated the existing pool")
	}
	var invalid VersionedSPPSettings
	if err := json.Unmarshal([]byte(`{"add":[{"field":"bid.cid","pool_size":50,"value":"fixed"}]}`), &invalid); err != nil {
		t.Fatal(err)
	}
	if _, err := compileSPPCIDAdd(invalid, scope); err == nil {
		t.Fatal("accepted both a fixed CID and a pool in one rule")
	}
}
