package sessioncontrols_test

import (
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPausedFactorySessionBuffersSubmittedWork proves a paused Factory Session
// accepts submitted work through the public session-control boundary while
// keeping that work buffered instead of advancing it into active processing.
func TestPausedFactorySessionBuffersSubmittedWork(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, pauseResumeControlsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	sessionID := factorysessions.DefaultSessionID

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

	pausedSession := support.GetDefaultSession(t, baseURL)
	if pausedSession.Runtime.LifecycleControlStatus == nil ||
		*pausedSession.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf(
			"session read after pause lifecycleControlStatus = %#v, want %q",
			pausedSession.Runtime.LifecycleControlStatus,
			factoryapi.FactorySessionDurableLifecycleStatusPaused,
		)
	}

	workName := "paused-buffered-task"
	submitted := support.SubmitDefaultSessionWork(t, baseURL, factoryapi.SubmitWorkRequest{
		Name:         stringPointer(workName),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "Submitted while paused"},
	})
	workID := stringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submit while paused missing work id: %#v", submitted)
	}

	listed := support.ListDefaultSessionWork(t, baseURL)
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
	factoryDir := support.ScaffoldFactory(t, pauseResumeControlsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:        factoryDir,
		MockWorkersConfig: pauseResumeControlsSlowMockWorkersConfig(),
	})
	defer server.Stop(t)

	baseURL := server.URL()
	sessionID := factorysessions.DefaultSessionID

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

	first := support.SubmitDefaultSessionWork(t, baseURL, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("paused-buffered-first"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "First paused submission"},
	})
	second := support.SubmitDefaultSessionWork(t, baseURL, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("paused-buffered-second"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "Second paused submission"},
	})
	firstID := stringPointerValue(first.WorkId)
	secondID := stringPointerValue(second.WorkId)
	if firstID == "" || secondID == "" {
		t.Fatalf("submit while paused missing work ids: first=%#v second=%#v", first, second)
	}

	listed := support.ListDefaultSessionWork(t, baseURL)
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

	assertBufferedWorkDrainedInSubmissionOrder(t, server, firstID, secondID)
}

// TestPauseResumeEmitsDurableLifecycleEvents proves pause and resume through
// the public session-control boundary leave durable SESSION_LIFECYCLE_CONTROL
// Factory Events in chronological order with public control operation kinds.
func TestPauseResumeEmitsDurableLifecycleEvents(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, pauseResumeControlsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	sessionID := factorysessions.DefaultSessionID

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

	submitted := support.SubmitDefaultSessionWork(t, baseURL, factoryapi.SubmitWorkRequest{
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

	waitForSessionWorkIDsAtCustomerState(
		t,
		baseURL,
		[]string{workID},
		support.WorkCustomerLocation("task", "complete"),
		pauseResumeDrainWaitTimeout,
	)

	assertPauseResumeLifecycleControlEvents(t, server.GetFactoryEvents(t))
}
