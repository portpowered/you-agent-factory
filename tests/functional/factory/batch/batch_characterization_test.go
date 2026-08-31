package root_composition_test

import (
	"reflect"
	"sort"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	btrcBatchRequestID   = "btrc-batch-request"
	btrcBatchParentID    = "btrc-batch-parent"
	btrcBatchChildID     = "btrc-batch-child"
	btrcBatchParentTrace = "btrc-batch-parent-trace"
	btrcBatchChildTrace  = "btrc-batch-child-trace"
)

var btrcBatchEventOrder = []interfaces.FactoryEventType{
	interfaces.FactoryEventTypeRunRequest,
	interfaces.FactoryEventTypeInitialStructureRequest,
	interfaces.FactoryEventTypeSessionStarted,
	interfaces.FactoryEventTypeFactoryStateResponse,
	interfaces.FactoryEventTypeWorkRequest,
	interfaces.FactoryEventTypeRelationshipChangeRequest,
	interfaces.FactoryEventTypeDispatchRequest,
	interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	interfaces.FactoryEventTypeModelRequest,
	interfaces.FactoryEventTypeModelResponse,
	interfaces.FactoryEventTypeAgentRunResponse,
	interfaces.FactoryEventTypeDispatchResponse,
	interfaces.FactoryEventTypeDispatchRequest,
	interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	interfaces.FactoryEventTypeModelRequest,
	interfaces.FactoryEventTypeModelResponse,
	interfaces.FactoryEventTypeAgentRunResponse,
	interfaces.FactoryEventTypeDispatchResponse,
	interfaces.FactoryEventTypeFactoryStateResponse,
	interfaces.FactoryEventTypeRunResponse,
	interfaces.FactoryEventTypeSessionResultUpdated,
	interfaces.FactoryEventTypeSessionCompleted,
}

func TestBTRCP0BatchSuccessCharacterization(t *testing.T) {
	provider := newBTRCBatchProvider(false)
	artifact, executeErr := runBTRCBatch(t, btrcBatchRequest(), provider)
	if executeErr != nil {
		t.Fatalf("Process.Execute(batch) error = %v", executeErr)
	}
	if got := provider.CallCount(); got != 2 {
		t.Fatalf("injected provider calls = %d, want 2", got)
	}

	assertBTRCBatchEventOrder(t, artifact.Events)
	assertBTRCBatchRequestAndChild(t, artifact.Events, btrcBatchRequest())
	dispatches := observeBTRCBatchDispatches(t, artifact.Events)
	assertBTRCBatchDispatchCorrelation(t, dispatches, []string{btrcBatchParentID, btrcBatchChildID})
	assertBTRCBatchOutcomes(t, dispatches, false)
	assertBTRCBatchTerminalSession(t, artifact.Events)
}

func TestBTRCP0BatchPartialFailureCharacterization(t *testing.T) {
	provider := newBTRCBatchProvider(true)
	artifact, executeErr := runBTRCBatch(t, btrcBatchRequest(), provider)
	if executeErr == nil {
		t.Fatal("Process.Execute(partial batch) error = nil, want truthful failed-Work result")
	}
	if got := provider.CallCount(); got != 2 {
		t.Fatalf("injected provider calls = %d, want 2", got)
	}

	assertBTRCBatchEventOrder(t, artifact.Events)
	assertBTRCBatchRequestAndChild(t, artifact.Events, btrcBatchRequest())
	dispatches := observeBTRCBatchDispatches(t, artifact.Events)
	assertBTRCBatchDispatchCorrelation(t, dispatches, []string{btrcBatchParentID, btrcBatchChildID})
	assertBTRCBatchOutcomes(t, dispatches, true)
	assertBTRCBatchTerminalSession(t, artifact.Events)
}

func btrcBatchRequest() work.WorkRequest {
	return work.WorkRequest{
		RequestID: btrcBatchRequestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{
				Name:       "batch-parent",
				WorkID:     btrcBatchParentID,
				WorkTypeID: "task",
				TraceID:    btrcBatchParentTrace,
				Payload:    map[string]string{"title": "batch parent"},
			},
			{
				Name:       "batch-child",
				WorkID:     btrcBatchChildID,
				WorkTypeID: "task",
				TraceID:    btrcBatchChildTrace,
				Payload:    map[string]string{"title": "batch child"},
			},
		},
		Relations: []work.WorkRelation{{
			Type:           work.WorkRelationParentChild,
			SourceWorkName: "batch-child",
			TargetWorkName: "batch-parent",
		}},
	}
}

