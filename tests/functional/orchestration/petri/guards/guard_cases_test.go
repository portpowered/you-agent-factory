package guards

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestGuardSameNameMismatchRemainsUndispatched(t *testing.T) {
	// CASE-G-004: a same-name guard mismatch leaves the joined inputs in their
	// authored nonterminal states and emits no joined dispatch.
	dir := newSharedGuardScenario(t, secondaryDependencyJoinFactoryConfig())
	seedGuardWork(t, dir, "producer", secondaryJoinProducerWorkID, "producer")
	seedGuardWork(t, dir, "plan-only", secondaryJoinPlanWorkID, "plan")
	seedGuardWorkWithRelation(t, dir, "task-only", secondaryJoinTaskWorkID, "task", work.Relation{
		Type:          work.RelationDependsOn,
		TargetWorkID:  secondaryJoinProducerWorkID,
		RequiredState: "complete",
	})

	fixture := sharedGuardProcess(t)
	completed := make(chan struct{})
	started := make(chan struct{})
	session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{
		provider: signaledSharedGuardProviderOutput("COMPLETE", started, completed),
	})
	waitForSharedGuardSignal(t, completed, "mismatch prerequisite completion")
	waitForGuardDispatchResponse(t, session, secondaryJoinProduce)
	_, listed, events := readSharedGuardSession(t, session)

	if got := len(dispatchRequestsForTransition(t, events, secondaryJoinTransition)); got != 0 {
		t.Fatalf("joined dispatches after same-name mismatch = %d, want zero", got)
	}
	assertGuardWorkState(t, listed, secondaryJoinProducerWorkID, "complete")
	assertGuardWorkState(t, listed, secondaryJoinPlanWorkID, "ready")
	assertGuardWorkState(t, listed, secondaryJoinTaskWorkID, "ready")
	if got := fixture.router.providerCallsFor(dir); got != 1 {
		t.Fatalf("provider command calls after same-name mismatch = %d, want one prerequisite call", got)
	}
	select {
	case <-started:
	default:
		t.Fatal("mismatch prerequisite did not reach the controlled provider edge")
	}
}

func TestGuardWorkerErrorProducesOneFailedOutcome(t *testing.T) {
	// CASE-G-006: a controlled nonzero Worker result fails the affected Work
	// once and preserves the primary diagnostic on the public response event.
	const diagnostic = "provider execution failed"
	dir := newSharedGuardScenario(t, sharedGuardSingleStepFactoryConfig(nil))
	seedGuardWork(t, dir, "worker-error", "guard-error-work", "task")
	session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{provider: sharedGuardProviderSequence(
		sharedGuardCommandResponse{result: platformprocess.CommandResult{
			Stderr:   []byte(diagnostic),
			ExitCode: 23,
		}},
	)})
	supportWaitForGuardTerminal(t, session)
	_, listed, events := readSharedGuardSession(t, session)

	assertGuardWorkCounts(t, listed, 0, 1)
	responses := guardDispatchResponseEventsForTransition(t, events, sharedGuardSingleStepTransition)
	if len(responses) != 1 {
		t.Fatalf("failed process dispatch responses = %d, want one", len(responses))
	}
	assertGuardResponseOutcome(t, responses[0], factoryapi.WorkOutcomeFailed, diagnostic)
	if got := session.fixture.router.providerCallsFor(dir); got != 1 {
		t.Fatalf("provider command calls for failed Work = %d, want one", got)
	}
}

func TestGuardTimeoutDiagnosticProducesFailedOutcome(t *testing.T) {
	// CASE-G-007: the controlled Worker edge reaches a real context deadline;
	// the public failure names timeout and does not claim successful completion.
	dir := newSharedGuardScenario(t, sharedGuardSingleStepFactoryConfig(nil))
	seedGuardWork(t, dir, "timeout", "guard-timeout-work", "task")
	session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{
		provider: sharedGuardProviderTimeout("guard prerequisite timeout"),
	})
	supportWaitForGuardTerminal(t, session)
	_, listed, events := readSharedGuardSession(t, session)

	assertGuardWorkCounts(t, listed, 0, 1)
	responses := guardDispatchResponseEventsForTransition(t, events, sharedGuardSingleStepTransition)
	if len(responses) != 3 {
		t.Fatalf("timeout process dispatch responses = %d, want three retry responses", len(responses))
	}
	for _, response := range responses {
		assertGuardResponseOutcome(t, response, factoryapi.WorkOutcomeFailed, "timeout")
	}
	if got := session.fixture.router.providerCallsFor(dir); got != 9 {
		t.Fatalf("provider command calls for timeout Work = %d, want nine bounded retry calls", got)
	}
}

