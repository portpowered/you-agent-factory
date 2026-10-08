package customer_lifecycles_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedFactoryInvokedByCLICanBeInspectedByAPI proves a packaged @you/goal
// invocation started through the public you run CLI with a run-scoped server
// exposes compatible session, status, work, and invocation identity facts through
// the public Factory Session API while the invocation completes.
func testFactorypackagedcrossPackagedFactoryInvokedByCLICanBeInspectedByAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("slow packaged factory CLI/API cross-surface inspectability")
	}

	fixture := factorypackagedcrossSharedCrossProcess(t)
	homeDir, factoryDir := installSharedPackagedGoal(t)
	factoryPath := filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)
	goalText := fmt.Sprintf(
		"functional-packaged-cross-cli-api-inspect-%d",
		time.Now().UnixNano(),
	)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	requestedURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	server := support.NewProcessAPIServer()
	fixture.router.set(server)
	process := fixture.process
	shutdownGate := make(chan struct{})
	var releaseShutdown sync.Once
	releaseServerShutdown := func() {
		releaseShutdown.Do(func() { close(shutdownGate) })
	}
	server.HoldShutdownUntilSignaled(shutdownGate)
	t.Cleanup(releaseServerShutdown)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	inputs := support.FakeInputs(ctx, []string{
		"you", "--json", "run",
		"--factory", factoryPath,
		"--with-server",
		"--server", requestedURL,
		"--provider", "CODEX",
		"--model", packagedGoalProviderModel,
		"--no-record",
		goalText,
	})
	inputs.Input.WorkingDirectory = factoryDir
	inputs.Input.Env = isolatedHomeEnvironment(homeDir)

	execDone := make(chan error, 1)
	go func() {

		execDone <- process.Execute(inputs.Input)
	}()

	baseURL, err := server.WaitForBaseURL(crossPackagedServerReadyTimeout)
	if err != nil {
		t.Fatalf("wait for packaged cross inspect API server: %v", err)
	}
	eventCollector := startPackagedGoalFactoryEventCollector(ctx, t, baseURL)

	inspection, execErr := pollPackagedGoalAPIInspectionUntilCLICompletes(
		ctx,
		t,
		baseURL,
		execDone,
		releaseServerShutdown,
	)
	eventCollector.stop()
	events := eventCollector.snapshot()
	if retained, retainErr := tryGetRetainedFactoryEvents(baseURL); retainErr == nil {
		events = mergePackagedGoalFactoryEvents(events, retained)
	}
	if refreshed, refreshErr := fetchPackagedGoalAPIInspectionSnapshot(baseURL); refreshErr == nil {
		inspection = preferStrongerPackagedGoalAPIInspection(inspection, refreshed)
	}

	assertPackagedCLIInspectionOutcome(t, baseURL, events, execErr, homeDir, inputs, inspection)

}

