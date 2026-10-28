package rtbrules

import (
	"fmt"
	"reflect"
	"strings"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

// protoMember resolves the original JSON name emitted for a protobuf field.
// The source path is matched against the explicit fields registry in schema.go
// before reaching these routines; reflection only locates the Go pointer.
func protoMember(t reflect.Type, name string) (int, reflect.Type, bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return 0, nil, false
	}
	for i := 0; i < t.NumField(); i++ {
		member := t.Field(i)
		if !member.IsExported() {
			continue
		}
		if strings.Split(member.Tag.Get("json"), ",")[0] == name {
			return i, member.Type, true
		}
	}
	return 0, nil, false
}

func protobufPathType(path string) (reflect.Type, bool) {
	t := reflect.TypeOf(ortb.BidRequest{})
	for _, part := range strings.Split(path, ".") {
		_, next, ok := protoMember(t, part)
		if !ok {
			return nil, false
		}
		t = next
	}
	return t, true
}

func isProtobufPath(path string) bool {
	_, ok := protobufPathType(path)
	return ok
}

// A delete target must be a *real pointer field*, not a slice, scalar alias,
// or a JSON-only key introduced by the rename module.
func validatePointerPath(path string) error {
	t, ok := protobufPathType(path)
	if !ok || t.Kind() != reflect.Pointer {
		return fmt.Errorf("delete field %q is not a protobuf pointer field", path)
	}
	return nil
}

// clearPointerPath mutates only the campaign-local proto.Clone; the source
// BidRequest is never modified. The final pointer is set to nil directly,
// regardless of its previous value. Missing parents are silently skipped.
func clearPointerPath(req *ortb.BidRequest, path string) {
	clearPointerValue(reflect.ValueOf(req), strings.Split(path, "."))
}

func clearPointerValue(v reflect.Value, parts []string) {
	if !v.IsValid() || len(parts) == 0 {
		return
	}
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			clearPointerValue(v.Index(i), parts)
		}
		return
	}
	if v.Kind() != reflect.Struct {
		return
	}
	index, _, ok := protoMember(v.Type(), parts[0])
	if !ok {
		return
	}
	target := v.Field(index)
	if len(parts) == 1 {
		// Guaranteed pointer by full-map validation; no nil check is needed.
		target.SetZero()
		return
	}
	clearPointerValue(target, parts[1:])
}
