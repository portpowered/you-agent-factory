package ttsobservability

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPolicy_TTSObservability(t *testing.T) {
	t.Parallel()

	tts := NewService()
	if !tts.IsPackagedTTSFactory(&factorydefinitions.FactoryConfig{
		Name: factorydefinitions.PackagedTTSFactoryName,
	}) {
		t.Fatal("IsPackagedTTSFactory() = false, want true for packaged TTS factory")
	}

	wantLabel := factorydefinitions.DefaultTTSModelName + "/" + factorydefinitions.DefaultTTSBackendName
	if got := tts.TTSBackendRuntimeLabel(); got != wantLabel {
		t.Fatalf("TTSBackendRuntimeLabel() = %q, want %q", got, wantLabel)
	}

	outcome, failure := tts.ClassifyTTSInvocationWait(factorydefinitions.FactoryWorldState{}, "req-1", true)
	if outcome != factorydefinitions.TTSInvocationWaitOutcomeLoading {
		t.Fatalf("ClassifyTTSInvocationWait loading outcome = %q, want loading", outcome)
	}
	if failure != nil {
		t.Fatalf("ClassifyTTSInvocationWait loading failure = %#v, want nil", failure)
	}

	state := packagedTTSFailureWorldState(
		"req-tts",
		"work-tts",
		"model not available: required assets missing in managed cache",
	)
	outcome, failure = tts.ClassifyTTSInvocationWait(state, "req-tts", false)
	if outcome != factorydefinitions.TTSInvocationWaitOutcomeModelNotReady {
		t.Fatalf("ClassifyTTSInvocationWait outcome = %q, want model_not_ready", outcome)
	}
	if failure == nil || failure.ErrorCode != factorydefinitions.TTSInvocationErrorCodeModelNotReady {
		t.Fatalf("ClassifyTTSInvocationWait failure = %#v, want model-not-ready code", failure)
	}

	if !tts.IsTTSModelNotReadyFailure("model not available: required assets missing") {
		t.Fatal("IsTTSModelNotReadyFailure() = false, want true for model-not-ready evidence")
	}
}

func packagedTTSFailureWorldState(requestID, workID, failureMessage string) factorydefinitions.FactoryWorldState {
	submitted := work.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: "task",
		State:      "init",
		TraceID:    requestID,
	}
	failed := submitted
	failed.State = "failed"

	state := factorydefinitions.FactoryWorldState{
		WorkRequestsByID:       make(map[string]factorydefinitions.WorkRequestPayload),
		FailedWorkItemsByID:    make(map[string]work.FactoryWorkItem),
		FailureDetailsByWorkID: make(map[string]factorydefinitions.FactoryWorldFailureDetail),
	}
	state.WorkRequestsByID[requestID] = factorydefinitions.WorkRequestPayload{
		RequestID: requestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		WorkItems: []work.FactoryWorkItem{submitted},
	}
	state.FailedWorkItemsByID[workID] = failed
	state.FailureDetailsByWorkID[workID] = factorydefinitions.FactoryWorldFailureDetail{
		WorkstationName: factorydefinitions.PackagedTTSInvokeWorkstationName,
		WorkItem:        failed,
		FailureDetail:   &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureTypeUnknown, Message: failureMessage},
	}
	return state
}
