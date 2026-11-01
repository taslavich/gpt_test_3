package rtbrules

import (
	"bytes"
	"encoding/json"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"google.golang.org/protobuf/proto"
	"net/url"
	"strings"
)

func decodeJSON(b []byte) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var obj map[string]any
	if e := d.Decode(&obj); e != nil {
		return nil, e
	}
	if obj == nil {
		return nil, fmt.Errorf("bid request must be an object")
	}
	return obj, nil
}

// Check is retained for callers already supplying encoded BidRequest JSON.
// ADV uses CheckWithCache so all RTB campaigns evaluating the same request
// share the decoded request and the parsed embedded native JSON.
func (s *Store) Check(campaignID string, payload []byte, impID string) (bool, string, error) {
	return s.CheckWithCache(campaignID, newFilterCacheFromJSON(payload), impID)
}

// CheckWithCache returns an error only for malformed input / invalid types,
// not when a well-formed request merely fails a campaign-specific condition.
func (s *Store) CheckWithCache(campaignID string, cache *FilterCache, impID string) (bool, string, error) {
	rule, ok := s.lookup(campaignID)
	if !ok || len(rule.Filters) == 0 {
		return true, "", nil
	}
	request, e := cache.rootObject()
	if e != nil {
		return false, "", e
	}
	for _, f := range rule.Filters {
		var vals []any
		var exists bool
		if f.JSONPath != "" {
			vals, exists, e = cache.nativeValues(impID, f.JSONPath)
			if e != nil {
				return false, "", e
			}
		} else {
			vals, exists = readField(request, f.Field, impID)
		}
		accepted := false
		switch {
		case f.Exists != nil:
			accepted = exists == *f.Exists
		case f.Compare != "":
			for _, val := range vals {
				matched, err := checkNumericComparison(val, f.Compare, f.Value)
				if err != nil {
					return false, "", err
				}
				if matched {
					accepted = true
				}
			}
		default:
			listed := false
			for _, v := range vals {
				if f.JSONPath != "" && nativeJSONType(v) != jsonScalarType(f.Values[0]) {
					return false, "", nativeFilterError("json_path %s expects %s, got %s", f.JSONPath, jsonScalarType(f.Values[0]), nativeJSONType(v))
				}
				for _, target := range f.Values {
					var want any
					d := json.NewDecoder(bytes.NewReader(target))
					d.UseNumber()
					_ = d.Decode(&want)
					if equalJSONValue(v, want) {
						listed = true
					}
				}
			}
			// Missing input never satisfies a whitelist; it cannot be blacklisted.
			accepted = listed == *f.IsWhiteList
		}
		if !accepted {
			reason := f.Field
			if f.JSONPath != "" {
				reason += "." + f.JSONPath
			}
			return false, fmt.Sprintf("rtb_rule_filter_%s", reason), nil
		}
	}
	return true, "", nil
}

// ApplyToRequest operates exclusively on the already campaign-local protobuf
// clone (created by ADV). The delete module clears only real protobuf pointers
// *before* the single outbound serialization. The map is validated to forbid
// overlapping add/replace/delete rules, so this order is equivalent to
// filters -> add -> replace -> delete -> rename for all valid configurations.
func (s *Store) ApplyToRequest(campaignID string, req *ortb.BidRequest) ([]byte, error) {
	rule, ok := s.lookup(campaignID)
	if req == nil {
		return nil, fmt.Errorf("nil RTB request")
	}
	if !ok {
		return jsoniter.Marshal(req)
	}
	// Known protobuf fields are added to a campaign-local copy *before* JSON
	// serialization. set_or_add uses the same protobuf add logic for missing
	// fields; its replacement step runs later on the serialized JSON.
	hasSetOrAdd := false
	for _, r := range rule.Replace {
		if r.Mode == "set_or_add" {
			hasSetOrAdd = true
			break
		}
	}
	source := req
	if len(rule.Delete) > 0 || len(rule.Add) > 0 || hasSetOrAdd {
		source = proto.Clone(req).(*ortb.BidRequest)
	}
	for _, a := range rule.Add {
		if a.PostfixField == "" {
			if err := addProtobufField(source, a.Field, a.Value); err != nil {
				return nil, err
			}
		}
	}
	for _, r := range rule.Replace {
		if r.Mode == "set_or_add" {
			if err := addProtobufField(source, r.Field, r.Value); err != nil {
				return nil, err
			}
		}
	}
	for _, d := range rule.Delete {
		clearPointerPath(source, d.Field)
	}
	raw, e := jsoniter.Marshal(source)
	if e != nil {
		return nil, e
	}
	hasFilterCopy := false
	for _, f := range rule.Filters {
		if f.CopyTo != "" {
			hasFilterCopy = true
			break
		}
	}
	if len(rule.Add) == 0 && len(rule.Replace) == 0 && len(rule.Rename) == 0 && !hasFilterCopy {
		return raw, nil
	}
	return applyEncodedRules(raw, rule)
}

