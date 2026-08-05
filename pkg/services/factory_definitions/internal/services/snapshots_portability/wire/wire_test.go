package wire_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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

func (stubSnapshotPorts) decodeConfig(*factorydefinitions.FactorySnapshot) (*factorydefinitions.FactoryConfig, error) {
	return &factorydefinitions.FactoryConfig{}, nil
}

func (stubSnapshotPorts) materializePortableFiles(string, *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
	return nil, nil
}

func (stubSnapshotPorts) validateMaterializeWrites(string, *factorydefinitions.FactoryConfig) error {
	return nil
}

func newSnapshotService(
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	captureLoaded factorydefinitions.LoadedFactorySnapshotCapturer,
	preparePortable factorydefinitions.PortableFactoryConfigPreparer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeConfig factorydefinitions.FactorySnapshotConfigDecoder,
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer,
	validateMaterializeWrites factorydefinitions.PortableBundledFileWritesValidator,
	fileSystem factorydefinitions.SnapshotMaterializationFileSystem,
	directories factorydefinitions.DirectoryReplacementStore,
) (factorydefinitions.Snapshots, error) {
	return snapshotsportabilitywire.NewService(
		loadCanonical,
		captureLoaded,
		preparePortable,
		decodeSnapshot,
		decodeConfig,
		materializePortableFiles,
		validateMaterializeWrites,
		fileSystem,
		directories,
	)
}

func TestNewServiceRequiresExactInjectedPorts(t *testing.T) {
	t.Parallel()

	ports := stubSnapshotPorts{}
	fileSystem := platformfilesystem.Local{}
	directories := directoryreplace.Local{}
	tests := []struct {
		name string
		call func() (factorydefinitions.Snapshots, error)
		want string
	}{
		{"loader", func() (factorydefinitions.Snapshots, error) {
			return newSnapshotService(nil, ports.captureLoaded, ports.preparePortable, ports.decodeSnapshot, ports.decodeConfig, ports.materializePortableFiles, ports.validateMaterializeWrites, fileSystem, directories)
		}, "canonical Factory loader is required"},
		{"capturer", func() (factorydefinitions.Snapshots, error) {
			return newSnapshotService(ports.loadCanonical, nil, ports.preparePortable, ports.decodeSnapshot, ports.decodeConfig, ports.materializePortableFiles, ports.validateMaterializeWrites, fileSystem, directories)
		}, "loaded Factory snapshot capturer is required"},
		{"preparer", func() (factorydefinitions.Snapshots, error) {
			return newSnapshotService(ports.loadCanonical, ports.captureLoaded, nil, ports.decodeSnapshot, ports.decodeConfig, ports.materializePortableFiles, ports.validateMaterializeWrites, fileSystem, directories)
		}, "portable Factory config preparer is required"},
		{"snapshot decoder", func() (factorydefinitions.Snapshots, error) {
			return newSnapshotService(ports.loadCanonical, ports.captureLoaded, ports.preparePortable, nil, ports.decodeConfig, ports.materializePortableFiles, ports.validateMaterializeWrites, fileSystem, directories)
		}, "Factory snapshot JSON decoder is required"},
		{"config decoder", func() (factorydefinitions.Snapshots, error) {
			return newSnapshotService(ports.loadCanonical, ports.captureLoaded, ports.preparePortable, ports.decodeSnapshot, nil, ports.materializePortableFiles, ports.validateMaterializeWrites, fileSystem, directories)
		}, "Factory snapshot config decoder is required"},
		{"materializer", func() (factorydefinitions.Snapshots, error) {
			return newSnapshotService(ports.loadCanonical, ports.captureLoaded, ports.preparePortable, ports.decodeSnapshot, ports.decodeConfig, nil, ports.validateMaterializeWrites, fileSystem, directories)
		}, "portable bundled-files materializer is required"},
		{"validator", func() (factorydefinitions.Snapshots, error) {
			return newSnapshotService(ports.loadCanonical, ports.captureLoaded, ports.preparePortable, ports.decodeSnapshot, ports.decodeConfig, ports.materializePortableFiles, nil, fileSystem, directories)
		}, "portable bundled-file writes validator is required"},
		{"filesystem", func() (factorydefinitions.Snapshots, error) {
			return newSnapshotService(ports.loadCanonical, ports.captureLoaded, ports.preparePortable, ports.decodeSnapshot, ports.decodeConfig, ports.materializePortableFiles, ports.validateMaterializeWrites, nil, directories)
		}, "snapshot materialization filesystem is required"},
		{"directories", func() (factorydefinitions.Snapshots, error) {
			return newSnapshotService(ports.loadCanonical, ports.captureLoaded, ports.preparePortable, ports.decodeSnapshot, ports.decodeConfig, ports.materializePortableFiles, ports.validateMaterializeWrites, fileSystem, nil)
		}, "directory replacement store is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, err := test.call()
			if err == nil || svc != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewService() = %#v, %v; want %q", svc, err, test.want)
			}
		})
	}
}

func TestNewServiceReturnsFocusedSnapshotCapability(t *testing.T) {
	t.Parallel()

	ports := stubSnapshotPorts{}
	svc, err := newSnapshotService(
		ports.loadCanonical,
		ports.captureLoaded,
		ports.preparePortable,
		ports.decodeSnapshot,
		ports.decodeConfig,
		ports.materializePortableFiles,
		ports.validateMaterializeWrites,
		platformfilesystem.Local{},
		directoryreplace.Local{},
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc == nil {
		t.Fatal("NewService returned nil capability")
	}

	_, err = svc.PrepareFactorySnapshotImport(context.Background(), factorydefinitions.PrepareFactorySnapshotImportRequest{})
	if !errors.Is(err, factorydefinitions.ErrFactorySnapshotMissing) {
		t.Fatalf("PrepareFactorySnapshotImport = %v, want missing snapshot", err)
	}
}
