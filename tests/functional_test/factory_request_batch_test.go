package functional_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/factory/projections"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
)

// TestFactoryRequestBatch_CreatesOneTokenPerWorkItem validates that a
// FACTORY_REQUEST_BATCH request creates one token per work item.
func TestFactoryRequestBatch_CreatesOneTokenPerWorkItem(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "factory_request_batch"))

	request := interfaces.WorkRequest{
		RequestID: "request-batch-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "alpha", WorkTypeID: "task", Tags: map[string]string{"project": "test-project"}},
			{Name: "beta", WorkTypeID: "task", Tags: map[string]string{"project": "test-project"}},
			{Name: "gamma", WorkTypeID: "task", Tags: map[string]string{"project": "test-project"}},
		},
	}

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"finisher":  {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitWorkRequest(context.Background(), request)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().PlaceTokenCount("task:complete", 3)
}

// TestFactoryRequestBatch_TagsAccessibleInTokenPayload verifies that tags from
// the request batch are carried on each token and accessible during
// dispatch (which is how they reach prompt rendering).
func TestFactoryRequestBatch_TagsAccessibleInTokenPayload(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "tags_test"))

	h := testutil.NewServiceTestHarness(t, dir)

	checker := &capturingExecutor{
		result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	}
	h.SetCustomExecutor("checker", checker)
	h.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-tags-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "story-1",
			WorkTypeID: "task",
			TraceID:    "trace-tags-1",
			Payload:    "story-1",
			Tags: map[string]string{
				"branch":  "feature/test",
				"project": "inventory-service",
			},
		}},
	})

	h.RunUntilComplete(t, 10*time.Second)

	if checker.callCount == 0 {
		t.Fatal("checker executor was never called")
	}

	if len(checker.lastDispatch.InputTokens) == 0 {
		t.Fatal("expected at least one input token in dispatch")
	}

	tags := firstInputToken(checker.lastDispatch.InputTokens).Color.Tags

	// Verify parent tags propagated.
	if tags["branch"] != "feature/test" {
		t.Errorf("expected tag branch=feature/test, got %q", tags["branch"])
	}
	if tags["project"] != "inventory-service" {
		t.Errorf("expected tag project=inventory-service, got %q", tags["project"])
	}

	// Verify auto-injected tags from request normalization.
	if tags["_work_name"] != "story-1" {
		t.Errorf("expected auto-injected tag _work_name=story-1, got %q", tags["_work_name"])
	}
	if tags["_work_type"] != "task" {
		t.Errorf("expected auto-injected tag _work_type=task, got %q", tags["_work_type"])
	}
}

// TestFactoryRequestBatch_FactoryProjectConfigWinsOverProjectTagForProviderContext
// verifies explicit factory project context wins over a token project tag in
// rendered dispatch context while per-token project data stays available.
func TestFactoryRequestBatch_FactoryProjectConfigWinsOverProjectTagForProviderContext(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "tags_test"))
	setWorkingDirectory(t, dir)

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"checker": {{Content: "checked COMPLETE"}},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-project-override-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "story-project-override",
			WorkID:     "work-project-override",
			WorkTypeID: "task",
			TraceID:    "trace-project-override-1",
			Payload:    "project override story",
			Tags: map[string]string{
				"branch":  "feature/project-override",
				"project": "billing-api",
			},
		}},
	})

	h.RunUntilComplete(t, 10*time.Second)

	calls := provider.Calls("checker")
	if len(calls) != 1 {
		t.Fatalf("checker provider calls = %d, want 1", len(calls))
	}

	call := calls[0]
	if call.WorkingDirectory != resolvedRuntimePath(dir, "/workspaces/fixture-default-project/feature/project-override") {
		t.Fatalf("working directory = %q, want explicit factory project context path", call.WorkingDirectory)
	}
	if call.EnvVars["PROJECT"] != "fixture-default-project" {
		t.Fatalf("PROJECT env = %q, want fixture-default-project", call.EnvVars["PROJECT"])
	}
	if call.EnvVars["CONTEXT_PROJECT"] != "fixture-default-project" {
		t.Fatalf("CONTEXT_PROJECT env = %q, want fixture-default-project", call.EnvVars["CONTEXT_PROJECT"])
	}
	if call.EnvVars["TOKEN_PROJECT"] != "billing-api" {
		t.Fatalf("TOKEN_PROJECT env = %q, want billing-api", call.EnvVars["TOKEN_PROJECT"])
	}
	if call.EnvVars["BRANCH"] != "feature/project-override" {
		t.Fatalf("BRANCH env = %q, want feature/project-override", call.EnvVars["BRANCH"])
	}

	if len(call.InputTokens) == 0 {
		t.Fatal("expected provider request input tokens")
	}
	tags := firstInputToken(call.InputTokens).Color.Tags
	if tags["project"] != "billing-api" {
		t.Fatalf("normalized token project tag = %q, want billing-api", tags["project"])
	}
	if tags["_work_name"] != "story-project-override" {
		t.Fatalf("normalized token _work_name = %q, want story-project-override", tags["_work_name"])
	}
}

