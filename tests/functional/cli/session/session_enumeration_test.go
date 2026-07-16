package session

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/factory/packages/definitions/loop"
	"github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	cli "github.com/portpowered/infinite-you/pkg/transports/cli"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func BasicCliInputWithArgs(t *testing.T, args []string) root.Input {
	return root.Input{
		Args:    args,
		Env:     os.Environ(),
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Context: t.Context(),
	}
}

func TestNamedLoopCLI_RemainsScheduledAndDispatchesOncePerFakeClockBoundary(t *testing.T) {
	start := time.Date(2028, time.January, 1, 12, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	projectDir := t.TempDir()
	provider := testutil.NewMockProvider()
	support.SetWorkingDirectory(t, projectDir)
	dir, err := factoryconfig.PersistNamedFactory(filepath.Join(projectDir, "factory"), "@you/loop", builtinloop.BuiltInLoopFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(@you/loop): %v", err)
	}
	var captured *service.FactoryService
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		CaptureService:            func(svc *service.FactoryService) { captured = svc },
		Configure: func(cfg *service.FactoryServiceConfig) {
			cfg.RuntimeMode = interfaces.RuntimeModeService
			cfg.Clock = fakeClock
			cfg.Logger = zap.NewNop()
			cfg.ProviderOverride = provider
			cfg.OperatorDefaults = operatorconfig.ResolvedDefaults{
				WorkerModelProvider: "CURSOR",
				WorkerModel:         "loop-functional-model",
			}
		},
	})

	output := bytes.Buffer{}
	stderr := bytes.Buffer{}
	body, err := json.Marshal(factoryapi.InvocationRequest{Args: &map[string]any{
		"request": "Check the release dashboard", "period": "1h", "worktree": "loop-functional-worktree",
	}})
	if err != nil {
		t.Fatalf("marshal loop invocation: %v", err)
	}
	invocationCtx, cancelInvocation := context.WithCancel(t.Context())
	t.Cleanup(cancelInvocation)
	invocationDone := make(chan error, 1)
	go func() {
		request, err := http.NewRequestWithContext(invocationCtx, http.MethodPost, strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/~default/invocations", bytes.NewReader(body))
		if err != nil {
			invocationDone <- err
			return
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				err = fmt.Errorf("loop invocation status = %d", response.StatusCode)
			}
		}
		invocationDone <- err
	}()
	waitForLoopDispatchResponses(t, captured, 2)
	assertLoopProviderRequests(t, provider, 1)
	input := BasicCliInputWithArgs(t, []string{"you", "session", "list", "--server", server.URL()})
	input.Stdout = &output
	input.Stderr = &stderr
	if exitCode := root.Run(input, root.Dependencies{}); exitCode != 0 {
		t.Fatalf("you session list exit code = %d; stdout=%q stderr=%q", exitCode, output.String(), stderr.String())
	}
	assertLoopSessionRunning(t, captured)

	waitForLoopFakeClockWaiter(t, fakeClock)
	fakeClock.Advance(time.Hour - time.Second)
	assertLoopDispatchResponseCount(t, captured, 2)
	fakeClock.Advance(time.Second)
	waitForLoopDispatchResponses(t, captured, 4)
	assertLoopProviderRequests(t, provider, 2)

	waitForLoopFakeClockWaiter(t, fakeClock)
	fakeClock.Advance(time.Hour)
	waitForLoopDispatchResponses(t, captured, 6)
	assertLoopProviderRequests(t, provider, 3)
	assertLoopSessionRunning(t, captured)
	cancelInvocation()
	select {
	case <-invocationDone:
	case <-time.After(time.Second):
		t.Fatal("@you/loop invocation did not stop after cancellation")
	}
}

