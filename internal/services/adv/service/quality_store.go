package auction

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

var validQualitySegments = map[string]struct{}{"usual": {}, "high": {}, "ultra": {}}

// QualityFeedMap is a replaceable set of SSP feed UUIDs for one quality segment.
// Only entries with value true are valid members of the set.
type QualityFeedMap map[string]bool

// QualityMaps contains the three independently replaceable quality maps.
// The same feed UUID may belong to more than one quality map.
type QualityMaps map[string]QualityFeedMap

type qualitySnapshot struct {
	bySegment QualityMaps
}

type QualityStore struct {
	filename string
	value    atomic.Pointer[qualitySnapshot]
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
		return fmt.Errorf("read quality maps: %w", err)
	}
	snapshot, err := parseQualityMaps(data)
	if err != nil {
		return err
	}
	s.value.Store(snapshot)
	return nil
}

// Update replaces exactly one of the usual/high/ultra feed maps. The complete
// next snapshot is validated before the file and in-memory pointer are replaced.
// Membership in the other quality maps is preserved, including overlaps.
func (s *QualityStore) Update(segment string, input QualityFeedMap) error {
	if s == nil || s.filename == "" {
		return errors.New("ADV quality store is not configured")
	}
	segment = normalizeQualitySegment(segment)
	if _, ok := validQualitySegments[segment]; !ok {
		return fmt.Errorf("invalid quality segment %q", segment)
	}
	normalized, err := normalizeQualityFeedMap(input)
	if err != nil {
		return err
	}

	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	current := s.value.Load()
	if current == nil {
		return errors.New("ADV quality snapshot is unavailable")
	}
	nextMaps := cloneQualityMaps(current.bySegment)
	nextMaps[segment] = normalized
	next, err := buildQualitySnapshot(nextMaps)
	if err != nil {
		return err
	}
	if err := writeJSONAtomic(s.filename, next.bySegment); err != nil {
		return err
	}
	s.value.Store(next)
	return nil
}

// UpdateAll atomically replaces all three quality maps.
func (s *QualityStore) UpdateAll(input QualityMaps) error {
	if s == nil || s.filename == "" {
		return errors.New("ADV quality store is not configured")
	}
	next, err := buildQualitySnapshot(input)
	if err != nil {
		return err
	}

	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	if err := writeJSONAtomic(s.filename, next.bySegment); err != nil {
		return err
	}
	s.value.Store(next)
	return nil
}

// Saved returns the persisted map for one quality segment.
func (s *QualityStore) Saved(segment string) (QualityFeedMap, error) {
	if s == nil || s.filename == "" {
		return nil, errors.New("ADV quality store is not configured")
	}
	segment = normalizeQualitySegment(segment)
	if _, ok := validQualitySegments[segment]; !ok {
		return nil, fmt.Errorf("invalid quality segment %q", segment)
	}
	data, err := os.ReadFile(s.filename)
	if err != nil {
		return nil, err
	}
	snapshot, err := parseQualityMaps(data)
	if err != nil {
		return nil, err
	}
	return cloneQualityFeedMap(snapshot.bySegment[segment]), nil
}

func (s *QualityStore) SavedAll() (QualityMaps, error) {
	if s == nil || s.filename == "" {
		return nil, errors.New("ADV quality store is not configured")
	}
	data, err := os.ReadFile(s.filename)
	if err != nil {
		return nil, err
	}
	snapshot, err := parseQualityMaps(data)
	if err != nil {
		return nil, err
	}
	return cloneQualityMaps(snapshot.bySegment), nil
}

// Memory returns the current in-memory map for one quality segment.
func (s *QualityStore) Memory(segment string) QualityFeedMap {
	segment = normalizeQualitySegment(segment)
	if _, ok := validQualitySegments[segment]; !ok || s == nil {
		return QualityFeedMap{}
	}
	current := s.value.Load()
	if current == nil {
		return QualityFeedMap{}
	}
	return cloneQualityFeedMap(current.bySegment[segment])
}

func (s *QualityStore) MemoryAll() QualityMaps {
	if s == nil {
		return emptyQualityMaps()
	}
	current := s.value.Load()
	if current == nil {
		return emptyQualityMaps()
	}
	return cloneQualityMaps(current.bySegment)
}

