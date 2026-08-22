package globalconfig

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// PreserveUnknownFields overlays the canonical representation of an updated
// GlobalConfig onto the unknown fields from the existing representation. The
// generated domain mapping intentionally does not expose those fields, so the
// merge happens only at this serialization boundary.
func PreserveUnknownFields(original, canonical []byte) ([]byte, error) {
	originalValue, err := decodeOneJSONValue(original)
	if err != nil {
		return nil, fmt.Errorf("decode existing global config: %w", err)
	}
	canonicalValue, err := decodeOneJSONValue(canonical)
	if err != nil {
		return nil, fmt.Errorf("decode canonical global config: %w", err)
	}
	originalObject, ok := originalValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("decode existing global config: expected a JSON object")
	}
	canonicalObject, ok := canonicalValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("decode canonical global config: expected a JSON object")
	}

	merged := mergePreservedJSONValue(
		originalObject,
		canonicalObject,
		reflect.TypeOf(factoryapi.GlobalConfig{}),
	)
	payload, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode preserved global config: %w", err)
	}
	return append(payload, '\n'), nil
}

func mergePreservedJSONValue(original, canonical any, typ reflect.Type) any {
	if original == nil || canonical == nil || typ == nil {
		return canonical
	}
	typ = dereferenceJSONType(typ)
	if typ == globalConfigRawMessageType || typ.Kind() == reflect.Interface {
		return canonical
	}

	switch typ.Kind() {
	case reflect.Map:
		return mergePreservedJSONMap(original, canonical, typ)
	case reflect.Slice, reflect.Array:
		return mergePreservedJSONSequence(original, canonical, typ)
	case reflect.Struct:
		return mergePreservedJSONStruct(original, canonical, typ)
	default:
		return canonical
	}
}

func mergePreservedJSONMap(original, canonical any, typ reflect.Type) any {
	originalObject, originalOK := original.(map[string]any)
	canonicalObject, canonicalOK := canonical.(map[string]any)
	if !originalOK || !canonicalOK || typ.Key().Kind() != reflect.String {
		return canonical
	}

	merged := make(map[string]any, len(canonicalObject))
	for key, canonicalChild := range canonicalObject {
		if originalChild, ok := originalObject[key]; ok {
			merged[key] = mergePreservedJSONValue(originalChild, canonicalChild, typ.Elem())
			continue
		}
		merged[key] = canonicalChild
	}
	return merged
}

func mergePreservedJSONSequence(original, canonical any, typ reflect.Type) any {
	originalValues, originalOK := original.([]any)
	canonicalValues, canonicalOK := canonical.([]any)
	if !originalOK || !canonicalOK {
		return canonical
	}

	merged := make([]any, len(canonicalValues))
	for index, canonicalChild := range canonicalValues {
		if index < len(originalValues) {
			merged[index] = mergePreservedJSONValue(originalValues[index], canonicalChild, typ.Elem())
			continue
		}
		merged[index] = canonicalChild
	}
	return merged
}

func mergePreservedJSONStruct(original, canonical any, typ reflect.Type) any {
	originalObject, originalOK := original.(map[string]any)
	canonicalObject, canonicalOK := canonical.(map[string]any)
	if !originalOK || !canonicalOK {
		return canonical
	}

	fields := jsonFields(typ)
	merged := make(map[string]any, len(canonicalObject)+len(originalObject))
	for key, originalChild := range originalObject {
		if _, known := fields[strings.ToLower(key)]; !known {
			merged[key] = originalChild
		}
	}

	for key, canonicalChild := range canonicalObject {
		field, known := fields[strings.ToLower(key)]
		if !known {
			merged[key] = canonicalChild
			continue
		}
		if originalChild, ok := matchingJSONField(originalObject, key); ok {
			merged[key] = mergePreservedJSONValue(originalChild, canonicalChild, field.typ)
			continue
		}
		merged[key] = canonicalChild
	}

	// A known object can be omitted by the canonical encoder when it only
	// contained fields outside the current domain model. Recreate just that
	// object's unknown portion so a semantic update does not erase it.
	for key, originalChild := range originalObject {
		field, known := fields[strings.ToLower(key)]
		if !known {
			continue
		}
		if _, present := matchingJSONField(canonicalObject, field.name); present {
			continue
		}
		projected, keep := projectUnknownJSONValue(originalChild, field.typ)
		if keep {
			merged[field.name] = projected
		}
	}
	return merged
}

func matchingJSONField(object map[string]any, name string) (any, bool) {
	if value, ok := object[name]; ok {
		return value, true
	}
	for key, value := range object {
		if strings.EqualFold(key, name) {
			return value, true
		}
	}
	return nil, false
}

func projectUnknownJSONValue(value any, typ reflect.Type) (any, bool) {
	if value == nil || typ == nil {
		return nil, false
	}
	typ = dereferenceJSONType(typ)
	if typ == globalConfigRawMessageType || typ.Kind() == reflect.Interface {
		return nil, false
	}

	switch typ.Kind() {
	case reflect.Struct:
		return projectUnknownJSONStruct(value, typ)
	case reflect.Map:
		return projectUnknownJSONMap(value, typ)
	default:
		return nil, false
	}
}

func projectUnknownJSONStruct(value any, typ reflect.Type) (any, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	fields := jsonFields(typ)
	projected := make(map[string]any)
	for key, child := range object {
		field, known := fields[strings.ToLower(key)]
		if !known {
			projected[key] = child
			continue
		}
		childProjection, keep := projectUnknownJSONValue(child, field.typ)
		if keep {
			projected[field.name] = childProjection
		}
	}
	return projected, len(projected) != 0
}

func projectUnknownJSONMap(value any, typ reflect.Type) (any, bool) {
	object, ok := value.(map[string]any)
	if !ok || typ.Key().Kind() != reflect.String {
		return nil, false
	}
	projected := make(map[string]any)
	for key, child := range object {
		childProjection, keep := projectUnknownJSONValue(child, typ.Elem())
		if keep {
			projected[key] = childProjection
		}
	}
	return projected, len(projected) != 0
}
