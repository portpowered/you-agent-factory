package root_composition_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRootBuildProcessExecuteKeepsHumanApprovalWorkerlessAndPending(t *testing.T) {
	t.Parallel()

	factoryDir := support.ScaffoldFactory(t, map[string]any{
		"name": "root-human-approval",
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "approved", "type": "TERMINAL"},
				map[string]any{"name": "rejected", "type": "TERMINAL"},
			},
		}},
		"workstations": []any{map[string]any{
			"id":          "release-approval",
			"name":        "Release Approval",
			"type":        "HUMAN_APPROVAL",
			"description": map[string]any{"type": "LOCALIZABLE_ASSET", "value": "Approve the release"},
			"inputs":      []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":     []any{map[string]any{"workType": "task", "state": "approved"}},
			"onRejection": []any{map[string]any{"workType": "task", "state": "rejected"}},
		}},
	})
	dispatches := make(chan interfaces.FactoryDispatchRecord, 4)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:       factoryDir,
		WorkingDirectory: factoryDir,
		Edges:            edges.Edges{DispatchRecorder: func(record interfaces.FactoryDispatchRecord) { dispatches <- record }},
	})
	defer server.Stop(t)
	accepted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         approvalStringPtr("Release candidate"),
		WorkTypeName: "task",
		TraceId:      approvalStringPtr("root-human-approval-trace"),
		Payload:      map[string]any{"title": "release candidate", "secret": "must-not-become-an-event-payload"},
	})
	if accepted.RequestId == "" {
		t.Fatalf("submitted approval work response = %#v, want request identity", accepted)
	}
	events := server.GetFactoryEvents(t)
	if countFactoryEventType(events, interfaces.FactoryEventTypeDispatchRequest) != 1 ||
		countFactoryEventType(events, interfaces.FactoryEventTypeHumanApprovalRequested) != 1 {
		t.Fatalf("factory events = %#v, want exactly one dispatch and human approval request", events)
	}
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeHumanApprovalRequested {
			continue
		}
		payload, err := event.Payload.AsHumanApprovalRequestedEventPayload()
		if err != nil {
			t.Fatalf("decode human approval event payload: %v", err)
		}
		if payload.WorkstationId != "release-approval" {
			t.Fatalf("human approval workstation id = %q, want canonical id release-approval", payload.WorkstationId)
		}
	}
	var dispatch interfaces.FactoryDispatchRecord
	select {
	case dispatch = <-dispatches:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the root dispatch recorder")
	}
	if !dispatch.HumanApproval {
		t.Fatalf("dispatch = %#v, want reserved human approval dispatch", dispatch)
	}
	if dispatch.Dispatch.WorkerType != "" {
		t.Fatalf("human approval worker type = %q, want empty", dispatch.Dispatch.WorkerType)
	}
}

func approvalStringPtr(value string) *string { return &value }

func containsFactoryEventType(events []factoryapi.FactoryEvent, want interfaces.FactoryEventType) bool {
	for _, event := range events {
		if string(event.Type) == string(want) {
			return true
		}
	}
	return false
}

func countFactoryEventType(events []factoryapi.FactoryEvent, want interfaces.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if string(event.Type) == string(want) {
			count++
		}
	}
	return count
}
