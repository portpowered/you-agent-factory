package script

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

func TestRunnerResolvesConfiguredInvocationDeterministically(t *testing.T) {
	commandEdge := &captureCommandRunner{
		result: workerprocess.CommandResult{Stdout: []byte("  completed  \n")},
	}
	factoryDirectory := filepath.Join("factory-root", "selected")
	scriptRunner := New(Config{
		Command:          "scripts/run.sh",
		FactoryDirectory: factoryDirectory,
		Args: []string{
			`{{ (index .Inputs 0).Name }}`,
			`{{ index (index .Inputs 0).Tags "lane" }}`,
			`{{ (index .Inputs 0).Payload }}`,
			`{{ (index .Inputs 0).Project }}`,
			`{{ .Context.Project }}`,
			`{{ index .Docs "guide.md" }}`,
			`{{ .Context.Env.RUNTIME }}`,
			`{{ .Context.WorkDir }}`,
			`{{ (index (index .Inputs 0).Content 0).Text }}`,
			"factory/scripts/helper.sh",
			"relative/value",
			"C:/absolute/tool",
		},
	}, commandEdge, func(directory string) (map[string]string, error) {
		if directory != factoryDirectory {
			t.Fatalf("Factory docs directory = %q, want %q", directory, factoryDirectory)
		}
		return map[string]string{"guide.md": "factory guidance"}, nil
	}, func() time.Time { return time.Unix(0, 0) }, func(workers.ProgressFragment) {}, func(workers.ScriptEvent) {},
	)

	request := validRequest()
	result, err := scriptRunner.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "completed" {
		t.Fatalf("result content = %q, want completed", result.Content)
	}

	captured := commandEdge.Request()
	wantArgs := []string{
		"input-name",
		"fast",
		"payload-value",
		"input-project",
		"request-project",
		"factory guidance",
		"request-env",
		"explicit-work-dir",
		"content-value",
		filepath.Join(factoryDirectory, "scripts", "helper.sh"),
		"relative/value",
		"C:/absolute/tool",
	}
	if !reflect.DeepEqual(captured.Args, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", captured.Args, wantArgs)
	}
	if captured.Command != filepath.Join(factoryDirectory, "scripts", "run.sh") {
		t.Fatalf("command = %q, want Factory-relative script", captured.Command)
	}
	if captured.WorkDir != "explicit-work-dir" {
		t.Fatalf("working directory = %q, want explicit-work-dir", captured.WorkDir)
	}
	assertEnvAbsent(t, captured.Env, "BASE")
	assertEnv(t, captured.Env, "RUNTIME", "request-env")
	assertEnvCount(t, captured.Env, "RUNTIME", 1)
	if captured.DispatchID != "dispatch-1" ||
		captured.WorkerType != "request-worker" ||
		captured.WorkstationName != "request-workstation" ||
		captured.ProjectID != "request-project" ||
		captured.Execution.RequestID != "request-1" {
		t.Fatalf("command execution metadata = %#v", captured)
	}
}

