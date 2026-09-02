package parameters_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const parameterProcessCloseTimeout = 5 * time.Second

type parameterProcessFixture struct {
	process             support.ApplicationProcess
	missingAssetProcess support.ApplicationProcess
	providerRunner      *support.ShapedProviderCommandRunner
	missingProvider     *testutil.ProviderCommandRunner
	lifecycleEffects    *atomic.Int32
}

var parameterProcesses *parameterProcessFixture

// TestMain constructs the two immutable process variants once for the package.
// The ordinary public command process is shared; only the missing-asset witness
// receives lifecycle-observation edges and a provider that must remain unused.
func TestMain(m *testing.M) {
	fixture, err := buildParameterProcessFixture()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build parameter functional process fixture: %v\n", err)
		os.Exit(1)
	}
	parameterProcesses = fixture

	exitCode := m.Run()
	if closeErr := fixture.close(); closeErr != nil {
		fmt.Fprintf(os.Stderr, "close parameter functional process fixture: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func buildParameterProcessFixture() (*parameterProcessFixture, error) {
	providerRunner := support.NewShapedProviderCommandRunner(
		successfulProviderResults(64)...,
	)
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: providerRunner,
	})
	if err != nil {
		return nil, fmt.Errorf("build parameter process: %w", err)
	}

	lifecycleEffects := &atomic.Int32{}
	missingProvider := testutil.NewProviderCommandRunner()
	missingAssetProcess, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: missingProvider,
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			lifecycleEffects.Add(1)
			return nil
		},
		BrowserOpener: func(context.Context, string) error {
			lifecycleEffects.Add(1)
			return nil
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			lifecycleEffects.Add(1)
		},
		FactorySessionIDGenerator: func() string {
			lifecycleEffects.Add(1)
			return "unexpected-session"
		},
	})
	if err != nil {
		_ = process.Close(context.Background())
		return nil, fmt.Errorf("build missing-asset process: %w", err)
	}

	return &parameterProcessFixture{
		process:             process,
		missingAssetProcess: missingAssetProcess,
		providerRunner:      providerRunner,
		missingProvider:     missingProvider,
		lifecycleEffects:    lifecycleEffects,
	}, nil
}

func successfulProviderResults(count int) []platformprocess.CommandResult {
	results := make([]platformprocess.CommandResult, count)
	for index := range results {
		results[index] = platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")}
	}
	return results
}

func (fixture *parameterProcessFixture) close() error {
	ctx, cancel := context.WithTimeout(context.Background(), parameterProcessCloseTimeout)
	defer cancel()

	var closeErr error
	processes := []struct {
		name    string
		process support.ApplicationProcess
	}{
		{name: "parameters", process: fixture.process},
		{name: "missing-asset", process: fixture.missingAssetProcess},
	}
	for _, entry := range processes {
		name, process := entry.name, entry.process
		if err := process.Close(ctx); err != nil {
			closeErr = fmt.Errorf("%s process: %w", name, err)
		}
	}
	return closeErr
}

func parameterInputs(t *testing.T, args []string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = t.TempDir()
	inputs.Input.Env = spineEnvironment(inputs.Input.Env, t.TempDir())
	return inputs
}

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
// for the parameter package. The reusable root-built process is immutable
// and their customer invocations run in lexical order with fresh inputs.
func TestCLIParameterReusableProcessSpine(t *testing.T) {
	if parameterProcesses == nil {
		t.Fatal("parameter process fixture is not initialized")
	}

	t.Run("full handler submits combined signature once", func(t *testing.T) {
		testFullHandlerSubmitsCombinedSignature(t)
	})

	t.Run("malformed parameters fail without dispatch", func(t *testing.T) {
		t.Parallel()
		testMalformedCombinedSignature(t)
	})

	t.Run("invalid JSON names the parameter", func(t *testing.T) {
		t.Parallel()
		testInvalidJSONCombinedSignature(t)
	})
}

func testFullHandlerSubmitsCombinedSignature(t *testing.T) {
	t.Helper()
	beforeProviderCalls := parameterProcesses.providerRunner.CallCount()
	factoryDir := scaffoldCombinedInvocationFactory(t)
	support.WriteAgentConfig(t, factoryDir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	inputs := spineInputs(t, combinedSignatureArgs(factoryPath))
	if err := parameterProcesses.process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(combined parameter invocation) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}

	if got := parameterProcesses.providerRunner.CallCount() - beforeProviderCalls; got != 1 {
		t.Fatalf("controlled provider command call delta = %d, want 1", got)
	}
	prompt := string(parameterProcesses.providerRunner.LastRequest().Stdin)
	for _, want := range []string{
		spinePositionalValue, spinePriorityValue, spineCallbackValue,
		spineMetadataValue,
		spineNullableValue, spineEmptyString, spineEmptyObject, spineEmptyArray,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expanded provider prompt omitted customer parameter value %q:\n%s", want, prompt)
		}
	}
}

