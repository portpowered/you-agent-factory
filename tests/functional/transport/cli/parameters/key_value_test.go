package parameters_test

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRunKeyValueParametersReachFactoryInvocation proves repeated key=value
// parameters on you run reach the factory invocation with each customer-supplied
// key and value intact, observed through the canonical submission edge.
func TestRunKeyValueParametersReachFactoryInvocation(t *testing.T) {
	topicValue := "Ship the café résumé plan"
	priorityValue := "urgent"

	factoryDir := scaffoldNamedKeyValueInvocationFactory(t)
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
		"--topic=" + topicValue,
		"--priority=" + priorityValue,
	})
	inputs.WorkingDirectory = t.TempDir()

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(key=value invocation) error = %v\nstdout:\n%s\nstderr:\n%s",
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
	assertInvocationArgumentValues(t, arguments, "topic", []string{topicValue})
	assertInvocationArgumentValues(t, arguments, "priority", []string{priorityValue})
}

// TestRunKeyValuePreservesEqualsInValue proves key=value parameters whose
// values contain embedded '=' characters (URLs, query strings, encoded forms)
// reach the factory invocation with the full value intact, not truncated at the
// first '=' after the key.
func TestRunKeyValuePreservesEqualsInValue(t *testing.T) {
	callbackValue := "https://example.com/callback?token=abc123&scope=read%3Dwrite"

	factoryDir := scaffoldNamedKeyValueInvocationFactory(t)
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
		"--topic=Ship the café résumé plan",
		"--priority=urgent",
		"--callback=" + callbackValue,
	})
	inputs.WorkingDirectory = t.TempDir()

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(embedded-equals key=value invocation) error = %v\nstdout:\n%s\nstderr:\n%s",
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
	assertInvocationArgumentValues(t, arguments, "callback", []string{callbackValue})
}

// TestRunDuplicateKeyUsesDocumentedPrecedence proves duplicate key=value
// parameters on you run follow the documented within-tier precedence: scalar
// parameters reject a second supply, while REPEATED parameters append each
// customer-supplied value in CLI observation order.
func TestRunDuplicateKeyUsesDocumentedPrecedence(t *testing.T) {
	firstTagValue := "alpha"
	secondTagValue := "beta"

	factoryDir := scaffoldNamedKeyValueInvocationFactory(t)
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
		"--topic=Ship the café résumé plan",
		"--priority=urgent",
		"--tag=" + firstTagValue,
		"--tag=" + secondTagValue,
	})
	inputs.WorkingDirectory = t.TempDir()

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(duplicate key=value invocation) error = %v\nstdout:\n%s\nstderr:\n%s",
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
	assertInvocationArgumentValues(t, arguments, "tag", []string{firstTagValue, secondTagValue})
}

// TestRunMalformedKeyValueFailsWithoutDispatch proves malformed key=value shapes
// on you run are rejected with stable diagnostics before any worker provider
// dispatch can start.
func TestRunMalformedKeyValueFailsWithoutDispatch(t *testing.T) {
	factoryDir := scaffoldNamedKeyValueInvocationFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	mockWorkersPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "processor",
			WorkstationName: "process",
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})

	tests := []struct {
		name           string
		invocationArgs []string
		wantFragments  []string
	}{
		{
			name: "missing named value after key",
			invocationArgs: []string{
				"invoke marker",
				"--topic=Ship the café résumé plan",
				"--priority",
			},
			wantFragments: []string{
				"RUN_INVOCATION_FAILED",
				"factory argument --priority requires a value",
			},
		},
		{
			name: "bare key=value without named prefix",
			invocationArgs: []string{
				"invoke marker",
				"topic=Ship the café résumé plan",
				"--priority=urgent",
			},
			wantFragments: []string{
				"INVOCATION_ARGUMENT_POSITIONAL_OVERFLOW",
				"received 2 positional arguments but the active invocationSignature only accepts 1",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			submissions := &invocationSubmissionObservation{}
			providerRunner := testutil.NewProviderCommandRunner()
			process := support.BuildProcess(t, serviceedges.Edges{
				SubmissionRecorder:    submissions.observe,
				ProviderCommandRunner: providerRunner,
			})
			base := []string{
				"you", "run",
				"--factory", factoryPath,
				"--no-record",
				"--with-mock-workers", mockWorkersPath,
			}
			inputs := support.FakeInputs(t.Context(), append(base, test.invocationArgs...))
			inputs.WorkingDirectory = t.TempDir()

			executeErr := process.Execute(inputs.Input)
			if executeErr == nil {
				t.Fatalf(
					"Process.Execute(malformed key=value) succeeded; stdout:\n%s\nstderr:\n%s",
					inputs.Stdout(),
					inputs.Stderr(),
				)
			}

			diagnostic := executeErr.Error() + "\n" + inputs.Stderr()
			for _, want := range test.wantFragments {
				if !strings.Contains(diagnostic, want) {
					t.Fatalf(
						"malformed key=value diagnostic missing %q:\n%s",
						want,
						diagnostic,
					)
				}
			}
			if records := submissions.snapshot(); len(records) != 0 {
				t.Fatalf("canonical submissions = %d, want 0; records=%#v", len(records), records)
			}
			if providerRunner.CallCount() != 0 {
				t.Fatalf("provider dispatch calls = %d, want 0", providerRunner.CallCount())
			}
		})
	}
}

type invocationSubmissionObservation struct {
	mu      sync.Mutex
	records []work.FactorySubmissionRecord
}

func (observation *invocationSubmissionObservation) observe(record work.FactorySubmissionRecord) {
	observation.mu.Lock()
	defer observation.mu.Unlock()
	record.Request.InvocationArguments = work.CloneInvocationArguments(record.Request.InvocationArguments)
	observation.records = append(observation.records, record)
}

func (observation *invocationSubmissionObservation) snapshot() []work.FactorySubmissionRecord {
	observation.mu.Lock()
	defer observation.mu.Unlock()
	return append([]work.FactorySubmissionRecord(nil), observation.records...)
}

func assertInvocationArgumentValues(
	t *testing.T,
	arguments *work.InvocationArguments,
	name string,
	want []string,
) {
	t.Helper()

	got, ok := arguments.Arguments[name]
	if !ok {
		t.Fatalf("invocation argument %q missing from %#v", name, arguments.Arguments)
	}
	if len(got.Values) != len(want) {
		t.Fatalf("invocation argument %q values = %#v, want %#v", name, got.Values, want)
	}
	for index := range want {
		if got.Values[index] != want[index] {
			t.Fatalf(
				"invocation argument %q value[%d] = %q, want %q",
				name,
				index,
				got.Values[index],
				want[index],
			)
		}
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

func scaffoldNamedKeyValueInvocationFactory(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": "key-value-params",
		"invocationSignature": map[string]any{
			"parameters": []any{
				map[string]any{
					"name":     "input",
					"required": true,
					"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
				},
				map[string]any{
					"name":     "topic",
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "priority",
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "callback",
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":         "tag",
					"externalName": "tag",
					"valueMode":    "REPEATED",
					"bindings":     []any{map[string]any{"kind": "NAMED"}},
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
