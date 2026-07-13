package auction

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

var validQualitySegments = map[string]struct{}{"usual": {}, "high": {}, "ultra": {}}

type QualityStore struct {
	filename string
	value    atomic.Pointer[map[string]string]
	updateMu sync.Mutex
}

func NewQualityStore(filename string) (*QualityStore, error) {
	store := &QualityStore{filename: strings.TrimSpace(filename)}
	if err := store.Reload(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *QualityStore) Reload() error {
	if s == nil || s.filename == "" {
		return errors.New("ADV quality map file is not configured")
	}
	data, err := os.ReadFile(s.filename)
	if err != nil {
		return fmt.Errorf("read quality map: %w", err)
	}
	mapping, err := parseQualityMap(data)
	if err != nil {
		return err
	}
	s.value.Store(&mapping)
	return nil
}

func (s *QualityStore) UpdateJSON(data []byte) error {
	if s == nil || s.filename == "" {
		return errors.New("ADV quality store is not configured")
	}
	mapping, err := parseQualityMap(data)
	if err != nil {
		return err
	}
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	if err := writeJSONAtomic(s.filename, mapping); err != nil {
		return err
	}
	s.value.Store(&mapping)
	return nil
}

func (s *QualityStore) Saved() (map[string]string, error) {
	if s == nil || s.filename == "" {
		return nil, errors.New("ADV quality store is not configured")
	}
	data, err := os.ReadFile(s.filename)
	if err != nil {
		return nil, err
	}
	return parseQualityMap(data)
}

func (s *QualityStore) Memory() map[string]string {
	if s == nil {
		return map[string]string{}
	}
	current := s.value.Load()
	if current == nil {
		return map[string]string{}
	}
	return cloneStringStringMap(*current)
}

func parseQualityMap(data []byte) (map[string]string, error) {
	direct := map[string]string{}
	if err := json.Unmarshal(data, &direct); err == nil {
		return validateQualityMap(direct)
	}
	bySegment := map[string][]string{}
	if err := json.Unmarshal(data, &bySegment); err != nil {
		return nil, fmt.Errorf("quality map must be domain->segment or segment->domains: %w", err)
	}
	for segment, domains := range bySegment {
		segment = strings.ToLower(strings.TrimSpace(segment))
		if _, ok := validQualitySegments[segment]; !ok {
			return nil, fmt.Errorf("invalid quality segment %q", segment)
		}
		for _, domain := range domains {
			domain = normalizeDomain(domain)
			if domain == "" {
				return nil, errors.New("quality map contains empty domain")
			}
			if _, exists := direct[domain]; exists {
				return nil, fmt.Errorf("quality domain %s is duplicated", domain)
			}
			direct[domain] = segment
		}
	}
	return validateQualityMap(direct)
}

func validateQualityMap(input map[string]string) (map[string]string, error) {
	if len(input) == 0 {
		return nil, errors.New("quality map must contain at least one SSP domain")
	}
	out := make(map[string]string, len(input))
	for rawDomain, rawSegment := range input {
		domain := normalizeDomain(rawDomain)
		segment := strings.ToLower(strings.TrimSpace(rawSegment))
		if domain == "" {
			return nil, errors.New("quality map contains empty domain")
		}
		if _, ok := validQualitySegments[segment]; !ok {
			return nil, fmt.Errorf("invalid quality segment %q for %s", segment, domain)
		}
		if _, exists := out[domain]; exists {
			return nil, fmt.Errorf("duplicate normalized quality domain %s", domain)
		}
		out[domain] = segment
	}
	return out, nil
}

func cloneStringStringMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func (s *QualityStore) Count() int {
	if s == nil {
		return 0
	}
	current := s.value.Load()
	if current == nil {
		return 0
	}
	return len(*current)
}

func (s *QualityStore) Lookup(domain string) (string, bool) {
	if s == nil {
		return "", false
	}
	current := s.value.Load()
	if current == nil {
		return "", false
	}
	segment, ok := (*current)[normalizeDomain(domain)]
	return segment, ok
}
