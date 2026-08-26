package acp_test

import (
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var btrcACPEventOrder = []factoryapi.FactoryEventType{
	factoryapi.FactoryEventTypeRunRequest,
	factoryapi.FactoryEventTypeInitialStructureRequest,
	factoryapi.FactoryEventTypeSessionStarted,
	factoryapi.FactoryEventTypeFactoryStateResponse,
	factoryapi.FactoryEventTypeWorkRequest,
	factoryapi.FactoryEventTypeDispatchRequest,
	factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation,
	factoryapi.FactoryEventTypeModelRequest,
	factoryapi.FactoryEventTypeModelResponse,
	factoryapi.FactoryEventTypeAgentRunResponse,
	factoryapi.FactoryEventTypeDispatchResponse,
}

const btrcACPCompletionCeiling = 20 * time.Second

func TestBTRCP0ACPTargetSuccessCharacterization(t *testing.T) {
	t.Parallel()
	run := runBTRCP0ACPTarget(t, "1")
	if run.starts != 1 {
		t.Fatalf("ACP process starts = %d, want exactly one", run.starts)
	}
	assertBTRCACPEventOrder(t, run.events)
	workID, dispatchID := assertBTRCACPDispatch(t, run.events, factoryapi.WorkOutcomeAccepted, "done")
	assertBTRCACPProviderSession(t, run.events, factoryapi.InferenceOutcomeSucceeded)
	assertBTRCACPResponseTerminal(t, run.responseEvents, "COMPLETED")
	assertBTRCACPCompletedTarget(t, run.session, run.listed, workID, dispatchID)
}

func TestBTRCP0ACPTargetProtocolFailureCharacterization(t *testing.T) {
	t.Parallel()
	run := runBTRCP0ACPTarget(t, "fail")
	if run.starts != 1 {
		t.Fatalf("ACP process starts = %d, want exactly one", run.starts)
	}
	assertBTRCACPEventOrder(t, run.events)
	workID, dispatchID := assertBTRCACPDispatch(t, run.events, factoryapi.WorkOutcomeFailed, "failed")
	assertBTRCACPProviderSession(t, run.events, factoryapi.InferenceOutcomeFailed)
	assertBTRCACPFailureDetail(t, run.events)
	assertBTRCACPResponseTerminal(t, run.responseEvents, "FAILED")
	assertBTRCACPFailedTarget(t, run.session, run.listed, workID, dispatchID)
}

type btrcACPTargetRun struct {
	session        factoryapi.FactorySession
	listed         factoryapi.ListWorkResponse
	events         []factoryapi.FactoryEvent
	responseEvents []factoryapi.FactoryResponseEvent
	starts         int32
}

func runBTRCP0ACPTarget(t *testing.T, mode string) btrcACPTargetRun {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP target characterization"}`))
	writeACPWorker(t, dir, "cursor-acp")
	fixture := functionalACPFixture(mode)

	var starts atomic.Int32
	// The helper ACP process is the injected command/protocol edge, so it has
	// no in-process completion callback for this test to await. The root-built
	// continuous process is positively synchronized by the public terminal
	// Factory Session observation inside this helper; this ceiling is only a
	// bounded failure guard for a broken helper or runtime, not a sleep/polling
	// synchronization mechanism.
	session, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts, fixture),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, btrcACPCompletionCeiling)
	return btrcACPTargetRun{session: session, listed: listed, events: events, responseEvents: responseEvents, starts: starts.Load()}
}

func assertBTRCACPEventOrder(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	got := make([]factoryapi.FactoryEventType, len(events))
	for index, event := range events {
		got[index] = event.Type
		if event.Context.Sequence != index {
			t.Fatalf("Factory event[%d] sequence = %d, want %d", index, event.Context.Sequence, index)
		}
		if strings.TrimSpace(event.Id) == "" {
			t.Fatalf("Factory event[%d] id is blank", index)
		}
	}
	if !reflect.DeepEqual(got, btrcACPEventOrder) {
		t.Fatalf("ACP canonical event order = %v, want %v", got, btrcACPEventOrder)
	}
}

