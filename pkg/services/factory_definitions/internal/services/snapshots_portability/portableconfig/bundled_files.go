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

const portableBundledFactoryPrefix = "factory"
const portableBundledScriptRoot = "factory/scripts/"
const portableBundledDocRoot = "factory/docs/"
const portableBundledInputRoot = "factory/inputs/"

var portableBundledDirectoryNames = []string{"scripts", "docs"}
var portableBundledFactoryRootHelperFiles = []string{"portable-dependencies.json"}

// applySupportedFiles discovers supported authored files and projects them
// onto the Factory Definition portability manifest.
func applySupportedFiles(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
	discoverUnlistedDocs bool,
	fileSystem portablefiles.FileSystem,
) error {
	if factoryConfig == nil {
		return nil
	}

	collected, err := collectSupportedFiles(
		factoryDir,
		includeInlineContent,
		fileSystem,
	)
	if err != nil {
		return err
	}
	rehydrated, err := rehydrateExplicitRootHelpers(
		factoryDir,
		factoryConfig,
		includeInlineContent,
		fileSystem,
	)
	if err != nil {
		return err
	}
	collected = append(collected, rehydrated...)
	if len(collected) == 0 {
		return nil
	}

	if factoryConfig.ResourceManifest == nil {
		factoryConfig.ResourceManifest =
			&factorydefinitions.PortableResourceManifestConfig{}
	}
	factoryConfig.ResourceManifest.BundledFiles =
		mergeDiscoveredFiles(
			factoryConfig.ResourceManifest.BundledFiles,
			collected,
			discoverUnlistedDocs,
		)
	return nil
}

func collectSupportedFiles(
	factoryDir string,
	includeInlineContent bool,
	fileSystem portablefiles.FileSystem,
) ([]factorydefinitions.BundledFileConfig, error) {
	resolvedFactoryDir, ok := portableFactoryDirectory(fileSystem, factoryDir)
	if !ok {
		return nil, nil
	}

	bundledFiles := make([]factorydefinitions.BundledFileConfig, 0)
	for _, directoryName := range portableBundledDirectoryNames {
		rootDir := filepath.Join(resolvedFactoryDir, directoryName)
		targetRoot := filepath.ToSlash(filepath.Join(
			portableBundledFactoryPrefix,
			directoryName,
		))
		fileType := factorydefinitions.BundledFileTypeDoc
		if directoryName == "scripts" {
			fileType = factorydefinitions.BundledFileTypeScript
		}
		collected, err := collectFilesFromDir(
			rootDir,
			targetRoot,
			fileType,
			includeInlineContent,
			fileSystem,
		)
		if err != nil {
			return nil, err
		}
		bundledFiles = append(bundledFiles, collected...)
	}

	for _, helperName := range portableBundledFactoryRootHelperFiles {
		targetPath := filepath.ToSlash(filepath.Join(
			portableBundledFactoryPrefix,
			helperName,
		))
		bundledFile, ok, err := collectRootHelperFile(
			filepath.Join(resolvedFactoryDir, helperName),
			targetPath,
			fileSystem,
		)
		if err != nil {
			return nil, err
		}
		if ok {
			bundledFiles = append(bundledFiles, bundledFile)
		}
	}

	sort.Slice(bundledFiles, func(i, j int) bool {
		return bundledFiles[i].TargetPath < bundledFiles[j].TargetPath
	})
	return bundledFiles, nil
}

func rehydrateExplicitRootHelpers(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
	fileSystem portablefiles.FileSystem,
) ([]factorydefinitions.BundledFileConfig, error) {
	if factoryConfig == nil ||
		factoryConfig.ResourceManifest == nil ||
		!includeInlineContent {
		return nil, nil
	}

	rehydrated := make([]factorydefinitions.BundledFileConfig, 0)
	for _, bundledFile := range factoryConfig.ResourceManifest.BundledFiles {
		if bundledFile.Type != factorydefinitions.BundledFileTypeRootHelper {
			continue
		}
		sourcePath, ok := resolveSupportedSourcePath(
			fileSystem,
			factoryDir,
			bundledFile,
		)
		if !ok {
			continue
		}
		collected, ok, err := collectRootHelperFile(
			sourcePath,
			bundledFile.TargetPath,
			fileSystem,
		)
		if err != nil {
			return nil, err
		}
		if ok {
			rehydrated = append(rehydrated, collected)
		}
	}
	return rehydrated, nil
}

