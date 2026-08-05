package materialize

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Snapshot restores bundled portable assets under one target directory and
// returns Definitions-owned portable materialize success facts.
func Snapshot(
	targetDir string,
	snapshot *factorydefinitions.FactorySnapshot,
	factoryConfig *factorydefinitions.FactoryConfig,
	validateMaterializeWrites factorydefinitions.PortableBundledFileWritesValidator,
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer,
	fileSystem factorydefinitions.SnapshotMaterializationFileSystem,
	directories factorydefinitions.DirectoryReplacementStore,
) (factorydefinitions.MaterializeFactorySnapshotResult, error) {
	if validateMaterializeWrites == nil || materializePortableFiles == nil || fileSystem == nil || directories == nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" || snapshot == nil || factoryConfig == nil || !safeTarget(targetDir) {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}
	if err := validateMaterializeWrites(targetDir, factoryConfig); err != nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}
	if err := stageAndPublish(
		targetDir,
		snapshot,
		factoryConfig,
		materializePortableFiles,
		fileSystem,
		directories,
	); err != nil {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, err
	}

	return factorydefinitions.MaterializeFactorySnapshotResult{
		TargetDir: targetDir,
		Portable: factorydefinitions.PortableFactorySnapshotFacts{
			FactoryDir: targetDir,
			Assets:     portableAssetsFromConfig(factoryConfig),
		},
	}, nil
}

func stageAndPublish(
	targetDir string,
	snapshot *factorydefinitions.FactorySnapshot,
	factoryConfig *factorydefinitions.FactoryConfig,
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer,
	fileSystem factorydefinitions.SnapshotMaterializationFileSystem,
	directories factorydefinitions.DirectoryReplacementStore,
) error {
	parentDir := filepath.Dir(targetDir)
	segment := filepath.Base(targetDir)
	if parentDir == "." || segment == "." || segment == string(filepath.Separator) {
		return factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}
	if info, err := fileSystem.Stat(parentDir); err != nil || !info.IsDir() {
		return fmt.Errorf("snapshot target parent is unavailable")
	}
	stagingDir, err := fileSystem.MkdirTemp(parentDir, "."+segment+".snapshot-")
	if err != nil {
		return fmt.Errorf("create snapshot staging directory: %w", err)
	}
	defer func() { _ = fileSystem.RemoveAll(stagingDir) }()

	if err := writeStagedSnapshot(stagingDir, snapshot, factoryConfig, materializePortableFiles, fileSystem); err != nil {
		return err
	}
	if err := publishStagedSnapshot(parentDir, targetDir, stagingDir, fileSystem, directories); err != nil {
		return err
	}
	return nil
}

func writeStagedSnapshot(
	stagingDir string,
	snapshot *factorydefinitions.FactorySnapshot,
	factoryConfig *factorydefinitions.FactoryConfig,
	materializePortableFiles factorydefinitions.PortableBundledFilesMaterializer,
	fileSystem factorydefinitions.SnapshotMaterializationFileSystem,
) error {
	if err := fileSystem.WriteFile(
		filepath.Join(stagingDir, factorydefinitions.FactoryConfigFile),
		bytes.Clone(*snapshot),
		0o644,
	); err != nil {
		return fmt.Errorf("write snapshot Factory source: %w", err)
	}
	if _, err := materializePortableFiles(stagingDir, factoryConfig); err != nil {
		return fmt.Errorf("materialize snapshot bundled artifacts: %w", err)
	}
	return nil
}

func publishStagedSnapshot(
	parentDir string,
	targetDir string,
	stagingDir string,
	fileSystem factorydefinitions.SnapshotMaterializationFileSystem,
	directories factorydefinitions.DirectoryReplacementStore,
) error {
	info, err := fileSystem.Stat(targetDir)
	if errors.Is(err, fs.ErrNotExist) {
		if err := fileSystem.Rename(stagingDir, targetDir); err != nil {
			return fmt.Errorf("publish new snapshot target: %w", err)
		}
		return nil
	}
	if err != nil || !info.IsDir() {
		return fmt.Errorf("snapshot target is not a directory")
	}
	backupDir, err := directories.Commit(parentDir, targetDir, stagingDir)
	if err != nil {
		return fmt.Errorf("publish snapshot target: %w", err)
	}
	if backupDir != "" {
		_ = fileSystem.RemoveAll(backupDir)
	}
	return nil
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
