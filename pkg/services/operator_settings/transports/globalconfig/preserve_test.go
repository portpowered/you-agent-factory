package globalconfig

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestPreserveUnknownFields_MergesKnownUpdatesAndNestedFutureValues(t *testing.T) {
	original := []byte(`{
		"DEFAULTS": {
			"workerModel": "before",
			"futureDefault": {"enabled": true}
		},
		"models": {
			"llm": {
				"source": "before-source",
				"futureModel": {"owner": "newer-binary"}
			}
		},
		"workers": {
			"acp": {
				"integrations": [{
					"id": "one",
					"name": "provider",
					"transport": "stdio",
					"command": "before-command",
					"futureIntegration": ["kept"]
				}],
				"futureACP": true
			},
			"futureWorkers": {"version": 2}
		},
		"workerPresets": [{
			"id": "one",
			"modelProvider": "codex",
			"futurePreset": "kept"
		}],
		"futureRoot": {"secret": "must-not-be-dropped"}
	}`)
	canonical := []byte(`{
		"defaults": {"workerModel": "after"},
		"models": {"llm": {"source": "after-source"}},
		"workers": {"acp": {"integrations": [{
			"id": "one",
			"name": "provider",
			"transport": "stdio",
			"command": "after-command"
		}]}},
		"workerPresets": [{"id": "one", "modelProvider": "codex"}]
	}`)

	merged, err := PreserveUnknownFields(original, canonical)
	if err != nil {
		t.Fatalf("PreserveUnknownFields() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("decode merged document: %v", err)
	}

	if got["futureRoot"] == nil {
		t.Fatalf("futureRoot = %#v, want preserved", got["futureRoot"])
	}
	defaults := got["defaults"].(map[string]any)
	if defaults["workerModel"] != "after" || !reflect.DeepEqual(defaults["futureDefault"], map[string]any{"enabled": true}) {
		t.Fatalf("defaults = %#v, want updated known value and preserved future value", defaults)
	}
	model := got["models"].(map[string]any)["llm"].(map[string]any)
	if model["source"] != "after-source" || !reflect.DeepEqual(model["futureModel"], map[string]any{"owner": "newer-binary"}) {
		t.Fatalf("model = %#v, want updated source and preserved future value", model)
	}
	workers := got["workers"].(map[string]any)
	if !reflect.DeepEqual(workers["futureWorkers"], map[string]any{"version": float64(2)}) {
		t.Fatalf("futureWorkers = %#v, want preserved value", workers["futureWorkers"])
	}
	acp := workers["acp"].(map[string]any)
	if acp["futureACP"] != true {
		t.Fatalf("futureACP = %#v, want preserved value", acp["futureACP"])
	}
	integration := acp["integrations"].([]any)[0].(map[string]any)
	if integration["command"] != "after-command" || !reflect.DeepEqual(integration["futureIntegration"], []any{"kept"}) {
		t.Fatalf("integration = %#v, want updated command and preserved future value", integration)
	}
	preset := got["workerPresets"].([]any)[0].(map[string]any)
	if preset["futurePreset"] != "kept" {
		t.Fatalf("preset = %#v, want preserved future value", preset)
	}
}

func TestPreserveUnknownFields_RecreatesOmittedKnownContainersWithFutureValues(t *testing.T) {
	original := []byte(`{
		"runtime": {
			"logging": {"futureLogging": {"enabled": true}},
			"futureRuntime": "kept"
		},
		"models": {
			"future-model": {"futureModel": {"enabled": true}}
		},
		"priceTable": {
			"futurePrices": ["kept"]
		}
	}`)

	merged, err := PreserveUnknownFields(original, []byte(`{"defaults":{"workerModel":"new"}}`))
	if err != nil {
		t.Fatalf("PreserveUnknownFields() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("decode merged document: %v", err)
	}

	runtime := got["runtime"].(map[string]any)
	if runtime["futureRuntime"] != "kept" || !reflect.DeepEqual(runtime["logging"], map[string]any{"futureLogging": map[string]any{"enabled": true}}) {
		t.Fatalf("runtime = %#v, want projected future values", runtime)
	}
	models := got["models"].(map[string]any)
	if !reflect.DeepEqual(models["future-model"], map[string]any{"futureModel": map[string]any{"enabled": true}}) {
		t.Fatalf("models = %#v, want projected future model value", models)
	}
	priceTable := got["priceTable"].(map[string]any)
	if !reflect.DeepEqual(priceTable["futurePrices"], []any{"kept"}) {
		t.Fatalf("priceTable = %#v, want projected future values", priceTable)
	}
}

func TestPreserveUnknownFields_RejectsMalformedTrailingAndNonObjectDocuments(t *testing.T) {
	tests := []struct {
		name      string
		original  string
		canonical string
		want      string
	}{
		{name: "malformed original", original: `{"defaults":`, canonical: `{}`, want: "decode existing global config"},
		{name: "trailing original", original: `{} {}`, canonical: `{}`, want: "decode existing global config"},
		{name: "non-object original", original: `[]`, canonical: `{}`, want: "expected a JSON object"},
		{name: "malformed canonical", original: `{}`, canonical: `{"defaults":`, want: "decode canonical global config"},
		{name: "trailing canonical", original: `{}`, canonical: `{} {}`, want: "decode canonical global config"},
		{name: "non-object canonical", original: `{}`, canonical: `[]`, want: "expected a JSON object"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := PreserveUnknownFields([]byte(testCase.original), []byte(testCase.canonical))
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("PreserveUnknownFields() error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestCompatibilityHelpersLeaveIncompatibleShapesUntouched(t *testing.T) {
	canonical := map[string]any{"known": true}
	stringType := reflect.TypeOf("")
	mapType := reflect.TypeOf(map[string]string{})
	nonStringMapType := reflect.TypeOf(map[int]string{})
	sequenceType := reflect.TypeOf([]string{})
	structType := reflect.TypeOf(struct{ Value string }{})
	interfaceType := reflect.TypeOf((*any)(nil)).Elem()
	rawMessageType := reflect.TypeOf(json.RawMessage{})

	mergeCases := []struct {
		name      string
		original  any
		valueType reflect.Type
	}{
		{name: "nil original", original: nil, valueType: structType},
		{name: "nil type", original: canonical, valueType: nil},
		{name: "interface", original: canonical, valueType: interfaceType},
		{name: "raw message", original: canonical, valueType: rawMessageType},
		{name: "scalar", original: canonical, valueType: stringType},
		{name: "map shape", original: "wrong", valueType: mapType},
		{name: "map key shape", original: canonical, valueType: nonStringMapType},
		{name: "sequence shape", original: "wrong", valueType: sequenceType},
		{name: "struct shape", original: "wrong", valueType: structType},
	}
	for _, testCase := range mergeCases {
		t.Run("merge/"+testCase.name, func(t *testing.T) {
			if got := mergePreservedJSONValue(testCase.original, canonical, testCase.valueType); !reflect.DeepEqual(got, canonical) {
				t.Fatalf("mergePreservedJSONValue() = %#v, want %#v", got, canonical)
			}
		})
	}

	projectCases := []struct {
		name      string
		value     any
		valueType reflect.Type
	}{
		{name: "nil value", value: nil, valueType: structType},
		{name: "nil type", value: canonical, valueType: nil},
		{name: "raw message", value: canonical, valueType: rawMessageType},
		{name: "interface", value: canonical, valueType: interfaceType},
		{name: "scalar", value: canonical, valueType: stringType},
		{name: "struct shape", value: "wrong", valueType: structType},
		{name: "map shape", value: "wrong", valueType: mapType},
		{name: "map key shape", value: canonical, valueType: nonStringMapType},
	}
	for _, testCase := range projectCases {
		t.Run("project/"+testCase.name, func(t *testing.T) {
			if got, keep := projectUnknownJSONValue(testCase.value, testCase.valueType); got != nil || keep {
				t.Fatalf("projectUnknownJSONValue() = %#v, %t, want nil, false", got, keep)
			}
		})
	}

	if got := mergePreservedJSONMap(
		map[string]any{},
		map[string]any{"new": "canonical"},
		reflect.TypeOf(map[string]string{}),
	); !reflect.DeepEqual(got, map[string]any{"new": "canonical"}) {
		t.Fatalf("mergePreservedJSONMap() = %#v, want canonical child", got)
	}
	if got := mergePreservedJSONSequence(
		[]any{"old"},
		[]any{"updated", "new"},
		reflect.TypeOf([]string{}),
	); !reflect.DeepEqual(got, []any{"updated", "new"}) {
		t.Fatalf("mergePreservedJSONSequence() = %#v, want updated and appended children", got)
	}
	if got := mergePreservedJSONStruct(
		map[string]any{},
		map[string]any{"futureCanonical": true},
		structType,
	); !reflect.DeepEqual(got, map[string]any{"futureCanonical": true}) {
		t.Fatalf("mergePreservedJSONStruct() = %#v, want canonical unknown child", got)
	}
}

func TestCompatibilityDecoderHelpersHandleOpaqueAndWrongShapes(t *testing.T) {
	if _, err := canonicalizeKnownJSONFieldNames([]byte(`{"`)); err == nil {
		t.Fatal("canonicalizeKnownJSONFieldNames() error = nil, want malformed JSON error")
	}
	if got := canonicalizeKnownJSONFieldNamesForType(
		map[string]any{"opaque": true},
		reflect.TypeOf(json.RawMessage{}),
	); !reflect.DeepEqual(got, map[string]any{"opaque": true}) {
		t.Fatalf("canonicalizeKnownJSONFieldNamesForType(raw) = %#v, want unchanged value", got)
	}
	if got := canonicalizeKnownJSONFieldNamesForMap("wrong", reflect.TypeOf(map[string]string{})); got != "wrong" {
		t.Fatalf("canonicalizeKnownJSONFieldNamesForMap() = %#v, want unchanged value", got)
	}
	if got := canonicalizeKnownJSONFieldNamesForMap(map[string]any{}, reflect.TypeOf(map[int]string{})); got == nil {
		t.Fatal("canonicalizeKnownJSONFieldNamesForMap(non-string key) = nil, want unchanged value")
	}
	if got := canonicalizeKnownJSONFieldNamesForSequence("wrong", reflect.TypeOf([]string{})); got != "wrong" {
		t.Fatalf("canonicalizeKnownJSONFieldNamesForSequence() = %#v, want unchanged value", got)
	}
	if got := canonicalizeKnownJSONFieldNamesForStruct("wrong", reflect.TypeOf(struct{}{})); got != "wrong" {
		t.Fatalf("canonicalizeKnownJSONFieldNamesForStruct() = %#v, want unchanged value", got)
	}

	if _, err := decodeOneJSONValue([]byte(`{} x`)); err == nil {
		t.Fatal("decodeOneJSONValue() error = nil, want trailing token error")
	}
	if _, err := collectUnknownJSONPaths([]byte(`{"`)); err == nil {
		t.Fatal("collectUnknownJSONPaths() error = nil, want malformed JSON error")
	}
	paths := []string{}
	collectUnknownJSONPathsForType(nil, reflect.TypeOf(factoryapi.GlobalConfig{}), "$", &paths)
	collectUnknownJSONPathsForType(map[string]any{}, reflect.TypeOf(json.RawMessage{}), "$", &paths)
	collectUnknownJSONPathsForMap("wrong", reflect.TypeOf(map[string]string{}), "$", &paths)
	collectUnknownJSONPathsForMap(map[string]any{}, reflect.TypeOf(map[int]string{}), "$", &paths)
	collectUnknownJSONPathsForSequence("wrong", reflect.TypeOf([]string{}), "$", &paths)
	collectUnknownJSONPathsForStruct("wrong", reflect.TypeOf(struct{}{}), "$", &paths)
	if len(paths) != 0 {
		t.Fatalf("wrong-shaped compatibility values produced paths = %#v, want none", paths)
	}

	type fieldShape struct {
		hidden string
		Value  string `json:",omitempty"`
	}
	fields := jsonFields(reflect.TypeOf(fieldShape{}))
	if _, ok := fields["value"]; !ok {
		t.Fatalf("jsonFields() = %#v, want fallback field name Value", fields)
	}
	if got := appendJSONPath("$", ""); got != `$[""]` {
		t.Fatalf("appendJSONPath(empty key) = %q, want bracket path", got)
	}
}
