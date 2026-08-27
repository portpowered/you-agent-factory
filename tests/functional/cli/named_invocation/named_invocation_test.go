package named_invocation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedGoalFactoryName             = "@you/goal"
	packagedGoalExecuteWorkstationName  = "execute-goal"
	packagedSubagentFactoryName         = "@you/subagent"
	packagedSubagentWorkerName          = "subagent-worker"
	packagedSubagentRunWorkstationName  = "run-subagent"
	packagedFactoryBuilderName          = "@you/factory-builder"
	packagedTournamentFactoryName       = "@you/tournament"
	customizedNamedGoalFactoryName      = "@test/goal"
	wantHermeticInvocationPrimaryResult = "mock worker accepted"
)

type listenerStartObservation struct {
	calls atomic.Int32
}

func (observation *listenerStartObservation) Start(
	context.Context,
	platformhttpserver.StartRequest,
) error {
	observation.calls.Add(1)
	return errors.New("one-shot named invocation attempted to start an HTTP listener")
}

type canonicalSubmissionObservation struct {
	mu      sync.Mutex
	records []work.FactorySubmissionRecord
}

func (observation *canonicalSubmissionObservation) observe(record work.FactorySubmissionRecord) {
	observation.mu.Lock()
	defer observation.mu.Unlock()
	record.Request.InvocationArguments = work.CloneInvocationArguments(record.Request.InvocationArguments)
	observation.records = append(observation.records, record)
}

func (observation *canonicalSubmissionObservation) snapshot() []work.FactorySubmissionRecord {
	observation.mu.Lock()
	defer observation.mu.Unlock()
	return append([]work.FactorySubmissionRecord(nil), observation.records...)
}

func assertEffectiveSignatureSubmission(t *testing.T, arguments *work.InvocationArguments, documentPath string) {
	t.Helper()
	if arguments == nil {
		t.Fatal("submitted invocation arguments = nil")
	}
	want := map[string]work.InvocationArgument{
		"input": {
			Values:    []string{"equivalent canonical prompt"},
			ValueMode: work.InvocationParameterValueModeExact,
			Sources:   []work.InvocationArgumentSource{{Kind: string(work.ArgumentSourceKindPositional), Name: "1"}},
		},
		"files": {
			Values:    []string{"one.md", "two.md"},
			ValueMode: work.InvocationParameterValueModeVariadic,
			Sources:   []work.InvocationArgumentSource{{Kind: string(work.ArgumentSourceKindPositional), Name: "2+"}},
		},
		"tags": {
			Values:    []string{"alpha", "beta"},
			ValueMode: work.InvocationParameterValueModeRepeated,
			Sensitive: true,
			Sources: []work.InvocationArgumentSource{
				{Kind: string(work.ArgumentSourceKindNamed), Name: "t", Redact: true},
				{Kind: string(work.ArgumentSourceKindNamed), Name: "tag", Redact: true},
			},
		},
		"format": {
			Values:    []string{"json"},
			ValueMode: work.InvocationParameterValueModeExact,
			Sources:   []work.InvocationArgumentSource{{Kind: string(work.ArgumentSourceKindDefault), Name: "default"}},
		},
		"count": {
			Values:    []string{"2"},
			ValueMode: work.InvocationParameterValueModeExact,
			Sources:   []work.InvocationArgumentSource{{Kind: string(work.ArgumentSourceKindNamed), Name: "count"}},
		},
		"document": {
			Values:    []string{documentPath},
			ValueMode: work.InvocationParameterValueModeFileContents,
			Sources:   []work.InvocationArgumentSource{{Kind: string(work.ArgumentSourceKindNamed), Name: "file"}},
		},
		"body": {
			Values:    []string{"canonical stdin body"},
			ValueMode: work.InvocationParameterValueModeExact,
			Sources:   []work.InvocationArgumentSource{{Kind: string(work.ArgumentSourceKindStdin), Name: "stdin"}},
		},
	}
	if !reflect.DeepEqual(arguments.Arguments, want) {
		t.Fatalf("submitted canonical arguments = %#v, want %#v", arguments.Arguments, want)
	}
}

