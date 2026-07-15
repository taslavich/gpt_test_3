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

// DefaultADVPercent is used when the campaign owner is absent from the map.
// 0.30 means a default 30% deduction from the auction price.
const DefaultADVPercent = 0.30

// PercentMap stores the ADV auction deduction percent by campaign owner user ID.
// JSON format: {"<user_id>": <percent>}, where percent must be in [0, 1].
type PercentMap map[string]float64

type percentSnapshot struct {
	Values PercentMap
}

type PercentStore struct {
	filename string
	value    atomic.Pointer[percentSnapshot]
	updateMu sync.Mutex
}

func NewPercentStore(filename string) (*PercentStore, error) {
	store := &PercentStore{filename: strings.TrimSpace(filename)}
	value, err := loadPercentMap(store.filename)
	if err != nil {
		return nil, fmt.Errorf("load ADV percent map: %w", err)
	}
	store.value.Store(&percentSnapshot{Values: value})
	return store, nil
}

func loadPercentMap(filename string) (PercentMap, error) {
	if filename == "" {
		return PercentMap{}, nil
	}
	data, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return PercentMap{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return PercentMap{}, nil
	}
	var input PercentMap
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, err
	}
	return validateAndNormalizePercentMap(input)
}

func validateAndNormalizePercentMap(input PercentMap) (PercentMap, error) {
	out := make(PercentMap, len(input))
	for rawUserID, percent := range input {
		userID := normalizePercentUserID(rawUserID)
		if userID == "" {
			return nil, errors.New("ADV percent map contains empty user ID")
		}
		if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > 1 {
			return nil, fmt.Errorf("ADV deduction percent out of range for user %q", userID)
		}
		if _, exists := out[userID]; exists {
			return nil, fmt.Errorf("duplicate normalized user ID %q in ADV percent map", userID)
		}
		out[userID] = percent
	}
	return out, nil
}

func normalizePercentUserID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *PercentStore) Lookup(userID string) float64 {
	if s == nil {
		return DefaultADVPercent
	}
	snapshot := s.value.Load()
	if snapshot == nil {
		return DefaultADVPercent
	}
	percent, exists := snapshot.Values[normalizePercentUserID(userID)]
	if !exists {
		return DefaultADVPercent
	}
	return percent
}

func (s *PercentStore) Saved() (PercentMap, error) {
	if s == nil {
		return nil, errors.New("percent store is nil")
	}
	return loadPercentMap(s.filename)
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
	normalized, err := validateAndNormalizePercentMap(input)
	if err != nil {
		return err
	}

	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	if err := writeJSONAtomic(s.filename, normalized); err != nil {
		return err
	}
	s.value.Store(&percentSnapshot{Values: clonePercentMap(normalized)})
	return nil
}

func clonePercentMap(input PercentMap) PercentMap {
	out := make(PercentMap, len(input))
	for userID, percent := range input {
		out[userID] = percent
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
