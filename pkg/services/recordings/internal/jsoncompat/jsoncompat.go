// Package jsoncompat contains policy-free helpers for forward-compatible JSON
// document boundaries owned by Recordings.
package jsoncompat

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
)

var (
	rawMessageType = reflect.TypeOf(json.RawMessage{})
	timeType       = reflect.TypeOf(time.Time{})
)

// Diagnostics contains only safe paths for object fields that were not known
// by the decode target. Ignored values are intentionally never retained.
type Diagnostics struct {
	IgnoredJSONPaths []string
}

// Paths returns a detached, deterministic copy of ignored JSON paths.
func (diagnostics Diagnostics) Paths() []string {
	return SortedUniquePaths(diagnostics.IgnoredJSONPaths)
}

// Decode reads exactly one JSON document, decodes it into target, and reports
// unknown object-field paths. Standard encoding/json semantics continue to
// reject malformed input and values whose types do not match target.
func Decode(data []byte, target any) (Diagnostics, error) {
	document, err := ReadSingleDocument(bytes.NewReader(data))
	if err != nil {
		return Diagnostics{}, err
	}
	return DecodeDocument(document, target)
}

// DecodeDocument decodes one already isolated JSON document and reports
// unknown object-field paths. The caller remains responsible for any
// domain-level validation after decoding.
func DecodeDocument(document []byte, target any) (Diagnostics, error) {
	if target == nil {
		return Diagnostics{}, fmt.Errorf("JSON decode target is required")
	}
	if err := json.Unmarshal(document, target); err != nil {
		return Diagnostics{}, err
	}
	typ := reflect.TypeOf(target)
	if typ.Kind() != reflect.Pointer || typ.Elem() == nil {
		return Diagnostics{}, fmt.Errorf("JSON decode target must be a non-nil pointer")
	}
	paths, err := CollectUnknownJSONPathsFromDocument(document, typ.Elem())
	if err != nil {
		return Diagnostics{}, err
	}
	return Diagnostics{IgnoredJSONPaths: paths}, nil
}

// ReadSingleDocument reads exactly one well-formed JSON document from reader.
// It does not apply an object schema; callers decode the returned bytes into
// their known contract separately.
func ReadSingleDocument(reader io.Reader) ([]byte, error) {
	if reader == nil {
		return nil, fmt.Errorf("JSON reader is required")
	}
	decoder := json.NewDecoder(reader)
	var document json.RawMessage
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("JSON document must contain exactly one value")
		}
		return nil, err
	}
	return document, nil
}

// CollectUnknownJSONPaths reports unknown object fields in one JSON document
// for the supplied Go JSON target type. RawMessage and interface values are
// opaque by design because their schemas are owned by another contract.
func CollectUnknownJSONPaths(data []byte, typ reflect.Type) ([]string, error) {
	document, err := ReadSingleDocument(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return CollectUnknownJSONPathsFromDocument(document, typ)
}

// CollectUnknownJSONPathsFromDocument is the isolated-document form of
// CollectUnknownJSONPaths.
func CollectUnknownJSONPathsFromDocument(document []byte, typ reflect.Type) ([]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var paths []string
	collect(value, typ, "$", &paths)
	return SortedUniquePaths(paths), nil
}

// PrefixPaths attaches a logical JSON path prefix to paths rooted at $. It is
// used when a nested raw document is decoded by its owning contract.
func PrefixPaths(prefix string, paths []string) []string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return SortedUniquePaths(paths)
	}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if path == "$" {
			result = append(result, prefix)
			continue
		}
		result = append(result, prefix+strings.TrimPrefix(path, "$"))
	}
	return SortedUniquePaths(result)
}

// SortedUniquePaths normalizes compatibility paths without preserving any
// ignored value or map iteration order.
func SortedUniquePaths(paths []string) []string {
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

func collect(value any, typ reflect.Type, path string, paths *[]string) {
	if value == nil || typ == nil {
		return
	}
	typ = dereference(typ)
	if typ == nil || typ == rawMessageType || typ == timeType {
		return
	}

	switch typ.Kind() {
	case reflect.Map:
		collectMap(value, typ, path, paths)
	case reflect.Slice, reflect.Array:
		collectSequence(value, typ, path, paths)
	case reflect.Struct:
		collectStruct(value, typ, path, paths)
	}
}

func collectMap(value any, typ reflect.Type, path string, paths *[]string) {
	object, ok := value.(map[string]any)
	if !ok || typ.Key().Kind() != reflect.String {
		return
	}
	for key, child := range object {
		collect(child, typ.Elem(), appendPath(path, key), paths)
	}
}

func collectSequence(value any, typ reflect.Type, path string, paths *[]string) {
	values, ok := value.([]any)
	if !ok {
		return
	}
	for index, item := range values {
		collect(item, typ.Elem(), path+"["+strconv.Itoa(index)+"]", paths)
	}
}

func collectStruct(value any, typ reflect.Type, path string, paths *[]string) {
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	fields := jsonFieldTypes(typ)
	for key, child := range object {
		fieldPath := appendPath(path, key)
		fieldType, known := fields[strings.ToLower(key)]
		if !known {
			*paths = append(*paths, fieldPath)
			continue
		}
		collect(child, fieldType, fieldPath, paths)
	}
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
		if name == "" && field.Anonymous {
			embedded := dereference(field.Type)
			if embedded != nil && embedded.Kind() == reflect.Struct && embedded != timeType {
				for key, fieldType := range jsonFieldTypes(embedded) {
					if _, exists := fields[key]; !exists {
						fields[key] = fieldType
					}
				}
				continue
			}
		}
		if name == "" {
			name = field.Name
		}
		fields[strings.ToLower(name)] = field.Type
	}
	return fields
}

func dereference(typ reflect.Type) reflect.Type {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

func appendPath(path, key string) string {
	if isSimplePathKey(key) {
		return path + "." + key
	}
	return path + "[" + strconv.Quote(key) + "]"
}

func isSimplePathKey(key string) bool {
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