// TestFactoryRequestBatch_FactoryProjectConfigFlowsToProviderContext verifies
// factory-level project config reaches provider-time template context when the
// submitted request does not include a project tag override.
func TestFactoryRequestBatch_FactoryProjectConfigFlowsToProviderContext(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "tags_test"))
	setWorkingDirectory(t, dir)

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"checker": {{Content: "checked COMPLETE"}},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-project-config-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "story-project-config",
			WorkID:     "work-project-config",
			WorkTypeID: "task",
			TraceID:    "trace-project-config-1",
			Payload:    "factory project config story",
			Tags: map[string]string{
				"branch": "feature/project-config",
			},
		}},
	})

	h.RunUntilComplete(t, 10*time.Second)

	calls := provider.Calls("checker")
	if len(calls) != 1 {
		t.Fatalf("checker provider calls = %d, want 1", len(calls))
	}

	call := calls[0]
	if call.ProjectID != "fixture-default-project" {
		t.Fatalf("provider dispatch project ID = %q, want fixture-default-project", call.ProjectID)
	}
	if call.WorkingDirectory != resolvedRuntimePath(dir, "/workspaces/fixture-default-project/feature/project-config") {
		t.Fatalf("working directory = %q, want factory project config path", call.WorkingDirectory)
	}
	if call.EnvVars["PROJECT"] != "fixture-default-project" {
		t.Fatalf("PROJECT env = %q, want fixture-default-project", call.EnvVars["PROJECT"])
	}
	if call.EnvVars["CONTEXT_PROJECT"] != "fixture-default-project" {
		t.Fatalf("CONTEXT_PROJECT env = %q, want fixture-default-project", call.EnvVars["CONTEXT_PROJECT"])
	}
	if call.EnvVars["TOKEN_PROJECT"] != "fixture-default-project" {
		t.Fatalf("TOKEN_PROJECT env = %q, want fixture-default-project", call.EnvVars["TOKEN_PROJECT"])
	}
	if call.EnvVars["BRANCH"] != "feature/project-config" {
		t.Fatalf("BRANCH env = %q, want feature/project-config", call.EnvVars["BRANCH"])
	}

	if len(call.InputTokens) == 0 {
		t.Fatal("expected provider request input tokens")
	}
	tags := firstInputToken(call.InputTokens).Color.Tags
	if tags["project"] != "" {
		t.Fatalf("normalized token project tag = %q, want no project tag override", tags["project"])
	}
	if tags["_work_name"] != "story-project-config" {
		t.Fatalf("normalized token _work_name = %q, want story-project-config", tags["_work_name"])
	}
}

