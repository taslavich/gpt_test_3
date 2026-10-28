package rtbrules

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type snapshot struct {
	byCampaign map[string]RuleSet
	all        RuleSet
	hasAll     bool
}

// Store uses the same disk-on-startup / immutable-runtime-snapshot / HTTP-PUT
// update contract as ADV Quality Map. The file is not polled at runtime.
type Store struct {
	path   string
	mu     sync.Mutex
	active atomic.Pointer[snapshot]
}

func NewStore(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("RTB rules map path is required")
	}
	s := &Store{path: path}
	if e := s.Reload(); e != nil {
		return nil, e
	}
	return s, nil
}
func (s *Store) Reload() error {
	b, e := os.ReadFile(s.path)
	if e != nil {
		return fmt.Errorf("read RTB rules: %w", e)
	}
	snap, e := parse(b)
	if e != nil {
		return e
	}
	s.active.Store(snap)
	return nil
}

// Saved returns the persisted, grouped representation from disk, not the
// expanded in-memory lookup representation.
// SavedJSON returns the unchanged user-authored JSON document for HTTP GET.
func (s *Store) SavedJSON() ([]byte, error) {
	if s == nil {
		return nil, errors.New("RTB rules unavailable")
	}
	return os.ReadFile(s.path)
}

func (s *Store) Saved() (map[string]RuleSet, error) {
	if s == nil {
		return nil, errors.New("RTB rules unavailable")
	}
	b, e := os.ReadFile(s.path)
	if e != nil {
		return nil, e
	}
	_, e = parse(b)
	if e != nil {
		return nil, e
	}
	var m map[string]RuleSet
	e = json.Unmarshal(b, &m)
	return m, e
}
func (s *Store) Memory() map[string]RuleSet {
	out := map[string]RuleSet{}
	if s == nil {
		return out
	}
	snap := s.active.Load()
	if snap == nil {
		return out
	}
	for k := range snap.byCampaign {
		rule, _ := s.lookup(k)
		out[k] = cloneSet(rule)
	}
	if snap.hasAll {
		out["ALL"] = cloneSet(snap.all)
	}
	return out
}
func (s *Store) lookup(campaignID string) (RuleSet, bool) {
	if s == nil {
		return RuleSet{}, false
	}
	snap := s.active.Load()
	if snap == nil {
		return RuleSet{}, false
	}
	rule, ok := snap.byCampaign[campaignID]
	if !snap.hasAll {
		return rule, ok
	}
	if !ok {
		return snap.all, true
	}
	// Like LinkMap's ALL fallback, apply global rules to every campaign.
	// Campaign rules update only colliding fields, retaining the rest.
	return mergeRuleSet(snap.all, rule), true
}

// HasFilters avoids encoding requests for campaigns that only transform output.
func (s *Store) HasFilters(campaignID string) bool {
	r, ok := s.lookup(campaignID)
	return ok && len(r.Filters) > 0
}

func (s *Store) Has(campaignID string) bool { _, ok := s.lookup(campaignID); return ok }
func Validate(data []byte) error            { _, e := parse(data); return e }
func (s *Store) UpdateJSON(data []byte) error {
	if s == nil {
		return errors.New("RTB rules store unavailable")
	}
	next, e := parse(data)
	if e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Persist the exact authored JSON: grouped fields and campaign keys remain
	// unchanged, and only the expanded snapshot is used at runtime.
	if e = atomicWrite(s.path, data); e != nil {
		return e
	}
	s.active.Store(next)
	return nil
}
func cloneSet(in RuleSet) RuleSet {
	b, _ := json.Marshal(in)
	var v RuleSet
	_ = json.Unmarshal(b, &v)
	return v
}

// parse follows LinkMap's simple map-expansion model. The disk representation
// remains grouped; the runtime holds one merged RuleSet for each campaign ID.
// Ordinary Go map iteration intentionally gives no priority to overlapping
// group keys, as agreed for duplicate campaign rules.
func parse(data []byte) (*snapshot, error) {
	if err := rejectDuplicateKeys(data); err != nil {
		return nil, err
	}
	var grouped map[string]json.RawMessage
	if err := json.Unmarshal(data, &grouped); err != nil {
		return nil, fmt.Errorf("invalid RTB rules JSON: %w", err)
	}
	if grouped == nil {
		return nil, errors.New("RTB rules map must be an object")
	}
	result := &snapshot{byCampaign: map[string]RuleSet{}}
	for group, raw := range grouped {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("campaign %s rules must not be null", group)
		}
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		var rs RuleSet
		if err := d.Decode(&rs); err != nil {
			return nil, fmt.Errorf("campaign %s: %w", group, err)
		}
		if err := d.Decode(new(any)); err != io.EOF {
			return nil, fmt.Errorf("trailing rule data for %s", group)
		}
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keys); err != nil {
			return nil, err
		}
		for _, name := range []string{"filters", "add", "replace", "delete", "rename"} {
			if val, ok := keys[name]; ok && bytes.Equal(bytes.TrimSpace(val), []byte("null")) {
				return nil, fmt.Errorf("campaign %s module %s cannot be null", group, name)
			}
		}
		var err error
		if rs, err = expandFields(rs); err != nil {
			return nil, fmt.Errorf("campaign %s: %w", group, err)
		}
		if err := validateSet(rs); err != nil {
			return nil, fmt.Errorf("campaign %s: %w", group, err)
		}
		seenIDs := map[string]bool{}
		for _, id := range strings.Split(group, ",") {
			id = strings.TrimSpace(id)
			if seenIDs[id] {
				return nil, fmt.Errorf("duplicate campaign UUID %q within group", id)
			}
			seenIDs[id] = true
			if id == "ALL" {
				result.all = mergeRuleSet(result.all, rs)
				result.hasAll = true
				continue
			}
			if !uuidPattern.MatchString(id) {
				return nil, fmt.Errorf("invalid campaign UUID %q", id)
			}
			result.byCampaign[id] = mergeRuleSet(result.byCampaign[id], rs)
		}
	}
	// Validate the actual per-campaign rules after merging independent groups.
	for id, rules := range result.byCampaign {
		if result.hasAll {
			rules = mergeRuleSet(result.all, rules)
		}
		if err := validateSet(rules); err != nil {
			return nil, fmt.Errorf("merged campaign %s: %w", id, err)
		}
	}
	return result, nil
}