func TestGuardPartialCompletionPreservesIndependentOutcome(t *testing.T) {
	// CASE-G-008: one controlled success and one controlled error settle as
	// independent terminal outcomes without replaying either dispatch.
	dir := newSharedGuardScenario(t, sharedGuardSingleStepFactoryConfig(nil))
	seedGuardWork(t, dir, "partial-complete", "guard-partial-complete", "task")
	seedGuardWork(t, dir, "partial-failed", "guard-partial-failed", "task")
	const diagnostic = "provider execution failed"
	session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{provider: sharedGuardProviderSequence(
		sharedGuardProviderOutput("COMPLETE"),
		sharedGuardCommandResponse{result: platformprocess.CommandResult{
			Stderr:   []byte("partial guard failure"),
			ExitCode: 31,
		}},
	)})
	supportWaitForGuardTerminal(t, session)
	_, listed, events := readSharedGuardSession(t, session)

	assertGuardWorkCounts(t, listed, 1, 1)
	assertGuardWorkState(t, listed, "guard-partial-complete", "complete")
	assertGuardWorkState(t, listed, "guard-partial-failed", "failed")
	responses := guardDispatchResponseEventsForTransition(t, events, sharedGuardSingleStepTransition)
	if len(responses) != 2 {
		t.Fatalf("partial process dispatch responses = %d, want two", len(responses))
	}
	var accepted, failed int
	for _, event := range responses {
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode partial dispatch response: %v", err)
		}
		switch payload.Outcome {
		case factoryapi.WorkOutcomeAccepted:
			accepted++
		case factoryapi.WorkOutcomeFailed:
			failed++
			assertGuardResponseDiagnostic(t, payload, diagnostic)
		default:
			t.Fatalf("partial dispatch outcome = %q, want accepted or failed", payload.Outcome)
		}
	}
	if accepted != 1 || failed != 1 {
		t.Fatalf("partial dispatch outcomes accepted/failed = %d/%d, want 1/1", accepted, failed)
	}
	if got := session.fixture.router.providerCallsFor(dir); got != 2 {
		t.Fatalf("provider command calls for partial Work = %d, want two", got)
	}
}

func TestGuardConcurrentSessionsKeepResponsesKeyed(t *testing.T) {
	// CASE-G-009: two explicit sessions overlap on one host, release in the
	// opposite order, and retain disjoint outcomes, events, and edge counts.
	firstDir := newSharedGuardScenario(t, sharedGuardSingleStepFactoryConfig(nil))
	secondDir := newSharedGuardScenario(t, sharedGuardSingleStepFactoryConfig(nil))
	seedGuardWork(t, firstDir, "concurrent-success", "guard-concurrent-success", "task")
	seedGuardWork(t, secondDir, "concurrent-failure", "guard-concurrent-failure", "task")

	firstGate := newSharedGuardCommandGate()
	secondGate := newSharedGuardCommandGate()
	first := openSharedGuardSession(t, firstDir, sharedGuardRouteConfig{provider: firstGate.responder("COMPLETE")})
	second := openSharedGuardSession(t, secondDir, sharedGuardRouteConfig{provider: secondGate.responderResult(platformprocess.CommandResult{
		Stderr:   []byte("concurrent controlled failure"),
		ExitCode: 37,
	})})
	waitForSharedGuardSignal(t, firstGate.started, "first concurrent provider")
	waitForSharedGuardSignal(t, secondGate.started, "second concurrent provider")

	secondGate.releaseResponse()
	firstGate.releaseResponse()
	supportWaitForGuardTerminal(t, first)
	supportWaitForGuardTerminal(t, second)
	_, firstWork, firstEvents := readSharedGuardSession(t, first)
	_, secondWork, secondEvents := readSharedGuardSession(t, second)

	assertGuardWorkCounts(t, firstWork, 1, 0)
	assertGuardWorkState(t, firstWork, "guard-concurrent-success", "complete")
	assertGuardWorkCounts(t, secondWork, 0, 1)
	assertGuardWorkState(t, secondWork, "guard-concurrent-failure", "failed")
	assertGuardSessionEventsBelongTo(t, first.sessionID, firstEvents)
	assertGuardSessionEventsBelongTo(t, second.sessionID, secondEvents)
	assertGuardSessionSequences(t, first.sessionID, firstEvents)
	assertGuardSessionSequences(t, second.sessionID, secondEvents)
	if first.sessionID == second.sessionID {
		t.Fatal("concurrent explicit sessions reused the same session ID")
	}
	if got := first.fixture.router.providerCallsFor(firstDir); got != 1 {
		t.Fatalf("first concurrent provider calls = %d, want one", got)
	}
	if got := first.fixture.router.providerCallsFor(secondDir); got != 1 {
		t.Fatalf("second concurrent provider calls = %d, want one", got)
	}
	assertGuardResponseOutcome(t, guardDispatchResponseEventsForTransition(t, firstEvents, sharedGuardSingleStepTransition)[0], factoryapi.WorkOutcomeAccepted, "")
	assertGuardResponseOutcome(t, guardDispatchResponseEventsForTransition(t, secondEvents, sharedGuardSingleStepTransition)[0], factoryapi.WorkOutcomeFailed, "provider execution failed")
}

