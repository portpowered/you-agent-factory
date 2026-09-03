package lifecycle_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	lifecycleAdverseSignalTimeout      = 5 * time.Second
	lifecycleAdverseCloseTimeout       = 5 * time.Second
	lifecycleObservationTimeoutForTest = 10 * time.Second
	lifecycleTimeoutPhaseBudget        = 250 * time.Millisecond
)

const lifecyclePartialCodexStdout = "" +
	`{"type":"turn.started"}` + "\n" +
	`{"type":"item.completed","item":{"id":"partial-codex-message","type":"agent_message","text":"partial provider output"}}` + "\n"

type hostedLifecycleInvocation struct {
	coordinator   *lifecycleCoordinator
	api           *support.ProcessAPIServer
	shutdownGate  *lifecycleGate
	command       *support.ProcessCommand
	inputs        *support.CapturedInputs
	baseURL       string
	sessionID     string
	listenerClose <-chan struct{}
}

type hostedCancelableLifecycleInvocation struct {
	coordinator   *lifecycleCoordinator
	api           *support.ProcessAPIServer
	shutdownGate  *lifecycleGate
	command       *lifecycleCancelableCommand
	inputs        *support.CapturedInputs
	baseURL       string
	sessionID     string
	listenerClose <-chan struct{}
}

type lifecycleWorkStatusObservation struct {
	work   factoryapi.ListWorkResponse
	status factoryapi.StatusResponse
}

// runLifecycleAdverseMatrix extends the existing six-selector package spine
// with deterministic adverse cases. Keeping these as subtests preserves the
// package's six top-level selector denominator used by the raised-parallelism
// gate.
func runLifecycleAdverseMatrix(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{name: "partial provider output", run: runLifecyclePartialProviderOutput},
		{name: "server attached provider failure", run: runLifecycleServerAttachedProviderFailure},
		{name: "cancellation and recovery", run: runLifecycleCancellationAndRecovery},
		{name: "observation timeout releases command", run: runLifecycleObservationTimeout},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.run(t)
		})
	}
}

func startHostedLifecycleInvocation(
	t *testing.T,
	factoryDir string,
	runner platformprocess.CommandRunner,
	prompt string,
) *hostedLifecycleInvocation {
	return startHostedLifecycleInvocationWithReuse(t, factoryDir, runner, prompt, false)
}

func startSharedHostedLifecycleInvocation(
	t *testing.T,
	factoryDir string,
	runner platformprocess.CommandRunner,
	prompt string,
) *hostedLifecycleInvocation {
	return startHostedLifecycleInvocationWithReuse(t, factoryDir, runner, prompt, true)
}

func startHostedLifecycleInvocationWithReuse(
	t *testing.T,
	factoryDir string,
	runner platformprocess.CommandRunner,
	prompt string,
	shared bool,
) *hostedLifecycleInvocation {
	t.Helper()
	api := newLifecycleAPIServer()
	shutdownGate := newLifecycleGate("adverse listener shutdown")
	api.HoldShutdownUntilSignaled(shutdownGate.channel())
	var coordinator *lifecycleCoordinator
	if shared {
		coordinator = buildSharedAdverseLifecycleProcess(t, runner, api.Start)
	} else {
		coordinator = buildLifecycleProcess(t, serviceedges.Edges{
			APIServerStarter:      api.Start,
			ProviderCommandRunner: runner,
		})
	}
	coordinator.TrackGate(shutdownGate)
	factoryPath := filepath.Join(factoryDir, "factory.json")
	inputs := coordinator.Inputs([]string{
		"you", "run",
		"--factory", factoryPath,
		"--with-server",
		"--no-record",
		"--quiet",
		prompt,
	}, factoryDir)
	command := coordinator.StartCommand(inputs)
	baseURL, err := coordinator.WaitForReadiness(api.server)
	if err != nil {
		t.Fatal(err)
	}
	return &hostedLifecycleInvocation{
		coordinator:   coordinator,
		api:           api.server,
		shutdownGate:  shutdownGate,
		command:       command,
		inputs:        inputs,
		baseURL:       baseURL,
		sessionID:     lifecycleSessionID(inputs.Input.Args),
		listenerClose: api.closed,
	}
}

