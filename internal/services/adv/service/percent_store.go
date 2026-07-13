package auction

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

type PercentMap map[string]map[string]map[string]*types.PercentAndBidfloor

type percentSnapshot struct {
	Adult      PercentMap
	Mainstream PercentMap
}

type PercentStore struct {
	adultFile      string
	mainstreamFile string
	value          atomic.Pointer[percentSnapshot]
	updateMu       sync.Mutex
}

func NewPercentStore(adultFile, mainstreamFile string) (*PercentStore, error) {
	store := &PercentStore{adultFile: strings.TrimSpace(adultFile), mainstreamFile: strings.TrimSpace(mainstreamFile)}
	adult, err := loadPercentMap(store.adultFile)
	if err != nil {
		return nil, fmt.Errorf("load ADULT percent map: %w", err)
	}
	mainstream, err := loadPercentMap(store.mainstreamFile)
	if err != nil {
		return nil, fmt.Errorf("load MAINSTREAM percent map: %w", err)
	}
	store.value.Store(&percentSnapshot{Adult: adult, Mainstream: mainstream})
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
	for rawDomain, countries := range input {
		domain := normalizeMapKey(rawDomain, true)
		if domain == "" {
			return nil, errors.New("percent map contains empty SSP domain")
		}
		if _, exists := out[domain]; exists {
			return nil, fmt.Errorf("duplicate normalized SSP domain %q", domain)
		}
		out[domain] = make(map[string]map[string]*types.PercentAndBidfloor, len(countries))
		for rawCountry, users := range countries {
			country := normalizeMapKey(rawCountry, false)
			if country == "" {
				return nil, fmt.Errorf("percent map %s contains empty country", domain)
			}
			if _, exists := out[domain][country]; exists {
				return nil, fmt.Errorf("duplicate normalized country %s/%s", domain, country)
			}
			out[domain][country] = make(map[string]*types.PercentAndBidfloor, len(users))
			for rawUserID, value := range users {
				userID := normalizeMapKey(rawUserID, false)
				if userID == "" || value == nil {
					return nil, fmt.Errorf("invalid percent value at %s/%s/%s", domain, country, rawUserID)
				}
				percent := float64(value.Percent)
				if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > 1 {
					return nil, fmt.Errorf("deduction percent out of range at %s/%s/%s", domain, country, userID)
				}
				if _, exists := out[domain][country][userID]; exists {
					return nil, fmt.Errorf("duplicate normalized user ID at %s/%s/%s", domain, country, userID)
				}
				copyValue := *value
				out[domain][country][userID] = &copyValue
			}
		}
	}
	return out, nil
}

func normalizeMapKey(value string, domain bool) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "ALL") {
		return "ALL"
	}
	if domain {
		return normalizeDomain(value)
	}
	return strings.ToUpper(value)
}

func normalizeDomain(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	parseValue := value
	if !strings.Contains(parseValue, "://") {
		parseValue = "//" + parseValue
	}
	if parsed, err := url.Parse(parseValue); err == nil && parsed.Hostname() != "" {
		value = parsed.Hostname()
	} else {
		value = strings.SplitN(value, "/", 2)[0]
		value = strings.SplitN(value, "?", 2)[0]
	}
	return strings.TrimSuffix(strings.TrimSpace(value), ".")
}

func (s *PercentStore) Lookup(trafficType, sspDomain, country, userID string) float64 {
	if s == nil {
		return 0
	}
	snapshot := s.value.Load()
	if snapshot == nil {
		return 0
	}
	selected := snapshot.Mainstream
	if normalizeTraffic(trafficType) == TrafficAdult {
		selected = snapshot.Adult
	}
	domains := []string{normalizeDomain(sspDomain), "ALL"}
	countries := []string{strings.ToUpper(strings.TrimSpace(country)), "ALL"}
	users := []string{strings.ToUpper(strings.TrimSpace(userID)), "ALL"}
	for _, domain := range domains {
		countryMap, ok := selected[domain]
		if !ok {
			continue
		}
		for _, c := range countries {
			userMap, ok := countryMap[c]
			if !ok {
				continue
			}
			for _, user := range users {
				if value := userMap[user]; value != nil {
					return float64(value.Percent)
				}
			}
		}
	}
	return 0
}

func (s *PercentStore) Saved(trafficType string) (PercentMap, error) {
	if s == nil {
		return nil, errors.New("percent store is nil")
	}
	filename := s.mainstreamFile
	if normalizeTraffic(trafficType) == TrafficAdult {
		filename = s.adultFile
	}
	return loadPercentMap(filename)
}

func (s *PercentStore) Memory(trafficType string) PercentMap {
	if s == nil {
		return PercentMap{}
	}
	current := s.value.Load()
	if current == nil {
		return PercentMap{}
	}
	if normalizeTraffic(trafficType) == TrafficAdult {
		return clonePercentMap(current.Adult)
	}
	return clonePercentMap(current.Mainstream)
}

func (s *PercentStore) Update(trafficType string, input PercentMap) error {
	if s == nil {
		return errors.New("percent store is nil")
	}
	normalized, err := validateAndNormalizePercentMap(input)
	if err != nil {
		return err
	}
	typ := normalizeTraffic(trafficType)
	if typ == "" {
		return errors.New("invalid traffic type")
	}
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	current := s.value.Load()
	if current == nil {
		current = &percentSnapshot{Adult: PercentMap{}, Mainstream: PercentMap{}}
	}
	next := &percentSnapshot{Adult: clonePercentMap(current.Adult), Mainstream: clonePercentMap(current.Mainstream)}
	filename := s.mainstreamFile
	if typ == TrafficAdult {
		next.Adult = normalized
		filename = s.adultFile
	} else {
		next.Mainstream = normalized
	}
	if err := writeJSONAtomic(filename, normalized); err != nil {
		return err
	}
	s.value.Store(next)
	return nil
}

func clonePercentMap(input PercentMap) PercentMap {
	out := make(PercentMap, len(input))
	for domain, countries := range input {
		out[domain] = make(map[string]map[string]*types.PercentAndBidfloor, len(countries))
		for country, users := range countries {
			out[domain][country] = make(map[string]*types.PercentAndBidfloor, len(users))
			for user, value := range users {
				if value == nil {
					continue
				}
				copyValue := *value
				out[domain][country][user] = &copyValue
			}
		}
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
