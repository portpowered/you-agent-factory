package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryinterfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const invokeContinuePackageFixtureTimeout = 15 * time.Second

// invokeContinuePackageFixture is the package-owned executable spine for the
// invoke/continue scenarios. The process and its route table are immutable
// after setup; scenario state is carried by explicit Factory Session IDs and
// the provider runner's recorded calls.
type invokeContinuePackageFixture struct {
	rootDir          string
	hostDir          string
	homeDir          string
	workingDirectory string
	baseURL          string
	process          support.ApplicationProcess
	command          *invokeContinuePackageCommand
	apiStopped       <-chan struct{}
	apiStarts        *atomic.Int32
	processBuilds    *atomic.Int32
	runner           *testutil.ProviderCommandRunner

	sessionsMu        sync.Mutex
	openedSessionIDs  []string
	deletedSessionIDs []string
}

type invokeContinuePackageCommand struct {
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
	err    error
}

// invokeContinuePackageFixtureState keeps lazy setup cheap for selectors that
// do not exercise CASE-01 while allowing TestMain to own final process cleanup.
var invokeContinuePackageFixtureState struct {
	sync.Mutex
	fixture *invokeContinuePackageFixture
}

func TestMain(m *testing.M) {
	exitCode := m.Run()
	if err := closeInvokeContinuePackageFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "invoke/continue package fixture cleanup failed: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

func ensureInvokeContinuePackageFixture(t *testing.T) *invokeContinuePackageFixture {
	t.Helper()

	invokeContinuePackageFixtureState.Lock()
	fixture := invokeContinuePackageFixtureState.fixture
	invokeContinuePackageFixtureState.Unlock()
	if fixture != nil {
		return fixture
	}

	created, err := newInvokeContinuePackageFixture(t)
	if err != nil {
		t.Fatalf("set up invoke/continue package fixture: %v", err)
	}

	invokeContinuePackageFixtureState.Lock()
	if invokeContinuePackageFixtureState.fixture == nil {
		invokeContinuePackageFixtureState.fixture = created
		fixture = created
	} else {
		fixture = invokeContinuePackageFixtureState.fixture
	}
	invokeContinuePackageFixtureState.Unlock()
	return fixture
}

func newInvokeContinuePackageFixture(t *testing.T) (*invokeContinuePackageFixture, error) {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "c11-invoke-continue-")
	if err != nil {
		return nil, fmt.Errorf("create package fixture root: %w", err)
	}
	keepRoot := false
	defer func() {
		if !keepRoot {
			_ = os.RemoveAll(rootDir)
		}
	}()

	hostDir := filepath.Join(rootDir, "host-factory")
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "routes", "local")
	if err := copyInvokeContinueDirectory(
		support.LegacyFixtureDir(t, "executor_success"),
		hostDir,
	); err != nil {
		return nil, fmt.Errorf("copy host Factory: %w", err)
	}
	// The copied fixture is used only to provide a valid Factory definition for
	// the explicit session. Removing its seed inputs prevents the hosted
	// default session from consuming the CASE-01 provider results at startup.
	if err := os.RemoveAll(filepath.Join(hostDir, factoryinterfaces.InputsDir)); err != nil {
		return nil, fmt.Errorf("clear host Factory seed inputs: %w", err)
	}
	for _, dir := range []string{homeDir, workingDirectory} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create package fixture directory %q: %w", dir, err)
		}
	}

	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("local-source-thread", "initial direct output COMPLETE")},
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("local-source-thread", "continued direct output COMPLETE")},
	)
	route := &invokeContinueStaticCommandRoute{
		workingDirectory: workingDirectory,
		runner:           runner,
	}
	api := support.NewProcessAPIServer()
	apiStopped := make(chan struct{})
	var apiStopOnce sync.Once
	apiStarts := &atomic.Int32{}
	processBuilds := &atomic.Int32{}

	processBuilds.Add(1)
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		// This route is complete before root construction and has no registration
		// or session-based fallback after the process starts.
		ProviderCommandRunner: route,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			apiStarts.Add(1)
			err := api.Start(ctx, request)
			apiStopOnce.Do(func() { close(apiStopped) })
			return err
		},
	})
	if err != nil {
		return nil, fmt.Errorf("BuildProcess: %w", err)
	}

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", hostDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = invokeContinueEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostDir
	command := startInvokeContinuePackageCommand(process, inputs)
	baseURL, err := api.WaitForBaseURL(invokeContinuePackageFixtureTimeout)
	if err != nil {
		_ = command.stop()
		closeCtx, cancel := context.WithTimeout(context.Background(), invokeContinuePackageFixtureTimeout)
		_ = process.Close(closeCtx)
		cancel()
		return nil, fmt.Errorf("wait for package fixture API: %w", err)
	}

	keepRoot = true
	return &invokeContinuePackageFixture{
		rootDir:          rootDir,
		hostDir:          hostDir,
		homeDir:          homeDir,
		workingDirectory: workingDirectory,
		baseURL:          baseURL,
		process:          process,
		command:          command,
		apiStopped:       apiStopped,
		apiStarts:        apiStarts,
		processBuilds:    processBuilds,
		runner:           runner,
	}, nil
}