func collectFilesFromDir(
	sourceDir string,
	targetRoot string,
	fileType string,
	includeInlineContent bool,
	fileSystem portablefiles.FileSystem,
) ([]factorydefinitions.BundledFileConfig, error) {
	info, err := fileSystem.Stat(sourceDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"stat portable bundled directory %s: %w",
			sourceDir,
			err,
		)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf(
			"portable bundled directory %s must be a directory",
			sourceDir,
		)
	}

	bundledFiles := make([]factorydefinitions.BundledFileConfig, 0)
	if err := fileSystem.WalkDir(sourceDir, func(
		sourcePath string,
		entry fs.DirEntry,
		walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() ||
			!entry.Type().IsRegular() ||
			isIgnoredPortableFile(filepath.Base(sourcePath)) {
			return nil
		}

		relativePath, err := filepath.Rel(sourceDir, sourcePath)
		if err != nil {
			return fmt.Errorf(
				"resolve bundled file path %s: %w",
				sourcePath,
				err,
			)
		}
		inline := ""
		if includeInlineContent {
			content, err := fileSystem.ReadFile(sourcePath)
			if err != nil {
				return fmt.Errorf(
					"read portable bundled file %s: %w",
					sourcePath,
					err,
				)
			}
			inline = string(content)
		}
		bundledFiles = append(
			bundledFiles,
			factorydefinitions.BundledFileConfig{
				Type: fileType,
				TargetPath: filepath.ToSlash(filepath.Join(
					targetRoot,
					relativePath,
				)),
				Content: factorydefinitions.BundledFileContentConfig{
					Encoding: factorydefinitions.BundledFileEncodingUTF8,
					Inline:   inline,
				},
			},
		)
		return nil
	}); err != nil {
		return nil, fmt.Errorf(
			"collect portable bundled files from %s: %w",
			sourceDir,
			err,
		)
	}
	return bundledFiles, nil
}

func collectRootHelperFile(
	sourcePath string,
	targetPath string,
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.BundledFileConfig, bool, error) {
	info, err := fileSystem.Stat(sourcePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return factorydefinitions.BundledFileConfig{}, false, nil
		}
		return factorydefinitions.BundledFileConfig{}, false, fmt.Errorf(
			"stat portable bundled helper file %s: %w",
			sourcePath,
			err,
		)
	}
	if !info.Mode().IsRegular() {
		return factorydefinitions.BundledFileConfig{}, false, nil
	}
	content, err := fileSystem.ReadFile(sourcePath)
	if err != nil {
		return factorydefinitions.BundledFileConfig{}, false, fmt.Errorf(
			"read portable bundled helper file %s: %w",
			sourcePath,
			err,
		)
	}
	return factorydefinitions.BundledFileConfig{
		Type:       factorydefinitions.BundledFileTypeRootHelper,
		TargetPath: filepath.ToSlash(targetPath),
		Content: factorydefinitions.BundledFileContentConfig{
			Encoding: factorydefinitions.BundledFileEncodingUTF8,
			Inline:   string(content),
		},
	}, true, nil
}

// NewSupportedSourceResolver binds supported-manifest source resolution to
// the filesystem selected at the process edge.
func NewSupportedSourceResolver(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.PortableBundledFileSourceResolver, error) {
	if fileSystem == nil {
		return nil, fmt.Errorf("portable filesystem is required")
	}
	return func(factoryDir string, bundledFile factorydefinitions.BundledFileConfig) (string, bool) {
		return resolveSupportedSourcePath(fileSystem, factoryDir, bundledFile)
	}, nil
}

func resolveSupportedSourcePath(
	fileSystem portablefiles.FileSystem,
	factoryDir string,
	bundledFile factorydefinitions.BundledFileConfig,
) (string, bool) {
	targetPath := filepath.ToSlash(strings.TrimSpace(bundledFile.TargetPath))
	switch bundledFile.Type {
	case factorydefinitions.BundledFileTypeScript:
		return supportedSubdirPath(
			factoryDir,
			targetPath,
			portableBundledScriptRoot,
			"scripts",
		)
	case factorydefinitions.BundledFileTypeDoc:
		return supportedSubdirPath(
			factoryDir,
			targetPath,
			portableBundledDocRoot,
			"docs",
		)
	case factorydefinitions.BundledFileTypeInput:
		return supportedSubdirPath(
			factoryDir,
			targetPath,
			portableBundledInputRoot,
			factorydefinitions.InputsDir,
		)
	case factorydefinitions.BundledFileTypeRootHelper:
		return supportedRootHelperPath(fileSystem, factoryDir, targetPath)
	default:
		return "", false
	}
}

