package internal

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestLoadReplayArtifactTypedFailuresAndSuccess(t *testing.T) {
	t.Parallel()

	svc := NewService(&unusedLedger{}, NewProjectionService()).(*combinedService)
	if _, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{}); !errors.Is(err, recordings.ErrMissingReplayArtifact) {
		t.Fatalf("missing artifact = %v, want ErrMissingReplayArtifact", err)
	}
	if _, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{
		ArtifactID: "missing",
	}); !errors.Is(err, recordings.ErrInvalidReplayArtifact) {
		t.Fatalf("unknown artifact = %v, want ErrInvalidReplayArtifact", err)
	}

	artifact := &recordings.ReplayArtifact{SchemaVersion: "replay.v1"}
	svc.lifecycleMu.Lock()
	svc.replayByKey = map[string]*recordings.ReplayArtifact{
		"artifact:legacy": artifact,
	}
	svc.lifecycleMu.Unlock()

	loaded, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{
		Path: "artifact:legacy",
	})
	if err != nil {
		t.Fatalf("LoadReplayArtifact: %v", err)
	}
	if loaded.Artifact == artifact {
		t.Fatal("expected detached replay artifact copy")
	}
	if loaded.Artifact.SchemaVersion != artifact.SchemaVersion {
		t.Fatalf("artifact = %#v", loaded.Artifact)
	}
}

func TestBindReplayExecutionPublishedSuccessShape(t *testing.T) {
	t.Parallel()

	svc := NewService(&unusedLedger{}, NewProjectionService()).(*combinedService)
	if _, err := svc.BindReplayExecution(recordings.BindReplayExecutionRequest{}); !errors.Is(err, recordings.ErrUnsupportedReplayBinding) {
		t.Fatalf("missing artifact = %v, want ErrUnsupportedReplayBinding", err)
	}
	result, err := svc.BindReplayExecution(recordings.BindReplayExecutionRequest{
		Artifact: &recordings.ReplayArtifact{SchemaVersion: "replay.v1"},
	})
	if err != nil {
		t.Fatalf("BindReplayExecution: %v", err)
	}
	if result.Hooks == nil {
		t.Fatalf("hooks = %#v, want empty published slice", result.Hooks)
	}
}

type unusedLedger struct {
	recordings.Ledger
}
