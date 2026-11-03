package bidEngine

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

// Map is a site_id -> GEO -> DSP domain -> retained share of the DSP bid.
// Values use the same fractional convention as BidEngine's existing percent
// maps: 0.20 means 20% is retained by the exchange.
type Map map[string]map[string]map[string]float32

const allKey = "ALL"

func splitAndTrimKeys(key string) []string {
	if key == "" {
		return []string{""}
	}
	parts := strings.Split(key, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return []string{""}
	}
	return result
}

// ValidationError means the JSON shape was valid but a configured rule is not.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// Store keeps an immutable expanded runtime snapshot and the raw JSON file used
// by the control-plane HTTP API. Readers never observe a partially updated map.
type Store struct {
	updateMu sync.Mutex
	filename string
	runtime  atomic.Value // Map
}

func NewStore(filename string) (*Store, error) {
	filename = strings.TrimSpace(filename)
	store := &Store{filename: filename}
	store.runtime.Store(Map{})
	if filename == "" {
		return store, nil
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// A missing optional override file is equivalent to an empty map and
			// preserves the existing SSP/GEO/DSP percent behavior.
			return store, nil
		}
		return nil, fmt.Errorf("read site_id/GEO/DSP percent map %s: %w", filename, err)
	}

	raw, err := decode(data)
	if err != nil {
		return nil, fmt.Errorf("parse site_id/GEO/DSP percent map %s: %w", filename, err)
	}
	expanded, err := expandAndValidate(raw)
	if err != nil {
		return nil, fmt.Errorf("validate site_id/GEO/DSP percent map %s: %w", filename, err)
	}
	store.runtime.Store(expanded)
	return store, nil
}

func decode(data []byte) (Map, error) {
	if strings.TrimSpace(string(data)) == "" {
		return Map{}, nil
	}
	var raw Map
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return Map{}, nil
	}
	return raw, nil
}

func validPercent(value float32) bool {
	v := float64(value)
	return !math.IsNaN(v) && !math.IsInf(v, 0) && value >= 0 && value <= 1
}

func expandAndValidate(raw Map) (Map, error) {
	expanded := make(Map)
	for siteKey, geoMap := range raw {
		if geoMap == nil {
			return nil, &ValidationError{Message: fmt.Sprintf("site_id key %q must contain a GEO object", siteKey)}
		}
		siteIDs := splitAndTrimKeys(siteKey)
		for geoKey, dspMap := range geoMap {
			if dspMap == nil {
				return nil, &ValidationError{Message: fmt.Sprintf("site_id key %q GEO key %q must contain a DSP object", siteKey, geoKey)}
			}
			geos := splitAndTrimKeys(geoKey)
			for dspKey, percent := range dspMap {
				if !validPercent(percent) {
					return nil, &ValidationError{Message: fmt.Sprintf("percent for site_id %q GEO %q DSP %q must be between 0 and 1; got %v", siteKey, geoKey, dspKey, percent)}
				}
				dsps := splitAndTrimKeys(dspKey)
				for _, site := range siteIDs {
					if expanded[site] == nil {
						expanded[site] = make(map[string]map[string]float32)
					}
					for _, geo := range geos {
						if expanded[site][geo] == nil {
							expanded[site][geo] = make(map[string]float32)
						}
						for _, dsp := range dsps {
							if _, exists := expanded[site][geo][dsp]; exists {
								return nil, &ValidationError{Message: fmt.Sprintf("duplicate expanded site_id/GEO/DSP rule for %q / %q / %q", site, geo, dsp)}
							}
							expanded[site][geo][dsp] = percent
						}
					}
				}
			}
		}
	}
	return expanded, nil
}

func clone(input Map) Map {
	output := make(Map, len(input))
	for site, geoMap := range input {
		clonedGeoMap := make(map[string]map[string]float32, len(geoMap))
		for geo, dspMap := range geoMap {
			clonedDSPMap := make(map[string]float32, len(dspMap))
			for dsp, percent := range dspMap {
				clonedDSPMap[dsp] = percent
			}
			clonedGeoMap[geo] = clonedDSPMap
		}
		output[site] = clonedGeoMap
	}
	return output
}

// Lookup uses the same precedence as the SSP/GEO/DSP map: at each level,
// try the exact key first, then ALL. A missing rule falls back to that map.
func (s *Store) Lookup(siteID, country, dsp string) (percent float32, matched bool) {
	if s == nil {
		return 0, false
	}
	runtimeMap, _ := s.runtime.Load().(Map)
	for _, site := range [2]string{siteID, allKey} {
		for _, geo := range [2]string{country, allKey} {
			dspMap := runtimeMap[site][geo]
			for _, candidate := range [2]string{dsp, allKey} {
				if value, ok := dspMap[candidate]; ok {
					return value, true
				}
			}
		}
	}
	return 0, false
}

func (s *Store) Snapshot() Map {
	if s == nil {
		return Map{}
	}
	runtimeMap, _ := s.runtime.Load().(Map)
	return clone(runtimeMap)
}

func (s *Store) ReadRaw() (Map, error) {
	if s == nil || strings.TrimSpace(s.filename) == "" {
		return Map{}, nil
	}
	data, err := os.ReadFile(s.filename)
	if err != nil {
		if os.IsNotExist(err) {
			return Map{}, nil
		}
		return nil, err
	}
	return decode(data)
}

func (s *Store) Update(raw Map) error {
	if s == nil {
		return errors.New("site_id/GEO/DSP percent store is not configured")
	}
	if strings.TrimSpace(s.filename) == "" {
		return errors.New("site_id/GEO/DSP percent map filename is empty")
	}
	if raw == nil {
		raw = Map{}
	}
	expanded, err := expandAndValidate(raw)
	if err != nil {
		return err
	}
	fileData, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal site_id/GEO/DSP percent map: %w", err)
	}

	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	if err := atomicWriteFile(s.filename, fileData, 0644); err != nil {
		return fmt.Errorf("write site_id/GEO/DSP percent map %s: %w", s.filename, err)
	}
	s.runtime.Store(expanded)
	return nil
}

func atomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	tmp, err := os.CreateTemp(dir, ".site_id_dsp_percents-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filename)
}

func IsValidationError(err error) bool {
	var target *ValidationError
	return errors.As(err, &target)
}