func startInvokeContinuePackageCommand(
	process support.ApplicationProcess,
	inputs *support.CapturedInputs,
) *invokeContinuePackageCommand {
	parent := inputs.Input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input := inputs.Input
	input.Context = ctx
	command := &invokeContinuePackageCommand{cancel: cancel, done: make(chan error, 1)}
	go func() {
		command.done <- process.Execute(input)
	}()
	return command
}

func (command *invokeContinuePackageCommand) stop() error {
	if command == nil {
		return nil
	}
	command.once.Do(func() {
		command.cancel()
		select {
		case err := <-command.done:
			if err != nil && !errors.Is(err, context.Canceled) {
				command.err = fmt.Errorf("stop package fixture command: %w", err)
			}
		case <-time.After(invokeContinuePackageFixtureTimeout):
			command.err = errors.New("timed out stopping package fixture command")
		}
	})
	return command.err
}

func closeInvokeContinuePackageFixture() error {
	invokeContinuePackageFixtureState.Lock()
	fixture := invokeContinuePackageFixtureState.fixture
	invokeContinuePackageFixtureState.Unlock()
	if fixture == nil {
		return nil
	}

	var errs []error
	if err := fixture.command.stop(); err != nil {
		errs = append(errs, err)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), invokeContinuePackageFixtureTimeout)
	if err := fixture.process.Close(closeCtx); err != nil {
		errs = append(errs, fmt.Errorf("close package fixture process: %w", err))
	}
	cancel()
	select {
	case <-fixture.apiStopped:
	case <-time.After(invokeContinuePackageFixtureTimeout):
		errs = append(errs, errors.New("package fixture API server did not stop"))
	}

	if got := fixture.processBuilds.Load(); got != 1 {
		errs = append(errs, fmt.Errorf("package fixture process builds = %d, want exactly one", got))
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		errs = append(errs, fmt.Errorf("package fixture API starts = %d, want exactly one", got))
	}
	fixture.sessionsMu.Lock()
	if len(fixture.openedSessionIDs) != len(fixture.deletedSessionIDs) {
		errs = append(errs, fmt.Errorf(
			"package fixture Factory Sessions opened = %d, deleted = %d",
			len(fixture.openedSessionIDs), len(fixture.deletedSessionIDs),
		))
	}
	fixture.sessionsMu.Unlock()
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove package fixture root %q: %w", fixture.rootDir, err))
	} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("package fixture root %q remains after cleanup: %v", fixture.rootDir, err))
	}
	return errors.Join(errs...)
}

func (fixture *invokeContinuePackageFixture) openSession(t *testing.T) *invokeContinueFactorySession {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, fixture.hostDir)
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("CASE-01 Factory Session ID = %q, want explicit non-default session", sessionID)
	}
	fixture.sessionsMu.Lock()
	fixture.openedSessionIDs = append(fixture.openedSessionIDs, sessionID)
	fixture.sessionsMu.Unlock()
	session := &invokeContinueFactorySession{fixture: fixture, id: sessionID}
	t.Cleanup(func() {
		if session.closed {
			return
		}
		session.close(t)
	})
	return session
}

