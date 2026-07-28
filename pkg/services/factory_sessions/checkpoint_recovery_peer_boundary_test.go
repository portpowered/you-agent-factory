package factorysessions_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const (
	factoryRuntimeRootImport         = "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryRuntimeCheckpointRecovery = factoryRuntimeRootImport + "/internal/services/checkpoint_recovery"
	factoryRuntimeCheckpointStore    = factoryRuntimeRootImport + "/checkpointstore"
	recordingsRootImport             = "github.com/portpowered/infinite-you/pkg/services/recordings"
)

var sessionsCheckpointPeerLeaseRoots = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/...",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/...",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/...",
}

var forbiddenSessionsCheckpointRecoveryImports = []string{
	factoryRuntimeCheckpointRecovery,
	factoryRuntimeCheckpointStore,
}

// sessionsResumeCoordinator stays on Recordings plus Runtime root checkpoint
// contracts without importing checkpoint_recovery package paths.
type sessionsResumeCoordinator struct {
	runtime factoryruntime.Service
}

func (c sessionsResumeCoordinator) resumeFromRecording(
	ctx context.Context,
	_ recordings.PortableRecording,
	checkpointID string,
) (factoryruntime.Checkpoint, error) {
	loaded, err := c.runtime.LoadCheckpoint(ctx, factoryruntime.LoadCheckpointRequest{CheckpointID: checkpointID})
	if err != nil {
		return factoryruntime.Checkpoint{}, err
	}
	if loaded.Outcome != factoryruntime.CheckpointOutcomeLoaded {
		return factoryruntime.Checkpoint{}, factoryruntime.ErrCheckpointNotFound
	}
	restored, err := c.runtime.RestoreCheckpoint(ctx, factoryruntime.RestoreCheckpointRequest{Checkpoint: loaded.Checkpoint})
	if err != nil {
		return factoryruntime.Checkpoint{}, err
	}
	if restored.Outcome != factoryruntime.CheckpointOutcomeRestored {
		return factoryruntime.Checkpoint{}, factoryruntime.ErrCorruptCheckpoint
	}
	return loaded.Checkpoint, nil
}

type sessionsCheckpointRuntimeFake struct {
	stored factoryruntime.Checkpoint
}

func (fake *sessionsCheckpointRuntimeFake) LoadCheckpoint(
	_ context.Context,
	req factoryruntime.LoadCheckpointRequest,
) (factoryruntime.LoadCheckpointResult, error) {
	if req.CheckpointID != fake.stored.CheckpointID {
		return factoryruntime.LoadCheckpointResult{}, factoryruntime.ErrCheckpointNotFound
	}
	return factoryruntime.LoadCheckpointResult{
		Outcome:    factoryruntime.CheckpointOutcomeLoaded,
		Checkpoint: fake.stored,
		Compatible: true,
	}, nil
}

func (fake *sessionsCheckpointRuntimeFake) RestoreCheckpoint(
	_ context.Context,
	req factoryruntime.RestoreCheckpointRequest,
) (factoryruntime.RestoreCheckpointResult, error) {
	if req.Checkpoint.CheckpointID != fake.stored.CheckpointID {
		return factoryruntime.RestoreCheckpointResult{}, factoryruntime.ErrCheckpointNotFound
	}
	return factoryruntime.RestoreCheckpointResult{
		Outcome:      factoryruntime.CheckpointOutcomeRestored,
		CheckpointID: req.Checkpoint.CheckpointID,
	}, nil
}

