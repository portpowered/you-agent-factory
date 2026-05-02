package workers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type capturingCommandRunner struct {
	mu      sync.Mutex
	request CommandRequest
}

func (r *capturingCommandRunner) Run(_ context.Context, req CommandRequest) (CommandResult, error) {
	r.mu.Lock()
	r.request = req
	r.mu.Unlock()
	return CommandResult{Stdout: []byte("ok")}, nil
}

type fixedCommandRunner struct {
	stdout   []byte
	stderr   []byte
	exitCode int
	err      error
}

func (r fixedCommandRunner) Run(_ context.Context, _ CommandRequest) (CommandResult, error) {
	return CommandResult{Stdout: r.stdout, Stderr: r.stderr, ExitCode: r.exitCode}, r.err
}

func testScriptRequest(dispatch interfaces.WorkDispatch, opts ...func(*interfaces.WorkstationExecutionRequest)) interfaces.WorkstationExecutionRequest {
	req := interfaces.WorkstationExecutionRequest{
		Dispatch:    interfaces.CloneWorkDispatch(dispatch),
		WorkerType:  dispatch.WorkerType,
		ProjectID:   dispatch.ProjectID,
		InputTokens: append([]any(nil), dispatch.InputTokens...),
	}
	for _, opt := range opts {
		opt(&req)
	}
	return req
}

func withScriptEnvVars(envVars map[string]string) func(*interfaces.WorkstationExecutionRequest) {
	return func(req *interfaces.WorkstationExecutionRequest) {
		req.EnvVars = envVars
	}
}

func withScriptWorktree(worktree string) func(*interfaces.WorkstationExecutionRequest) {
	return func(req *interfaces.WorkstationExecutionRequest) {
		req.Worktree = worktree
	}
}

func withScriptWorkingDirectory(workingDirectory string) func(*interfaces.WorkstationExecutionRequest) {
	return func(req *interfaces.WorkstationExecutionRequest) {
		req.WorkingDirectory = workingDirectory
	}
}

type envPrintingCommandRunner struct{}

func (envPrintingCommandRunner) Run(_ context.Context, req CommandRequest) (CommandResult, error) {
	return CommandResult{Stdout: []byte(strings.Join(req.Env, "\n"))}, nil
}

type commandRunnerFunc func(context.Context, CommandRequest) (CommandResult, error)

func (fn commandRunnerFunc) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	return fn(ctx, req)
}

func echoCommand(msg string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", "echo " + msg}
	}
	return "echo", []string{msg}
}

func failCommand(msg string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", "echo " + msg + " 1>&2 && exit 1"}
	}
	return "sh", []string{"-c", "echo '" + msg + "' >&2; exit 1"}
}

