package rtbrules

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	jsoniter "github.com/json-iterator/go"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

// ErrNativeFilter marks an invalid embedded native.request JSON document or
// a JSON-path type mismatch. A normal filter non-match is NOT this error.
var ErrNativeFilter = errors.New("RTB native.request filter error")

func nativeFilterError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrNativeFilter, fmt.Sprintf(format, args...))
}

// FilterCache is scoped to a single inbound BidRequest. Its owner (ADV
// preflight) uses it sequentially, before launching campaign HTTP requests.
// The whole request is decoded once, each native.request string at most once
// per impression, and cached failures are reused too.
type FilterCache struct {
	request   *ortb.BidRequest
	raw       []byte
	decoded   bool
	root      map[string]any
	decodeErr error
	native    map[int]nativeCacheEntry
}

type nativeCacheEntry struct {
	document any
	err      error
}

func NewFilterCache(req *ortb.BidRequest) *FilterCache {
	return &FilterCache{request: req, native: make(map[int]nativeCacheEntry)}
}

func newFilterCacheFromJSON(raw []byte) *FilterCache {
	return &FilterCache{raw: raw, native: make(map[int]nativeCacheEntry)}
}

func (c *FilterCache) rootObject() (map[string]any, error) {
	if c == nil {
		return nil, fmt.Errorf("nil RTB filter cache")
	}
	if !c.decoded {
		c.decoded = true
		if c.request != nil {
			c.raw, c.decodeErr = jsoniter.Marshal(c.request)
		}
		if c.decodeErr == nil {
			c.root, c.decodeErr = decodeJSON(c.raw)
		}
	}
	return c.root, c.decodeErr
}

// Supports ordinary JSON keys, numeric indexes and a stable selector by
// native asset ID, e.g. assets[id=102].img.wmin or assets[0].title.len.
type nativeJSONPathStep struct {
	key      string
	index    int
	id       json.Number
	selector byte // 0: no selector, 'i': index, 'd': ID
}

var nativePathSegment = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)(?:\[(\d+|id=\d+)\])?$`)

func parseNativeJSONPath(path string) ([]nativeJSONPathStep, error) {
	if path == "" || strings.TrimSpace(path) != path {
		return nil, fmt.Errorf("empty or padded path")
	}
	parts := strings.Split(path, ".")
	steps := make([]nativeJSONPathStep, 0, len(parts))
	for _, part := range parts {
		match := nativePathSegment.FindStringSubmatch(part)
		if match == nil {
			return nil, fmt.Errorf("invalid segment %q", part)
		}
		step := nativeJSONPathStep{key: match[1]}
		if match[2] != "" {
			if strings.HasPrefix(match[2], "id=") {
				step.selector = 'd'
				step.id = json.Number(strings.TrimPrefix(match[2], "id="))
				if _, err := step.id.Int64(); err != nil {
					return nil, fmt.Errorf("invalid asset id in %q", part)
				}
			} else {
				step.selector = 'i'
				n, err := strconv.Atoi(match[2])
				if err != nil {
					return nil, fmt.Errorf("invalid index in %q", part)
				}
				step.index = n
			}
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func jsonScalarType(raw json.RawMessage) string {
	if validType(raw, "string") {
		return "string"
	}
	if validType(raw, "number") {
		return "number"
	}
	if validType(raw, "bool") {
		return "bool"
	}
	return ""
}

func nativeJSONType(v any) string {
	switch v.(type) {
	case json.Number:
		return "number"
	case string:
		return "string"
	case bool:
		return "bool"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}

func readNativeJSONPath(document any, steps []nativeJSONPathStep) (any, bool, error) {
	var current any = document
	for _, step := range steps {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false, nativeFilterError("expected JSON object before %s, got %s", step.key, nativeJSONType(current))
		}
		value, exists := obj[step.key]
		if !exists || value == nil {
			return nil, false, nil
		}
		if step.selector != 0 {
			list, ok := value.([]any)
			if !ok {
				return nil, false, nativeFilterError("%s must be an array, got %s", step.key, nativeJSONType(value))
			}
			if step.selector == 'i' {
				if step.index >= len(list) {
					return nil, false, nil
				}
				value = list[step.index]
			} else {
				found := false
				for _, element := range list {
					item, ok := element.(map[string]any)
					if !ok {
						return nil, false, nativeFilterError("%s contains non-object asset", step.key)
					}
					id, present := item["id"]
					if !present || id == nil {
						continue
					}
					num, ok := id.(json.Number)
					if !ok {
						return nil, false, nativeFilterError("%s asset id must be numeric", step.key)
					}
					if num.String() == step.id.String() {
						value, found = element, true
						break
					}
				}
				if !found {
					return nil, false, nil
				}
			}
			if value == nil {
				return nil, false, nil
			}
		}
		current = value
	}
	return current, true, nil
}

func parseNativeDocument(raw string) (any, error) {
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	var document any
	if err := d.Decode(&document); err != nil {
		return nil, nativeFilterError("invalid native.request JSON: %v", err)
	}
	if _, ok := document.(map[string]any); !ok {
		return nil, nativeFilterError("native.request must contain a JSON object")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, nativeFilterError("trailing data in native.request JSON")
	}
	return document, nil
}

func (c *FilterCache) nativeValues(impID string, path string) ([]any, bool, error) {
	root, err := c.rootObject()
	if err != nil {
		return nil, false, err
	}
	steps, err := parseNativeJSONPath(path)
	if err != nil {
		return nil, false, err
	} // Already validated at config load.
	imps, _ := root["imp"].([]any)
	var values []any
	foundAny := false
	for i, rawImp := range imps {
		imp, ok := rawImp.(map[string]any)
		if !ok || (impID != "" && imp["id"] != impID) {
			continue
		}
		native, _ := imp["native"].(map[string]any)
		rawRequest, exists := native["request"]
		if !exists || rawRequest == nil {
			continue
		}
		source, ok := rawRequest.(string)
		if !ok {
			return nil, false, nativeFilterError("imp.native.request must be a JSON string")
		}
		entry, cached := c.native[i]
		if !cached {
			entry.document, entry.err = parseNativeDocument(source)
			c.native[i] = entry
		}
		if entry.err != nil {
			return nil, false, entry.err
		}
		value, exists, err := readNativeJSONPath(entry.document, steps)
		if err != nil {
			return nil, false, err
		}
		if exists {
			foundAny = true
			values = append(values, explode(value)...)
		}
	}
	return values, foundAny, nil
}

func checkNumericComparison(left any, operation string, target json.RawMessage) (bool, error) {
	number, ok := left.(json.Number)
	if !ok {
		return false, nativeFilterError("numeric comparison requires number, got %s", nativeJSONType(left))
	}
	actual, err := number.Float64()
	if err != nil {
		return false, nativeFilterError("invalid numeric value: %v", err)
	}
	var want json.Number
	decoder := json.NewDecoder(bytes.NewReader(target))
	decoder.UseNumber()
	if err = decoder.Decode(&want); err != nil {
		return false, err
	}
	expected, err := want.Float64()
	if err != nil {
		return false, err
	}
	switch operation {
	case ">":
		return actual > expected, nil
	case ">=":
		return actual >= expected, nil
	case "<":
		return actual < expected, nil
	case "<=":
		return actual <= expected, nil
	}
	return false, fmt.Errorf("unknown numeric comparison %q", operation)
}
