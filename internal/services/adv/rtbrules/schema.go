// Package rtbrules implements campaign-scoped OpenRTB checks and outbound JSON
// transformations. It does not alter the original request or existing ADV filters.
package rtbrules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type RuleSet struct {
	Filters []Filter  `json:"filters,omitempty"`
	Add     []Add     `json:"add,omitempty"`
	Replace []Replace `json:"replace,omitempty"`
	Delete  []Delete  `json:"delete,omitempty"`
	Rename  []Rename  `json:"rename,omitempty"`
}
type Filter struct {
	Field       string            `json:"field"`
	IsWhiteList *bool             `json:"isWhiteList,omitempty"`
	Values      []json.RawMessage `json:"values,omitempty"`
	Exists      *bool             `json:"exists,omitempty"`
}

// Filter mode is inferred from the presence of exactly one of values or exists.
// Validate source keys at map load/PUT time, not on the request hot path.
func (f *Filter) UnmarshalJSON(data []byte) error {
	type filterAlias Filter
	var wire filterAlias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}
	for _, key := range []string{"values", "exists"} {
		if value, present := keys[key]; present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("filter %s cannot be null", key)
		}
	}
	if _, values := keys["values"]; values {
		if _, exists := keys["exists"]; exists {
			return fmt.Errorf("filter cannot combine values and exists")
		}
	}
	*f = Filter(wire)
	return nil
}

type Add struct {
	Field        string          `json:"field"`
	PostfixField string          `json:"postfix_field,omitempty"`
	Value        json.RawMessage `json:"value"`
}

// An add has no mode. A postfix is the exact JSON-only suffix to create.
// Reject obsolete/unknown settings at load and HTTP PUT time.
func (a *Add) UnmarshalJSON(data []byte) error {
	type alias Add
	var value alias
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&value); err != nil {
		return err
	}
	var explicit map[string]json.RawMessage
	if err := json.Unmarshal(data, &explicit); err != nil {
		return err
	}
	if raw, ok := explicit["postfix_field"]; ok && (bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || value.PostfixField == "") {
		return fmt.Errorf("postfix_field must be a nonempty path")
	}
	*a = Add(value)
	return nil
}

type Replace struct {
	Field     string            `json:"field"`
	Mode      string            `json:"mode,omitempty"` // set,iso,hostname,full_page; countries uses countries key
	Value     json.RawMessage   `json:"value,omitempty"`
	Countries map[string]string `json:"countries,omitempty"`
	CopyTo    string            `json:"copy_to,omitempty"`
}

// Countries is selected by the countries key alone. Its presence is mutually
// exclusive with mode (including mode:iso), even if the map is empty.
func (r *Replace) UnmarshalJSON(data []byte) error {
	type replaceAlias Replace
	var wire replaceAlias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}
	if countries, ok := keys["countries"]; ok {
		if _, mode := keys["mode"]; mode {
			return fmt.Errorf("replace cannot combine countries and mode")
		}
		if bytes.Equal(bytes.TrimSpace(countries), []byte("null")) {
			return fmt.Errorf("replace countries cannot be null")
		}
	}
	if copyTo, ok := keys["copy_to"]; ok {
		if bytes.Equal(bytes.TrimSpace(copyTo), []byte("null")) || wire.CopyTo == "" {
			return fmt.Errorf("replace copy_to requires a supported destination field")
		}
	}
	*r = Replace(wire)
	return nil
}

type Delete struct {
	Field string `json:"field"`
}
type Rename struct {
	Field   string `json:"field"`
	ToField string `json:"to_field"`
}