func TestGuardCancellationStopsGatedDispatchAndHostReuse(t *testing.T) {
	// CASE-G-010: terminating a session cancels its controlled dispatch and a
	// fresh explicit session can use the same process after that early exit.
	dir := newSharedGuardScenario(t, sharedGuardSingleStepFactoryConfig(nil))
	seedGuardWork(t, dir, "cancelled", "guard-cancelled", "task")
	fixture := sharedGuardProcess(t)
	gate := newSharedGuardCommandGate()
	session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{provider: gate.responder("COMPLETE")})
	waitForSharedGuardSignal(t, gate.started, "cancellable provider")
	support.TerminateFactorySessionAt(t, fixture.baseURL, session.sessionID)
	waitForSharedGuardSignal(t, gate.canceled, "cancellable provider context")
	support.WaitForSessionStopped(t, fixture.baseURL, session.sessionID, sharedGuardFixtureShutdownTimeout)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, session.sessionID)
	if responses := guardDispatchResponseEventsForTransition(t, events, sharedGuardSingleStepTransition); len(responses) != 0 {
		t.Fatalf("canceled dispatch responses = %d, want no post-cancellation response", len(responses))
	}
	_, listed, _ := readSharedGuardSession(t, session)
	assertGuardWorkCounts(t, listed, 0, 0)
	session.close(t)
	if got := fixture.router.routeCount(); got != 0 {
		t.Fatalf("routes after canceled session cleanup = %d, want zero", got)
	}

	reuseDir := newSharedGuardScenario(t, sharedGuardSingleStepFactoryConfig(nil))
	seedGuardWork(t, reuseDir, "reuse", "guard-reuse", "task")
	reuse := openSharedGuardSession(t, reuseDir, sharedGuardRouteConfig{provider: sharedGuardFixedProviderOutput("COMPLETE")})
	supportWaitForGuardTerminal(t, reuse)
	_, reuseWork, _ := readSharedGuardSession(t, reuse)
	assertGuardWorkState(t, reuseWork, "guard-reuse", "complete")
	reuse.close(t)
}

func TestGuardEmptyInputGuardListRemainsValid(t *testing.T) {
	// CASE-G-011: an explicitly empty input-guard list is valid and behaves as
	// an unguarded admission without panic or residue.
	dir := newSharedGuardScenario(t, sharedGuardSingleStepFactoryConfig([]map[string]any{}))
	seedGuardWork(t, dir, "empty-guards", "guard-empty-guards", "task")
	session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{provider: sharedGuardFixedProviderOutput("COMPLETE")})
	supportWaitForGuardTerminal(t, session)
	_, listed, events := readSharedGuardSession(t, session)
	assertGuardWorkCounts(t, listed, 1, 0)
	assertGuardWorkState(t, listed, "guard-empty-guards", "complete")
	assertGuardResponseOutcome(t, guardDispatchResponseEventsForTransition(t, events, sharedGuardSingleStepTransition)[0], factoryapi.WorkOutcomeAccepted, "")
}

