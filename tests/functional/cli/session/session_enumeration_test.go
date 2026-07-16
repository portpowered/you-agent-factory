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
	"github.com/portpowered/infinite-you/pkg/factory/packages/definitions/loop"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"

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
	support.SetWorkingDirectory(t, projectDir)
	dir, err := factoryconfig.PersistNamedFactory(filepath.Join(projectDir, "factory"), "@you/loop", builtinloop.BuiltInLoopFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(@you/loop): %v", err)
	}
	var captured *service.FactoryService
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		CaptureService:            func(svc *service.FactoryService) { captured = svc },
		Configure: func(cfg *service.FactoryServiceConfig) {
			cfg.RuntimeMode = interfaces.RuntimeModeService
			cfg.Clock = fakeClock
			cfg.Logger = zap.NewNop()
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

	waitForLoopFakeClockWaiter(t, fakeClock)
	fakeClock.Advance(time.Hour)
	waitForLoopDispatchResponses(t, captured, 6)
	assertLoopSessionRunning(t, captured)
	cancelInvocation()
	select {
	case <-invocationDone:
	case <-time.After(time.Second):
		t.Fatal("@you/loop invocation did not stop after cancellation")
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
