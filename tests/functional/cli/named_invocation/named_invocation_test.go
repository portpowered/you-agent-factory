package named_invocation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

const (
	packagedGoalFactoryName             = "@you/goal"
	packagedGoalExecuteWorkstationName  = "execute-goal"
	packagedSubagentFactoryName         = "@you/subagent"
	packagedSubagentWorkerName          = "subagent-worker"
	packagedSubagentRunWorkstationName  = "run-subagent"
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

func TestRun_NamedGoalHermeticInvocationSucceedsWithoutListeningServer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for hermetic named goal no-server invocation")
	}

	goalText := "hermetic no-server named goal prompt"
	stdout, listenerStarts := runHermeticNamedInvocation(
		t,
		packagedGoalFactoryName,
		goalText,
		workers.MockWorkersConfig{
			UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
			MockWorkers: []workers.MockWorkerConfig{{
				WorkerName:      "goal-executor",
				WorkstationName: packagedGoalExecuteWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			}},
		},
	)

	if stdout != wantHermeticInvocationPrimaryResult {
		t.Fatalf("stdout = %q, want primary result %s", stdout, wantHermeticInvocationPrimaryResult)
	}
	if listenerStarts != 0 {
		t.Fatalf("HTTP listener start calls = %d, want 0", listenerStarts)
	}
}

func TestRun_NamedSubagentHermeticInvocationSucceedsWithoutListeningServer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for hermetic named subagent no-server invocation")
	}

	requestText := "hermetic no-server named subagent prompt"
	stdout, listenerStarts := runHermeticNamedInvocation(
		t,
		packagedSubagentFactoryName,
		requestText,
		workers.MockWorkersConfig{
			UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
			MockWorkers: []workers.MockWorkerConfig{{
				WorkerName:      packagedSubagentWorkerName,
				WorkstationName: packagedSubagentRunWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			}},
		},
	)

	if stdout != wantHermeticInvocationPrimaryResult {
		t.Fatalf("stdout = %q, want agent response %s", stdout, wantHermeticInvocationPrimaryResult)
	}
	if stdout == requestText {
		t.Fatalf("stdout echoed submitted request text instead of agent response")
	}
	if listenerStarts != 0 {
		t.Fatalf("HTTP listener start calls = %d, want 0", listenerStarts)
	}
}

func TestRun_NamedAndExplicitFactorySelectionsExecuteEquivalentEffectiveSignatureInput(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for named and explicit Factory invocation parity")
	}

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	initStdout, initStderr := executeCustomerCommand(
		t, process, environment, workingDirectory,
		[]string{"you", "--json", "config", "init"},
	)
	if initStderr != "" {
		t.Fatalf("config init stderr = %q, want empty; stdout=%s", initStderr, initStdout)
	}
	factoryDir := packagedFactoryDir(t, initStdout, packagedGoalFactoryName)
	factoryPath := filepath.Join(factoryDir, "factory.json")
	addEffectiveSignatureFixture(t, factoryPath)
	mockWorkersPath := writeMockWorkersConfig(t, workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: packagedGoalExecuteWorkstationName,
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})
	documentPath := filepath.Join(workingDirectory, "story.md")
	if err := os.WriteFile(documentPath, []byte("factory invocation document"), 0o600); err != nil {
		t.Fatalf("write FILE_CONTENTS fixture: %v", err)
	}
	common := []string{
		"--with-mock-workers", "--no-record", "--quiet",
		mockWorkersPath,
		"equivalent canonical prompt", "one.md", "two.md",
		"--t", "alpha", "--tag", "beta",
		"--count", "2",
		"--file", documentPath,
		"-",
	}
	namedStdout, namedStderr := executeCustomerCommandWithStdin(
		t, process, environment, workingDirectory,
		append([]string{"you", "run", "--named", packagedGoalFactoryName}, common...),
		"canonical stdin body",
	)
	fileStdout, fileStderr := executeCustomerCommandWithStdin(
		t, process, environment, workingDirectory,
		append([]string{"you", "run", "--factory", factoryPath}, common...),
		"canonical stdin body",
	)
	if namedStderr != "" || fileStderr != "" {
		t.Fatalf("invocation stderr: named=%q file=%q", namedStderr, fileStderr)
	}
	if namedStdout != wantHermeticInvocationPrimaryResult || fileStdout != namedStdout {
		t.Fatalf("selection outputs differ: named=%q file=%q", namedStdout, fileStdout)
	}
}

