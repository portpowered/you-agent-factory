package sessioncontrols_test

import (
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPausedFactorySessionReturnsInvocationPausedStatus proves invocation against
// a paused live Factory Session returns a typed INVOCATION_PAUSED failure through
// the public session invocation API without fabricating a completed primary result.
func TestPausedFactorySessionReturnsInvocationPausedStatus(t *testing.T) {
	factoryDir := scaffoldInvocationFactory(t, nil)
	session := openSharedControlsSession(t, factoryDir, sharedControlsRouteConfig{
		provider: support.NewStaticSuccessCommandRunner("primary result COMPLETE"),
	})

	baseURL := session.baseURL()
	sessionID := session.id()
	pause := postSessionLifecycleControl(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}

	response := postInvocation(t, baseURL, sessionID, textInvocationRequest(t, "invoke this", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil ||
		*response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_PAUSED") {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_PAUSED", response.ErrorCode)
	}
	if response.Message == nil ||
		!strings.Contains(*response.Message, `session "`+sessionID+`" is paused`) {
		gotMessage := "<nil>"
		if response.Message != nil {
			gotMessage = *response.Message
		}
		t.Fatalf("invocation message = %q, want paused session detail", gotMessage)
	}
	if response.SessionId == nil || *response.SessionId != sessionID {
		t.Fatalf("invocation sessionId = %#v, want %q", response.SessionId, sessionID)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on paused output", response.PrimaryResult)
	}
}

// TestPausedFactorySessionBuffersSubmittedWork proves a paused Factory Session
// accepts submitted work through the public session-control boundary while
// keeping that work buffered instead of advancing it into active processing.
func TestPausedFactorySessionBuffersSubmittedWork(t *testing.T) {
	factoryDir := scaffoldPauseResumeControlsFactory(t)

	session := openSharedControlsSession(t, factoryDir, sharedControlsRouteConfig{})
	baseURL := session.baseURL()
	sessionID := session.id()

	pause := postSessionLifecycleControl(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}
	if pause.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("pause status = %q, want PAUSED", pause.Status)
	}

	pausedSession := getControlsSession(t, baseURL, sessionID)
	if pausedSession.Runtime.LifecycleControlStatus == nil ||
		*pausedSession.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf(
			"session read after pause lifecycleControlStatus = %#v, want %q",
			pausedSession.Runtime.LifecycleControlStatus,
			factoryapi.FactorySessionDurableLifecycleStatusPaused,
		)
	}

	workName := "paused-buffered-task"
	submitted := submitControlsSessionWork(t, baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         stringPointer(workName),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "Submitted while paused"},
	})
	workID := stringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submit while paused missing work id: %#v", submitted)
	}

	listed := listControlsSessionWork(t, baseURL, sessionID)
	if support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "init")) {
		t.Fatalf("work %q reached task:init while session was paused: %#v", workID, listed.Results)
	}
	if support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "complete")) {
		t.Fatalf("work %q reached task:complete while session was paused: %#v", workID, listed.Results)
	}
}

// TestResumedFactorySessionDrainsBufferedWorkInOrder proves resume drains work
// buffered during pause in submission order through public session-control and
// dispatch observation boundaries.
func TestResumedFactorySessionDrainsBufferedWorkInOrder(t *testing.T) {
	factoryDir := scaffoldPauseResumeControlsFactory(t)

	session := openSharedControlsSession(t, factoryDir, sharedControlsRouteConfig{})
	baseURL := session.baseURL()
	sessionID := session.id()

	pause := postSessionLifecycleControl(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}

	first := submitControlsSessionWork(t, baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("paused-buffered-first"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "First paused submission"},
	})
	second := submitControlsSessionWork(t, baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("paused-buffered-second"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "Second paused submission"},
	})
	firstID := stringPointerValue(first.WorkId)
	secondID := stringPointerValue(second.WorkId)
	if firstID == "" || secondID == "" {
		t.Fatalf("submit while paused missing work ids: first=%#v second=%#v", first, second)
	}

	listed := listControlsSessionWork(t, baseURL, sessionID)
	for _, workID := range []string{firstID, secondID} {
		if support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "complete")) {
			t.Fatalf("work %q reached task:complete before resume: %#v", workID, listed.Results)
		}
	}

	resume := postSessionLifecycleControl(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}
	if resume.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("resume status = %q, want RUNNING", resume.Status)
	}

	assertBufferedWorkDrainedInSubmissionOrder(t, baseURL, sessionID, firstID, secondID)
}