func startHostedCancelableLifecycleInvocation(
	t *testing.T,
	factoryDir string,
	runner platformprocess.CommandRunner,
	prompt string,
) *hostedCancelableLifecycleInvocation {
	t.Helper()
	api := newLifecycleAPIServer()
	shutdownGate := newLifecycleGate("cancelable listener shutdown")
	api.HoldShutdownUntilSignaled(shutdownGate.channel())
	coordinator := buildLifecycleProcess(t, serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: runner,
	})
	coordinator.TrackGate(shutdownGate)
	factoryPath := filepath.Join(factoryDir, "factory.json")
	inputs := coordinator.Inputs([]string{
		"you", "run",
		"--factory", factoryPath,
		"--with-server",
		"--no-record",
		"--quiet",
		prompt,
	}, factoryDir)
	command := coordinator.StartCancelableCommand(inputs)
	baseURL, err := coordinator.WaitForReadiness(api.server)
	if err != nil {
		t.Fatal(err)
	}
	return &hostedCancelableLifecycleInvocation{
		coordinator:   coordinator,
		api:           api.server,
		shutdownGate:  shutdownGate,
		command:       command,
		inputs:        inputs,
		baseURL:       baseURL,
		sessionID:     lifecycleSessionID(inputs.Input.Args),
		listenerClose: api.closed,
	}
}

func runLifecyclePartialProviderOutput(t *testing.T) {
	factoryDir := scaffoldProviderBackedFactory(t)
	runner := &lifecycleResultRunner{result: platformprocess.CommandResult{
		Stdout: []byte(lifecyclePartialCodexStdout),
	}}
	invocation := startSharedHostedLifecycleInvocation(
		t,
		factoryDir,
		runner,
		"prove partial provider output cannot fabricate completion",
	)
	workID, workerSessionID := assertHostedAdverseProjection(
		t,
		invocation.baseURL,
		invocation.sessionID,
		[]factoryapi.WorkOutcome{factoryapi.WorkOutcomeFailed, factoryapi.WorkOutcomeRejected},
		[]factoryapi.WorkerSessionObservationState{factoryapi.WorkerSessionObservationStateFailed},
	)
	if runner.CallCount() != 1 {
		t.Fatalf("partial provider command calls = %d, want one", runner.CallCount())
	}
	if strings.Contains(invocation.inputs.Stdout(), "partial provider output") {
		t.Fatalf("partial provider stdout = %q, want no success result", invocation.inputs.Stdout())
	}
	invocation.command.AcceptError()
	invocation.coordinator.ReleaseGate(
		invocation.shutdownGate,
		lifecyclePhaseTerminal,
		"partial failure public projections",
	)
	err := invocation.coordinator.WaitCommand(invocation.command)
	if err == nil {
		t.Fatal("partial provider Process.Execute error = nil, want terminal failure")
	}
	assertLifecycleFailureOutput(t, invocation.inputs, err)
	finishHostedLifecycleInvocation(t, invocation.coordinator, invocation.baseURL, invocation.listenerClose, workID, workerSessionID)
}

func runLifecycleServerAttachedProviderFailure(t *testing.T) {
	factoryDir := scaffoldProviderBackedFactory(t)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: deterministicProviderFailureExit,
		Stderr:   []byte(deterministicProviderFailureStderr),
	})
	invocation := startSharedHostedLifecycleInvocation(
		t,
		factoryDir,
		runner,
		"prove server-attached provider failure remains correlated",
	)
	workID, workerSessionID := assertHostedAdverseProjection(
		t,
		invocation.baseURL,
		invocation.sessionID,
		[]factoryapi.WorkOutcome{factoryapi.WorkOutcomeFailed},
		[]factoryapi.WorkerSessionObservationState{factoryapi.WorkerSessionObservationStateFailed},
	)
	if runner.CallCount() != 1 {
		t.Fatalf("server-attached provider command calls = %d, want one", runner.CallCount())
	}
	invocation.command.AcceptError()
	invocation.coordinator.ReleaseGate(
		invocation.shutdownGate,
		lifecyclePhaseTerminal,
		"server-attached failure public projections",
	)
	err := invocation.coordinator.WaitCommand(invocation.command)
	if err == nil {
		t.Fatal("server-attached failure Process.Execute error = nil, want terminal failure")
	}
	assertLifecycleFailureOutput(t, invocation.inputs, err)
	finishHostedLifecycleInvocation(t, invocation.coordinator, invocation.baseURL, invocation.listenerClose, workID, workerSessionID)
}