// TestRun_EmptyEffectiveSignatureInputUsesSchemaBeforeExecution retains the
// preparation-failure anchors until the shared failure fixture story migrates
// them.
func TestRun_EmptyEffectiveSignatureInputUsesSchemaBeforeExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for empty selected-Factory invocation preparation")
	}

	for _, selection := range []string{"named", "file"} {
		selection := selection
		for _, failure := range []struct {
			name      string
			signature map[string]any
			wantCode  string
		}{
			{
				name: "missing required",
				signature: map[string]any{"parameters": []any{map[string]any{
					"name": "input", "required": true,
					"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
				}}},
				wantCode: string(work.ArgumentErrorCodeMissingRequiredInput),
			},
			{
				name: "reserved collision",
				signature: map[string]any{"parameters": []any{map[string]any{
					"name": "reserved", "externalName": "quiet",
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				}}},
				wantCode: climanifest.CompositionCollisionLongName,
			},
		} {
			failure := failure
			t.Run(selection+" "+failure.name, func(t *testing.T) {
				runEmptyPreparationFailureCase(t, selection, failure.signature, failure.wantCode)
			})
		}
	}
}

func runEmptyPreparationFailureCase(
	t *testing.T,
	selection string,
	signature map[string]any,
	wantCode string,
) {
	t.Helper()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	observation := &preparationSideEffectObservation{}
	provider := testutil.NewProviderCommandRunner()
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		FactorySessionIDGenerator: observation.nextSessionID,
		RuntimeHostObserver:       observation.observeRuntimeHost,
		WorkRequestIDGenerator:    observation.nextWorkRequestID,
		ProviderCommandRunner:     provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	factoryDir := initializePackagedFactory(
		t, process, environment, workingDirectory, homeDir, packagedGoalFactoryName,
	)
	factoryPath := filepath.Join(factoryDir, "factory.json")
	replaceInvocationSignatureFixture(t, factoryPath, signature)
	factoryDir = support.CopyFactoryAsNamed(t, factoryDir, homeDir, customizedNamedGoalFactoryName)
	factoryPath = filepath.Join(factoryDir, "factory.json")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdinIsTTY := true
	stdoutIsTTY := false
	err = process.Execute(root.Input{
		Args:             emptyInvocationArguments(selection, factoryPath, customizedNamedGoalFactoryName),
		Env:              environment,
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	if err == nil || !strings.Contains(err.Error(), wantCode) {
		t.Fatalf("empty-input error = %v, want stable code %s", err, wantCode)
	}
	observation.assertNoExecution(t, provider.CallCount())
}

func emptyInvocationArguments(selection, factoryPath, factoryName string) []string {
	if selection == "named" {
		return []string{"you", "run", "--named", factoryName, "--no-record", "--quiet"}
	}
	return []string{"you", "run", "--factory", factoryPath, "--no-record", "--quiet"}
}

const preparationFailureSensitiveValue = "credential-that-must-not-leak"

// TestRun_EffectiveSchemaPreparationFailuresStopBeforeExecutionSideEffects proves preparation failure is side-effect free.
func TestRun_EffectiveSchemaPreparationFailuresStopBeforeExecutionSideEffects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		factory           string
		arguments         []string
		wantCode          string
		cancelDuringRoot  bool
		wantContextCancel bool
	}{
		{
			name: "reserved static collision",
			factory: `{
  "name": "collision",
  "invocationSignature": {
    "parameters": [
      {
        "name": "credential",
        "sensitive": true,
        "bindings": [{"kind": "POSITIONAL", "position": 1}]
      },
      {
        "name": "reserved",
        "externalName": "quiet",
        "bindings": [{"kind": "NAMED"}]
      }
    ]
  }
}`,
			arguments: []string{preparationFailureSensitiveValue},
			wantCode:  climanifest.CompositionCollisionLongName,
		},
		{
			name: "sensitive normalization failure",
			factory: `{
  "name": "sensitive",
  "invocationSignature": {
    "parameters": [{
      "name": "token",
      "sensitive": true,
      "required": true,
      "choices": ["allowed"],
      "bindings": [{"kind": "POSITIONAL", "position": 1}]
    }]
  }
}`,
			arguments: []string{preparationFailureSensitiveValue},
			wantCode:  string(work.ArgumentErrorCodeStringValidationMismatch),
		},
		{
			name: "cancellation during explicit file root lookup",
			factory: `{
  "name": "canceled",
  "invocationSignature": {
    "parameters": [{
      "name": "input",
      "bindings": [{"kind": "POSITIONAL", "position": 1}]
    }]
  }
}`,
			arguments:         []string{"draft"},
			cancelDuringRoot:  true,
			wantContextCancel: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runPreparationFailureCase(t, test.factory, test.arguments, test.wantCode, test.cancelDuringRoot, test.wantContextCancel)
		})
	}
}

