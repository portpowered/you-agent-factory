package runtime_api

import (
	"reflect"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestGeneratedAPIIntegrationSmoke_BatchWorkTypeNameNormalizesRuntimeWork(t *testing.T) {
	dir := support.ScaffoldFactory(t, competingPipelineConfig())
	server := support.StartFunctionalAPIServiceModeServer(t, dir, true)
	stream := openDefaultSessionFactoryEventHTTPStream(t, server.URL())
	_ = stream.next(5 * time.Second) // RUN_REQUEST
	_ = stream.next(5 * time.Second) // INITIAL_STRUCTURE_REQUEST

	firstWorkID := "work-generated-api-batch-first"
	secondWorkID := "work-generated-api-batch-second"
	requiredState := "complete"
	workTypeName := "task"
	request := factoryapi.WorkRequest{
		RequestId: "request-generated-api-batch", Type: factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:     &[]factoryapi.Work{{Name: "first", WorkId: &firstWorkID, WorkTypeName: &workTypeName, Payload: map[string]string{"step": "first"}}, {Name: "second", WorkId: &secondWorkID, WorkTypeName: &workTypeName, Payload: map[string]string{"step": "second"}}},
		Relations: &[]factoryapi.Relation{{Type: factoryapi.RelationTypeDependsOn, SourceWorkName: "second", TargetWorkName: "first", RequiredState: &requiredState}},
	}

	response := putGeneratedWorkRequest(t, server.URL(), request.RequestId, request)
	if response.RequestId != request.RequestId || response.TraceId == "" || len(response.Works) != 2 {
		t.Fatalf("PUT /work-requests response = %#v, want request id, trace id, and two works", response)
	}
	wantWorks := []factoryapi.UpsertWorkRequestSubmittedWork{{Name: "first", WorkTypeName: workTypeName, WorkId: firstWorkID}, {Name: "second", WorkTypeName: workTypeName, WorkId: secondWorkID}}
	if !reflect.DeepEqual(response.Works, wantWorks) {
		t.Fatalf("PUT /work-requests works = %#v, want %#v", response.Works, wantWorks)
	}
	if replayed := putGeneratedWorkRequest(t, server.URL(), request.RequestId, request); !reflect.DeepEqual(replayed, response) {
		t.Fatalf("replayed PUT /work-requests response = %#v, want original %#v", replayed, response)
	}

	items := waitForGeneratedWorkIDsComplete(t, server.URL(), []string{firstWorkID, secondWorkID}, 10*time.Second)
	for _, item := range items {
		if stringPointerValue(item.WorkTypeName) != workTypeName {
			t.Fatalf("generated batch work %s work type = %q, want %q", stringPointerValue(item.WorkId), stringPointerValue(item.WorkTypeName), workTypeName)
		}
	}
	assertPublicBatchDurableOutcomes(t, server.URL(), firstWorkID, secondWorkID)
	assertPublicBatchDependencyAndIdempotency(t, stream, request.RequestId, firstWorkID, secondWorkID)
}

func competingPipelineConfig() map[string]any {
	config := simplePipelineConfig()
	config["workers"] = []map[string]string{{"name": "worker-a"}, {"name": "worker-b"}}
	config["workstations"] = append(config["workstations"].([]map[string]any), map[string]any{
		"name":      "process-alternate",
		"worker":    "worker-b",
		"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
		"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
		"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
	})
	return config
}

func assertPublicBatchDurableOutcomes(t *testing.T, baseURL, firstWorkID, secondWorkID string) {
	t.Helper()
	workList := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
	counts := map[string]int{}
	for _, work := range workList.Results {
		if workID := stringPointerValue(work.WorkId); workID == firstWorkID || workID == secondWorkID {
			counts[workID]++
		}
	}
	if counts[firstWorkID] != 1 || counts[secondWorkID] != 1 {
		t.Fatalf("public durable batch work counts = %#v, want one outcome for each batch work", counts)
	}
}

func assertPublicBatchDependencyAndIdempotency(t *testing.T, stream *factoryEventHTTPStream, requestID, firstWorkID, secondWorkID string) {
	t.Helper()
	requestEvents, relationEvents, firstTerminalSequence, secondDispatchSequence := 0, 0, -1, -1
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		event := stream.next(time.Until(deadline))
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			if support.StringPointerValue(event.Context.RequestId) == requestID {
				requestEvents++
				payload, err := event.Payload.AsWorkRequestEventPayload()
				if err != nil {
					t.Fatalf("decode public WORK_REQUEST event: %v", err)
				}
				if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch || len(support.FactoryWorksValue(payload.Works)) != 2 {
					t.Fatalf("public WORK_REQUEST payload = %#v, want two-work FACTORY_REQUEST_BATCH", payload)
				}
			}
		case factoryapi.FactoryEventTypeRelationshipChangeRequest:
			if support.StringPointerValue(event.Context.RequestId) == requestID {
				relationEvents++
				payload, err := event.Payload.AsRelationshipChangeRequestEventPayload()
				if err != nil {
					t.Fatalf("decode public RELATIONSHIP_CHANGE_REQUEST event: %v", err)
				}
				if payload.Relation.Type != factoryapi.RelationTypeDependsOn || payload.Relation.SourceWorkName != "second" || support.StringPointerValue(payload.Relation.TargetWorkId) != firstWorkID {
					t.Fatalf("public dependency relation = %#v, want second DEPENDS_ON first", payload.Relation)
				}
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			if payload, err := event.Payload.AsDispatchResponseEventPayload(); err == nil && payload.Outcome == factoryapi.WorkOutcomeAccepted && publicEventWorkIDsContain(event.Context.WorkIds, firstWorkID) {
				firstTerminalSequence = event.Context.Sequence
			}
		case factoryapi.FactoryEventTypeDispatchRequest:
			if payload, err := event.Payload.AsDispatchRequestEventPayload(); err == nil && publicDispatchInputsContainWork(payload, secondWorkID) {
				secondDispatchSequence = event.Context.Sequence
			}
		}
		if requestEvents == 1 && relationEvents == 1 && firstTerminalSequence >= 0 && secondDispatchSequence > firstTerminalSequence {
			return
		}
	}
	t.Fatalf("public batch events = requests:%d relations:%d first-terminal:%d second-dispatch:%d; want one request, one relation, and dependency ordering", requestEvents, relationEvents, firstTerminalSequence, secondDispatchSequence)
}

func publicDispatchInputsContainWork(payload factoryapi.DispatchRequestEventPayload, workID string) bool {
	for _, input := range payload.Inputs {
		if input.WorkId == workID {
			return true
		}
	}
	return false
}

func publicEventWorkIDsContain(workIDs *[]string, want string) bool {
	if workIDs == nil {
		return false
	}
	for _, workID := range *workIDs {
		if workID == want {
			return true
		}
	}
	return false
}
