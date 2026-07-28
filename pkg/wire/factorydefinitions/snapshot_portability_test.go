package factorydefinitions_test

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	wirefactorydefinitions "github.com/portpowered/infinite-you/pkg/wire/factorydefinitions"
)

func TestWireSnapshotHelpersCapturePrepareMaterializeAndReplay(t *testing.T) {
	t.Parallel()

	fileSystem := platformfilesystem.Local{}
	applySupportedFiles, err := factorydefinitionswire.NewPortableBundledFilesApplier(fileSystem)
	if err != nil {
		t.Fatalf("NewPortableBundledFilesApplier() error = %v", err)
	}
	applyStarterWork, err := factorydefinitionswire.NewFactoryStarterWorkApplier(fileSystem)
	if err != nil {
		t.Fatalf("NewFactoryStarterWorkApplier() error = %v", err)
	}

	factoryConfig, err := factorymapping.FactoryConfigFromOpenAPIJSON(
		[]byte(factoryfixtures.CrossPathValidAlphaFactoryJSON),
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON() error = %v", err)
	}

	prepared, err := wirefactorydefinitions.PortableFactoryConfigPreparer(
		applySupportedFiles,
		applyStarterWork,
	)("/factories/alpha", factoryConfig, false)
	if err != nil {
		t.Fatalf("PortableFactoryConfigPreparer() error = %v", err)
	}
	if prepared == nil || prepared.Name != "alpha" {
		t.Fatalf("PortableFactoryConfigPreparer() = %#v, want alpha config", prepared)
	}

	capture := wirefactorydefinitions.FactorySnapshotCapturer()
	snapshot, err := capture("/factories/alpha", prepared, nil, "", nil)
	if err != nil {
		t.Fatalf("FactorySnapshotCapturer() error = %v", err)
	}
	if snapshot == nil {
		t.Fatal("FactorySnapshotCapturer() returned nil snapshot")
	}

	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("Marshal(snapshot) error = %v", err)
	}
	imported, err := factorydefinitionswire.PrepareFactorySnapshotImport(payload)
	if err != nil {
		t.Fatalf("prepare.Import() error = %v", err)
	}
	if imported.Snapshot == nil || imported.Name != "alpha" {
		t.Fatalf("prepare.Import() = %#v, want alpha portable import facts", imported)
	}

	targetDir := t.TempDir()
	materializer := factorydefinitionswire.NewPortableBundledFilesMaterializer(fileSystem)
	if _, err := materializer(targetDir, prepared); err != nil {
		t.Fatalf("Materializer() error = %v", err)
	}

	replayConfig, err := wirefactorydefinitions.ReplayRuntimeConfigDecoder()(snapshot)
	if err != nil {
		t.Fatalf("ReplayRuntimeConfigDecoder() error = %v", err)
	}
	if replayConfig.FactoryDir() != "/factories/alpha" {
		t.Fatalf("replay factory dir = %q, want /factories/alpha", replayConfig.FactoryDir())
	}
	if _, ok := replayConfig.Worker("worker-a"); !ok {
		t.Fatal("replay lookup missing worker-a")
	}
	if _, ok := replayConfig.Workstation("process"); !ok {
		t.Fatal("replay lookup missing process workstation")
	}
}
