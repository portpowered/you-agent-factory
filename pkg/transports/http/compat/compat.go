// Package compat contains shared HTTP request compatibility helpers.
package compat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	// WarningCode is the RFC 9110 warning code for a response that succeeded
	// while applying a non-fatal compatibility interpretation.
	WarningCode = 299

	warningMessage                  = "ignored unknown request fields at "
	requestMultipleDocumentsMessage = "request payload must contain one JSON object"
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

// Paths returns a detached, deterministic copy of the ignored-field paths.
func (d Diagnostics) Paths() []string {
	return SortedUniquePaths(d.IgnoredJSONPaths)
}

// RequestFieldValidationError identifies a request-shape error whose stable
// message should be returned by the owning HTTP adapter.
type RequestFieldValidationError struct {
	Message string
}

func (e RequestFieldValidationError) Error() string {
	return e.Message
}

// DecodeResult carries a known request value and safe compatibility metadata.
type DecodeResult[T any] struct {
	Value       T
	Diagnostics Diagnostics
}

// Decode reads exactly one JSON value, decodes known fields into T, and
// reports unknown object-field paths. Standard encoding/json semantics remain
// authoritative for malformed input and known-field type errors.
func Decode[T any](body io.Reader) (DecodeResult[T], error) {
	var zero DecodeResult[T]
	data, err := io.ReadAll(body)
	if err != nil {
		return zero, err
	}
	return DecodeBytes[T](data)
}

// DecodeBytes is the byte-slice form of Decode used by optional request
// bodies after their empty-body policy has been applied.
func DecodeBytes[T any](data []byte) (DecodeResult[T], error) {
	var result DecodeResult[T]
	document, err := readSingleDocument(data)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(document, &result.Value); err != nil {
		return DecodeResult[T]{}, err
	}
	paths, err := collectUnknownJSONPaths(document, reflect.TypeOf((*T)(nil)).Elem())
	if err != nil {
		return DecodeResult[T]{}, err
	}
	result.Diagnostics = Diagnostics{IgnoredJSONPaths: paths}
	return result, nil
}

// DecodeOptional reads an optional JSON request. An empty or whitespace-only
// body returns zero without diagnostics; a non-empty body follows Decode.
func DecodeOptional[T any](body io.Reader, zero func() T) (DecodeResult[T], error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return DecodeResult[T]{}, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return DecodeResult[T]{Value: zero()}, nil
	}
	return DecodeBytes[T](data)
}

// SetWarningHeader adds the deterministic compatibility warning to a
// successful response. It leaves responses without ignored fields unchanged.
func SetWarningHeader(w http.ResponseWriter, paths []string) {
	paths = SortedUniquePaths(paths)
	if len(paths) == 0 || w == nil {
		return
	}
	warningText := warningMessage + strings.Join(paths, ", ")
	w.Header().Add("Warning", fmt.Sprintf("%d - %s", WarningCode, strconv.Quote(warningText)))
}

// LogWarning emits path-only structured metadata for an accepted request. The
// operation and boundary identify the owning public surface without retaining
// request values or secrets.
func LogWarning(logger *zap.Logger, boundary, operation string, paths []string) {
	paths = SortedUniquePaths(paths)
	if logger == nil || len(paths) == 0 {
		return
	}
	logger.Warn(
		"HTTP request ignored unknown fields",
		zap.Int("warning_code", WarningCode),
		zap.String("boundary", strings.TrimSpace(boundary)),
		zap.String("operation", strings.TrimSpace(operation)),
		zap.Strings("json_paths", paths),
	)
}

// ApplyWarning records both externally visible and operational compatibility
// diagnostics for a successful response.
func ApplyWarning(w http.ResponseWriter, logger *zap.Logger, boundary, operation string, paths []string) {
	paths = SortedUniquePaths(paths)
	if len(paths) == 0 {
		return
	}
	SetWarningHeader(w, paths)
	LogWarning(logger, boundary, operation, paths)
}

// SortedUniquePaths normalizes compatibility paths without preserving map
// iteration order or any ignored value.
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

func readSingleDocument(data []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var document json.RawMessage
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, RequestFieldValidationError{Message: requestMultipleDocumentsMessage}
		}
		return nil, err
	}
	return document, nil
}

func collectUnknownJSONPaths(document []byte, typ reflect.Type) ([]string, error) {
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
