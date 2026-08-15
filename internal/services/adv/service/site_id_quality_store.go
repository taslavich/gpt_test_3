package auction

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// SiteIDQualityRule configures the second ADV quality stage for one campaign
// quality segment. The stage is always enabled; there is intentionally no
// "apply" flag.
type SiteIDQualityRule struct {
	IsWhiteList bool     `json:"isWhiteList"`
	SiteIDs     []string `json:"siteIds"`
}

// SiteIDQualityMaps must contain exactly the usual/high/ultra segments.
type SiteIDQualityMaps map[string]*SiteIDQualityRule

type compiledSiteIDQualityRule struct {
	isWhiteList bool
	siteIDs     map[string]struct{}
}

type siteIDQualitySnapshot struct {
	config    SiteIDQualityMaps
	bySegment map[string]compiledSiteIDQualityRule
}

// SiteIDQualityStore keeps an immutable, atomically replaceable snapshot for
// the site.id quality stage. Request processing never reads or parses the file.
type SiteIDQualityStore struct {
	filename string
	value    atomic.Pointer[siteIDQualitySnapshot]
	updateMu sync.Mutex
}

func NewSiteIDQualityStore(filename string) (*SiteIDQualityStore, error) {
	store := &SiteIDQualityStore{filename: strings.TrimSpace(filename)}
	if err := store.Reload(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SiteIDQualityStore) Reload() error {
	if s == nil || s.filename == "" {
		return errors.New("ADV site ID quality map file is not configured")
	}
	data, err := os.ReadFile(s.filename)
	if err != nil {
		return fmt.Errorf("read ADV site ID quality map: %w", err)
	}
	snapshot, err := parseSiteIDQualityMaps(data)
	if err != nil {
		return err
	}
	s.value.Store(snapshot)
	return nil
}

// UpdateJSON validates the complete replacement before touching either the
// persisted file or the in-memory snapshot. Invalid input therefore leaves
// both states unchanged.
func (s *SiteIDQualityStore) UpdateJSON(data []byte) error {
	if s == nil || s.filename == "" {
		return errors.New("ADV site ID quality store is not configured")
	}
	next, err := parseSiteIDQualityMaps(data)
	if err != nil {
		return err
	}

	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	if err := writeJSONAtomic(s.filename, next.config); err != nil {
		return fmt.Errorf("persist ADV site ID quality map: %w", err)
	}
	s.value.Store(next)
	return nil
}

func (s *SiteIDQualityStore) Saved() (SiteIDQualityMaps, error) {
	if s == nil || s.filename == "" {
		return nil, errors.New("ADV site ID quality store is not configured")
	}
	data, err := os.ReadFile(s.filename)
	if err != nil {
		return nil, fmt.Errorf("read ADV site ID quality map: %w", err)
	}
	snapshot, err := parseSiteIDQualityMaps(data)
	if err != nil {
		return nil, err
	}
	return cloneSiteIDQualityMaps(snapshot.config), nil
}

func (s *SiteIDQualityStore) Memory() SiteIDQualityMaps {
	if s == nil {
		return SiteIDQualityMaps{}
	}
	current := s.value.Load()
	if current == nil {
		return SiteIDQualityMaps{}
	}
	return cloneSiteIDQualityMaps(current.config)
}

// Allows evaluates the second quality stage. A missing/blank request site.id
// always passes this stage. A missing segment or unavailable runtime snapshot
// fails closed for requests that do contain site.id.
func (s *SiteIDQualityStore) Allows(segment, siteID string) bool {
	return s.allowsNormalized(normalizeQualitySegment(segment), normalizeSiteID(siteID))
}

// allowsNormalized is used on the auction hot path. Campaign quality segments
// are normalized when the campaign snapshot is built, and siteID is normalized
// once per request before campaign iteration.
func (s *SiteIDQualityStore) allowsNormalized(segment, siteID string) bool {
	if siteID == "" {
		return true
	}
	if s == nil {
		return false
	}
	current := s.value.Load()
	if current == nil {
		return false
	}
	rule, ok := current.bySegment[segment]
	if !ok {
		return false
	}
	_, listed := rule.siteIDs[siteID]
	if rule.isWhiteList {
		return listed
	}
	return !listed
}

// ValidateSiteIDQualityMapsJSON applies the same strict validation used at
// startup and during updates without mutating file or runtime state.
func ValidateSiteIDQualityMapsJSON(data []byte) error {
	_, err := parseSiteIDQualityMaps(data)
	return err
}

func parseSiteIDQualityMaps(data []byte) (*siteIDQualitySnapshot, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var rawTop map[string]json.RawMessage
	if err := decoder.Decode(&rawTop); err != nil {
		return nil, fmt.Errorf("invalid ADV site ID quality map JSON: %w", err)
	}
	if err := ensureDecoderEOF(decoder); err != nil {
		return nil, fmt.Errorf("invalid ADV site ID quality map JSON: %w", err)
	}
	if rawTop == nil {
		return nil, errors.New("ADV site ID quality maps must be a JSON object, not null")
	}

	for segment := range rawTop {
		if _, ok := validQualitySegments[segment]; !ok {
			return nil, fmt.Errorf("invalid ADV site ID quality segment %q; expected exactly usual, high and ultra", segment)
		}
	}
	for _, segment := range []string{"usual", "high", "ultra"} {
		if _, ok := rawTop[segment]; !ok {
			return nil, fmt.Errorf("ADV site ID quality map %q is missing", segment)
		}
	}
	if len(rawTop) != 3 {
		return nil, fmt.Errorf("ADV site ID quality map must contain exactly 3 segments: usual, high and ultra")
	}

	config := make(SiteIDQualityMaps, 3)
	compiled := make(map[string]compiledSiteIDQualityRule, 3)
	for _, segment := range []string{"usual", "high", "ultra"} {
		rule, compiledRule, err := parseSiteIDQualityRule(segment, rawTop[segment])
		if err != nil {
			return nil, err
		}
		config[segment] = rule
		compiled[segment] = compiledRule
	}
	return &siteIDQualitySnapshot{config: config, bySegment: compiled}, nil
}

func parseSiteIDQualityRule(segment string, raw json.RawMessage) (*SiteIDQualityRule, compiledSiteIDQualityRule, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, compiledSiteIDQualityRule{}, fmt.Errorf("ADV site ID quality map %q must not be null", segment)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, compiledSiteIDQualityRule{}, fmt.Errorf("ADV site ID quality map %q must be a JSON object: %w", segment, err)
	}
	if fields == nil {
		return nil, compiledSiteIDQualityRule{}, fmt.Errorf("ADV site ID quality map %q must be a JSON object, not null", segment)
	}
	for field := range fields {
		switch field {
		case "isWhiteList", "siteIds":
		default:
			return nil, compiledSiteIDQualityRule{}, fmt.Errorf("ADV site ID quality map %q contains unknown field %q", segment, field)
		}
	}

	rawWhitelist, ok := fields["isWhiteList"]
	if !ok {
		return nil, compiledSiteIDQualityRule{}, fmt.Errorf("ADV site ID quality map %q is missing required field \"isWhiteList\"", segment)
	}
	if bytes.Equal(bytes.TrimSpace(rawWhitelist), []byte("null")) {
		return nil, compiledSiteIDQualityRule{}, fmt.Errorf("ADV site ID quality map %q field \"isWhiteList\" must not be null", segment)
	}
	var isWhiteList bool
	if err := json.Unmarshal(rawWhitelist, &isWhiteList); err != nil {
		return nil, compiledSiteIDQualityRule{}, fmt.Errorf("ADV site ID quality map %q field \"isWhiteList\" must be boolean: %w", segment, err)
	}

	rawSiteIDs, ok := fields["siteIds"]
	if !ok {
		return nil, compiledSiteIDQualityRule{}, fmt.Errorf("ADV site ID quality map %q is missing required field \"siteIds\"", segment)
	}
	if bytes.Equal(bytes.TrimSpace(rawSiteIDs), []byte("null")) {
		return nil, compiledSiteIDQualityRule{}, fmt.Errorf("ADV site ID quality map %q field \"siteIds\" must be an array, not null", segment)
	}
	var siteIDs []string
	if err := json.Unmarshal(rawSiteIDs, &siteIDs); err != nil {
		return nil, compiledSiteIDQualityRule{}, fmt.Errorf("ADV site ID quality map %q field \"siteIds\" must be an array of strings: %w", segment, err)
	}
	if siteIDs == nil {
		return nil, compiledSiteIDQualityRule{}, fmt.Errorf("ADV site ID quality map %q field \"siteIds\" must be an array, not null", segment)
	}

	normalizedIDs := make([]string, 0, len(siteIDs))
	set := make(map[string]struct{}, len(siteIDs))
	for index, rawSiteID := range siteIDs {
		siteID := normalizeSiteID(rawSiteID)
		if siteID == "" {
			return nil, compiledSiteIDQualityRule{}, fmt.Errorf("ADV site ID quality map %q contains empty siteIds[%d]", segment, index)
		}
		if _, exists := set[siteID]; exists {
			continue
		}
		set[siteID] = struct{}{}
		normalizedIDs = append(normalizedIDs, siteID)
	}

	rule := &SiteIDQualityRule{IsWhiteList: isWhiteList, SiteIDs: normalizedIDs}
	compiledRule := compiledSiteIDQualityRule{isWhiteList: isWhiteList, siteIDs: set}
	return rule, compiledRule, nil
}

func ensureDecoderEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("document contains more than one JSON value")
}

func normalizeSiteID(value string) string {
	return strings.TrimSpace(value)
}

func cloneSiteIDQualityMaps(input SiteIDQualityMaps) SiteIDQualityMaps {
	out := make(SiteIDQualityMaps, len(input))
	for segment, rule := range input {
		if rule == nil {
			out[segment] = nil
			continue
		}
		ids := make([]string, len(rule.SiteIDs))
		copy(ids, rule.SiteIDs)
		if ids == nil {
			ids = []string{}
		}
		out[segment] = &SiteIDQualityRule{IsWhiteList: rule.IsWhiteList, SiteIDs: ids}
	}
	return out
}
