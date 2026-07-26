package script

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

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
	}, commandEdge, func(directory string) (map[string]string, error) {
		if directory != factoryDirectory {
			t.Fatalf("Factory docs directory = %q, want %q", directory, factoryDirectory)
		}
		return map[string]string{"guide.md": "factory guidance"}, nil
	})
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
		runner workers.CommandRunner
		docs   workers.FactoryDocsLoader
	}{
		{name: "command", runner: &captureCommandRunner{}, docs: emptyDocs},
		{name: "command runner", config: Config{Command: "echo"}, docs: emptyDocs},
		{name: "Factory docs loader", config: Config{Command: "echo"}, runner: &captureCommandRunner{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(test.config, test.runner, test.docs)
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
	scriptRunner, err := New(config, commandEdge, func(string) (map[string]string, error) {
		close(docsEntered)
		<-releaseDocs
		return map[string]string{}, nil
	})
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
		commandEdge,
		func(string) (map[string]string, error) {
			docsCalls++
			return nil, nil
		},
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
	scriptRunner, err := New(config, commandRunner, emptyDocs)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return scriptRunner
}

func emptyDocs(string) (map[string]string, error) {
	return map[string]string{}, nil
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
