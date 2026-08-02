package portableconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotscontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/contracts"
	snapshotsportabilitymaterialize "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/materialize"
)

// NewFilesCopier binds portable-file copying to an injected symlink resolver.
func NewFilesCopier(
	fileSystem portablefiles.FileSystem,
) snapshotscontracts.PortableBundledFilesCopier {
	return func(sourceDir, targetDir string, config *factorydefinitions.FactoryConfig) error {
		return CopySupportedFiles(fileSystem, sourceDir, targetDir, config)
	}
}

// NewMaterializer binds portable-file materialization to an injected symlink
// resolver.
func NewMaterializer(
	fileSystem portablefiles.FileSystem,
) snapshotscontracts.PortableBundledFilesMaterializer {
	return snapshotsportabilitymaterialize.NewMaterializer(fileSystem)
}

// NewWritesValidator binds portable-path validation to an injected symlink
// resolver.
func NewWritesValidator(
	fileSystem portablefiles.FileSystem,
) snapshotscontracts.PortableBundledFileWritesValidator {
	return snapshotsportabilitymaterialize.NewWritesValidator(fileSystem)
}

// MaterializeFiles restores inline portable assets described by a Factory
// Definition and reports existing files whose contents changed.
func MaterializeFiles(
	fileSystem portablefiles.FileSystem,
	targetDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) ([]factorydefinitions.PortableBundledFileReplacement, error) {
	return snapshotsportabilitymaterialize.MaterializeFiles(fileSystem, targetDir, factoryConfig)
}

// ValidateWrites checks that all inline portable assets can be safely
// resolved without mutating the filesystem.
func ValidateWrites(
	fileSystem portablefiles.FileSystem,
	targetDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) error {
	return snapshotsportabilitymaterialize.ValidateWrites(fileSystem, targetDir, factoryConfig)
}

// CloneReplacements returns a caller-owned replacement list.
func CloneReplacements(
	replacements []factorydefinitions.PortableBundledFileReplacement,
) []factorydefinitions.PortableBundledFileReplacement {
	return append(
		[]factorydefinitions.PortableBundledFileReplacement(nil),
		replacements...,
	)
}

// PruneRemovedDocs removes authored documentation files no longer declared by
// the Factory Definition manifest.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func PruneRemovedDocs(
	fileSystem portablefiles.FileSystem,
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) error {
	if fileSystem == nil {
		return fmt.Errorf("portable filesystem is required")
	}
	resolvedFactoryDir, ok := portableFactoryDirectory(fileSystem, factoryDir)
	if !ok {
		return nil
	}

	docsDir := filepath.Join(resolvedFactoryDir, "docs")
	info, err := fileSystem.Stat(docsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf(
			"stat portable bundled docs directory %s: %w",
			docsDir,
			err,
		)
	}
	if !info.IsDir() {
		return nil
	}

	allowed := make(map[string]bool)
	if factoryConfig != nil && factoryConfig.ResourceManifest != nil {
		for _, bundledFile := range factoryConfig.ResourceManifest.BundledFiles {
			if bundledFile.Type == factorydefinitions.BundledFileTypeDoc {
				allowed[bundledFile.TargetPath] = true
			}
		}
	}

	return fileSystem.WalkDir(
		docsDir,
		func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() ||
				!entry.Type().IsRegular() ||
				isIgnoredPortableFile(filepath.Base(filePath)) {
				return nil
			}
			relativePath, err := filepath.Rel(docsDir, filePath)
			if err != nil {
				return fmt.Errorf(
					"resolve bundled doc path %s: %w",
					filePath,
					err,
				)
			}
			targetPath := filepath.ToSlash(filepath.Join(
				portableBundledDocRoot,
				relativePath,
			))
			if allowed[targetPath] {
				return nil
			}
			if err := fileSystem.Remove(filePath); err != nil {
				return fmt.Errorf(
					"remove bundled doc %s: %w",
					filePath,
					err,
				)
			}
			return nil
		},
	)
}

// CopySupportedFiles copies thin, disk-backed portable assets from one
// Factory layout into another.
func CopySupportedFiles(
	fileSystem portablefiles.FileSystem,
	sourceDir string,
	targetDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) error {
	if factoryConfig == nil ||
		factoryConfig.ResourceManifest == nil ||
		len(factoryConfig.ResourceManifest.BundledFiles) == 0 {
		return nil
	}
	validationRoot, err := portablefiles.PrepareValidationRoot(fileSystem, targetDir)
	if err != nil {
		return err
	}
	for _, bundledFile := range factoryConfig.ResourceManifest.BundledFiles {
		if err := copySupportedFile(
			fileSystem,
			validationRoot,
			sourceDir,
			bundledFile,
		); err != nil {
			return err
		}
	}
	return nil
}

func copySupportedFile(
	fileSystem portablefiles.FileSystem,
	validationRoot portablefiles.ValidationRoot,
	sourceDir string,
	bundledFile factorydefinitions.BundledFileConfig,
) error {
	if err := factorydefinitions.ValidatePortableBundledFileType(
		bundledFile,
	); err != nil {
		return err
	}
	if err := factorydefinitions.ValidatePortableBundledFileTarget(
		bundledFile,
	); err != nil {
		return err
	}
	if !factorydefinitions.ShouldOmitSupportedPortableBundledInline(
		bundledFile,
	) || strings.TrimSpace(bundledFile.Content.Inline) != "" {
		return nil
	}
	sourcePath, ok := resolveSupportedSourcePath(fileSystem, sourceDir, bundledFile)
	if !ok {
		return nil
	}
	info, err := fileSystem.Stat(sourcePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf(
			"stat portable bundled file %s: %w",
			sourcePath,
			err,
		)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	target, err := portablefiles.ResolveTarget(
		validationRoot,
		bundledFile.TargetPath,
		portableBundledFactoryPrefix,
	)
	if err != nil {
		return fmt.Errorf(
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
		return fmt.Errorf(
			"resolve bundled file %q: %w",
			bundledFile.TargetPath,
			err,
		)
	}
	data, err := fileSystem.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf(
			"read portable bundled file %s: %w",
			sourcePath,
			err,
		)
	}
	if err := fileSystem.MkdirAll(filepath.Dir(target.Path()), 0o755); err != nil {
		return fmt.Errorf(
			"create bundled file directory for %s: %w",
			target.Path(),
			err,
		)
	}
	if err := portablefiles.WriteFile(
		fileSystem,
		target.Path(),
		data,
		bundledFileMode(bundledFile),
	); err != nil {
		return fmt.Errorf(
			"write bundled file %s: %w",
			target.Path(),
			err,
		)
	}
	return nil
}

func bundledFileMode(
	bundledFile factorydefinitions.BundledFileConfig,
) fs.FileMode {
	if bundledFile.Type == factorydefinitions.BundledFileTypeScript {
		return 0o755
	}
	return 0o644
}
