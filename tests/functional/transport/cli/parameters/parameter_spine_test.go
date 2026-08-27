package parameters_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	spinePositionalValue = "Ship the café résumé plan"
	spinePriorityValue   = "urgent"
	spineCallbackValue   = "https://example.com/callback?token=abc123&scope=read%3Dwrite"
	spineFirstTagValue   = "alpha"
	spineSecondTagValue  = "beta"
	spineMetadataValue   = `{"user":{"name":"alice","roles":["admin","editor"]},"version":2}`
	spineNullableValue   = "null"
	spineEmptyString     = `""`
	spineEmptyObject     = `{}`
	spineEmptyArray      = `[]`
)

// TestCLIParameterReusableProcessSpine establishes the shared process shape
// for the parameter package. The two root-built processes are immutable and
// their customer invocations run in lexical order with fresh inputs.
func TestCLIParameterReusableProcessSpine(t *testing.T) {
	var observations []cliobservation.Result
	observerProcess := support.BuildProcess(t, serviceedges.Edges{
		CLIObserver: cliobservation.CaptureAppend(&observations),
	})
	support.CleanupProcess(t, observerProcess)

	submissions := &invocationSubmissionObservation{}
	providerRunner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")},
	)
	fullHandlerProcess := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: providerRunner,
		SubmissionRecorder:    submissions.observe,
	})
	support.CleanupProcess(t, fullHandlerProcess)

	t.Run("observer root parses generic flags", func(t *testing.T) {
		first := executeSpineObservation(t, observerProcess, &observations, []string{
			"you",
			"--server", "https://factory.example",
			"-v",
			"worker-sessions", "list",
			"--state", "RESERVED",
			"--state", "RUNNING",
			"--json",
		})

		if first.Parse.CommandPath != "you worker-sessions list" {
			t.Fatalf("observed command path = %q, want you worker-sessions list", first.Parse.CommandPath)
		}
		if len(first.Parse.Positionals) != 0 {
			t.Fatalf("observed positional parse = %#v, want none", first.Parse.Positionals)
		}
		assertSpineParsedFlag(t, first, "server", true, "https://factory.example")
		assertSpineParsedFlag(t, first, "verbose", true, "true")
		assertSpineParsedFlag(t, first, "json", true, "true")
		assertSpineParsedFlag(t, first, "state", true, "[RESERVED,RUNNING]")

		second := executeSpineObservation(t, observerProcess, &observations, []string{
			"you",
			"--server", "https://second.example",
			"worker-sessions", "list",
			"--state", "COMPLETED",
		})
		if second.Parse.CommandPath != "you worker-sessions list" {
			t.Fatalf("second observed command path = %q, want you worker-sessions list", second.Parse.CommandPath)
		}
		assertSpineParsedFlag(t, second, "server", true, "https://second.example")
		assertSpineParsedFlag(t, second, "state", true, "[COMPLETED]")
		verbose, found := cliobservation.Flag(second.Parse, "verbose")
		if !found || verbose.Changed {
			t.Fatalf("second observed --verbose parse = %#v found=%v, want unchanged", verbose, found)
		}

		if len(observations) != 2 {
			t.Fatalf("detached CLI observations = %d, want 2", len(observations))
		}
		firstState, found := cliobservation.Flag(observations[0].Parse, "state")
		if !found || firstState.Value != "[RESERVED,RUNNING]" {
			t.Fatalf("first detached state observation = %#v found=%v, want [RESERVED,RUNNING]", firstState, found)
		}
	})

	t.Run("full handler submits combined signature once", func(t *testing.T) {
		factoryDir := scaffoldCombinedInvocationFactory(t)
		support.WriteAgentConfig(
			t,
			factoryDir,
			"processor",
			support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
		)
		factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

		before := len(submissions.snapshot())
		inputs := spineInputs(t, []string{
			"you", "run",
			"--factory", factoryPath,
			"--no-record",
			spinePositionalValue,
			"--priority=" + spinePriorityValue,
			"--callback=" + spineCallbackValue,
			"--tag=" + spineFirstTagValue,
			"--tag=" + spineSecondTagValue,
			"--metadata=" + spineMetadataValue,
			"--nullable=" + spineNullableValue,
			"--emptyString=" + spineEmptyString,
			"--emptyObject=" + spineEmptyObject,
			"--emptyArray=" + spineEmptyArray,
		})

		if err := fullHandlerProcess.Execute(inputs.Input); err != nil {
			t.Fatalf(
				"Process.Execute(combined parameter invocation) error = %v\nstdout:\n%s\nstderr:\n%s",
				err,
				inputs.Stdout(),
				inputs.Stderr(),
			)
		}

		records := submissions.snapshot()
		if got := len(records) - before; got != 1 {
			t.Fatalf("canonical submission delta = %d, want 1; records=%#v", got, records)
		}
		arguments := records[before].Request.InvocationArguments
		if arguments == nil {
			t.Fatal("submitted invocation arguments = nil")
		}

		assertSpineArgument(t, arguments, "input", []string{spinePositionalValue}, work.ArgumentSourceKindPositional)
		assertSpineArgument(t, arguments, "priority", []string{spinePriorityValue}, work.ArgumentSourceKindNamed)
		assertSpineArgument(t, arguments, "callback", []string{spineCallbackValue}, work.ArgumentSourceKindNamed)
		assertSpineArgument(t, arguments, "tag", []string{spineFirstTagValue, spineSecondTagValue}, work.ArgumentSourceKindNamed)
		assertSpineJSONArgument(t, arguments, "metadata", spineMetadataValue)
		assertSpineJSONArgument(t, arguments, "nullable", spineNullableValue)
		assertSpineJSONArgument(t, arguments, "emptyString", spineEmptyString)
		assertSpineJSONArgument(t, arguments, "emptyObject", spineEmptyObject)
		assertSpineJSONArgument(t, arguments, "emptyArray", spineEmptyArray)

		values := []string{
			arguments.Arguments["nullable"].Values[0],
			arguments.Arguments["emptyString"].Values[0],
			arguments.Arguments["emptyObject"].Values[0],
			arguments.Arguments["emptyArray"].Values[0],
		}
		for left, value := range values {
			for right := left + 1; right < len(values); right++ {
				if value == values[right] {
					t.Fatalf("JSON values at indexes %d and %d normalized to %q", left, right, value)
				}
			}
		}
		if got := providerRunner.CallCount(); got != 1 {
			t.Fatalf("controlled provider command calls = %d, want 1", got)
		}
	})
}

