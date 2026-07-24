package recordings_test

import (
	"errors"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestReplayRootContract_SuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	seed := &interfaces.ReplayArtifact{
		SchemaVersion: "factory-event-log/v1",
		RecordedAt:    time.Unix(1_700_000_000, 0).UTC(),
		Events: []interfaces.FactoryEvent{
			{Id: "event-1", Type: "WORK_REQUEST"},
		},
	}
	service := recordings.Service(&peerRootServiceFake{
		replayByKey: map[string]*interfaces.ReplayArtifact{
			"session.replay.json": seed,
			"artifact-42":         seed,
		},
		replayCorruptKeys: map[string]bool{
			"corrupt.replay.json": true,
		},
		replayBindingHooks: []recordings.ReplayHook{},
	})

	assertReplayLoadSuccess(t, service, seed)
	assertReplayBindingAndTypedFailures(t, service)
}

func assertReplayLoadSuccess(
	t *testing.T,
	service recordings.Service,
	seed *interfaces.ReplayArtifact,
) {
	t.Helper()
	loaded, err := service.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{
		Path: "session.replay.json",
	})
	if err != nil {
		t.Fatalf("LoadReplayArtifact success path: %v", err)
	}
	if loaded.Artifact == nil {
		t.Fatal("LoadReplayArtifact Artifact nil, want detached replay artifact")
	}
	if loaded.Artifact.SchemaVersion != seed.SchemaVersion {
		t.Fatalf("LoadReplayArtifact SchemaVersion = %q, want %q", loaded.Artifact.SchemaVersion, seed.SchemaVersion)
	}
	if loaded.Artifact == seed {
		t.Fatal("LoadReplayArtifact must return a detached artifact, not the seeded pointer")
	}

	byID, err := service.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{
		ArtifactID: "artifact-42",
	})
	if err != nil {
		t.Fatalf("LoadReplayArtifact by id: %v", err)
	}
	if byID.Artifact == nil || byID.Artifact.SchemaVersion != seed.SchemaVersion {
		t.Fatalf("LoadReplayArtifact by id = %#v, want seeded schema", byID.Artifact)
	}

	bound, err := service.BindReplayExecution(recordings.BindReplayExecutionRequest{
		Artifact: loaded.Artifact,
	})
	if err != nil {
		t.Fatalf("BindReplayExecution success path: %v", err)
	}
	if bound.Hooks == nil {
		t.Fatal("BindReplayExecution Hooks nil, want published success shape with non-nil hooks slice")
	}
}

func assertReplayBindingAndTypedFailures(t *testing.T, service recordings.Service) {
	t.Helper()
	_, err := service.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{})
	if !errors.Is(err, recordings.ErrMissingReplayArtifact) {
		t.Fatalf("missing artifact path/id error = %v, want ErrMissingReplayArtifact", err)
	}

	_, err = service.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{
		Path: "corrupt.replay.json",
	})
	if !errors.Is(err, recordings.ErrInvalidReplayArtifact) {
		t.Fatalf("corrupt artifact error = %v, want ErrInvalidReplayArtifact", err)
	}
	if errors.Is(err, recordings.ErrMissingReplayArtifact) {
		t.Fatalf("corrupt artifact must remain distinct from ErrMissingReplayArtifact")
	}

	_, err = service.BindReplayExecution(recordings.BindReplayExecutionRequest{
		Artifact: nil,
	})
	if !errors.Is(err, recordings.ErrUnsupportedReplayBinding) {
		t.Fatalf("unsupported binding error = %v, want ErrUnsupportedReplayBinding", err)
	}
	if errors.Is(err, recordings.ErrMissingReplayArtifact) || errors.Is(err, recordings.ErrInvalidReplayArtifact) {
		t.Fatalf("unsupported binding must remain distinct from load typed errors")
	}

	_, err = service.BindReplayExecution(recordings.BindReplayExecutionRequest{
		Artifact: &interfaces.ReplayArtifact{},
	})
	if !errors.Is(err, recordings.ErrUnsupportedReplayBinding) {
		t.Fatalf("empty-schema binding error = %v, want ErrUnsupportedReplayBinding", err)
	}
}