func TestRunnerUsesDetachedWorkflowContextForPromptAndCommand(t *testing.T) {
	commandEdge := &captureCommandRunner{result: workerprocess.CommandResult{Stdout: []byte("completed")}}
	contextFactoryDirectory := filepath.Join("factory-root", "detached")
	scriptRunner := New(Config{
		Command:          "scripts/run.sh",
		FactoryDirectory: filepath.Join("factory-root", "configured"),
		Args: []string{
			`{{ .Context.Project }}`,
			`{{ .Context.Env.RUNTIME }}`,
			`{{ .Context.WorkDir }}`,
			`{{ .Context.SessionID }}`,
		},
	}, commandEdge, func(directory string) (map[string]string, error) {
		if directory != contextFactoryDirectory {
			t.Fatalf("Factory docs directory = %q, want detached context directory %q", directory, contextFactoryDirectory)
		}
		return nil, nil
	}, func() time.Time { return time.Unix(0, 0) }, func(workers.ProgressFragment) {}, func(workers.ScriptEvent) {},
	)

	request := validRequest()
	request.FactoryDirectory = ""
	request.WorkingDirectory = ""
	request.Worktree = ""
	request.ProjectID = ""
	request.EnvVars = nil
	request.SessionID = ""
	request.Correlation = workers.ExecutionCorrelation{FactorySessionID: "session-correlation"}
	request.WorkflowContext = &workers.Context{
		FactoryDirectory: contextFactoryDirectory,
		WorkDirectory:    "detached-work-dir",
		EnvVars:          map[string]string{"RUNTIME": "detached-env"},
		ProjectID:        "detached-project",
		SessionID:        "detached-session",
	}
	if _, err := scriptRunner.Execute(t.Context(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	captured := commandEdge.Request()
	wantArgs := []string{"detached-project", "detached-env", "detached-work-dir", "detached-session"}
	if !reflect.DeepEqual(captured.Args, wantArgs) {
		t.Fatalf("command args = %#v, want detached workflow context values %#v", captured.Args, wantArgs)
	}
	if captured.Command != filepath.Join(contextFactoryDirectory, "scripts", "run.sh") {
		t.Fatalf("command = %q, want detached context factory script", captured.Command)
	}
	if captured.WorkDir != "detached-work-dir" || captured.ProjectID != "detached-project" ||
		captured.FactorySessionID != "detached-session" {
		t.Fatalf("command context = %#v, want detached workflow context", captured)
	}
	assertEnv(t, captured.Env, "RUNTIME", "detached-env")
}

func TestRunnerUsesRequestScopedEffectsAndCommandOverrides(t *testing.T) {
	cases := []struct {
		name   string
		edge   workerprocess.CommandRunner
		chunks []outputChunk
	}{
		{
			name: "non-streaming override preserves output and correlation",
			edge: nonStreamingCommandRunner{result: workerprocess.CommandResult{
				Stdout: []byte("override stdout"), Stderr: []byte("override stderr"),
			}},
			chunks: []outputChunk{{platformprocess.OutputStreamStdout, "override stdout"},
				{platformprocess.OutputStreamStderr, "override stderr"}},
		},
		{
			name: "streaming override keeps streaming boundary",
			edge: &streamingCommandEdge{
				chunks: []outputChunk{{platformprocess.OutputStreamStdout, "streamed"}},
				result: workerprocess.CommandResult{Stdout: []byte("streamed")},
			},
			chunks: []outputChunk{{platformprocess.OutputStreamStdout, "streamed"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			constructionLog, overrideLog := &observationLog{}, &observationLog{}
			constructionChunks := []outputChunk{{platformprocess.OutputStreamStdout, "construction"},
				{platformprocess.OutputStreamStderr, "construction note"}}
			constructionRunner := &streamingCommandEdge{
				observations: constructionLog, chunks: constructionChunks,
				result: workerprocess.CommandResult{Stdout: []byte("construction"), Stderr: []byte("construction note")},
			}
			started := time.Unix(100, 0)
			clock := &sequenceClock{times: []time.Time{started, started.Add(time.Second),
				started.Add(2 * time.Second), started.Add(3 * time.Second)}}
			scriptRunner := New(Config{Command: "echo", Args: []string{"safe-arg"}},
				constructionRunner, emptyDocs, clock.Now, constructionLog.CaptureProgress, constructionLog.CaptureEvent)
			request := observedRequest()
			ctx := workerexecution.WithWorkerCommandRunnerOverride(t.Context(), tc.edge)
			ctx = workerexecution.WithProgressPublisher(ctx, overrideLog.CaptureProgress)
			ctx = workerexecution.WithScriptEventRecorder(ctx, overrideLog.CaptureEvent)
			result, err := scriptRunner.Execute(ctx, request)
			if err != nil {
				t.Fatalf("override Execute() error = %v", err)
			}
			assertObservedValue(t, "override output", result.Content, tc.chunks[0].payload)
			assertAttributedObservations(t, overrideLog, request, started, started.Add(time.Second),
				"echo", []string{"safe-arg"}, tc.chunks)
			assertObservedValue(t, "construction command calls before default", constructionLog.Values(), []string(nil))
			assertObservedValue(t, "construction progress before default", len(constructionLog.fragments), 0)
			assertObservedValue(t, "construction events before default", len(constructionLog.events), 0)
			assertDefaultExecutionAfterOverride(t, scriptRunner, constructionRunner, constructionLog,
				overrideLog, request, result, started, constructionChunks, tc.chunks)
		})
	}
}

func TestCommandRunnerWithStreamingFallbackPreservesResultWithoutObserver(t *testing.T) {
	want := workerprocess.CommandResult{Stdout: []byte("stdout"), Stderr: []byte("stderr"), ExitCode: 3}
	runner := commandRunnerWithStreamingFallback{runner: nonStreamingCommandRunner{result: want}}
	got, err := runner.RunStreaming(context.Background(), workerprocess.CommandRequest{}, nil)
	if err != nil {
		t.Fatalf("RunStreaming() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RunStreaming() result = %#v, want %#v", got, want)
	}
}

func TestRunnerReturnsSuccessfulOutputWithOrderedSafeDiagnostics(t *testing.T) {
	observations := &observationLog{}
	commandEdge := &streamingCommandEdge{
		observations: observations,
		chunks: []outputChunk{
			{stream: platformprocess.OutputStreamStdout, payload: "first"},
			{stream: platformprocess.OutputStreamStderr, payload: "warn-1"},
			{stream: platformprocess.OutputStreamStdout, payload: "second\n"},
			{stream: platformprocess.OutputStreamStderr, payload: "warn-2"},
		},
		result: workerprocess.CommandResult{
			Stdout:   []byte("firstsecond\n"),
			Stderr:   []byte("warn-1warn-2"),
			ExitCode: 0,
		},
	}
	started := time.Date(2026, 7, 26, 20, 0, 0, 0, time.FixedZone("selected", 3600))
	clock := &sequenceClock{times: []time.Time{started, started.Add(1500 * time.Millisecond)}}
	scriptRunner := New(Config{Command: "echo", Args: []string{"safe-arg"}},
		commandEdge,
		emptyDocs,
		clock.Now,
		func(fragment workers.ProgressFragment) {
			observations.CaptureProgress(fragment)
			observations.Append(fragment.Type + ":" + fragment.Payload)
			fragment.Metadata["stream"] = "mutated"
		},
		func(event workers.ScriptEvent) {
			observations.CaptureEvent(event)
			switch event.Kind {
			case workers.ScriptEventKindRequest:
				observations.Append("request")
				event.Request.Args[0] = "mutated"
			case workers.ScriptEventKindResponse:
				observations.Append("terminal")
				observations.SetTerminal(*event.Response)
				event.Response.Stdout = "mutated"
			}
		},
	)

	request := observedRequest()
	request.EnvVars["CI"] = "true"
	request.EnvVars["SCRIPT_API_TOKEN"] = "fixture-secret"

	result, err := scriptRunner.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "firstsecond" {
		t.Fatalf("result content = %q, want trimmed stdout", result.Content)
	}
	wantOrder := []string{
		"request",
		"command",
		"stdout:first",
		"stderr:warn-1",
		"stdout:second\n",
		"stderr:warn-2",
		"terminal",
	}
	if got := observations.Values(); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("observation order = %#v, want %#v", got, wantOrder)
	}
	assertSuccessfulDiagnostics(t, result, 1500*time.Millisecond)
	assertAttributedObservations(t, observations, request, started, started.Add(1500*time.Millisecond), "echo", []string{"safe-arg"}, commandEdge.chunks)
	terminal := observations.Terminal()
	if terminal.Stdout != "firstsecond\n" || terminal.Stderr != "warn-1warn-2" {
		t.Fatalf("terminal output = stdout %q stderr %q", terminal.Stdout, terminal.Stderr)
	}
	assertObservedValue(t, "success exit code", terminal.ExitCode, intPointer(0))
	assertObservedValue(t, "success outcome", terminal.Outcome, workers.ScriptExecutionOutcomeSucceeded)
	assertObservedValue(t, "success duration", terminal.DurationMillis, int64(1500))
	assertObservedValue(t, "success failure type", terminal.FailureType, (*workers.ScriptFailureType)(nil))
	if captured := commandEdge.Request(); !reflect.DeepEqual(captured.Args, []string{"safe-arg"}) {
		t.Fatalf("command args = %#v, want recorder mutation isolated", captured.Args)
	}
}

func TestRunnerSuccessResultsStayDetachedAcrossRepeatedAndConcurrentExecutions(t *testing.T) {
	commandEdge := &streamingCommandEdge{
		result: workerprocess.CommandResult{Stdout: []byte("stable"), Stderr: []byte("note")},
	}
	scriptRunner := New(Config{Command: "echo", Args: []string{"one"}},
		commandEdge,
		emptyDocs,
		func() time.Time { return time.Unix(100, 0) },
		func(fragment workers.ProgressFragment) {
			fragment.Metadata["stream"] = "mutated"
		},
		func(event workers.ScriptEvent) {
			if event.Request != nil {
				event.Request.Args[0] = "mutated"
			}
			if event.Response != nil {
				event.Response.Stdout = "mutated"
			}
		},
	)

	first, err := scriptRunner.Execute(t.Context(), validRequest())
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	first.Diagnostics.Command.Args[0] = "mutated"
	first.Diagnostics.Command.Env["BASE"] = "mutated"
	first.Diagnostics.Metadata["dispatch_id"] = "mutated"

	const executions = 16
	errs := make(chan error, executions)
	var wait sync.WaitGroup
	for range executions {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, executeErr := scriptRunner.Execute(context.Background(), validRequest())
			if executeErr != nil {
				errs <- executeErr
				return
			}
			if result.Content != "stable" ||
				result.Diagnostics.Command.Args[0] != "one" ||
				result.Diagnostics.Metadata["dispatch_id"] != "dispatch-1" {
				errs <- errors.New("concurrent result was not detached")
			}
		}()
	}
	wait.Wait()
	close(errs)
	for executeErr := range errs {
		t.Fatal(executeErr)
	}
}

func TestRunnerNormalizesCommandFailuresWithPartialDiagnosticsAndOneTerminalResponse(t *testing.T) {
	processFailure := errors.New("exec: executable file not found")
	tests := []struct {
		name            string
		result          workerprocess.CommandResult
		commandErr      error
		wantMessage     string
		wantOutcome     workers.ScriptExecutionOutcome
		wantFailureType *workers.ScriptFailureType
		wantWorkFailure workers.WorkFailureType
		wantExitCode    *int
	}{
		{
			name: "non-zero exit",
			result: workerprocess.CommandResult{
				Stdout:   []byte("partial stdout\n"),
				Stderr:   []byte("command rejected input\n"),
				ExitCode: 17,
			},
			wantMessage:  "command rejected input",
			wantOutcome:  workers.ScriptExecutionOutcomeFailedExitCode,
			wantExitCode: intPointer(17),
		},
		{
			name: "process start failure",
			result: workerprocess.CommandResult{
				Stdout: []byte("partial stdout"),
				Stderr: []byte("partial stderr"),
			},
			commandErr:      processFailure,
			wantMessage:     "script command execution failed",
			wantOutcome:     workers.ScriptExecutionOutcomeProcessError,
			wantFailureType: scriptFailureTypePointer(workers.ScriptFailureTypeProcessError),
			wantWorkFailure: workers.WorkFailureTypeInternalServerError,
		},
		{
			name:            "command deadline without context cancellation",
			result:          workerprocess.CommandResult{Stdout: []byte("partial stdout"), Stderr: []byte("partial stderr")},
			commandErr:      context.DeadlineExceeded,
			wantMessage:     "execution timeout",
			wantOutcome:     workers.ScriptExecutionOutcomeTimedOut,
			wantFailureType: scriptFailureTypePointer(workers.ScriptFailureTypeTimeout),
			wantWorkFailure: workers.WorkFailureTypeTimeout,
		},
		{
			name: "missing executable",
			result: workerprocess.CommandResult{
				Stdout: []byte("partial stdout"),
				Stderr: []byte("partial stderr"),
			},
			commandErr:      exec.ErrNotFound,
			wantMessage:     "Script executable could not be found.",
			wantOutcome:     workers.ScriptExecutionOutcomeProcessError,
			wantFailureType: scriptFailureTypePointer(workers.ScriptFailureTypeProcessError),
			wantWorkFailure: workers.WorkFailureTypeMissingExecutable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantWorkFailure := test.wantWorkFailure
			if wantWorkFailure == "" {
				wantWorkFailure = workers.WorkFailureTypeInternalServerError
			}
			runCommandFailureCase(
				t,
				test.result,
				test.commandErr,
				test.wantMessage,
				test.wantOutcome,
				test.wantFailureType,
				wantWorkFailure,
				test.wantExitCode,
			)
		})
	}
}

