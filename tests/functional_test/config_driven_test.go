package functional_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/api"
	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"go.uber.org/zap"
)

// TestConfigDriven_HappyPath validates a two-stage pipeline through the full
// service layer: BuildFactoryService → WorkstationExecutor → AgentExecutor →
// mock Provider. Work enters via a seed file picked up by preseed.
func TestConfigDriven_HappyPath(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "happy_path"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Config-driven happy path"}`))

	// Both workers use stop_token: COMPLETE — include it in response Content.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Step one done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Step two done. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:failed").
		TokenCount(1)

	// Verify the provider was called twice (once per worker in the pipeline).
	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}
}

// TestConfigDriven_HappyPath_FailureRouting verifies that a provider error
// routes the token to the failed state via the config-driven on_failure field.
func TestConfigDriven_HappyPath_FailureRouting(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "happy_path"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will fail"}`))

	// Provider returns an error on the first call → AgentExecutor maps to OutcomeFailed.
	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{{Content: ""}},
		[]error{fmt.Errorf("something went wrong")},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete").
		TokenCount(1)
}

// TestConfigDriven_RetryLoopBreaker validates a rejection loop with a guarded
// loop breaker. Reviewer responses omit "ACCEPTED" -> REJECTED, triggering
// on_rejection back to init. After 3 reviewer rejections the loop breaker
// fires -> task:failed.
func TestConfigDriven_RetryLoopBreaker(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "retry_exhaustion"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will exhaust retries"}`))

	// Interleaved responses: processor (COMPLETE→accept), reviewer (no ACCEPTED→reject), ...
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"}, // processor 1 → ACCEPTED
		interfaces.InferenceResponse{Content: "Needs work"},          // reviewer 1 → REJECTED
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"}, // processor 2 → ACCEPTED
		interfaces.InferenceResponse{Content: "Still needs work"},    // reviewer 2 → REJECTED
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"}, // processor 3 -> ACCEPTED
		interfaces.InferenceResponse{Content: "Not good enough"},     // reviewer 3 -> REJECTED -> loop breaker
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	// Token should be in failed state due to the guarded loop breaker.
	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:in-review").
		HasNoTokenInPlace("task:complete")

	// 6 provider calls total: 3 processor + 3 reviewer, interleaved.
	if provider.CallCount() != 6 {
		t.Errorf("expected provider called 6 times, got %d", provider.CallCount())
	}

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "review-exhaustion", "task:failed")
}

// TestConfigDriven_RetryLoopBreaker_SucceedsBeforeLimit verifies that if the
// reviewer accepts before the loop-breaker limit, the token completes normally.
func TestConfigDriven_RetryLoopBreaker_SucceedsBeforeLimit(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "retry_exhaustion"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will succeed on second try"}`))

	// Interleaved: processor accept, reviewer reject, processor accept, reviewer accept.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"},  // processor 1 → ACCEPTED
		interfaces.InferenceResponse{Content: "Needs work"},           // reviewer 1 → REJECTED
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"},  // processor 2 → ACCEPTED
		interfaces.InferenceResponse{Content: "Looks good. ACCEPTED"}, // reviewer 2 → ACCEPTED
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

// TestConfigDriven_ResourceContention validates that with a resource of
// capacity 1, two work items both complete via serialized access to the
// shared resource. Work enters via two seed files.
func TestConfigDriven_ResourceContention(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "resource_contention"))

	// Write two seed files that compete for a single resource slot.
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Work item A"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Work item B"}`))

	// Processor uses stop_token: COMPLETE. Two items need processing.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	// Both tokens should reach complete.
	h.Assert().PlaceTokenCount("task:complete", 2)

	// Provider called twice (once per work item, serialized by resource).
	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times total, got %d", provider.CallCount())
	}
}

// TestConfigDriven_DynamicFanout_ThreeChildren validates dynamic fanout:
// submit a chapter, parser spawns 3 pages, processor and completer run
// via mock Provider, and the per-input guard fires after all pages complete.
// Work enters via a seed file.
func TestConfigDriven_DynamicFanout_ThreeChildren(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dynamic_fanout"))

	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Config-driven fanout"}`))

	// Processor (3 pages) + completer (1 guard release) = 4 provider calls.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Page 1 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Page 2 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Page 3 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Chapter finalized. COMPLETE"},
	)

	parserExec := &fanoutParserExecutor{childCount: 3}

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(factory.WithWorkerExecutor("parser", parserExec)),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		PlaceTokenCount("page:complete", 3).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("page:init")

	if parserExec.callCount() != 1 {
		t.Errorf("expected parser called 1 time, got %d", parserExec.callCount())
	}
	if provider.CallCount() != 4 {
		t.Errorf("expected provider called 4 times, got %d", provider.CallCount())
	}
}

// TestConfigDriven_DynamicFanout_AnyChildFailedRoutesParent verifies that an
// any_child_failed per-input guard observes failed child work with parent
// context and routes the parent to failed without running the completion path.
func TestConfigDriven_DynamicFanout_AnyChildFailedRoutesParent(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dynamic_fanout"))

	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Child failure fan-in"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Chapter failure recorded. COMPLETE"},
	)

	parserExec := &fanoutParserExecutor{childCount: 3}
	processorExec := &failOnNthPageExecutor{failOn: 2}

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(
			factory.WithWorkerExecutor("parser", parserExec),
			factory.WithWorkerExecutor("processor", processorExec),
		),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	if err := h.RunUntilCompleteError(15 * time.Second); err != nil {
		marking := h.Marking()
		t.Fatalf("RunUntilComplete: %v; token places: %#v", err, tokenPlaces(*marking))
	}

	h.Assert().
		PlaceTokenCount("chapter:failed", 1).
		PlaceTokenCount("page:complete", 2).
		PlaceTokenCount("page:failed", 1).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("chapter:complete").
		HasNoTokenInPlace("page:init")

	if parserExec.callCount() != 1 {
		t.Errorf("expected parser called 1 time, got %d", parserExec.callCount())
	}
	if provider.CallCount() != 1 {
		t.Errorf("expected failure handler provider call only, got %d", provider.CallCount())
	}
}

