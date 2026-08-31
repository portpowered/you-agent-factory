package parameters_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLIJSONParameterPreservesNestedObjectAndArray proves typed nested JSON
// object and array parameters on you run reach the factory invocation with
// structure and typed values intact, observed through the canonical submission
// edge rather than private decoder internals.
func TestCLIJSONParameterPreservesNestedObjectAndArray(t *testing.T) {
	metadataValue := `{"user":{"name":"alice","roles":["admin","editor"]},"version":2}`
	itemsValue := `[{"id":1,"label":"alpha"},{"id":2,"label":"beta"}]`

	factoryDir := scaffoldJSONInvocationFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	beforeSubmissions := len(parameterProcesses.submissions.snapshot())
	inputs := parameterInputs(t, []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		"invoke marker",
		"--metadata=" + metadataValue,
		"--items=" + itemsValue,
	})

	if err := parameterProcesses.handlerRuntime.execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(JSON parameter invocation) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	records := parameterProcesses.submissions.snapshot()
	if got := len(records) - beforeSubmissions; got != 1 {
		t.Fatalf("canonical submissions = %d, want 1; records=%#v", len(records), records)
	}
	arguments := records[beforeSubmissions].Request.InvocationArguments
	if arguments == nil {
		t.Fatal("submitted invocation arguments = nil")
	}
	assertInvocationArgumentJSON(t, arguments, "metadata", metadataValue)
	assertInvocationArgumentJSON(t, arguments, "items", itemsValue)
}

// TestCLIInvalidJSONParameterNamesTheParameter proves invalid JSON for a named
// factory parameter is rejected with an actionable diagnostic that names the
// parameter before any worker provider dispatch can start.
func TestCLIInvalidJSONParameterNamesTheParameter(t *testing.T) {
	validItemsValue := `[{"id":1,"label":"alpha"}]`
	invalidMetadataValue := `{not-json`

	factoryDir := scaffoldJSONInvocationFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	support.UpdateFactoryConfig(t, factoryDir, func(cfg map[string]any) {
		signature, ok := cfg["invocationSignature"].(map[string]any)
		if !ok {
			t.Fatal("factory config missing invocationSignature")
		}
		parameters, ok := signature["parameters"].([]any)
		if !ok {
			t.Fatal("factory config missing invocationSignature.parameters")
		}
		for _, raw := range parameters {
			parameter, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			switch parameter["name"] {
			case "metadata", "items":
				parameter["typeHint"] = work.InvocationParameterTypeHintJSON
			}
		}
	})

	beforeSubmissions := len(parameterProcesses.submissions.snapshot())
	beforeProviderCalls := parameterProcesses.providerRunner.CallCount()
	inputs := parameterInputs(t, []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		"invoke marker",
		"--metadata=" + invalidMetadataValue,
		"--items=" + validItemsValue,
	})

	executeErr := parameterProcesses.handlerRuntime.execute(inputs.Input)
	if executeErr == nil {
		t.Fatalf(
			"Process.Execute(invalid JSON parameter) succeeded; stdout:\n%s\nstderr:\n%s",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	diagnostic := executeErr.Error() + "\n" + inputs.Stderr()
	for _, want := range []string{
		string(work.ArgumentErrorCodeStringValidationMismatch),
		`parameter "metadata"`,
		"is not valid JSON",
	} {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("invalid JSON diagnostic missing %q:\n%s", want, diagnostic)
		}
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &response); err != nil {
		t.Fatalf("stderr is not one ErrorResponse: %v\nstderr:\n%s", err, inputs.Stderr())
	}
	if response.Code != factoryapi.ErrorResponseCode(work.ArgumentErrorCodeStringValidationMismatch) ||
		response.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("ErrorResponse = %#v, want string-validation code and BAD_REQUEST", response)
	}
	if records := parameterProcesses.submissions.snapshot(); len(records)-beforeSubmissions != 0 {
		t.Fatalf("canonical submission delta = %d, want 0; records=%#v", len(records)-beforeSubmissions, records)
	}
	if got := parameterProcesses.providerRunner.CallCount() - beforeProviderCalls; got != 0 {
		t.Fatalf("provider dispatch call delta = %d, want 0", got)
	}
}

