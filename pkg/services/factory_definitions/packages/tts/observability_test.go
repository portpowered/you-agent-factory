package tts

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestClassifyInvocationWait_LoadingWhenActiveWorkRemains(t *testing.T) {
	outcome, failure := ClassifyInvocationWait(interfaces.FactoryWorldState{}, "req-1", true)
	if outcome != InvocationWaitOutcomeLoading {
		t.Fatalf("outcome = %q, want loading", outcome)
	}
	if failure != nil {
		t.Fatalf("failure = %#v, want nil while loading", failure)
	}
}

func TestClassifyInvocationWait_ModelNotReadyFailure(t *testing.T) {
	state := packagedTTSFailureWorldState(
		"req-tts",
		"work-tts",
		"model not available: required assets missing in managed cache",
	)
	outcome, failure := ClassifyInvocationWait(state, "req-tts", false)
	if outcome != InvocationWaitOutcomeModelNotReady {
		t.Fatalf("outcome = %q, want model_not_ready", outcome)
	}
	if failure == nil || failure.ErrorCode != InvocationErrorCodeModelNotReady {
		t.Fatalf("failure = %#v, want model-not-ready code", failure)
	}
	if failure.FailureClass != FailureClassModelNotReady {
		t.Fatalf("failure_class = %q, want %s", failure.FailureClass, FailureClassModelNotReady)
	}
}

func TestClassifyInvocationWait_GenerationFailure(t *testing.T) {
	state := packagedTTSFailureWorldState("req-tts", "work-tts", "omnivoice invoke failed: exit status 1")
	outcome, failure := ClassifyInvocationWait(state, "req-tts", false)
	if outcome != InvocationWaitOutcomeGenerationFailed {
		t.Fatalf("outcome = %q, want generation_failed", outcome)
	}
	if failure == nil || failure.ErrorCode != InvocationErrorCodeGenerationFailed {
		t.Fatalf("failure = %#v, want generation-failed code", failure)
	}
	if failure.FailureClass != FailureClassGenerationFailed {
		t.Fatalf("failure_class = %q, want %s", failure.FailureClass, FailureClassGenerationFailed)
	}
}

func TestIsModelNotReadyFailure_DetectsStableModelFailureEvidence(t *testing.T) {
	message := "model not available: required assets missing"
	if !isModelNotReadyFailure(message) {
		t.Fatalf("expected model-not-ready detection for %q", message)
	}
}

func TestIsPackagedFactory_MatchesBuiltInCatalogIdentity(t *testing.T) {
	if !IsPackagedFactory(&interfaces.FactoryConfig{Name: PackagedFactoryName}) {
		t.Fatal("expected factory name match")
	}
	if !IsPackagedFactory(&interfaces.FactoryConfig{Project: PackagedFactoryProject}) {
		t.Fatal("expected factory project match")
	}
	if IsPackagedFactory(&interfaces.FactoryConfig{Name: "alpha"}) {
		t.Fatal("unexpected packaged factory match for unrelated factory")
	}
}

func packagedTTSFailureWorldState(requestID, workID, failureMessage string) interfaces.FactoryWorldState {
	submitted := work.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: "task",
		State:      "init",
		TraceID:    requestID,
	}
	failed := submitted
	failed.State = "failed"
	failed.PlaceID = "task:failed"

	state := interfaces.FactoryWorldState{
		WorkRequestsByID:       make(map[string]interfaces.WorkRequestPayload),
		FailedWorkItemsByID:    make(map[string]work.FactoryWorkItem),
		FailureDetailsByWorkID: make(map[string]interfaces.FactoryWorldFailureDetail),
	}
	state.WorkRequestsByID[requestID] = interfaces.WorkRequestPayload{
		RequestID: requestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		WorkItems: []work.FactoryWorkItem{submitted},
	}
	state.FailedWorkItemsByID[workID] = failed
	state.FailureDetailsByWorkID[workID] = interfaces.FactoryWorldFailureDetail{
		WorkstationName: PackagedInvokeWorkstationName,
		WorkItem:        failed,
		FailureDetail:   &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureTypeUnknown, Message: failureMessage},
	}
	return state
}
