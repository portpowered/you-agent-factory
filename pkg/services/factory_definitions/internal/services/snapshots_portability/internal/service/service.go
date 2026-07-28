package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilitycapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
	snapshotsportabilitymaterialize "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/materialize"
	snapshotsportabilityprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/prepare"
)

// Service is the private nested snapshots_portability implementation behind
// the CTR-DEF root snapshot slice.
type Service struct {
	loadCanonical             factorydefinitions.CanonicalFactoryJSONLoader
	captureLoaded             factorydefinitions.LoadedFactorySnapshotCapturer
	preparePortable           factorydefinitions.PortableFactoryConfigPreparer
	decodeSnapshot            factorydefinitions.FactorySnapshotJSONDecoder
	materializePortableFiles  factorydefinitions.PortableBundledFilesMaterializer
	validateMaterializeWrites factorydefinitions.PortableBundledFileWritesValidator
}

var _ snapshotsportability.Service = (*Service)(nil)

// New constructs the snapshots_portability implementation from exact injected ports.
func New(
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	captureLoaded factorydefinitions.LoadedFactorySnapshotCapturer,
	preparePortable factorydefinitions.PortableFactoryConfigPreparer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer,
	validateMaterializeWrites factorydefinitions.PortableBundledFileWritesValidator,
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
	request factorydefinitions.CaptureFactorySnapshotRequest,
) (factorydefinitions.CaptureFactorySnapshotResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, err
	}
	canonical := bytes.TrimSpace(request.Canonical)
	if len(canonical) == 0 || bytes.Equal(canonical, []byte("{")) {
		return factorydefinitions.CaptureFactorySnapshotResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	if !isJSONObjectCanonical(canonical) {
		return factorydefinitions.CaptureFactorySnapshotResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}

	loaded, err := s.loadCanonical(canonical, nil)
	if err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, invalidSnapshotPayloadErr(err)
	}
	if loaded == nil || loaded.FactoryConfig() == nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}

	factoryDir := strings.TrimSpace(request.FactoryDir)
	if factoryDir == "" {
		factoryDir = loaded.FactoryDir()
	}

	factoryCfg, err := s.preparePortable(factoryDir, loaded.FactoryConfig(), true)
	if err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, fmt.Errorf("prepare portable factory config: %w", err)
	}

	preparedSource := snapshotsportabilitycapture.NewExplicitSource(
		factoryDir,
		factoryCfg,
		loaded,
	)
	snapshot, err := s.captureLoaded(preparedSource, factoryDir, nil)
	if err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, err
	}
	if snapshot == nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	return factorydefinitions.CaptureFactorySnapshotResult{Snapshot: snapshot}, nil
}

func (s *Service) PrepareFactorySnapshotImport(
	ctx context.Context,
	request factorydefinitions.PrepareFactorySnapshotImportRequest,
) (factorydefinitions.PrepareFactorySnapshotImportResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, err
	}
	return snapshotsportabilityprepare.Import(request.Payload, s.decodeSnapshot)
}

func (s *Service) MaterializeFactorySnapshot(
	ctx context.Context,
	request factorydefinitions.MaterializeFactorySnapshotRequest,
) (factorydefinitions.MaterializeFactorySnapshotResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, err
	}
	return snapshotsportabilitymaterialize.Snapshot(
		request.TargetDir,
		request.Snapshot,
		s.validateMaterializeWrites,
		s.materializePortableFiles,
	)
}

func isJSONObjectCanonical(canonical []byte) bool {
	var decoded any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		return false
	}
	_, ok := decoded.(map[string]any)
	return ok
}

func invalidSnapshotPayloadErr(err error) error {
	if err == nil {
		return factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	if errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
		return factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	return err
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