func runBTRCBatch(
	t *testing.T,
	request work.WorkRequest,
	provider platformprocess.CommandRunner,
) (*interfaces.ReplayArtifact, error) {
	t.Helper()

	session := openBTRCBatchSession(t, provider)
	artifact, executeErr := executeBTRCBatch(t, session, request)
	session.close(t)
	assertBTRCBatchRouteRequests(t, session)
	return artifact, executeErr
}

func assertBTRCBatchEventOrder(t *testing.T, events []interfaces.FactoryEvent) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("batch recording has no events")
	}
	seenEventIDs := make(map[string]struct{}, len(events))
	got := make([]interfaces.FactoryEventType, len(events))
	for index, event := range events {
		got[index] = event.Type
		if event.Context.Sequence != index {
			t.Fatalf("event[%d] global sequence = %d, want %d", index, event.Context.Sequence, index)
		}
		if event.Id == "" {
			t.Fatalf("event[%d] has empty id", index)
		}
		if _, duplicate := seenEventIDs[event.Id]; duplicate {
			t.Fatalf("event[%d] id %q is duplicated", index, event.Id)
		}
		seenEventIDs[event.Id] = struct{}{}
	}
	if !reflect.DeepEqual(got, btrcBatchEventOrder) {
		t.Fatalf("canonical batch event order = %v, want %v", got, btrcBatchEventOrder)
	}

	for _, event := range events {
		switch event.Type {
		case interfaces.FactoryEventTypeSessionStarted,
			interfaces.FactoryEventTypeSessionResultUpdated,
			interfaces.FactoryEventTypeSessionCompleted:
			if got := btrcStringPointerValue(event.Context.SessionID); got != "~default" {
				t.Fatalf("%s session id = %q, want %q", event.Type, got, "~default")
			}
		}
	}
}

func assertBTRCBatchRequestAndChild(
	t *testing.T,
	events []interfaces.FactoryEvent,
	request work.WorkRequest,
) {
	t.Helper()
	var requestEvent, relationshipEvent *interfaces.FactoryEvent
	for index := range events {
		event := &events[index]
		switch event.Type {
		case interfaces.FactoryEventTypeWorkRequest:
			requestEvent = event
		case interfaces.FactoryEventTypeRelationshipChangeRequest:
			relationshipEvent = event
		}
	}
	if requestEvent == nil || relationshipEvent == nil {
		t.Fatalf("batch request events = request:%v relationship:%v, want both", requestEvent != nil, relationshipEvent != nil)
	}
	if got := btrcStringPointerValue(requestEvent.Context.RequestID); got != request.RequestID {
		t.Fatalf("WORK_REQUEST request id = %q, want %q", got, request.RequestID)
	}
	if got := btrcStringSliceValue(requestEvent.Context.WorkIDs); !reflect.DeepEqual(got, []string{btrcBatchParentID, btrcBatchChildID}) {
		t.Fatalf("WORK_REQUEST work ids = %#v, want parent and child", got)
	}
	var requestPayload work.WorkRequestEventPayload
	if err := requestEvent.DecodePayload(&requestPayload); err != nil {
		t.Fatalf("decode WORK_REQUEST: %v", err)
	}
	if requestPayload.Type != request.Type || len(requestPayload.Works) != 2 {
		t.Fatalf("WORK_REQUEST payload = %#v, want request %q with two work items", requestPayload, request.RequestID)
	}
	if got := []string{requestPayload.Works[0].WorkID, requestPayload.Works[1].WorkID}; !reflect.DeepEqual(got, []string{btrcBatchParentID, btrcBatchChildID}) {
		t.Fatalf("WORK_REQUEST payload work ids = %#v, want parent and child", got)
	}

	if got := btrcStringPointerValue(relationshipEvent.Context.RequestID); got != request.RequestID {
		t.Fatalf("RELATIONSHIP_CHANGE_REQUEST request id = %q, want %q", got, request.RequestID)
	}
	if got := btrcStringSliceValue(relationshipEvent.Context.WorkIDs); !reflect.DeepEqual(got, []string{btrcBatchChildID, btrcBatchParentID}) {
		t.Fatalf("RELATIONSHIP_CHANGE_REQUEST work ids = %#v, want child and parent", got)
	}
	var relationshipPayload interfaces.RelationshipChangePayload
	if err := relationshipEvent.DecodePayload(&relationshipPayload); err != nil {
		t.Fatalf("decode RELATIONSHIP_CHANGE_REQUEST: %v", err)
	}
	if relationshipPayload.Relation.Type != string(work.WorkRelationParentChild) ||
		relationshipPayload.Relation.SourceWorkName != "batch-child" ||
		relationshipPayload.Relation.TargetWorkName != "batch-parent" {
		t.Fatalf("parent-child relation = %#v, want child -> parent", relationshipPayload.Relation)
	}
}

