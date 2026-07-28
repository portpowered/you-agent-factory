package portableconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const portableFactoryDirectoryName = "factory"
const portableBatchInputDirectoryName = "BATCH"

// applyStarterWork snapshots eligible inputs/ files into the portability
// manifest without retaining links to their source directory.
func applyStarterWork(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	fileSystem portablefiles.FileSystem,
) error {
	if factoryConfig == nil {
		return nil
	}

	collected, err := collectStarterWork(
		factoryDir,
		factoryConfig,
		fileSystem,
	)
	if err != nil {
		return err
	}

	var existing []factorydefinitions.BundledFileConfig
	if factoryConfig.ResourceManifest != nil {
		existing = removeBundledFilesByType(
			factoryConfig.ResourceManifest.BundledFiles,
			factorydefinitions.BundledFileTypeInput,
		)
	}
	if len(collected) == 0 {
		if factoryConfig.ResourceManifest != nil {
			factoryConfig.ResourceManifest.BundledFiles = existing
		}
		return nil
	}

	if factoryConfig.ResourceManifest == nil {
		factoryConfig.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{}
	}
	factoryConfig.ResourceManifest.BundledFiles = mergeBundledFiles(existing, collected)
	return nil
}

func collectStarterWork(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	fileSystem portablefiles.FileSystem,
) ([]factorydefinitions.BundledFileConfig, error) {
	inputsDir := filepath.Join(factoryDir, factorydefinitions.InputsDir)
	info, err := fileSystem.Stat(inputsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat shared Factory inputs %s: %w", inputsDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("shared Factory inputs %s must be a directory", inputsDir)
	}

	validWorkTypes := map[string]bool{portableBatchInputDirectoryName: true}
	for _, workType := range factoryConfig.WorkTypes {
		name := strings.TrimSpace(workType.Name)
		if name != "" {
			validWorkTypes[name] = true
		}
	}

	bundledFiles := make([]factorydefinitions.BundledFileConfig, 0)
	if err := fileSystem.WalkDir(inputsDir, func(
		path string,
		entry fs.DirEntry,
		walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() ||
			isIgnoredPortableFile(filepath.Base(path)) {
			return nil
		}
		return appendStarterWorkFile(
			&bundledFiles,
			inputsDir,
			path,
			validWorkTypes,
			fileSystem,
		)
	}); err != nil {
		return nil, fmt.Errorf("collect shared Factory starter Work: %w", err)
	}

	sort.Slice(bundledFiles, func(i, j int) bool {
		return bundledFiles[i].TargetPath < bundledFiles[j].TargetPath
	})
	return bundledFiles, nil
}

func appendStarterWorkFile(
	bundledFiles *[]factorydefinitions.BundledFileConfig,
	inputsDir string,
	sourcePath string,
	validWorkTypes map[string]bool,
	fileSystem portablefiles.FileSystem,
) error {
	relativePath, ok, err := starterWorkRelativePath(
		inputsDir,
		sourcePath,
		validWorkTypes,
	)
	if err != nil || !ok {
		return err
	}
	content, err := fileSystem.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read starter Work %s: %w", sourcePath, err)
	}
	*bundledFiles = append(*bundledFiles, factorydefinitions.BundledFileConfig{
		Type: factorydefinitions.BundledFileTypeInput,
		TargetPath: filepath.ToSlash(filepath.Join(
			portableFactoryDirectoryName,
			factorydefinitions.InputsDir,
			relativePath,
		)),
		Content: factorydefinitions.BundledFileContentConfig{
			Encoding: factorydefinitions.BundledFileEncodingUTF8,
			Inline:   string(content),
		},
	})
	return nil
}

func starterWorkRelativePath(
	inputsDir string,
	sourcePath string,
	validWorkTypes map[string]bool,
) (string, bool, error) {
	relativePath, err := filepath.Rel(inputsDir, sourcePath)
	if err != nil {
		return "", false, fmt.Errorf(
			"resolve starter Work path %s: %w",
			sourcePath,
			err,
		)
	}
	relativePath = filepath.ToSlash(relativePath)
	parts := strings.Split(relativePath, "/")
	if len(parts) != 3 || !validWorkTypes[parts[0]] {
		return "", false, nil
	}
	return relativePath, true, nil
}

func isIgnoredPortableFile(name string) bool {
	if name == ".gitkeep" {
		return true
	}
	return strings.HasSuffix(name, ".tmp") ||
		strings.HasSuffix(name, ".swp") ||
		strings.HasSuffix(name, "~")
}

func removeBundledFilesByType(
	bundledFiles []factorydefinitions.BundledFileConfig,
	fileType string,
) []factorydefinitions.BundledFileConfig {
	if len(bundledFiles) == 0 {
		return nil
	}
	filtered := make([]factorydefinitions.BundledFileConfig, 0, len(bundledFiles))
	for _, bundledFile := range bundledFiles {
		if bundledFile.Type != fileType {
			filtered = append(filtered, bundledFile)
		}
	}
	return filtered
}

func mergeBundledFiles(
	existing []factorydefinitions.BundledFileConfig,
	collected []factorydefinitions.BundledFileConfig,
) []factorydefinitions.BundledFileConfig {
	byTarget := make(
		map[string]factorydefinitions.BundledFileConfig,
		len(existing)+len(collected),
	)
	for _, bundledFile := range existing {
		byTarget[bundledFile.TargetPath] = bundledFile
	}
	for _, bundledFile := range collected {
		byTarget[bundledFile.TargetPath] = bundledFile
	}

	targets := make([]string, 0, len(byTarget))
	for target := range byTarget {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	merged := make([]factorydefinitions.BundledFileConfig, 0, len(targets))
	for _, target := range targets {
		merged = append(merged, byTarget[target])
	}
	return merged
}
