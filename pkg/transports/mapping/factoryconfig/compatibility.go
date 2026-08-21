package factoryconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactoryDecodeDiagnostics contains safe, representation-independent
// diagnostics produced while decoding one customer-authored Factory payload.
// It records paths only; ignored values are never retained.
type FactoryDecodeDiagnostics struct {
	IgnoredJSONPaths []string
}

// Paths returns a detached, deterministic copy of the ignored-field paths.
func (d FactoryDecodeDiagnostics) Paths() []string {
	return sortedUniqueJSONPaths(d.IgnoredJSONPaths)
}

var (
	timeType       = reflect.TypeOf(time.Time{})
	rawMessageType = reflect.TypeOf(json.RawMessage{})
)

func collectUnknownFactoryJSONPaths(data []byte) ([]string, error) {
	value, err := decodeOneJSONValue(data)
	if err != nil {
		return nil, err
	}
	var paths []string
	collectUnknownJSONPaths(value, reflect.TypeOf(factoryapi.Factory{}), "$", &paths)
	return sortedUniqueJSONPaths(paths), nil
}

func decodeOneJSONValue(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing JSON value")
		}
		return nil, err
	}
	return value, nil
}

func collectUnknownJSONPaths(value any, typ reflect.Type, path string, paths *[]string) {
	if value == nil || typ == nil {
		return
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if isOpaqueJSONType(typ) {
		return
	}

	switch typ.Kind() {
	case reflect.Interface, reflect.Map:
		return
	case reflect.Slice, reflect.Array:
		values, ok := value.([]any)
		if !ok {
			return
		}
		for index, item := range values {
			collectUnknownJSONPaths(
				item,
				typ.Elem(),
				path+"["+strconv.Itoa(index)+"]",
				paths,
			)
		}
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return
		}
		fields := jsonFieldTypes(typ)
		for key, child := range object {
			fieldType, known := fields[strings.ToLower(key)]
			fieldPath := appendJSONPath(path, key)
			if !known {
				*paths = append(*paths, fieldPath)
				continue
			}
			collectUnknownJSONPaths(child, fieldType, fieldPath, paths)
		}
	}
}

func isOpaqueJSONType(typ reflect.Type) bool {
	return typ == timeType || typ == rawMessageType
}

func jsonFieldTypes(typ reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			name = field.Name
		}
		fields[strings.ToLower(name)] = field.Type
	}
	return fields
}

func appendJSONPath(path, key string) string {
	if isSimpleJSONPathKey(key) {
		return path + "." + key
	}
	return path + "[" + strconv.Quote(key) + "]"
}

func isSimpleJSONPathKey(key string) bool {
	if key == "" {
		return false
	}
	for index, character := range key {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9' && index > 0) ||
			character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func sortedUniqueJSONPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			unique[path] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return nil
	}
	result := make([]string, 0, len(unique))
	for path := range unique {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}
