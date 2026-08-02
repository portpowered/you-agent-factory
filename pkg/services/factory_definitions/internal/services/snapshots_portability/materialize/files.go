package materialize

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotscontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/contracts"
)

const portableBundledFactoryPrefix = "factory"

type bundledFileWrite struct {
	targetPath     string
	targetLocation string
	content        string
	mode           fs.FileMode
}

// NewMaterializer binds portable-file materialization to an injected filesystem.
func NewMaterializer(
	fileSystem portablefiles.FileSystem,
) snapshotscontracts.PortableBundledFilesMaterializer {
	return func(targetDir string, config *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
		return MaterializeFiles(fileSystem, targetDir, config)
	}
}

// NewWritesValidator binds portable-path validation to an injected filesystem.
func NewWritesValidator(
	fileSystem portablefiles.FileSystem,
) snapshotscontracts.PortableBundledFileWritesValidator {
	return func(targetDir string, config *factorydefinitions.FactoryConfig) error {
		return ValidateWrites(fileSystem, targetDir, config)
	}
}

// MaterializeFiles restores inline portable assets described by a Factory
// Definition and reports existing files whose contents changed.
func MaterializeFiles(
	fileSystem portablefiles.FileSystem,
	targetDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) ([]factorydefinitions.PortableBundledFileReplacement, error) {
	resolvedWrites, err := prepareFileWrites(fileSystem, targetDir, factoryConfig)
	if err != nil {
		return nil, err
	}

	replacements := make(
		[]factorydefinitions.PortableBundledFileReplacement,
		0,
	)
	for _, write := range resolvedWrites {
		if err := fileSystem.MkdirAll(filepath.Dir(write.targetPath), 0o755); err != nil {
			return nil, fmt.Errorf(
				"create bundled file directory for %s: %w",
				write.targetPath,
				err,
			)
		}
		replaced, err := portablefiles.ReplacementNeeded(
			fileSystem,
			write.targetPath,
			[]byte(write.content),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"inspect bundled file %s: %w",
				write.targetPath,
				err,
			)
		}
		if replaced {
			replacements = append(
				replacements,
				factorydefinitions.PortableBundledFileReplacement{
					TargetPath: write.targetLocation,
				},
			)
		}
		if err := portablefiles.WriteFile(
			fileSystem,
			write.targetPath,
			[]byte(write.content),
			write.mode,
		); err != nil {
			return nil, fmt.Errorf(
				"write bundled file %s: %w",
				write.targetPath,
				err,
			)
		}
	}
	if err := normalizeFileModes(fileSystem, targetDir, factoryConfig); err != nil {
		return nil, err
	}
	return replacements, nil
}

// ValidateWrites checks that all inline portable assets can be safely
// resolved without mutating the filesystem.
func ValidateWrites(
	fileSystem portablefiles.FileSystem,
	targetDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) error {
	_, err := prepareFileWrites(fileSystem, targetDir, factoryConfig)
	return err
}

func prepareFileWrites(
	fileSystem portablefiles.FileSystem,
	targetDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) ([]bundledFileWrite, error) {
	if factoryConfig == nil ||
		factoryConfig.ResourceManifest == nil ||
		len(factoryConfig.ResourceManifest.BundledFiles) == 0 {
		return nil, nil
	}
	validationRoot, err := portablefiles.PrepareValidationRoot(fileSystem, targetDir)
	if err != nil {
		return nil, err
	}
	bundledFiles := append(
		[]factorydefinitions.BundledFileConfig(nil),
		factoryConfig.ResourceManifest.BundledFiles...,
	)
	sort.Slice(bundledFiles, func(i, j int) bool {
		return bundledFiles[i].TargetPath < bundledFiles[j].TargetPath
	})

	resolvedWrites := make([]bundledFileWrite, 0, len(bundledFiles))
	for _, bundledFile := range bundledFiles {
		if strings.TrimSpace(bundledFile.Content.Inline) == "" {
			continue
		}
		if err := factorydefinitions.ValidatePortableBundledFileType(
			bundledFile,
		); err != nil {
			return nil, err
		}
		if err := factorydefinitions.ValidatePortableBundledFileTarget(
			bundledFile,
		); err != nil {
			return nil, err
		}
		target, err := portablefiles.ResolveTarget(
			validationRoot,
			bundledFile.TargetPath,
			portableBundledFactoryPrefix,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"resolve bundled file %q: %w",
				bundledFile.TargetPath,
				err,
			)
		}
		if err := portablefiles.ValidateFilesystemPath(
			fileSystem,
			validationRoot,
			bundledFile.TargetPath,
			target,
		); err != nil {
			return nil, fmt.Errorf(
				"resolve bundled file %q: %w",
				bundledFile.TargetPath,
				err,
			)
		}
		resolvedWrites = append(resolvedWrites, bundledFileWrite{
			targetPath:     target.Path(),
			targetLocation: bundledFile.TargetPath,
			content:        bundledFile.Content.Inline,
			mode:           bundledFileMode(bundledFile),
		})
	}
	return resolvedWrites, nil
}

func normalizeFileModes(
	fileSystem portablefiles.FileSystem,
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) error {
	if factoryConfig == nil || factoryConfig.ResourceManifest == nil {
		return nil
	}
	for _, bundledFile := range factoryConfig.ResourceManifest.BundledFiles {
		if !factorydefinitions.ShouldOmitSupportedPortableBundledInline(
			bundledFile,
		) || strings.TrimSpace(bundledFile.Content.Inline) != "" {
			continue
		}
		sourcePath, ok := resolveSupportedSourcePath(fileSystem, factoryDir, bundledFile)
		if !ok {
			continue
		}
		info, err := fileSystem.Stat(sourcePath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf(
				"stat bundled file %s: %w",
				sourcePath,
				err,
			)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := fileSystem.Chmod(sourcePath, bundledFileMode(bundledFile)); err != nil {
			return fmt.Errorf(
				"normalize bundled file mode %s: %w",
				sourcePath,
				err,
			)
		}
	}
	return nil
}

func resolveSupportedSourcePath(
	fileSystem portablefiles.FileSystem,
	factoryDir string,
	bundledFile factorydefinitions.BundledFileConfig,
) (string, bool) {
	resolvedFactoryDir, ok := portableFactoryDirectory(fileSystem, factoryDir)
	if !ok {
		return "", false
	}
	relative := strings.TrimPrefix(bundledFile.TargetPath, portableBundledFactoryPrefix+"/")
	if relative == bundledFile.TargetPath {
		return "", false
	}
	return filepath.Join(resolvedFactoryDir, filepath.FromSlash(relative)), true
}

func portableFactoryDirectory(fileSystem portablefiles.FileSystem, factoryDir string) (string, bool) {
	cleanFactoryDir := filepath.Clean(factoryDir)
	if filepath.Base(cleanFactoryDir) == portableBundledFactoryPrefix {
		return cleanFactoryDir, true
	}
	factoryChild := filepath.Join(cleanFactoryDir, portableBundledFactoryPrefix)
	if info, err := fileSystem.Stat(factoryChild); err == nil && info.IsDir() {
		return factoryChild, true
	}
	return cleanFactoryDir, true
}

func bundledFileMode(
	bundledFile factorydefinitions.BundledFileConfig,
) fs.FileMode {
	if bundledFile.Type == factorydefinitions.BundledFileTypeScript {
		return 0o755
	}
	return 0o644
}
