package wire_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

// Root snapshot/portability behavioral proofs exercise capture, prepare-import,
// materialize, and replay reconstruction through factory_definitions/wire and
// the published Service root after the snapshots_portability fold.

func TestWireRootSnapshotSliceCapturePrepareMaterializeRoundTrip(t *testing.T) {
	t.Parallel()

	service := newWireFoldPreservationService(t)
	ctx := context.Background()
	validCanonical := []byte(factoryfixtures.CrossPathValidAlphaFactoryJSON)
	factoryDir := "/factories/alpha"

	captured, err := service.CaptureFactorySnapshot(
		ctx,
		factorydefinitions.CaptureFactorySnapshotRequest{
			FactoryDir: factoryDir,
			Canonical:  validCanonical,
			Name:       "alpha",
		},
	)
	if err != nil {
		t.Fatalf("CaptureFactorySnapshot() error = %v", err)
	}
	if captured.Snapshot == nil {
		t.Fatal("CaptureFactorySnapshot() snapshot is nil")
	}

	detachedPayload, err := json.Marshal(captured.Snapshot)
	if err != nil {
		t.Fatalf("Marshal(captured snapshot) error = %v", err)
	}
	if !json.Valid(detachedPayload) || detachedPayload[0] != '{' {
		t.Fatalf("detached snapshot payload = %s, want JSON object", detachedPayload)
	}

	imported, err := service.PrepareFactorySnapshotImport(
		ctx,
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: detachedPayload},
	)
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport() error = %v", err)
	}
	if imported.Snapshot == nil || imported.Name != "alpha" {
		t.Fatalf("PrepareFactorySnapshotImport() = %#v, want alpha snapshot facts", imported)
	}
	if imported.Portable.FactoryDir == "" {
		t.Fatalf("PrepareFactorySnapshotImport() portable = %#v, want portable success facts", imported.Portable)
	}

	targetDir := t.TempDir()
	materialized, err := service.MaterializeFactorySnapshot(
		ctx,
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: targetDir,
			Snapshot:  imported.Snapshot,
		},
	)
	if err != nil {
		t.Fatalf("MaterializeFactorySnapshot() error = %v", err)
	}
	if materialized.TargetDir != targetDir || materialized.Portable.FactoryDir == "" {
		t.Fatalf("MaterializeFactorySnapshot() = %#v, want portable success facts", materialized)
	}
}

func TestWireRootSnapshotSliceReplayReconstruction(t *testing.T) {
	t.Parallel()

	service := newWireFoldPreservationService(t)
	ctx := context.Background()
	validCanonical := []byte(factoryfixtures.CrossPathValidAlphaFactoryJSON)
	factoryDir := "/factories/alpha"

	captured, err := service.CaptureFactorySnapshot(
		ctx,
		factorydefinitions.CaptureFactorySnapshotRequest{
			FactoryDir: factoryDir,
			Canonical:  validCanonical,
			Name:       "alpha",
		},
	)
	if err != nil {
		t.Fatalf("CaptureFactorySnapshot() error = %v", err)
	}
	if captured.Snapshot == nil {
		t.Fatal("CaptureFactorySnapshot() snapshot is nil")
	}

	replayConfig, err := factorydefinitionswire.NewReplayRuntimeConfigDecoder(testRepresentation())(captured.Snapshot)
	if err != nil {
		t.Fatalf("ReplayRuntimeConfigDecoder() error = %v", err)
	}
	if replayConfig.FactoryDir() != factoryDir {
		t.Fatalf("replay factory dir = %q, want %q", replayConfig.FactoryDir(), factoryDir)
	}
	if _, ok := replayConfig.Worker("worker-a"); !ok {
		t.Fatal("replay lookup missing worker-a")
	}
	if _, ok := replayConfig.Workstation("process"); !ok {
		t.Fatal("replay lookup missing process workstation")
	}
}
