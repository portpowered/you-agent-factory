package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
)

// Service is the private nested snapshots_portability implementation behind
// the CTR-DEF root snapshot slice.
type Service struct {
	loadCanonical             factorycontracts.CanonicalFactoryJSONLoader
	captureLoaded             factorycontracts.LoadedFactorySnapshotCapturer
	preparePortable           factorycontracts.PortableFactoryConfigPreparer
	decodeSnapshot            factorydefinitions.FactorySnapshotJSONDecoder
	materializePortableFiles  factorycontracts.PortableBundledFilesMaterializer
	validateMaterializeWrites factorycontracts.PortableBundledFileWritesValidator
}

var _ snapshotsportability.Service = (*Service)(nil)

// New constructs the snapshots_portability implementation from exact injected ports.
func New(
	loadCanonical factorycontracts.CanonicalFactoryJSONLoader,
	captureLoaded factorycontracts.LoadedFactorySnapshotCapturer,
	preparePortable factorycontracts.PortableFactoryConfigPreparer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	materializePortableFiles factorycontracts.PortableBundledFilesMaterializer,
	validateMaterializeWrites factorycontracts.PortableBundledFileWritesValidator,
) *Service {
	if loadCanonical == nil ||
		captureLoaded == nil ||
		preparePortable == nil ||
		decodeSnapshot == nil ||
		materializePortableFiles == nil ||
		validateMaterializeWrites == nil {
		return nil
	}
	return &Service{
		loadCanonical:             loadCanonical,
		captureLoaded:             captureLoaded,
		preparePortable:           preparePortable,
		decodeSnapshot:            decodeSnapshot,
		materializePortableFiles:  materializePortableFiles,
		validateMaterializeWrites: validateMaterializeWrites,
	}
}

func (s *Service) CaptureFactorySnapshot(
	ctx context.Context,
	_ factorydefinitions.CaptureFactorySnapshotRequest,
) (factorydefinitions.CaptureFactorySnapshotResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, err
	}
	return factorydefinitions.CaptureFactorySnapshotResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
}

func (s *Service) PrepareFactorySnapshotImport(
	ctx context.Context,
	_ factorydefinitions.PrepareFactorySnapshotImportRequest,
) (factorydefinitions.PrepareFactorySnapshotImportResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, err
	}
	return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
}

func (s *Service) MaterializeFactorySnapshot(
	ctx context.Context,
	_ factorydefinitions.MaterializeFactorySnapshotRequest,
) (factorydefinitions.MaterializeFactorySnapshotResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, err
	}
	return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
}

func (s *Service) requirePorts() error {
	if s == nil || s.loadCanonical == nil {
		return fmt.Errorf("canonical Factory loader is required")
	}
	if s.captureLoaded == nil {
		return fmt.Errorf("loaded Factory snapshot capturer is required")
	}
	if s.preparePortable == nil {
		return fmt.Errorf("portable Factory config preparer is required")
	}
	if s.decodeSnapshot == nil {
		return fmt.Errorf("Factory snapshot JSON decoder is required")
	}
	if s.materializePortableFiles == nil {
		return fmt.Errorf("portable bundled-files materializer is required")
	}
	if s.validateMaterializeWrites == nil {
		return fmt.Errorf("portable bundled-file writes validator is required")
	}
	return nil
}
