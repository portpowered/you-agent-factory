package routing

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertWorkAtCustomerStates(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for location, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Errorf("%s Work count = %d, want %d", location, got, want)
		}
	}
}

func assertQuiescentSession(t *testing.T, session factoryapi.FactorySession, wantTerminal, wantFailed int) {
	t.Helper()
	categories := session.Runtime.Progress.Categories
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
