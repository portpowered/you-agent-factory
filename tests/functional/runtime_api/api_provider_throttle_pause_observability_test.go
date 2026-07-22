package runtime_api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// portos:func-length-exception owner=agent-factory reason=provider-throttle-runtime-observability-smoke review=2026-07-19 removal=split-pause-setup-runtime-polling-and-dashboard-assertions-before-next-throttle-observability-change
func TestProviderErrorSmoke_ThrottleFailureIsolatesOtherLaneThroughPublicSession(t *testing.T) {
	support.SkipLongFunctional(t, "slow throttle-pause observability smoke")
	fixture := newThrottlePauseObservabilityFixture(t)

	waitForThrottlePausePublicSession(
		t,
		fixture.server,
		10*time.Second,
		func(session factoryapi.FactorySession) bool {
			return fixture.runner.CallCount() >= 3 &&
				support.SessionHasWorkAtPlace(session, fixture.throttledWork.WorkID, fixture.throttledWork.WorkTypeID+":init")
		},
	)
	fixture.unaffectedWork.WorkID = submitThrottlePauseWork(t, fixture.server, fixture.unaffectedWork)

	isolatedSession := waitForThrottlePausePublicSession(
		t,
		fixture.server,
		5*time.Second,
		func(session factoryapi.FactorySession) bool {
			return support.SessionHasWorkAtPlace(session, fixture.throttledWork.WorkID, fixture.throttledWork.WorkTypeID+":init") &&
				support.SessionHasWorkAtPlace(session, fixture.unaffectedWork.WorkID, fixture.unaffectedWork.WorkTypeID+":complete")
		},
	)

	if isolatedSession.Runtime.Progress.InFlightCount != 0 {
		t.Fatalf("isolated public session in-flight count = %d, want 0", isolatedSession.Runtime.Progress.InFlightCount)
	}

	assertThrottlePauseRequestSequence(t, fixture.runner.Requests())

	dispatches := support.ObserveDispatchEvents(t, fixture.server.GetFactoryEvents(t))
	throttledDispatches := dispatchesForProviderSmokeWork(dispatches, fixture.throttledWork)
	unaffectedDispatches := dispatchesForProviderSmokeWork(dispatches, fixture.unaffectedWork)
	if len(throttledDispatches) == 0 {
		t.Fatal("throttled lane dispatch count = 0, want at least one failed dispatch")
	}
	if len(unaffectedDispatches) != 1 {
		t.Fatalf("unaffected lane dispatch count = %d, want 1", len(unaffectedDispatches))
	}
	if throttledDispatches[0].Response == nil || throttledDispatches[0].Response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("first throttled dispatch = %#v, want FAILED response", throttledDispatches[0])
	}
	if len(throttledDispatches) > 1 &&
		(throttledDispatches[1].Response == nil || throttledDispatches[1].Response.Outcome != factoryapi.WorkOutcomeAccepted) {
		t.Fatalf("second throttled dispatch = %#v, want ACCEPTED response", throttledDispatches[1])
	}
	if unaffectedDispatches[0].Response == nil || unaffectedDispatches[0].Response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("unaffected dispatch = %#v, want ACCEPTED response", unaffectedDispatches[0])
	}
}

type throttlePauseObservabilityFixture struct {
	server         *functionalAPIServer
	runner         *testutil.ProviderCommandRunner
	throttledWork  testutil.ProviderErrorSmokeWork
	unaffectedWork testutil.ProviderErrorSmokeWork
}