// TestPackagedFactoryContinuousServerWithoutInvocationRemainsReachable proves
// a signature-bearing packaged Factory can enter continuous service mode with
// no invocation input, bind its API, and remain idle without creating Work.
// This scenario intentionally remains isolated because its material witness
// is a local-real process and TCP listener that must stay reachable while idle.
func testFactorypackagedcrossPackagedFactoryContinuousServerWithoutInvocationRemainsReachable(t *testing.T) {
	if testing.Short() {
		t.Skip("slow packaged Factory continuous-service readiness")
	}

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, factorydefinitions.PackagedGoalFactoryName)

	server := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {

			err := server.Start(ctx, request)

			return err
		},
	})
	support.CleanupProcess(t, process)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{
		"you", "run",
		"--named", factorydefinitions.PackagedGoalFactoryName,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = isolatedHomeEnvironment(homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	inputs.Input.Stdin = strings.NewReader("")
	stdinIsTTY := false
	inputs.Input.StdinIsTTY = &stdinIsTTY

	command := support.StartProcessCommand(t, process, inputs.Input)
	baseURL, err := server.WaitForBaseURL(crossPackagedServerReadyTimeout)
	if err != nil {
		t.Fatalf("wait for packaged cross idle API server: %v", err)
	}
	support.GetDefaultSession(t, baseURL)
	support.WaitForRuntimeIdle(t, baseURL, 10*time.Second)

	listed := support.ListDefaultSessionWork(t, baseURL)
	if len(listed.Results) != 0 {
		t.Fatalf("continuous service startup created Work without invocation input: %#v", listed.Results)
	}
	select {
	case <-command.Done():
		t.Fatalf("continuous service command exited before a later API request: %v", command.Err())
	default:
	}
	command.Stop(t)

	assertCrossListenerClosed(t, baseURL)
	removeCrossOwnedPath(t, "idle home", homeDir)

}

// TestPackagedFactoryCLIAndAPIPrimaryOutcomeShapesAgree proves packaged @you/goal
// CLI and API invocations agree on compatible public primary-outcome facts:
// terminal status, primary-result text on success, and stable public error codes
// on representative empty-input, source-conflict, and unresolved-primary-result
// failures across positional, stdin, and named-factory success paths.
func testFactorypackagedcrossPackagedFactoryCLIAndAPIPrimaryOutcomeShapesAgree(t *testing.T) {
	if testing.Short() {
		t.Skip("slow packaged factory CLI/API primary-outcome parity")
	}

	fixture := factorypackagedcrossSharedCrossProcess(t)
	// The named invocation uses the parent-owned installation and its own session.
	namedHomeDir, namedFactoryDir := fixture.homeDir, fixture.factoryDir
	apiServer := startPackagedGoalParityAPIServer(t, fixture.factoryDir)
	t.Cleanup(func() { assertPackagedGoalParityAPIServerHealthy(t, apiServer) })

	t.Run("positional success", func(t *testing.T) { runPackagedOutcomeParityCase1(t, apiServer) })

	t.Run("stdin success", func(t *testing.T) { runPackagedOutcomeParityCase2(t, apiServer) })

	t.Run("named factory success", func(t *testing.T) { runPackagedOutcomeParityCase3(t, apiServer, namedFactoryDir, namedHomeDir) })

	t.Run("empty input rejected", func(t *testing.T) { runPackagedOutcomeParityCase4(t, apiServer) })

	t.Run("source conflict rejected before invocation", runPackagedOutcomeParityCase5)

	t.Run("unresolved primary result reports stable failure", func(t *testing.T) { runPackagedOutcomeParityCase6(t, apiServer) })

}
func runPackagedOutcomeParityCase1(t *testing.T, apiServer *packagedGoalParityAPIServer) {
	t.Helper()

	t.Parallel()
	dir := scaffoldPackagedGoalInvocationFactory(t)
	factoryPath := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
	goalText := fmt.Sprintf(
		"functional-packaged-cross-cli-api-parity-positional-%d",
		time.Now().UnixNano(),
	)

	apiResponse, apiObservation := invokePackagedGoalViaAPI(t, apiServer, dir, goalText)
	cliResponse, _, stderr, err := runPackagedGoalInvocationCLIJSON(
		t,
		dir,
		factoryPath,
		isolatedHomeEnvironment(factorypackagedcrossSharedCrossProcess(t).homeDir),
		nil,
		goalText,
	)
	if err != nil {
		t.Fatalf("CLI positional invocation: %v\nstderr:\n%s", err, stderr)
	}
	assertPackagedGoalCLIAPIPrimaryOutcomeParity(
		t,
		apiResponse,
		cliResponse,
		wantPackagedGoalPrimaryResult,
	)
	assertPackagedGoalInvocationWorkAndEvents(t, apiResponse, apiObservation, "complete")

}
func runPackagedOutcomeParityCase2(t *testing.T, apiServer *packagedGoalParityAPIServer) {
	t.Helper()

	t.Parallel()
	dir := scaffoldPackagedGoalInvocationFactory(t)
	factoryPath := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
	goalText := fmt.Sprintf(
		"functional-packaged-cross-cli-api-parity-stdin-%d",
		time.Now().UnixNano(),
	)

	apiResponse, apiObservation := invokePackagedGoalViaAPI(t, apiServer, dir, goalText)
	cliResponse, _, stderr, err := runPackagedGoalInvocationCLIJSON(
		t,
		dir,
		factoryPath,
		isolatedHomeEnvironment(factorypackagedcrossSharedCrossProcess(t).homeDir),
		strings.NewReader(goalText),
	)
	if err != nil {
		t.Fatalf("CLI stdin invocation: %v\nstderr:\n%s", err, stderr)
	}
	assertPackagedGoalCLIAPIPrimaryOutcomeParity(
		t,
		apiResponse,
		cliResponse,
		wantPackagedGoalPrimaryResult,
	)
	assertPackagedGoalInvocationWorkAndEvents(t, apiResponse, apiObservation, "complete")

}
func runPackagedOutcomeParityCase3(t *testing.T, apiServer *packagedGoalParityAPIServer, namedFactoryDir string, namedHomeDir string) {
	t.Helper()

	t.Parallel()
	homeDir := namedHomeDir
	factoryDir := namedFactoryDir
	goalText := fmt.Sprintf(
		"functional-packaged-cross-cli-api-parity-named-%d",
		time.Now().UnixNano(),
	)

	apiResponse, apiObservation := invokePackagedGoalViaAPI(t, apiServer, factoryDir, goalText)
	cliResponse, _, stderr, err := runPackagedGoalNamedInvocationCLIJSON(
		t,
		homeDir,
		goalText,
	)
	if err != nil {
		t.Fatalf("CLI named invocation: %v\nstderr:\n%s", err, stderr)
	}
	assertPackagedGoalCLIAPIPrimaryOutcomeParity(
		t,
		apiResponse,
		cliResponse,
		wantPackagedGoalPrimaryResult,
	)
	assertPackagedGoalInvocationWorkAndEvents(t, apiResponse, apiObservation, "complete")

}
func runPackagedOutcomeParityCase4(t *testing.T, apiServer *packagedGoalParityAPIServer) {
	t.Helper()

	t.Parallel()
	dir := scaffoldPackagedGoalInvocationFactory(t)
	factoryPath := filepath.Join(dir, factorydefinitions.FactoryConfigFile)

	apiErr, apiObservation := postPackagedGoalInvocationExpectError(t, apiServer, dir, "   ")
	if string(apiErr.Code) != "INVOCATION_INPUT_EMPTY" {
		t.Fatalf("API error code = %q, want INVOCATION_INPUT_EMPTY", apiErr.Code)
	}
	assertPackagedGoalNoAdmission(t, apiObservation)

	_, _, stderr, err := runPackagedGoalInvocationCLIJSON(
		t,
		dir,
		factoryPath,
		isolatedHomeEnvironment(factorypackagedcrossSharedCrossProcess(t).homeDir),
		nil,
		"   ",
	)
	if err == nil {
		t.Fatal("expected CLI empty invocation input to fail")
	}
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_EMPTY") &&
		!strings.Contains(stderr, "INVOCATION_INPUT_EMPTY") {
		t.Fatalf("CLI failure = %v\nstderr = %q, want INVOCATION_INPUT_EMPTY", err, stderr)
	}

}
func runPackagedOutcomeParityCase5(t *testing.T) {
	t.Helper()

	t.Parallel()
	dir := scaffoldPackagedGoalInvocationFactory(t)
	factoryPath := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
	conflictMessage := "invocation input sources conflict: positional_text, stdin_text"

	_, stdout, stderr, err := runPackagedGoalInvocationCLI(
		t,
		dir,
		factoryPath,
		isolatedHomeEnvironment(factorypackagedcrossSharedCrossProcess(t).homeDir),
		strings.NewReader("from stdin"),
		"from positional",
	)
	if err == nil {
		t.Fatal("expected conflicting positional and stdin invocation inputs to fail")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on conflict failure", stdout)
	}
	if !strings.Contains(stderr, "INVOCATION_INPUT_SOURCE_CONFLICT") {
		t.Fatalf("stderr = %q, want stable conflict code", stderr)
	}
	if !strings.Contains(stderr, conflictMessage) {
		t.Fatalf("stderr = %q, want conflict detail %q", stderr, conflictMessage)
	}

}
func runPackagedOutcomeParityCase6(t *testing.T, apiServer *packagedGoalParityAPIServer) {
	t.Helper()

	t.Parallel()
	dir := scaffoldPackagedGoalInvocationFactoryWithUnresolvedPrimaryResult(t)
	factoryPath := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
	goalText := fmt.Sprintf(
		"functional-packaged-cross-cli-api-parity-unresolved-%d",
		time.Now().UnixNano(),
	)

	apiResponse, apiObservation := invokePackagedGoalViaAPI(t, apiServer, dir, goalText)
	if apiResponse.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("API status = %q, want FAILED", apiResponse.Status)
	}
	if apiResponse.ErrorCode == nil ||
		*apiResponse.ErrorCode != factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED {
		t.Fatalf(
			"API errorCode = %v, want INVOCATION_PRIMARY_RESULT_UNRESOLVED",
			errorCodeString(apiResponse.ErrorCode),
		)
	}
	if apiResponse.PrimaryResult != nil {
		t.Fatalf("API primaryResult = %#v, want nil on unresolved output", apiResponse.PrimaryResult)
	}
	assertPackagedGoalInvocationWorkAndEvents(t, apiResponse, apiObservation, "")

	cliResponse, _, stderr, err := runPackagedGoalInvocationCLIJSON(
		t,
		dir,
		factoryPath,
		isolatedHomeEnvironment(factorypackagedcrossSharedCrossProcess(t).homeDir),
		nil,
		goalText,
	)
	if err == nil {
		t.Fatal("expected CLI unresolved invocation primary result to fail")
	}
	if !strings.Contains(err.Error(), "INVOCATION_PRIMARY_RESULT_UNRESOLVED") &&
		!strings.Contains(stderr, "INVOCATION_PRIMARY_RESULT_UNRESOLVED") {
		t.Fatalf(
			"CLI failure = %v\nstderr = %q, want INVOCATION_PRIMARY_RESULT_UNRESOLVED",
			err,
			stderr,
		)
	}
	if cliResponse.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("CLI status = %q, want FAILED", cliResponse.Status)
	}
	if cliResponse.ErrorCode == nil ||
		*cliResponse.ErrorCode != factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED {
		t.Fatalf(
			"CLI errorCode = %v, want INVOCATION_PRIMARY_RESULT_UNRESOLVED",
			errorCodeString(cliResponse.ErrorCode),
		)
	}
	if cliResponse.PrimaryResult != nil {
		t.Fatalf("CLI primaryResult = %#v, want nil on unresolved output", cliResponse.PrimaryResult)
	}

}
func assertPackagedCLIInspectionOutcome(t *testing.T, baseURL string, events []factoryapi.FactoryEvent, execErr error, homeDir string, inputs *support.CapturedInputs, inspection packagedGoalAPIInspection) {
	t.Helper()
	if execErr != nil {
		t.Fatalf(
			"CLI packaged-factory invocation: %v\nstdout:\n%s\nstderr:\n%s",
			execErr,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	cliResponse := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if traceListed, traceErr := tryListDefaultSessionWorkByTrace(baseURL, cliResponse.TraceId); traceErr == nil {
		inspection = preferStrongerPackagedGoalAPIInspection(inspection, packagedGoalAPIInspection{
			session: inspection.session,
			status:  inspection.status,
			listed:  traceListed,
			live:    inspection.live,
		})
	}

	if !inspection.live {
		t.Fatalf("API inspection never observed a live session/status while CLI invocation ran")
	}

	assertPackagedGoalCLIInvocationInspectableByAPI(
		t,
		cliResponse,
		inspection,
		events,
		wantPackagedGoalPrimaryResult,
	)
	assertPackagedGoalInvocationWorkAndEvents(
		t,
		cliResponse,
		packagedGoalInvocationObservation{
			sessionID: factorysessions.DefaultSessionID,
			listed:    inspection.listed,
			events:    events,
		},
		"complete",
	)
	assertCrossListenerClosed(t, baseURL)
	removeCrossOwnedPath(t, "inspect home", homeDir)
}