// TestPauseResumeEmitsDurableLifecycleEvents proves pause and resume through
// the public session-control boundary leave durable SESSION_LIFECYCLE_CONTROL
// Factory Events in chronological order with public control operation kinds.
func TestPauseResumeEmitsDurableLifecycleEvents(t *testing.T) {
	factoryDir := scaffoldPauseResumeControlsFactory(t)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("buffered task accepted COMPLETE"),
	})
	session := openSharedControlsSession(t, factoryDir, sharedControlsRouteConfig{
		provider: runner,
	})

	baseURL := session.baseURL()
	sessionID := session.id()
	eventStream := openPauseResumeLifecycleEventStream(t, baseURL, sessionID)

	pause := postSessionLifecycleControl(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}

	submitted := submitControlsSessionWork(t, baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("lifecycle-event-buffered-task"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "Submitted between pause and resume"},
	})
	workID := stringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submit while paused missing work id: %#v", submitted)
	}

	resume := postSessionLifecycleControl(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}

	waitForPauseResumeLifecycleControlEvents(
		t,
		eventStream,
		pauseResumeDurableStatusTimeout,
	)
}

// TestInterruptedWorkInspectSurfacesDispatchAndStopSummary proves work stopped
// on an interrupted customer state surfaces INTERRUPTED stop summaries with
// dispatch context on public session and work read surfaces.
func TestInterruptedWorkInspectSurfacesDispatchAndStopSummary(t *testing.T) {
	factoryDir := scaffoldInterruptedInspectFactory(t)

	session := openSharedControlsSession(t, factoryDir, sharedControlsRouteConfig{
		script: support.NewStaticSuccessCommandRunner("interrupted"),
	})
	baseURL := session.baseURL()
	sessionID := session.id()
	submitted := submitControlsSessionWork(t, baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("interrupted-operator-inspect"),
		WorkTypeName: interruptedInspectWorkTypeName,
		Payload:      map[string]string{"title": "Interrupted inspect probe"},
	})
	workID := stringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submit interrupted inspect work missing work id: %#v", submitted)
	}

	interruptedLocation := support.WorkCustomerLocation(
		interruptedInspectWorkTypeName,
		"interrupted",
	)
	waitForSessionWorkIDsAtCustomerState(
		t,
		baseURL,
		sessionID,
		[]string{workID},
		interruptedLocation,
		pauseResumeDrainWaitTimeout,
	)

	work := getControlsSessionWorkByID(t, baseURL, sessionID, workID)
	if work.StopSummary == nil {
		t.Fatalf("work show = %#v, want stopSummary on interrupted work", work)
	}
	assertInterruptedStopSummary(t, work.StopSummary, "work")

	liveSession := getControlsSession(t, baseURL, sessionID)
	if liveSession.Runtime.StopSummary != nil {
		assertInterruptedStopSummary(t, liveSession.Runtime.StopSummary, "session")
	}

	listed := listControlsSessionWork(t, baseURL, sessionID)
	completeLocation := support.WorkCustomerLocation(interruptedInspectWorkTypeName, "complete")
	if support.HasWorkAtCustomerState(listed, workID, completeLocation) {
		t.Fatalf("interrupted work %q reached %s", workID, completeLocation)
	}
	if !support.HasWorkAtCustomerState(listed, workID, interruptedLocation) {
		t.Fatalf(
			"work listing missing %s for work %q: %#v",
			interruptedLocation,
			workID,
			listed.Results,
		)
	}
}