// TestConfigDriven_DynamicFanout_OneChild validates dynamic fanout with a
// single child. Work enters via a seed file.
func TestConfigDriven_DynamicFanout_OneChild(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dynamic_fanout"))

	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Single child fanout"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Page done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Chapter finalized. COMPLETE"},
	)

	parserExec := &fanoutParserExecutor{childCount: 1}

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(factory.WithWorkerExecutor("parser", parserExec)),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		PlaceTokenCount("page:complete", 1).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("page:init")

	if parserExec.callCount() != 1 {
		t.Errorf("expected parser called 1 time, got %d", parserExec.callCount())
	}
	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}
}

// TestConfigDriven_DynamicFanout_ZeroChildren validates that the guard releases
// immediately when no children are spawned (expected_count=0).
// Work enters via a seed file.
func TestConfigDriven_DynamicFanout_ZeroChildren(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dynamic_fanout"))

	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Zero child fanout"}`))

	// Only completer fires (no pages to process).
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Chapter finalized. COMPLETE"},
	)

	parserExec := &fanoutParserExecutor{childCount: 0}

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(factory.WithWorkerExecutor("parser", parserExec)),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing")

	if parserExec.callCount() != 1 {
		t.Errorf("expected parser called 1 time, got %d", parserExec.callCount())
	}
}

// TestConfigDriven_DynamicFanout_ParentCompletes verifies the parent work item
// reaches the completed state after the per-input guard releases.
// Work enters via a seed file.
func TestConfigDriven_DynamicFanout_ParentCompletes(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "dynamic_fanout"))

	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Parent completion check"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Page 1 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Page 2 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Chapter finalized. COMPLETE"},
	)

	parserExec := &fanoutParserExecutor{childCount: 2}

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(factory.WithWorkerExecutor("parser", parserExec)),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	// Parent must be in complete state.
	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing")

	if provider.CallCount() != 3 {
		t.Errorf("expected provider called 3 times, got %d", provider.CallCount())
	}

	// All children must be in complete state.
	h.Assert().PlaceTokenCount("page:complete", 2)
}

// TestConfigDriven_AddWorkType validates that multiple independent work types
// process correctly. Work enters via seed files for each type.
func TestConfigDriven_AddWorkType(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_work_type"))

	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "New request"}`))
	testutil.WriteSeedFile(t, dir, "review", []byte(`{"title": "New review"}`))

	// Both workers use stop_token: COMPLETE — provider responses include it.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Request handled. COMPLETE"},
		interfaces.InferenceResponse{Content: "Review handled. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("request:complete").
		HasTokenInPlace("review:complete").
		HasNoTokenInPlace("request:init").
		HasNoTokenInPlace("review:init").
		PlaceTokenCount("request:complete", 1).
		PlaceTokenCount("review:complete", 1)

	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}
}

// TestConfigDriven_RESTAPISubmitAndQuery validates submitting work via the REST
// API and verifying completion via API response, using the full service layer.
// Work enters via a seed file and the REST API is queried for results.
func TestConfigDriven_RESTAPISubmitAndQuery(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "simple_pipeline"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "API test"}`))

	// Processor uses stop_token: COMPLETE.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Run to completion via the service layer harness.
	h.RunUntilComplete(t, 10*time.Second)

	// Verify the work completed via the harness first.
	h.Assert().HasTokenInPlace("task:complete").TokenCount(1)

	// Create a MockFactory with the final marking for the API server.
	snap := h.Marking()
	mockFactory := &testutil.MockFactory{Marking: snap}
	logger := zap.NewNop()

	// Create the API server backed by the completed factory.
	srv := api.NewServer(mockFactory, 0, logger)

	postWorkViaAPI(t, srv)
	assertListWorkResponse(t, srv)
}

func postWorkViaAPI(t *testing.T, srv *api.Server) {
	t.Helper()

	submitBody := `{"workTypeName": "task", "payload": {"title": "REST submit"}}`
	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(submitBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("POST /work: expected status 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var submitResp factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(rec.Body).Decode(&submitResp); err != nil {
		t.Fatalf("POST /work: failed to decode response: %v", err)
	}
	if submitResp.TraceId == "" {
		t.Error("POST /work: expected non-empty trace_id")
	}
}

func assertListWorkResponse(t *testing.T, srv *api.Server) {
	t.Helper()

	req := httptest.NewRequest("GET", "/work", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /work: expected status 200, got %d", rec.Code)
	}

	var listResp factoryapi.ListWorkResponse
	if err := json.NewDecoder(rec.Body).Decode(&listResp); err != nil {
		t.Fatalf("GET /work: failed to decode response: %v", err)
	}

	if len(listResp.Results) != 1 {
		t.Fatalf("GET /work: expected 1 result, got %d", len(listResp.Results))
	}

	// Verify the token shows as completed via the API response.
	token := listResp.Results[0]
	if token.WorkType != "task" {
		t.Errorf("GET /work: expected work_type 'task', got %q", token.WorkType)
	}
	if token.PlaceId != "task:complete" {
		t.Errorf("GET /work: expected place_id 'task:complete', got %q", token.PlaceId)
	}
}
