// Package jsonvalue contains policy-free helpers for detached native JSON
// values and optional JSON object fields.
package jsonvalue

import (
	"bytes"
	"encoding/json"
)

// Clone returns a detached copy of a native JSON value. Values are expected to
// use the standard encoding/json shapes (map[string]any, []any, strings,
// numbers, booleans, and nil).
func Clone(value any) any {
	return clone(value)
}

// Present reports whether an optional JSON value should be serialized. The
// explicit bit is required only for a valid JSON null.
func Present(value any, explicit bool) bool {
	return explicit || value != nil
}

// MarshalOptionalField adds an optional native JSON field to an already
// encoded object, preserving the distinction between omission and JSON null.
func MarshalOptionalField(base any, value any, present bool, fieldName string) ([]byte, error) {
	encoded, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	if !Present(value, present) {
		return encoded, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, err
	}
	field, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	object[fieldName] = field
	return json.Marshal(object)
}

// UnmarshalOptionalField decodes an optional native JSON field and reports
// whether it was present, including when its value was null.
func UnmarshalOptionalField(data []byte, fieldName string) (any, bool, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, false, err
	}
	raw, present := object[fieldName]
	if !present {
		return nil, false, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, true, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func clone(value any) any {
	switch typed := value.(type) {
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = clone(item)
		}
		return cloned
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, item := range typed {
			cloned[key] = clone(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		for key, item := range typed {
			cloned[key] = item
		}
		return cloned
	case map[string][]string:
		cloned := make(map[string][]string, len(typed))
		for key, items := range typed {
			cloned[key] = append([]string(nil), items...)
		}
		return cloned
	default:
		return value
	}
}