func runLifecycleCancellationAndRecovery(t *testing.T) {
	factoryDir := scaffoldProviderBackedFactory(t)
	runner := newBlockingLifecycleRunner()
	invocation := startHostedCancelableLifecycleInvocation(
		t,
		factoryDir,
		runner,
		"hold active dispatch until the lifecycle caller cancels",
	)
	waitLifecycleSignal(t, runner.started, "blocking provider start")
	activeWork, _ := waitForLifecycleWorkState(t, invocation.baseURL, invocation.sessionID, func(work factoryapi.Work) bool {
		return work.State != nil && work.State.Type == factoryapi.WorkStateTypePROCESSING
	})
	if activeWork.WorkId == nil || strings.TrimSpace(*activeWork.WorkId) == "" {
		t.Fatal("cancellation active Work has no identity")
	}

	activeWorker := waitForLifecycleWorkerSession(t, invocation.baseURL, invocation.sessionID, *activeWork.WorkId, func(worker factoryapi.WorkerSessionObservation) bool {
		return worker.State == factoryapi.WorkerSessionObservationStateRunning
	})
	cancelResult := postLifecycleFactorySessionControl(t, invocation.baseURL, invocation.sessionID, "cancel", "lifecycle-cancel-control")
	if cancelResult.SessionId != invocation.sessionID ||
		cancelResult.Operation != factoryapi.FactorySessionLifecycleControlKindCancel ||
		(cancelResult.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted &&
			cancelResult.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp) {
		t.Fatalf("cancel Factory Session result = %#v, want accepted or idempotent no-op cancellation", cancelResult)
	}
	waitLifecycleSignal(t, runner.finished, "blocking provider cancellation")
	if runner.CancellationCount() != 1 {
		t.Fatalf("blocking provider cancellation count = %d, want one", runner.CancellationCount())
	}
	workID, workerSessionID := assertHostedCancellationStop(
		t,
		invocation.baseURL,
		invocation.sessionID,
		*activeWork.WorkId,
	)
	if invocation.inputs.Stdout() != "" {
		t.Fatalf("canceled stdout = %q, want no success result", invocation.inputs.Stdout())
	}
	select {
	case <-invocation.command.Done():
		t.Fatal("canceled command completed before its listener shutdown gate was released")
	default:
	}
	// The Worker Session identity was captured before the Factory Session
	// control and the stop observation retained that exact association. No
	// second control is sent because the live session control owns the runtime
	// stop and may retire the session registry entry after the first join.
	if strings.TrimSpace(activeWorker.WorkerSessionId) == "" {
		t.Fatal("active Worker Session identity disappeared before cancellation projection")
	}
	invocation.coordinator.ReleaseGate(
		invocation.shutdownGate,
		lifecyclePhaseTerminal,
		"canceled public projections",
	)
	// Canceling the Factory Session and stopping the server-owning CLI command
	// are separate customer controls. The session control above proves the
	// provider and public projections stop; this context cancellation then ends
	// the invocation-owned --with-server command before starting recovery.
	invocation.command.Cancel()
	err := waitCancelableLifecycleCommand(invocation.command)
	if err == nil {
		t.Fatal("canceled Process.Execute error = nil, want cancellation-compatible error")
	}
	if strings.Contains(err.Error(), "completion deadline expired") {
		t.Fatalf("canceled Process.Execute did not join after command cancellation: %v", err)
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "INVOCATION_CANCELED") {
		t.Fatalf("canceled Process.Execute error = %v, want cancellation-compatible diagnostic", err)
	}
	finishCancelableHostedLifecycleInvocation(t, invocation.coordinator, invocation.baseURL, invocation.listenerClose, workID, workerSessionID)

	recoveryWorkID, recoveryWorkerSessionID := runHostedLifecycleRecovery(t, factoryDir)
	if recoveryWorkID == workID {
		t.Fatalf("recovery Work ID = %q, want a distinct identity from canceled Work", recoveryWorkID)
	}
	if recoveryWorkerSessionID == workerSessionID {
		t.Fatalf("recovery Worker Session ID = %q, want a distinct identity from canceled Worker Session", recoveryWorkerSessionID)
	}
}

