package filter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func domainRequest(id, domain string) *ortb.BidRequest {
	page := "https://original.example/page"
	keyword := "keep-keywords"
	requestID := "bid-1"
	testValue := int32(1)
	return &ortb.BidRequest{
		Id:   &requestID,
		Test: &testValue,
		Site: &ortb.Site{Id: &id, Domain: &domain, Page: &page, Keywords: &keyword},
	}
}

func TestDomainChangersPreserveOtherFieldsAndDoNotLeakAcrossDSP(t *testing.T) {
	box := &DomainChangersBoxChanger{}
	box.active.Store(&domainChangersSnapshot{changers: ToManyDomainChangersMap(DomainChangerType{
		"adl_first.example, adl_other.example": {
			Apply: true,
			SiteIdBoxes: map[string]*DomainBoxChanger{
				"123, 456": {ToChange: true, Domain: "first.example"},
			},
		},
		"adl_second.example": {Apply: true, SiteIdBoxes: map[string]*DomainBoxChanger{
			"123": {ToChange: true, Domain: "second.example"},
		}},
	})})
	original := domainRequest("123", "original.example")

	first, changed := box.Change(original, "adl_first.example")
	if !changed || first.Site.GetDomain() != "first.example" || first.Site.GetId() != "123" {
		t.Fatalf("first DSP changed=%v site=%v", changed, first.Site)
	}
	if first.Site.GetKeywords() != "keep-keywords" || first.Site.GetPage() != "https://original.example/page" || first.GetTest() != 1 {
		t.Fatalf("other bid fields lost: %+v", first)
	}
	if first == original || first.Site == original.Site || original.Site.GetDomain() != "original.example" {
		t.Fatal("original DSP payload mutated")
	}
	second, changed := box.Change(original, "adl_second.example")
	if !changed || second.Site.GetDomain() != "second.example" {
		t.Fatalf("second DSP changed=%v site=%v", changed, second.Site)
	}
	if got, ok := box.Change(original, "adl_none.example"); ok || got != original {
		t.Fatalf("unmapped DSP changed unexpectedly: %v", got)
	}
	if got, ok := box.Change(domainRequest("456", "orig"), "adl_other.example"); !ok || got.Site.GetDomain() != "first.example" {
		t.Fatalf("grouped DSP/site_id not expanded: %v", got)
	}
}

func TestDomainChangerApplyToChangeAndAllFallback(t *testing.T) {
	raw := DomainChangerType{
		"ALL": {Apply: true, SiteIdBoxes: map[string]*DomainBoxChanger{
			"ALL": {ToChange: true, Domain: "all.example"},
		}},
		"disabled.example": {Apply: false, SiteIdBoxes: map[string]*DomainBoxChanger{
			"123": {ToChange: true, Domain: "disabled.example"},
		}},
		"noop.example": {Apply: true, SiteIdBoxes: map[string]*DomainBoxChanger{
			"123": {ToChange: false, Domain: "disabled.example"},
		}},
		"specific.example": {Apply: true, SiteIdBoxes: map[string]*DomainBoxChanger{
			"999": {ToChange: true, Domain: "specific.example"},
		}},
	}
	box := &DomainChangersBoxChanger{}
	box.active.Store(&domainChangersSnapshot{changers: ToManyDomainChangersMap(raw)})
	req := domainRequest("123", "original.example")
	if got, changed := box.Change(req, "disabled.example"); !changed || got.Site.GetDomain() != "all.example" {
		t.Fatalf("disabled DSP must fall back to ALL, changed=%v site=%v", changed, got.Site)
	}
	if got, changed := box.Change(req, "noop.example"); changed || got != req {
		t.Fatal("toChange=false must not fall back to ALL")
	}
	if got, changed := box.Change(req, "specific.example"); changed || got != req {
		t.Fatal("active DSP with unmatched ID must not fall back to ALL")
	}
	if got, changed := box.Change(req, "absent.example"); !changed || got.Site.GetDomain() != "all.example" {
		t.Fatal("unknown DSP must fall back to ALL")
	}
	if got, changed := box.Change(&ortb.BidRequest{}, "absent.example"); changed || got == nil {
		t.Fatal("nil site must not be modified")
	}
}

func TestDomainChangerPersistenceReloadAndFailedPUT(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "domains.json")
	if err := os.WriteFile(filename, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	box, err := NewDomainChangersBoxChanger(filename)
	if err != nil {
		t.Fatal(err)
	}
	changed := DomainChangerType{
		"adl_one,adl_two": {Apply: true, SiteIdBoxes: map[string]*DomainBoxChanger{
			"101,102": {ToChange: true, Domain: "mapped.example"},
		}},
	}
	if err := box.Rewrite(changed, filename); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk DomainChangerType
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	if len(onDisk) != 1 || onDisk["adl_one,adl_two"] == nil || onDisk["adl_one"] != nil {
		t.Fatalf("original grouped JSON not retained: %s", raw)
	}
	reloaded, err := NewDomainChangersBoxChanger(filename)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := reloaded.Change(domainRequest("102", "original.example"), "adl_two"); !ok || got.Site.GetDomain() != "mapped.example" {
		t.Fatalf("reloaded map incomplete: %v", got)
	}
	if err := box.Rewrite(DomainChangerType{}, filepath.Join(t.TempDir(), "missing", "map.json")); err == nil {
		t.Fatal("expected disk failure")
	}
	if got, ok := box.Change(domainRequest("101", "original.example"), "adl_one"); !ok || got.Site.GetDomain() != "mapped.example" {
		t.Fatal("failed persistence changed live snapshot")
	}
}

func TestDomainChangerSnapshotConcurrentReadersAndWriters(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "domains.json")
	if err := os.WriteFile(filename, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	box, err := NewDomainChangersBoxChanger(filename)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				got, changed := box.Change(domainRequest("12", "original.example"), "adl_dsp")
				if changed && got.Site.GetDomain() != "a.example" && got.Site.GetDomain() != "b.example" {
					t.Errorf("torn snapshot: %s", got.Site.GetDomain())
					return
				}
			}
		}()
	}
	for i := 0; i < 30; i++ {
		domain := "a.example"
		if i%2 != 0 {
			domain = "b.example"
		}
		if err := box.Rewrite(DomainChangerType{"adl_dsp": {
			Apply: true, SiteIdBoxes: map[string]*DomainBoxChanger{
				"12": {ToChange: true, Domain: domain},
			},
		}}, filename); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}