func runCommandFailureCase(
	t *testing.T,
	commandResult workerprocess.CommandResult,
	commandErr error,
	wantMessage string,
	wantOutcome workers.ScriptExecutionOutcome,
	wantFailureType *workers.ScriptFailureType,
	wantWorkFailure workers.WorkFailureType,
	wantExitCode *int,
) {
	t.Helper()
	observations := &observationLog{}
	commandEdge := &streamingCommandEdge{
		observations: observations,
		chunks: []outputChunk{
			{stream: platformprocess.OutputStreamStdout, payload: "partial stdout"},
			{stream: platformprocess.OutputStreamStderr, payload: "partial stderr"},
		},
		result: commandResult,
		err:    commandErr,
	}
	started := time.Date(2026, 7, 26, 21, 0, 0, 0, time.FixedZone("selected", -7200))
	scriptRunner := New(Config{Command: "missing-tool"},
		commandEdge,
		emptyDocs,
		(&sequenceClock{times: []time.Time{started, started.Add(2 * time.Second)}}).Now,
		func(fragment workers.ProgressFragment) {
			observations.CaptureProgress(fragment)
			observations.Append(fragment.Type + ":" + fragment.Payload)
		},
		func(event workers.ScriptEvent) {
			observations.CaptureEvent(event)
			if event.Request != nil {
				observations.Append("request")
			}
			if event.Response != nil {
				observations.Append("terminal")
				observations.SetTerminal(*event.Response)
			}
		},
	)

	request := observedRequest()
	result, executeErr := scriptRunner.Execute(t.Context(), request)
	var failure *workers.ProviderError
	if !errors.As(executeErr, &failure) {
		t.Fatalf("Execute() error = %#v, want ProviderError", executeErr)
	}
	assertObservedValue(t, "error type", failure.Type, wantWorkFailure)
	assertObservedValue(t, "error message", failure.Message, wantMessage)
	assertObservedValue(t, "error cause", failure.Cause, commandErr)
	if commandErr != nil && !errors.Is(executeErr, commandErr) {
		t.Fatalf("Execute() error = %v, want process cause %v", executeErr, commandErr)
	}
	assertFailureDiagnostics(t, result.Diagnostics, commandResult, 2*time.Second)
	assertFailureDiagnostics(t, failure.Diagnostics, commandResult, 2*time.Second)
	assertObservedValue(t, "failure content", result.Content, strings.TrimSpace(string(commandResult.Stdout)))
	assertObservedValue(t, "failure timed out", result.Diagnostics.Command.TimedOut, errors.Is(commandErr, context.DeadlineExceeded))
	assertAttributedObservations(t, observations, request, started, started.Add(2*time.Second), "missing-tool", nil, commandEdge.chunks)
	assertFailureObservation(t, observations, commandResult, wantOutcome, wantFailureType, wantExitCode)

	result.Diagnostics.Command.Stdout = "mutated"
	if failure.Diagnostics.Command.Stdout != string(commandResult.Stdout) {
		t.Fatalf("failure diagnostics changed through returned result mutation: %#v", failure.Diagnostics.Command)
	}
}

