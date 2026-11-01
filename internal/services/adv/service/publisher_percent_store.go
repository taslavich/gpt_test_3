package auction

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// PublisherPercentMap contains only explicit Publisher owner IDs. Values are
// retained fractions (0.20 = 20%). There is deliberately no ALL/default entry.
type PublisherPercentMap map[string]float64

var ErrPublisherPercentValidation = errors.New("invalid Publisher percent map")

// null is not an explicit zero-percent agreement.
func (m *PublisherPercentMap) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("Publisher percent map must be a JSON object")
	}
	result := make(PublisherPercentMap, len(fields))
	for key, raw := range fields {
		if strings.TrimSpace(string(raw)) == "null" {
			return fmt.Errorf("Publisher percent for %q must be a number, not null", key)
		}
		var percent float64
		if err := json.Unmarshal(raw, &percent); err != nil {
			return fmt.Errorf("Publisher percent for %q: %w", key, err)
		}
		result[key] = percent
	}
	*m = result
	return nil
}

type publisherPercentSnapshot struct {
	saved  PublisherPercentMap
	values PublisherPercentMap
}

// PublisherPercentStore follows BidEngine's percent store: validate/expand,
// persist atomically, then publish an immutable snapshot without reader locks.
type PublisherPercentStore struct {
	filename string
	updateMu sync.Mutex
	value    atomic.Pointer[publisherPercentSnapshot]
}

func NewPublisherPercentStore(filename string) (*PublisherPercentStore, error) {
	s := &PublisherPercentStore{filename: strings.TrimSpace(filename)}
	if s.filename == "" {
		return nil, fmt.Errorf("Publisher percent map filename is empty")
	}
	input := PublisherPercentMap{}
	data, err := os.ReadFile(s.filename)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read Publisher percent map: %w", err)
	}
	if err == nil && strings.TrimSpace(string(data)) != "" {
		if err = json.Unmarshal(data, &input); err != nil {
			return nil, fmt.Errorf("parse Publisher percent map: %w", err)
		}
	}
	snapshot, err := validatePublisherPercents(input)
	if err != nil {
		return nil, err
	}
	s.value.Store(snapshot)
	return s, nil
}

func validatePublisherPercents(input PublisherPercentMap) (*publisherPercentSnapshot, error) {
	snapshot := &publisherPercentSnapshot{saved: PublisherPercentMap{}, values: PublisherPercentMap{}}
	for key, percent := range input {
		if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > 1 {
			return nil, fmt.Errorf("%w: percent for %q must be between 0 and 1", ErrPublisherPercentValidation, key)
		}
		for _, part := range strings.Split(key, ",") {
			id := strings.TrimSpace(part)
			parsed, err := uuid.Parse(id)
			if err != nil || parsed == uuid.Nil || parsed.String() != id {
				return nil, fmt.Errorf("%w: key %q must contain canonical owner UUIDs; ALL is not supported", ErrPublisherPercentValidation, key)
			}
			if _, exists := snapshot.values[id]; exists {
				return nil, fmt.Errorf("%w: duplicate percent for %q", ErrPublisherPercentValidation, id)
			}
			snapshot.values[id] = percent
		}
		snapshot.saved[key] = percent
	}
	return snapshot, nil
}

func (s *PublisherPercentStore) Lookup(ownerID string) (float64, bool) {
	if s == nil {
		return 0, false
	}
	current := s.value.Load()
	if current == nil {
		return 0, false
	}
	percent, matched := current.values[ownerID]
	return percent, matched
}

func (s *PublisherPercentStore) Saved() PublisherPercentMap {
	if s == nil || s.value.Load() == nil {
		return PublisherPercentMap{}
	}
	return clonePublisherPercents(s.value.Load().saved)
}

func (s *PublisherPercentStore) Memory() PublisherPercentMap {
	if s == nil || s.value.Load() == nil {
		return PublisherPercentMap{}
	}
	return clonePublisherPercents(s.value.Load().values)
}

func clonePublisherPercents(input PublisherPercentMap) PublisherPercentMap {
	out := make(PublisherPercentMap, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func (s *PublisherPercentStore) Update(input PublisherPercentMap) error {
	if s == nil {
		return fmt.Errorf("Publisher percent store is unavailable")
	}
	next, err := validatePublisherPercents(input)
	if err != nil {
		return err
	}
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	if err = writeJSONAtomic(s.filename, next.saved); err != nil {
		return err
	}
	s.value.Store(next)
	return nil
}
