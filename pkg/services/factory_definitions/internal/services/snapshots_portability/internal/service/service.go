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
	decodeConfig              factorydefinitions.FactorySnapshotConfigDecoder
	materializePortableFiles  factorydefinitions.PortableBundledFilesMaterializer
	validateMaterializeWrites factorydefinitions.PortableBundledFileWritesValidator
	fileSystem                factorydefinitions.SnapshotMaterializationFileSystem
	directories               factorydefinitions.DirectoryReplacementStore
}

var _ snapshotsportability.Service = (*Service)(nil)

// New constructs the snapshots_portability implementation from exact injected ports.
func New(
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	captureLoaded factorydefinitions.LoadedFactorySnapshotCapturer,
	preparePortable factorydefinitions.PortableFactoryConfigPreparer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeConfig factorydefinitions.FactorySnapshotConfigDecoder,
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer,
	validateMaterializeWrites factorydefinitions.PortableBundledFileWritesValidator,
	fileSystem factorydefinitions.SnapshotMaterializationFileSystem,
	directories factorydefinitions.DirectoryReplacementStore,
) *Service {
	if loadCanonical == nil ||
		captureLoaded == nil ||
		preparePortable == nil ||
		decodeSnapshot == nil ||
		decodeConfig == nil ||
		materializePortableFiles == nil ||
		validateMaterializeWrites == nil ||
		fileSystem == nil ||
		directories == nil {
		return nil
	}
	return &Service{
		loadCanonical:             loadCanonical,
		captureLoaded:             captureLoaded,
		preparePortable:           preparePortable,
		decodeSnapshot:            decodeSnapshot,
		decodeConfig:              decodeConfig,
		materializePortableFiles:  materializePortableFiles,
		validateMaterializeWrites: validateMaterializeWrites,
		fileSystem:                fileSystem,
		directories:               directories,
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
		return factorydefinitions.CaptureFactorySnapshotResult{}, missingSnapshotError()
	}
	if !isJSONObjectCanonical(canonical) {
		return factorydefinitions.CaptureFactorySnapshotResult{}, malformedSnapshotError(nil)
	}

	loaded, err := s.loadCanonical(canonical, nil)
	if err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, malformedSnapshotError(err)
	}
	if loaded == nil || loaded.FactoryConfig() == nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, malformedSnapshotError(nil)
	}

	factoryDir := strings.TrimSpace(request.FactoryDir)
	if factoryDir == "" {
		factoryDir = loaded.FactoryDir()
	}

	factoryCfg, err := s.preparePortable(factoryDir, loaded.FactoryConfig(), true)
	if err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, malformedSnapshotError(err)
	}

	preparedSource := snapshotsportabilitycapture.NewExplicitSource(
		factoryDir,
		factoryCfg,
		loaded,
	)
	snapshot, err := s.captureLoaded(preparedSource, factoryDir, nil)
	if err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, malformedSnapshotError(err)
	}
	if snapshot == nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, malformedSnapshotError(nil)
	}
	return capturedSnapshotResult(snapshot, factoryCfg)
}

func (s *Service) CaptureLoadedFactorySnapshot(
	ctx context.Context,
	request factorydefinitions.CaptureLoadedFactorySnapshotRequest,
) (factorydefinitions.CaptureFactorySnapshotResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, err
	}
	if request.Source == nil || request.Source.FactoryConfig() == nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, missingSnapshotError()
	}
	sourceDirectory := strings.TrimSpace(request.SourceDirectory)
	if sourceDirectory == "" {
		sourceDirectory = request.Source.FactoryDir()
	}
	factoryDir := request.Source.FactoryDir()
	prepared, err := s.preparePortable(factoryDir, request.Source.FactoryConfig(), true)
	if err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, malformedSnapshotError(err)
	}
	if prepared == nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, malformedSnapshotError(nil)
	}
	source := snapshotsportabilitycapture.NewExplicitSource(factoryDir, prepared, request.Source)
	snapshot, err := s.captureLoaded(source, sourceDirectory, cloneMetadata(request.Metadata))
	if err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, malformedSnapshotError(err)
	}
	return capturedSnapshotResult(snapshot, prepared)
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
	if len(bytes.TrimSpace(request.Payload)) == 0 {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, missingSnapshotError()
	}
	prepared, err := snapshotsportabilityprepare.Import(request.Payload, s.decodeSnapshot)
	if err != nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, malformedSnapshotError(err)
	}
	factoryConfig, err := s.decodeConfig(prepared.Snapshot)
	if err != nil || factoryConfig == nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, malformedSnapshotError(err)
	}
	identity, err := factorydefinitions.VerifyFactorySnapshot(
		prepared.Snapshot,
		factoryConfig,
		request.ExpectedIdentity,
	)
	if err != nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, err
	}
	prepared.Snapshot = prepared.Snapshot.Clone()
	prepared.Identity = identity
	prepared.Portable.Assets = snapshotAssets(factoryConfig)
	return prepared, nil
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
	if request.Snapshot == nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, missingSnapshotError()
	}
	factoryConfig, err := s.decodeConfig(request.Snapshot)
	if err != nil || factoryConfig == nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, malformedSnapshotError(err)
	}
	identity, err := factorydefinitions.VerifyFactorySnapshot(
		request.Snapshot,
		factoryConfig,
		request.ExpectedIdentity,
	)
	if err != nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, err
	}
	materialized, err := snapshotsportabilitymaterialize.Snapshot(
		request.TargetDir,
		request.Snapshot,
		factoryConfig,
		s.validateMaterializeWrites,
		s.materializePortableFiles,
		s.fileSystem,
		s.directories,
	)
	if err != nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, unsafeSnapshotError(identity, err)
	}
	materialized.Identity = identity
	return materialized, nil
}