type btrcBatchDispatch struct {
	dispatchID            string
	workID                string
	requestID             string
	requestSequence       int
	associationSequence   int
	modelRequestSequence  int
	modelResponseSequence int
	responseSequence      int
	workerSessionID       string
	response              workerexecution.DispatchResponseEventPayload
	hasRequest            bool
	hasAssociation        bool
	hasModelRequest       bool
	hasModelResponse      bool
	hasResponse           bool
}

func observeBTRCBatchDispatches(
	t *testing.T,
	events []interfaces.FactoryEvent,
) map[string]*btrcBatchDispatch {
	t.Helper()
	observed := make(map[string]*btrcBatchDispatch, 2)
	for _, event := range events {
		dispatchID := btrcStringPointerValue(event.Context.DispatchID)
		if dispatchID == "" {
			continue
		}
		entry := observed[dispatchID]
		if entry == nil {
			entry = &btrcBatchDispatch{dispatchID: dispatchID}
			observed[dispatchID] = entry
		}
		switch event.Type {
		case interfaces.FactoryEventTypeDispatchRequest:
			if entry.hasRequest {
				t.Fatalf("dispatch %q has duplicate DISPATCH_REQUEST events", dispatchID)
			}
			if got := btrcStringSliceValue(event.Context.WorkIDs); len(got) != 1 {
				t.Fatalf("dispatch %q request work ids = %#v, want one", dispatchID, got)
			} else {
				entry.workID = got[0]
			}
			entry.requestID = btrcAssertDispatchRequestID(t, event, entry.requestID)
			entry.requestSequence = event.Context.Sequence
			entry.hasRequest = true
		case interfaces.FactoryEventTypeDispatchWorkerSessionAssoc:
			if entry.hasAssociation {
				t.Fatalf("dispatch %q has duplicate Worker Session associations", dispatchID)
			}
			var payload interfaces.DispatchWorkerSessionAssociationEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode association %q: %v", event.Id, err)
			}
			entry.workerSessionID = payload.WorkerSessionID
			entry.requestID = btrcAssertDispatchRequestID(t, event, entry.requestID)
			entry.associationSequence = event.Context.Sequence
			entry.hasAssociation = true
		case interfaces.FactoryEventTypeModelRequest:
			if entry.hasModelRequest {
				t.Fatalf("dispatch %q has duplicate MODEL_REQUEST events", dispatchID)
			}
			entry.requestID = btrcAssertDispatchRequestID(t, event, entry.requestID)
			entry.modelRequestSequence = event.Context.Sequence
			entry.hasModelRequest = true
		case interfaces.FactoryEventTypeModelResponse:
			if entry.hasModelResponse {
				t.Fatalf("dispatch %q has duplicate MODEL_RESPONSE events", dispatchID)
			}
			entry.requestID = btrcAssertDispatchRequestID(t, event, entry.requestID)
			entry.modelResponseSequence = event.Context.Sequence
			entry.hasModelResponse = true
		case interfaces.FactoryEventTypeDispatchResponse:
			if entry.hasResponse {
				t.Fatalf("dispatch %q has duplicate DISPATCH_RESPONSE events", dispatchID)
			}
			if err := event.DecodePayload(&entry.response); err != nil {
				t.Fatalf("decode dispatch response %q: %v", event.Id, err)
			}
			entry.responseSequence = event.Context.Sequence
			entry.hasResponse = true
		}
	}
	return observed
}