// MergeDiscoveredFiles applies manifest-authoritative document rules while
// merging discovered authored files.
func MergeDiscoveredFiles(
	existing []factorydefinitions.BundledFileConfig,
	collected []factorydefinitions.BundledFileConfig,
	discoverUnlistedDocs bool,
) []factorydefinitions.BundledFileConfig {
	return mergeDiscoveredFiles(existing, collected, discoverUnlistedDocs)
}

func supportedSubdirPath(
	factoryDir string,
	targetPath string,
	targetRoot string,
	localDirectory string,
) (string, bool) {
	if !strings.HasPrefix(targetPath, targetRoot) {
		return "", false
	}
	relativePath := strings.TrimPrefix(targetPath, targetRoot)
	if relativePath == "" {
		return "", false
	}
	return filepath.Join(
		factoryDir,
		filepath.FromSlash(filepath.Join(localDirectory, relativePath)),
	), true
}

func supportedRootHelperPath(
	fileSystem portablefiles.FileSystem,
	factoryDir string,
	targetPath string,
) (string, bool) {
	switch targetPath {
	case "Makefile":
		factoryLocalPath := filepath.Join(factoryDir, "Makefile")
		if _, err := fileSystem.Stat(factoryLocalPath); err == nil {
			return factoryLocalPath, true
		}
		return filepath.Join(filepath.Dir(factoryDir), "Makefile"), true
	case "factory/portable-dependencies.json":
		return filepath.Join(factoryDir, "portable-dependencies.json"), true
	default:
		return "", false
	}
}

func portableFactoryDirectory(fileSystem portablefiles.FileSystem, factoryDir string) (string, bool) {
	cleanFactoryDir := filepath.Clean(factoryDir)
	if filepath.Base(cleanFactoryDir) == portableBundledFactoryPrefix {
		return cleanFactoryDir, true
	}

	factoryPath := filepath.Join(
		cleanFactoryDir,
		factorydefinitions.FactoryConfigFile,
	)
	info, err := fileSystem.Stat(factoryPath)
	if err != nil || info.IsDir() {
		return "", false
	}
	pointerPath := filepath.Join(
		filepath.Dir(cleanFactoryDir),
		factorydefinitions.CurrentFactoryPointerFile,
	)
	if _, err := fileSystem.Stat(pointerPath); err != nil {
		return "", false
	}
	return cleanFactoryDir, true
}

func mergeDiscoveredFiles(
	existing []factorydefinitions.BundledFileConfig,
	collected []factorydefinitions.BundledFileConfig,
	discoverUnlistedDocs bool,
) []factorydefinitions.BundledFileConfig {
	collected = filterDiscoveredFiles(
		existing,
		collected,
		discoverUnlistedDocs,
	)
	return mergeBundledFiles(existing, collected)
}

func filterDiscoveredFiles(
	existing []factorydefinitions.BundledFileConfig,
	collected []factorydefinitions.BundledFileConfig,
	discoverUnlistedDocs bool,
) []factorydefinitions.BundledFileConfig {
	if discoverUnlistedDocs || len(collected) == 0 {
		return collected
	}
	manifestDocPaths := manifestDocTargetPaths(existing)
	if len(manifestDocPaths) == 0 {
		return collected
	}

	filtered := make([]factorydefinitions.BundledFileConfig, 0, len(collected))
	for _, bundledFile := range collected {
		if bundledFile.Type == factorydefinitions.BundledFileTypeDoc &&
			!manifestDocPaths[bundledFile.TargetPath] &&
			!isNestedDocTarget(bundledFile.TargetPath) {
			continue
		}
		filtered = append(filtered, bundledFile)
	}
	return filtered
}

func isNestedDocTarget(targetPath string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(targetPath))
	if !strings.HasPrefix(normalized, portableBundledDocRoot) {
		return false
	}
	relativePath := strings.TrimPrefix(normalized, portableBundledDocRoot)
	return strings.Contains(relativePath, "/")
}

func manifestDocTargetPaths(
	bundledFiles []factorydefinitions.BundledFileConfig,
) map[string]bool {
	paths := make(map[string]bool)
	for _, bundledFile := range bundledFiles {
		if bundledFile.Type == factorydefinitions.BundledFileTypeDoc {
			paths[bundledFile.TargetPath] = true
		}
	}
	return paths
}
