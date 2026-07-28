package wire_test

import (
	"context"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilitywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
)

type stubSnapshotPorts struct{}

func (stubSnapshotPorts) loadCanonical([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	return nil, nil
}

func (stubSnapshotPorts) captureLoaded(factorydefinitions.FactorySnapshotSource, string, map[string]string) (*factorydefinitions.FactorySnapshot, error) {
	return nil, nil
}

func (stubSnapshotPorts) preparePortable(string, *factorydefinitions.FactoryConfig, bool) (*factorydefinitions.FactoryConfig, error) {
	return nil, nil
}

func (stubSnapshotPorts) decodeSnapshot([]byte) (*factorydefinitions.FactorySnapshot, error) {
	return nil, nil
}

func (stubSnapshotPorts) materializePortableFiles(string, *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
	return nil, nil
}

func (stubSnapshotPorts) validateMaterializeWrites(string, *factorydefinitions.FactoryConfig) error {
	return nil
}

func stubDependencies() snapshotsportability.Dependencies {
	ports := stubSnapshotPorts{}
	return snapshotsportability.Dependencies{
		LoadCanonical:             ports.loadCanonical,
		CaptureLoaded:             ports.captureLoaded,
		PreparePortable:           ports.preparePortable,
		DecodeSnapshot:            ports.decodeSnapshot,
		MaterializePortableFiles:  ports.materializePortableFiles,
		ValidateMaterializeWrites: ports.validateMaterializeWrites,
	}
}

func TestNewService_RequiresExactInjectedPorts(t *testing.T) {
	t.Parallel()

	deps := stubDependencies()

	if svc, err := snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             nil,
		CaptureLoaded:             deps.CaptureLoaded,
		PreparePortable:           deps.PreparePortable,
		DecodeSnapshot:            deps.DecodeSnapshot,
		MaterializePortableFiles:  deps.MaterializePortableFiles,
		ValidateMaterializeWrites: deps.ValidateMaterializeWrites,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "canonical Factory loader is required") {
		t.Fatalf("NewService(nil LoadCanonical) = %#v, %v; want canonical loader required error", svc, err)
	}
	if svc, err := snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             deps.LoadCanonical,
		CaptureLoaded:             nil,
		PreparePortable:           deps.PreparePortable,
		DecodeSnapshot:            deps.DecodeSnapshot,
		MaterializePortableFiles:  deps.MaterializePortableFiles,
		ValidateMaterializeWrites: deps.ValidateMaterializeWrites,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "loaded Factory snapshot capturer is required") {
		t.Fatalf("NewService(nil CaptureLoaded) = %#v, %v; want capturer required error", svc, err)
	}
	if svc, err := snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             deps.LoadCanonical,
		CaptureLoaded:             deps.CaptureLoaded,
		PreparePortable:           nil,
		DecodeSnapshot:            deps.DecodeSnapshot,
		MaterializePortableFiles:  deps.MaterializePortableFiles,
		ValidateMaterializeWrites: deps.ValidateMaterializeWrites,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "portable Factory config preparer is required") {
		t.Fatalf("NewService(nil PreparePortable) = %#v, %v; want preparer required error", svc, err)
	}
	if svc, err := snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             deps.LoadCanonical,
		CaptureLoaded:             deps.CaptureLoaded,
		PreparePortable:           deps.PreparePortable,
		DecodeSnapshot:            nil,
		MaterializePortableFiles:  deps.MaterializePortableFiles,
		ValidateMaterializeWrites: deps.ValidateMaterializeWrites,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "Factory snapshot JSON decoder is required") {
		t.Fatalf("NewService(nil DecodeSnapshot) = %#v, %v; want decoder required error", svc, err)
	}
	if svc, err := snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             deps.LoadCanonical,
		CaptureLoaded:             deps.CaptureLoaded,
		PreparePortable:           deps.PreparePortable,
		DecodeSnapshot:            deps.DecodeSnapshot,
		MaterializePortableFiles:  nil,
		ValidateMaterializeWrites: deps.ValidateMaterializeWrites,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "portable bundled-files materializer is required") {
		t.Fatalf("NewService(nil MaterializePortableFiles) = %#v, %v; want materializer required error", svc, err)
	}
	if svc, err := snapshotsportabilitywire.NewService(snapshotsportability.Dependencies{
		LoadCanonical:             deps.LoadCanonical,
		CaptureLoaded:             deps.CaptureLoaded,
		PreparePortable:           deps.PreparePortable,
		DecodeSnapshot:            deps.DecodeSnapshot,
		MaterializePortableFiles:  deps.MaterializePortableFiles,
		ValidateMaterializeWrites: nil,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "portable bundled-file writes validator is required") {
		t.Fatalf("NewService(nil ValidateMaterializeWrites) = %#v, %v; want validator required error", svc, err)
	}

	svc, err := snapshotsportabilitywire.NewService(deps)
	if err != nil {
		t.Fatalf("NewService with exact injected ports: %v", err)
	}
	if svc == nil {
		t.Fatal("NewService returned nil service")
	}
	var _ snapshotsportability.Service = svc
}

func TestNewService_StubMethodsReturnTypedFailuresUntilRelocation(t *testing.T) {
	t.Parallel()

	svc, err := snapshotsportabilitywire.NewService(stubDependencies())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()
	if _, err := svc.PrepareFactorySnapshotImport(ctx, factorydefinitions.PrepareFactorySnapshotImportRequest{}); err != factorydefinitions.ErrInvalidFactorySnapshotPayload {
		t.Fatalf("PrepareFactorySnapshotImport = %v, want ErrInvalidFactorySnapshotPayload", err)
	}
	if _, err := svc.MaterializeFactorySnapshot(ctx, factorydefinitions.MaterializeFactorySnapshotRequest{}); err != factorydefinitions.ErrUnsafeFactorySnapshotMaterialize {
		t.Fatalf("MaterializeFactorySnapshot = %v, want ErrUnsafeFactorySnapshotMaterialize", err)
	}
}