func runPreparationFailureCase(
	t *testing.T,
	factory string,
	arguments []string,
	wantCode string,
	cancelDuringRoot bool,
	wantContextCancel bool,
) {
	t.Helper()

	workingDirectory := t.TempDir()
	factoryPath := filepath.Join(workingDirectory, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(factory), 0o600); err != nil {
		t.Fatalf("write Factory fixture: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	observation := &preparationSideEffectObservation{}
	provider := testutil.NewProviderCommandRunner()
	edges := serviceedges.Edges{
		FactorySessionIDGenerator: observation.nextSessionID,
		RuntimeHostObserver:       observation.observeRuntimeHost,
		WorkRequestIDGenerator:    observation.nextWorkRequestID,
		ProviderCommandRunner:     provider,
	}
	if cancelDuringRoot {
		edges.FactoryDefinitionAuthoredReaderFileSystem = cancelingRootLookupFileSystem{
			target: factoryPath,
			cancel: cancel,
		}
	}
	process, err := support.BuildProcessWithContext(t.Context(), edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdinIsTTY := true
	stdoutIsTTY := false
	homeDir := t.TempDir()
	err = process.Execute(root.Input{
		Args: append(
			[]string{"you", "run", "--factory", factoryPath, "--no-record"},
			arguments...,
		),
		Env: append(
			os.Environ(),
			"HOME="+homeDir,
			"USERPROFILE="+homeDir,
		),
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          ctx,
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	if wantContextCancel {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Process.Execute() error = %v, want context cancellation", err)
		}
		if errors.Is(err, errCanceledFactoryRootLookup) {
			t.Fatalf("Process.Execute() returned lookup failure instead of context cancellation: %v", err)
		}
	} else if err == nil || !strings.Contains(err.Error(), wantCode) {
		t.Fatalf("Process.Execute() error = %v, want stable code %s", err, wantCode)
	}
	observable := errText(err) + stdout.String() + stderr.String()
	if strings.Contains(observable, preparationFailureSensitiveValue) {
		t.Fatalf("preparation failure leaked sensitive input: %s", observable)
	}
	observation.assertNoExecution(t, provider.CallCount())
}

type preparationSideEffectObservation struct {
	sessionIDs   atomic.Int32
	runtimeHosts atomic.Int32
	workIDs      atomic.Int32
}

func (observation *preparationSideEffectObservation) nextSessionID() string {
	observation.sessionIDs.Add(1)
	return "unexpected-session"
}

func (observation *preparationSideEffectObservation) observeRuntimeHost(factorysessions.RuntimeHostBinding) {
	observation.runtimeHosts.Add(1)
}

func (observation *preparationSideEffectObservation) nextWorkRequestID() string {
	observation.workIDs.Add(1)
	return "unexpected-work"
}

func (observation *preparationSideEffectObservation) assertNoExecution(t *testing.T, providerCalls int) {
	t.Helper()
	if sessionIDs := observation.sessionIDs.Load(); sessionIDs != 0 {
		t.Fatalf("Factory Session ID calls = %d, want 0", sessionIDs)
	}
	if runtimeHosts := observation.runtimeHosts.Load(); runtimeHosts != 0 {
		t.Fatalf("runtime host observations = %d, want 0", runtimeHosts)
	}
	if workIDs := observation.workIDs.Load(); workIDs != 0 {
		t.Fatalf("Work request ID calls = %d, want 0", workIDs)
	}
	if providerCalls != 0 {
		t.Fatalf("provider command calls = %d, want 0", providerCalls)
	}
}

var errCanceledFactoryRootLookup = errors.New("explicit Factory root lookup failed after cancellation")

type cancelingRootLookupFileSystem struct {
	target string
	cancel context.CancelFunc
}

func (filesystem cancelingRootLookupFileSystem) Stat(path string) (fs.FileInfo, error) {
	if filepath.Clean(path) == filepath.Clean(filesystem.target) {
		filesystem.cancel()
		return nil, errCanceledFactoryRootLookup
	}
	return os.Stat(path)
}

func (filesystem cancelingRootLookupFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func addEffectiveSignatureFixture(t *testing.T, factoryPath string) {
	t.Helper()
	payload, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read installed Factory: %v", err)
	}
	var factory map[string]any
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode installed Factory: %v", err)
	}
	factory["invocationSignature"] = map[string]any{
		"unknownNamedArgumentPolicy": "REJECT",
		"parameters": []any{
			map[string]any{
				"name":     "input",
				"required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			},
			map[string]any{
				"name":      "files",
				"valueMode": "VARIADIC",
				"bindings":  []any{map[string]any{"kind": "POSITIONAL", "position": 2}},
			},
			map[string]any{
				"name":         "tags",
				"externalName": "tag",
				"aliases":      []any{"t"},
				"valueMode":    "REPEATED",
				"sensitive":    true,
				"bindings":     []any{map[string]any{"kind": "NAMED"}},
			},
			map[string]any{
				"name":         "format",
				"choices":      []any{"json", "text"},
				"defaultValue": "json",
				"bindings":     []any{map[string]any{"kind": "NAMED"}},
			},
			map[string]any{
				"name":     "count",
				"typeHint": "NUMBER_STRING",
				"bindings": []any{map[string]any{"kind": "NAMED"}},
			},
			map[string]any{
				"name":         "document",
				"externalName": "document",
				"aliases":      []any{"file"},
				"typeHint":     "FILE_PATH",
				"valueMode":    "FILE_CONTENTS",
				"bindings":     []any{map[string]any{"kind": "NAMED"}},
			},
			map[string]any{
				"name":     "body",
				"bindings": []any{map[string]any{"kind": "STDIN"}},
			},
		},
	}
	workersConfig, ok := factory["workers"].([]any)
	if !ok || len(workersConfig) == 0 {
		t.Fatalf("installed Factory workers = %#v", factory["workers"])
	}
	workerConfig, ok := workersConfig[0].(map[string]any)
	if !ok {
		t.Fatalf("installed Factory worker = %#v", workersConfig[0])
	}
	delete(workerConfig, "promptFile")
	workerConfig["body"] = "input=${input}|format=${format}|count=${count}|document=${document}|stdin=${body}"
	updated, err := json.MarshalIndent(factory, "", "  ")
	if err != nil {
		t.Fatalf("encode signature Factory fixture: %v", err)
	}
	if err := os.WriteFile(factoryPath, updated, 0o600); err != nil {
		t.Fatalf("write signature Factory fixture: %v", err)
	}
	support.ReplaceGoalWorkerInstructions(
		t,
		factoryPath,
		"input=${input}|format=${format}|count=${count}|document=${document}|stdin=${body}",
	)
}

func replaceInvocationSignatureFixture(t *testing.T, factoryPath string, signature map[string]any) {
	t.Helper()
	payload, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read installed Factory: %v", err)
	}
	var factory map[string]any
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode installed Factory: %v", err)
	}
	factory["invocationSignature"] = signature
	updated, err := json.MarshalIndent(factory, "", "  ")
	if err != nil {
		t.Fatalf("encode signature Factory fixture: %v", err)
	}
	if err := os.WriteFile(factoryPath, updated, 0o600); err != nil {
		t.Fatalf("write signature Factory fixture: %v", err)
	}
}

type customerProcess interface {
	Execute(root.Input) error
}

func executeCustomerCommand(
	t *testing.T,
	process customerProcess,
	environment []string,
	workingDirectory string,
	args []string,
) (string, string) {
	t.Helper()
	return executeCustomerCommandWithStdin(t, process, environment, workingDirectory, args, "")
}

func executeCustomerCommandWithStdin(
	t *testing.T,
	process customerProcess,
	environment []string,
	workingDirectory string,
	args []string,
	stdin string,
) (string, string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	stdinIsTTY := true
	stdoutIsTTY := false
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := process.Execute(root.Input{
		Args:             args,
		Env:              environment,
		Stdin:            strings.NewReader(stdin),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          ctx,
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	if err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v; stdout=%q stderr=%q",
			args,
			err,
			stdout.String(),
			stderr.String(),
		)
	}
	return stdout.String(), stderr.String()
}

func initializePackagedFactory(
	t *testing.T,
	process customerProcess,
	environment []string,
	workingDirectory string,
	homeDir string,
	name string,
) string {
	t.Helper()
	factoriesRoot := filepath.Join(homeDir, ".you-agent-factory", "factories")
	missingFactory := filepath.Join(workingDirectory, "missing-initialization-factory.json")
	err := process.Execute(root.Input{
		Args:             []string{"you", "run", "--factory", missingFactory},
		Env:              environment,
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
	})
	if err == nil || !strings.Contains(err.Error(), filepath.Base(missingFactory)) {
		t.Fatalf("Process.Execute(run missing Factory) error = %v", err)
	}
	factoryDir := filepath.Join(
		append([]string{factoriesRoot}, strings.Split(name, "/")...)...,
	)
	if _, err := os.Stat(filepath.Join(factoryDir, "factory.json")); err != nil {
		t.Fatalf("installed packaged Factory %q: %v", name, err)
	}
	return factoryDir
}