// TestFactoryRequestBatch_DependsOnBlocksDispatch verifies that DEPENDS_ON
// relations prevent dependent tokens from dispatching before their
// predecessors reach the required terminal state.
func TestFactoryRequestBatch_DependsOnBlocksDispatch(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "factory_request_batch"))

	request := interfaces.WorkRequest{
		RequestID: "request-deps-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "A", WorkID: "work-A", WorkTypeID: "task", Payload: "A"},
			{Name: "B", WorkID: "work-B", WorkTypeID: "task", Payload: "B"},
		},
		Relations: []interfaces.WorkRelation{
			{
				Type:           interfaces.WorkRelationDependsOn,
				SourceWorkName: "B",
				TargetWorkName: "A",
			},
		},
	}

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"finisher":  {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitWorkRequest(context.Background(), request)

	// Run to completion: A processes first (no deps), then B processes
	// after A reaches task:complete (dependency satisfied).
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().PlaceTokenCount("task:complete", 2)

	if provider.CallCount("processor") != 2 {
		t.Errorf("expected processor called 2 times total, got %d", provider.CallCount("processor"))
	}
}

// TestFactoryRequestBatch_HarnessSnapshotObservesNormalizedWork verifies that
// service-level tests can submit canonical batches and inspect normalized work
// through GetEngineStateSnapshot instead of runtime internals.
func TestFactoryRequestBatch_HarnessSnapshotObservesNormalizedWork(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "factory_request_batch"))

	request := interfaces.WorkRequest{
		RequestID: "request-snapshot-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "first", WorkID: "work-snapshot-first", WorkTypeID: "task", TraceID: "trace-snapshot-1", Payload: "first"},
			{Name: "second", WorkID: "work-snapshot-second", WorkTypeID: "task", TraceID: "trace-snapshot-1", Payload: "second"},
		},
		Relations: []interfaces.WorkRelation{{
			Type:           interfaces.WorkRelationDependsOn,
			SourceWorkName: "second",
			TargetWorkName: "first",
		}},
	}

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"finisher":  {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitWorkRequest(context.Background(), request)

	h.RunUntilComplete(t, 10*time.Second)

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	var firstSeen bool
	var secondSeen bool
	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.Color.WorkTypeID != "task" {
			continue
		}
		if token.Color.RequestID != "request-snapshot-1" {
			t.Fatalf("normalized token %s request_id = %q, want request-snapshot-1", token.ID, token.Color.RequestID)
		}
		switch token.Color.WorkID {
		case "work-snapshot-first":
			firstSeen = true
			if token.Color.Tags["_work_name"] != "first" {
				t.Fatalf("first token _work_name = %q, want first", token.Color.Tags["_work_name"])
			}
		case "work-snapshot-second":
			secondSeen = true
			if token.Color.Tags["_work_name"] != "second" {
				t.Fatalf("second token _work_name = %q, want second", token.Color.Tags["_work_name"])
			}
			if len(token.Color.Relations) != 1 {
				t.Fatalf("second token relations = %d, want 1", len(token.Color.Relations))
			}
			if token.Color.Relations[0].TargetWorkID != "work-snapshot-first" {
				t.Fatalf("second relation target = %q, want work-snapshot-first", token.Color.Relations[0].TargetWorkID)
			}
		}
	}
	if !firstSeen || !secondSeen {
		t.Fatalf("snapshot missing normalized work tokens: first=%v second=%v", firstSeen, secondSeen)
	}
}