// Field grouping is purely configuration syntax; the runtime has one entry
// per original protobuf path. Filters, rename and copy_to remain single-field.
func splitRuleFields(s string) ([]string, error) {
	parts := strings.Split(s, ",")
	seen := map[string]bool{}
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			return nil, fmt.Errorf("invalid or repeated field in %q", s)
		}
		seen[part] = true
		parts[i] = part
	}
	return parts, nil
}
func expandFields(in RuleSet) (RuleSet, error) {
	out := RuleSet{Filters: in.Filters, Rename: in.Rename}
	for _, a := range in.Add {
		parts, err := splitRuleFields(a.Field)
		if err != nil {
			return RuleSet{}, err
		}
		for _, field := range parts {
			item := a
			item.Field = field
			out.Add = append(out.Add, item)
		}
	}
	for _, r := range in.Replace {
		parts, err := splitRuleFields(r.Field)
		if err != nil {
			return RuleSet{}, err
		}
		if len(parts) > 1 && r.CopyTo != "" {
			return RuleSet{}, fmt.Errorf("replace copy_to cannot group source fields %q", r.Field)
		}
		for _, field := range parts {
			item := r
			item.Field = field
			out.Replace = append(out.Replace, item)
		}
	}
	for _, d := range in.Delete {
		parts, err := splitRuleFields(d.Field)
		if err != nil {
			return RuleSet{}, err
		}
		for _, field := range parts {
			out.Delete = append(out.Delete, Delete{Field: field})
		}
	}
	return out, nil
}

// Merge by module and exact field path. Each campaign holds a single RuleSet,
// without any secondary group pointers or rule-block lookup indirection.
func mergeRuleSet(prev, next RuleSet) RuleSet {
	out := RuleSet{}
	out.Filters = mergeByPath(prev.Filters, next.Filters, func(f Filter) string { return f.Field })
	out.Add = mergeByPath(prev.Add, next.Add, func(a Add) string {
		if a.PostfixField != "" {
			return a.Field + "." + a.PostfixField
		}
		return a.Field
	})
	out.Replace = mergeByPath(prev.Replace, next.Replace, func(r Replace) string { return r.Field })
	out.Delete = mergeByPath(prev.Delete, next.Delete, func(d Delete) string { return d.Field })
	out.Rename = mergeByPath(prev.Rename, next.Rename, func(r Rename) string { return r.Field })
	return out
}
func mergeByPath[T any](prev, next []T, key func(T) string) []T {
	// Keep deliberately repeated checks inside a single original block.
	if len(prev) == 0 {
		return append([]T(nil), next...)
	}
	out := append([]T(nil), prev...)
	for _, current := range next {
		found := false
		for i := range out {
			if key(out[i]) == key(current) {
				out[i] = current
				found = true
				break
			}
		}
		if !found {
			out = append(out, current)
		}
	}
	return out
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if e := os.MkdirAll(dir, 0o750); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".adv-rtb-rules-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(tmp, path); e != nil {
		return e
	}
	// Make the directory rename durable where supported.
	d, e := os.Open(dir)
	if e == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Reject duplicated JSON keys at every nesting depth. encoding/json otherwise
// silently keeps the last value, which is unsafe for hand-edited policy maps.
func rejectDuplicateKeys(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var scan func() error
	scan = func() error {
		v, e := d.Token()
		if e != nil {
			return e
		}
		switch v {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				raw, e := d.Token()
				if e != nil {
					return e
				}
				key, ok := raw.(string)
				if !ok {
					return errors.New("expected JSON object key")
				}
				if seen[key] {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = true
				if e = scan(); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		case json.Delim('['):
			for d.More() {
				if e = scan(); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		}
		return nil
	}
	if e := scan(); e != nil {
		return fmt.Errorf("invalid RTB rules JSON: %w", e)
	}
	if _, e := d.Token(); e != io.EOF {
		return errors.New("trailing data after RTB rules map")
	}
	return nil
}