// Whitelist of supported, explicitly named JSON paths, including only fields
// that the ADV OpenRTB representation can serialize or this module can add.
// Parent object paths are allowed for deletion, not arbitrary additions.
var fields = map[string]string{
	"id": "string", "test": "number", "at": "number", "tmax": "number", "cur": "strings", "bcat": "strings",
	"device": "object", "device.ip": "string", "device.ipv6": "string", "device.ua": "string", "device.geo": "object",
	"device.geo.country": "string", "device.geo.region": "string", "device.geo.city": "string", "device.geo.zip": "string", "device.geo.lat": "number", "device.geo.lon": "number", "device.geo.type": "number",
	"device.deviceType": "number", "device.devicetype": "number", "device.js": "number", "device.language": "string", "device.os": "string", "device.make": "string", "device.model": "string", "device.osv": "string", "device.w": "number", "device.h": "number", "device.connectiontype": "number", "device.carrier": "string",
	"site": "object", "site.id": "string", "site.name": "string", "site.domain": "string", "site.page": "string", "site.ref": "string", "site.cat": "strings", "site.keywords": "string",
	"site.publisher": "object", "site.publisher.id": "string", "site.publisher.name": "string", "site.publisher.domain": "string",
	"user": "object", "user.id": "string", "user.keywords": "string", "user.buyeruid": "string", "user.geo": "object", "user.geo.country": "string", "user.geo.city": "string", "user.geo.region": "string", "user.geo.zip": "string", "user.geo.lat": "number", "user.geo.lon": "number", "user.geo.type": "number",
	"imp.id": "string", "imp.tagid": "string", "imp.bidfloor": "number", "imp.bidfloorcur": "string", "imp.secure": "number", "imp.instl": "number", "imp.ext": "object", "imp.ext.subid": "string",
	"imp.banner": "object", "imp.banner.w": "number", "imp.banner.h": "number", "imp.banner.pos": "number", "imp.banner.mimes": "strings", "imp.banner.topframe": "number", "imp.banner.btype": "numbers", "imp.banner.battr": "numbers",
	"imp.native": "object", "imp.native.ver": "string", "imp.native.request": "string",
	"imp.video": "object", "imp.video.mimes": "strings", "imp.video.protocols": "numbers", "imp.video.w": "number", "imp.video.h": "number", "imp.video.linearity": "number", "imp.video.boxingallowed": "number", "imp.video.skipmin": "number", "imp.video.skipafter": "number", "imp.video.skip": "number", "imp.video.minduration": "number", "imp.video.maxduration": "number", "imp.video.placement": "number", "imp.video.plcmt": "number", "imp.video.sequence": "number",
}

func validateField(path string) (string, error) {
	if strings.TrimSpace(path) != path {
		return "", fmt.Errorf("field %q contains surrounding whitespace", path)
	}
	typ, ok := fields[path]
	if !ok {
		return "", fmt.Errorf("unsupported OpenRTB field %q", path)
	}
	return typ, nil
}
func validType(v json.RawMessage, typ string) bool {
	if len(v) == 0 || string(v) == "null" {
		return false
	}
	var x any
	d := json.NewDecoder(strings.NewReader(string(v)))
	d.UseNumber()
	if d.Decode(&x) != nil {
		return false
	}
	switch typ {
	case "string":
		_, ok := x.(string)
		return ok
	case "number":
		_, ok := x.(json.Number)
		return ok
	case "bool":
		_, ok := x.(bool)
		return ok
	case "object":
		_, ok := x.(map[string]any)
		return ok
	case "strings":
		a, ok := x.([]any)
		if !ok {
			return false
		}
		for _, it := range a {
			if _, ok := it.(string); !ok {
				return false
			}
		}
		return true
	case "numbers":
		a, ok := x.([]any)
		if !ok {
			return false
		}
		for _, it := range a {
			if _, ok := it.(json.Number); !ok {
				return false
			}
		}
		return true
	}
	return false
}

