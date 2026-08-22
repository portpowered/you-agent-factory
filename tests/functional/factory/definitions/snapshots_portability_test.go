package definitions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const snapshotPortabilityImportedName = "snapshot-service-import"

// TestFactoryDefinitionsPortableSnapshotRoundTripThroughServiceRoot exercises
// detached snapshot capture, prepare-import, and materialization through the
// public Definitions root before importing the same payload through the public
// named Factory command and observing its portable assets.
func TestFactoryDefinitionsPortableSnapshotRoundTripThroughServiceRoot(t *testing.T) {
	sourceDir := support.ScaffoldFactory(t, importExportPortableFactoryConfig())
	seedImportExportPortableFilesOnDisk(t, sourceDir)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	canonical, err := support.FlattenFactoryConfigWithProcess(
		t,
		process,
		filepath.Join(sourceDir, factorydefinitions.FactoryConfigFile),
	)
	if err != nil {
		t.Fatalf("Process.Execute(factory config flatten): %v", err)
	}

	service := newFunctionalDefinitionsService(t)
	captured, err := service.CaptureFactorySnapshot(
		t.Context(),
		factorydefinitions.CaptureFactorySnapshotRequest{
			FactoryDir: sourceDir,
			Canonical:  canonical,
			Name:       importExportSourceName,
		},
	)
	if err != nil {
		t.Fatalf("Definitions.CaptureFactorySnapshot: %v", err)
	}
	if captured.Snapshot == nil {
		t.Fatal("Definitions.CaptureFactorySnapshot returned a nil snapshot")
	}

	payload, err := json.Marshal(captured.Snapshot)
	if err != nil {
		t.Fatalf("marshal detached Factory snapshot: %v", err)
	}
	exportedFactory, err := support.DecodeFactoryDefinition(payload)
	if err != nil {
		t.Fatalf("decode detached Factory snapshot: %v", err)
	}
	assertImportExportPortableBundledFileInline(
		t,
		exportedFactory,
		factoryapi.BundledFileTypeDOC,
		importExportPortableDocPath,
		importExportPortableDocBody,
		"captured snapshot",
	)
	assertImportExportPortableBundledFileInline(
		t,
		exportedFactory,
		factoryapi.BundledFileTypeSCRIPT,
		importExportPortableScriptPath,
		importExportPortableScriptBody,
		"captured snapshot",
	)
	assertImportExportPortableLayoutNote(t, exportedFactory, "captured snapshot")

	prepared, err := service.PrepareFactorySnapshotImport(
		t.Context(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: payload},
	)
	if err != nil {
		t.Fatalf("Definitions.PrepareFactorySnapshotImport: %v", err)
	}
	if prepared.Snapshot == nil || prepared.Name != importExportSourceName {
		t.Fatalf("prepared snapshot result = %#v, want %q identity", prepared, importExportSourceName)
	}
	if prepared.Portable.FactoryDir != sourceDir {
		t.Fatalf("prepared snapshot FactoryDir = %q, want %q", prepared.Portable.FactoryDir, sourceDir)
	}

	materializedDir := t.TempDir()
	materialized, err := service.MaterializeFactorySnapshot(
		t.Context(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: materializedDir,
			Snapshot:  prepared.Snapshot,
		},
	)
	if err != nil {
		t.Fatalf("Definitions.MaterializeFactorySnapshot: %v", err)
	}
	if materialized.TargetDir != materializedDir || materialized.Portable.FactoryDir != materializedDir {
		t.Fatalf("materialized snapshot result = %#v, want target %q", materialized, materializedDir)
	}

	snapshotPath := filepath.Join(t.TempDir(), "factory-snapshot.json")
	if err := os.WriteFile(snapshotPath, payload, 0o644); err != nil {
		t.Fatalf("write detached Factory snapshot: %v", err)
	}
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	importedDir := support.CreateNamedFactoryWithProcess(
		t,
		process,
		homeDir,
		workingDir,
		snapshotPortabilityImportedName,
		snapshotPath,
	)
	assertImportExportPortableFileOnDisk(
		t,
		filepath.Join(importedDir, "docs", "standards", "review.md"),
		importExportPortableDocBody,
	)
	assertImportExportPortableFileOnDisk(
		t,
		filepath.Join(importedDir, "scripts", "execute-task.sh"),
		importExportPortableScriptBody,
	)

	importedFactory, err := support.LoadedFactoryWithProcessAndEnv(
		t,
		process,
		env,
		filepath.Join(importedDir, factorydefinitions.FactoryConfigFile),
	)
	if err != nil {
		t.Fatalf("load named Factory imported from snapshot: %v", err)
	}
	assertImportExportPortableBundledFile(
		t,
		importedFactory,
		factoryapi.BundledFileTypeDOC,
		importExportPortableDocPath,
		"named imported Factory",
	)
	assertImportExportPortableBundledFile(
		t,
		importedFactory,
		factoryapi.BundledFileTypeSCRIPT,
		importExportPortableScriptPath,
		"named imported Factory",
	)
	assertImportExportPortableLayoutNote(t, importedFactory, "named imported Factory")
}