func newThrottlePauseObservabilityFixture(t *testing.T) throttlePauseObservabilityFixture {
	t.Helper()

	const pauseDuration = 2 * time.Second
	pauseHarness := testutil.NewProviderErrorSmokePauseIsolationHarness(
		t,
		testutil.ProviderErrorSmokeLane{
			WorkTypeID:      "claude-task",
			WorkerName:      "claude-worker",
			WorkstationName: "process-claude",
			Provider:        modelprovider.ProviderClaude,
			Model:           "claude-sonnet-4-5-20250514",
			PromptBody:      "Process the Claude lane task.\n",
		},
		testutil.ProviderErrorSmokeLane{
			WorkTypeID:      "codex-task",
			WorkerName:      "codex-worker",
			WorkstationName: "process-codex",
			Provider:        modelprovider.ProviderCodex,
			Model:           "gpt-5-codex",
			PromptBody:      "Process the Codex lane task.\n",
		},
	)
	runner := pauseHarness.ProviderRunner()
	pauseHarness.QueueProviderResults(
		support.RepeatedProviderErrorCommandResults(t, "claude_rate_limit_error", 3)...,
	)
	pauseHarness.QueueProviderResults(
		platformprocess.CommandResult{Stdout: []byte("codex lane completed while claude was paused. COMPLETE")},
	)

	fixture := throttlePauseObservabilityFixture{
		runner: runner,
		throttledWork: testutil.ProviderErrorSmokeWork{
			Name:       "claude-observable-throttle-lane",
			WorkID:     "work-claude-observable-throttle-lane",
			WorkTypeID: "claude-task",
			TraceID:    "trace-claude-observable-throttle-lane",
			Payload:    []byte("claude observable throttle payload"),
		},
		unaffectedWork: testutil.ProviderErrorSmokeWork{
			Name:       "codex-observable-healthy-lane",
			WorkID:     "work-codex-observable-healthy-lane",
			WorkTypeID: "codex-task",
			TraceID:    "trace-codex-observable-healthy-lane",
			Payload:    []byte("codex observable healthy payload"),
		},
	}
	pauseHarness.SeedWork(t, fixture.throttledWork)
	testutil.AppendFactoryInferenceThrottleGuard(
		t,
		pauseHarness.Dir,
		modelprovider.ProviderClaude,
		"claude-sonnet-4-5-20250514",
		pauseDuration,
	)
	fixture.server = startFunctionalServerWithArgs(
		t,
		pauseHarness.Dir,
		false,
		nil,
		withWorkerCommands(runner, nil),
	)
	return fixture
}

func submitThrottlePauseWork(
	t *testing.T,
	server *functionalAPIServer,
	work testutil.ProviderErrorSmokeWork,
) string {
	t.Helper()

	payload, err := json.Marshal(string(work.Payload))
	if err != nil {
		t.Fatalf("marshal throttle-pause Work payload: %v", err)
	}
	submitted := server.SubmitRuntimeWork(t, workdomain.SubmitRequest{
		Name:       work.Name,
		WorkID:     work.WorkID,
		WorkTypeID: work.WorkTypeID,
		TraceID:    work.TraceID,
		Payload:    payload,
	})
	if len(submitted) != 1 || submitted[0].WorkID == "" {
		t.Fatalf("submit throttle-pause Work response = %#v, want one Work ID", submitted)
	}
	return submitted[0].WorkID
}

func assertThrottlePauseRequestSequence(t *testing.T, requests []platformprocess.CommandRequest) {
	t.Helper()

	if len(requests) < 4 {
		t.Fatalf("provider command count = %d, want at least 4", len(requests))
	}
	for i := 0; i < 3; i++ {
		if requests[i].Command != string(modelprovider.ProviderClaude) {
			t.Fatalf("request %d command = %q, want %q", i, requests[i].Command, modelprovider.ProviderClaude)
		}
	}
	if requests[3].Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("request 3 command = %q, want %q", requests[3].Command, modelprovider.ProviderCodex)
	}
}

func waitForThrottlePausePublicSession(
	t *testing.T,
	server *functionalAPIServer,
	timeout time.Duration,
	match func(factoryapi.FactorySession) bool,
) factoryapi.FactorySession {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := support.GetDefaultSession(t, server.URL())
		if match(session) {
			return session
		}
		time.Sleep(100 * time.Millisecond)
	}

	session := support.GetDefaultSession(t, server.URL())
	if session.Runtime.Petri != nil {
		t.Fatalf("timed out waiting for public Factory Session within %s: progress=%#v marking=%#v", timeout, session.Runtime.Progress, session.Runtime.Petri.Marking)
	}
	t.Fatalf("timed out waiting for public Factory Session within %s: %#v", timeout, session.Runtime)
	return session
}

func dispatchesForProviderSmokeWork(
	dispatches []support.DispatchEventObservation,
	work testutil.ProviderErrorSmokeWork,
) []support.DispatchEventObservation {
	matches := make([]support.DispatchEventObservation, 0, len(dispatches))
	for _, dispatch := range dispatches {
		if support.DispatchObservationIncludesWork(dispatch, work.WorkID) {
			matches = append(matches, dispatch)
		}
	}
	return matches
}
