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

func TestRun_NamedAndExplicitNoSignatureFactoriesPreserveCompatibilityInputs(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-signature Factory invocation compatibility")
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
	mockWorkersPath := writeMockWorkersConfig(t, workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: packagedGoalExecuteWorkstationName,
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})
	base := []string{
		"--with-mock-workers", "--no-record", "--quiet", mockWorkersPath,
	}
	tests := []struct {
		name  string
		input []string
		stdin string
	}{
		{name: "positional compatibility", input: []string{"legacy positional input"}},
		{name: "stdin compatibility", input: []string{"-"}, stdin: "legacy stdin input\n"},
		{name: "signature-only syntax remains literal text", input: []string{"--mode", "fast"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			namedStdout, namedStderr := executeCustomerCommandWithStdin(
				t, process, environment, workingDirectory,
				append(append([]string{"you", "run", "--named", packagedGoalFactoryName}, base...), test.input...),
				test.stdin,
			)
			fileStdout, fileStderr := executeCustomerCommandWithStdin(
				t, process, environment, workingDirectory,
				append(append([]string{"you", "run", "--factory", factoryPath}, base...), test.input...),
				test.stdin,
			)
			if namedStderr != "" || fileStderr != "" {
				t.Fatalf("invocation stderr: named=%q file=%q", namedStderr, fileStderr)
			}
			if namedStdout != wantHermeticInvocationPrimaryResult || fileStdout != namedStdout {
				t.Fatalf("selection outputs differ: named=%q file=%q", namedStdout, fileStdout)
			}
		})
	}
}

func TestRun_NamedAndExplicitFactorySelectionsExecuteEquivalentEffectiveSignatureInput(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for named and explicit Factory invocation parity")
	}

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	provider := testutil.NewMockProvider(
		workers.InferenceResponse{Content: "canonical provider result\n<COMPLETE>"},
		workers.InferenceResponse{Content: "canonical provider result\n<COMPLETE>"},
	)
	submissions := &canonicalSubmissionObservation{}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		ProviderOverride:   provider,
		SubmissionRecorder: submissions.observe,
	})
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
	documentPath := filepath.Join(workingDirectory, "story.md")
	if err := os.WriteFile(documentPath, []byte("factory invocation document"), 0o600); err != nil {
		t.Fatalf("write FILE_CONTENTS fixture: %v", err)
	}
	common := []string{
		"--no-record", "--quiet",
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
	if namedStdout == "" || fileStdout != namedStdout {
		t.Fatalf("selection outputs differ: named=%q file=%q", namedStdout, fileStdout)
	}
	records := submissions.snapshot()
	if len(records) != 2 {
		t.Fatalf("canonical submissions = %d, want named and explicit-file records", len(records))
	}
	namedArguments := records[0].Request.InvocationArguments
	fileArguments := records[1].Request.InvocationArguments
	if !reflect.DeepEqual(namedArguments, fileArguments) {
		t.Fatalf("selection canonical arguments differ: named=%#v file=%#v", namedArguments, fileArguments)
	}
	assertEffectiveSignatureSubmission(t, namedArguments, documentPath)

	calls := provider.Calls()
	if len(calls) != 2 {
		t.Fatalf("provider calls = %d, want named and explicit-file calls", len(calls))
	}
	if calls[0].SystemPrompt != calls[1].SystemPrompt {
		t.Fatalf("selection provider prompts differ: named=%q file=%q", calls[0].SystemPrompt, calls[1].SystemPrompt)
	}
	wantPrompt := "input=equivalent canonical prompt|format=json|count=2|document=factory invocation document|stdin=canonical stdin body"
	if calls[0].SystemPrompt != wantPrompt {
		t.Fatalf("provider system prompt = %q, want resolved effective input %q", calls[0].SystemPrompt, wantPrompt)
	}
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

func TestRun_EmptyEffectiveSignatureInputUsesSchemaBeforeExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for empty selected-Factory invocation preparation")
	}

	for _, selection := range []string{"named", "file"} {
		selection := selection
		t.Run(selection+" default-only", func(t *testing.T) {
			runEmptyDefaultInvocationCase(t, selection)
		})
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

func runEmptyDefaultInvocationCase(t *testing.T, selection string) {
	t.Helper()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	submissions := &canonicalSubmissionObservation{}
	provider := testutil.NewMockProvider(workers.InferenceResponse{Content: "default applied\n<COMPLETE>"})
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		ProviderOverride:   provider,
		SubmissionRecorder: submissions.observe,
	})
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
	factoryPath := filepath.Join(packagedFactoryDir(t, initStdout, packagedGoalFactoryName), "factory.json")
	replaceInvocationSignatureFixture(t, factoryPath, map[string]any{
		"parameters": []any{map[string]any{
			"name": "mode", "defaultValue": "safe",
			"bindings": []any{map[string]any{"kind": "NAMED"}},
		}},
	})
	replaceGoalWorkerInstructions(t, factoryPath, "mode=${mode}")

	stdout, stderr := executeCustomerCommand(
		t,
		process,
		environment,
		workingDirectory,
		emptyInvocationArguments(selection, factoryPath),
	)
	if stderr != "" || stdout == "" {
		t.Fatalf("default-only invocation output: stdout=%q stderr=%q", stdout, stderr)
	}
	records := submissions.snapshot()
	if len(records) != 1 || records[0].Request.InvocationArguments == nil {
		t.Fatalf("default-only submissions = %#v, want one canonical request", records)
	}
	want := work.InvocationArgument{
		Values:    []string{"safe"},
		ValueMode: work.InvocationParameterValueModeExact,
		Sources:   []work.InvocationArgumentSource{{Kind: string(work.ArgumentSourceKindDefault), Name: "default"}},
	}
	if got := records[0].Request.InvocationArguments.Arguments["mode"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("default-only argument = %#v, want %#v", got, want)
	}
	if calls := provider.Calls(); len(calls) != 1 || calls[0].SystemPrompt != "mode=safe" {
		t.Fatalf("default-only provider calls = %#v, want interpolated default", calls)
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
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		FactorySessionIDGenerator: observation.nextSessionID,
		RuntimeHostObserver:       observation.observeRuntimeHost,
		WorkRequestIDGenerator:    observation.nextWorkRequestID,
		ProviderCommandRunner:     provider,
	})
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
	factoryPath := filepath.Join(packagedFactoryDir(t, initStdout, packagedGoalFactoryName), "factory.json")
	replaceInvocationSignatureFixture(t, factoryPath, signature)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdinIsTTY := true
	stdoutIsTTY := false
	err = process.Execute(root.Input{
		Args:             emptyInvocationArguments(selection, factoryPath),
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

func emptyInvocationArguments(selection, factoryPath string) []string {
	if selection == "named" {
		return []string{"you", "run", "--named", packagedGoalFactoryName, "--no-record"}
	}
	return []string{"you", "run", "--factory", factoryPath, "--no-record"}
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
	replaceGoalWorkerInstructions(
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

func replaceGoalWorkerInstructions(t *testing.T, factoryPath, instructions string) {
	t.Helper()
	workerInstructions := filepath.Join(
		filepath.Dir(factoryPath),
		"workers",
		"goal-executor",
		"AGENTS.md",
	)
	if err := os.WriteFile(
		workerInstructions,
		[]byte(instructions),
		0o600,
	); err != nil {
		t.Fatalf("write interpolated worker instructions: %v", err)
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
