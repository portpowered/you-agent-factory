package permission

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	permissionScenarioTimeout     = 20 * time.Second
	permissionProcessCloseTimeout = 5 * time.Second
)

type permissionProcessBuild struct {
	name    string
	process support.ApplicationProcess
	err     error
}

type permissionProcessServer struct {
	process   support.ApplicationProcess
	command   *support.ProcessCommand
	api       *support.ProcessAPIServer
	baseURL   string
	closeOnce sync.Once
	closeErr  error
}

// permissionGatedCommandRunner keeps the capable provider call in flight
// after the command edge has recorded its request. The incapable process is
// then exercised while that immutable capability graph is still active.
type permissionGatedCommandRunner struct {
	inner     *support.ShapedProviderCommandRunner
	started   chan struct{}
	release   <-chan struct{}
	startOnce sync.Once
}

func (runner *permissionGatedCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	result, err := runner.inner.Run(ctx, request)
	if err != nil {
		return result, err
	}
	runner.startOnce.Do(func() { close(runner.started) })
	select {
	case <-runner.release:
		return result, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func TestProviderPermissionBypassFunctionalContract(t *testing.T) {
	capableDir, incapableDir := permissionScenarioDirs(t)

	capableAPI := support.NewProcessAPIServer()
	incapableAPI := support.NewProcessAPIServer()
	capableRelease := make(chan struct{})
	capableRunner := &permissionGatedCommandRunner{
		inner:   support.NewShapedProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("permission bypass completed\nCOMPLETE")}),
		started: make(chan struct{}),
		release: capableRelease,
	}
	incapableRunner := support.NewShapedProviderCommandRunner()
	incapableOverrides := []providerswire.CatalogCapabilityOverride{{
		Provider:     providers.IDCodex,
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
	}}

	buildResults := make(chan permissionProcessBuild, 2)
	go buildPermissionProcess(buildResults, "capable", serviceedges.Edges{
		APIServerStarter:      capableAPI.Start,
		ProviderCommandRunner: capableRunner,
	})
	go buildPermissionProcess(buildResults, "incapable", serviceedges.Edges{
		APIServerStarter:                   incapableAPI.Start,
		ProviderCommandRunner:              incapableRunner,
		ProviderCatalogCapabilityOverrides: incapableOverrides,
	})

	var capableProcess, incapableProcess support.ApplicationProcess
	var buildErr error
	for range 2 {
		result := <-buildResults
		if result.err != nil && buildErr == nil {
			buildErr = result.err
		}
		if result.name == "capable" {
			capableProcess = result.process
		} else {
			incapableProcess = result.process
		}
	}
	if buildErr != nil {
		closePermissionProcess(capableProcess)
		closePermissionProcess(incapableProcess)
		t.Fatalf("build permission process graphs: %v", buildErr)
	}

	capableServer := startPermissionProcessServer(t, capableProcess, capableAPI, capableDir)
	incapableServer := startPermissionProcessServer(t, incapableProcess, incapableAPI, incapableDir)
	waitForPermissionProcessServer(t, capableServer)
	waitForPermissionProcessServer(t, incapableServer)

	// Cleanup owns both local roots even if a later assertion aborts the test.
	// The gate is likewise released on every exit so the capable command cannot
	// strand its process during failure cleanup.
	var releaseOnce sync.Once
	releaseCapable := func() { releaseOnce.Do(func() { close(capableRelease) }) }
	t.Cleanup(releaseCapable)

	capableName := "permission-capable"
	support.SubmitDefaultSessionWork(t, capableServer.baseURL, factoryapi.SubmitWorkRequest{
		Name:         &capableName,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "capable permission payload"},
	})
	waitForPermissionCommand(t, capableRunner.started)

	incapableName := "permission-incapable"
	support.SubmitDefaultSessionWork(t, incapableServer.baseURL, factoryapi.SubmitWorkRequest{
		Name:         &incapableName,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "incapable permission payload"},
	})
	support.WaitForTerminalStatus(t, incapableServer.baseURL, permissionScenarioTimeout)
	incapableListed := support.ListDefaultSessionWork(t, incapableServer.baseURL)
	incapableEvents := support.GetFactoryEventsAt(t, incapableServer.baseURL)

	releaseCapable()
	support.WaitForTerminalStatus(t, capableServer.baseURL, permissionScenarioTimeout)
	capableListed := support.ListDefaultSessionWork(t, capableServer.baseURL)
	capableRequests := capableRunner.inner.Requests()

	capableServer.close(t)
	incapableServer.close(t)

	t.Run("capable Codex route uses the command edge", func(t *testing.T) {
		assertCapablePermissionScenario(t, capableListed, capableRequests, capableDir)
	})

	// The capability override targets the real published Codex route while
	// leaving its built-in adapter and the command edge intact. It models a
	// route-specific authoritative capability view without registering an
	// in-process provider fake or selecting an unknown provider.
	t.Run("registered incapable Codex route fails before the command edge", func(t *testing.T) {
		assertIncapablePermissionScenario(t, incapableListed, incapableEvents, incapableRunner.Requests())
	})
}