func TestGuardUnknownDefinitionReturnsValidation(t *testing.T) {
	// CASE-G-012: unknown guard data is rejected at Factory open and creates no
	// live session, route, or dispatch.
	fixture := sharedGuardProcess(t)
	config := sharedGuardSingleStepFactoryConfig([]map[string]any{{"type": "unknown_guard"}})
	dir := newSharedGuardScenario(t, config)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	beforeDispatches := len(fixture.dispatches.Snapshot())
	statusCode, body := postSharedGuardOpenRequest(t, fixture.baseURL, dir)
	if statusCode != http.StatusBadRequest {
		t.Fatalf("unknown guard open status = %d, want 400; body=%s", statusCode, body)
	}
	assertGuardValidationDiagnostic(t, body, "unknown_guard")
	assertNoLiveGuardSession(t, fixture.baseURL, dir)
	if got := fixture.router.routeCount(); got != 0 {
		t.Fatalf("routes after unknown guard rejection = %d, want zero", got)
	}
	if got := len(fixture.dispatches.Snapshot()); got != beforeDispatches {
		t.Fatalf("dispatch records after unknown guard rejection = %d, want %d", got, beforeDispatches)
	}
}

func TestGuardDuplicateStateDefinitionReturnsValidation(t *testing.T) {
	// CASE-G-013: duplicate authored state identity is rejected before a live
	// session is created and leaves the shared host unchanged.
	fixture := sharedGuardProcess(t)
	config := sharedGuardSingleStepFactoryConfig(nil)
	workTypes := config["workTypes"].([]map[string]any)
	states := workTypes[0]["states"].([]map[string]string)
	workTypes[0]["states"] = append(states, map[string]string{"name": "init", "type": "INITIAL"})
	dir := newSharedGuardScenario(t, config)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	beforeDispatches := len(fixture.dispatches.Snapshot())
	statusCode, body := postSharedGuardOpenRequest(t, fixture.baseURL, dir)
	if statusCode != http.StatusBadRequest {
		t.Fatalf("duplicate state open status = %d, want 400; body=%s", statusCode, body)
	}
	assertGuardValidationDiagnostic(t, body, "duplicate")
	assertNoLiveGuardSession(t, fixture.baseURL, dir)
	if got := len(fixture.dispatches.Snapshot()); got != beforeDispatches {
		t.Fatalf("dispatch records after duplicate state rejection = %d, want %d", got, beforeDispatches)
	}
}

func TestGuardWorkRequestIDIsIdempotent(t *testing.T) {
	// CASE-G-015: repeating one stable Work Request identity returns the prior
	// admission and does not create another Work Request or dispatch.
	dir := newSharedGuardScenario(t, sharedGuardSingleStepFactoryConfig(nil))
	session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{provider: sharedGuardFixedProviderOutput("COMPLETE")})
	workTypeName := "task"
	workID := "guard-idempotent-work"
	request := factoryapi.WorkRequest{
		RequestId: "guard-idempotent-request",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         "idempotent",
			WorkId:       &workID,
			WorkTypeName: &workTypeName,
			Payload:      map[string]any{"case": "idempotent"},
		}},
	}
	first := upsertSharedGuardWorkRequest(t, session, request)
	second := upsertSharedGuardWorkRequest(t, session, request)
	if first.RequestId != second.RequestId || first.TraceId != second.TraceId || len(first.Works) != len(second.Works) {
		t.Fatalf("idempotent Work Request responses differ: first=%#v second=%#v", first, second)
	}
	if len(first.Works) != 1 || first.Works[0] != second.Works[0] {
		t.Fatalf("idempotent submitted Work differs: first=%#v second=%#v", first.Works, second.Works)
	}
	supportWaitForGuardTerminal(t, session)
	_, listed, events := readSharedGuardSession(t, session)
	if len(listed.Results) != 1 {
		t.Fatalf("idempotent Work results = %d, want one", len(listed.Results))
	}
	assertGuardWorkCounts(t, listed, 1, 0)
	requestEvents := 0
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeWorkRequest {
			requestEvents++
		}
	}
	if requestEvents != 1 {
		t.Fatalf("idempotent Work Request events = %d, want one", requestEvents)
	}
	if got := len(guardDispatchResponseEventsForTransition(t, events, sharedGuardSingleStepTransition)); got != 1 {
		t.Fatalf("idempotent process dispatch responses = %d, want one", got)
	}
}

