package restart_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertBoardCLIWorkerSessionsForWork(
	t *testing.T,
	ctx context.Context,
	daemon *boardPersistenceDaemon,
	binaryPath, factoryDir, homeDir, workID string,
) {
	t.Helper()
	output, err := runBoardPersistenceCLI(ctx, binaryPath, factoryDir, homeDir, daemon.baseURL, "--json", "worker-sessions", "list", "--work-id", workID)
	if err != nil {
		t.Fatalf("you worker-sessions list --work-id %s after restart: %v\noutput:\n%s", workID, err, output)
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(bytes.TrimSpace(output), &response); err != nil {
		t.Fatalf("decode worker-sessions list --work-id %s: %v\noutput:\n%s", workID, err, output)
	}
	if len(response.Sessions) == 0 {
		t.Fatalf("worker-sessions list --work-id %s returned no historical attempts", workID)
	}
	for _, observation := range response.Sessions {
		if observation.State == factoryapi.WorkerSessionObservationStateRunning || observation.State == factoryapi.WorkerSessionObservationStateStarting {
			t.Fatalf("worker-sessions list --work-id %s returned non-terminal attempt %#v", workID, observation)
		}
	}
}

func assertBoardWork(t *testing.T, item factoryapi.Work, want boardPersistenceExpectedWork) {
	t.Helper()
	if item.Name != want.Name || support.StringPointerValue(item.WorkId) != want.WorkID || support.StringPointerValue(item.RequestId) != want.RequestID {
		t.Fatalf("Work identity = %#v, want name=%q workId=%q requestId=%q", item, want.Name, want.WorkID, want.RequestID)
	}
	if item.State == nil || item.State.Name != want.State || string(item.State.Type) != want.StateType {
		t.Fatalf("Work %q state = %#v, want %s/%s", want.WorkID, item.State, want.State, want.StateType)
	}
	if support.StringPointerValue(item.TraceId) != want.TraceID || support.StringPointerValue(item.CurrentChainingTraceId) != want.CurrentTraceID {
		t.Fatalf("Work %q lineage = trace=%q current=%q, want %q/%q", want.WorkID, support.StringPointerValue(item.TraceId), support.StringPointerValue(item.CurrentChainingTraceId), want.TraceID, want.CurrentTraceID)
	}
	if item.Content == nil || len(*item.Content) != 1 {
		t.Fatalf("Work %q content = %#v, want one text part", want.WorkID, item.Content)
	}
	part, err := (*item.Content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("Work %q content part = %#v, decode error=%v, want %q", want.WorkID, part, err, want.Content)
	}
	if want.WorkerOutput {
		assertBoardPersistenceWorkerOutput(t, part.Text, want.Content)
	} else if part.Text != want.Content {
		t.Fatalf("Work %q content part = %#v, want %q", want.WorkID, part, want.Content)
	}
	if want.RelationTarget == "" {
		if item.Relations != nil && len(*item.Relations) != 0 {
			t.Fatalf("Work %q relations = %#v, want none", want.WorkID, item.Relations)
		}
		return
	}
	if item.Relations == nil || len(*item.Relations) != 1 {
		t.Fatalf("Work %q relations = %#v, want one PARENT_CHILD relation", want.WorkID, item.Relations)
	}
	relation := (*item.Relations)[0]
	if relation.Type != factoryapi.RelationTypeParentChild || relation.SourceWorkName != want.Name || relation.TargetWorkId == nil || *relation.TargetWorkId != want.RelationTarget {
		t.Fatalf("Work %q relation = %#v, want source=%q target=%q PARENT_CHILD", want.WorkID, relation, want.Name, want.RelationTarget)
	}
}

func assertBoardPersistenceWorkerOutput(t *testing.T, got, sentinel string) {
	t.Helper()
	lines := strings.Split(got, "\n")
	if len(lines) < 2 || lines[0] != sentinel || lines[1] != "PASS" {
		t.Fatalf("worker output = %q, want exactly the sentinel and PASS worker lines", got)
	}

	suffix := lines[2:]
	if len(suffix) == 0 {
		return
	}
	if len(suffix) == 1 && suffix[0] == "coverage: [no statements]" {
		return
	}

	const coveragePrefix = "coverage: 0.0% of statements"
	if !strings.HasPrefix(suffix[0], coveragePrefix) {
		t.Fatalf("worker output = %q, want the sentinel/PASS lines plus a recognized Go coverage suffix", got)
	}
	coverageDetail := strings.TrimPrefix(suffix[0], coveragePrefix)
	if coverageDetail != "" && !strings.HasPrefix(coverageDetail, " in ") {
		t.Fatalf("worker output = %q, want Go coverage detail to use the standard ' in <package list>' form", got)
	}
	if strings.TrimSpace(coverageDetail) == "in" {
		t.Fatalf("worker output = %q, want a non-empty Go coverage package list", got)
	}
	for _, packageLine := range suffix[1:] {
		if !strings.Contains(packageLine, "github.com/portpowered/infinite-you/") {
			t.Fatalf("worker output = %q, want only the Go harness package-list suffix after coverage", got)
		}
	}
}

type boardPersistenceDispatchState struct {
	ID                 string
	WorkIDs            map[string]struct{}
	RequestEvents      int
	ResponseEvents     int
	InterruptedEvents  int
	ReconciledStatuses []factoryapi.FactoryDispatchStatus
	WorkerSessionIDs   []string
}

func waitForBoardActiveDispatch(t *testing.T, baseURL, workID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last map[string]boardPersistenceDispatchState
	var lastErr error
	for {
		states, err := readBoardDispatchStates(t.Context(), baseURL)
		if err == nil {
			last = states
			active := activeBoardDispatches(states, workID)
			if len(active) == 1 {
				return active[0].ID
			}
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for one active dispatch for Work %q; last=%#v, error=%v", workID, last, lastErr)
		}
	}
}

func waitForBoardRearmedDispatch(t *testing.T, baseURL, workID, oldDispatchID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last map[string]boardPersistenceDispatchState
	var lastErr error
	for {
		states, err := readBoardDispatchStates(t.Context(), baseURL)
		if err == nil {
			last = states
			old, oldFound := states[oldDispatchID]
			active := activeBoardDispatches(states, workID)
			if oldFound && old.InterruptedEvents == 1 && len(active) == 1 && active[0].ID != oldDispatchID {
				if active[0].RequestEvents != 1 {
					t.Fatalf("re-armed dispatch %q request events = %d, want exactly one", active[0].ID, active[0].RequestEvents)
				}
				return active[0].ID
			}
			if oldFound && old.InterruptedEvents > 1 {
				t.Fatalf("original dispatch %q interruption events = %d, want exactly one", oldDispatchID, old.InterruptedEvents)
			}
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for dispatch %q interruption and one re-armed dispatch for Work %q; last=%#v, error=%v", oldDispatchID, workID, last, lastErr)
		}
	}
}

func waitForBoardDispatchResponse(t *testing.T, baseURL, workID, dispatchID string, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last map[string]boardPersistenceDispatchState
	var lastErr error
	for {
		states, err := readBoardDispatchStates(t.Context(), baseURL)
		if err == nil {
			last = states
			if state, ok := states[dispatchID]; ok && state.ResponseEvents == 1 && len(activeBoardDispatches(states, workID)) == 0 {
				return
			}
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for dispatch %q response for Work %q; last=%#v, error=%v", dispatchID, workID, last, lastErr)
		}
	}
}

func waitForBoardDispatchStates(t *testing.T, baseURL string, timeout time.Duration) map[string]boardPersistenceDispatchState {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last map[string]boardPersistenceDispatchState
	var lastErr error
	for {
		states, err := readBoardDispatchStates(t.Context(), baseURL)
		if err == nil {
			last = states
			return states
		}
		lastErr = err
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out reading public Factory Event dispatch history; last=%#v, error=%v", last, lastErr)
		}
	}
}

