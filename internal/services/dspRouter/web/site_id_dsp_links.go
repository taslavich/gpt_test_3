package dspRouterWeb

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
)

// SiteIDDSPLinkMap is the site_id -> DSP -> allow/deny routing map.
// It supports the same comma-separated key shorthand and ALL fallback style
// used by the router's existing link maps.
type SiteIDDSPLinkMap map[string]map[string]bool

// SiteIDDSPLinkStore keeps the expanded runtime map synchronized with the
// JSON file managed through the router HTTP API.
type SiteIDDSPLinkStore struct {
	updateMu sync.Mutex
	filename string
	runtime  atomic.Value // SiteIDDSPLinkMap; replaced atomically and never mutated in place.
}

func NewSiteIDDSPLinkStore(filename string) (*SiteIDDSPLinkStore, error) {
	filename = strings.TrimSpace(filename)
	store := &SiteIDDSPLinkStore{filename: filename}
	store.runtime.Store(SiteIDDSPLinkMap{})
	if filename == "" {
		return store, nil
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing file means there are no site_id overrides yet.
			return store, nil
		}
		return nil, fmt.Errorf("read site_id/DSP link map %s: %w", filename, err)
	}

	var raw SiteIDDSPLinkMap
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse site_id/DSP link map %s: %w", filename, err)
	}
	store.runtime.Store(expandSiteIDDSPLinkMap(raw))
	return store, nil
}

func expandSiteIDDSPLinkMap(raw SiteIDDSPLinkMap) SiteIDDSPLinkMap {
	expanded := make(SiteIDDSPLinkMap)
	for siteKey, dspMap := range raw {
		siteIDs := utils.SplitAndTrimKeys(siteKey)
		for dspKey, allowed := range dspMap {
			dsps := utils.SplitAndTrimKeys(dspKey)
			for _, siteID := range siteIDs {
				if expanded[siteID] == nil {
					expanded[siteID] = make(map[string]bool)
				}
				for _, dsp := range dsps {
					expanded[siteID][dsp] = allowed
				}
			}
		}
	}
	return expanded
}

func cloneSiteIDDSPLinkMap(input SiteIDDSPLinkMap) SiteIDDSPLinkMap {
	output := make(SiteIDDSPLinkMap, len(input))
	for siteID, dspMap := range input {
		clonedDSPMap := make(map[string]bool, len(dspMap))
		for dsp, allowed := range dspMap {
			clonedDSPMap[dsp] = allowed
		}
		output[siteID] = clonedDSPMap
	}
	return output
}

// Lookup specificity order:
// exact site/exact DSP -> exact site/ALL -> ALL/exact DSP -> ALL/ALL.
// The second return value distinguishes an explicit false rule from no rule.
func (s *SiteIDDSPLinkStore) Lookup(siteID, dsp string) (allowed bool, matched bool) {
	if s == nil {
		return false, false
	}
	runtime, _ := s.runtime.Load().(SiteIDDSPLinkMap)

	sites := []string{siteID}
	if siteID != utils.ALL {
		sites = append(sites, utils.ALL)
	}
	dsps := []string{dsp}
	if dsp != utils.ALL {
		dsps = append(dsps, utils.ALL)
	}

	for _, site := range sites {
		dspMap, ok := runtime[site]
		if !ok {
			continue
		}
		for _, candidateDSP := range dsps {
			if allowed, ok := dspMap[candidateDSP]; ok {
				return allowed, true
			}
		}
	}
	return false, false
}

func (s *SiteIDDSPLinkStore) Snapshot() SiteIDDSPLinkMap {
	if s == nil {
		return SiteIDDSPLinkMap{}
	}
	runtime, _ := s.runtime.Load().(SiteIDDSPLinkMap)
	return cloneSiteIDDSPLinkMap(runtime)
}

func (s *SiteIDDSPLinkStore) ReadRaw() (SiteIDDSPLinkMap, error) {
	if s == nil || strings.TrimSpace(s.filename) == "" {
		return SiteIDDSPLinkMap{}, nil
	}
	data, err := os.ReadFile(s.filename)
	if err != nil {
		if os.IsNotExist(err) {
			return SiteIDDSPLinkMap{}, nil
		}
		return nil, err
	}
	var raw SiteIDDSPLinkMap
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *SiteIDDSPLinkStore) Update(raw SiteIDDSPLinkMap) error {
	if s == nil {
		return fmt.Errorf("site_id/DSP link store is not configured")
	}
	if strings.TrimSpace(s.filename) == "" {
		return fmt.Errorf("site_id/DSP link map filename is empty")
	}
	expanded := expandSiteIDDSPLinkMap(raw)

	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	fileData, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal site_id/DSP link map: %w", err)
	}
	if err := os.WriteFile(s.filename, fileData, 0644); err != nil {
		return fmt.Errorf("write site_id/DSP link map %s: %w", s.filename, err)
	}

	s.runtime.Store(expanded)
	return nil
}

func shouldRouteDSPByMaps(
	siteID, dsp, ssp, geo string,
	siteIDDSPStore *SiteIDDSPLinkStore,
	linkMap GeoDspLinkMap,
) bool {
	if siteIDDSPStore != nil {
		if allowed, matched := siteIDDSPStore.Lookup(siteID, dsp); matched {
			// site_id + DSP is authoritative. An explicit true or false bypasses
			// the legacy SSP + GEO + DSP map completely.
			return allowed
		}
	}
	if linkMap == nil {
		return true
	}
	return utils.GetValueFomSspGeoDspMap(ssp, geo, dsp, linkMap, false)
}
