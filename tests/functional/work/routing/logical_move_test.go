package routing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	workservice "github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const logicalMoveRouterWorkstation = "router"

// TestSharedProcessWorkRouting establishes the logical-move executable spine.
// Each child owns one explicit Factory Session while the package fixture keeps
// the root-built service-mode process and controlled command edge shared.
func TestSharedProcessWorkRouting(t *testing.T) {
	fixture := ensureWorkRoutingPackageFixture(t)

	t.Run("LogicalMove/CompletesWithoutWorkerDispatch", func(t *testing.T) {
		runLogicalMoveCompletesWithoutWorkerDispatch(t, fixture)
	})
	t.Run("LogicalMove/PreservesWorkPayloadAndLineage", func(t *testing.T) {
		runLogicalMovePreservesWorkPayloadAndLineage(t, fixture)
	})
	t.Run("LogicalMove/MultipleOutputsCreatesEveryExpectedWork", func(t *testing.T) {
		runLogicalMoveMultipleOutputsCreatesEveryExpectedWork(t, fixture)
	})
	t.Run("ClassifierSuccess/RoutesEveryKnownDecision", func(t *testing.T) {
		runClassifierRoutesEveryKnownDecision(t, fixture)
	})
	t.Run("ClassifierFanout/PreservesPayload", func(t *testing.T) {
		runClassifierMultiOutputPreservesPayload(t, fixture)
	})
	t.Run("RoutingGuard/SelectorFailureClosesSession", func(t *testing.T) {
		runClassifierRoutingSelectorGuard(t, fixture)
	})
	t.Run("ClassifierFailure/UnknownAndMalformedDecision", func(t *testing.T) {
		runClassifierUnknownAndMalformedDecisionFailures(t, fixture)
	})
	t.Run("ClassifierFailure/ReworkFailureTerminatesWithoutCompletion", func(t *testing.T) {
		runClassifierReworkFailureTerminatesWithoutCompletion(t, fixture)
	})
	t.Run("ClassifierRejection/RoutesToFailedTerminal", func(t *testing.T) {
		runClassifierRejectionWithoutArcsRoutesToFailedTerminal(t, fixture)
	})
	t.Run("ClassifierRejection/RecordsDispatchFeedback", func(t *testing.T) {
		runClassifierRejectionWithoutArcsRecordsDispatchFeedback(t, fixture)
	})
	t.Run("ClassifierRejection/ReleasesResourcesForSubsequentWork", func(t *testing.T) {
		runClassifierRejectionWithoutArcsReleasesResourcesForSubsequentWork(t, fixture)
	})
}

// runLogicalMoveCompletesWithoutWorkerDispatch proves that Work submitted into
// a LOGICAL_MOVE workstation advances to the authored terminal output state
// without any worker or provider dispatch attributable to that routing step.
func runLogicalMoveCompletesWithoutWorkerDispatch(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	runner := newWorkRoutingScenarioCommandRunner("logical-move-completes", nil, nil)
	scenario := fixture.newScenario(t, "logical-move-completes", "logical_move_dir", runner)
	configureLogicalMoveWorkstation(t, scenario.factoryDir, logicalMoveRouterWorkstation)
	writeLogicalMoveSeedRequest(t, scenario.factoryDir, "logical-move-completes", "my-payload")
	scenario.open(t)

	_, listed, events := scenario.observe(t, 10*time.Second)
	if got := runner.callCount(); got != 0 {
		t.Fatalf("provider command count = %d, want 0 for workerless logical move", got)
	}
	assertWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "done"): 1,
		support.WorkCustomerLocation("task", "init"): 0,
	})
	assertNoInferenceResponses(t, events)
	assertLogicalMoveDispatchesCompleteWithoutProviderFailure(
		t,
		support.ObserveDispatchEvents(t, events),
		logicalMoveRouterWorkstation,
	)
}