func runLifecycleObservationTimeout(t *testing.T) {
	factoryDir := scaffoldProviderBackedFactory(t)
	runner := newBlockingLifecycleRunner()
	api := newLifecycleAPIServer()
	shutdownGate := newLifecycleGate("observation-timeout listener shutdown")
	api.HoldShutdownUntilSignaled(shutdownGate.channel())
	coordinator := buildLifecycleProcess(t, serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: runner,
	})
	coordinator.TrackGate(shutdownGate)
	inputs := coordinator.Inputs([]string{
		"you", "run",
		"--factory", filepath.Join(factoryDir, "factory.json"),
		"--with-server",
		"--no-record",
		"--quiet",
		"keep the observation target absent until the local deadline fires",
	}, factoryDir)
	command := coordinator.StartCancelableCommand(inputs)
	baseURL, err := coordinator.WaitForReadiness(api.server)
	if err != nil {
		t.Fatal(err)
	}
	waitLifecycleSignal(t, runner.started, "observation-timeout provider start")
	_, _, _, err = coordinator.ObserveHostedServerAttachedWithin(
		baseURL,
		lifecycleSessionID(inputs.Input.Args),
		"this terminal output is intentionally absent",
		nil,
		command.Done(),
		lifecycleTimeoutPhaseBudget,
	)
	if err == nil || !strings.Contains(err.Error(), "phase terminal") {
		t.Fatalf("observation timeout error = %v, want terminal phase diagnostic", err)
	}
	if unreleased := coordinator.unreleasedGates(); len(unreleased) != 0 {
		t.Fatalf("observation timeout unreleased gates = %v, want none", unreleased)
	}
	command.Cancel()
	if err := waitCancelableLifecycleCommand(command); err == nil {
		t.Fatal("observation-timeout Process.Execute error = nil, want cancellation-compatible error")
	}
	if inputs.Stdout() != "" {
		t.Fatalf("observation-timeout stdout = %q, want no success result", inputs.Stdout())
	}
	closeLifecycleCoordinator(t, coordinator)
	if !lifecycleListenerClosed(baseURL, api.closed) {
		t.Fatalf("observation-timeout listener at %s remained reachable after cancellation and close", baseURL)
	}
}

func assertHostedAdverseProjection(
	t *testing.T,
	baseURL, sessionID string,
	wantedOutcomes []factoryapi.WorkOutcome,
	wantedWorkerStates []factoryapi.WorkerSessionObservationState,
) (string, string) {
	t.Helper()
	work, status := waitForLifecycleWorkState(t, baseURL, sessionID, func(work factoryapi.Work) bool {
		return work.State != nil && (work.State.Type == factoryapi.WorkStateTypeFAILED || work.State.Type == factoryapi.WorkStateTypeTERMINAL)
	})
	if work.State == nil || work.State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("adverse Work state = %#v, want FAILED and no fabricated success", work.State)
	}
	if status.Categories.Processing != 0 {
		t.Fatalf("adverse status = %#v, want no active processing Work", status)
	}
	if work.WorkId == nil || strings.TrimSpace(*work.WorkId) == "" {
		t.Fatalf("adverse Work = %#v, want non-empty identity", work)
	}
	workID := *work.WorkId
	workerSessions := support.ListSessionWorkerSessions(t, baseURL, sessionID, workID)
	if len(workerSessions.Sessions) != 1 {
		t.Fatalf("adverse Worker Sessions for Work %q = %#v, want exactly one", workID, workerSessions.Sessions)
	}
	worker := workerSessions.Sessions[0]
	if strings.TrimSpace(worker.WorkerSessionId) == "" {
		t.Fatalf("adverse Worker Session = %#v, want non-empty identity", worker)
	}
	if !containsWorkerSessionState(wantedWorkerStates, worker.State) {
		t.Fatalf("adverse Worker Session %q state = %q, want one of %v", worker.WorkerSessionId, worker.State, wantedWorkerStates)
	}
	if worker.WorkId == nil || *worker.WorkId != workID || !containsString(worker.WorkIds, workID) {
		t.Fatalf("adverse Worker Session %q Work correlation = workId:%#v workIds:%#v, want %q", worker.WorkerSessionId, worker.WorkId, worker.WorkIds, workID)
	}
	// The replay endpoint's completion summary proves the response stream and
	// its HTTP body were closed after the terminal Worker Session observation.
	if events := support.GetWorkerSessionEventsForSessionByIDAt(t, baseURL, sessionID, worker.WorkerSessionId); len(events) == 0 {
		t.Fatalf("adverse Worker Session %q replay = empty, want terminal history", worker.WorkerSessionId)
	}

	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	dispatches := support.ObserveDispatchEvents(t, events)
	matching := make([]support.DispatchEventObservation, 0, 1)
	for _, dispatch := range dispatches {
		if support.DispatchObservationIncludesWork(dispatch, workID) {
			matching = append(matching, dispatch)
		}
	}
	if len(matching) != 1 || matching[0].Response == nil {
		t.Fatalf("adverse dispatches for Work %q = %#v, want exactly one terminal response", workID, matching)
	}
	dispatch := matching[0]
	if !containsWorkOutcome(wantedOutcomes, dispatch.Response.Outcome) {
		t.Fatalf("adverse dispatch %q outcome = %q, want one of %v", dispatch.DispatchID, dispatch.Response.Outcome, wantedOutcomes)
	}
	if dispatch.Response.Outcome == factoryapi.WorkOutcomeAccepted {
		t.Fatalf("adverse dispatch %q fabricated ACCEPTED outcome", dispatch.DispatchID)
	}
	if dispatch.StartedAt.IsZero() || dispatch.CompletedAt.IsZero() || dispatch.StartedAt.After(dispatch.CompletedAt) {
		t.Fatalf("adverse dispatch %q times = %s -> %s, want ordered terminal times", dispatch.DispatchID, dispatch.StartedAt, dispatch.CompletedAt)
	}
	if dispatch.Response.Outcome == factoryapi.WorkOutcomeFailed &&
		(dispatch.Response.Error == nil || strings.TrimSpace(*dispatch.Response.Error) == "") {
		t.Fatalf("adverse failed dispatch %q error = %#v, want failure diagnostic", dispatch.DispatchID, dispatch.Response.Error)
	}
	return workID, worker.WorkerSessionId
}