func TestNamedLoopCLI_ModelFlagsReachScheduledWorkerAcrossFakeClockBoundary(t *testing.T) {
	start := time.Date(2028, time.January, 1, 12, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	provider := testutil.NewMockProvider()
	support.SetWorkingDirectory(t, projectDir)
	if _, err := factoryconfig.PersistNamedFactory(filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories"), "@you/loop", builtinloop.BuiltInLoopFactoryJSON); err != nil {
		t.Fatalf("PersistNamedFactory(@you/loop): %v", err)
	}

	var captured *service.FactoryService
	runtimeReady := make(chan struct{})
	command := newLoopFlagCLICommand(t, homeDir, fakeClock, provider, &captured, runtimeReady)
	command.SetArgs([]string{
		"run",
		"--default-worker-model-provider", "cursor",
		"--default-worker-model", "loop-flag-model",
		"--named", "@you/loop",
		"--no-record",
		"Check the release dashboard",
		"--period", "1h",
		"--worktree", "loop-flag-worktree",
	})
	invocationCtx, cancelInvocation := context.WithCancel(t.Context())
	command.SetContext(invocationCtx)
	commandDone := make(chan error, 1)
	go func() { commandDone <- command.Execute() }()

	select {
	case <-runtimeReady:
	case err := <-commandDone:
		t.Fatalf("you run --named @you/loop ended before scheduled dispatch: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("you run --named @you/loop did not start its Factory Session runtime")
	}
	select {
	case err := <-commandDone:
		t.Fatalf("you run --named @you/loop ended before scheduled dispatch: %v", err)
	default:
	}
	waitForLoopDispatchResponses(t, captured, 2)
	assertFlaggedLoopProviderRequests(t, provider, 1)
	waitForLoopFakeClockWaiter(t, fakeClock)
	fakeClock.Advance(time.Hour - time.Second)
	assertLoopDispatchResponseCount(t, captured, 2)
	assertFlaggedLoopProviderRequests(t, provider, 1)
	fakeClock.Advance(time.Second)
	waitForLoopDispatchResponses(t, captured, 4)
	assertFlaggedLoopProviderRequests(t, provider, 2)
	assertLoopSessionRunning(t, captured)

	cancelInvocation()
	select {
	case err := <-commandDone:
		if err != nil && err != context.Canceled {
			t.Fatalf("you run --named @you/loop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("you run --named @you/loop did not stop after cancellation")
	}
}

func newLoopFlagCLICommand(
	t *testing.T,
	homeDir string,
	fakeClock *clockwork.FakeClock,
	provider *testutil.MockProvider,
	captured **service.FactoryService,
	runtimeReady chan<- struct{},
) *cobra.Command {
	t.Helper()
	return cli.NewRootCommandWithOptions(cli.RootCommandOptions{
		HomeDir: func() (string, error) { return homeDir, nil },
		RunFactory: func(ctx context.Context, cfg runcli.RunConfig) error {
			svc, err := wire.InjectFactoryService(ctx, &service.FactoryServiceConfig{
				Dir:              cfg.Dir,
				Port:             1,
				RuntimeMode:      interfaces.RuntimeModeService,
				Clock:            fakeClock,
				Logger:           zap.NewNop(),
				ProviderOverride: provider,
				OperatorDefaults: cfg.OperatorDefaults,
				APIServerStarter: func(ctx context.Context, _ apisurface.APISurface, _ int, _ *zap.Logger) error {
					<-ctx.Done()
					return nil
				},
			})
			if err != nil {
				return err
			}
			*captured = svc
			runDone := make(chan error, 1)
			go func() { runDone <- svc.Run(ctx) }()
			if err := waitForLoopRuntime(ctx, svc); err != nil {
				return err
			}
			close(runtimeReady)
			request := loopInvocationRequest(t, cfg)
			invokeErr := invokeLoopWhenSidecarsReady(ctx, svc, request)
			if invokeErr != nil && invokeErr != context.Canceled {
				return invokeErr
			}
			if runErr := <-runDone; runErr != nil && runErr != context.Canceled {
				return runErr
			}
			return nil
		},
	})
}

func invokeLoopWhenSidecarsReady(
	ctx context.Context,
	svc *service.FactoryService,
	request factoryapi.InvocationRequest,
) error {
	for {
		_, err := svc.InvokeFactorySession(ctx, factorysessions.DefaultSessionID, request)
		if err == nil || !strings.Contains(err.Error(), "Factory Session sidecars are unavailable") {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
}

func waitForLoopRuntime(ctx context.Context, svc *service.FactoryService) error {
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := svc.GetCurrentFactoryForSession(context.Background(), factorysessions.DefaultSessionID)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return fmt.Errorf("wait for CLI Factory Session runtime: %w", err)
		case <-ticker.C:
		}
	}
}

func loopInvocationRequest(t *testing.T, cfg runcli.RunConfig) factoryapi.InvocationRequest {
	t.Helper()
	if cfg.InvocationNormalizedArguments == nil {
		t.Fatal("CLI did not normalize @you/loop invocation arguments")
	}
	args := map[string]any{}
	for name, argument := range cfg.InvocationNormalizedArguments.Arguments {
		if len(argument.Values) != 1 {
			t.Fatalf("CLI argument %q values = %#v, want one value", name, argument.Values)
		}
		args[name] = argument.Values[0]
	}
	return factoryapi.InvocationRequest{Args: &args}
}

func assertFlaggedLoopProviderRequests(t *testing.T, provider *testutil.MockProvider, want int) {
	t.Helper()
	requests := provider.Calls()
	if len(requests) != want {
		t.Fatalf("flagged loop provider request count = %d, want %d", len(requests), want)
	}
	for index, request := range requests {
		if request.Worktree != "loop-flag-worktree" || request.ModelProvider != string(interfaces.ModelProviderCursor) || request.Model != "loop-flag-model" {
			t.Fatalf("flagged loop provider request %d = %#v, want CLI-selected model and configured worktree", index, request)
		}
		if request.Dispatch.WorkstationName != "run-loop-iteration" {
			t.Fatalf("flagged loop provider request %d dispatch = %#v, want iteration worker dispatch", index, request.Dispatch)
		}
		if !hasLoopRequestLineage(request.InputTokens, "Check the release dashboard") {
			t.Fatalf("flagged loop provider request %d inputs = %#v, want submitted request lineage", index, request.InputTokens)
		}
	}
}

func hasLoopRequestLineage(inputTokens []any, want string) bool {
	for _, inputToken := range inputTokens {
		token, ok := inputToken.(interfaces.Token)
		if !ok || token.Color.InvocationArguments == nil {
			continue
		}
		argument, ok := token.Color.InvocationArguments.Arguments["request"]
		if ok && len(argument.Values) == 1 && argument.Values[0] == want {
			return true
		}
	}
	return false
}

func assertLoopProviderRequests(t *testing.T, provider *testutil.MockProvider, want int) {
	t.Helper()
	requests := provider.Calls()
	if len(requests) != want {
		t.Fatalf("loop provider request count = %d, want %d", len(requests), want)
	}
	for index, request := range requests {
		if request.Worktree != "loop-functional-worktree" {
			t.Fatalf("loop provider request %d worktree = %q, want configured worktree; input tokens = %#v", index, request.Worktree, request.InputTokens)
		}
		if request.ModelProvider != string(interfaces.ModelProviderCursor) || request.Model != "loop-functional-model" {
			t.Fatalf("loop provider request %d model = %q/%q, want CURSOR/loop-functional-model", index, request.ModelProvider, request.Model)
		}
		if request.Dispatch.WorkstationName != "run-loop-iteration" {
			t.Fatalf("loop provider request %d dispatch = %#v, want iteration worker dispatch", index, request.Dispatch)
		}
	}
}

func assertLoopSessionRunning(t *testing.T, svc *service.FactoryService) {
	t.Helper()
	if svc == nil {
		t.Fatal("functional server did not capture FactoryService")
	}
	sessions, err := svc.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(sessions.Sessions) != 1 {
		t.Fatalf("Factory Session count = %d, want 1", len(sessions.Sessions))
	}
	if sessions.Sessions[0].Runtime == nil || sessions.Sessions[0].Runtime.Status == "TERMINATED" {
		t.Fatalf("loop Factory Session runtime = %#v, want active scheduled session", sessions.Sessions[0].Runtime)
	}
}

func waitForLoopFakeClockWaiter(t *testing.T, fakeClock *clockwork.FakeClock) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fakeClock.BlockUntilContext(ctx, 1); err != nil {
		t.Fatalf("wait for loop interval watcher: %v", err)
	}
}

func assertLoopDispatchResponseCount(t *testing.T, svc *service.FactoryService, want int) {
	t.Helper()
	events, err := svc.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	got := 0
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
			got++
		}
	}
	if got != want {
		t.Fatalf("loop dispatch response count = %d, want %d", got, want)
	}
}

func waitForLoopDispatchResponses(t *testing.T, svc *service.FactoryService, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		events, err := svc.GetFactoryEvents(context.Background())
		if err == nil && countLoopDispatchResponses(events) == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	assertLoopDispatchResponseCount(t, svc, want)
}

func countLoopDispatchResponses(events []factoryapi.FactoryEvent) int {
	count := 0
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
			count++
		}
	}
	return count
}

