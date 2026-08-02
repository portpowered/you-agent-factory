package wire_test

import (
	"context"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotscontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/contracts"
	snapshotsportabilitywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
)

type stubSnapshotPorts struct{}

func (stubSnapshotPorts) loadCanonical([]byte, snapshotscontracts.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	return nil, nil
}

func (stubSnapshotPorts) captureLoaded(snapshotscontracts.FactorySnapshotSource, string, map[string]string) (*factorydefinitions.FactorySnapshot, error) {
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

func TestNewService_RequiresExactInjectedPorts(t *testing.T) {
	t.Parallel()

	ports := stubSnapshotPorts{}

	if svc, err := snapshotsportabilitywire.NewService(nil, ports.captureLoaded, ports.preparePortable, ports.decodeSnapshot, ports.materializePortableFiles, ports.validateMaterializeWrites); err == nil || svc != nil || !strings.Contains(err.Error(), "canonical Factory loader is required") {
		t.Fatalf("NewService(nil LoadCanonical) = %#v, %v; want canonical loader required error", svc, err)
	}
	if svc, err := snapshotsportabilitywire.NewService(ports.loadCanonical, nil, ports.preparePortable, ports.decodeSnapshot, ports.materializePortableFiles, ports.validateMaterializeWrites); err == nil || svc != nil || !strings.Contains(err.Error(), "loaded Factory snapshot capturer is required") {
		t.Fatalf("NewService(nil CaptureLoaded) = %#v, %v; want capturer required error", svc, err)
	}
	if svc, err := snapshotsportabilitywire.NewService(ports.loadCanonical, ports.captureLoaded, nil, ports.decodeSnapshot, ports.materializePortableFiles, ports.validateMaterializeWrites); err == nil || svc != nil || !strings.Contains(err.Error(), "portable Factory config preparer is required") {
		t.Fatalf("NewService(nil PreparePortable) = %#v, %v; want preparer required error", svc, err)
	}
	if svc, err := snapshotsportabilitywire.NewService(ports.loadCanonical, ports.captureLoaded, ports.preparePortable, nil, ports.materializePortableFiles, ports.validateMaterializeWrites); err == nil || svc != nil || !strings.Contains(err.Error(), "Factory snapshot JSON decoder is required") {
		t.Fatalf("NewService(nil DecodeSnapshot) = %#v, %v; want decoder required error", svc, err)
	}
	if svc, err := snapshotsportabilitywire.NewService(ports.loadCanonical, ports.captureLoaded, ports.preparePortable, ports.decodeSnapshot, nil, ports.validateMaterializeWrites); err == nil || svc != nil || !strings.Contains(err.Error(), "portable bundled-files materializer is required") {
		t.Fatalf("NewService(nil MaterializePortableFiles) = %#v, %v; want materializer required error", svc, err)
	}
	if svc, err := snapshotsportabilitywire.NewService(ports.loadCanonical, ports.captureLoaded, ports.preparePortable, ports.decodeSnapshot, ports.materializePortableFiles, nil); err == nil || svc != nil || !strings.Contains(err.Error(), "portable bundled-file writes validator is required") {
		t.Fatalf("NewService(nil ValidateMaterializeWrites) = %#v, %v; want validator required error", svc, err)
	}

	svc, err := snapshotsportabilitywire.NewService(ports.loadCanonical, ports.captureLoaded, ports.preparePortable, ports.decodeSnapshot, ports.materializePortableFiles, ports.validateMaterializeWrites)
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

	ports := stubSnapshotPorts{}
	svc, err := snapshotsportabilitywire.NewService(ports.loadCanonical, ports.captureLoaded, ports.preparePortable, ports.decodeSnapshot, ports.materializePortableFiles, ports.validateMaterializeWrites)
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