func assertHostedCancellationStop(t *testing.T, baseURL, sessionID, workID string) (string, string) {
	t.Helper()
	workerSessions, err := support.WaitForObservation(
		lifecycleObservationTimeoutForTest,
		func() (factoryapi.ListWorkerSessionsResponse, error) {
			return readLifecycleHTTPJSON[factoryapi.ListWorkerSessionsResponse](
				strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/worker-sessions?workId=" + url.QueryEscape(workID),
			)
		},
		func(observation factoryapi.ListWorkerSessionsResponse) bool {
			return len(observation.Sessions) == 1 &&
				strings.TrimSpace(observation.Sessions[0].WorkerSessionId) != ""
		},
	)
	if err != nil {
		t.Fatalf("wait for canceled Worker Session observation: %v", err)
	}
	worker := workerSessions.Sessions[0]
	if worker.WorkId == nil || *worker.WorkId != workID || !containsString(worker.WorkIds, workID) {
		t.Fatalf("canceled Worker Session %q Work correlation = workId:%#v workIds:%#v, want %q", worker.WorkerSessionId, worker.WorkId, worker.WorkIds, workID)
	}
	work, status := waitForLifecycleWorkState(t, baseURL, sessionID, func(candidate factoryapi.Work) bool {
		return candidate.WorkId != nil && *candidate.WorkId == workID
	})
	if work.State == nil || work.State.Type == factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("canceled Work %q state = %#v, want no fabricated successful terminal state", workID, work.State)
	}
	if status.Categories.Terminal != 0 {
		t.Fatalf("canceled status = %#v, want no successful terminal Work", status)
	}
	session, err := support.WaitForObservation(
		lifecycleObservationTimeoutForTest,
		func() (factoryapi.FactorySession, error) {
			observed, ok, diagnostic := tryReadFactorySession(baseURL, sessionID)
			if !ok {
				return factoryapi.FactorySession{}, errors.New(diagnostic)
			}
			return observed, nil
		},
		func(observation factoryapi.FactorySession) bool {
			return observation.Runtime.Status == factoryapi.FactorySessionStatusFINISHED ||
				observation.Runtime.Status == factoryapi.FactorySessionStatusIDLE
		},
	)
	if err != nil {
		t.Fatalf("wait for canceled Factory Session terminal status: %v", err)
	}
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	dispatches := support.ObserveDispatchEvents(t, events)
	matching := make([]support.DispatchEventObservation, 0, 1)
	for _, dispatch := range dispatches {
		if support.DispatchObservationIncludesWork(dispatch, workID) {
			matching = append(matching, dispatch)
		}
	}
	if len(matching) != 1 {
		t.Fatalf("canceled dispatches for Work %q = %#v, want exactly one request observation", workID, matching)
	}
	if matching[0].Response != nil && matching[0].Response.Outcome == factoryapi.WorkOutcomeAccepted {
		t.Fatalf("canceled dispatch %q fabricated ACCEPTED outcome", matching[0].DispatchID)
	}
	t.Logf("canceled public stop: Factory Session status=%q, Work state=%q, Worker Session %q state=%q failure=%#v, status=%#v, dispatch response=%#v", session.Runtime.Status, work.State.Type, worker.WorkerSessionId, worker.State, worker.Failure, status, matching[0].Response)
	return workID, worker.WorkerSessionId
}