const preparationFailureSensitiveValue = "credential-that-must-not-leak"

func TestRun_EffectiveSchemaPreparationFailuresStopBeforeExecutionSideEffects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		factory           string
		arguments         []string
		wantCode          string
		cancelDuringLoad  bool
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
			name: "cancellation during explicit file lookup",
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
			cancelDuringLoad:  true,
			wantContextCancel: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runPreparationFailureCase(t, test.factory, test.arguments, test.wantCode, test.cancelDuringLoad, test.wantContextCancel)
		})
	}
}

func runPreparationFailureCase(
	t *testing.T,
	factory string,
	arguments []string,
	wantCode string,
	cancelDuringLoad bool,
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
	if cancelDuringLoad {
		edges.FactoryDefinitionAuthoredReaderFileSystem = cancelingLoadingFileSystem{
			target: factoryPath,
			cancel: cancel,
		}
	}
	process, err := root.BuildProcess(t.Context(), edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdinIsTTY := true
	stdoutIsTTY := false
	err = process.Execute(root.Input{
		Args: append(
			[]string{"you", "run", "--factory", factoryPath, "--no-record"},
			arguments...,
		),
		Env:              os.Environ(),
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

type cancelingLoadingFileSystem struct {
	target string
	cancel context.CancelFunc
}

func (filesystem cancelingLoadingFileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (filesystem cancelingLoadingFileSystem) ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil && filepath.Clean(path) == filepath.Clean(filesystem.target) {
		filesystem.cancel()
	}
	return data, err
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
	updated, err := json.MarshalIndent(factory, "", "  ")
	if err != nil {
		t.Fatalf("encode signature Factory fixture: %v", err)
	}
	if err := os.WriteFile(factoryPath, updated, 0o600); err != nil {
		t.Fatalf("write signature Factory fixture: %v", err)
	}
}

func runHermeticNamedInvocation(
	t *testing.T,
	factoryName string,
	requestText string,
	mockWorkers workers.MockWorkersConfig,
) (string, int) {
	t.Helper()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	listenerStarts := &listenerStartObservation{}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		APIServerStarter: listenerStarts.Start,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	initStdout, initStderr := executeCustomerCommand(
		t,
		process,
		environment,
		workingDirectory,
		[]string{"you", "--json", "config", "init"},
	)
	if initStderr != "" {
		t.Fatalf("config init stderr = %q, want empty; stdout=%s", initStderr, initStdout)
	}
	assertPackagedFactoryInstalled(t, initStdout, factoryName)

	mockWorkersPath := writeMockWorkersConfig(t, mockWorkers)
	args := []string{
		"you", "run", "--named", factoryName,
		"--with-mock-workers", "--no-record", "--quiet",
		mockWorkersPath, requestText,
	}
	stdout, stderr := executeCustomerCommand(
		t,
		process,
		environment,
		workingDirectory,
		args,
	)
	if stderr != "" {
		t.Fatalf("named invocation stderr = %q, want empty; stdout=%s", stderr, stdout)
	}
	return stdout, int(listenerStarts.calls.Load())
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

func assertPackagedFactoryInstalled(t *testing.T, payload, name string) {
	t.Helper()
	_ = packagedFactoryDir(t, payload, name)
}

func packagedFactoryDir(t *testing.T, payload, name string) string {
	t.Helper()
	var result struct {
		PackagedFactories []struct {
			Name       string `json:"name"`
			FactoryDir string `json:"factoryDirectory"`
		} `json:"packagedFactories"`
	}
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		t.Fatalf("decode config init result: %v\nstdout:\n%s", err, payload)
	}
	for _, factory := range result.PackagedFactories {
		if factory.Name != name {
			continue
		}
		if _, err := os.Stat(filepath.Join(factory.FactoryDir, "factory.json")); err != nil {
			t.Fatalf("installed packaged Factory %q: %v", name, err)
		}
		return factory.FactoryDir
	}
	t.Fatalf("config init result omitted packaged Factory %q: %#v", name, result.PackagedFactories)
	return ""
}

func writeMockWorkersConfig(t *testing.T, config workers.MockWorkersConfig) string {
	t.Helper()
	payload, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("marshal mock workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers.json")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write mock workers config: %v", err)
	}
	return path
}
