package composebridge_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/composebridge"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestBuildCore_RejectsRecordAndReplayTogether(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := &runtimehost.Config{
		Dir:        t.TempDir(),
		RecordPath: "recording.json",
		ReplayPath: "recording.json",
		Logger:     zap.NewNop(),
	}

	core, err := composebridge.BuildCore(ctx, cfg)
	if core != nil {
		t.Fatal("expected BuildCore to return nil core for conflicting record/replay paths")
	}
	if err == nil {
		t.Fatal("expected BuildCore to fail for conflicting record/replay paths")
	}
}

func TestBuildCore_RejectsMissingFactoryConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := &runtimehost.Config{
		Dir:    t.TempDir(),
		Logger: zap.NewNop(),
	}

	core, err := composebridge.BuildCore(ctx, cfg)
	if core != nil {
		t.Fatal("expected BuildCore to return nil core without factory.json")
	}
	if err == nil {
		t.Fatal("expected BuildCore to fail without factory.json")
	}
}

func TestBuildCore_ComposesCoreForValidFactoryConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := context.Background()
	cfg := &runtimehost.Config{
		Dir:                                     dir,
		Logger:                                  zap.NewNop(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	}

	core, err := composebridge.BuildCore(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildCore: %v", err)
	}
	if core == nil {
		t.Fatal("expected composed core")
	}
	if core.Sessions() == nil || core.RuntimeBuild() == nil || core.WorkersScheduler() == nil {
		t.Fatal("expected session, runtime build, and workers scheduler collaborators on composed core")
	}
	if core.DurableExecution() == nil {
		t.Fatal("expected durable execution collaborator on composed core")
	}
	host := runtimehost.NewHostFromCore(core)
	if host.DurableExecutionService() != core.DurableExecution() {
		t.Fatal("runtime host did not receive the core-owned durable execution collaborator")
	}
	if composebridge.NewModelServiceFromCore(core) == nil {
		t.Fatal("expected model service from composed core")
	}
	if composebridge.NewFactoryDefinitionServiceFromCore(core) == nil {
		t.Fatal("expected factory definition service from composed core")
	}
	if err := composebridge.CloseRuntimeBundleSinks(nil, nil); err != nil {
		t.Fatalf("CloseRuntimeBundleSinks(nil): %v", err)
	}
}