func waitForLifecycleWorkerSession(
	t *testing.T,
	baseURL, sessionID, workID string,
	accept func(factoryapi.WorkerSessionObservation) bool,
) factoryapi.WorkerSessionObservation {
	t.Helper()
	observation, err := support.WaitForObservation(
		lifecycleObservationTimeoutForTest,
		func() (factoryapi.ListWorkerSessionsResponse, error) {
			return readLifecycleHTTPJSON[factoryapi.ListWorkerSessionsResponse](
				strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/worker-sessions?workId=" + url.QueryEscape(workID),
			)
		},
		func(observation factoryapi.ListWorkerSessionsResponse) bool {
			return len(observation.Sessions) == 1 && accept != nil && accept(observation.Sessions[0])
		},
	)
	if err != nil {
		t.Fatalf("wait for Worker Session for Work %q: %v", workID, err)
	}
	return observation.Sessions[0]
}

func postLifecycleFactorySessionControl(
	t *testing.T,
	baseURL, sessionID, action, requestID string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	control := factoryapi.FactorySessionLifecycleControlRequest{}
	if strings.TrimSpace(requestID) != "" {
		control.RequestId = &requestID
	}
	body, err := json.Marshal(control)
	if err != nil {
		t.Fatalf("marshal Factory Session %s request: %v", action, err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/" + action
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("construct Factory Session %s request: %v", action, err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := lifecycleHTTPClient.Do(request)
	if err != nil {
		t.Fatalf("POST Factory Session %s: %v", action, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST Factory Session %s status = %d, body = %s", action, response.StatusCode, strings.TrimSpace(string(payload)))
	}
	var result factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode Factory Session %s result: %v", action, err)
	}
	return result
}

func runHostedLifecycleRecovery(t *testing.T, factoryDir string) (string, string) {
	t.Helper()
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("recovery invocation COMPLETE"),
	})
	invocation := startHostedLifecycleInvocation(
		t,
		factoryDir,
		runner,
		"recover the canceled lifecycle with a fresh invocation",
	)
	workID, _, diagnostic := waitForLifecycleTerminalWorkText(
		invocation.baseURL,
		invocation.sessionID,
		"recovery invocation COMPLETE",
	)
	if diagnostic != "" {
		t.Fatal(diagnostic)
	}
	workerSessions := support.ListSessionWorkerSessions(t, invocation.baseURL, invocation.sessionID, workID)
	if len(workerSessions.Sessions) != 1 {
		t.Fatalf("recovery Worker Sessions for Work %q = %#v, want one", workID, workerSessions.Sessions)
	}
	worker := workerSessions.Sessions[0]
	if worker.State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("recovery Worker Session %q state = %q, want COMPLETED", worker.WorkerSessionId, worker.State)
	}
	events := support.GetFactoryEventsForSessionAt(t, invocation.baseURL, invocation.sessionID)
	if dispatchID := assertHostedServerAttachedFactoryEvents(
		t,
		events,
		support.ObserveDispatchEvents(t, events),
		invocation.sessionID,
		workID,
		worker.WorkerSessionId,
	); strings.TrimSpace(dispatchID) == "" {
		t.Fatal("recovery dispatch identity is empty")
	}
	if runner.CallCount() != 1 {
		t.Fatalf("recovery provider command calls = %d, want one", runner.CallCount())
	}
	invocation.coordinator.ReleaseGate(
		invocation.shutdownGate,
		lifecyclePhaseTerminal,
		"recovery public projections",
	)
	if err := invocation.coordinator.WaitCommand(invocation.command); err != nil {
		t.Fatalf("recovery Process.Execute error = %v", err)
	}
	if got := strings.TrimSuffix(invocation.inputs.Stdout(), "\n"); got != "recovery invocation COMPLETE" {
		t.Fatalf("recovery stdout = %q, want exact primary result", got)
	}
	if invocation.inputs.Stderr() != "" {
		t.Fatalf("recovery stderr = %q, want empty", invocation.inputs.Stderr())
	}
	finishHostedLifecycleInvocation(t, invocation.coordinator, invocation.baseURL, invocation.listenerClose, workID, worker.WorkerSessionId)
	return workID, worker.WorkerSessionId
}

