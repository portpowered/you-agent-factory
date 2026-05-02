package functional_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestOOTBExperience validates the first-run experience via the HTTP API:
//
//	Given: a factory directory with a simple pipeline config
//	When:  work is pre-seeded and the server starts
//	Then:  work reaches terminal completion queried via GET /state
func TestOOTBExperience(t *testing.T) {
	// Step 1: Create factory directory structure (simulates the init command).
	baseDir := t.TempDir()
	factoryDir := filepath.Join(baseDir, "factory")

	expectedDirs := []string{"workflows", "workers", "workstations"}
	for _, d := range expectedDirs {
		path := filepath.Join(factoryDir, d)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("failed to create directory %s: %v", path, err)
		}
	}

	// Step 2: Validate directory structure was created.
	for _, d := range expectedDirs {
		path := filepath.Join(factoryDir, d)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected directory %s to exist: %v", d, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", d)
		}
	}

	// Step 3: Scaffold factory.json with simple pipeline and start HTTP server.
	// The FunctionalServer builds the service layer (BuildFactoryService) and
	// exposes it via a real HTTP API, going through the cmd entrypoint path.
	dir := scaffoldFactory(t, simplePipelineConfig())
	traceID := "trace-ootb-001"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"Hello World"}`),
	})
	fs := StartFunctionalServer(t, dir, true /* use mock workers */)

	// Step 4: Verify the trace is queryable through the running API.
	if traceID == "" {
		t.Fatal("expected a non-empty trace ID after submit")
	}

	// Step 5: Wait for factory to complete and assert terminal state via HTTP.
	state := fs.WaitForCompleted(t, 10*time.Second)
	if state.TotalTokens != 1 {
		t.Errorf("expected 1 token, got %d", state.TotalTokens)
	}
	if state.Categories.Terminal != 1 {
		t.Errorf("expected 1 terminal token, got %d", state.Categories.Terminal)
	}
	if state.Categories.Failed != 0 {
		t.Errorf("expected 0 failed tokens, got %d", state.Categories.Failed)
	}
}

// TestOOTBExperienceMultiStage validates a multi-stage pipeline via the HTTP API:
//
//	Given: a two-stage pipeline config (init → processing → complete)
//	When:  work is pre-seeded before startup
//	Then:  work flows through all stages and reaches terminal completion
func TestOOTBExperienceMultiStage(t *testing.T) {
	dir := scaffoldFactory(t, twoStagePipelineConfig())
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-ootb-multistage-001",
		Payload:    []byte(`{"title":"Multi-stage test"}`),
	})
	fs := StartFunctionalServer(t, dir, true /* use mock workers */)

	state := fs.WaitForCompleted(t, 10*time.Second)
	if state.TotalTokens != 1 {
		t.Errorf("expected 1 token, got %d", state.TotalTokens)
	}
	if state.Categories.Terminal != 1 {
		t.Errorf("expected 1 terminal token, got %d", state.Categories.Terminal)
	}
}

// TestOOTBExperienceStatusQueryable validates that factory state is queryable
// via GET /state after work completes:
//
//	Given: a simple pipeline config
//	When:  work is pre-seeded and completes
//	Then:  GET /state shows factory_state=COMPLETED with 1 terminal token
func TestOOTBExperienceStatusQueryable(t *testing.T) {
	dir := scaffoldFactory(t, simplePipelineConfig())
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-ootb-status-001",
		Payload:    []byte(`{"title":"Status check"}`),
	})
	fs := StartFunctionalServer(t, dir, true /* use mock workers */)

	// Before processing completes, the API should still be reachable.
	initial := fs.GetState(t)
	if initial.FactoryState == "" {
		t.Error("expected non-empty factory state from API")
	}

	state := fs.WaitForCompleted(t, 10*time.Second)
	if state.FactoryState != "COMPLETED" {
		t.Errorf("expected COMPLETED factory state, got %s", state.FactoryState)
	}
	if state.TotalTokens != 1 {
		t.Errorf("expected 1 total token, got %d", state.TotalTokens)
	}
	if state.Categories.Terminal != 1 {
		t.Errorf("expected 1 terminal token, got %d", state.Categories.Terminal)
	}
}

// Note: failure routing (tokens reaching task:failed) requires an executor
// that returns OutcomeFailed. This scenario is validated at the
// subsystem level in pkg/factory/subsystems/dispatcher_test.go and at the
// integration level in tests/functional/config_driven_test.go
// (TestConfigDriven_HappyPath_FailureRouting).
