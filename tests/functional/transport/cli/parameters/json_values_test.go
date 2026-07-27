package parameters_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
	mockWorkersPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "processor",
			WorkstationName: "process",
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})

	submissions := &invocationSubmissionObservation{}
	process := support.BuildProcess(t, serviceedges.Edges{
		SubmissionRecorder: submissions.observe,
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		"--with-mock-workers", mockWorkersPath,
		"invoke marker",
		"--metadata=" + metadataValue,
		"--items=" + itemsValue,
	})
	inputs.WorkingDirectory = t.TempDir()

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(JSON parameter invocation) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	records := submissions.snapshot()
	if len(records) != 1 {
		t.Fatalf("canonical submissions = %d, want 1; records=%#v", len(records), records)
	}
	arguments := records[0].Request.InvocationArguments
	if arguments == nil {
		t.Fatal("submitted invocation arguments = nil")
	}
	assertInvocationArgumentJSON(t, arguments, "metadata", metadataValue)
	assertInvocationArgumentJSON(t, arguments, "items", itemsValue)
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
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "items",
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