func waitForLifecycleWorkState(
	t *testing.T,
	baseURL, sessionID string,
	accept func(factoryapi.Work) bool,
) (factoryapi.Work, factoryapi.StatusResponse) {
	t.Helper()
	observation, err := support.WaitForObservation(
		lifecycleObservationTimeoutForTest,
		func() (lifecycleWorkStatusObservation, error) {
			work, err := readLifecycleHTTPJSON[factoryapi.ListWorkResponse](
				support.SessionWorkURL(baseURL, sessionID, "/work"),
			)
			if err != nil {
				return lifecycleWorkStatusObservation{}, err
			}
			status, err := readLifecycleHTTPJSON[factoryapi.StatusResponse](
				strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/status",
			)
			if err != nil {
				return lifecycleWorkStatusObservation{}, err
			}
			return lifecycleWorkStatusObservation{work: work, status: status}, nil
		},
		func(observation lifecycleWorkStatusObservation) bool {
			for _, work := range observation.work.Results {
				if work.WorkId != nil && accept(work) {
					return true
				}
			}
			return false
		},
	)
	if err != nil {
		states := make([]factoryapi.WorkStateType, 0, len(observation.work.Results))
		for _, work := range observation.work.Results {
			if work.State == nil {
				continue
			}
			states = append(states, work.State.Type)
		}
		t.Fatalf("wait for adverse Work projection: %v; states=%v status=%#v", err, states, observation.status)
	}
	for _, work := range observation.work.Results {
		if work.WorkId != nil && accept(work) {
			return work, observation.status
		}
	}
	t.Fatal("accepted Work projection disappeared")
	return factoryapi.Work{}, factoryapi.StatusResponse{}
}

func waitForLifecycleTerminalWorkText(baseURL, sessionID, wantText string) (string, bool, string) {
	workID, err := support.WaitForObservation(
		lifecycleObservationTimeoutForTest,
		func() (string, error) {
			workID, ok, diagnostic := tryReadTerminalWorkPrimaryText(baseURL, sessionID, wantText)
			if !ok {
				return "", errors.New(diagnostic)
			}
			return workID, nil
		},
		func(workID string) bool { return strings.TrimSpace(workID) != "" },
	)
	if err != nil {
		return "", false, fmt.Sprintf("wait for terminal Work %q: %v", wantText, err)
	}
	return workID, true, ""
}

func finishHostedLifecycleInvocation(
	t *testing.T,
	coordinator *lifecycleCoordinator,
	baseURL string,
	listenerClose <-chan struct{},
	workID, workerSessionID string,
) {
	t.Helper()
	if coordinator == nil {
		t.Fatal("finish hosted lifecycle invocation: coordinator is nil")
	}
	if unreleased := coordinator.unreleasedGates(); len(unreleased) != 0 {
		t.Fatalf("hosted lifecycle unreleased gates before close = %v", unreleased)
	}
	closeLifecycleCoordinator(t, coordinator)
	waitLifecycleSignal(t, listenerClose, "hosted lifecycle listener close")
	if !lifecycleListenerClosed(baseURL, listenerClose) {
		t.Fatalf("hosted lifecycle listener at %s remained reachable after Work %q / Worker Session %q close", baseURL, workID, workerSessionID)
	}
}

func finishCancelableHostedLifecycleInvocation(
	t *testing.T,
	coordinator *lifecycleCoordinator,
	baseURL string,
	listenerClose <-chan struct{},
	workID, workerSessionID string,
) {
	t.Helper()
	if unreleased := coordinator.unreleasedGates(); len(unreleased) != 0 {
		t.Fatalf("cancelable hosted lifecycle unreleased gates before close = %v", unreleased)
	}
	closeLifecycleCoordinator(t, coordinator)
	waitLifecycleSignal(t, listenerClose, "cancelable hosted lifecycle listener close")
	if !lifecycleListenerClosed(baseURL, listenerClose) {
		t.Fatalf("cancelable hosted lifecycle listener at %s remained reachable after Work %q / Worker Session %q close", baseURL, workID, workerSessionID)
	}
}