func readBoardDispatchStates(ctx context.Context, baseURL string) (map[string]boardPersistenceDispatchState, error) {
	events, err := readBoardEvents(ctx, baseURL)
	if err != nil {
		return nil, err
	}
	states := make(map[string]boardPersistenceDispatchState)
	for _, event := range events {
		if event.Context.DispatchId == nil || strings.TrimSpace(*event.Context.DispatchId) == "" {
			continue
		}
		dispatchID := strings.TrimSpace(*event.Context.DispatchId)
		state := states[dispatchID]
		state.ID = dispatchID
		if event.Context.WorkIds != nil {
			if state.WorkIDs == nil {
				state.WorkIDs = make(map[string]struct{})
			}
			for _, workID := range *event.Context.WorkIds {
				if workID != "" {
					state.WorkIDs[workID] = struct{}{}
				}
			}
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, decodeErr := event.Payload.AsDispatchRequestEventPayload()
			if decodeErr != nil {
				return nil, fmt.Errorf("decode public dispatch request %q: %w", dispatchID, decodeErr)
			}
			state.RequestEvents++
			if state.WorkIDs == nil {
				state.WorkIDs = make(map[string]struct{})
			}
			for _, input := range payload.Inputs {
				if input.WorkId != "" {
					state.WorkIDs[input.WorkId] = struct{}{}
				}
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			if _, decodeErr := event.Payload.AsDispatchResponseEventPayload(); decodeErr != nil {
				return nil, fmt.Errorf("decode public dispatch response %q: %w", dispatchID, decodeErr)
			}
			state.ResponseEvents++
		case factoryapi.FactoryEventTypeDispatchInterrupted:
			if _, decodeErr := event.Payload.AsDispatchInterruptedEventPayload(); decodeErr != nil {
				return nil, fmt.Errorf("decode public dispatch interruption %q: %w", dispatchID, decodeErr)
			}
			state.InterruptedEvents++
		case factoryapi.FactoryEventTypeDispatchReconciled:
			payload, decodeErr := event.Payload.AsDispatchReconciledEventPayload()
			if decodeErr != nil {
				return nil, fmt.Errorf("decode public dispatch reconciliation %q: %w", dispatchID, decodeErr)
			}
			state.ReconciledStatuses = append(state.ReconciledStatuses, payload.ReconciledStatus)
		case factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation:
			payload, decodeErr := event.Payload.AsDispatchWorkerSessionAssociationEventPayload()
			if decodeErr != nil {
				return nil, fmt.Errorf("decode public Worker Session association %q: %w", dispatchID, decodeErr)
			}
			if payload.WorkerSessionId != "" {
				state.WorkerSessionIDs = append(state.WorkerSessionIDs, payload.WorkerSessionId)
			}
		}
		states[dispatchID] = state
	}
	return states, nil
}

func activeBoardDispatches(states map[string]boardPersistenceDispatchState, workID string) []boardPersistenceDispatchState {
	ids := make([]string, 0, len(states))
	for dispatchID, state := range states {
		if state.RequestEvents == 0 || !boardPersistenceDispatchIncludesWork(state, workID) || state.ResponseEvents > 0 || state.InterruptedEvents > 0 || len(state.ReconciledStatuses) > 0 {
			continue
		}
		ids = append(ids, dispatchID)
	}
	sort.Strings(ids)
	active := make([]boardPersistenceDispatchState, 0, len(ids))
	for _, dispatchID := range ids {
		active = append(active, states[dispatchID])
	}
	return active
}

func boardPersistenceDispatchIncludesWork(state boardPersistenceDispatchState, workID string) bool {
	_, ok := state.WorkIDs[workID]
	return ok
}