func assertFailureObservation(
	t *testing.T,
	observations *observationLog,
	result workerprocess.CommandResult,
	wantOutcome workers.ScriptExecutionOutcome,
	wantFailureType *workers.ScriptFailureType,
	wantExitCode *int,
) {
	t.Helper()
	wantOrder := []string{
		"request",
		"command",
		"stdout:partial stdout",
		"stderr:partial stderr",
		"terminal",
	}
	if got := observations.Values(); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("observation order = %#v, want %#v", got, wantOrder)
	}
	terminal := observations.Terminal()
	assertObservedValue(t, "failure outcome", terminal.Outcome, wantOutcome)
	assertObservedValue(t, "failure duration", terminal.DurationMillis, int64(2000))
	assertObservedValue(t, "failure exit code", terminal.ExitCode, wantExitCode)
	assertObservedValue(t, "failure type", terminal.FailureType, wantFailureType)
	assertObservedValue(t, "failure stdout", terminal.Stdout, string(result.Stdout))
	assertObservedValue(t, "failure stderr", terminal.Stderr, string(result.Stderr))
}

func TestRunnerValidationFailureDoesNotRecordOrStartCommand(t *testing.T) {
	observations := &observationLog{}
	commandEdge := &streamingCommandEdge{observations: observations}
	scriptRunner := New(
		Config{Command: "echo", Args: []string{"{{"}},

		commandEdge,
		emptyDocs,
		func() time.Time { return time.Unix(0, 0) },
		func(workers.ProgressFragment) { observations.Append("progress") },
		func(workers.ScriptEvent) { observations.Append("event") },
	)

	_, executeErr := scriptRunner.Execute(t.Context(), validRequest())
	assertFailureType(t, executeErr, workers.WorkFailureTypePermanentBadRequest)
	if got := observations.Values(); len(got) != 0 {
		t.Fatalf("observations = %#v, want none for validation rejection", got)
	}
}

