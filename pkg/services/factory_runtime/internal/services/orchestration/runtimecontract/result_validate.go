package runtimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// ValidateTypedValue checks that one workflow return/final value is
// structured-cloneable and JSON-compatible for MVP result projection.
func ValidateTypedValue(value TypedValue) ResultValidation {
	var issues []ResultIssue
	if value.Unresolved {
		issues = append(issues, ResultIssue{
			Code:    CodeUnresolvedPromise,
			Message: "workflow result cannot include an unresolved promise",
			Path:    "$",
		})
	}
	if value.Function {
		issues = append(issues, ResultIssue{
			Code:    CodeUnsupportedType,
			Message: "workflow result cannot include a function value",
			Path:    "$",
		})
	}
	if handle := strings.TrimSpace(value.HostHandle); handle != "" {
		issues = append(issues, ResultIssue{
			Code:    CodeHostHandle,
			Message: fmt.Sprintf("workflow result cannot include host handle %q", handle),
			Path:    "$",
		})
	}
	if len(value.RawBinary) > 0 {
		issues = append(issues, ResultIssue{
			Code:    CodeUnsupportedBinary,
			Message: "workflow result cannot embed raw binary blobs; use workflow.artifact instead",
			Path:    "$",
		})
	}
	if value.HostObject != nil {
		issues = append(issues, walkHostObject(value.HostObject, "$", value.visited())...)
	}
	if len(value.JSON) == 0 {
		if len(issues) > 0 {
			return ResultValidation{Issues: issues}
		}
		return ResultValidation{}
	}
	if !json.Valid(value.JSON) {
		issues = append(issues, ResultIssue{
			Code:    CodeNonJSONValue,
			Message: "workflow result must be JSON-compatible",
			Path:    "$",
		})
		return ResultValidation{Issues: issues}
	}
	var decoded any
	if err := json.Unmarshal(value.JSON, &decoded); err != nil {
		issues = append(issues, ResultIssue{
			Code:    CodeNonJSONValue,
			Message: "workflow result must be JSON-compatible: " + err.Error(),
			Path:    "$",
		})
		return ResultValidation{Issues: issues}
	}
	issues = append(issues, walkJSONValue(decoded, "$")...)
	return ResultValidation{Issues: issues}
}

func (v TypedValue) visited() map[uintptr]struct{} {
	if v.Visited != nil {
		return v.Visited
	}
	return make(map[uintptr]struct{})
}

func walkHostObject(value any, path string, visited map[uintptr]struct{}) []ResultIssue {
	if value == nil {
		return nil
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Func:
		return []ResultIssue{{
			Code:    CodeUnsupportedType,
			Message: "workflow result cannot include a function value",
			Path:    path,
		}}
	case reflect.Chan, reflect.UnsafePointer:
		return []ResultIssue{{
			Code:    CodeHostHandle,
			Message: "workflow result cannot include host handles",
			Path:    path,
		}}
	case reflect.Ptr, reflect.Map, reflect.Slice:
		ptr := rv.Pointer()
		if ptr != 0 {
			if _, seen := visited[ptr]; seen {
				return []ResultIssue{{
					Code:    CodeCyclicValue,
					Message: "workflow result cannot include cyclic values",
					Path:    path,
				}}
			}
			visited[ptr] = struct{}{}
		}
	}
	switch rv.Kind() {
	case reflect.Ptr:
		if rv.IsNil() {
			return nil
		}
		return walkHostObject(rv.Elem().Interface(), path, visited)
	case reflect.Map:
		var issues []ResultIssue
		for _, key := range rv.MapKeys() {
			issues = append(issues, walkHostObject(rv.MapIndex(key).Interface(), path+"."+fmt.Sprint(key.Interface()), visited)...)
		}
		return issues
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return []ResultIssue{{
				Code:    CodeUnsupportedBinary,
				Message: "workflow result cannot embed raw binary blobs; use workflow.artifact instead",
				Path:    path,
			}}
		}
		var issues []ResultIssue
		for i := 0; i < rv.Len(); i++ {
			issues = append(issues, walkHostObject(rv.Index(i).Interface(), fmt.Sprintf("%s[%d]", path, i), visited)...)
		}
		return issues
	default:
		return nil
	}
}

func walkJSONValue(value any, path string) []ResultIssue {
	switch typed := value.(type) {
	case map[string]any:
		var issues []ResultIssue
		for key, child := range typed {
			childPath := path + "." + key
			if strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
				issues = append(issues, ResultIssue{
					Code:    CodeArtifactURIPathTraversal,
					Message: fmt.Sprintf("workflow result key %q is not allowed", key),
					Path:    childPath,
				})
			}
			issues = append(issues, walkJSONValue(child, childPath)...)
		}
		return issues
	case []any:
		var issues []ResultIssue
		for index, child := range typed {
			issues = append(issues, walkJSONValue(child, fmt.Sprintf("%s[%d]", path, index))...)
		}
		return issues
	case string:
		if looksLikeHostPath(typed) {
			return []ResultIssue{{
				Code:    CodeArtifactURIHostPath,
				Message: fmt.Sprintf("workflow result cannot embed host path %q", typed),
				Path:    path,
			}}
		}
	}
	return nil
}

func looksLikeHostPath(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "\\") {
		return true
	}
	if strings.HasPrefix(trimmed, "file://") {
		return true
	}
	return false
}