// TestCLIJSONNullAndEmptyValuesRemainDistinct proves JSON null and empty value
// shapes (empty string, empty object, and empty array) remain observably distinct
// through CLI parameter mapping to the factory invocation, without silent
// normalization into one another.
func TestCLIJSONNullAndEmptyValuesRemainDistinct(t *testing.T) {
	nullValue := "null"
	emptyStringValue := `""`
	emptyObjectValue := `{}`
	emptyArrayValue := `[]`

	factoryDir := scaffoldJSONNullAndEmptyInvocationFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	beforeSubmissions := len(parameterProcesses.submissions.snapshot())
	inputs := parameterInputs(t, []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		"invoke marker",
		"--nullable=" + nullValue,
		"--emptyString=" + emptyStringValue,
		"--emptyObject=" + emptyObjectValue,
		"--emptyArray=" + emptyArrayValue,
	})

	if err := parameterProcesses.handlerRuntime.execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(null and empty JSON parameter invocation) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	records := parameterProcesses.submissions.snapshot()
	if got := len(records) - beforeSubmissions; got != 1 {
		t.Fatalf("canonical submissions = %d, want 1; records=%#v", len(records), records)
	}
	arguments := records[beforeSubmissions].Request.InvocationArguments
	if arguments == nil {
		t.Fatal("submitted invocation arguments = nil")
	}

	assertInvocationArgumentJSON(t, arguments, "nullable", nullValue)
	assertInvocationArgumentJSON(t, arguments, "emptyString", emptyStringValue)
	assertInvocationArgumentJSON(t, arguments, "emptyObject", emptyObjectValue)
	assertInvocationArgumentJSON(t, arguments, "emptyArray", emptyArrayValue)

	observed := map[string]string{
		"nullable":    arguments.Arguments["nullable"].Values[0],
		"emptyString": arguments.Arguments["emptyString"].Values[0],
		"emptyObject": arguments.Arguments["emptyObject"].Values[0],
		"emptyArray":  arguments.Arguments["emptyArray"].Values[0],
	}
	for name, value := range observed {
		for otherName, otherValue := range observed {
			if name == otherName {
				continue
			}
			if value == otherValue {
				t.Fatalf(
					"invocation arguments %q and %q normalized to the same value %q",
					name,
					otherName,
					value,
				)
			}
		}
	}
}

func assertInvocationArgumentJSON(
	t *testing.T,
	arguments *work.InvocationArguments,
	name string,
	wantJSON string,
) {
	t.Helper()

	got, ok := arguments.Arguments[name]
	if !ok {
		t.Fatalf("invocation argument %q missing from %#v", name, arguments.Arguments)
	}
	if len(got.Values) != 1 {
		t.Fatalf("invocation argument %q values = %#v, want one value", name, got.Values)
	}
	if got.Values[0] != wantJSON {
		t.Fatalf("invocation argument %q value = %q, want %q", name, got.Values[0], wantJSON)
	}
	if !json.Valid([]byte(got.Values[0])) {
		t.Fatalf("invocation argument %q value is not valid JSON: %q", name, got.Values[0])
	}
	var gotValue any
	if err := json.Unmarshal([]byte(got.Values[0]), &gotValue); err != nil {
		t.Fatalf("unmarshal invocation argument %q: %v", name, err)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(wantJSON), &wantValue); err != nil {
		t.Fatalf("unmarshal want JSON for %q: %v", name, err)
	}
	if !jsonValuesEqual(gotValue, wantValue) {
		t.Fatalf("invocation argument %q decoded = %#v, want %#v", name, gotValue, wantValue)
	}
	if got.Sources[0].Kind != string(work.ArgumentSourceKindNamed) {
		t.Fatalf(
			"invocation argument %q source kind = %q, want %q",
			name,
			got.Sources[0].Kind,
			work.ArgumentSourceKindNamed,
		)
	}
}

func jsonValuesEqual(left, right any) bool {
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}

func scaffoldJSONNullAndEmptyInvocationFactory(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": "json-null-empty-params",
		"invocationSignature": map[string]any{
			"parameters": []any{
				map[string]any{
					"name":     "input",
					"required": true,
					"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
				},
				map[string]any{
					"name":     "nullable",
					"typeHint": work.InvocationParameterTypeHintJSON,
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "emptyString",
					"typeHint": work.InvocationParameterTypeHintJSON,
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "emptyObject",
					"typeHint": work.InvocationParameterTypeHintJSON,
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "emptyArray",
					"typeHint": work.InvocationParameterTypeHintJSON,
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
			},
		},
		"workTypes": []any{map[string]any{
			"name":             "task",
			"handlingBehavior": []any{"DEFAULT"},
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "processor",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
		}},
	})
}

func scaffoldJSONInvocationFactory(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": "json-params",
		"invocationSignature": map[string]any{
			"parameters": []any{
				map[string]any{
					"name":     "input",
					"required": true,
					"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
				},
				map[string]any{
					"name":     "metadata",
					"typeHint": work.InvocationParameterTypeHintJSON,
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "items",
					"typeHint": work.InvocationParameterTypeHintJSON,
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
			},
		},
		"workTypes": []any{map[string]any{
			"name":             "task",
			"handlingBehavior": []any{"DEFAULT"},
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "processor",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
		}},
	})
}