func testMalformedCombinedSignature(t *testing.T) {
	t.Helper()
	factoryDir := scaffoldCombinedInvocationFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	tests := []struct {
		name          string
		args          []string
		wantFragments []string
	}{
		{
			name: "missing named value",
			args: []string{"invoke marker", "--priority"},
			wantFragments: []string{
				"INVOCATION_ARGUMENT_MISSING_VALUE",
				"factory argument --priority requires a value",
			},
		},
		{
			name: "bare key=value is positional overflow",
			args: []string{"invoke marker", "priority=urgent"},
			wantFragments: []string{
				"INVOCATION_ARGUMENT_POSITIONAL_OVERFLOW",
				"received 2 positional arguments but the active invocationSignature only accepts 1",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			beforeProviderCalls := parameterProcesses.providerRunner.CallCount()
			args := append([]string{"you", "run", "--factory", factoryPath, "--no-record"}, test.args...)
			inputs := parameterInputs(t, args)
			executeErr := parameterProcesses.process.Execute(inputs.Input)
			if executeErr == nil {
				t.Fatalf("Process.Execute(malformed parameter) succeeded; stdout:\n%s\nstderr:\n%s", inputs.Stdout(), inputs.Stderr())
			}
			diagnostic := executeErr.Error() + "\n" + inputs.Stderr()
			for _, want := range test.wantFragments {
				if !strings.Contains(diagnostic, want) {
					t.Fatalf("malformed parameter diagnostic missing %q:\n%s", want, diagnostic)
				}
			}
			if got := parameterProcesses.providerRunner.CallCount() - beforeProviderCalls; got != 0 {
				t.Fatalf("provider dispatch call delta = %d, want 0", got)
			}
		})
	}
}

func testInvalidJSONCombinedSignature(t *testing.T) {
	t.Helper()
	factoryDir := scaffoldCombinedInvocationFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	args := combinedSignatureArgs(factoryPath)
	for index, arg := range args {
		if strings.HasPrefix(arg, "--metadata=") {
			args[index] = "--metadata={not-json"
		}
	}
	beforeProviderCalls := parameterProcesses.providerRunner.CallCount()
	inputs := parameterInputs(t, args)
	executeErr := parameterProcesses.process.Execute(inputs.Input)
	if executeErr == nil {
		t.Fatalf("Process.Execute(invalid JSON parameter) succeeded; stdout:\n%s\nstderr:\n%s", inputs.Stdout(), inputs.Stderr())
	}
	diagnostic := executeErr.Error() + "\n" + inputs.Stderr()
	for _, want := range []string{
		string(work.ArgumentErrorCodeStringValidationMismatch), `parameter "metadata"`, "is not valid JSON",
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
	if got := parameterProcesses.providerRunner.CallCount() - beforeProviderCalls; got != 0 {
		t.Fatalf("provider dispatch call delta = %d, want 0", got)
	}
}

func combinedSignatureArgs(factoryPath string) []string {
	return []string{
		"you", "run", "--factory", factoryPath, "--no-record", spinePositionalValue,
		"--priority=" + spinePriorityValue, "--callback=" + spineCallbackValue,
		"--tag=" + spineFirstTagValue, "--tag=" + spineSecondTagValue,
		"--metadata=" + spineMetadataValue, "--nullable=" + spineNullableValue,
		"--emptyString=" + spineEmptyString, "--emptyObject=" + spineEmptyObject,
		"--emptyArray=" + spineEmptyArray,
	}
}

func spineInputs(t *testing.T, args []string) *support.CapturedInputs {
	t.Helper()
	return parameterInputs(t, args)
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

	dir := support.ScaffoldFactory(t, map[string]any{
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
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
---
input=${input}
priority=${priority}
callback=${callback}
metadata=${metadata}
nullable=${nullable}
emptyString=${emptyString}
emptyObject=${emptyObject}
emptyArray=${emptyArray}
`)
	return dir
}