// runLogicalMovePreservesWorkPayloadAndLineage proves that Work payload and
// observable lineage identity survive a LOGICAL_MOVE routing step so the next
// worker-backed workstation still receives the same customer content and Work ID.
func runLogicalMovePreservesWorkPayloadAndLineage(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	const (
		wantPayload = "logical-move-pipeline-payload"
		wantWorkID  = "logical-move-pipeline-work"
	)
	runner := newWorkRoutingScenarioCommandRunner(
		"logical-move-pipeline",
		[]platformprocess.CommandResult{{Stdout: support.CodexSuccessStdout("done COMPLETE")}},
		nil,
	)
	scenario := fixture.newScenario(t, "logical-move-pipeline", "logical_move_pipeline_dir", runner)
	configureLogicalMoveWorkstation(t, scenario.factoryDir, logicalMoveRouterWorkstation)
	writeLogicalMoveSeedRequest(t, scenario.factoryDir, wantWorkID, wantPayload)
	scenario.open(t)

	_, listed, events := scenario.observe(t, 10*time.Second)
	assertWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "done"):    1,
		support.WorkCustomerLocation("task", "init"):    0,
		support.WorkCustomerLocation("task", "staging"): 0,
	})

	calls := runner.requestsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("provider command count = %d, want 1 worker dispatch after logical move", len(calls))
	}
	admittedWork := workRoutingAdmissionWork(t, events, wantWorkID)
	if got := workRoutingPublicWorkText(admittedWork); got != wantPayload {
		t.Fatalf("WORK_REQUEST payload = %q, want %q preserved across logical move", got, wantPayload)
	}
	publicWork := getWorkRoutingWorkByID(t, scenario.fixture.baseURL, scenario.sessionID, wantWorkID)
	if got := support.StringPointerValue(publicWork.WorkId); got != wantWorkID {
		t.Fatalf("public Work ID = %q, want %q preserved across logical move", got, wantWorkID)
	}
	if got := support.StringPointerValue(publicWork.RequestId); got != wantWorkID+"-request" {
		t.Fatalf("public Work request ID = %q, want %q preserved across logical move", got, wantWorkID+"-request")
	}
	if got := support.StringPointerValue(publicWork.TraceId); got != wantWorkID+"-trace" {
		t.Fatalf("public Work trace ID = %q, want %q preserved across logical move", got, wantWorkID+"-trace")
	}
	if !support.HasWorkAtCustomerState(listed, wantWorkID, support.WorkCustomerLocation("task", "done")) {
		t.Fatalf(
			"listed Work %q missing at task:done, want same Work identity after logical move; listed=%#v",
			wantWorkID,
			listed,
		)
	}
	workerDispatches := support.ObserveDispatchEvents(t, events)
	workerDispatchFound := false
	for _, dispatch := range workerDispatches {
		if dispatch.Request.TransitionId != "process" {
			continue
		}
		if !support.DispatchObservationIncludesWork(dispatch, wantWorkID) {
			continue
		}
		workerDispatchFound = true
		break
	}
	if !workerDispatchFound {
		t.Fatalf("worker dispatch missing Work ID %q after logical move; dispatches=%#v", wantWorkID, workerDispatches)
	}
}