func assertBTRCBatchDispatchCorrelation(
	t *testing.T,
	dispatches map[string]*btrcBatchDispatch,
	wantWorkIDs []string,
) {
	t.Helper()
	if len(dispatches) != len(wantWorkIDs) {
		t.Fatalf("dispatch count = %d, want %d: %#v", len(dispatches), len(wantWorkIDs), dispatches)
	}
	seen := make(map[string]bool, len(dispatches))
	for dispatchID, dispatch := range dispatches {
		if dispatchID == "" || dispatch.workID == "" || seen[dispatch.workID] {
			t.Fatalf("dispatch identity = %#v, want one distinct work per dispatch", dispatch)
		}
		seen[dispatch.workID] = true
		if dispatch.workerSessionID != dispatchID {
			t.Fatalf("dispatch %q Worker Session id = %q, want stable one-to-one association", dispatchID, dispatch.workerSessionID)
		}
		if !dispatch.hasRequest || !dispatch.hasAssociation || !dispatch.hasModelRequest || !dispatch.hasModelResponse || !dispatch.hasResponse {
			t.Fatalf("dispatch %q missing lifecycle event: %#v", dispatchID, dispatch)
		}
		if dispatch.requestID != btrcBatchRequestID {
			t.Fatalf("dispatch %q request id = %q, want %q", dispatchID, dispatch.requestID, btrcBatchRequestID)
		}
		if dispatch.associationSequence <= dispatch.requestSequence ||
			dispatch.modelRequestSequence <= dispatch.associationSequence ||
			dispatch.modelResponseSequence <= dispatch.modelRequestSequence ||
			dispatch.responseSequence <= dispatch.modelResponseSequence {
			t.Fatalf("dispatch %q lifecycle sequences = request:%d association:%d model request:%d model response:%d response:%d", dispatchID, dispatch.requestSequence, dispatch.associationSequence, dispatch.modelRequestSequence, dispatch.modelResponseSequence, dispatch.responseSequence)
		}
	}
	for _, workID := range wantWorkIDs {
		if !seen[workID] {
			t.Fatalf("no dispatch observed for work %q: %#v", workID, dispatches)
		}
	}
	ordered := make([]*btrcBatchDispatch, 0, len(dispatches))
	for _, dispatch := range dispatches {
		ordered = append(ordered, dispatch)
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].requestSequence < ordered[j].requestSequence
	})
	if len(ordered) != 2 || ordered[0].workID != btrcBatchParentID || ordered[1].workID != btrcBatchChildID {
		t.Fatalf("dispatch work order = %#v, want parent then child", ordered)
	}
}

func assertBTRCBatchOutcomes(
	t *testing.T,
	dispatches map[string]*btrcBatchDispatch,
	wantPartialFailure bool,
) {
	t.Helper()
	accepted, failed := 0, 0
	for _, dispatch := range dispatches {
		if dispatch.response.OutputWork == nil || len(*dispatch.response.OutputWork) != 1 {
			t.Fatalf("dispatch %q output work = %#v, want exactly one terminal work result", dispatch.dispatchID, dispatch.response.OutputWork)
		}
		outputWork := (*dispatch.response.OutputWork)[0]
		if outputWork.WorkID != dispatch.workID {
			t.Fatalf("dispatch %q output work id = %q, want %q", dispatch.dispatchID, outputWork.WorkID, dispatch.workID)
		}
		if outputWork.State == nil || (outputWork.State.Name != "complete" && outputWork.State.Name != "failed") {
			t.Fatalf("dispatch %q work %q state = %#v, want complete or failed terminal state", dispatch.dispatchID, outputWork.WorkID, outputWork.State)
		}
		switch dispatch.response.Outcome {
		case workerexecution.OutcomeAccepted:
			accepted++
			if outputWork.State.Name != "complete" || dispatch.response.FailureDetail != nil || dispatch.response.ProviderFailure != nil {
				t.Fatalf("accepted dispatch %q response = %#v, want complete work without failure metadata", dispatch.dispatchID, dispatch.response)
			}
		case workerexecution.OutcomeFailed:
			failed++
			if outputWork.State.Name != "failed" || dispatch.response.FailureDetail == nil || dispatch.response.ProviderFailure == nil {
				t.Fatalf("failed dispatch %q response = %#v, want failed work with typed failure metadata", dispatch.dispatchID, dispatch.response)
			}
			if dispatch.response.FailureDetail.Reason != workerexecution.WorkFailureTypePermanentBadRequest ||
				dispatch.response.ProviderFailure.Family != workerexecution.WorkFailureFamilyTerminal ||
				dispatch.response.ProviderFailure.Type != workerexecution.WorkFailureTypePermanentBadRequest {
				t.Fatalf("failed dispatch %q classification = detail:%#v provider:%#v, want terminal permanent_bad_request", dispatch.dispatchID, dispatch.response.FailureDetail, dispatch.response.ProviderFailure)
			}
		default:
			t.Fatalf("dispatch %q outcome = %q, want ACCEPTED or FAILED", dispatch.dispatchID, dispatch.response.Outcome)
		}
	}
	if wantPartialFailure {
		if accepted != 1 || failed != 1 {
			t.Fatalf("partial batch outcomes = accepted:%d failed:%d, want one of each", accepted, failed)
		}
		parent := btrcBatchDispatchForWork(t, dispatches, btrcBatchParentID)
		child := btrcBatchDispatchForWork(t, dispatches, btrcBatchChildID)
		if parent.response.Outcome != workerexecution.OutcomeAccepted || child.response.Outcome != workerexecution.OutcomeFailed {
			t.Fatalf("partial batch parent/child outcomes = parent:%q child:%q, want parent ACCEPTED and child FAILED", parent.response.Outcome, child.response.Outcome)
		}
		return
	}
	if accepted != 2 || failed != 0 {
		t.Fatalf("successful batch outcomes = accepted:%d failed:%d, want 2 accepted and 0 failed", accepted, failed)
	}
}