func isJSONObjectCanonical(canonical []byte) bool {
	var decoded any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		return false
	}
	_, ok := decoded.(map[string]any)
	return ok
}

func capturedSnapshotResult(
	snapshot *factorydefinitions.FactorySnapshot,
	factoryConfig *factorydefinitions.FactoryConfig,
) (factorydefinitions.CaptureFactorySnapshotResult, error) {
	if snapshot == nil || factoryConfig == nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, malformedSnapshotError(nil)
	}
	sealed, identity, err := factorydefinitions.SealFactorySnapshot(snapshot, factoryConfig)
	if err != nil {
		var inputErr *factorydefinitions.SnapshotInputError
		if errors.As(err, &inputErr) {
			return factorydefinitions.CaptureFactorySnapshotResult{}, err
		}
		return factorydefinitions.CaptureFactorySnapshotResult{}, malformedSnapshotError(err)
	}
	return factorydefinitions.CaptureFactorySnapshotResult{
		Snapshot: sealed.Clone(),
		Identity: identity,
	}, nil
}

func snapshotAssets(factoryConfig *factorydefinitions.FactoryConfig) []factorydefinitions.PortableSnapshotAssetFact {
	if factoryConfig == nil || factoryConfig.ResourceManifest == nil {
		return nil
	}
	assets := make([]factorydefinitions.PortableSnapshotAssetFact, 0, len(factoryConfig.ResourceManifest.BundledFiles))
	for _, file := range factoryConfig.ResourceManifest.BundledFiles {
		if strings.TrimSpace(file.TargetPath) == "" {
			continue
		}
		assets = append(assets, factorydefinitions.PortableSnapshotAssetFact{TargetPath: file.TargetPath})
	}
	return assets
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	clone := make(map[string]string, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}

func missingSnapshotError() error {
	return factorydefinitions.NewSnapshotInputError(
		factorydefinitions.SnapshotErrorMissing, "", "", factorydefinitions.ErrFactorySnapshotMissing,
	)
}

func malformedSnapshotError(cause error) error {
	if cause == nil {
		cause = factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	if errors.Is(cause, factorydefinitions.ErrInvalidNamedFactory) {
		cause = factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	return factorydefinitions.NewSnapshotInputError(
		factorydefinitions.SnapshotErrorMalformed, "", "", cause,
	)
}

func unsafeSnapshotError(identity factorydefinitions.SnapshotIdentity, cause error) error {
	return factorydefinitions.NewSnapshotInputError(
		factorydefinitions.SnapshotErrorUnsafe, identity, "", cause,
	)
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
	if s.decodeConfig == nil {
		return fmt.Errorf("Factory snapshot config decoder is required")
	}
	if s.materializePortableFiles == nil {
		return fmt.Errorf("portable bundled-files materializer is required")
	}
	if s.validateMaterializeWrites == nil {
		return fmt.Errorf("portable bundled-file writes validator is required")
	}
	if s.fileSystem == nil {
		return fmt.Errorf("snapshot materialization filesystem is required")
	}
	if s.directories == nil {
		return fmt.Errorf("directory replacement store is required")
	}
	return nil
}