// TestFactoryRequestBatch_EndToEndSmoke verifies external request batches,
// dependency enforcement, canonical history, idempotent retries, and
// worker-emitted fanout together through the service harness.
func TestFactoryRequestBatch_EndToEndSmoke(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "factory_request_batch"))
	writeAgentConfig(t, dir, "processor", `---
type: MODEL_WORKER
model: test-model
---
Process the input task.
`)
	writeAgentConfig(t, dir, "finisher", `---
type: MODEL_WORKER
model: test-model
---
Finish the input task.
`)

	generatedBatchOutput := `{"request":{"requestId":"request-e2e-generated-batch","type":"FACTORY_REQUEST_BATCH","works":[{"name":"generated-alpha","workId":"work-e2e-generated-alpha","workTypeName":"task","payload":"generated alpha"},{"name":"generated-beta","workId":"work-e2e-generated-beta","workTypeName":"task","payload":"generated beta"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"generated-beta","targetWorkName":"generated-alpha"}]},"metadata":{"source":"generator:e2e-smoke","relationContext":[{"type":"DEPENDS_ON","sourceWorkName":"generated-beta","targetWorkName":"generated-alpha","requiredState":"complete"}],"parentLineage":["request-e2e-batch-smoke","work-e2e-second"]},"submissions":[{"name":"generated-alpha","workId":"work-e2e-generated-alpha","tags":{"runtime":"alpha"}},{"name":"generated-beta","workId":"work-e2e-generated-beta","tags":{"runtime":"beta"}}]}`
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {
			{Content: "external first complete COMPLETE"},
			{Content: generatedBatchOutput},
			{Content: "generated alpha complete COMPLETE"},
			{Content: "generated beta complete COMPLETE"},
		},
		"finisher": {
			{Content: "finish external first COMPLETE"},
			{Content: "finish generated alpha COMPLETE"},
			{Content: "finish generated beta COMPLETE"},
		},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	request := interfaces.WorkRequest{
		RequestID: "request-e2e-batch-smoke",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "first", WorkID: "work-e2e-first", WorkTypeID: "task", TraceID: "trace-e2e-batch-smoke", Payload: "first"},
			{Name: "second", WorkID: "work-e2e-second", WorkTypeID: "task", TraceID: "trace-e2e-batch-smoke", Payload: "second"},
		},
		Relations: []interfaces.WorkRelation{{
			Type:           interfaces.WorkRelationDependsOn,
			SourceWorkName: "second",
			TargetWorkName: "first",
		}},
	}
	h.SubmitWorkRequest(context.Background(), request)
	h.SubmitWorkRequest(context.Background(), request)
	h.RunUntilComplete(t, 10*time.Second)

	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	assertExternalBatchSmokeEvents(t, events)
	assertSecondWorkDispatchedAfterFirstTerminal(t, events)
	assertWorkerGeneratedBatchEvents(t, events)
	assertWorkerGeneratedBatchWorldState(t, events)
}

func TestFactoryRequestBatch_ChainingTraceFanIn_EndToEndSmoke(t *testing.T) {
	h, provider := newChainingTraceFanInHarness(t)
	submitChainingTraceFanInWork(t, h)
	assertChainingTraceFanInHarnessState(t, h, provider)
	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	dispatchID, currentChainingTraceID := assertChainingTraceFanInEvents(t, events)
	assertChainingTraceFanInWorldState(t, events, dispatchID, currentChainingTraceID)
}

func newChainingTraceFanInHarness(t *testing.T) (*testutil.ServiceTestHarness, *testutil.MockWorkerMapProvider) {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "factory_request_batch"))
	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		workTypes := cfg["workTypes"].([]any)
		cfg["workTypes"] = append(workTypes, chainingTraceFanInWorkTypes()...)
		workstations := cfg["workstations"].([]any)
		cfg["workstations"] = append(workstations, chainingTraceFanInWorkstation())
	})
	writeWorkstationConfig(t, dir, "merge", "---\ntype: MODEL_WORKSTATION\n---\nMerge the completed work.\n")

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "merged lineage COMPLETE"}},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	return h, provider
}

func chainingTraceFanInWorkTypes() []any {
	return []any{
		map[string]any{
			"name": "left",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
			},
		},
		map[string]any{
			"name": "right",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
			},
		},
		map[string]any{
			"name": "merged",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
			},
		},
	}
}

func chainingTraceFanInWorkstation() map[string]any {
	return map[string]any{
		"name": "merge",
		"inputs": []any{
			map[string]any{"state": "init", "workType": "left"},
			map[string]any{"state": "init", "workType": "right"},
		},
		"outputs": []any{
			map[string]any{"state": "complete", "workType": "merged"},
		},
		"worker": "processor",
	}
}