func permissionScenarioDirs(t *testing.T) (capableDir, incapableDir string) {
	t.Helper()
	// Capability overrides are immutable construction-time provider wiring, so
	// the capable and incapable graphs remain separate. Build both roots in
	// parallel, start their independent local servers, and hold the capable
	// command edge while the incapable graph executes.
	capableDir = testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	incapableDir = testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, capableDir, "worker", permissionBypassWorkerConfig("codex"))
	support.WriteAgentConfig(t, incapableDir, "worker", permissionBypassWorkerConfig("codex"))
	return capableDir, incapableDir
}

func assertCapablePermissionScenario(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	requests []platformprocess.CommandRequest,
	dir string,
) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if len(requests) != 1 {
		t.Fatalf("provider command calls = %d, want one Codex execution", len(requests))
	}
	request := requests[0]
	if request.Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("provider command = %q, want %q", request.Command, modelprovider.ProviderCodex)
	}
	if !slices.Contains(request.Args, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("provider args = %#v, want Codex permission-bypass flag", request.Args)
	}
	if request.WorkDir != dir {
		t.Fatalf("capable provider WorkDir = %q, want %q", request.WorkDir, dir)
	}
}

func assertIncapablePermissionScenario(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
	requests []platformprocess.CommandRequest,
) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want one capability failure; listed=%#v", got, listed)
	}
	observations := support.ObserveDispatchEvents(t, events)
	if len(observations) != 1 || observations[0].Response == nil {
		t.Fatalf("dispatch observations = %#v, want one terminal response", observations)
	}
	response := observations[0].Response
	if response.FailureDetail == nil || !strings.Contains(response.FailureDetail.Message, `provider "codex" does not support capability "permission_bypass"`) {
		t.Fatalf("capability failure detail = %#v, want bounded provider capability diagnostic", response.FailureDetail)
	}
	if response.Error != nil && strings.Contains(*response.Error, "command") {
		t.Fatalf("capability failure error = %q, want no command detail", *response.Error)
	}
	if response.FailureDetail.Reason != factoryapi.WorkFailureTypePermanentBadRequest {
		t.Fatalf("capability failure reason = %q, want permanent bad request", response.FailureDetail.Reason)
	}
	if len(requests) != 0 {
		t.Fatalf("provider command calls = %d, want zero for incapable route", len(requests))
	}
}

func buildPermissionProcess(results chan<- permissionProcessBuild, name string, edges serviceedges.Edges) {
	process, err := support.BuildProcessWithContext(context.Background(), edges)
	results <- permissionProcessBuild{name: name, process: process, err: err}
}

func startPermissionProcessServer(
	t *testing.T,
	process support.ApplicationProcess,
	api *support.ProcessAPIServer,
	dir string,
) *permissionProcessServer {
	t.Helper()
	server := &permissionProcessServer{process: process, api: api}
	t.Cleanup(func() { server.close(t) })

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir
	server.command = support.StartProcessCommand(t, process, inputs.Input)
	return server
}

func waitForPermissionProcessServer(t *testing.T, server *permissionProcessServer) {
	t.Helper()
	server.baseURL = server.api.WaitForURL(t)
}

func waitForPermissionCommand(t *testing.T, started <-chan struct{}) {
	t.Helper()
	// The command edge emits this signal; the timer is only a bounded failure
	// guard and avoids polling or adding synchronization delay to the path.
	timer := time.NewTimer(permissionScenarioTimeout)
	defer timer.Stop()
	select {
	case <-started:
	case <-timer.C:
		t.Fatal("timed out waiting for capable permission command edge")
	}
}

func (server *permissionProcessServer) close(t testing.TB) {
	t.Helper()
	if server == nil {
		return
	}
	server.closeOnce.Do(func() {
		if server.command != nil {
			server.command.Stop(t)
		}
		ctx, cancel := context.WithTimeout(context.Background(), permissionProcessCloseTimeout)
		defer cancel()
		if server.process != nil {
			server.closeErr = server.process.Close(ctx)
		}
	})
	if server.closeErr != nil {
		t.Errorf("close permission process: %v", server.closeErr)
	}
}

func closePermissionProcess(process support.ApplicationProcess) {
	if process == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), permissionProcessCloseTimeout)
	defer cancel()
	_ = process.Close(ctx)
}

func permissionBypassWorkerConfig(provider string) string {
	return "---\n" +
		"type: MODEL_WORKER\n" +
		"model: test-model\n" +
		"modelProvider: " + provider + "\n" +
		"skipPermissions: true\n" +
		"stopToken: COMPLETE\n" +
		"---\n" +
		"Process the input task.\n"
}
