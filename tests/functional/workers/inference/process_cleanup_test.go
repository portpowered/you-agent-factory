package inference_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestProcessGoneReconciliationThroughRootProcess proves the customer-visible
// failure and route-release outcome through the public root composition. Real
// executable, pipe, and descendant-process termination remain in the platform
// and integration lanes.
func TestProcessGoneReconciliationThroughRootProcess(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	const (
		workID  = "work-process-gone-functional"
		traceID = "trace-process-gone-functional"
	)
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte("deterministic process gone reconciliation"),
	})

	observerSeen := make(chan struct{}, 1)
	session, listed, events := runSharedInferenceFactoryToCompletion(t, dir, sharedInferenceScenario{
		scriptRunner: processGoneFunctionalCommandRunner{observerSeen: observerSeen},
	}, 20*time.Second)
	select {
	case <-observerSeen:
	default:
		t.Fatal("root script command did not receive a process lifecycle observer")
	}

	assertProcessCleanupListedWorkIdentity(t, listed, "failed", workID, "task", traceID)
	if session.Runtime.Progress.Categories.Failed != 1 || session.Runtime.Progress.Categories.Terminal != 0 {
		t.Fatalf(
			"session progress categories = %+v, want one PROCESS_GONE failure and no successful terminal Work",
			session.Runtime.Progress.Categories,
		)
	}
	if session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf("session processing count = %d, want zero after route release", session.Runtime.Progress.Categories.Processing)
	}
	assertProcessCleanupDispatchOutcomeSequence(t, events, factoryapi.WorkOutcomeFailed,
		"Workers workstation process exited before dispatch completion")
}

// processGoneFunctionalCommandRunner is a root.BuildProcess edge. It preserves
// the platform command contract while deterministically reporting the lifecycle
// facts that produce the customer-visible PROCESS_GONE result.
type processGoneFunctionalCommandRunner struct {
	observerSeen chan struct{}
}

func (runner processGoneFunctionalCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return runner.RunStreaming(ctx, request, nil)
}

func (runner processGoneFunctionalCommandRunner) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	_ platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	if observer := request.ProcessLifecycleObserver; observer != nil {
		if runner.observerSeen != nil {
			select {
			case runner.observerSeen <- struct{}{}:
			default:
			}
		}
		observer.ProcessStarted(platformprocess.ProcessInfo{PID: 1})
		observer.ProcessExited(platformprocess.ProcessInfo{PID: 1})
		<-ctx.Done()
		return platformprocess.CommandResult{}, ctx.Err()
	}
	return platformprocess.CommandResult{}, errors.New("process exited")
}

func assertProcessCleanupDispatchOutcomeSequence(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	want factoryapi.WorkOutcome,
	wantError string,
) {
	t.Helper()
	responses := processCleanupDispatchResponses(t, events)
	if len(responses) != 1 {
		t.Fatalf("dispatch response count = %d, want one", len(responses))
	}
	if responses[0].Outcome != want {
		t.Errorf("dispatch response outcome = %s, want %s", responses[0].Outcome, want)
	}
	if responses[0].Error == nil || !strings.Contains(*responses[0].Error, wantError) {
		t.Errorf("dispatch error = %#v, want text %q", responses[0].Error, wantError)
	}
}

func assertProcessCleanupListedWorkIdentity(
	t *testing.T,
	response factoryapi.ListWorkResponse,
	stateName, workID, workType, traceID string,
) {
	t.Helper()
	for _, item := range response.Results {
		if item.State == nil || item.State.Name != stateName {
			continue
		}
		if item.WorkId == nil || *item.WorkId != workID {
			t.Errorf("listed Work ID = %#v, want %q", item.WorkId, workID)
		}
		if item.WorkTypeName == nil || *item.WorkTypeName != workType {
			t.Errorf("listed Work type = %#v, want %q", item.WorkTypeName, workType)
		}
		if item.TraceId == nil || *item.TraceId != traceID {
			t.Errorf("listed Work trace ID = %#v, want %q", item.TraceId, traceID)
		}
		return
	}
	t.Fatalf("listed Work has no item in state %q: %#v", stateName, response.Results)
}

func processCleanupDispatchResponses(t *testing.T, events []factoryapi.FactoryEvent) []factoryapi.DispatchResponseEventPayload {
	t.Helper()
	var responses []factoryapi.DispatchResponseEventPayload
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		responses = append(responses, payload)
	}
	return responses
}