func submitChainingTraceFanInWork(t *testing.T, h *testutil.ServiceTestHarness) {
	t.Helper()

	h.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-chaining-fan-in-smoke",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{
				Name:                   "lineage-z",
				WorkID:                 "work-lineage-z",
				WorkTypeID:             "left",
				CurrentChainingTraceID: "chain-z",
				TraceID:                "chain-z",
				Payload:                "lineage z",
			},
			{
				Name:                   "lineage-a",
				WorkID:                 "work-lineage-a",
				WorkTypeID:             "right",
				CurrentChainingTraceID: "chain-a",
				TraceID:                "chain-a",
				Payload:                "lineage a",
			},
		},
	})
	h.RunUntilComplete(t, 10*time.Second)
}

func assertChainingTraceFanInHarnessState(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	provider *testutil.MockWorkerMapProvider,
) {
	t.Helper()

	h.Assert().
		PlaceTokenCount("merged:complete", 1).
		HasNoTokenInPlace("left:init").
		HasNoTokenInPlace("right:init")
	if provider.CallCount("processor") != 1 {
		t.Fatalf("processor call count = %d, want 1", provider.CallCount("processor"))
	}
}

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

// TestFactoryRequestBatch_BatchSubmissionAtomic verifies that batch
// normalization is all-or-nothing: if validation fails, no requests are produced.
func TestFactoryRequestBatch_BatchSubmissionAtomic(t *testing.T) {
	validWorkTypes := map[string]bool{"task": true}

	// A batch with one valid and one invalid work item should be rejected entirely.
	invalidInput := interfaces.WorkRequest{
		RequestID: "request-atomic-invalid",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{WorkTypeID: "task", Name: "valid-item"},
			{WorkTypeID: "task", Name: ""}, // invalid: missing name
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

	// Confirm: valid batch produces correct count.
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

	// Verify each request has a deterministic WorkID.
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

// --- helpers ---

// capturingExecutor captures the last dispatch and returns a fixed result.
type capturingExecutor struct {
	result       interfaces.WorkResult
	lastDispatch interfaces.WorkDispatch
	callCount    int
}

func (e *capturingExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.lastDispatch = dispatch
	e.callCount++
	result := e.result
	result.DispatchID = dispatch.DispatchID
	result.TransitionID = dispatch.TransitionID
	return result, nil
}

func assertExternalBatchSmokeEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	requestIndex := -1
	requestEvents := 0
	relationCount := 0
	relationIndex := -1

	for i, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil || eventString(event.Context.RequestId) != "request-e2e-batch-smoke" {
				continue
			}
			requestEvents++
			requestIndex = i
			if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
				t.Fatalf("external request type = %q, want FACTORY_REQUEST_BATCH", payload.Type)
			}
			if eventString(payload.Source) != "external-submit" {
				t.Fatalf("external request source = %q, want external-submit", eventString(payload.Source))
			}
			if payload.ParentLineage != nil && len(*payload.ParentLineage) != 0 {
				t.Fatalf("external request parent lineage = %#v, want none", *payload.ParentLineage)
			}
			if payload.Works == nil || len(*payload.Works) != 2 {
				t.Fatalf("external request work items = %d, want 2", eventWorkCount(payload.Works))
			}
		case factoryapi.FactoryEventTypeRelationshipChangeRequest:
			payload, err := event.Payload.AsRelationshipChangeRequestEventPayload()
			if err != nil || eventString(event.Context.RequestId) != "request-e2e-batch-smoke" {
				continue
			}
			relationCount++
			relationIndex = i
			relation := payload.Relation
			if relation.Type != factoryapi.RelationTypeDependsOn ||
				relation.SourceWorkName != "second" ||
				eventString(relation.TargetWorkId) != "work-e2e-first" {
				t.Fatalf("external relation payload = %#v, want second DEPENDS_ON first", relation)
			}
		}
	}

	if requestIndex == -1 {
		t.Fatal("missing external WORK_REQUEST event")
	}
	if requestEvents != 1 {
		t.Fatalf("external WORK_REQUEST events = %d, want 1", requestEvents)
	}
	if relationCount != 1 {
		t.Fatalf("external RELATIONSHIP_CHANGE events = %d, want 1", relationCount)
	}
	if relationIndex <= requestIndex {
		t.Fatalf("external WORK_REQUEST index %d should precede RELATIONSHIP_CHANGE index %d", requestIndex, relationIndex)
	}
}

