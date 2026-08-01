package materialize

import (
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Snapshot restores bundled portable assets under one target directory and
// returns Definitions-owned portable materialize success facts.
func Snapshot(
	targetDir string,
	snapshot *factorydefinitions.FactorySnapshot,
	validateMaterializeWrites factorydefinitions.PortableBundledFileWritesValidator,
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer,
) (factorydefinitions.MaterializeFactorySnapshotResult, error) {
	if validateMaterializeWrites == nil || materializePortableFiles == nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" || snapshot == nil || !safeTarget(targetDir) {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}

	factoryConfig, assets, err := factoryConfigFromSnapshot(snapshot)
	if err != nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}
	if err := validateMaterializeWrites(targetDir, factoryConfig); err != nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}
	if _, err := materializePortableFiles(targetDir, factoryConfig); err != nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}

	return factorydefinitions.MaterializeFactorySnapshotResult{
		TargetDir: targetDir,
		Portable: factorydefinitions.PortableFactorySnapshotFacts{
			FactoryDir: targetDir,
			Assets:     assets,
		},
	}, nil
}

func factoryConfigFromSnapshot(
	snapshot *factorydefinitions.FactorySnapshot,
) (*factorydefinitions.FactoryConfig, []factorydefinitions.PortableSnapshotAssetFact, error) {
	var factoryConfig factorydefinitions.FactoryConfig
	if err := snapshot.Decode(&factoryConfig); err != nil {
		return nil, nil, err
	}
	if factoryConfig.ResourceManifest == nil {
		factoryConfig.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{}
	}
	return &factoryConfig, portableAssetsFromConfig(&factoryConfig), nil
}

func portableAssetsFromConfig(
	factoryConfig *factorydefinitions.FactoryConfig,
) []factorydefinitions.PortableSnapshotAssetFact {
	if factoryConfig == nil ||
		factoryConfig.ResourceManifest == nil ||
		len(factoryConfig.ResourceManifest.BundledFiles) == 0 {
		return nil
	}
	assets := make(
		[]factorydefinitions.PortableSnapshotAssetFact,
		0,
		len(factoryConfig.ResourceManifest.BundledFiles),
	)
	for _, bundledFile := range factoryConfig.ResourceManifest.BundledFiles {
		targetPath := strings.TrimSpace(bundledFile.TargetPath)
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

func safeTarget(targetDir string) bool {
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