func assertFailureDiagnostics(
	t *testing.T,
	diagnostics *workers.WorkDiagnostics,
	commandResult workerprocess.CommandResult,
	wantDuration time.Duration,
) {
	t.Helper()
	if diagnostics == nil || diagnostics.Command == nil {
		t.Fatalf("diagnostics = %#v, want command diagnostics", diagnostics)
	}
	command := diagnostics.Command
	if command.Command != "missing-tool" ||
		command.Stdout != string(commandResult.Stdout) ||
		command.Stderr != string(commandResult.Stderr) ||
		command.ExitCode != commandResult.ExitCode ||
		command.Duration != wantDuration {
		t.Fatalf("command diagnostics = %#v", command)
	}
}

func intPointer(value int) *int {
	return &value
}

func scriptFailureTypePointer(value workers.ScriptFailureType) *workers.ScriptFailureType {
	return &value
}

func assertSuccessfulDiagnostics(
	t *testing.T,
	result workers.RunnerExecutionResult,
	wantDuration time.Duration,
) {
	t.Helper()
	if result.Diagnostics == nil || result.Diagnostics.Command == nil {
		t.Fatalf("result diagnostics = %#v, want command diagnostics", result.Diagnostics)
	}
	command := result.Diagnostics.Command
	assertCommandDiagnosticFields(t, command, wantDuration)
	assertCommandEnvironmentDiagnostics(t, command.Env)
	assertCommandLineageDiagnostics(t, result.Diagnostics.Metadata)
}