func assertSecondWorkDispatchedAfterFirstTerminal(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	firstTerminalIndex := -1
	secondProcessRequestIndex := -1

	for i, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil || !eventStringSliceContains(event.Context.WorkIds, "work-e2e-first") {
				continue
			}
			if payload.TransitionId == "finish" && payload.Outcome == factoryapi.WorkOutcomeAccepted {
				firstTerminalIndex = i
			}
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil || payload.TransitionId != "process" {
				continue
			}
			if dispatchCreatedIncludesWork(payload, "work-e2e-second") {
				secondProcessRequestIndex = i
			}
		}
	}

	if firstTerminalIndex == -1 {
		t.Fatal("missing terminal response for first external work")
	}
	if secondProcessRequestIndex == -1 {
		t.Fatal("missing process dispatch for second external work")
	}
	if secondProcessRequestIndex <= firstTerminalIndex {
		t.Fatalf("second process dispatch index %d should be after first terminal index %d", secondProcessRequestIndex, firstTerminalIndex)
	}
}

func assertWorkerGeneratedBatchEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	const generatedRequestID = "request-e2e-generated-batch"

	generatedRequestIndex := -1
	generatedRequestEvents := 0
	generatedRelations := 0

	for i, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil {
				continue
			}
			if eventString(event.Context.RequestId) != generatedRequestID {
				continue
			}
			generatedRequestIndex = i
			generatedRequestEvents++
			assertGeneratedWorkRequestPayload(t, payload)
		case factoryapi.FactoryEventTypeRelationshipChangeRequest:
			payload, err := event.Payload.AsRelationshipChangeRequestEventPayload()
			if err != nil || eventString(event.Context.RequestId) != generatedRequestID {
				continue
			}
			if payload.Relation.SourceWorkName == "generated-beta" &&
				eventString(payload.Relation.TargetWorkId) == "work-e2e-generated-alpha" {
				generatedRelations++
			}
		}
	}

	if generatedRequestIndex == -1 {
		t.Fatal("missing worker-generated WORK_REQUEST event")
	}
	if generatedRequestEvents != 1 {
		t.Fatalf("generated WORK_REQUEST events = %d, want 1", generatedRequestEvents)
	}
	if generatedRelations != 1 {
		t.Fatalf("generated RELATIONSHIP_CHANGE events = %d, want 1", generatedRelations)
	}
}

func assertWorkerGeneratedBatchWorldState(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	worldState, err := projections.ReconstructFactoryWorldState(events, lastFactoryEventTick(events))
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	for _, workID := range []string{"work-e2e-generated-alpha", "work-e2e-generated-beta"} {
		item, ok := worldState.WorkItemsByID[workID]
		if !ok {
			t.Fatalf("world state missing generated work item %q", workID)
		}
		if item.CurrentChainingTraceID != "trace-e2e-batch-smoke" {
			t.Fatalf("world state work %q current chaining trace ID = %q, want trace-e2e-batch-smoke", workID, item.CurrentChainingTraceID)
		}
		assertStringSliceEqual(t, "world state generated previous chaining trace IDs", item.PreviousChainingTraceIDs, []string{"trace-e2e-batch-smoke"})
	}
}