type invokeContinueFactorySession struct {
	fixture *invokeContinuePackageFixture
	id      string
	closed  bool
}

func (session *invokeContinueFactorySession) close(t testing.TB) {
	t.Helper()
	if session.closed {
		return
	}
	support.CloseFactorySessionAt(t, session.fixture.baseURL, session.id)
	session.closed = true
	session.fixture.sessionsMu.Lock()
	session.fixture.deletedSessionIDs = append(session.fixture.deletedSessionIDs, session.id)
	session.fixture.sessionsMu.Unlock()
}

func (session *invokeContinueFactorySession) assertDeleted(t *testing.T) {
	t.Helper()
	if !session.closed {
		t.Fatal("assertDeleted requires a closed Factory Session")
	}
	response, err := http.Get(strings.TrimSuffix(session.fixture.baseURL, "/") + "/factory-sessions/" + url.PathEscape(session.id))
	if err != nil {
		t.Fatalf("GET deleted Factory Session %q: %v", session.id, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted Factory Session %q status = %d, want 404", session.id, response.StatusCode)
	}
}

func (fixture *invokeContinuePackageFixture) assertSpine(t *testing.T) {
	t.Helper()
	if got := fixture.processBuilds.Load(); got != 1 {
		t.Fatalf("SPINE-001 process builds = %d, want exactly one", got)
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		t.Fatalf("SPINE-001 API starts = %d, want exactly one", got)
	}
	fixture.sessionsMu.Lock()
	defer fixture.sessionsMu.Unlock()
	if len(fixture.openedSessionIDs) != 1 || len(fixture.deletedSessionIDs) != 1 ||
		fixture.openedSessionIDs[0] != fixture.deletedSessionIDs[0] {
		t.Fatalf("SPINE-001 session lifecycle opened=%#v deleted=%#v, want one matching open/delete", fixture.openedSessionIDs, fixture.deletedSessionIDs)
	}
}

type invokeContinueStaticCommandRoute struct {
	workingDirectory string
	runner           *testutil.ProviderCommandRunner
}

// Run selects only the route fixed before process construction. It deliberately
// has no mutable map, request-order fallback, or Factory Session lookup.
func (route *invokeContinueStaticCommandRoute) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if route == nil || route.runner == nil {
		return platformprocess.CommandResult{}, errors.New("invoke/continue provider route is unavailable")
	}
	if filepath.Clean(request.WorkDir) != filepath.Clean(route.workingDirectory) {
		return platformprocess.CommandResult{}, fmt.Errorf("no invoke/continue provider route matched WorkDir %q", request.WorkDir)
	}
	return route.runner.Run(ctx, request)
}

var _ platformprocess.CommandRunner = (*invokeContinueStaticCommandRoute)(nil)

func invokeContinueEnvironment(homeDir string) []string {
	return append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func writeInvokeContinueExecution(
	t testing.TB,
	path string,
	sessionID string,
	workingDirectory string,
) {
	t.Helper()
	document := map[string]any{
		"requestId":       "local-invoke-request",
		"workerSessionId": "local-source-session",
		"execution": map[string]any{
			"workstationName":  "direct",
			"workingDirectory": workingDirectory,
			"factorySessionId": sessionID,
			"workerType":       "direct-worker",
			"runnerId":         "codex",
			"modelProvider":    "codex",
			"model":            "functional-model",
			"userMessage":      "initial direct prompt",
			"dispatch": map[string]any{
				"dispatchId":      "local-source-dispatch",
				"workstationName": "direct",
				"workerType":      "direct-worker",
			},
		},
	}
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("marshal explicit-session execution: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write explicit-session execution: %v", err)
	}
}

func copyInvokeContinueDirectory(sourceDir, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(targetDir, 0o755)
		}
		targetPath := filepath.Join(targetDir, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode().Perm())
	})
}