func TestGuardCleanupReleasesEarlyExitSessionAndAllowsReuse(t *testing.T) {
	// CASE-G-017: early assertion exit still closes the explicit session and
	// keyed route, after which the same host can execute another scenario.
	fixture := sharedGuardProcess(t)
	t.Run("early-exit", func(t *testing.T) {
		dir := newSharedGuardScenario(t, sharedGuardSingleStepFactoryConfig(nil))
		session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{provider: sharedGuardFixedProviderOutput("COMPLETE")})
		_, _, _ = readSharedGuardSession(t, session)
	})
	if got := fixture.router.routeCount(); got != 0 {
		t.Fatalf("routes after early-exit cleanup = %d, want zero", got)
	}
	t.Run("reuse", func(t *testing.T) {
		dir := newSharedGuardScenario(t, sharedGuardSingleStepFactoryConfig(nil))
		seedGuardWork(t, dir, "reuse-after-early-exit", "guard-reuse-after-early-exit", "task")
		session := openSharedGuardSession(t, dir, sharedGuardRouteConfig{provider: sharedGuardFixedProviderOutput("COMPLETE")})
		supportWaitForGuardTerminal(t, session)
		_, listed, _ := readSharedGuardSession(t, session)
		assertGuardWorkState(t, listed, "guard-reuse-after-early-exit", "complete")
	})
	if got := fixture.router.routeCount(); got != 0 {
		t.Fatalf("routes after host reuse cleanup = %d, want zero", got)
	}
}

const sharedGuardSingleStepTransition = "process"

func sharedGuardSingleStepFactoryConfig(inputGuards []map[string]any) map[string]any {
	input := map[string]any{"workType": "task", "state": "init"}
	if inputGuards != nil {
		input["guards"] = inputGuards
	}
	return map[string]any{
		"name": "single-step-guard-case",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker"}},
		"workstations": []map[string]any{{
			"name":      sharedGuardSingleStepTransition,
			"worker":    "worker",
			"inputs":    []map[string]any{input},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func seedGuardWork(t *testing.T, dir, name, workID, workType string) {
	t.Helper()
	seedGuardWorkWithOptionalRelation(t, dir, name, workID, workType, nil)
}

func seedGuardWorkWithRelation(t *testing.T, dir, name, workID, workType string, relation work.Relation) {
	t.Helper()
	seedGuardWorkWithOptionalRelation(t, dir, name, workID, workType, &relation)
}

func seedGuardWorkWithOptionalRelation(t *testing.T, dir, name, workID, workType string, relation *work.Relation) {
	t.Helper()
	request := work.SubmitRequest{
		Name:       name,
		WorkID:     workID,
		WorkTypeID: workType,
		Payload:    []byte(fmt.Sprintf(`{"case":"%s"}`, name)),
	}
	if relation != nil {
		request.Relations = []work.Relation{*relation}
	}
	testutil.WriteSeedRequest(t, dir, request)
}

func signaledSharedGuardProviderOutput(content string, started, completed chan struct{}) sharedGuardCommandResponder {
	var startOnce sync.Once
	var completedOnce sync.Once
	return func(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		startOnce.Do(func() { close(started) })
		result, err := sharedGuardFixedProviderOutput(content)(ctx, request)
		completedOnce.Do(func() { close(completed) })
		return result, err
	}
}

func sharedGuardProviderTimeout(stage string) sharedGuardCommandResponder {
	return func(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		deadline, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		<-deadline.Done()
		return platformprocess.CommandResult{Stderr: []byte(stage)}, deadline.Err()
	}
}

func guardDispatchResponseEventsForTransition(t testing.TB, events []factoryapi.FactoryEvent, transitionID string) []factoryapi.FactoryEvent {
	t.Helper()
	var matches []factoryapi.FactoryEvent
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode guard dispatch response %q: %v", event.Id, err)
		}
		if payload.TransitionId == transitionID {
			matches = append(matches, event)
		}
	}
	return matches
}

func assertGuardResponseOutcome(t testing.TB, event factoryapi.FactoryEvent, want factoryapi.WorkOutcome, diagnostic string) {
	t.Helper()
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode guard response %q: %v", event.Id, err)
	}
	if payload.Outcome != want {
		t.Fatalf("guard response outcome = %q, want %q; payload=%#v", payload.Outcome, want, payload)
	}
	if diagnostic != "" {
		assertGuardResponseDiagnostic(t, payload, diagnostic)
	}
}

func assertGuardResponseDiagnostic(t testing.TB, payload factoryapi.DispatchResponseEventPayload, diagnostic string) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal guard response diagnostic: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(raw)), strings.ToLower(diagnostic)) {
		t.Fatalf("guard response diagnostic = %s, want substring %q", raw, diagnostic)
	}
}

