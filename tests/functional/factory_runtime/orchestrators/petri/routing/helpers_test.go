package routing

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func workIDAtCustomerState(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	location string,
	traceID string,
) (string, bool) {
	t.Helper()
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != location {
			continue
		}
		if item.TraceId == nil || *item.TraceId != traceID {
			continue
		}
		if item.WorkId == nil || *item.WorkId == "" {
			t.Fatalf("work at %s for trace %q missing workId", location, traceID)
		}
		return *item.WorkId, true
	}
	return "", false
}

func assertTraceAbsentAtCustomerState(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	location string,
	traceID string,
) {
	t.Helper()
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != location {
			continue
		}
		if item.TraceId != nil && *item.TraceId == traceID {
			t.Errorf("trace %q should not be at %s", traceID, location)
		}
	}
}

func assertFailedDispatchForWork(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	workID string,
) {
	t.Helper()

	for _, dispatch := range dispatches {
		if !support.DispatchObservationIncludesWork(dispatch, workID) {
			continue
		}
		if dispatch.Response == nil {
			continue
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeFailed {
			continue
		}
		return
	}
	t.Fatalf("no failed dispatch observation for work %q", workID)
}

func assertDispatchTransitionSequence(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	want []string,
) {
	t.Helper()
	observed := support.ObserveDispatchEvents(t, events)
	if len(observed) != len(want) {
		t.Fatalf("dispatch transition count = %d, want %d", len(observed), len(want))
	}
	for index, transition := range want {
		if got := observed[index].Request.TransitionId; got != transition {
			t.Errorf("dispatch transition[%d] = %q, want %q", index, got, transition)
		}
	}
}

func assertWorkAtCustomerStates(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for location, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Errorf("%s Work count = %d, want %d", location, got, want)
		}
	}
}

func assertQuiescentSession(t *testing.T, status factoryapi.StatusResponse, wantTerminal, wantFailed int) {
	t.Helper()
	categories := status.Categories
	if categories.Initial != 0 || categories.Processing != 0 {
		t.Errorf(
			"session still has in-progress Work: initial=%d processing=%d",
			categories.Initial,
			categories.Processing,
		)
	}
	if categories.Terminal != wantTerminal {
		t.Errorf("session terminal count = %d, want %d", categories.Terminal, wantTerminal)
	}
	if categories.Failed != wantFailed {
		t.Errorf("session failed count = %d, want %d", categories.Failed, wantFailed)
	}
}

func assertListedWorkStateTrace(
	t *testing.T,
	response factoryapi.ListWorkResponse,
	workType, state, traceID string,
) {
	t.Helper()
	for _, item := range response.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType || item.State == nil || item.State.Name != state {
			continue
		}
		if item.TraceId == nil || *item.TraceId != traceID {
			t.Errorf("%s:%s trace ID = %#v, want %q", workType, state, item.TraceId, traceID)
		}
		return
	}
	t.Errorf("listed Work missing %s:%s", workType, state)
}

func assertDispatchEventsReferenceTerminalWork(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	listed factoryapi.ListWorkResponse,
	terminalLocation string,
	traceIDs []string,
) {
	t.Helper()
	terminalWorkIDs := map[string]string{}
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != terminalLocation {
			continue
		}
		if item.TraceId == nil || item.WorkId == nil {
			continue
		}
		terminalWorkIDs[*item.TraceId] = *item.WorkId
	}
	for _, traceID := range traceIDs {
		workID, ok := terminalWorkIDs[traceID]
		if !ok {
			t.Errorf("missing terminal Work ID for trace %q", traceID)
			continue
		}
		referenced := false
		for _, dispatch := range support.ObserveDispatchEvents(t, events) {
			if support.DispatchObservationIncludesWork(dispatch, workID) {
				referenced = true
				break
			}
		}
		if !referenced {
			t.Errorf("dispatch events missing public Work ID %q for trace %q", workID, traceID)
		}
	}
}

func assertTerminalWorkCorrelatesToTraceIDs(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	terminalLocation string,
	traceIDs []string,
) {
	t.Helper()
	wants := make(map[string]bool, len(traceIDs))
	for _, traceID := range traceIDs {
		wants[traceID] = true
	}
	found := map[string]bool{}
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != terminalLocation {
			continue
		}
		if item.TraceId == nil || !wants[*item.TraceId] {
			t.Errorf("unexpected %s trace ID %#v", terminalLocation, item.TraceId)
			continue
		}
		if item.WorkId == nil || *item.WorkId == "" {
			t.Errorf("terminal Work at %s missing workId for trace %q", terminalLocation, *item.TraceId)
			continue
		}
		if found[*item.TraceId] {
			t.Errorf("duplicate terminal Work for trace %q at %s", *item.TraceId, terminalLocation)
			continue
		}
		found[*item.TraceId] = true
	}
	for traceID := range wants {
		if !found[traceID] {
			t.Errorf("listed Work missing %s trace %q", terminalLocation, traceID)
		}
	}
}