func (fake *sessionsCheckpointRuntimeFake) CaptureCheckpoint(context.Context, factoryruntime.CaptureCheckpointRequest) (factoryruntime.CaptureCheckpointResult, error) {
	return factoryruntime.CaptureCheckpointResult{}, nil
}
func (fake *sessionsCheckpointRuntimeFake) ControlPause(context.Context, factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
	return factoryruntime.PauseResult{}, nil
}
func (fake *sessionsCheckpointRuntimeFake) ControlResume(context.Context, factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
	return factoryruntime.ResumeResult{}, nil
}
func (fake *sessionsCheckpointRuntimeFake) ControlTerminate(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	return factoryruntime.TerminateResult{}, nil
}
func (fake *sessionsCheckpointRuntimeFake) ControlWaitToComplete(factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factoryruntime.WaitToCompleteResult{Done: done}
}
func (fake *sessionsCheckpointRuntimeFake) ControlMoveWork(context.Context, factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error) {
	return factoryruntime.MoveWorkResult{}, nil
}
func (fake *sessionsCheckpointRuntimeFake) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{}, nil
}
func (fake *sessionsCheckpointRuntimeFake) PlanDispatch(context.Context, factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
	return factoryruntime.PlanDispatchResult{}, nil
}
func (fake *sessionsCheckpointRuntimeFake) AcceptDispatchResult(context.Context, factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
	return factoryruntime.AcceptDispatchResultResult{}, nil
}

func TestSessionsCheckpointPeerLease_DoesNotImportCheckpointRecoveryPaths(t *testing.T) {
	t.Parallel()

	for _, root := range sessionsCheckpointPeerLeaseRoots {
		root := root
		t.Run(root, func(t *testing.T) {
			t.Parallel()
			assertSessionsLeaseForbiddenImports(t, root)
		})
	}
}

func TestSessionsCheckpointPeerLease_ImportsFactoryRuntimeOnlyThroughRoot(t *testing.T) {
	t.Parallel()

	for _, root := range sessionsCheckpointPeerLeaseRoots {
		root := root
		t.Run(root, func(t *testing.T) {
			t.Parallel()
			assertSessionsLeaseRuntimeImportsResolveToRoot(t, root)
		})
	}
}

func TestSessionsResumeCoordination_UsesRecordingsAndRuntimeRootContracts(t *testing.T) {
	t.Parallel()

	runtime := &sessionsCheckpointRuntimeFake{
		stored: factoryruntime.Checkpoint{
			CheckpointID:  "sess-checkpoint-1",
			SchemaVersion: 1,
			StrategyKind:  "runtime",
			Payload:       []byte(`{"factoryState":"PAUSED"}`),
		},
	}
	coordinator := sessionsResumeCoordinator{runtime: runtime}
	var recording recordings.PortableRecording
	checkpoint, err := coordinator.resumeFromRecording(context.Background(), recording, "sess-checkpoint-1")
	if err != nil {
		t.Fatalf("resumeFromRecording() error = %v", err)
	}
	if checkpoint.CheckpointID != "sess-checkpoint-1" || len(checkpoint.Payload) == 0 {
		t.Fatalf("checkpoint = %#v, want opaque restored checkpoint", checkpoint)
	}
}

func assertSessionsLeaseForbiddenImports(t *testing.T, packageRoot string) {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		packageRoot,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps for %s: %v\n%s", packageRoot, err, output)
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenSessionsCheckpointRecoveryImports {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf("%s must not import %s; found dependency %s", packageRoot, forbidden, dep)
			}
		}
	}
}

func assertSessionsLeaseRuntimeImportsResolveToRoot(t *testing.T, packageRoot string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}} {{join .Imports \" \"}} {{join .TestImports \" \"}}", packageRoot)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packageRoot, err, output)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		pkgPath := fields[0]
		for _, imp := range fields[1:] {
			if imp == recordingsRootImport || strings.HasPrefix(imp, recordingsRootImport+"/") {
				continue
			}
			if imp != factoryRuntimeRootImport && strings.HasPrefix(imp, factoryRuntimeRootImport+"/") {
				t.Fatalf(
					"%s must import Factory Runtime only through %s; found direct import %s",
					pkgPath,
					factoryRuntimeRootImport,
					imp,
				)
			}
		}
	}
}