func TestScriptExecutor_SuccessfulEcho_PopulatesOutput(t *testing.T) {
	cmd, args := echoCommand("hello world")
	executor := &ScriptExecutor{Command: cmd, Args: args}

	result, err := executor.Execute(context.Background(), testScriptRequest(interfaces.WorkDispatch{DispatchID: "d-1", TransitionID: "t-1"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if !strings.Contains(result.Output, "hello world") {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestScriptExecutor_FailingCommand_ReturnsFailedResult(t *testing.T) {
	cmd, args := failCommand("something went wrong")
	executor := &ScriptExecutor{Command: cmd, Args: args}

	result, err := executor.Execute(context.Background(), testScriptRequest(interfaces.WorkDispatch{DispatchID: "d-1", TransitionID: "t-2"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if !strings.Contains(result.Error, "something went wrong") {
		t.Fatalf("Error = %q", result.Error)
	}
}

func TestScriptExecutor_CancellationReturnsFailedResult(t *testing.T) {
	executor := &ScriptExecutor{}
	if runtime.GOOS == "windows" {
		executor.Command = "powershell"
		executor.Args = []string{"-Command", "Start-Sleep -Seconds 30"}
	} else {
		executor.Command = "sleep"
		executor.Args = []string{"30"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result, err := executor.Execute(ctx, testScriptRequest(interfaces.WorkDispatch{DispatchID: "d-1", TransitionID: "t-3"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Error != "execution timeout" {
		t.Fatalf("Error = %q, want %q", result.Error, "execution timeout")
	}
	if result.ProviderFailure == nil || result.ProviderFailure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("ProviderFailure = %#v, want timeout metadata", result.ProviderFailure)
	}
}

func TestScriptExecutor_TemplateSubstitutionAndEnvMerging(t *testing.T) {
	cmd, args := echoCommand("{{ (index .Inputs 0).WorkID }}")
	executor := &ScriptExecutor{Command: cmd, Args: args}

	result, err := executor.Execute(context.Background(), testScriptRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-4",
			InputTokens: InputTokens(interfaces.Token{
				ID:    "token-script-template",
				Color: interfaces.TokenColor{WorkID: "work-script-template"},
			}),
		},
		withScriptEnvVars(map[string]string{
			"TEST_FACTORY_VAR": "injected-value",
		}),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if !strings.Contains(result.Output, "work-script-template") {
		t.Fatalf("Output = %q, want rendered work ID", result.Output)
	}
}

func TestScriptExecutor_ExecutionWorkDirPrefersWorkingDirectory(t *testing.T) {
	runner := &capturingCommandRunner{}
	executor := &ScriptExecutor{
		Command:       "echo",
		Args:          []string{"ok"},
		CommandRunner: runner,
	}

	_, err := executor.Execute(context.Background(), testScriptRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-5",
		},
		withScriptWorktree("/tmp/worktree"),
		withScriptWorkingDirectory("/tmp/working-dir"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.request.WorkDir != "/tmp/working-dir" {
		t.Fatalf("workDir = %q, want %q", runner.request.WorkDir, "/tmp/working-dir")
	}
}

func TestScriptExecutor_PropagatesExecutionMetadataToCommandRunner(t *testing.T) {
	runner := &capturingCommandRunner{}
	executor := &ScriptExecutor{
		Command:       "echo",
		Args:          []string{"ok"},
		CommandRunner: runner,
	}

	want := interfaces.ExecutionMetadata{
		DispatchCreatedTick: 4,
		CurrentTick:         5,
		RequestID:           "request-1",
		TraceID:             "trace-1",
		WorkIDs:             []string{"work-1", "work-2"},
		ReplayKey:           "transition-1/trace-1/work-1/work-2",
	}
	_, err := executor.Execute(context.Background(), testScriptRequest(interfaces.WorkDispatch{
		DispatchID:   "d-1",
		TransitionID: "transition-1",
		Execution:    want,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.mu.Lock()
	got := runner.request.Execution
	runner.mu.Unlock()
	if got.DispatchCreatedTick != want.DispatchCreatedTick ||
		got.CurrentTick != want.CurrentTick ||
		got.RequestID != want.RequestID ||
		got.TraceID != want.TraceID ||
		got.ReplayKey != want.ReplayKey ||
		strings.Join(got.WorkIDs, ",") != strings.Join(want.WorkIDs, ",") {
		t.Fatalf("Execution = %#v, want %#v", got, want)
	}
}

func TestScriptExecutor_SharedCommandRunnerReceivesResolvedDispatchRequest(t *testing.T) {
	runner := &capturingCommandRunner{}
	executor := &ScriptExecutor{
		Command:       "script-tool",
		Args:          []string{"--work", "{{ (index .Inputs 0).WorkID }}", "--tag", `{{ index (index .Inputs 0).Tags "priority" }}`, "--project", "{{ .Context.Project }}"},
		CommandRunner: runner,
	}

	wantExecution := interfaces.ExecutionMetadata{
		DispatchCreatedTick: 7,
		CurrentTick:         9,
		RequestID:           "request-script",
		TraceID:             "trace-script",
		WorkIDs:             []string{"work-script"},
		ReplayKey:           "transition-script/trace-script/work-script",
	}

	result, err := executor.Execute(context.Background(), sharedRunnerDispatch(wantExecution))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}

	assertSharedRunnerRequest(t, runner, wantExecution)
}

func TestScriptExecutor_EmitsScriptRequestEventBeforeCommandRunner(t *testing.T) {
	order := make([]string, 0, 3)
	recorded := make([]factoryapi.FactoryEvent, 0, 2)
	result := executeRecordedScript(t, newRecordedScriptExecutor(
		commandRunnerFunc(func(_ context.Context, req CommandRequest) (CommandResult, error) {
			order = append(order, "run")
			if strings.Join(req.Args, " ") != "--work work-script --priority high" {
				t.Fatalf("command runner args = %#v, want resolved args", req.Args)
			}
			return CommandResult{Stdout: []byte("ok")}, nil
		}),
		func(event factoryapi.FactoryEvent) {
			order = append(order, "record")
			recorded = append(recorded, event)
		},
	))
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if strings.Join(order, ",") != "record,run,record" {
		t.Fatalf("event order = %v, want request before command runner and response after", order)
	}
	if len(recorded) != 2 {
		t.Fatalf("recorded events = %d, want request and response", len(recorded))
	}
	assertScriptRequestEvent(t, recorded[0])
	if recorded[1].Type != factoryapi.FactoryEventTypeScriptResponse {
		t.Fatalf("second event type = %s, want %s", recorded[1].Type, factoryapi.FactoryEventTypeScriptResponse)
	}
}

func TestScriptExecutor_EmitsScriptResponseEventForCommandOutcomes(t *testing.T) {
	for _, tc := range scriptResponseOutcomeCases() {
		t.Run(tc.name, func(t *testing.T) {
			recorded := make([]factoryapi.FactoryEvent, 0, 2)
			result := executeRecordedScript(t, newRecordedScriptExecutor(
				tc.runner,
				func(event factoryapi.FactoryEvent) {
					recorded = append(recorded, event)
				},
			))
			if result.Outcome != tc.wantResult {
				t.Fatalf("result outcome = %s, want %s", result.Outcome, tc.wantResult)
			}
			if tc.wantErrorText != "" && result.Error != tc.wantErrorText {
				t.Fatalf("result error = %q, want %q", result.Error, tc.wantErrorText)
			}
			if len(recorded) != 2 {
				t.Fatalf("recorded events = %d, want request and response", len(recorded))
			}

			response := recorded[1]
			if response.Type != factoryapi.FactoryEventTypeScriptResponse {
				t.Fatalf("response event type = %s, want %s", response.Type, factoryapi.FactoryEventTypeScriptResponse)
			}
			if response.Id != "factory-event/script-response/dispatch-script/1" {
				t.Fatalf("response event id = %q, want stable response id", response.Id)
			}
			assertScriptResponsePayload(t, response, tc.wantOutcome, tc.wantFailure, tc.wantExitCode, tc.wantStdout, tc.wantStderr)
		})
	}
}

type scriptResponseOutcomeCase struct {
	name          string
	runner        CommandRunner
	wantOutcome   factoryapi.ScriptExecutionOutcome
	wantFailure   *factoryapi.ScriptFailureType
	wantExitCode  *int
	wantStdout    string
	wantStderr    string
	wantResult    interfaces.WorkOutcome
	wantErrorText string
}

func scriptResponseOutcomeCases() []scriptResponseOutcomeCase {
	timedOut := factoryapi.ScriptFailureTypeTimeout
	processError := factoryapi.ScriptFailureTypeProcessError

	return []scriptResponseOutcomeCase{
		{
			name: "success",
			runner: fixedCommandRunner{
				stdout: []byte("script ok\n"),
			},
			wantOutcome:  factoryapi.ScriptExecutionOutcomeSucceeded,
			wantExitCode: intPtr(0),
			wantStdout:   "script ok\n",
			wantResult:   interfaces.OutcomeAccepted,
		},
		{
			name: "failed exit code",
			runner: fixedCommandRunner{
				stdout:   []byte("before failure\n"),
				stderr:   []byte("boom\n"),
				exitCode: 17,
			},
			wantOutcome:   factoryapi.ScriptExecutionOutcomeFailedExitCode,
			wantExitCode:  intPtr(17),
			wantStdout:    "before failure\n",
			wantStderr:    "boom\n",
			wantResult:    interfaces.OutcomeFailed,
			wantErrorText: "boom",
		},
		{
			name: "timeout",
			runner: commandRunnerFunc(func(_ context.Context, _ CommandRequest) (CommandResult, error) {
				return CommandResult{Stdout: []byte("partial stdout"), Stderr: []byte("partial stderr")}, context.DeadlineExceeded
			}),
			wantOutcome:   factoryapi.ScriptExecutionOutcomeTimedOut,
			wantFailure:   &timedOut,
			wantStdout:    "partial stdout",
			wantStderr:    "partial stderr",
			wantResult:    interfaces.OutcomeFailed,
			wantErrorText: "execution timeout",
		},
		{
			name: "process error",
			runner: commandRunnerFunc(func(_ context.Context, _ CommandRequest) (CommandResult, error) {
				return CommandResult{Stderr: []byte("exec failed")}, errors.New("exec: file not found")
			}),
			wantOutcome:   factoryapi.ScriptExecutionOutcomeProcessError,
			wantFailure:   &processError,
			wantStderr:    "exec failed",
			wantResult:    interfaces.OutcomeFailed,
			wantErrorText: "execution cancelled: exec: file not found",
		},
		{
			name: "process error omits zero exit code diagnostics",
			runner: commandRunnerFunc(func(_ context.Context, _ CommandRequest) (CommandResult, error) {
				return CommandResult{Stderr: []byte("exec failed"), ExitCode: 0}, errors.New("exec: file not found")
			}),
			wantOutcome:   factoryapi.ScriptExecutionOutcomeProcessError,
			wantFailure:   &processError,
			wantStderr:    "exec failed",
			wantResult:    interfaces.OutcomeFailed,
			wantErrorText: "execution cancelled: exec: file not found",
		},
	}
}

func sharedRunnerDispatch(execution interfaces.ExecutionMetadata) interfaces.WorkstationExecutionRequest {
	dispatch := interfaces.WorkDispatch{
		DispatchID:   "dispatch-script",
		TransitionID: "transition-script",
		ProjectID:    "analytics-platform",
		Execution:    execution,
		InputTokens: InputTokens(interfaces.Token{
			ID: "token-script",
			Color: interfaces.TokenColor{
				WorkID:     "work-script",
				WorkTypeID: "task",
				DataType:   interfaces.DataTypeWork,
				Tags:       map[string]string{"priority": "high"},
			},
		}),
	}
	return testScriptRequest(
		dispatch,
		withScriptWorkingDirectory("/tmp/script-workdir"),
		withScriptEnvVars(map[string]string{
			"SCRIPT_SHARED_RUNNER_VAR": "visible",
		}),
	)
}

func newRecordedScriptExecutor(runner CommandRunner, recorder func(factoryapi.FactoryEvent)) *ScriptExecutor {
	return &ScriptExecutor{
		Command: "script-tool",
		Args: []string{
			"--work",
			"{{ (index .Inputs 0).WorkID }}",
			"--priority",
			`{{ index (index .Inputs 0).Tags "priority" }}`,
		},
		CommandRunner: runner,
		recorder:      recorder,
	}
}

func executeRecordedScript(t *testing.T, executor *ScriptExecutor) interfaces.WorkResult {
	t.Helper()

	result, err := executor.Execute(context.Background(), sharedRunnerDispatch(interfaces.ExecutionMetadata{
		DispatchCreatedTick: 7,
		CurrentTick:         9,
		RequestID:           "request-script",
		TraceID:             "trace-script",
		WorkIDs:             []string{"work-script"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}

func assertSharedRunnerRequest(t *testing.T, runner *capturingCommandRunner, wantExecution interfaces.ExecutionMetadata) {
	t.Helper()

	runner.mu.Lock()
	got := runner.request
	runner.mu.Unlock()

	if got.Command != "script-tool" {
		t.Fatalf("Command = %q, want script-tool", got.Command)
	}
	if strings.Join(got.Args, " ") != "--work work-script --tag high --project analytics-platform" {
		t.Fatalf("Args = %#v, want resolved templated args", got.Args)
	}
	if got.WorkDir != "/tmp/script-workdir" {
		t.Fatalf("WorkDir = %q, want /tmp/script-workdir", got.WorkDir)
	}
	if got.DispatchID != "dispatch-script" || got.TransitionID != "transition-script" {
		t.Fatalf("canonical dispatch identity = dispatch %q transition %q", got.DispatchID, got.TransitionID)
	}
	if got.ProjectID != "analytics-platform" {
		t.Fatalf("canonical dispatch project ID = %q, want analytics-platform", got.ProjectID)
	}
	commandTokens := CommandRequestInputTokens(got)
	if len(commandTokens) != 1 || commandTokens[0].ID != "token-script" || commandTokens[0].Color.WorkID != "work-script" {
		t.Fatalf("canonical dispatch input tokens = %#v", commandTokens)
	}
	if !envContains(got.Env, "SCRIPT_SHARED_RUNNER_VAR=visible") {
		t.Fatalf("Env did not include merged workflow var: %#v", got.Env)
	}
	if got.Execution.DispatchCreatedTick != wantExecution.DispatchCreatedTick ||
		got.Execution.CurrentTick != wantExecution.CurrentTick ||
		got.Execution.RequestID != wantExecution.RequestID ||
		got.Execution.TraceID != wantExecution.TraceID ||
		got.Execution.ReplayKey != wantExecution.ReplayKey ||
		strings.Join(got.Execution.WorkIDs, ",") != strings.Join(wantExecution.WorkIDs, ",") {
		t.Fatalf("Execution = %#v, want %#v", got.Execution, wantExecution)
	}
}

func TestScriptExecutor_DirectCommandEnvironmentDoesNotAddProviderAutomationDefaults(t *testing.T) {
	ambientValues := map[string]string{
		"GIT_EDITOR":           "vim",
		"GIT_SEQUENCE_EDITOR":  "vim",
		"GIT_MERGE_AUTOEDIT":   "yes",
		"GIT_TERMINAL_PROMPT":  "1",
		"EDITOR":               "vim",
		"VISUAL":               "vim",
		"SCRIPT_WORKER_MARKER": "direct",
	}
	for name, value := range ambientValues {
		t.Setenv(name, value)
	}

	executor := &ScriptExecutor{
		Command:       "print-env",
		CommandRunner: envPrintingCommandRunner{},
	}

	result, err := executor.Execute(context.Background(), testScriptRequest(interfaces.WorkDispatch{
		DispatchID:   "d-script-env",
		TransitionID: "t-script-env",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}

	observed := envSliceToMap(strings.Split(result.Output, "\n"))
	for name, want := range ambientValues {
		if got := observed[name]; got != want {
			t.Fatalf("script worker env %s = %q, want inherited value %q", name, got, want)
		}
	}
	for _, entry := range providerAutomationEnvDefaults {
		if got := observed[entry.name]; got == entry.value {
			t.Fatalf("script worker env %s unexpectedly used provider automation default %q", entry.name, entry.value)
		}
	}
}

func TestScriptExecutor_CommandEnvironmentUsesDispatchEnvOverrides(t *testing.T) {
	t.Setenv("AGENT_FACTORY_SCRIPT_ENV_PRECEDENCE", "process")
	runner := &capturingCommandRunner{}
	executor := &ScriptExecutor{
		Command:       "script-tool",
		CommandRunner: runner,
	}

	result, err := executor.Execute(context.Background(), testScriptRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-script-override-env",
			TransitionID: "t-script-override-env",
		},
		withScriptEnvVars(map[string]string{
			"AGENT_FACTORY_SCRIPT_ENV_PRECEDENCE": "dispatch",
			"AGENT_FACTORY_PROJECT":               "inventory-service",
		}),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}

	runner.mu.Lock()
	env := append([]string(nil), runner.request.Env...)
	runner.mu.Unlock()
	assertEnvValue(t, env, "AGENT_FACTORY_SCRIPT_ENV_PRECEDENCE", "dispatch")
	assertEnvValue(t, env, "AGENT_FACTORY_PROJECT", "inventory-service")
	assertEnvEntryCount(t, env, "AGENT_FACTORY_SCRIPT_ENV_PRECEDENCE", 1)
	assertEnvEntryCount(t, env, "AGENT_FACTORY_PROJECT", 1)
}

func TestScriptExecutor_AttachesCommandDiagnosticsToWorkResult(t *testing.T) {
	executor := &ScriptExecutor{
		Command: "script-tool",
		Args:    []string{"--work", "{{ (index .Inputs 0).WorkID }}"},
		CommandRunner: fixedCommandRunner{
			stdout:   []byte("script stdout\n"),
			stderr:   []byte("script stderr\n"),
			exitCode: 3,
		},
	}

	result, err := executor.Execute(context.Background(), testScriptRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			InputTokens: InputTokens(interfaces.Token{
				ID: "token-1",
				Color: interfaces.TokenColor{
					WorkID:     "work-1",
					WorkTypeID: "task",
					DataType:   interfaces.DataTypeWork,
				},
			}),
		},
		withScriptEnvVars(map[string]string{"SCRIPT_DIAG_VAR": "visible"}),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Diagnostics == nil || result.Diagnostics.Command == nil {
		t.Fatal("expected command diagnostics on work result")
	}
	diag := result.Diagnostics.Command
	if diag.Command != "script-tool" {
		t.Fatalf("diagnostic command = %q, want script-tool", diag.Command)
	}
	if strings.Join(diag.Args, " ") != "--work work-1" {
		t.Fatalf("diagnostic args = %#v, want resolved args", diag.Args)
	}
	if diag.Stdout != "script stdout\n" {
		t.Fatalf("diagnostic stdout = %q", diag.Stdout)
	}
	if diag.Stderr != "script stderr\n" {
		t.Fatalf("diagnostic stderr = %q", diag.Stderr)
	}
	if diag.ExitCode != 3 {
		t.Fatalf("diagnostic exit code = %d, want 3", diag.ExitCode)
	}
	if diag.Env["SCRIPT_DIAG_VAR"] != MetadataOnlyCommandEnvValue {
		t.Fatalf("diagnostic env SCRIPT_DIAG_VAR = %q, want metadata marker", diag.Env["SCRIPT_DIAG_VAR"])
	}
}

func TestScriptExecutor_CommandDiagnosticsRedactSensitiveEnvWithoutChangingExecution(t *testing.T) {
	runner := &capturingCommandRunner{}
	executor := &ScriptExecutor{
		Command:       "script-tool",
		CommandRunner: runner,
	}

	const rawSecret = "super-secret-script-token"
	result, err := executor.Execute(context.Background(), testScriptRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-sensitive-env",
			TransitionID: "t-sensitive-env",
		},
		withScriptEnvVars(map[string]string{
			"CI":                 "true",
			"SCRIPT_API_TOKEN":   rawSecret,
			"SCRIPT_CONTEXT_DIR": "/local/workspace",
		}),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}

	runner.mu.Lock()
	commandEnv := append([]string(nil), runner.request.Env...)
	runner.mu.Unlock()
	if !envContains(commandEnv, "SCRIPT_API_TOKEN="+rawSecret) {
		t.Fatalf("command runner env did not receive raw sensitive value")
	}

	if result.Diagnostics == nil || result.Diagnostics.Command == nil {
		t.Fatal("expected command diagnostics on work result")
	}
	diag := result.Diagnostics.Command
	if got := diag.Env["SCRIPT_API_TOKEN"]; got != RedactedCommandEnvValue {
		t.Fatalf("diagnostic env SCRIPT_API_TOKEN = %q, want redaction marker", got)
	}
	if got := diag.Env["SCRIPT_CONTEXT_DIR"]; got != MetadataOnlyCommandEnvValue {
		t.Fatalf("diagnostic env SCRIPT_CONTEXT_DIR = %q, want metadata marker", got)
	}
	if got := diag.Env["CI"]; got != "true" {
		t.Fatalf("diagnostic env CI = %q, want allowlisted raw value", got)
	}
	if strings.Contains(strings.Join(mapValues(diag.Env), "\n"), rawSecret) {
		t.Fatalf("diagnostic env leaked raw sensitive value")
	}
	if result.Diagnostics.Metadata["env_count"] == "" {
		t.Fatalf("diagnostic metadata missing env_count")
	}
	if !strings.Contains(result.Diagnostics.Metadata["env_keys"], "SCRIPT_API_TOKEN") {
		t.Fatalf("diagnostic metadata env_keys = %q, want sensitive key name", result.Diagnostics.Metadata["env_keys"])
	}
}

func envContains(env []string, want string) bool {
	for _, pair := range env {
		if pair == want {
			return true
		}
	}
	return false
}

func mapValues(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func TestExecutionWorkDir_FallsBackFromWorktreeToContext(t *testing.T) {
	request := testScriptRequest(interfaces.WorkDispatch{}, withScriptWorktree("/tmp/worktree"))
	if got := executionWorkDir(request); got != "/tmp/worktree" {
		t.Fatalf("executionWorkDir() = %q, want %q", got, "/tmp/worktree")
	}

	request = testScriptRequest(interfaces.WorkDispatch{}, withScriptWorkingDirectory("/tmp/context"))
	if got := executionWorkDir(request); got != "/tmp/context" {
		t.Fatalf("executionWorkDir() = %q, want %q", got, "/tmp/context")
	}
}

func TestScriptExecutor_TimeoutStopsProcessBeforeItCanFinish(t *testing.T) {
	outputFile := filepath.Join(t.TempDir(), "helper-finished.txt")
	executor := &ScriptExecutor{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestScriptExecutor_HelperProcess"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := executor.Execute(ctx, testScriptRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-helper",
			TransitionID: "t-helper",
		},
		withScriptEnvVars(map[string]string{
			"GO_WANT_SCRIPT_HELPER":     "1",
			"SCRIPT_HELPER_SLEEP_MS":    "250",
			"SCRIPT_HELPER_OUTPUT_FILE": outputFile,
		}),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Error != "execution timeout" {
		t.Fatalf("Error = %q, want %q", result.Error, "execution timeout")
	}
	if result.ProviderFailure == nil || result.ProviderFailure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("ProviderFailure = %#v, want timeout metadata", result.ProviderFailure)
	}

	time.Sleep(350 * time.Millisecond)
	if _, statErr := os.Stat(outputFile); !os.IsNotExist(statErr) {
		t.Fatalf("expected helper output file to stay absent after timeout, stat err = %v", statErr)
	}
}

func TestScriptExecutor_HelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_SCRIPT_HELPER") != "1" {
		return
	}

	sleepMS, err := strconv.Atoi(os.Getenv("SCRIPT_HELPER_SLEEP_MS"))
	if err != nil {
		t.Fatalf("parse SCRIPT_HELPER_SLEEP_MS: %v", err)
	}

	time.Sleep(time.Duration(sleepMS) * time.Millisecond)

	outputFile := os.Getenv("SCRIPT_HELPER_OUTPUT_FILE")
	if outputFile == "" {
		t.Fatal("SCRIPT_HELPER_OUTPUT_FILE must be set")
	}
	if err := os.WriteFile(outputFile, []byte("finished"), 0o644); err != nil {
		t.Fatalf("write helper output file: %v", err)
	}
}

func stringValueForScriptTest(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringSliceValueForScriptTest(value *[]string) []string {
	if value == nil {
		return nil
	}
	out := make([]string, len(*value))
	copy(out, *value)
	return out
}

func assertScriptRequestEvent(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	if event.Type != factoryapi.FactoryEventTypeScriptRequest {
		t.Fatalf("event type = %s, want %s", event.Type, factoryapi.FactoryEventTypeScriptRequest)
	}
	if event.Id != "factory-event/script-request/dispatch-script/script-request/1" {
		t.Fatalf("event id = %q, want stable request event id", event.Id)
	}
	if stringValueForScriptTest(event.Context.DispatchId) != "dispatch-script" ||
		stringValueForScriptTest(event.Context.RequestId) != "request-script" {
		t.Fatalf("event context = %#v, want dispatch/request correlation", event.Context)
	}
	if got := stringSliceValueForScriptTest(event.Context.TraceIds); len(got) != 1 || got[0] != "trace-script" {
		t.Fatalf("trace IDs = %#v, want trace-script", got)
	}
	if got := stringSliceValueForScriptTest(event.Context.WorkIds); len(got) != 1 || got[0] != "work-script" {
		t.Fatalf("work IDs = %#v, want work-script", got)
	}

	payload, err := event.Payload.AsScriptRequestEventPayload()
	if err != nil {
		t.Fatalf("decode script request payload: %v", err)
	}
	if payload.ScriptRequestId != "dispatch-script/script-request/1" ||
		payload.DispatchId != "dispatch-script" ||
		payload.TransitionId != "transition-script" ||
		payload.Attempt != 1 ||
		payload.Command != "script-tool" ||
		strings.Join(payload.Args, " ") != "--work work-script --priority high" {
		t.Fatalf("script request payload = %#v, want stable request fields", payload)
	}

	assertEventDoesNotLeakScriptInternals(t, event)
}

func assertScriptResponsePayload(
	t *testing.T,
	event factoryapi.FactoryEvent,
	wantOutcome factoryapi.ScriptExecutionOutcome,
	wantFailureType *factoryapi.ScriptFailureType,
	wantExitCode *int,
	wantStdout string,
	wantStderr string,
) {
	t.Helper()

	if stringValueForScriptTest(event.Context.DispatchId) != "dispatch-script" ||
		stringValueForScriptTest(event.Context.RequestId) != "request-script" {
		t.Fatalf("response context = %#v, want dispatch/request correlation", event.Context)
	}
	if got := stringSliceValueForScriptTest(event.Context.TraceIds); len(got) != 1 || got[0] != "trace-script" {
		t.Fatalf("response trace IDs = %#v, want trace-script", got)
	}
	if got := stringSliceValueForScriptTest(event.Context.WorkIds); len(got) != 1 || got[0] != "work-script" {
		t.Fatalf("response work IDs = %#v, want work-script", got)
	}

	payload, err := event.Payload.AsScriptResponseEventPayload()
	if err != nil {
		t.Fatalf("decode script response payload: %v", err)
	}
	if payload.ScriptRequestId != "dispatch-script/script-request/1" ||
		payload.DispatchId != "dispatch-script" ||
		payload.TransitionId != "transition-script" ||
		payload.Attempt != 1 ||
		payload.Outcome != wantOutcome ||
		payload.Stdout != wantStdout ||
		payload.Stderr != wantStderr ||
		payload.DurationMillis < 0 {
		t.Fatalf("script response payload = %#v, want stable response fields", payload)
	}
	if !equalOptionalScriptFailureType(payload.FailureType, wantFailureType) {
		t.Fatalf("script response failure type = %#v, want %#v", payload.FailureType, wantFailureType)
	}
	if !equalOptionalInt(payload.ExitCode, wantExitCode) {
		t.Fatalf("script response exit code = %#v, want %#v", payload.ExitCode, wantExitCode)
	}

	assertEventDoesNotLeakScriptInternals(t, event)
}

func assertEventDoesNotLeakScriptInternals(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal script event: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{`"stdin"`, `"env"`, `"SCRIPT_SHARED_RUNNER_VAR"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("script event leaked %s: %s", forbidden, body)
		}
	}
}

func equalOptionalScriptFailureType(left, right *factoryapi.ScriptFailureType) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalOptionalInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func intPtr(value int) *int {
	return &value
}