// Only original protobuf field paths may be referenced before rename.
// The rename destination may use a supported alternate wire spelling.
func validateSourceField(path string) (string, error) {
	typ, err := validateField(path)
	if err != nil {
		return "", err
	}
	if !isProtobufPath(path) {
		return "", fmt.Errorf("field %q is not an original protobuf field", path)
	}
	return typ, nil
}
func validateSet(rs RuleSet) error {
	for _, f := range rs.Filters {
		typ, e := validateSourceField(f.Field)
		if e != nil {
			return e
		}
		switch {
		case f.Exists != nil && f.Values == nil:
			if f.IsWhiteList != nil {
				return fmt.Errorf("exists filter %s must not specify isWhiteList", f.Field)
			}
		case f.Values != nil && f.Exists == nil:
			if f.IsWhiteList == nil || len(f.Values) == 0 || typ == "object" {
				return fmt.Errorf("value filter %s requires isWhiteList and nonempty values (scalar/array field)", f.Field)
			}
			valTyp := typ
			if typ == "strings" {
				valTyp = "string"
			}
			if typ == "numbers" {
				valTyp = "number"
			}
			for _, v := range f.Values {
				if !validType(v, valTyp) {
					return fmt.Errorf("invalid filter value for %s (expected %s)", f.Field, valTyp)
				}
			}
		default:
			return fmt.Errorf("filter %s must specify exactly one of values or exists", f.Field)
		}
	}
	for _, a := range rs.Add {
		typ, e := validateSourceField(a.Field)
		if e != nil {
			return e
		}
		if a.PostfixField != "" {
			if typ != "object" || !validPostfix(a.PostfixField) || postfixConflictsWithProtobuf(a.Field, a.PostfixField) {
				return fmt.Errorf("postfix_field %q requires a protobuf object field, got %s", a.PostfixField, a.Field)
			}
			if len(a.Value) == 0 || bytes.Equal(bytes.TrimSpace(a.Value), []byte("null")) || !json.Valid(a.Value) {
				return fmt.Errorf("invalid postfix add value for %s", a.Field)
			}
			continue
		}
		if !validType(a.Value, typ) || (typ == "object" && !validNestedObject(a.Field, a.Value)) {
			return fmt.Errorf("invalid add value for %s", a.Field)
		}
	}
	// Two JSON-only extensions may share object parents, but an extension
	// cannot turn another extension's scalar leaf into a parent object.
	for i, a := range rs.Add {
		if a.PostfixField == "" {
			continue
		}
		ap := a.Field + "." + a.PostfixField
		for _, b := range rs.Add[i+1:] {
			if b.PostfixField == "" {
				continue
			}
			bp := b.Field + "." + b.PostfixField
			if ap != bp && overlaps(ap, bp) {
				return fmt.Errorf("conflicting JSON postfix paths %s and %s", ap, bp)
			}
		}
	}
	for _, r := range rs.Replace {
		typ, e := validateSourceField(r.Field)
		if e != nil {
			return e
		}
		mode := r.Mode
		if r.Countries != nil {
			mode = "countries"
		}
		switch mode {
		case "set":
			if !validType(r.Value, typ) || (typ == "object" && !validNestedObject(r.Field, r.Value)) {
				return fmt.Errorf("invalid replacement value for %s", r.Field)
			}
		case "countries", "iso":
			if typ != "string" || !(r.Field == "device.geo.country" || r.Field == "user.geo.country") {
				return fmt.Errorf("%s only allowed for geo country paths", mode)
			}
			if mode == "countries" {
				if len(r.Countries) == 0 {
					return fmt.Errorf("empty countries for %s", r.Field)
				}
				for a, b := range r.Countries {
					if len(a) != 2 || len(b) != 3 || strings.ToUpper(a) != a || strings.ToUpper(b) != b {
						return fmt.Errorf("invalid country substitution %s -> %s", a, b)
					}
				}
			}
		case "hostname":
			if r.Field != "site.domain" {
				return fmt.Errorf("hostname only supports site.domain")
			}
		case "full_page":
			if r.Field != "site.page" {
				return fmt.Errorf("full_page only supports site.page")
			}
		default:
			return fmt.Errorf("unknown replace mode %q", r.Mode)
		}
		if mode != "set" && len(r.Value) != 0 {
			return fmt.Errorf("unexpected replacement value for %s", r.Field)
		}
		if mode != "countries" && r.Countries != nil {
			return fmt.Errorf("unexpected countries for %s", r.Field)
		}
		if r.CopyTo != "" {
			toType, err := validateSourceField(r.CopyTo)
			if err != nil {
				return fmt.Errorf("invalid copy_to target for %s: %w", r.Field, err)
			}
			if typ != toType || !compatibleCopyPaths(r.Field, r.CopyTo) {
				return fmt.Errorf("copy_to fields %s -> %s have incompatible types", r.Field, r.CopyTo)
			}
			// The source impression cannot be determined uniquely when the
			// destination is outside imp and multiple impressions are present.
			if strings.HasPrefix(r.Field, "imp.") && !strings.HasPrefix(r.CopyTo, "imp.") {
				return fmt.Errorf("copy_to from impression to root is ambiguous: %s -> %s", r.Field, r.CopyTo)
			}
		}
	}
	for _, d := range rs.Delete {
		if _, e := validateSourceField(d.Field); e != nil {
			return e
		}
		if e := validatePointerPath(d.Field); e != nil {
			return e
		}
		// Deletion is performed on the protobuf copy before JSON encoding.
		// Prevent another module from re-adding or mutating the same path.
		for _, a := range rs.Add {
			addPath := a.Field
			if a.PostfixField != "" {
				addPath += "." + a.PostfixField
			}
			if overlaps(d.Field, addPath) {
				return fmt.Errorf("delete %s conflicts with add %s", d.Field, addPath)
			}
		}
		for _, r := range rs.Replace {
			if overlaps(d.Field, r.Field) || (r.CopyTo != "" && overlaps(d.Field, r.CopyTo)) {
				return fmt.Errorf("delete %s conflicts with replace %s", d.Field, r.Field)
			}
		}
		for _, r := range rs.Rename {
			if overlaps(d.Field, r.Field) || overlaps(d.Field, renamedPath(r.Field, r.ToField)) {
				return fmt.Errorf("delete %s conflicts with rename %s", d.Field, r.Field)
			}
		}
	}
	for _, r := range rs.Rename {
		typ, e := validateSourceField(r.Field)
		if e != nil {
			return e
		}
		if r.ToField == "" || strings.Contains(r.ToField, ".") {
			return fmt.Errorf("rename destination must be a terminal field name, got %q", r.ToField)
		}
		toPath := renamedPath(r.Field, r.ToField)
		toType, e := validateField(toPath)
		if e != nil {
			return e
		}
		if typ != toType || r.Field == toPath {
			return fmt.Errorf("invalid rename %s -> %s", r.Field, toPath)
		}
		// A rename must operate on an original protobuf key, never on an
		// already renamed JSON-only key.
		if !isProtobufPath(r.Field) {
			return fmt.Errorf("rename source %s is not a protobuf field", r.Field)
		}
		for _, a := range rs.Add {
			if a.PostfixField != "" && overlaps(a.Field+"."+a.PostfixField, toPath) {
				return fmt.Errorf("rename destination %s conflicts with JSON-only add %s.%s", toPath, a.Field, a.PostfixField)
			}
		}
	}

	return nil
}

