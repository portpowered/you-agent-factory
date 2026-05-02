package functional_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
)

// scaffoldFactory creates a temp factory directory with factory.json and the
// multi-channel inputs/<work-type>/default/ directory structure.
// It also creates workstations/<name>/AGENTS.md files for each workstation
// declared in the config so that loadWorkersFromConfig can find them.
// Returns the factory root directory path.
func scaffoldFactory(t *testing.T, cfg map[string]any) string {
	t.Helper()
	dir := t.TempDir()

	// Write factory.json.
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	// Create workstations/<name>/AGENTS.md for each workstation in the config.
	if workstations, ok := cfg["workstations"].([]map[string]any); ok {
		for _, ws := range workstations {
			name, _ := ws["name"].(string)
			if name == "" {
				continue
			}
			wsDir := filepath.Join(dir, "workstations", name)
			if err := os.MkdirAll(wsDir, 0o755); err != nil {
				t.Fatalf("create workstation dir %s: %v", name, err)
			}
			agentsMD := "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"
			if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
				t.Fatalf("write workstation AGENTS.md for %s: %v", name, err)
			}
		}
	}

	return dir
}

// simplePipelineConfig returns a factory config for a single-transition
// pipeline: task:init → task:complete (with task:failed for failures).
func simplePipelineConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
		},
	}
}

// twoStagePipelineConfig returns a factory config with two transitions:
// task:init → task:processing → task:complete.
func twoStagePipelineConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "processing", "type": "PROCESSING"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
			{"name": "worker-b"},
		},
		"workstations": []map[string]any{
			{
				"name":      "step-one",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "processing"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
			{
				"name":      "step-two",
				"worker":    "worker-b",
				"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
		},
	}
}

// TestIntegration_EndToEnd_InitInputCompletion boots a real FactoryService
// with default accepted mock workers and verifies a pre-seeded work item flows
// through to completion.
func TestIntegration_EndToEnd_InitInputCompletion(t *testing.T) {
	dir := scaffoldFactory(t, simplePipelineConfig())

	// Create the multi-channel inputs directory and pre-seed the work before
	// startup. Idle factories now complete immediately, so the work must exist
	// when Run begins.
	inputDir := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create input dir: %v", err)
	}
	workFile := filepath.Join(inputDir, "work-1.json")
	if err := os.WriteFile(workFile, []byte(`{"title": "integration test"}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger := zap.NewNop()
	svc, err := service.BuildFactoryService(ctx, &service.FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            logger,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	// Run blocks until the factory completes (all tokens terminal) or ctx times out.
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("FactoryService.Run: %v", err)
	}

	// Verify the token reached the terminal state.
	snap, err := svc.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	if len(snap.Marking.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(snap.Marking.Tokens))
	}

	for _, tok := range snap.Marking.Tokens {
		if tok.PlaceID != "task:complete" {
			t.Errorf("expected token in task:complete, got %s", tok.PlaceID)
		}
	}

	// Verify factory reached completed state.
	if snap.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Errorf("expected COMPLETED state, got %s", snap.FactoryState)
	}
}

// TestIntegration_EndToEnd_WorkFileSubmission boots a FactoryService
// using the WorkFile config to submit initial work, verifying the
// programmatic work submission path.
func TestIntegration_EndToEnd_WorkFileSubmission(t *testing.T) {
	dir := scaffoldFactory(t, twoStagePipelineConfig())

	// Create the inputs/ directory for the file watcher.
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	// Write initial work file.
	workFilePath := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    json.RawMessage(`{"title": "work-file test"}`),
	}
	writeWorkRequestFileForFunctionalTest(t, workFilePath, work)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	svc, err := service.BuildFactoryService(ctx, &service.FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		WorkFile:          workFilePath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if err := svc.Run(ctx); err != nil {
		t.Fatalf("FactoryService.Run: %v", err)
	}

	// Verify the token flowed through both stages to completion.
	snap, err := svc.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	if len(snap.Marking.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(snap.Marking.Tokens))
	}
	for _, tok := range snap.Marking.Tokens {
		if tok.PlaceID != "task:complete" {
			t.Errorf("expected token in task:complete, got %s", tok.PlaceID)
		}
	}
}

// TestIntegration_Preseed_SubmitsExistingFilesOnStartup validates the preseed
// feature: files that exist in the inputs directory BEFORE the factory starts
// are submitted automatically on startup, without requiring a POST /work call.
//
//	Given: a factory directory with a pre-staged .json file in inputs/task/default/
//	When:  the factory service starts with default accepted mock workers
//	Then:  the pre-staged file is picked up and work reaches terminal completion
func TestIntegration_Preseed_SubmitsExistingFilesOnStartup(t *testing.T) {
	dir := scaffoldFactory(t, simplePipelineConfig())

	// Create the multi-channel inputs directory and drop a work file BEFORE
	// the factory service is constructed (simulating pre-staged work).
	inputDir := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create input dir: %v", err)
	}
	workFile := filepath.Join(inputDir, "preseed-work.json")
	if err := os.WriteFile(workFile, []byte(`{"title": "preseed test"}`), 0o644); err != nil {
		t.Fatalf("write preseed work file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	svc, err := service.BuildFactoryService(ctx, &service.FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	// Run without submitting any work via HTTP — preseed should handle it.
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("FactoryService.Run: %v", err)
	}

	// Verify the pre-staged token reached the terminal state.
	snap, err := svc.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.Marking.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(snap.Marking.Tokens))
	}
	for _, tok := range snap.Marking.Tokens {
		if tok.PlaceID != "task:complete" {
			t.Errorf("expected token in task:complete, got %s", tok.PlaceID)
		}
	}

	if snap.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Errorf("expected COMPLETED, got %s", snap.FactoryState)
	}
}

// TestIntegration_Preseed_EmptyDirectoryCompletesImmediately validates that an
// empty inputs directory now completes immediately because there is no work in
// the system.
func TestIntegration_Preseed_EmptyDirectoryCompletesImmediately(t *testing.T) {
	dir := scaffoldFactory(t, simplePipelineConfig())

	// Create an empty inputs directory.
	inputDir := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create input dir: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	svc, err := service.BuildFactoryService(ctx, &service.FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if err := svc.Run(ctx); err != nil {
		t.Fatalf("FactoryService.Run: %v", err)
	}
	snap, err := svc.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.Marking.Tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(snap.Marking.Tokens))
	}
	if snap.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Errorf("expected COMPLETED state, got %s", snap.FactoryState)
	}
}

// TestIntegration_WaitToComplete_SeedFileAsync demonstrates the recommended
// test pattern: write a seed file to the inputs directory, start the service
// via Run() in a background goroutine, and use WaitToComplete() with a timeout
// to block until all tokens reach terminal places.
//
// This validates that FactoryService.WaitToComplete() correctly signals
// completion when work enters exclusively through the file watcher.
func TestIntegration_WaitToComplete_SeedFileAsync(t *testing.T) {
	dir := scaffoldFactory(t, simplePipelineConfig())

	// Create the multi-channel inputs directory and drop a seed file BEFORE
	// starting the service (preseed path).
	inputDir := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create input dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(inputDir, "seed-work.json"),
		[]byte(`{"title": "wait-to-complete demo"}`),
		0o644,
	); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	svc, err := service.BuildFactoryService(ctx, &service.FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	// Start Run() in a background goroutine — does not block.
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()

	// Block until WaitToComplete signals or timeout.
	select {
	case <-svc.WaitToComplete():
		// Success — all tokens reached terminal places.
		cancel()
	case <-ctx.Done():
		t.Fatal("timed out waiting for WaitToComplete()")
	}

	// Drain the Run goroutine.
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error: %v", err)
	}

	// Verify the seed file's token reached the terminal state.
	snap, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.Marking.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(snap.Marking.Tokens))
	}
	for _, tok := range snap.Marking.Tokens {
		if tok.PlaceID != "task:complete" {
			t.Errorf("expected token in task:complete, got %s", tok.PlaceID)
		}
	}

	// Verify factory reached completed state.
	if snap.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Errorf("expected COMPLETED state, got %s", snap.FactoryState)
	}
}

// TestCopyFixtureDir_ParallelIsolation demonstrates that two parallel subtests
// using CopyFixtureDir on the same source fixture do not interfere with each
// other. Each subtest gets its own isolated directory copy, writes a different
// seed file, and verifies only its own token reaches the terminal state.
func TestCopyFixtureDir_ParallelIsolation(t *testing.T) {
	srcDir := createParallelIsolationSourceFixture(t)

	for _, tc := range []struct {
		name    string
		payload string
	}{
		{name: "subtest-alpha", payload: `{"title":"alpha-work"}`},
		{name: "subtest-beta", payload: `{"title":"beta-work"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runParallelIsolationFixtureCopy(t, srcDir, tc.payload)
		})
	}
}

