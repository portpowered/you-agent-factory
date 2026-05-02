//go:build functionallong

package smoke

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
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
)

func TestServiceLifecycle_InitInputCompletesThroughFactoryService(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-lifecycle init-input sweep")
	dir := support.ScaffoldFactory(t, simpleServicePipelineConfig())

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

	svc := buildFunctionalService(t, ctx, dir)
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("FactoryService.Run: %v", err)
	}

	assertSingleCompletedTaskToken(t, context.Background(), svc)

	snap, err := svc.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snap.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Errorf("expected COMPLETED state, got %s", snap.FactoryState)
	}
}

func TestServiceLifecycle_WorkFileSubmissionCompletesTwoStagePipeline(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-lifecycle work-file sweep")
	dir := support.ScaffoldFactory(t, twoStageServicePipelineConfig())

	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFilePath := filepath.Join(dir, "initial-work.json")
	support.WriteWorkRequestFile(t, workFilePath, interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    json.RawMessage(`{"title": "work-file test"}`),
	})

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

	assertSingleCompletedTaskToken(t, ctx, svc)
}

func TestServiceLifecycle_PreseededWorkCompletesOnStartup(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-lifecycle preseeded-startup sweep")
	dir := support.ScaffoldFactory(t, simpleServicePipelineConfig())

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

	svc := buildFunctionalService(t, ctx, dir)
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("FactoryService.Run: %v", err)
	}

	assertSingleCompletedTaskToken(t, ctx, svc)

	snap, err := svc.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snap.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Errorf("expected COMPLETED, got %s", snap.FactoryState)
	}
}

func TestServiceLifecycle_EmptyPreseedDirectoryCompletesImmediately(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-lifecycle empty-preseed sweep")
	dir := support.ScaffoldFactory(t, simpleServicePipelineConfig())

	inputDir := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create input dir: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	svc := buildFunctionalService(t, ctx, dir)
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

func TestServiceLifecycle_WaitToCompleteSignalsForSeededWatcherInput(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-lifecycle wait-to-complete sweep")
	dir := support.ScaffoldFactory(t, simpleServicePipelineConfig())

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

	svc := buildFunctionalService(t, ctx, dir)
	errCh := runFunctionalService(ctx, svc)

	select {
	case <-svc.WaitToComplete():
		cancel()
	case <-ctx.Done():
		t.Fatal("timed out waiting for WaitToComplete()")
	}

	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error: %v", err)
	}

	assertSingleCompletedTaskToken(t, context.Background(), svc)

	snap, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snap.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Errorf("expected COMPLETED state, got %s", snap.FactoryState)
	}
}

func TestServiceLifecycle_CopyFixtureDirParallelCopiesStayIsolated(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-lifecycle fixture-copy isolation sweep")
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

			dir := testutil.CopyFixtureDir(t, srcDir)
			writeParallelIsolationSeed(t, dir, tc.payload)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			svc := buildFunctionalService(t, ctx, dir)
			errCh := runFunctionalService(ctx, svc)
			waitForFunctionalServiceCompletion(t, ctx, cancel, svc, errCh)
			assertSingleCompletedTaskToken(t, context.Background(), svc)
		})
	}
}

func simpleServicePipelineConfig() map[string]any {
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

func twoStageServicePipelineConfig() map[string]any {
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

func createParallelIsolationSourceFixture(t *testing.T) string {
	t.Helper()

	srcDir := support.ScaffoldFactory(t, simpleServicePipelineConfig())
	support.WriteAgentConfig(t, srcDir, "worker-a", "---\ntype: MODEL_WORKER\nstop_tokens:\n  - COMPLETE\n---\nProcess work.\n")
	return srcDir
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

func buildFunctionalService(t *testing.T, ctx context.Context, dir string) *service.FactoryService {
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

func assertSingleCompletedTaskToken(t *testing.T, ctx context.Context, svc *service.FactoryService) {
	t.Helper()

	snap, err := svc.GetEngineStateSnapshot(ctx)
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