// TestAPIPauseResumeCancelAndTerminateFactorySession proves public API pause,
// resume, cancel, and terminate controls return typed lifecycle-control outcomes
// and leave each Factory Session in the expected lifecycle state after control.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestAPIPauseResumeCancelAndTerminateFactorySession(t *testing.T) {
	factoryDir := pauseResumeControlsFactoryDirWithBusyLoop(t)

	session := openSharedControlsSession(t, factoryDir, sharedControlsRouteConfig{})
	baseURL := session.baseURL()
	liveSession := getControlsSession(t, baseURL, session.id())
	liveSessionID := liveSession.Id
	if liveSessionID == "" {
		t.Fatal("live Factory Session id is empty")
	}

	pause := postSessionLifecycleControl(
		t,
		baseURL,
		liveSessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	assertAcceptedSessionLifecycleControl(
		t,
		pause,
		liveSessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
	)
	assertLiveSessionLifecycleControlStatus(
		t,
		baseURL,
		liveSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
	)

	resume := postSessionLifecycleControl(
		t,
		baseURL,
		liveSessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	assertAcceptedSessionLifecycleControl(
		t,
		resume,
		liveSessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
	)
	assertLiveSessionLifecycleControlStatus(
		t,
		baseURL,
		liveSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
	)

	cancelSessionID := startBusyLoopDurableSession(
		t,
		baseURL,
		uniqueControlsDurableRequestID("req-sessions-controls-cancel"),
	)
	cancelEvents := openDurableSessionEventStream(t, baseURL, cancelSessionID)
	waitForDurableSessionLifecycleStatus(
		t,
		baseURL,
		cancelEvents,
		cancelSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		pauseResumeDurableStatusTimeout,
	)

	cancel := postSessionLifecycleControl(
		t,
		baseURL,
		cancelSessionID,
		factoryapi.FactorySessionLifecycleControlKindCancel,
	)
	assertAcceptedSessionLifecycleControl(
		t,
		cancel,
		cancelSessionID,
		factoryapi.FactorySessionLifecycleControlKindCancel,
		factoryapi.FactorySessionDurableLifecycleStatusCanceling,
	)
	waitForDurableSessionLifecycleStatus(
		t,
		baseURL,
		cancelEvents,
		cancelSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusCanceled,
		pauseResumeDurableStatusTimeout,
	)

	terminateSessionID := startBusyLoopDurableSession(
		t,
		baseURL,
		uniqueControlsDurableRequestID("req-sessions-controls-terminate"),
	)
	terminateEvents := openDurableSessionEventStream(t, baseURL, terminateSessionID)
	waitForDurableSessionLifecycleStatus(
		t,
		baseURL,
		terminateEvents,
		terminateSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		pauseResumeDurableStatusTimeout,
	)

	terminate := postSessionLifecycleControl(
		t,
		baseURL,
		terminateSessionID,
		factoryapi.FactorySessionLifecycleControlKindTerminate,
	)
	assertAcceptedSessionLifecycleControl(
		t,
		terminate,
		terminateSessionID,
		factoryapi.FactorySessionLifecycleControlKindTerminate,
		factoryapi.FactorySessionDurableLifecycleStatusTerminated,
	)
	waitForDurableSessionLifecycleStatus(
		t,
		baseURL,
		terminateEvents,
		terminateSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusTerminated,
		pauseResumeDurableStatusTimeout,
	)

	terminated := readDurableFactorySession(t, baseURL, terminateSessionID)
	if terminated.Status != factoryapi.FactorySessionDurableLifecycleStatusTerminated &&
		terminated.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceled {
		t.Fatalf(
			"durable session %s status after terminate = %q, want TERMINATED or CANCELED",
			terminateSessionID,
			terminated.Status,
		)
	}
}

// TestAPIInvalidLifecycleTransitionReturnsConflict proves illegal Factory Session
// lifecycle controls through the public API return HTTP conflict with typed rejection
// outcomes and leave the session in its prior terminal lifecycle state.
func TestAPIInvalidLifecycleTransitionReturnsConflict(t *testing.T) {
	acquireSharedControlsScenarioSlot(t)
	fixture := sharedControlsProcess(t)
	baseURL := fixture.baseURL
	sessionID := startBusyLoopDurableSession(
		t,
		baseURL,
		uniqueControlsDurableRequestID("req-sessions-controls-invalid-transition"),
	)
	events := openDurableSessionEventStream(t, baseURL, sessionID)
	waitForDurableSessionLifecycleStatus(
		t,
		baseURL,
		events,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		pauseResumeDurableStatusTimeout,
	)

	terminate := postSessionLifecycleControl(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindTerminate,
	)
	assertAcceptedSessionLifecycleControl(
		t,
		terminate,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindTerminate,
		factoryapi.FactorySessionDurableLifecycleStatusTerminated,
	)

	waitForDurableSessionLifecycleStatus(
		t,
		baseURL,
		events,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusTerminated,
		pauseResumeDurableStatusTimeout,
	)
	before := readDurableFactorySession(t, baseURL, sessionID)
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusTerminated &&
		before.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceled {
		t.Fatalf(
			"pre-invalid-control session %s status = %q, want TERMINATED or CANCELED",
			sessionID,
			before.Status,
		)
	}
	terminalStatus := before.Status

	resume := postSessionLifecycleControlExpectConflict(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	assertRejectedSessionLifecycleControl(
		t,
		resume,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
		terminalStatus,
	)

	assertDurableFactorySessionRemainsTerminal(
		t,
		baseURL,
		sessionID,
		"session status after rejected resume",
	)

	pause := postSessionLifecycleControlExpectConflict(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	assertRejectedSessionLifecycleControl(
		t,
		pause,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
		terminalStatus,
	)

	assertDurableFactorySessionRemainsTerminal(
		t,
		baseURL,
		sessionID,
		"session status after rejected pause",
	)
}