func createParallelIsolationSourceFixture(t *testing.T) string {
	t.Helper()

	srcDir := scaffoldFactory(t, simplePipelineConfig())
	workerDir := filepath.Join(srcDir, "workers", "worker-a")
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKER\nstop_tokens:\n  - COMPLETE\n---\nProcess work.\n"
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}
	return srcDir
}

func runParallelIsolationFixtureCopy(t *testing.T, srcDir, payload string) {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, srcDir)
	writeParallelIsolationSeed(t, dir, payload)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	svc := buildParallelIsolationService(t, ctx, dir)
	errCh := runFunctionalService(ctx, svc)
	waitForFunctionalServiceCompletion(t, ctx, cancel, svc, errCh)
	assertSingleCompletedTaskToken(t, svc)
}

func writeParallelIsolationSeed(t *testing.T, dir, payload string) {
	t.Helper()

	inputDir := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create input dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "seed.json"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
}

func buildParallelIsolationService(t *testing.T, ctx context.Context, dir string) *service.FactoryService {
	t.Helper()

	svc, err := service.BuildFactoryService(ctx, &service.FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	return svc
}

func runFunctionalService(ctx context.Context, svc *service.FactoryService) chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()
	return errCh
}

func waitForFunctionalServiceCompletion(
	t *testing.T,
	ctx context.Context,
	cancel context.CancelFunc,
	svc *service.FactoryService,
	errCh <-chan error,
) {
	t.Helper()

	select {
	case <-svc.WaitToComplete():
		cancel()
	case <-ctx.Done():
		t.Fatal("timed out waiting for completion")
	}
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error: %v", err)
	}
}

func assertSingleCompletedTaskToken(t *testing.T, svc *service.FactoryService) {
	t.Helper()

	snap, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.Marking.Tokens) != 1 {
		t.Fatalf("expected exactly 1 token, got %d", len(snap.Marking.Tokens))
	}
	for _, tok := range snap.Marking.Tokens {
		if tok.PlaceID != "task:complete" {
			t.Errorf("expected token in task:complete, got %s", tok.PlaceID)
		}
	}
}
