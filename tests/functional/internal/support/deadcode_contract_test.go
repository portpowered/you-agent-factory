package support

import (
	"context"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestAcceptedCommandResults_ReturnsRequestedCompleteResponses(t *testing.T) {
	results := AcceptedCommandResults(3)

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	for i, result := range results {
		if got := string(result.Stdout); got != "Done. COMPLETE" {
			t.Fatalf("results[%d].Stdout = %q, want %q", i, got, "Done. COMPLETE")
		}
	}
}

func TestProviderCommandRequestsForWorker_FiltersRecordedRequests(t *testing.T) {
	runner := testutil.NewProviderCommandRunner()
	requests := []workers.CommandRequest{
		{WorkerType: "planner"},
		{WorkerType: "executor"},
		{WorkerType: "planner"},
	}
	for _, request := range requests {
		if _, err := runner.Run(context.Background(), request); err != nil {
			t.Fatalf("runner.Run(%#v): %v", request, err)
		}
	}

	filtered := ProviderCommandRequestsForWorker(runner, "planner")

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	for i, request := range filtered {
		if request.WorkerType != "planner" {
			t.Fatalf("filtered[%d].WorkerType = %q, want %q", i, request.WorkerType, "planner")
		}
	}
}

func TestCountFactoryEvents_CountsMatchingEventTypes(t *testing.T) {
	events := []factoryapi.FactoryEvent{
		{Type: factoryapi.FactoryEventTypeDispatchRequest},
		{Type: factoryapi.FactoryEventTypeDispatchResponse},
		{Type: factoryapi.FactoryEventTypeDispatchRequest},
	}

	if got := CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchRequest); got != 2 {
		t.Fatalf("CountFactoryEvents(dispatch request) = %d, want 2", got)
	}
	if got := CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchResponse); got != 1 {
		t.Fatalf("CountFactoryEvents(dispatch response) = %d, want 1", got)
	}
}

func TestNewStaticSuccessCommandRunner_ReturnsFixedStdoutWithoutFailureFields(t *testing.T) {
	runner := NewStaticSuccessCommandRunner("script-output-ok")

	result, err := runner.Run(context.Background(), workers.CommandRequest{Command: "script-tool"})
	if err != nil {
		t.Fatalf("runner.Run: %v", err)
	}
	if got := string(result.Stdout); got != "script-output-ok" {
		t.Fatalf("result.Stdout = %q, want %q", got, "script-output-ok")
	}
	if got := string(result.Stderr); got != "" {
		t.Fatalf("result.Stderr = %q, want empty", got)
	}
	if result.ExitCode != 0 {
		t.Fatalf("result.ExitCode = %d, want 0", result.ExitCode)
	}
}
