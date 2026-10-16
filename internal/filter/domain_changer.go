package filter

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"google.golang.org/protobuf/proto"
)

// DomainChangerType matches the existing dsp_changers_map JSON layout, with
// "domain" replacing "siteId" in each siteIdBoxes entry.
type DomainChangerType map[string]*DomainChangersChanger

type DomainBoxChanger struct {
	ToChange bool   `json:"toChange"`
	Domain   string `json:"domain"`
}

func (s *DomainBoxChanger) Change(request *ortb_V2_5.BidRequest) (*ortb_V2_5.BidRequest, bool) {
	if s == nil || !s.ToChange || request == nil || request.Site == nil {
		return request, false
	}
	// Only site.domain changes. Cloning preserves the complete OpenRTB request,
	// including optional fields, and prevents mutations leaking to other DSPs.
	changed := proto.Clone(request).(*ortb_V2_5.BidRequest)
	changed.Site.Domain = &s.Domain
	return changed, true
}

type DomainChangersChanger struct {
	SiteIdBoxes map[string]*DomainBoxChanger `json:"siteIdBoxes"`
	Apply       bool                         `json:"apply"`
}

func (f *DomainChangersChanger) ToMany() *DomainChangersChanger {
	return &DomainChangersChanger{
		Apply:       f.Apply,
		SiteIdBoxes: expandGroupedKeys(f.SiteIdBoxes),
	}
}

func (f *DomainChangersChanger) Change(request *ortb_V2_5.BidRequest) (*ortb_V2_5.BidRequest, bool) {
	if f == nil || !f.Apply || request == nil || request.Site == nil {
		return request, false
	}
	if box, ok := f.SiteIdBoxes[request.Site.GetId()]; ok {
		return box.Change(request)
	}
	if box, ok := f.SiteIdBoxes["ALL"]; ok {
		return box.Change(request)
	}
	return request, false
}

func ToManyDomainChangersMap(changers DomainChangerType) DomainChangerType {
	grouped := make(DomainChangerType, len(changers))
	for key, value := range changers {
		if value != nil {
			grouped[key] = value.ToMany()
		}
	}
	return expandGroupedKeys(grouped)
}

type domainChangersSnapshot struct {
	changers DomainChangerType // Immutable after publication.
}

// DomainChangersBoxChanger uses atomic snapshot replacement so DSP workers can
// read a fully parsed immutable map concurrently with an HTTP PUT update.
type DomainChangersBoxChanger struct {
	active   atomic.Pointer[domainChangersSnapshot]
	updateMu sync.Mutex
}

func NewDomainChangersBoxChanger(filename string) (*DomainChangersBoxChanger, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read domain changers %s: %w", filename, err)
	}
	var parsed DomainChangerType
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse domain changers %s: %w", filename, err)
	}
	box := &DomainChangersBoxChanger{}
	box.active.Store(&domainChangersSnapshot{changers: ToManyDomainChangersMap(parsed)})
	return box, nil
}

// Snapshot exposes the currently parsed map for read-only debug serialization.
func (f *DomainChangersBoxChanger) Snapshot() DomainChangerType {
	if f == nil {
		return nil
	}
	snapshot := f.active.Load()
	if snapshot == nil {
		return nil
	}
	return snapshot.changers
}

func (f *DomainChangersBoxChanger) Change(request *ortb_V2_5.BidRequest, dspName string) (*ortb_V2_5.BidRequest, bool) {
	if request == nil || request.Site == nil || request.Site.Id == nil {
		return request, false
	}
	changers := f.Snapshot()
	if len(changers) == 0 {
		return request, false
	}
	if selected, ok := changers[dspName]; ok && selected != nil && selected.Apply {
		return selected.Change(request)
	}
	if all, ok := changers["ALL"]; ok && all != nil && all.Apply {
		return all.Change(request)
	}
	return request, false
}

// Rewrite persists the original grouped JSON and only then atomically swaps the
// expanded runtime snapshot. Failed writes leave the previous map active.
func (f *DomainChangersBoxChanger) Rewrite(changers DomainChangerType, filename string) error {
	if f == nil {
		return fmt.Errorf("domain changers are not configured")
	}
	data, err := json.MarshalIndent(changers, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal domain changers: %w", err)
	}
	parsed := ToManyDomainChangersMap(changers)
	f.updateMu.Lock()
	defer f.updateMu.Unlock()
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("write domain changers %s: %w", filename, err)
	}
	f.active.Store(&domainChangersSnapshot{changers: parsed})
	return nil
}