func assertBTRCACPDispatch(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantOutcome factoryapi.WorkOutcome,
	wantState string,
) (string, string) {
	t.Helper()
	var workID, requestID, dispatchID, workerSessionID string
	responseCount := 0
	var response factoryapi.DispatchResponseEventPayload
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			if event.Context.WorkIds == nil || len(*event.Context.WorkIds) != 1 {
				t.Fatalf("WORK_REQUEST work ids = %#v, want one", event.Context.WorkIds)
			}
			workID = (*event.Context.WorkIds)[0]
			if event.Context.RequestId == nil || strings.TrimSpace(*event.Context.RequestId) == "" {
				t.Fatal("WORK_REQUEST request id is blank")
			}
			requestID = *event.Context.RequestId
		case factoryapi.FactoryEventTypeDispatchRequest,
			factoryapi.FactoryEventTypeModelRequest,
			factoryapi.FactoryEventTypeModelResponse:
			if event.Context.DispatchId == nil || strings.TrimSpace(*event.Context.DispatchId) == "" {
				t.Fatalf("%s dispatch id = %#v, want non-empty", event.Type, event.Context.DispatchId)
			}
			if dispatchID == "" {
				dispatchID = *event.Context.DispatchId
			} else if dispatchID != *event.Context.DispatchId {
				t.Fatalf("%s dispatch id = %q, want %q", event.Type, *event.Context.DispatchId, dispatchID)
			}
			if event.Context.RequestId == nil || *event.Context.RequestId != requestID {
				t.Fatalf("%s request id = %#v, want %q", event.Type, event.Context.RequestId, requestID)
			}
		case factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation:
			if event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID {
				t.Fatalf("association dispatch id = %#v, want %q", event.Context.DispatchId, dispatchID)
			}
			association, err := event.Payload.AsDispatchWorkerSessionAssociationEventPayload()
			if err != nil {
				t.Fatalf("decode Worker Session association: %v", err)
			}
			workerSessionID = association.WorkerSessionId
			if strings.TrimSpace(workerSessionID) == "" {
				t.Fatal("Worker Session association id is blank")
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			responseCount++
			if event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID {
				t.Fatalf("DISPATCH_RESPONSE dispatch id = %#v, want %q", event.Context.DispatchId, dispatchID)
			}
			var err error
			response, err = event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode DISPATCH_RESPONSE: %v", err)
			}
		}
	}
	if workID == "" || dispatchID == "" || workerSessionID == "" || responseCount != 1 {
		t.Fatalf("ACP dispatch correlation = work:%q dispatch:%q workerSession:%q responses:%d, want one complete dispatch", workID, dispatchID, workerSessionID, responseCount)
	}
	if response.Outcome != wantOutcome || response.OutputWork == nil || len(*response.OutputWork) != 1 {
		t.Fatalf("ACP dispatch response = %#v, want %s with one output Work", response, wantOutcome)
	}
	outputWork := (*response.OutputWork)[0]
	if outputWork.WorkId == nil || *outputWork.WorkId != workID || outputWork.State == nil || outputWork.State.Name != wantState {
		t.Fatalf("ACP output Work = %#v, want %q in %s", outputWork, workID, wantState)
	}
	if wantOutcome == factoryapi.WorkOutcomeAccepted && (response.Output == nil || !strings.Contains(*response.Output, "execution COMPLETE")) {
		t.Fatalf("ACP successful dispatch output = %#v, want the provider completion text", response.Output)
	}
	return workID, dispatchID
}

func assertBTRCACPProviderSession(t *testing.T, events []factoryapi.FactoryEvent, wantOutcome factoryapi.InferenceOutcome) {
	t.Helper()
	count := 0
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		count++
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil {
			t.Fatalf("decode MODEL_RESPONSE: %v", err)
		}
		if payload.Outcome != wantOutcome {
			t.Fatalf("MODEL_RESPONSE outcome = %q, want %q", payload.Outcome, wantOutcome)
		}
		if payload.ProviderSession == nil || payload.ProviderSession.Provider == nil || payload.ProviderSession.Id == nil {
			t.Fatalf("MODEL_RESPONSE provider session = %#v, want cursor-acp/acp-session-functional-1", payload.ProviderSession)
		}
		if *payload.ProviderSession.Provider != "cursor-acp" || *payload.ProviderSession.Id != "acp-session-functional-1" {
			t.Fatalf("MODEL_RESPONSE provider session = %#v, want cursor-acp/acp-session-functional-1", payload.ProviderSession)
		}
	}
	if count != 1 {
		t.Fatalf("MODEL_RESPONSE count = %d, want exactly one terminal provider observation", count)
	}
}