func executeSpineObservation(
	t *testing.T,
	process support.Process,
	observations *[]cliobservation.Result,
	args []string,
) cliobservation.Result {
	t.Helper()
	before := len(*observations)
	inputs := spineInputs(t, args)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(parser observation) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if got := len(*observations) - before; got != 1 {
		t.Fatalf("detached CLI observation delta = %d, want 1", got)
	}
	return (*observations)[before]
}

func assertSpineParsedFlag(
	t *testing.T,
	result cliobservation.Result,
	name string,
	wantChanged bool,
	wantValue string,
) {
	t.Helper()
	flag, found := cliobservation.Flag(result.Parse, name)
	if !found || flag.Changed != wantChanged || flag.Value != wantValue {
		t.Fatalf(
			"observed --%s parse = %#v found=%v, want changed=%v value=%q",
			name,
			flag,
			found,
			wantChanged,
			wantValue,
		)
	}
}

func assertSpineArgument(
	t *testing.T,
	arguments *work.InvocationArguments,
	name string,
	wantValues []string,
	wantKind work.ArgumentSourceKind,
) {
	t.Helper()
	argument, found := arguments.Arguments[name]
	if !found {
		t.Fatalf("invocation argument %q missing from %#v", name, arguments.Arguments)
	}
	if len(argument.Values) != len(wantValues) {
		t.Fatalf("invocation argument %q values = %#v, want %#v", name, argument.Values, wantValues)
	}
	for index, want := range wantValues {
		if argument.Values[index] != want {
			t.Fatalf("invocation argument %q value[%d] = %q, want %q", name, index, argument.Values[index], want)
		}
	}
	if len(argument.Sources) == 0 {
		t.Fatalf("invocation argument %q sources = nil, want %q", name, wantKind)
	}
	for index, source := range argument.Sources {
		if source.Kind != string(wantKind) {
			t.Fatalf(
				"invocation argument %q source[%d] kind = %q, want %q",
				name,
				index,
				source.Kind,
				wantKind,
			)
		}
	}
}

func assertSpineJSONArgument(
	t *testing.T,
	arguments *work.InvocationArguments,
	name string,
	want string,
) {
	t.Helper()
	assertSpineArgument(t, arguments, name, []string{want}, work.ArgumentSourceKindNamed)
	argument := arguments.Arguments[name]
	if !json.Valid([]byte(argument.Values[0])) {
		t.Fatalf("invocation argument %q value is not valid JSON: %q", name, argument.Values[0])
	}
}

func spineInputs(t *testing.T, args []string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = t.TempDir()
	inputs.Input.Env = spineEnvironment(inputs.Input.Env, t.TempDir())
	return inputs
}

func spineEnvironment(environment []string, home string) []string {
	filtered := make([]string, 0, len(environment)+2)
	for _, item := range environment {
		if strings.HasPrefix(item, "HOME=") || strings.HasPrefix(item, "USERPROFILE=") {
			continue
		}
		filtered = append(filtered, item)
	}
	return append(filtered, "HOME="+home, "USERPROFILE="+home)
}

func scaffoldCombinedInvocationFactory(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": "combined-parameter-spine",
		"invocationSignature": map[string]any{
			"parameters": []any{
				map[string]any{
					"name":     "input",
					"required": true,
					"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
				},
				map[string]any{
					"name":     "priority",
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "callback",
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":      "tag",
					"valueMode": "REPEATED",
					"required":  true,
					"bindings":  []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "metadata",
					"typeHint": work.InvocationParameterTypeHintJSON,
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
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