func parseQualityMaps(data []byte) (*qualitySnapshot, error) {
	input := QualityMaps{}
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("quality maps must be an object with usual/high/ultra feed maps: %w", err)
	}
	return buildQualitySnapshot(input)
}

func buildQualitySnapshot(input QualityMaps) (*qualitySnapshot, error) {
	if input == nil {
		return nil, errors.New("quality maps are nil")
	}
	for segment := range input {
		normalized := normalizeQualitySegment(segment)
		if _, ok := validQualitySegments[normalized]; !ok {
			return nil, fmt.Errorf("invalid quality segment %q", segment)
		}
		if segment != normalized {
			return nil, fmt.Errorf("quality segment key %q must be lowercase", segment)
		}
	}
	for _, segment := range []string{"usual", "high", "ultra"} {
		if _, ok := input[segment]; !ok {
			return nil, fmt.Errorf("quality map %q is missing", segment)
		}
	}

	bySegment := emptyQualityMaps()
	totalMemberships := 0
	for _, segment := range []string{"usual", "high", "ultra"} {
		normalized, err := normalizeQualityFeedMap(input[segment])
		if err != nil {
			return nil, fmt.Errorf("%s quality map: %w", segment, err)
		}
		bySegment[segment] = normalized
		totalMemberships += len(normalized)
	}
	if totalMemberships == 0 {
		return nil, errors.New("quality maps must contain at least one feed UUID")
	}
	return &qualitySnapshot{bySegment: bySegment}, nil
}

func normalizeQualityFeedMap(input QualityFeedMap) (QualityFeedMap, error) {
	if input == nil {
		return nil, errors.New("quality feed map must be a JSON object, not null")
	}
	out := make(QualityFeedMap, len(input))
	for rawFeed, enabled := range input {
		feed := normalizeFeed(rawFeed)
		if feed == "" {
			return nil, errors.New("quality map contains an empty feed UUID")
		}
		if !enabled {
			return nil, fmt.Errorf("quality map entry %s must be true or removed", rawFeed)
		}
		parsed, err := uuid.Parse(feed)
		if err != nil {
			return nil, fmt.Errorf("invalid feed UUID %q: %w", rawFeed, err)
		}
		feed = strings.ToLower(parsed.String())
		if _, exists := out[feed]; exists {
			return nil, fmt.Errorf("duplicate normalized feed UUID %s", feed)
		}
		out[feed] = true
	}
	return out, nil
}

func emptyQualityMaps() QualityMaps {
	return QualityMaps{
		"usual": QualityFeedMap{},
		"high":  QualityFeedMap{},
		"ultra": QualityFeedMap{},
	}
}

func cloneQualityFeedMap(input QualityFeedMap) QualityFeedMap {
	out := make(QualityFeedMap, len(input))
	for feed, enabled := range input {
		out[feed] = enabled
	}
	return out
}

func cloneQualityMaps(input QualityMaps) QualityMaps {
	out := emptyQualityMaps()
	for segment, feeds := range input {
		segment = normalizeQualitySegment(segment)
		if _, ok := validQualitySegments[segment]; ok {
			out[segment] = cloneQualityFeedMap(feeds)
		}
	}
	return out
}

func normalizeQualitySegment(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeFeed(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *QualityStore) Count() int {
	if s == nil {
		return 0
	}
	current := s.value.Load()
	if current == nil {
		return 0
	}
	count := 0
	for _, segment := range []string{"usual", "high", "ultra"} {
		count += len(current.bySegment[segment])
	}
	return count
}

// Contains reports whether the incoming SSP feed UUID belongs to the requested
// quality map. The campaign quality_type must be passed as segment.
func (s *QualityStore) Contains(segment, feed string) bool {
	if s == nil {
		return false
	}
	segment = normalizeQualitySegment(segment)
	if _, ok := validQualitySegments[segment]; !ok {
		return false
	}
	feed = normalizeFeed(feed)
	if feed == "" {
		return false
	}
	current := s.value.Load()
	if current == nil {
		return false
	}
	return current.bySegment[segment][feed]
}

// ContainsAny reports whether the feed is present in at least one quality map.
func (s *QualityStore) ContainsAny(feed string) bool {
	for _, segment := range []string{"usual", "high", "ultra"} {
		if s.Contains(segment, feed) {
			return true
		}
	}
	return false
}