func assertCommandDiagnosticFields(
	t *testing.T,
	command *workers.CommandDiagnostic,
	wantDuration time.Duration,
) {
	t.Helper()
	if command.Command != "echo" ||
		!reflect.DeepEqual(command.Args, []string{"safe-arg"}) ||
		command.WorkingDir != "explicit-work-dir" ||
		command.ExitCode != 0 ||
		command.Duration != wantDuration ||
		command.Stdout != "firstsecond\n" ||
		command.Stderr != "warn-1warn-2" {
		t.Fatalf("command diagnostics = %#v", command)
	}
}

func assertCommandEnvironmentDiagnostics(t *testing.T, env map[string]string) {
	t.Helper()
	if got := env["SCRIPT_API_TOKEN"]; got != workers.RedactedCommandEnvValue {
		t.Fatalf("sensitive env diagnostic = %q, want redacted", got)
	}
	if got := env["CI"]; got != "true" {
		t.Fatalf("allowlisted env diagnostic = %q, want true", got)
	}
	if got := env["RUNTIME"]; got != workers.MetadataOnlyCommandEnvValue {
		t.Fatalf("metadata-only env diagnostic = %q", got)
	}
}

func assertCommandLineageDiagnostics(t *testing.T, metadata map[string]string) {
	t.Helper()
	if metadata["dispatch_id"] != "dispatch-1" ||
		metadata["transition_id"] != "transition-1" ||
		metadata["current_chaining_trace_id"] != "trace-current" ||
		metadata["previous_chaining_trace_ids"] != "trace-previous" ||
		metadata["request_id"] != "request-1" {
		t.Fatalf("diagnostic lineage = %#v", metadata)
	}
}

func TestRunnerWorkingDirectoryPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		workingDir string
		worktree   string
		want       string
	}{
		{name: "explicit beats worktree", workingDir: "explicit", worktree: "worktree", want: "explicit"},
		{name: "worktree fallback", worktree: "worktree", want: "worktree"},
		{name: "absent stays absent"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commandEdge := &captureCommandRunner{}
			scriptRunner := newTestRunner(t, Config{Command: "echo"}, commandEdge)
			request := validRequest()
			request.WorkingDirectory = test.workingDir
			request.Worktree = test.worktree
			if _, err := scriptRunner.Execute(t.Context(), request); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got := commandEdge.Request().WorkDir; got != test.want {
				t.Fatalf("working directory = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRunnerRejectsInvalidInputBeforeCommandExecution(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		mutate   func(*workers.RunnerExecutionRequest)
		wantType workers.WorkFailureType
		wantCap  bool
	}{
		{
			name:     "invalid argument template",
			config:   Config{Command: "echo", Args: []string{"{{"}},
			wantType: workers.WorkFailureTypePermanentBadRequest,
		},
		{
			name:   "unsupported image content",
			config: Config{Command: "echo"},
			mutate: func(request *workers.RunnerExecutionRequest) {
				token := request.InputTokens[0].(workers.Token)
				token.Color.Content = []work.WorkContentPart{{
					Type: work.WorkContentPartTypeImage,
				}}
				request.InputTokens[0] = token
			},
			wantType: workers.WorkFailureTypePermanentBadRequest,
		},
		{
			name:   "unsupported authored image alias",
			config: Config{Command: "echo"},
			mutate: func(request *workers.RunnerExecutionRequest) {
				token := request.InputTokens[0].(workers.Token)
				token.Color.Content = []work.WorkContentPart{{
					Type: "IMAGE",
				}}
				request.InputTokens[0] = token
			},
			wantType: workers.WorkFailureTypePermanentBadRequest,
		},
		{
			name:   "unsupported capability",
			config: Config{Command: "echo"},
			mutate: func(request *workers.RunnerExecutionRequest) {
				request.RequiredOptionalCapabilities = []workers.RunnerOptionalCapability{
					workers.RunnerOptionalCapabilityImageInput,
				}
			},
			wantCap: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commandEdge := &captureCommandRunner{}
			var effects []string
			scriptRunner := New(test.config, commandEdge, func(string) (map[string]string, error) {
				effects = append(effects, "docs")
				return nil, nil
			},
				func() time.Time { effects = append(effects, "clock"); return time.Time{} },
				func(workers.ProgressFragment) { effects = append(effects, "progress") },
				func(workers.ScriptEvent) { effects = append(effects, "record") },
			)

			request := validRequest()
			if test.mutate != nil {
				test.mutate(&request)
			}
			_, err := scriptRunner.Execute(t.Context(), request)
			if test.wantCap {
				if !errors.Is(err, workers.ErrUnsupportedRunnerCapability) {
					t.Fatalf("Execute() error = %v, want unsupported capability", err)
				}
			} else {
				assertFailureType(t, err, test.wantType)
			}
			if commandEdge.Calls() != 0 {
				t.Fatalf("command calls = %d, want 0", commandEdge.Calls())
			}
			for _, effect := range effects {
				if effect != "docs" || test.name != "invalid argument template" {
					t.Fatalf("unexpected effect before input rejection: %s", effect)
				}
			}
		})
	}
}

func TestRunnerSnapshotsCallerOwnedDataBeforeInjectedWork(t *testing.T) {
	docsEntered := make(chan struct{})
	releaseDocs := make(chan struct{})
	commandEdge := &captureCommandRunner{}
	config := Config{
		Command:          "echo",
		FactoryDirectory: "factory-root",
		Args: []string{
			`{{ (index .Inputs 0).Payload }}`,
			`{{ (index (index .Inputs 0).Content 0).Text }}`,
			`{{ .Context.Env.RUNTIME }}`,
		},
	}
	scriptRunner := New(config, commandEdge, func(string) (map[string]string, error) {
		close(docsEntered)
		<-releaseDocs
		return map[string]string{}, nil
	}, func() time.Time { return time.Unix(0, 0) }, func(workers.ProgressFragment) {}, func(workers.ScriptEvent) {},
	)

	config.Args[0] = "mutated-config"
	request := validRequest()

	done := make(chan error, 1)
	go func() {
		_, executeErr := scriptRunner.Execute(context.Background(), request)
		done <- executeErr
	}()
	<-docsEntered
	token := request.InputTokens[0].(workers.Token)
	token.Color.Payload[0] = 'X'
	token.Color.Tags["lane"] = "mutated"
	token.Color.Content[0].Text = "mutated"
	request.InputTokens[0] = token
	request.EnvVars["RUNTIME"] = "mutated"
	request.Dispatch.Execution.RequestID = "mutated"
	request.RequiredOptionalCapabilities[0] = workers.RunnerOptionalCapabilityImageInput
	close(releaseDocs)
	if err := <-done; err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	captured := commandEdge.Request()
	if !reflect.DeepEqual(captured.Args, []string{"payload-value", "content-value", "request-env"}) {
		t.Fatalf("captured args = %#v, want caller-owned originals", captured.Args)
	}
	if captured.Execution.RequestID != "request-1" {
		t.Fatalf("captured request ID = %q, want request-1", captured.Execution.RequestID)
	}
	if got := captured.Inputs[0].Tags["lane"]; got != "fast" {
		t.Fatalf("captured input tag = %q, want fast", got)
	}
}

func TestRunnerPreservesPreCanceledContextWithoutCallingEffects(t *testing.T) {
	commandEdge := &captureCommandRunner{}
	var effects []string
	scriptRunner := New(Config{Command: "echo"},
		commandEdge,
		func(string) (map[string]string, error) { effects = append(effects, "docs"); return nil, nil },
		func() time.Time { effects = append(effects, "clock"); return time.Time{} },
		func(workers.ProgressFragment) { effects = append(effects, "progress") },
		func(workers.ScriptEvent) { effects = append(effects, "record") },
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := scriptRunner.Execute(ctx, validRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	assertObservedValue(t, "pre-canceled result", result, workers.RunnerExecutionResult{})
	assertObservedValue(t, "pre-canceled command calls", commandEdge.Calls(), 0)
	assertObservedValue(t, "pre-canceled effects", effects, []string(nil))
}

func TestEngineNeutralInputAndScriptEventHelpersPreserveBoundaryFacts(t *testing.T) {
	published := make([]workers.ProgressFragment, 0, 1)
	observer := (&runner{
		publish: func(fragment workers.ProgressFragment) {
			published = append(published, fragment)
		},
	}).outputObserver("dispatch-1", workers.ExecutionCorrelation{FactorySessionID: "session-1"})
	observer(platformprocess.OutputStreamStdout, nil)
	observer(platformprocess.OutputStreamStdout, []byte("partial"))
	if len(published) != 1 || published[0].Payload != "partial" {
		t.Fatalf("published progress = %#v, want one non-empty fragment", published)
	}

	if got := scriptResponseEventID(""); got != "factory-event/script-response/1" {
		t.Fatalf("scriptResponseEventID(empty) = %q, want deterministic empty-dispatch ID", got)
	}
	if got := scriptEventTick(work.ExecutionMetadata{CurrentTick: 7, DispatchCreatedTick: 3}); got != 7 {
		t.Fatalf("scriptEventTick() = %d, want current tick", got)
	}

	inputs := commandInputs([]workers.Token{{
		ID:    "token-1",
		State: "ready",
		Color: workers.Color{
			DataType:   workers.DataTypeWork,
			Payload:    []byte("payload fallback"),
			WorkID:     "work-1",
			WorkTypeID: "task",
		},
	}}, map[string][]string{"input": {"token-1"}})
	if len(inputs) != 1 || inputs[0].State != "ready" || len(inputs[0].Content) != 1 || inputs[0].Content[0].Text != "payload fallback" ||
		len(inputs[0].InputNames) != 1 || inputs[0].InputNames[0] != "input" {
		t.Fatalf("command inputs = %#v, want detached state, payload, and binding facts", inputs)
	}
}

func TestRunnerRejectsInvalidDetachedInputSnapshots(t *testing.T) {
	scriptRunner := newTestRunner(t, Config{Command: "echo"}, &captureCommandRunner{})

	request := validRequest()
	request.InputTokens = []any{make(chan int)}
	_, err := scriptRunner.Execute(t.Context(), request)
	assertFailureType(t, err, workers.WorkFailureTypePermanentBadRequest)

	request = validRequest()
	request.Dispatch.InputTokens = []any{make(chan int)}
	_, err = scriptRunner.Execute(t.Context(), request)
	assertFailureType(t, err, workers.WorkFailureTypePermanentBadRequest)
}

func validRequest() workers.RunnerExecutionRequest {
	token := workers.Token{Color: workers.Color{
		Name:     "input-name",
		WorkID:   "work-1",
		DataType: workers.DataTypeWork,
		Tags: map[string]string{
			"lane":                "fast",
			workers.ProjectTagKey: "input-project",
		},
		Payload: []byte("payload-value"),
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "content-value",
		}},
	}}
	return workers.RunnerExecutionRequest{
		RunnerID:        Identity,
		WorkerType:      "request-worker",
		WorkstationType: "request-workstation",
		ProjectID:       "request-project",
		SessionID:       "session-1",
		InputTokens:     []any{token},
		EnvVars:         map[string]string{"RUNTIME": "request-env"},
		ProcessEnvironment: []string{
			"BASE=injected",
			"RUNTIME=base",
			"RUNTIME=stale",
		},
		WorkingDirectory: "explicit-work-dir",
		Worktree:         "worktree-fallback",
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityWorkingDirectory,
		},
		Dispatch: work.WorkDispatch{
			DispatchID:             "dispatch-1",
			TransitionID:           "transition-1",
			WorkerType:             "dispatch-worker",
			WorkstationName:        "dispatch-workstation",
			ProjectID:              "dispatch-project",
			CurrentChainingTraceID: "trace-current",
			PreviousChainingTraceIDs: []string{
				"trace-previous",
			},
			InputTokens: []any{token},
			InputBindings: map[string][]string{
				"input": {"work-1"},
			},
			Execution: work.ExecutionMetadata{
				RequestID: "request-1",
				TraceID:   "trace-1",
				WorkIDs:   []string{"work-1"},
			},
		},
	}
}

