package internal

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// TestReplayExecutionImportsRuntimeRootOnly seals CUT-REC-RUN story 003: replay
// execution construction may depend on Factory Runtime only through the service
// root contract.

// TestReplayExecutionYieldsRuntimeRootHookAndPlanOutcomes proves replay-execution
// construction yields Runtime-facing hook and completion-plan outcomes through
// the sealed Runtime root boundary.
func TestReplayExecutionYieldsRuntimeRootHookAndPlanOutcomes(t *testing.T) {
	t.Parallel()

	artifact := loadReplayExecutionBoundaryArtifact(t)
	provider, runner, hooks, planner, err := NewReplayExecution(
		artifact,
		replayExecutionFactorySnapshotDecoder,
		replayExecutionRuntimeConfigDecoder,
	)
	if err != nil {
		t.Fatalf("NewReplayExecution: %v", err)
	}
	if provider == nil || runner == nil {
		t.Fatal("expected replay side-effect provider and command runner")
	}
	if len(hooks) != 2 {
		t.Fatalf("hook count = %d, want submission and work-state hooks", len(hooks))
	}
	if planner == nil {
		t.Fatal("expected completion delivery planner")
	}

	if hooks[0].Name() != factoryruntime.ReplaySubmissionHookName {
		t.Fatalf("submission hook name = %q, want %q", hooks[0].Name(), factoryruntime.ReplaySubmissionHookName)
	}
	if hooks[1].Name() != factoryruntime.ReplayWorkStateChangeHookName {
		t.Fatalf(
			"work-state hook name = %q, want %q",
			hooks[1].Name(),
			factoryruntime.ReplayWorkStateChangeHookName,
		)
	}
	if hooks[0].Priority() != -100 {
		t.Fatalf("submission hook priority = %d, want -100", hooks[0].Priority())
	}

	var submissionHook factoryruntime.ReplayHook = hooks[0]
	var workStateHook factoryruntime.ReplayHook = hooks[1]
	if submissionHook.Name() != factoryruntime.ReplaySubmissionHookName {
		t.Fatalf("typed submission hook name = %q, want %q", submissionHook.Name(), factoryruntime.ReplaySubmissionHookName)
	}
	if workStateHook.Name() != factoryruntime.ReplayWorkStateChangeHookName {
		t.Fatalf(
			"typed work-state hook name = %q, want %q",
			workStateHook.Name(),
			factoryruntime.ReplayWorkStateChangeHookName,
		)
	}

	result, err := submissionHook.OnTick(context.Background(), factoryruntime.ReplaySnapshot{Tick: 0})
	if err != nil {
		t.Fatalf("submission hook OnTick: %v", err)
	}
	if len(result.GeneratedBatches) != 0 {
		t.Fatalf("tick-0 generated batches = %d, want none before due submissions", len(result.GeneratedBatches))
	}
}

func loadReplayExecutionBoundaryArtifact(t *testing.T) *recordings.ReplayArtifact {
	t.Helper()

	storage := platformreplay.NewLocal(runtime.GOOS)
	path := filepath.FromSlash("replay/testdata/inference-events.replay.json")
	artifact, err := replayimpl.Load(storage, path, replayExecutionFactorySnapshotDecoder)
	if err != nil {
		t.Fatalf("Load replay fixture: %v", err)
	}
	return artifact
}

var replayExecutionFactorySnapshotDecoder replayimpl.SnapshotDecoder = func(
	data []byte,
) (*factorydefinitions.FactorySnapshot, error) {
	generated, err := factorymapping.GeneratedFactoryFromOpenAPIJSON(data)
	if err != nil {
		return nil, err
	}
	return factorydefinitions.NewFactorySnapshot(generated)
}

func replayExecutionRuntimeConfigDecoder(
	snapshot *factorydefinitions.FactorySnapshot,
) (replayimpl.ReplayRuntimeConfig, error) {
	var generated factoryapi.Factory
	if err := snapshot.Decode(&generated); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(generated)
	if err != nil {
		return nil, err
	}
	config, err := factorymapping.FactoryConfigFromOpenAPIJSON(payload)
	if err != nil {
		return nil, err
	}
	factoryDir := ""
	if generated.FactoryDirectory != nil {
		factoryDir = *generated.FactoryDirectory
	}
	return runtimefixtures.ReplayRuntimeConfigValue(config, factoryDir), nil
}