func assertGuardWorkCounts(t testing.TB, listed factoryapi.ListWorkResponse, wantTerminal, wantFailed int) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != wantTerminal {
		t.Fatalf("terminal task count = %d, want %d; listed=%#v", got, wantTerminal, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "failed")); got != wantFailed {
		t.Fatalf("failed task count = %d, want %d; listed=%#v", got, wantFailed, listed)
	}
}

func assertGuardWorkState(t testing.TB, listed factoryapi.ListWorkResponse, workID, wantState string) {
	t.Helper()
	for _, item := range listed.Results {
		if item.WorkId == nil || *item.WorkId != workID {
			continue
		}
		if item.State == nil || item.State.Name != wantState {
			t.Fatalf("Work %q state = %#v, want %q; item=%#v", workID, item.State, wantState, item)
		}
		return
	}
	t.Fatalf("listed Work does not contain %q: %#v", workID, listed.Results)
}

func assertGuardSessionEventsBelongTo(t testing.TB, sessionID string, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("session %s observed event for %q", sessionID, *event.Context.SessionId)
		}
	}
}

func assertGuardSessionSequences(t testing.TB, sessionID string, events []factoryapi.FactoryEvent) {
	t.Helper()
	assertGuardSessionEventsBelongTo(t, sessionID, events)
	previous := -1
	for _, event := range events {
		if event.Context.SessionSequence == nil {
			continue
		}
		if *event.Context.SessionSequence < previous {
			t.Fatalf("session %s event sequence = %d after %d", sessionID, *event.Context.SessionSequence, previous)
		}
		previous = *event.Context.SessionSequence
	}
}

func assertGuardSessionEventOrdering(t testing.TB, sessionID string, events []factoryapi.FactoryEvent, prerequisiteTransition, joinedTransition, joinedWorkID string) {
	t.Helper()
	assertGuardSessionSequences(t, sessionID, events)
	prerequisiteResponse := -1
	joinedRequest := -1
	joinedResponse := -1
	joinedTerminalState := -1
	for index, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatalf("decode ordered dispatch request: %v", err)
			}
			if payload.TransitionId == joinedTransition && joinedRequest == -1 {
				joinedRequest = index
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode ordered dispatch response: %v", err)
			}
			switch payload.TransitionId {
			case prerequisiteTransition:
				if prerequisiteResponse == -1 {
					prerequisiteResponse = index
				}
			case joinedTransition:
				if joinedResponse == -1 {
					joinedResponse = index
				}
			}
		case factoryapi.FactoryEventTypeWorkStateChange:
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err != nil {
				t.Fatalf("decode ordered Work state change: %v", err)
			}
			if payload.WorkId == joinedWorkID && payload.ToState == "matched" && joinedTerminalState == -1 {
				joinedTerminalState = index
			}
		}
	}
	if prerequisiteResponse == -1 || joinedRequest == -1 || joinedResponse == -1 {
		t.Fatalf("ordered guard events missing prerequisite/join facts: prerequisiteResponse=%d joinedRequest=%d joinedResponse=%d events=%#v", prerequisiteResponse, joinedRequest, joinedResponse, events)
	}
	if prerequisiteResponse >= joinedRequest {
		t.Fatalf("prerequisite response index = %d, want before joined request index %d", prerequisiteResponse, joinedRequest)
	}
	if joinedRequest >= joinedResponse {
		t.Fatalf("joined request index = %d, want before joined response index %d", joinedRequest, joinedResponse)
	}
	if joinedTerminalState != -1 && joinedResponse >= joinedTerminalState {
		t.Fatalf("joined response index = %d, want before terminal Work state index %d", joinedResponse, joinedTerminalState)
	}
}

