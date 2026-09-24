package auction

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	PercentMapDefaultKey      = "ALL"
	PercentMapRTBDefaultKey   = "ALL_RTB"
	PercentMapDefaultFallback = 0.30
	MaxAdvertiserMargin       = 0.90
)

// PercentMap is the persisted ADV percentage configuration. Campaign keys may
// be grouped (for example "123,456,789"). Runtime lookup always expands those
// groups into individual campaign IDs.
type PercentMap map[string]float64

type percentSnapshot struct {
	Saved  PercentMap
	Values PercentMap
}

type PercentStore struct {
	filename string
	value    atomic.Pointer[percentSnapshot]
	updateMu sync.Mutex
}

func NewPercentStore(filename string) (*PercentStore, error) {
	store := &PercentStore{filename: strings.TrimSpace(filename)}
	if store.filename == "" {
		return nil, errors.New("ADV percent map filename is empty")
	}
	saved, runtime, repaired, err := loadPercentMap(store.filename)
	if err != nil {
		return nil, fmt.Errorf("load ADV percent map: %w", err)
	}
	if repaired {
		if err := writeJSONAtomic(store.filename, saved); err != nil {
			return nil, fmt.Errorf("persist repaired ADV percent map: %w", err)
		}
	}
	store.value.Store(&percentSnapshot{Saved: saved, Values: runtime})
	return store, nil
}

func loadPercentMap(filename string) (PercentMap, PercentMap, bool, error) {
	input := PercentMap{}
	data, err := os.ReadFile(filename)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, false, err
	}
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &input); err != nil {
			return nil, nil, false, err
		}
	}
	return validateAndNormalizePercentMap(input)
}

func validateAndNormalizePercentMap(input PercentMap) (PercentMap, PercentMap, bool, error) {
	saved := make(PercentMap, len(input)+2)
	runtime := make(PercentMap, len(input)+2)
	seenCampaigns := make(map[string]string)

	for rawKey, percent := range input {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			return nil, nil, false, errors.New("ADV percent map contains empty key")
		}
		if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > MaxAdvertiserMargin {
			return nil, nil, false, fmt.Errorf("ADV percentage out of range for key %q: must be between 0 and %.2f", key, MaxAdvertiserMargin)
		}

		switch strings.ToUpper(key) {
		case PercentMapDefaultKey:
			if _, exists := saved[PercentMapDefaultKey]; exists {
				return nil, nil, false, errors.New("duplicate ALL key in ADV percent map")
			}
			saved[PercentMapDefaultKey] = percent
			runtime[PercentMapDefaultKey] = percent
			continue
		case PercentMapRTBDefaultKey:
			if _, exists := saved[PercentMapRTBDefaultKey]; exists {
				return nil, nil, false, errors.New("duplicate ALL_RTB key in ADV percent map")
			}
			saved[PercentMapRTBDefaultKey] = percent
			runtime[PercentMapRTBDefaultKey] = percent
			continue
		}

		parts := strings.Split(key, ",")
		campaignIDs := make([]string, 0, len(parts))
		for _, part := range parts {
			campaignID := normalizePercentCampaignID(part)
			if campaignID == "" {
				return nil, nil, false, fmt.Errorf("ADV percent map key %q contains an empty campaign_id", key)
			}
			if previous, duplicate := seenCampaigns[campaignID]; duplicate {
				return nil, nil, false, fmt.Errorf("campaign_id %q is present in both percent map keys %q and %q", campaignID, previous, key)
			}
			seenCampaigns[campaignID] = key
			campaignIDs = append(campaignIDs, campaignID)
		}
		// Preserve the persisted grouped key (apart from surrounding whitespace).
		saved[key] = percent
		for _, campaignID := range campaignIDs {
			runtime[campaignID] = percent
		}
	}

	repaired := false
	if _, exists := saved[PercentMapDefaultKey]; !exists {
		saved[PercentMapDefaultKey] = PercentMapDefaultFallback
		runtime[PercentMapDefaultKey] = PercentMapDefaultFallback
		repaired = true
	}
	if _, exists := saved[PercentMapRTBDefaultKey]; !exists {
		saved[PercentMapRTBDefaultKey] = PercentMapDefaultFallback
		runtime[PercentMapRTBDefaultKey] = PercentMapDefaultFallback
		repaired = true
	}
	return saved, runtime, repaired, nil
}

func normalizePercentCampaignID(value string) string {
	return strings.TrimSpace(value)
}

func (s *PercentStore) Lookup(campaignID string) float64 {
	return s.LookupForCampaign(campaignID, false)
}

func (s *PercentStore) LookupForCampaign(campaignID string, rtb bool) float64 {
	percent, _ := s.LookupForCampaignWithSource(campaignID, rtb)
	return percent
}

func (s *PercentStore) LookupForCampaignWithSource(campaignID string, rtb bool) (float64, string) {
	defaultKey := PercentMapDefaultKey
	if rtb {
		defaultKey = PercentMapRTBDefaultKey
	}
	if s == nil {
		return 0, ""
	}
	snapshot := s.value.Load()
	if snapshot == nil {
		return 0, ""
	}
	if percent, exists := snapshot.Values[normalizePercentCampaignID(campaignID)]; exists {
		return percent, "campaign"
	}
	if percent, exists := snapshot.Values[defaultKey]; exists {
		return percent, defaultKey
	}
	// load/update validation requires both ALL and ALL_RTB. Returning an empty
	// source here makes an impossible/corrupt snapshot fail closed in pricing
	// instead of silently reintroducing a hard-coded runtime default.
	return 0, ""
}

func (s *PercentStore) Saved() (PercentMap, error) {
	if s == nil {
		return nil, errors.New("percent store is nil")
	}
	current := s.value.Load()
	if current == nil {
		return nil, errors.New("percent store is not initialized")
	}
	return clonePercentMap(current.Saved), nil
}

func (s *PercentStore) Memory() PercentMap {
	if s == nil {
		return PercentMap{}
	}
	current := s.value.Load()
	if current == nil {
		return PercentMap{}
	}
	return clonePercentMap(current.Values)
}

func (s *PercentStore) Update(input PercentMap) error {
	if s == nil {
		return errors.New("percent store is nil")
	}
	saved, runtime, _, err := validateAndNormalizePercentMap(input)
	if err != nil {
		return err
	}

	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	if err := writeJSONAtomic(s.filename, saved); err != nil {
		return err
	}
	s.value.Store(&percentSnapshot{Saved: clonePercentMap(saved), Values: clonePercentMap(runtime)})
	return nil
}

func clonePercentMap(input PercentMap) PercentMap {
	out := make(PercentMap, len(input))
	for key, percent := range input {
		out[key] = percent
	}
	return out
}

func writeJSONAtomic(filename string, value any) error {
	if strings.TrimSpace(filename) == "" {
		return errors.New("map filename is empty")
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create map directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".adv-map-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return err
	}
	if dirHandle, err := os.Open(dir); err == nil {
		defer dirHandle.Close()
		if err := dirHandle.Sync(); err != nil {
			return fmt.Errorf("sync map directory %s: %w", dir, err)
		}
	}
	return nil
}
