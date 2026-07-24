package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
)

// Service is the private snapshots_portability implementation behind the
// CTR-DEF root snapshot/portability vocabulary.
type Service struct{}

var _ snapshotsportability.Service = (*Service)(nil)

// New constructs the detached snapshots_portability implementation.
func New() *Service {
	return &Service{}
}

// CaptureFactorySnapshot captures one authored/effective Factory source into a
// detached FactorySnapshot under the CTR-DEF portability contract.
func (s *Service) CaptureFactorySnapshot(
	_ context.Context,
	request factorydefinitions.CaptureFactorySnapshotRequest,
) (factorydefinitions.CaptureFactorySnapshotResult, error) {
	if s == nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	object, err := decodeFactoryObject(request.Canonical)
	if err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	if name := strings.TrimSpace(request.Name); name != "" {
		object["name"] = name
	}
	if factoryDir := strings.TrimSpace(request.FactoryDir); factoryDir != "" {
		object["factoryDirectory"] = factoryDir
	}
	snapshot, err := factorydefinitions.NewFactorySnapshot(object)
	if err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	return factorydefinitions.CaptureFactorySnapshotResult{Snapshot: snapshot}, nil
}

// PrepareFactorySnapshotImport decodes a valid object snapshot payload into
// Definitions-owned portable import facts.
func (s *Service) PrepareFactorySnapshotImport(
	_ context.Context,
	request factorydefinitions.PrepareFactorySnapshotImportRequest,
) (factorydefinitions.PrepareFactorySnapshotImportResult, error) {
	if s == nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	object, err := decodeFactoryObject(request.Payload)
	if err != nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	snapshot, err := factorydefinitions.NewFactorySnapshot(object)
	if err != nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	name := stringField(object, "name")
	factoryDir := stringField(object, "factoryDirectory")
	return factorydefinitions.PrepareFactorySnapshotImportResult{
		Snapshot: snapshot,
		Name:     name,
		Portable: factorydefinitions.PortableFactorySnapshotFacts{
			FactoryDir: factoryDir,
			Assets:     portableAssets(object),
		},
	}, nil
}

// MaterializeFactorySnapshot materializes one safe detached snapshot into
// Definitions-owned portable success facts.
func (s *Service) MaterializeFactorySnapshot(
	_ context.Context,
	request factorydefinitions.MaterializeFactorySnapshotRequest,
) (factorydefinitions.MaterializeFactorySnapshotResult, error) {
	if s == nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}
	targetDir := strings.TrimSpace(request.TargetDir)
	if targetDir == "" || request.Snapshot == nil || !safeMaterializeTarget(targetDir) {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}
	var object map[string]any
	if err := request.Snapshot.Decode(&object); err != nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}
	return factorydefinitions.MaterializeFactorySnapshotResult{
		TargetDir: targetDir,
		Portable: factorydefinitions.PortableFactorySnapshotFacts{
			FactoryDir: targetDir,
			Assets:     portableAssets(object),
		},
	}, nil
}

func decodeFactoryObject(payload []byte) (map[string]any, error) {
	snapshot, err := factorydefinitions.NewFactorySnapshot(json.RawMessage(payload))
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := snapshot.Decode(&object); err != nil {
		return nil, err
	}
	return object, nil
}

func stringField(object map[string]any, key string) string {
	value, ok := object[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func portableAssets(object map[string]any) []factorydefinitions.PortableSnapshotAssetFact {
	manifest, ok := object["resourceManifest"].(map[string]any)
	if !ok {
		return nil
	}
	rawFiles, ok := manifest["bundledFiles"].([]any)
	if !ok || len(rawFiles) == 0 {
		return nil
	}
	assets := make([]factorydefinitions.PortableSnapshotAssetFact, 0, len(rawFiles))
	for _, raw := range rawFiles {
		file, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		targetPath := stringField(file, "targetPath")
		if targetPath == "" {
			continue
		}
		assets = append(assets, factorydefinitions.PortableSnapshotAssetFact{
			TargetPath: targetPath,
		})
	}
	if len(assets) == 0 {
		return nil
	}
	return assets
}

func safeMaterializeTarget(targetDir string) bool {
	cleaned := filepath.Clean(targetDir)
	if cleaned == "." || cleaned == string(filepath.Separator) {
		return false
	}
	if strings.Contains(cleaned, "..") {
		return false
	}
	for _, segment := range strings.Split(filepath.ToSlash(cleaned), "/") {
		if segment == ".." {
			return false
		}
	}
	return true
}