func waitForGuardDispatchResponse(t testing.TB, session *sharedGuardSession, transitionID string) {
	t.Helper()
	// The controlled command edge can return before the public Factory Event
	// ledger commits its dispatch response. The public API exposes no readiness
	// channel for that projection, so this bounded first-match poll is an
	// observation barrier, not a workflow delay.
	deadline := time.NewTimer(sharedGuardFixtureShutdownTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if len(guardDispatchResponseEventsForTransition(t, support.GetFactoryEventsForSessionAt(t, session.fixture.baseURL, session.sessionID), transitionID)) > 0 {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s dispatch response", transitionID)
		}
	}
}

func waitForGuardFactoryEvent(t testing.TB, session *sharedGuardSession, eventType factoryapi.FactoryEventType, label string) {
	t.Helper()
	// The runtime edge and the retained public Factory Event projection are
	// separate completion boundaries. No public readiness event exists for an
	// arbitrary event kind; return on the first committed match and reserve the
	// deadline only for a missing edge or cancellation.
	deadline := time.NewTimer(sharedGuardFixtureShutdownTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, event := range support.GetFactoryEventsForSessionAt(t, session.fixture.baseURL, session.sessionID) {
			if event.Type == eventType {
				return
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s Factory Event", label)
		}
	}
}

func waitForGuardWorkState(t testing.TB, session *sharedGuardSession, workID, wantState string) {
	t.Helper()
	// A Worker response edge can complete before the public Work read model
	// applies its state change. The read API has no readiness channel; this
	// bounded first-match poll is the projection observation barrier, not a
	// delay inserted into the Factory workflow.
	deadline := time.NewTimer(sharedGuardFixtureShutdownTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		listed := support.GetJSON[factoryapi.ListWorkResponse](t, strings.TrimSuffix(session.fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(session.sessionID)+"/work")
		for _, item := range listed.Results {
			if item.WorkId != nil && *item.WorkId == workID && item.State != nil && item.State.Name == wantState {
				return
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for Work %q to reach %q", workID, wantState)
		}
	}
}

func postSharedGuardOpenRequest(t testing.TB, baseURL, folderPath string) (int, []byte) {
	t.Helper()
	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{FolderPath: folderPath})
	if err != nil {
		t.Fatalf("marshal guard open request: %v", err)
	}
	response, err := http.Post(strings.TrimSuffix(baseURL, "/")+"/factory-sessions", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST guard open request: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read guard open response: %v", err)
	}
	return response.StatusCode, body
}

func assertGuardValidationDiagnostic(t testing.TB, body []byte, want string) {
	t.Helper()
	diagnostic := strings.ToLower(string(body))
	if !strings.Contains(diagnostic, strings.ToLower(want)) && !strings.Contains(diagnostic, "validation") && !strings.Contains(diagnostic, "invalid") && !strings.Contains(diagnostic, "unsupported") {
		t.Fatalf("guard validation body = %q, want %q or validation diagnostic", body, want)
	}
}

func assertNoLiveGuardSession(t testing.TB, baseURL, factoryDir string) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, strings.TrimSuffix(baseURL, "/")+"/factory-sessions?scope=live")
	for _, summary := range listed.Sessions {
		if filepath.Clean(summary.FolderPath) == filepath.Clean(factoryDir) {
			t.Fatalf("rejected guard Factory appeared in live sessions: %#v", summary)
		}
	}
}

func upsertSharedGuardWorkRequest(t testing.TB, session *sharedGuardSession, request factoryapi.WorkRequest) factoryapi.UpsertWorkRequestResponse {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal guard Work Request: %v", err)
	}
	endpoint := strings.TrimSuffix(session.fixture.baseURL, "/") + "/factory-sessions/" + url.PathEscape(session.sessionID) + "/work-requests/" + url.PathEscape(request.RequestId)
	httpRequest, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build guard Work Request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("PUT guard Work Request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("PUT guard Work Request status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode guard Work Request response: %v", err)
	}
	return result
}