func TestScriptStdinTemplatesAndOmission(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, stdin, want string
		fail              bool
		exitCode          int
	}{
		{name: "payload", stdin: `{{ (index .Inputs 0).Payload }}`, want: "payload-value"},
		{name: "omitted"},
		{name: "literal", stdin: "café $() \n", want: "café $() \n"},
		{name: "command failure", stdin: "café", want: "café", exitCode: 2},
		{name: "parse failure", stdin: "{{", fail: true},
		{name: "render failure", stdin: `{{ (index .Inputs 9).Payload }}`, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			edge := &captureCommandRunner{result: workerprocess.CommandResult{ExitCode: tc.exitCode}}
			events := 0
			var recorded *workers.ScriptRequestEventPayload
			runner := New(Config{Command: "python", Stdin: tc.stdin}, edge, emptyDocs, func() time.Time { return time.Unix(0, 0) }, func(workers.ProgressFragment) {}, func(event workers.ScriptEvent) {
				events++
				if event.Request != nil {
					recorded = event.Request
				}
			})
			_, err := runner.Execute(t.Context(), validRequest())
			if tc.fail {
				if err == nil || events != 0 || edge.Request().Command != "" {
					t.Fatalf("template failure launched/recorded command: %v", err)
				}
				return
			}
			if (err != nil) != (tc.exitCode != 0) {
				t.Fatal(err)
			}
			if got := edge.Request(); string(got.Stdin) != tc.want || len(got.Args) != 0 {
				t.Fatalf("stdin/args=%q/%q, want %q/empty", got.Stdin, got.Args, tc.want)
			}
			stdin := edge.Request().Stdin
			if recorded == nil || recorded.StdinByteLength == nil || recorded.StdinSha256 == nil ||
				*recorded.StdinByteLength != int64(len(stdin)) || *recorded.StdinSha256 != fmt.Sprintf("%x", sha256.Sum256(stdin)) {
				t.Fatalf("request fingerprint = %#v, want exact runner stdin %q", recorded, stdin)
			}
		})
	}
}