func basicServer(t *testing.T, dir string) *support.FunctionalAPIServer {
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		CaptureService: func(captured *service.FactoryService) {
		},
		Configure: func(cfg *service.FactoryServiceConfig) {
			cfg.RuntimeMode = interfaces.RuntimeModeService
			support.ConfigureWorkerCommands(t, cfg, support.NewStaticSuccessCommandRunner("primary result COMPLETE"), nil)
			cfg.Logger = zap.NewNop()
		},
	})
}

func TestSessionEnumeration(t *testing.T) {
	// Arrange
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	support.SetWorkingDirectory(t, dir)

	// Act

	// Instantiate the server
	server := basicServer(t, dir)

	// Enumerate the server configs
	output := bytes.Buffer{}
	stderr := bytes.Buffer{}
	fakeEnv := BasicCliInputWithArgs(t, []string{"you", "session", "list", "--server", server.URL()})
	fakeEnv.Stdout = &output
	fakeEnv.Stderr = &stderr
	exitCode := root.Run(fakeEnv, root.Dependencies{})

	// Assert

	if !bytes.Contains(output.Bytes(), []byte(dir)) {
		t.Errorf("expected output to contain copied fixture directory %q, got: %s", dir, output.String())
	}

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
}

func TestSessionEnumerationJson(t *testing.T) {
	// Arrange
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	support.SetWorkingDirectory(t, dir)

	// Act

	// Instantiate the server
	server := basicServer(t, dir)

	// Enumerate the server configs
	output := bytes.Buffer{}
	stderr := bytes.Buffer{}
	fakeEnv := BasicCliInputWithArgs(t, []string{"you", "session", "list", "--json", "--server", server.URL()})
	fakeEnv.Stdout = &output
	fakeEnv.Stderr = &stderr
	exitCode := root.Run(fakeEnv, root.Dependencies{})

	// Assert

	var session factoryapi.ListFactorySessionsResponse
	err := json.Unmarshal(output.Bytes(), &session)
	if err != nil {
		t.Fatalf("failed to unmarshal session output: %v", err)
	}
	if len(session.Sessions) != 1 {
		t.Fatalf("expected at least one session, got 0")
	}

	if (session.Sessions[0].Id == "") || (session.Sessions[0].Runtime == nil) {
		t.Fatalf("expected session to have id and runtime, got: %#v", session.Sessions[0])
	}

	if session.Sessions[0].FolderPath != dir {
		t.Fatalf("expected session folder path to be %q, got: %q", dir, session.Sessions[0].FolderPath)
	}

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
}