func closeLifecycleCoordinator(t *testing.T, coordinator *lifecycleCoordinator) {
	t.Helper()
	coordinator.close()
	if err, duration := coordinator.closeResult(); err != nil {
		t.Fatalf("Process.Close error = %v after %s", err, duration)
	} else if duration > lifecycleAdverseCloseTimeout {
		t.Fatalf("Process.Close duration = %s, want <= %s cleanup ceiling", duration, lifecycleAdverseCloseTimeout)
	}
	if unreleased := coordinator.unreleasedGates(); len(unreleased) != 0 {
		t.Fatalf("Process.Close left lifecycle gates unreleased: %v", unreleased)
	}
}

func assertLifecycleFailureOutput(t *testing.T, inputs *support.CapturedInputs, executeErr error) {
	t.Helper()
	if executeErr == nil {
		t.Fatal("failure output assertion received nil Process.Execute error")
	}
	if strings.TrimSpace(inputs.Stdout()) != "" {
		t.Fatalf("failure stdout = %q, want empty stdout without false success", inputs.Stdout())
	}
	response := decodeSingleErrorResponse(t, inputs.Stderr())
	if response.Code == "" || strings.TrimSpace(response.Message) == "" {
		t.Fatalf("failure ErrorResponse = %#v, want actionable code and message", response)
	}
	if response.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("failure ErrorResponse family = %q, want %q", response.Family, factoryapi.ErrorFamilyInternalServerError)
	}
}

func readLifecycleHTTPJSON[T any](endpoint string) (T, error) {
	var result T
	response, err := lifecycleHTTPClient.Get(endpoint)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		return result, fmt.Errorf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(payload)))
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("decode GET %s: %w", endpoint, err)
	}
	return result, nil
}

func waitLifecycleSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	timer := time.NewTimer(lifecycleAdverseSignalTimeout)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("%s was not observed within %s", name, lifecycleAdverseSignalTimeout)
	}
}

func containsWorkOutcome(wanted []factoryapi.WorkOutcome, got factoryapi.WorkOutcome) bool {
	for _, candidate := range wanted {
		if candidate == got {
			return true
		}
	}
	return false
}

func containsWorkerSessionState(wanted []factoryapi.WorkerSessionObservationState, got factoryapi.WorkerSessionObservationState) bool {
	for _, candidate := range wanted {
		if candidate == got {
			return true
		}
	}
	return false
}

type lifecycleResultRunner struct {
	result platformprocess.CommandResult
	calls  atomic.Int32
}

func (runner *lifecycleResultRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	result := runner.result
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result, nil
}

func (runner *lifecycleResultRunner) CallCount() int {
	if runner == nil {
		return 0
	}
	return int(runner.calls.Load())
}

type blockingLifecycleRunner struct {
	started    chan struct{}
	finished   chan struct{}
	startOnce  sync.Once
	finishOnce sync.Once
	calls      atomic.Int32
	canceled   atomic.Int32
}

func newBlockingLifecycleRunner() *blockingLifecycleRunner {
	return &blockingLifecycleRunner{
		started:  make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (runner *blockingLifecycleRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	runner.startOnce.Do(func() { close(runner.started) })
	<-ctx.Done()
	runner.canceled.Add(1)
	runner.finishOnce.Do(func() { close(runner.finished) })
	return platformprocess.CommandResult{}, ctx.Err()
}

func (runner *blockingLifecycleRunner) CancellationCount() int {
	if runner == nil {
		return 0
	}
	return int(runner.canceled.Load())
}

func waitCancelableLifecycleCommand(command *lifecycleCancelableCommand) error {
	if command == nil {
		return errors.New("cancelable lifecycle command is nil")
	}
	timer := time.NewTimer(lifecycleCommandDoneTimeout)
	defer timer.Stop()
	select {
	case <-command.Done():
		return command.Err()
	case <-timer.C:
		return fmt.Errorf("lifecycle command completion deadline expired after %s", lifecycleCommandDoneTimeout)
	}
}

func isClosed(signal <-chan struct{}) bool {
	if signal == nil {
		return true
	}
	select {
	case <-signal:
		return true
	default:
		return false
	}
}

var _ platformprocess.CommandRunner = (*lifecycleResultRunner)(nil)
var _ platformprocess.CommandRunner = (*blockingLifecycleRunner)(nil)