// runLogicalMoveMultipleOutputsCreatesEveryExpectedWork proves that a
// LOGICAL_MOVE workstation with multiple authored outputs creates Work in every
// expected downstream workType:state pair without dropping any fan-out branch.
func runLogicalMoveMultipleOutputsCreatesEveryExpectedWork(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	runner := newWorkRoutingScenarioCommandRunner("logical-move-fanout", nil, nil)
	scenario := fixture.newScenario(t, "logical-move-fanout", "logical_move_multi_output_dir", runner)
	configureLogicalMoveWorkstation(t, scenario.factoryDir, logicalMoveRouterWorkstation)
	writeLogicalMoveSeedRequest(t, scenario.factoryDir, "logical-move-fanout", "logical-move-fanout-payload")
	scenario.open(t)

	_, listed, events := scenario.observe(t, 10*time.Second)
	if got := runner.callCount(); got != 0 {
		t.Fatalf("provider command count = %d, want 0 for workerless logical move fan-out", got)
	}
	assertWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "init"):     0,
		support.WorkCustomerLocation("task", "done"):     1,
		support.WorkCustomerLocation("branch-a", "done"): 1,
		support.WorkCustomerLocation("branch-b", "done"): 1,
		support.WorkCustomerLocation("branch-a", "init"): 0,
		support.WorkCustomerLocation("branch-b", "init"): 0,
	})
	assertNoInferenceResponses(t, events)
	assertLogicalMoveDispatchesCompleteWithoutProviderFailure(
		t,
		support.ObserveDispatchEvents(t, events),
		logicalMoveRouterWorkstation,
	)
}

func writeLogicalMoveSeedRequest(t *testing.T, dir, workID, payload string) {
	t.Helper()
	testutil.WriteSeedRequest(t, dir, workservice.SubmitRequest{
		RequestID:  workID + "-request",
		WorkID:     workID,
		Name:       workID,
		WorkTypeID: "task",
		TraceID:    workID + "-trace",
		Payload:    []byte(payload),
	})
}

func workRoutingPublicWorkText(item factoryapi.Work) string {
	if item.Content != nil && len(*item.Content) > 0 {
		if part, err := (*item.Content)[0].AsWorkTextContentPart(); err == nil {
			return part.Text
		}
	}
	switch payload := item.Payload.(type) {
	case string:
		return payload
	case []byte:
		return string(payload)
	default:
		return ""
	}
}

func workRoutingAdmissionWork(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	workID string,
) factoryapi.Work {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_REQUEST event %q: %v", event.Id, err)
		}
		for _, work := range support.FactoryWorksValue(payload.Works) {
			if support.StringPointerValue(work.WorkId) == workID {
				return work
			}
		}
	}
	t.Fatalf("WORK_REQUEST event missing Work ID %q", workID)
	return factoryapi.Work{}
}

func configureLogicalMoveWorkstation(t *testing.T, dir, workstationName string) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("decode factory config: %v", err)
	}
	for _, raw := range config["workstations"].([]any) {
		workstation := raw.(map[string]any)
		if workstation["name"] == workstationName {
			workstation["type"] = "LOGICAL_MOVE"
			workstation["worker"] = ""
		}
	}
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("encode factory config: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory config: %v", err)
	}

	workstationConfigPath := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	if err := os.WriteFile(workstationConfigPath, []byte("---\ntype: LOGICAL_MOVE\n---\n"), 0o644); err != nil {
		t.Fatalf("write logical workstation config: %v", err)
	}
}

func assertWorkCustomerStates(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	wants map[string]int,
) {
	t.Helper()
	for location, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Fatalf("%s work count = %d, want %d; listed=%#v", location, got, want, listed)
		}
	}
}

func assertNoInferenceResponses(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		t.Fatalf("factory event %q emitted INFERENCE_RESPONSE, want no provider invocation for logical move", event.Id)
	}
}

func assertLogicalMoveDispatchesCompleteWithoutProviderFailure(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	workstation string,
) {
	t.Helper()

	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != workstation {
			continue
		}
		if dispatch.Response == nil {
			continue
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf(
				"dispatch %q at logical-move workstation %q outcome = %s, want ACCEPTED workerless completion",
				dispatch.DispatchID,
				workstation,
				dispatch.Response.Outcome,
			)
		}
		if dispatch.Response.ProviderFailure != nil {
			t.Fatalf(
				"dispatch %q at logical-move workstation %q providerFailure = %#v, want no provider invocation",
				dispatch.DispatchID,
				workstation,
				dispatch.Response.ProviderFailure,
			)
		}
	}
}