// The JSON phase is independent of protobuf preparation; malformed encoded
// input is rejected before any transformation.
func applyEncodedRules(raw []byte, rule RuleSet) ([]byte, error) {
	payload, e := decodeJSON(raw)
	if e != nil {
		return nil, e
	}
	applyFilterCopies(payload, rule.Filters)
	for _, a := range rule.Add {
		if a.PostfixField == "" {
			continue
		}
		var value any
		dec := json.NewDecoder(bytes.NewReader(a.Value))
		dec.UseNumber()
		if e = dec.Decode(&value); e != nil {
			return nil, e
		}
		if e = addPostfixField(payload, a.Field, a.PostfixField, value); e != nil {
			return nil, e
		}
	}
	for _, r := range rule.Replace {
		if e = replaceField(payload, r); e != nil {
			return nil, e
		}
	}
	// Key renaming is always the final module, after every other operation.
	for _, r := range rule.Rename {
		renameField(payload, r.Field, r.ToField)
	}
	return jsoniter.Marshal(payload)
}

// ADV checks every filter on the immutable inbound request before calling
// ApplyToRequest with only the accepted impressions. Copy actions therefore
// belong to this outgoing JSON phase, never to the shared FilterCache.
// Capture all sources first so one copy cannot feed another filter's copy.
func applyFilterCopies(payload map[string]any, filters []Filter) {
	type assignment struct {
		root  map[string]any
		path  string
		value any
	}
	var pending []assignment
	for _, f := range filters {
		if f.CopyTo == "" {
			continue
		}
		if strings.HasPrefix(f.Field, "imp.") {
			imps, _ := payload["imp"].([]any)
			for _, item := range imps {
				imp, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if value, present := getFieldValue(imp, strings.TrimPrefix(f.Field, "imp.")); present {
					pending = append(pending, assignment{imp, strings.TrimPrefix(f.CopyTo, "imp."), copyValue(value)})
				}
			}
		} else if value, present := getFieldValue(payload, f.Field); present {
			pending = append(pending, assignment{payload, f.CopyTo, copyValue(value)})
		}
	}
	for _, a := range pending {
		setField(a.root, a.path, a.value)
	}
}

func equalJSONValue(a, b any) bool {
	// For booleans, numbers, and strings decoded using UseNumber.
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ab, bb)
}
func fieldTargets(root map[string]any, path string, impID string, create bool) []map[string]any {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return []map[string]any{root}
	}
	if parts[0] == "imp" {
		imps, ok := root["imp"].([]any)
		if !ok {
			return nil
		}
		out := make([]map[string]any, 0, len(imps))
		for _, item := range imps {
			imp, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if impID != "" && imp["id"] != impID {
				continue
			}
			if target := descend(imp, parts[1:len(parts)-1], create); target != nil {
				out = append(out, target)
			}
		}
		return out
	}
	if target := descend(root, parts[:len(parts)-1], create); target != nil {
		return []map[string]any{target}
	}
	return nil
}
func descend(root map[string]any, parts []string, create bool) map[string]any {
	current := root
	for _, part := range parts {
		next, ok := current[part].(map[string]any)
		if !ok {
			if !create {
				return nil
			}
			if current[part] != nil {
				return nil
			}
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	return current
}
func readField(root map[string]any, path, impID string) ([]any, bool) {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		v, ok := root[path]
		if !ok || v == nil {
			return nil, false
		}
		return explode(v), true
	}
	parents := fieldTargets(root, path, impID, false)
	var values []any
	found := false
	for _, p := range parents {
		v, ok := p[parts[len(parts)-1]]
		if ok && v != nil {
			found = true
			values = append(values, explode(v)...)
		}
	}
	return values, found
}
func explode(v any) []any {
	if arr, ok := v.([]any); ok {
		return arr
	}
	return []any{v}
}

// addPostfixField creates JSON-only nested keys after protobuf serialization.
// Multiple rules may share parents; incompatible scalar parents are rejected.
func addPostfixField(payload map[string]any, base, postfix string, value any) error {
	fullPath := base + "." + postfix
	parts := strings.Split(fullPath, ".")
	if strings.HasPrefix(fullPath, "imp.") {
		if _, ok := payload["imp"].([]any); !ok {
			return nil // no impression to extend
		}
	}
	parents := fieldTargets(payload, base+".__rtb_postfix_placeholder__", "", true)
	if len(parents) == 0 {
		// The base may be an existing scalar/object conflict; never overwrite it.
		return fmt.Errorf("cannot create postfix %s under %s", postfix, base)
	}
	for _, parent := range parents {
		current := parent
		for _, key := range strings.Split(postfix, ".")[:len(strings.Split(postfix, "."))-1] {
			next, ok := current[key].(map[string]any)
			if !ok {
				if current[key] != nil {
					return fmt.Errorf("postfix path %s collides with non-object %s", fullPath, key)
				}
				next = map[string]any{}
				current[key] = next
			}
			current = next
		}
		terminal := parts[len(parts)-1]
		current[terminal] = copyValue(value)
	}
	return nil
}

// replaceExistingField never creates the last key or any parent objects.
// Missing values, including JSON null, are left untouched.
func replaceExistingField(payload map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		if old, ok := payload[path]; ok && old != nil {
			payload[path] = value
		}
		return
	}
	for _, parent := range fieldTargets(payload, path, "", false) {
		key := parts[len(parts)-1]
		if old, ok := parent[key]; ok && old != nil {
			parent[key] = value
		}
	}
}

