package script

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRunnerResolvesConfiguredInvocationDeterministically(t *testing.T) {
	commandEdge := &captureCommandRunner{
		result: workers.CommandResult{Stdout: []byte("  completed  \n")},
	}
	factoryDirectory := filepath.Join("factory-root", "selected")
	scriptRunner, err := New(Config{
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
	}, testDependencies(commandEdge, func(directory string) (map[string]string, error) {
		if directory != factoryDirectory {
			t.Fatalf("Factory docs directory = %q, want %q", directory, factoryDirectory)
		}
		return map[string]string{"guide.md": "factory guidance"}, nil
	}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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
	assertEnv(t, captured.Env, "BASE", "injected")
	assertEnv(t, captured.Env, "RUNTIME", "request-env")
	assertEnvCount(t, captured.Env, "RUNTIME", 1)
	if captured.DispatchID != "dispatch-1" ||
		captured.TransitionID != "transition-1" ||
		captured.WorkerType != "request-worker" ||
		captured.WorkstationName != "request-workstation" ||
		captured.ProjectID != "request-project" ||
		captured.Execution.RequestID != "request-1" {
		t.Fatalf("command execution metadata = %#v", captured)
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
		result: workers.CommandResult{
			Stdout:   []byte("firstsecond\n"),
			Stderr:   []byte("warn-1warn-2"),
			ExitCode: 0,
		},
	}
	started := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	clock := &sequenceClock{times: []time.Time{started, started.Add(1500 * time.Millisecond)}}
	dependencies := Dependencies{
		CommandRunner: commandEdge,
		FactoryDocs:   emptyDocs,
		Now:           clock.Now,
		Publish: func(fragment workers.ProgressFragment) {
			observations.Append(fragment.Type + ":" + fragment.Payload)
			fragment.Metadata["stream"] = "mutated"
		},
		Record: func(event workers.ScriptEvent) {
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
	}
	scriptRunner, err := New(Config{Command: "echo", Args: []string{"safe-arg"}}, dependencies)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := validRequest()
	request.ProcessEnvironment = append(
		request.ProcessEnvironment,
		"CI=true",
		"SCRIPT_API_TOKEN=fixture-secret",
	)

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
	terminal := observations.Terminal()
	if terminal.Stdout != "firstsecond\n" || terminal.Stderr != "warn-1warn-2" {
		t.Fatalf("terminal output = stdout %q stderr %q", terminal.Stdout, terminal.Stderr)
	}
	if terminal.ExitCode == nil || *terminal.ExitCode != 0 ||
		terminal.Outcome != workers.ScriptExecutionOutcomeSucceeded ||
		terminal.DurationMillis != 1500 {
		t.Fatalf("terminal response = %#v", terminal)
	}
	if captured := commandEdge.Request(); !reflect.DeepEqual(captured.Args, []string{"safe-arg"}) {
		t.Fatalf("command args = %#v, want recorder mutation isolated", captured.Args)
	}
}

func TestRunnerSuccessResultsStayDetachedAcrossRepeatedAndConcurrentExecutions(t *testing.T) {
	commandEdge := &streamingCommandEdge{
		result: workers.CommandResult{Stdout: []byte("stable"), Stderr: []byte("note")},
	}
	scriptRunner, err := New(Config{Command: "echo", Args: []string{"one"}}, Dependencies{
		CommandRunner: commandEdge,
		FactoryDocs:   emptyDocs,
		Now:           func() time.Time { return time.Unix(100, 0) },
		Publish: func(fragment workers.ProgressFragment) {
			fragment.Metadata["stream"] = "mutated"
		},
		Record: func(event workers.ScriptEvent) {
			if event.Request != nil {
				event.Request.Args[0] = "mutated"
			}
			if event.Response != nil {
				event.Response.Stdout = "mutated"
			}
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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
		result          workers.CommandResult
		commandErr      error
		wantMessage     string
		wantOutcome     workers.ScriptExecutionOutcome
		wantFailureType *workers.ScriptFailureType
		wantExitCode    *int
	}{
		{
			name: "non-zero exit",
			result: workers.CommandResult{
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
			result: workers.CommandResult{
				Stdout: []byte("partial stdout"),
				Stderr: []byte("partial stderr"),
			},
			commandErr:      processFailure,
			wantMessage:     "script command execution failed",
			wantOutcome:     workers.ScriptExecutionOutcomeProcessError,
			wantFailureType: scriptFailureTypePointer(workers.ScriptFailureTypeProcessError),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCommandFailureCase(t, test.result, test.commandErr, test.wantMessage,
				test.wantOutcome, test.wantFailureType, test.wantExitCode, processFailure)
		})
	}
}

func runCommandFailureCase(
	t *testing.T,
	commandResult workers.CommandResult,
	commandErr error,
	wantMessage string,
	wantOutcome workers.ScriptExecutionOutcome,
	wantFailureType *workers.ScriptFailureType,
	wantExitCode *int,
	processFailure error,
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
	started := time.Date(2026, 7, 26, 21, 0, 0, 0, time.UTC)
	scriptRunner, err := New(Config{Command: "missing-tool"}, Dependencies{
		CommandRunner: commandEdge,
		FactoryDocs:   emptyDocs,
		Now:           (&sequenceClock{times: []time.Time{started, started.Add(2 * time.Second)}}).Now,
		Publish: func(fragment workers.ProgressFragment) {
			observations.Append(fragment.Type + ":" + fragment.Payload)
		},
		Record: func(event workers.ScriptEvent) {
			if event.Request != nil {
				observations.Append("request")
			}
			if event.Response != nil {
				observations.Append("terminal")
				observations.SetTerminal(*event.Response)
			}
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, executeErr := scriptRunner.Execute(t.Context(), validRequest())
	var failure *workers.ProviderError
	if !errors.As(executeErr, &failure) ||
		failure.Type != workers.WorkFailureTypeInternalServerError ||
		failure.Message != wantMessage {
		t.Fatalf("Execute() error = %#v, want normalized failure message %q", executeErr, wantMessage)
	}
	if commandErr != nil && !errors.Is(executeErr, processFailure) {
		t.Fatalf("Execute() error = %v, want process cause", executeErr)
	}
	assertFailureDiagnostics(t, result.Diagnostics, commandResult, 2*time.Second)
	assertFailureDiagnostics(t, failure.Diagnostics, commandResult, 2*time.Second)
	assertFailureObservation(t, observations, commandResult, wantOutcome, wantFailureType, wantExitCode)

	result.Diagnostics.Command.Stdout = "mutated"
	if failure.Diagnostics.Command.Stdout != string(commandResult.Stdout) {
		t.Fatalf("failure diagnostics changed through returned result mutation: %#v", failure.Diagnostics.Command)
	}
}

func assertFailureObservation(
	t *testing.T,
	observations *observationLog,
	result workers.CommandResult,
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
	if terminal.Outcome != wantOutcome ||
		terminal.DurationMillis != 2000 ||
		!reflect.DeepEqual(terminal.ExitCode, wantExitCode) ||
		!reflect.DeepEqual(terminal.FailureType, wantFailureType) ||
		terminal.Stdout != string(result.Stdout) ||
		terminal.Stderr != string(result.Stderr) {
		t.Fatalf("terminal response = %#v", terminal)
	}
}

func TestRunnerValidationFailureDoesNotRecordOrStartCommand(t *testing.T) {
	observations := &observationLog{}
	commandEdge := &streamingCommandEdge{observations: observations}
	scriptRunner, err := New(
		Config{Command: "echo", Args: []string{"{{"}},
		Dependencies{
			CommandRunner: commandEdge,
			FactoryDocs:   emptyDocs,
			Now:           func() time.Time { return time.Unix(0, 0) },
			Publish:       func(workers.ProgressFragment) { observations.Append("progress") },
			Record:        func(workers.ScriptEvent) { observations.Append("event") },
		},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, executeErr := scriptRunner.Execute(t.Context(), validRequest())
	assertFailureType(t, executeErr, workers.WorkFailureTypePermanentBadRequest)
	if got := observations.Values(); len(got) != 0 {
		t.Fatalf("observations = %#v, want none for validation rejection", got)
	}
}

func assertFailureDiagnostics(
	t *testing.T,
	diagnostics *workers.WorkDiagnostics,
	commandResult workers.CommandResult,
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
			scriptRunner := newTestRunner(t, test.config, commandEdge)
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
		})
	}
}

func TestNewRejectsMissingConfigurationAndEffects(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		mutate func(*Dependencies)
	}{
		{name: "command"},
		{name: "command runner", config: Config{Command: "echo"}, mutate: func(deps *Dependencies) {
			deps.CommandRunner = nil
		}},
		{name: "streaming command runner", config: Config{Command: "echo"}, mutate: func(deps *Dependencies) {
			deps.CommandRunner = nonStreamingCommandRunner{}
		}},
		{name: "Factory docs loader", config: Config{Command: "echo"}, mutate: func(deps *Dependencies) {
			deps.FactoryDocs = nil
		}},
		{name: "clock", config: Config{Command: "echo"}, mutate: func(deps *Dependencies) {
			deps.Now = nil
		}},
		{name: "progress publisher", config: Config{Command: "echo"}, mutate: func(deps *Dependencies) {
			deps.Publish = nil
		}},
		{name: "event recorder", config: Config{Command: "echo"}, mutate: func(deps *Dependencies) {
			deps.Record = nil
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dependencies := testDependencies(&captureCommandRunner{}, emptyDocs)
			if test.mutate != nil {
				test.mutate(&dependencies)
			}
			_, err := New(test.config, dependencies)
			assertFailureType(t, err, workers.WorkFailureTypeMisconfigured)
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
	scriptRunner, err := New(config, testDependencies(commandEdge, func(string) (map[string]string, error) {
		close(docsEntered)
		<-releaseDocs
		return map[string]string{}, nil
	}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
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
	if got := captured.InputTokens[0].(workers.Token).Color.Tags["lane"]; got != "fast" {
		t.Fatalf("captured input tag = %q, want fast", got)
	}
}

func TestRunnerPreservesPreCanceledContextWithoutCallingEffects(t *testing.T) {
	commandEdge := &captureCommandRunner{}
	docsCalls := 0
	scriptRunner, err := New(
		Config{Command: "echo"},
		testDependencies(commandEdge, func(string) (map[string]string, error) {
			docsCalls++
			return nil, nil
		}),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = scriptRunner.Execute(ctx, validRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	if commandEdge.Calls() != 0 || docsCalls != 0 {
		t.Fatalf("effects called after pre-cancellation: command=%d docs=%d", commandEdge.Calls(), docsCalls)
	}
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

func newTestRunner(
	t *testing.T,
	config Config,
	commandRunner workers.CommandRunner,
) workers.Runner {
	t.Helper()
	scriptRunner, err := New(config, testDependencies(commandRunner, emptyDocs))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return scriptRunner
}

func emptyDocs(string) (map[string]string, error) {
	return map[string]string{}, nil
}

func testDependencies(
	commandRunner workers.CommandRunner,
	factoryDocs workers.FactoryDocsLoader,
) Dependencies {
	return Dependencies{
		CommandRunner: commandRunner,
		FactoryDocs:   factoryDocs,
		Now:           func() time.Time { return time.Unix(0, 0) },
		Publish:       func(workers.ProgressFragment) {},
		Record:        func(workers.ScriptEvent) {},
	}
}

type outputChunk struct {
	stream  string
	payload string
}

type streamingCommandEdge struct {
	mu           sync.Mutex
	observations *observationLog
	request      workers.CommandRequest
	chunks       []outputChunk
	result       workers.CommandResult
	err          error
}

func (edge *streamingCommandEdge) Run(
	ctx context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	return edge.RunStreaming(ctx, request, nil)
}

func (edge *streamingCommandEdge) RunStreaming(
	_ context.Context,
	request workers.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	edge.mu.Lock()
	edge.request = workers.CloneSubprocessExecutionRequest(request)
	edge.mu.Unlock()
	if edge.observations != nil {
		edge.observations.Append("command")
	}
	for _, chunk := range edge.chunks {
		if observer != nil {
			observer(chunk.stream, []byte(chunk.payload))
		}
	}
	return workers.CommandResult{
		Stdout:   append([]byte(nil), edge.result.Stdout...),
		Stderr:   append([]byte(nil), edge.result.Stderr...),
		ExitCode: edge.result.ExitCode,
	}, edge.err
}

func (edge *streamingCommandEdge) Request() workers.CommandRequest {
	edge.mu.Lock()
	defer edge.mu.Unlock()
	return workers.CloneSubprocessExecutionRequest(edge.request)
}

type observationLog struct {
	mu       sync.Mutex
	values   []string
	terminal workers.ScriptResponseEventPayload
}

func (log *observationLog) Append(value string) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.values = append(log.values, value)
}

func (log *observationLog) Values() []string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]string(nil), log.values...)
}

func (log *observationLog) SetTerminal(terminal workers.ScriptResponseEventPayload) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.terminal = terminal
	if terminal.ExitCode != nil {
		exitCode := *terminal.ExitCode
		log.terminal.ExitCode = &exitCode
	}
}

func (log *observationLog) Terminal() workers.ScriptResponseEventPayload {
	log.mu.Lock()
	defer log.mu.Unlock()
	terminal := log.terminal
	if log.terminal.ExitCode != nil {
		exitCode := *log.terminal.ExitCode
		terminal.ExitCode = &exitCode
	}
	return terminal
}

type sequenceClock struct {
	mu    sync.Mutex
	times []time.Time
}

func (clock *sequenceClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if len(clock.times) == 0 {
		return time.Time{}
	}
	value := clock.times[0]
	clock.times = clock.times[1:]
	return value
}

type nonStreamingCommandRunner struct{}

func (nonStreamingCommandRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

type captureCommandRunner struct {
	mu      sync.Mutex
	request workers.CommandRequest
	result  workers.CommandResult
	calls   int
}

func (runner *captureCommandRunner) Run(
	_ context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	return runner.RunStreaming(context.Background(), request, nil)
}

func (runner *captureCommandRunner) RunStreaming(
	_ context.Context,
	request workers.CommandRequest,
	_ platformprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.calls++
	runner.request = workers.CloneSubprocessExecutionRequest(request)
	return runner.result, nil
}

func (runner *captureCommandRunner) Request() workers.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return workers.CloneSubprocessExecutionRequest(runner.request)
}

func (runner *captureCommandRunner) Calls() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func assertFailureType(t *testing.T, err error, want workers.WorkFailureType) {
	t.Helper()
	var failure *workers.ProviderError
	if !errors.As(err, &failure) || failure.Type != want {
		t.Fatalf("error = %#v, want ProviderError type %q", err, want)
	}
}

func assertEnv(t *testing.T, env []string, name, want string) {
	t.Helper()
	prefix := name + "="
	for _, entry := range env {
		if entry == prefix+want {
			return
		}
	}
	t.Fatalf("environment %s = %#v, want %q", name, env, want)
}

func assertEnvCount(t *testing.T, env []string, name string, want int) {
	t.Helper()
	prefix := name + "="
	count := 0
	for _, entry := range env {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			count++
		}
	}
	if count != want {
		t.Fatalf("environment %s count = %d, want %d in %#v", name, count, want, env)
	}
}