func assertBTRCACPFailureDetail(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil {
			t.Fatalf("decode failed MODEL_RESPONSE: %v", err)
		}
		if payload.FailureDetail == nil || payload.FailureDetail.Reason != factoryapi.WorkFailureTypeUnknown {
			t.Fatalf("ACP failure detail = %#v, want typed unknown failure", payload.FailureDetail)
		}
		if strings.TrimSpace(payload.FailureDetail.Message) == "" || len(payload.FailureDetail.Message) > 512 {
			t.Fatalf("ACP failure detail message = %q, want bounded non-empty diagnostic", payload.FailureDetail.Message)
		}
		return
	}
	t.Fatal("failed ACP run omitted MODEL_RESPONSE failure detail")
}

func assertBTRCACPResponseTerminal(t *testing.T, events []factoryapi.FactoryResponseEvent, wantPhase string) {
	t.Helper()
	if wantPhase == string(factoryapi.FactoryResponseEventPhaseFailed) {
		for _, event := range events {
			if event.Provenance.Provider != "cursor-acp" || event.Kind != "ERROR" || event.Phase != factoryapi.FactoryResponseEventPhaseFailed {
				continue
			}
			if event.ProviderSessionRef == nil || *event.ProviderSessionRef != "acp-session-functional-1" {
				t.Fatalf("ACP failed response ProviderSessionRef = %#v, want acp-session-functional-1", event.ProviderSessionRef)
			}
			payload, err := event.Payload.AsFactoryResponseEventErrorPayload()
			if err != nil {
				t.Fatalf("decode ACP failed response: %v", err)
			}
			if strings.TrimSpace(payload.Code) == "" || strings.TrimSpace(payload.Message) == "" {
				t.Fatalf("ACP failed response payload = %#v, want code and message", payload)
			}
			return
		}
		t.Fatalf("ACP failed response event missing; all=%#v", events)
	}

	terminal := 0
	for _, event := range events {
		if event.Provenance.Provider != "cursor-acp" || event.Kind != factoryapi.FactoryResponseEventKindRun {
			continue
		}
		if event.Phase == factoryapi.FactoryResponseEventPhaseCompleted || event.Phase == factoryapi.FactoryResponseEventPhaseFailed || event.Phase == factoryapi.FactoryResponseEventPhaseCanceled {
			terminal++
			if string(event.Phase) != wantPhase {
				t.Fatalf("ACP terminal response phase = %q, want %q", event.Phase, wantPhase)
			}
			if event.ProviderSessionRef == nil || *event.ProviderSessionRef != "acp-session-functional-1" {
				t.Fatalf("ACP terminal response ProviderSessionRef = %#v, want acp-session-functional-1", event.ProviderSessionRef)
			}
		}
	}
	if terminal != 1 {
		t.Fatalf("ACP terminal response events = %d, want exactly one %s terminal publication; all=%#v", terminal, wantPhase, events)
	}
}

func assertBTRCACPCompletedTarget(t *testing.T, session factoryapi.FactorySession, listed factoryapi.ListWorkResponse, workID, dispatchID string) {
	t.Helper()
	if session.Id == "" || session.Runtime.Status != factoryapi.FactorySessionStatusIDLE || session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 || session.Runtime.Progress.Categories.Initial != 0 || session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf("ACP successful target session = %#v, want IDLE with one terminal Work and no failures/in-flight work", session.Runtime)
	}
	assertBTRCACPListedWork(t, listed, workID, "done")
	if strings.TrimSpace(dispatchID) == "" {
		t.Fatal("successful ACP dispatch id is blank")
	}
}

func assertBTRCACPFailedTarget(t *testing.T, session factoryapi.FactorySession, listed factoryapi.ListWorkResponse, workID, dispatchID string) {
	t.Helper()
	if session.Id == "" || session.Runtime.Status != factoryapi.FactorySessionStatusIDLE || session.Runtime.Progress.Categories.Terminal != 0 || session.Runtime.Progress.Categories.Failed != 1 || session.Runtime.Progress.Categories.Initial != 0 || session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf("ACP failed target session = %#v, want IDLE with one failed Work and no non-terminal work", session.Runtime)
	}
	assertBTRCACPListedWork(t, listed, workID, "failed")
	if strings.TrimSpace(dispatchID) == "" {
		t.Fatal("failed ACP dispatch id is blank")
	}
}

func assertBTRCACPListedWork(t *testing.T, listed factoryapi.ListWorkResponse, workID, state string) {
	t.Helper()
	if len(listed.Results) != 1 || listed.Results[0].WorkId == nil || *listed.Results[0].WorkId != workID || listed.Results[0].State == nil || listed.Results[0].State.Name != state {
		t.Fatalf("ACP listed Work = %#v, want one %q in %s", listed.Results, workID, state)
	}
}