// copyTo copies an existing source field's *final* value regardless of
// whether the destination already has a value. A missing source is a no-op.
// When both endpoints are impression-scoped, copy inside each impression;
// values from different impressions must never be mixed.
func copyTo(payload map[string]any, from, to string) {
	if strings.HasPrefix(from, "imp.") {
		imps, _ := payload["imp"].([]any)
		for _, item := range imps {
			imp, ok := item.(map[string]any)
			if !ok {
				continue
			}
			fromRelative := strings.TrimPrefix(from, "imp.")
			toRelative := strings.TrimPrefix(to, "imp.")
			vals, found := readField(imp, fromRelative, "")
			if !found || len(vals) == 0 {
				continue
			}
			value, _ := getFieldValue(imp, fromRelative)
			setField(imp, toRelative, copyValue(value))
		}
		return
	}
	value, exists := getFieldValue(payload, from)
	if !exists {
		return
	}
	setField(payload, to, copyValue(value))
}

func getFieldValue(root map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var parents []map[string]any
	if len(parts) == 1 {
		parents = []map[string]any{root}
	} else {
		parents = fieldTargets(root, path, "", false)
	}
	for _, parent := range parents {
		if value, ok := parent[parts[len(parts)-1]]; ok && value != nil {
			return value, true
		}
	}
	return nil, false
}

// Avoid sharing mutable maps/slices when copy_to targets another field.
func copyValue(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[k] = copyValue(x)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = copyValue(x)
		}
		return out
	default:
		return v
	}
}

func setField(payload map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		payload[path] = value
		return
	}
	for _, target := range fieldTargets(payload, path, "", true) {
		target[parts[len(parts)-1]] = value
	}
}
func renameField(payload map[string]any, field, toField string) {
	parts := strings.Split(field, ".")
	for _, parent := range fieldTargets(payload, field, "", false) {
		if v, ok := parent[parts[len(parts)-1]]; ok {
			delete(parent, parts[len(parts)-1])
			parent[toField] = v
		}
	}
}
func replaceField(payload map[string]any, r Replace) error {
	mode := r.Mode
	if r.Countries != nil {
		mode = "countries"
	}
	switch mode {
	case "set", "set_or_add":
		var value any
		d := json.NewDecoder(bytes.NewReader(r.Value))
		d.UseNumber()
		if e := d.Decode(&value); e != nil {
			return e
		}
		replaceExistingField(payload, r.Field, value)
	case "countries", "iso":
		vals, found := readField(payload, r.Field, "")
		if found && len(vals) > 0 {
			if country, ok := vals[0].(string); ok {
				normalized := strings.ToUpper(strings.TrimSpace(country))
				replacement, matched := r.Countries[normalized]
				if mode == "iso" {
					replacement, matched = isoAlpha3[normalized]
				}
				if matched {
					replaceExistingField(payload, r.Field, replacement)
				}
			}
		}
	case "hostname":
		site, ok := payload["site"].(map[string]any)
		if !ok {
			break
		}
		domain, _ := site["domain"].(string)
		page, _ := site["page"].(string)
		host := hostname(domain)
		if host == "" {
			host = hostname(page)
		}
		if host != "" {
			site["domain"] = host
		}
	case "full_page":
		site, ok := payload["site"].(map[string]any)
		if !ok {
			break
		}
		page, _ := site["page"].(string)
		domain, _ := site["domain"].(string)
		host := hostname(domain)
		if host == "" {
			host = hostname(page)
		}
		if full := fullPageURL(page, host); full != "" {
			site["page"] = full
		}
	default:
		return fmt.Errorf("unsupported replace mode %q", r.Mode)
	}
	if r.CopyTo != "" {
		copyTo(payload, r.Field, r.CopyTo)
	}
	return nil
}

func hostname(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "//" + strings.TrimPrefix(raw, "//")
	}
	u, e := url.Parse(raw)
	if e != nil {
		return ""
	}
	return strings.TrimSpace(u.Hostname())
}
func fullPageURL(rawPage, host string) string {
	page := strings.TrimSpace(rawPage)
	if page == "" {
		if host == "" {
			return ""
		}
		return "https://" + host + "/"
	}
	if u, e := url.Parse(page); e == nil && u.IsAbs() && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return page
	}
	if strings.HasPrefix(page, "//") {
		return "https:" + page
	}
	if strings.HasPrefix(page, "/") && host != "" {
		return "https://" + host + page
	}
	if !strings.Contains(page, "://") {
		return "https://" + strings.TrimPrefix(page, "//")
	}
	return page
}
