package guards_batch

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func TestFactoryRequestBatch_InvalidStructureRejected(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		wantErr string
	}{
		{
			name:    "empty work array",
			payload: `{"requestId": "invalid-1", "type": "FACTORY_REQUEST_BATCH", "works": []}`,
			wantErr: "works array must contain at least one item",
		},
		{
			name:    "missing name field",
			payload: `{"requestId": "invalid-2", "type": "FACTORY_REQUEST_BATCH", "works": [{"workTypeName": "task"}]}`,
			wantErr: "missing required name",
		},
		{
			name:    "missing work type field",
			payload: `{"requestId": "invalid-3", "type": "FACTORY_REQUEST_BATCH", "works": [{"name": "foo"}]}`,
			wantErr: "missing workTypeName",
		},
		{
			name:    "duplicate work names",
			payload: `{"requestId": "invalid-4", "type": "FACTORY_REQUEST_BATCH", "works": [{"workTypeName": "task", "name": "dup"}, {"workTypeName": "task", "name": "dup"}]}`,
			wantErr: "duplicate name",
		},
		{
			name:    "unknown work type",
			payload: `{"requestId": "invalid-5", "type": "FACTORY_REQUEST_BATCH", "works": [{"workTypeName": "nonexistent", "name": "foo"}]}`,
			wantErr: "unknown work type",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertInvalidBatchPayload(t, tc.payload, tc.wantErr)
		})
	}
}

func TestFactoryRequestBatch_InvalidRelationsRejected(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		wantErr string
	}{
		{
			name:    "unknown source in relation",
			payload: `{"requestId": "invalid-6", "type": "FACTORY_REQUEST_BATCH", "works": [{"workTypeName": "task", "name": "a"}], "relations": [{"type": "DEPENDS_ON", "sourceWorkName": "missing", "targetWorkName": "a"}]}`,
			wantErr: "unknown sourceWorkName",
		},
		{
			name:    "unknown target in relation",
			payload: `{"requestId": "invalid-7", "type": "FACTORY_REQUEST_BATCH", "works": [{"workTypeName": "task", "name": "a"}], "relations": [{"type": "DEPENDS_ON", "sourceWorkName": "a", "targetWorkName": "missing"}]}`,
			wantErr: "unknown targetWorkName",
		},
		{
			name:    "self-referencing dependency",
			payload: `{"requestId": "invalid-8", "type": "FACTORY_REQUEST_BATCH", "works": [{"workTypeName": "task", "name": "a"}], "relations": [{"type": "DEPENDS_ON", "sourceWorkName": "a", "targetWorkName": "a"}]}`,
			wantErr: "self-dependency",
		},
		{
			name:    "self-parenting relation",
			payload: `{"requestId": "invalid-9", "type": "FACTORY_REQUEST_BATCH", "works": [{"workTypeName": "task", "name": "a"}], "relations": [{"type": "PARENT_CHILD", "sourceWorkName": "a", "targetWorkName": "a"}]}`,
			wantErr: "self-parenting",
		},
		{
			name:    "duplicate parent-child relation",
			payload: `{"requestId": "invalid-10", "type": "FACTORY_REQUEST_BATCH", "works": [{"workTypeName": "task", "name": "parent"}, {"workTypeName": "task", "name": "child"}], "relations": [{"type": "PARENT_CHILD", "sourceWorkName": "child", "targetWorkName": "parent"}, {"type": "PARENT_CHILD", "sourceWorkName": "child", "targetWorkName": "parent"}]}`,
			wantErr: "duplicates relations[0]",
		},
		{
			name:    "invalid dependency required_state",
			payload: `{"requestId": "invalid-11", "type": "FACTORY_REQUEST_BATCH", "works": [{"workTypeName": "task", "name": "draft"}, {"workTypeName": "task", "name": "review"}], "relations": [{"type": "DEPENDS_ON", "sourceWorkName": "review", "targetWorkName": "draft", "requiredState": "queued"}]}`,
			wantErr: "unknown requiredState",
		},
		{
			name:    "unsupported relation type",
			payload: `{"requestId": "invalid-12", "type": "FACTORY_REQUEST_BATCH", "works": [{"workTypeName": "task", "name": "a"}, {"workTypeName": "task", "name": "b"}], "relations": [{"type": "INVALID", "sourceWorkName": "a", "targetWorkName": "b"}]}`,
			wantErr: "unsupported type",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertInvalidBatchPayload(t, tc.payload, tc.wantErr)
		})
	}
}

func TestFactoryRequestBatch_InvalidJSONRejected(t *testing.T) {
	assertInvalidBatchPayload(t, `{not json}`, "invalid character")
}

func TestFactoryRequestBatch_BatchSubmissionAtomic(t *testing.T) {
	validWorkTypes := map[string]bool{"task": true}

	invalidInput := interfaces.WorkRequest{
		RequestID: "request-atomic-invalid",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{WorkTypeID: "task", Name: "valid-item"},
			{WorkTypeID: "task", Name: ""},
		},
	}

	payload, err := json.Marshal(invalidInput)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	var invalidRequest interfaces.WorkRequest
	if err := json.Unmarshal(payload, &invalidRequest); err != nil {
		t.Fatalf("failed to unmarshal input: %v", err)
	}
	_, err = factory.NormalizeWorkRequest(invalidRequest, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: validWorkTypes,
	})
	if err == nil {
		t.Fatal("expected validation error for batch with invalid item, got nil")
	}

	validInput := interfaces.WorkRequest{
		RequestID: "request-atomic-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{WorkTypeID: "task", Name: "item-1"},
			{WorkTypeID: "task", Name: "item-2"},
			{WorkTypeID: "task", Name: "item-3"},
		},
	}

	payload, err = json.Marshal(validInput)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	var validRequest interfaces.WorkRequest
	if err := json.Unmarshal(payload, &validRequest); err != nil {
		t.Fatalf("failed to unmarshal input: %v", err)
	}
	expanded, err := factory.NormalizeWorkRequest(validRequest, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: validWorkTypes,
	})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest failed: %v", err)
	}

	if len(expanded) != 3 {
		t.Errorf("expected 3 expanded requests, got %d", len(expanded))
	}
	for _, r := range expanded {
		if r.WorkID == "" {
			t.Error("expanded request has empty WorkID")
		}
		if !strings.HasPrefix(r.WorkID, "batch-request-atomic-1-") {
			t.Errorf("expected WorkID prefix 'batch-request-atomic-1-', got %q", r.WorkID)
		}
	}
}

func assertInvalidBatchPayload(t *testing.T, payload string, wantErr string) {
	t.Helper()

	var request interfaces.WorkRequest
	err := json.Unmarshal([]byte(payload), &request)
	if err == nil {
		_, err = factory.NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{
			ValidWorkTypes: map[string]bool{"task": true},
			ValidStatesByType: map[string]map[string]bool{
				"task": {"init": true, "complete": true},
			},
		})
	}
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("expected error containing %q, got %v", wantErr, err)
	}
}