func btrcBatchDispatchForWork(
	t *testing.T,
	dispatches map[string]*btrcBatchDispatch,
	workID string,
) *btrcBatchDispatch {
	t.Helper()
	for _, dispatch := range dispatches {
		if dispatch.workID == workID {
			return dispatch
		}
	}
	t.Fatalf("missing dispatch for work %q", workID)
	return nil
}

func btrcAssertDispatchRequestID(
	t *testing.T,
	event interfaces.FactoryEvent,
	previous string,
) string {
	t.Helper()
	requestID := btrcStringPointerValue(event.Context.RequestID)
	if requestID != btrcBatchRequestID {
		t.Fatalf("%s request id = %q, want %q", event.Type, requestID, btrcBatchRequestID)
	}
	if previous != "" && previous != requestID {
		t.Fatalf("dispatch request identity changed from %q to %q", previous, requestID)
	}
	return requestID
}

func assertBTRCBatchTerminalSession(t *testing.T, events []interfaces.FactoryEvent) {
	t.Helper()
	resultIndex, completedIndex, runResponseIndex := -1, -1, -1
	var resultPayload interfaces.FactorySessionResultUpdatedEventPayload
	var completedPayload interfaces.FactorySessionCompletedEventPayload
	for index, event := range events {
		switch event.Type {
		case interfaces.FactoryEventTypeRunResponse:
			runResponseIndex = index
		case interfaces.FactoryEventTypeSessionResultUpdated:
			if resultIndex != -1 {
				t.Fatalf("duplicate SESSION_RESULT_UPDATED at %d and %d", resultIndex, index)
			}
			resultIndex = index
			if err := event.DecodePayload(&resultPayload); err != nil {
				t.Fatalf("decode SESSION_RESULT_UPDATED: %v", err)
			}
		case interfaces.FactoryEventTypeSessionCompleted:
			if completedIndex != -1 {
				t.Fatalf("duplicate SESSION_COMPLETED at %d and %d", completedIndex, index)
			}
			completedIndex = index
			if err := event.DecodePayload(&completedPayload); err != nil {
				t.Fatalf("decode SESSION_COMPLETED: %v", err)
			}
		}
	}
	if runResponseIndex < 0 || resultIndex != runResponseIndex+1 || completedIndex != resultIndex+1 {
		t.Fatalf("terminal publication indexes = run response:%d result:%d completed:%d, want contiguous terminal order", runResponseIndex, resultIndex, completedIndex)
	}
	if resultPayload.ResultStatus != interfaces.FactorySessionResultStatusFinal {
		t.Fatalf("SESSION_RESULT_UPDATED result status = %q, want FINAL", resultPayload.ResultStatus)
	}
	if completedPayload.FinalStatus != interfaces.FactorySessionLifecycleStatusSucceeded ||
		completedPayload.ResultStatus == nil || *completedPayload.ResultStatus != interfaces.FactorySessionResultStatusFinal {
		t.Fatalf("SESSION_COMPLETED projection = %#v, want SUCCEEDED/FINAL", completedPayload)
	}
	if completedPayload.FailureDetail != nil {
		t.Fatalf("SESSION_COMPLETED failure detail = %#v, want nil for current terminal aggregate", completedPayload.FailureDetail)
	}
}

func newBTRCBatchProvider(failSecond bool) *support.ShapedProviderCommandRunner {
	results := []platformprocess.CommandResult{
		{Stdout: []byte("processed. COMPLETE")},
		{Stdout: []byte("processed. COMPLETE")},
	}
	if failSecond {
		results[1] = platformprocess.CommandResult{
			ExitCode: 1,
			Stderr:   []byte("ERROR: unexpected status 400 Bad Request: invalid request"),
		}
	}
	return support.NewShapedProviderCommandRunner(results...)
}

func btrcStringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func btrcStringSliceValue(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}