// A postfix is a sequence of new JSON object keys, not a protobuf path.
// No blank segments, indexing, or conflicting parent assignment is allowed.
func validPostfix(path string) bool {
	if path == "" || strings.TrimSpace(path) != path {
		return false
	}
	for _, segment := range strings.Split(path, ".") {
		if segment == "" || strings.TrimSpace(segment) != segment || strings.ContainsAny(segment, "\t\r\n") {
			return false
		}
	}
	return true
}

// A JSON-only extension cannot overwrite an existing protobuf field or
// pass through a scalar protobuf member of its parent object.
func postfixConflictsWithProtobuf(base, postfix string) bool {
	path := base
	parts := strings.Split(postfix, ".")
	for i, part := range parts {
		path += "." + part
		if protoTyp, ok := protobufPathType(path); ok {
			if i == len(parts)-1 {
				return true
			}
			for protoTyp.Kind() == reflect.Pointer || protoTyp.Kind() == reflect.Slice {
				protoTyp = protoTyp.Elem()
			}
			if protoTyp.Kind() != reflect.Struct {
				return true
			}
		}
	}
	return false
}

// Compound objects may be added only when every nested member is another
// explicit allowlisted field, with a type-valid value.
func validNestedObject(path string, raw json.RawMessage) bool {
	var obj map[string]json.RawMessage
	if e := json.Unmarshal(raw, &obj); e != nil || obj == nil {
		return false
	}
	for key, val := range obj {
		field := path + "." + key
		typ, ok := fields[field]
		if !ok || !validType(val, typ) {
			return false
		}
		if typ == "object" && !validNestedObject(field, val) {
			return false
		}
	}
	return true
}

// Copying fields is type-preserving. Scalar/array JSON types must match.
// Compound protobuf messages must also have the *same actual Go type*, not
// just the common JSON classification "object".
func compatibleCopyPaths(from, to string) bool {
	fromType, fromOK := protobufPathType(from)
	toType, toOK := protobufPathType(to)
	if !fromOK || !toOK {
		return false
	}
	for fromType.Kind() == reflect.Pointer {
		fromType = fromType.Elem()
	}
	for toType.Kind() == reflect.Pointer {
		toType = toType.Elem()
	}
	// Identical types are trivially compatible. Different protobuf structs,
	// integer widths, or float/int representations cannot be copied blindly.
	return fromType == toType
}

func renamedPath(field, toField string) string {
	parent := parentPath(field)
	if parent == "" {
		return toField
	}
	return parent + "." + toField
}

func parentPath(field string) string {
	i := strings.LastIndex(field, ".")
	if i < 0 {
		return ""
	}
	return field[:i]
}

func overlaps(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+".") || strings.HasPrefix(b, a+".")
}
