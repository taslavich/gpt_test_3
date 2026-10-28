package rtbrules

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

// addProtobufField fills only absent protobuf fields, before serializing the
// campaign-local request to JSON. All paths were validated against the schema.
func addProtobufField(req *ortb.BidRequest, path string, raw json.RawMessage) error {
	return fillProtobufValue(reflect.ValueOf(req), strings.Split(path, "."), raw)
}

func fillProtobufValue(v reflect.Value, parts []string, raw json.RawMessage) error {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			if err := fillProtobufValue(v.Index(i), parts, raw); err != nil {
				return err
			}
		}
		return nil
	}
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("add parent is not a protobuf object")
	}
	index, _, ok := protoMember(v.Type(), parts[0])
	if !ok {
		return fmt.Errorf("unknown protobuf field %q", parts[0])
	}
	field := v.Field(index)
	if len(parts) != 1 {
		return fillProtobufValue(field, parts[1:], raw)
	}
	// Optional protobuf pointers and nil arrays/objects are absent; non-nil
	// values, including 0, false and empty strings, are already present.
	if !field.IsZero() {
		return nil
	}
	value := reflect.New(field.Type())
	if err := json.Unmarshal(raw, value.Interface()); err != nil {
		return fmt.Errorf("add protobuf %q: %w", parts[0], err)
	}
	field.Set(value.Elem())
	return nil
}