func assertGeneratedWorkRequestPayload(t *testing.T, payload factoryapi.WorkRequestEventPayload) {
	t.Helper()

	if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("generated request type = %q, want FACTORY_REQUEST_BATCH", payload.Type)
	}
	if eventString(payload.Source) != "generator:e2e-smoke" {
		t.Fatalf("generated request source = %q, want generator:e2e-smoke", eventString(payload.Source))
	}
	if got := strings.Join(eventStringSlice(payload.ParentLineage), ","); got != "request-e2e-batch-smoke,work-e2e-second" {
		t.Fatalf("generated parent lineage = %#v, want external request/work lineage", eventStringSlice(payload.ParentLineage))
	}
	relations := eventRelations(payload.Relations)
	if len(relations) != 1 ||
		relations[0].SourceWorkName != "generated-beta" ||
		relations[0].TargetWorkName != "generated-alpha" ||
		eventString(relations[0].RequiredState) != "complete" {
		t.Fatalf("generated relation context = %#v, want generated-beta depends on generated-alpha", relations)
	}
	if len(eventWorks(payload.Works)) != 2 {
		t.Fatalf("generated request work items = %d, want 2", len(eventWorks(payload.Works)))
	}
	for _, item := range eventWorks(payload.Works) {
		if eventString(item.CurrentChainingTraceId) != "trace-e2e-batch-smoke" {
			t.Fatalf("generated work item %q current chaining trace ID = %q, want trace-e2e-batch-smoke", eventString(item.WorkId), eventString(item.CurrentChainingTraceId))
		}
		if got := eventStringSlice(item.PreviousChainingTraceIds); len(got) != 1 || got[0] != "trace-e2e-batch-smoke" {
			t.Fatalf("generated work item %q previous chaining trace IDs = %#v, want [trace-e2e-batch-smoke]", eventString(item.WorkId), got)
		}
		tags := eventTags(item.Tags)
		if tags["_parent_request_id"] != "request-e2e-batch-smoke" {
			t.Fatalf("generated work item %q parent request tag = %q, want request-e2e-batch-smoke", eventString(item.WorkId), tags["_parent_request_id"])
		}
		if tags["_parent_work_id"] != "work-e2e-second" {
			t.Fatalf("generated work item %q parent work tag = %q, want work-e2e-second", eventString(item.WorkId), tags["_parent_work_id"])
		}
		if tags["_source_dispatch_id"] == "" {
			t.Fatalf("generated work item %q missing source dispatch tag", eventString(item.WorkId))
		}
		switch eventString(item.WorkId) {
		case "work-e2e-generated-alpha":
			if tags["runtime"] != "alpha" {
				t.Fatalf("generated alpha runtime tag = %q, want alpha", tags["runtime"])
			}
		case "work-e2e-generated-beta":
			if tags["runtime"] != "beta" {
				t.Fatalf("generated beta runtime tag = %q, want beta", tags["runtime"])
			}
		}
	}
}

func assertChainingTraceFanInEvents(t *testing.T, events []factoryapi.FactoryEvent) (string, string) {
	t.Helper()

	var requestPayload *factoryapi.DispatchRequestEventPayload
	var responsePayload *factoryapi.DispatchResponseEventPayload
	dispatchID := ""

	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil || payload.TransitionId != "merge" {
				continue
			}
			dispatchID = eventString(event.Context.DispatchId)
			requestPayload = &payload
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil || payload.TransitionId != "merge" {
				continue
			}
			responsePayload = &payload
		}
	}

	if requestPayload == nil {
		t.Fatal("missing merge dispatch request event")
	}
	if responsePayload == nil {
		t.Fatal("missing merge dispatch response event")
	}
	if len(requestPayload.Inputs) != 2 {
		t.Fatalf("merge dispatch request inputs = %#v, want two consumed inputs", requestPayload.Inputs)
	}

	currentChainingTraceID := eventString(requestPayload.CurrentChainingTraceId)
	if currentChainingTraceID != "chain-z" {
		t.Fatalf("dispatch request current chaining trace ID = %q, want chain-z", currentChainingTraceID)
	}
	assertStringSliceEqual(t, "dispatch request previous chaining trace IDs", eventStringSlice(requestPayload.PreviousChainingTraceIds), []string{"chain-a", "chain-z"})

	if eventString(responsePayload.CurrentChainingTraceId) != currentChainingTraceID {
		t.Fatalf("dispatch response current chaining trace ID = %q, want %q", eventString(responsePayload.CurrentChainingTraceId), currentChainingTraceID)
	}
	assertStringSliceEqual(t, "dispatch response previous chaining trace IDs", eventStringSlice(responsePayload.PreviousChainingTraceIds), []string{"chain-a", "chain-z"})
	if responsePayload.OutputWork == nil || len(*responsePayload.OutputWork) != 1 {
		t.Fatalf("dispatch response output work = %#v, want one merged output work item", responsePayload.OutputWork)
	}

	output := (*responsePayload.OutputWork)[0]
	if eventString(output.CurrentChainingTraceId) != currentChainingTraceID {
		t.Fatalf("output work current chaining trace ID = %q, want %q", eventString(output.CurrentChainingTraceId), currentChainingTraceID)
	}
	assertStringSliceEqual(t, "output work previous chaining trace IDs", eventStringSlice(output.PreviousChainingTraceIds), []string{"chain-a", "chain-z"})
	return dispatchID, currentChainingTraceID
}

func assertChainingTraceFanInWorldState(t *testing.T, events []factoryapi.FactoryEvent, dispatchID string, currentChainingTraceID string) {
	t.Helper()

	worldState, err := projections.ReconstructFactoryWorldState(events, lastFactoryEventTick(events))
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	var completion *interfaces.FactoryWorldDispatchCompletion
	for i := range worldState.CompletedDispatches {
		if worldState.CompletedDispatches[i].DispatchID == dispatchID {
			completion = &worldState.CompletedDispatches[i]
			break
		}
	}
	if completion == nil {
		t.Fatalf("world state missing completed dispatch %q", dispatchID)
	}

	if completion.CurrentChainingTraceID != currentChainingTraceID {
		t.Fatalf("world state completion current chaining trace ID = %q, want %q", completion.CurrentChainingTraceID, currentChainingTraceID)
	}
	assertStringSliceEqual(t, "world state completion previous chaining trace IDs", completion.PreviousChainingTraceIDs, []string{"chain-a", "chain-z"})
	if len(completion.OutputWorkItems) != 1 {
		t.Fatalf("world state completion output work items = %#v, want one merged output", completion.OutputWorkItems)
	}

	output := completion.OutputWorkItems[0]
	if output.CurrentChainingTraceID != currentChainingTraceID {
		t.Fatalf("world state output current chaining trace ID = %q, want %q", output.CurrentChainingTraceID, currentChainingTraceID)
	}
	assertStringSliceEqual(t, "world state output previous chaining trace IDs", output.PreviousChainingTraceIDs, []string{"chain-a", "chain-z"})
	projected := worldState.WorkItemsByID[output.ID]
	assertStringSliceEqual(t, "world state projected output previous chaining trace IDs", projected.PreviousChainingTraceIDs, []string{"chain-a", "chain-z"})
}

func assertStringSliceEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
}

func dispatchCreatedIncludesWork(payload factoryapi.DispatchRequestEventPayload, workID string) bool {
	for _, input := range payload.Inputs {
		if input.WorkId == workID {
			return true
		}
	}
	return false
}

func eventString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func eventStringSlice(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}

func eventStringSliceContains(values *[]string, want string) bool {
	for _, value := range eventStringSlice(values) {
		if value == want {
			return true
		}
	}
	return false
}

func eventWorkCount(works *[]factoryapi.Work) int {
	if works == nil {
		return 0
	}
	return len(*works)
}

func eventWorks(works *[]factoryapi.Work) []factoryapi.Work {
	if works == nil {
		return nil
	}
	return *works
}

func eventRelations(relations *[]factoryapi.Relation) []factoryapi.Relation {
	if relations == nil {
		return nil
	}
	return *relations
}

func eventTags(tags *factoryapi.StringMap) map[string]string {
	if tags == nil {
		return nil
	}
	return map[string]string(*tags)
}
