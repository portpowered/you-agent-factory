package config

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const portableFactoryDirName = "factory"

var portableBundledDirectoryNames = []string{"scripts", "docs"}

var portableBundledRootHelperFiles = []string{"Makefile"}

var portableBundledFactoryRootHelperFiles = []string{"portable-dependencies.json"}

func applySupportedPortableBundledFiles(factoryDir string, cfg *interfaces.FactoryConfig) error {
	if cfg == nil {
		return nil
	}

	collected, err := collectSupportedPortableBundledFiles(factoryDir)
	if err != nil {
		return err
	}
	if len(collected) == 0 {
		return nil
	}

	if cfg.ResourceManifest == nil {
		cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{}
	}
	cfg.ResourceManifest.BundledFiles = mergePortableBundledFiles(cfg.ResourceManifest.BundledFiles, collected)
	return nil
}

func collectSupportedPortableBundledFiles(factoryDir string) ([]interfaces.BundledFileConfig, error) {
	layout, ok := portableBundledLayoutForFactoryDir(factoryDir)
	if !ok {
		return nil, nil
	}

	bundledFiles := make([]interfaces.BundledFileConfig, 0)
	for _, dirName := range portableBundledDirectoryNames {
		rootDir := filepath.Join(layout.factoryDir, dirName)
		targetRoot := filepath.ToSlash(filepath.Join(layout.factoryPrefix, dirName))
		fileType := interfaces.BundledFileTypeDoc
		if dirName == "scripts" {
			fileType = interfaces.BundledFileTypeScript
		}
		collected, err := collectPortableBundledFilesFromDir(rootDir, targetRoot, fileType)
		if err != nil {
			return nil, err
		}
		bundledFiles = append(bundledFiles, collected...)
	}

	for _, helperName := range portableBundledRootHelperFiles {
		bundledFile, ok, err := collectPortableBundledRootHelperFile(filepath.Join(layout.projectRoot, helperName), helperName)
		if err != nil {
			return nil, err
		}
		if ok {
			bundledFiles = append(bundledFiles, bundledFile)
		}
	}
	for _, helperName := range portableBundledFactoryRootHelperFiles {
		targetPath := filepath.ToSlash(filepath.Join(layout.factoryPrefix, helperName))
		bundledFile, ok, err := collectPortableBundledRootHelperFile(filepath.Join(layout.factoryDir, helperName), targetPath)
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

type portableBundledLayout struct {
	projectRoot   string
	factoryDir    string
	factoryPrefix string
}

func portableBundledLayoutForFactoryDir(factoryDir string) (portableBundledLayout, bool) {
	cleanFactoryDir := filepath.Clean(factoryDir)
	if filepath.Base(cleanFactoryDir) != portableFactoryDirName {
		return portableBundledLayout{}, false
	}
	return portableBundledLayout{
		projectRoot:   filepath.Dir(cleanFactoryDir),
		factoryDir:    cleanFactoryDir,
		factoryPrefix: portableFactoryDirName,
	}, true
}

func collectPortableBundledFilesFromDir(sourceDir, targetRoot, fileType string) ([]interfaces.BundledFileConfig, error) {
	info, err := os.Stat(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat portable bundled directory %s: %w", sourceDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("portable bundled directory %s must be a directory", sourceDir)
	}

	bundledFiles := make([]interfaces.BundledFileConfig, 0)
	if err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}

		relativePath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("resolve bundled file path %s: %w", path, err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read bundled file %s: %w", path, err)
		}
		bundledFiles = append(bundledFiles, interfaces.BundledFileConfig{
			Type:       fileType,
			TargetPath: filepath.ToSlash(filepath.Join(targetRoot, relativePath)),
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   string(content),
			},
		})
		return nil
	}); err != nil {
		return nil, fmt.Errorf("collect portable bundled files from %s: %w", sourceDir, err)
	}
	return bundledFiles, nil
}

func collectPortableBundledRootHelperFile(sourcePath, targetPath string) (interfaces.BundledFileConfig, bool, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return interfaces.BundledFileConfig{}, false, nil
		}
		return interfaces.BundledFileConfig{}, false, fmt.Errorf("stat portable bundled helper file %s: %w", sourcePath, err)
	}
	if !info.Mode().IsRegular() {
		return interfaces.BundledFileConfig{}, false, nil
	}

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return interfaces.BundledFileConfig{}, false, fmt.Errorf("read portable bundled helper file %s: %w", sourcePath, err)
	}
	return interfaces.BundledFileConfig{
		Type:       interfaces.BundledFileTypeRootHelper,
		TargetPath: filepath.ToSlash(targetPath),
		Content: interfaces.BundledFileContentConfig{
			Encoding: interfaces.BundledFileEncodingUTF8,
			Inline:   string(content),
		},
	}, true, nil
}

func mergePortableBundledFiles(existing, collected []interfaces.BundledFileConfig) []interfaces.BundledFileConfig {
	byTarget := make(map[string]interfaces.BundledFileConfig, len(existing)+len(collected))
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

	merged := make([]interfaces.BundledFileConfig, 0, len(targets))
	for _, target := range targets {
		merged = append(merged, byTarget[target])
	}
	return merged
}

func materializePortableBundledFiles(targetDir string, cfg *interfaces.FactoryConfig) error {
	resolvedWrites, err := preparePortableBundledFileWrites(targetDir, cfg)
	if err != nil {
		return err
	}

	for _, write := range resolvedWrites {
		if err := os.MkdirAll(filepath.Dir(write.targetPath), 0o755); err != nil {
			return fmt.Errorf("create bundled file directory for %s: %w", write.targetPath, err)
		}
		if err := os.WriteFile(write.targetPath, []byte(write.content), write.mode); err != nil {
			return fmt.Errorf("write bundled file %s: %w", write.targetPath, err)
		}
	}
	return nil
}

func preparePortableBundledFileWrites(targetDir string, cfg *interfaces.FactoryConfig) ([]portableBundledFileWrite, error) {
	if cfg == nil || cfg.ResourceManifest == nil || len(cfg.ResourceManifest.BundledFiles) == 0 {
		return nil, nil
	}

	validationRoot, err := preparePortableBundledValidationRoot(targetDir)
	if err != nil {
		return nil, err
	}

	bundledFiles := append([]interfaces.BundledFileConfig(nil), cfg.ResourceManifest.BundledFiles...)
	sort.Slice(bundledFiles, func(i, j int) bool {
		return bundledFiles[i].TargetPath < bundledFiles[j].TargetPath
	})

	resolvedWrites := make([]portableBundledFileWrite, 0, len(bundledFiles))
	for _, bundledFile := range bundledFiles {
		targetPath, err := portableBundledTargetPath(validationRoot.targetDir, bundledFile.TargetPath)
		if err != nil {
			return nil, fmt.Errorf("resolve bundled file %q: %w", bundledFile.TargetPath, err)
		}
		if err := validatePortableBundledFilesystemPath(validationRoot, bundledFile.TargetPath, targetPath); err != nil {
			return nil, fmt.Errorf("resolve bundled file %q: %w", bundledFile.TargetPath, err)
		}
		resolvedWrites = append(resolvedWrites, portableBundledFileWrite{
			targetPath: targetPath,
			content:    bundledFile.Content.Inline,
			mode:       portableBundledFileMode(bundledFile),
		})
	}
	return resolvedWrites, nil
}

type portableBundledFileWrite struct {
	targetPath string
	content    string
	mode       fs.FileMode
}

func portableBundledFileMode(bundledFile interfaces.BundledFileConfig) fs.FileMode {
	if bundledFile.Type == interfaces.BundledFileTypeScript {
		return 0o755
	}
	return 0o644
}

type portableBundledValidationRoot struct {
	targetDir    string
	resolvedRoot string
}

func portableBundledTargetPath(targetDir, targetLocation string) (string, error) {
	trimmed := strings.TrimSpace(targetLocation)
	if trimmed == "" {
		return "", fmt.Errorf("target location is required")
	}

	normalized := strings.ReplaceAll(trimmed, `\`, "/")
	cleaned := path.Clean(normalized)
	if cleaned == "" || cleaned == "." {
		return "", fmt.Errorf("target location is required")
	}
	if strings.HasPrefix(normalized, "/") || strings.HasPrefix(normalized, `\`) || filepath.IsAbs(trimmed) || filepath.VolumeName(trimmed) != "" {
		return "", fmt.Errorf("target location %q must be relative to the expand target", targetLocation)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("target location %q cannot escape the expand target", targetLocation)
	}

	materializedPath := cleaned
	if strings.HasPrefix(materializedPath, portableFactoryDirName+"/") {
		materializedPath = strings.TrimPrefix(materializedPath, portableFactoryDirName+"/")
	}

	targetPath := filepath.Join(targetDir, filepath.FromSlash(materializedPath))
	relativePath, err := filepath.Rel(targetDir, targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve bundled file path for %q: %w", targetLocation, err)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("target location %q cannot escape the expand target", targetLocation)
	}
	return targetPath, nil
}

func preparePortableBundledValidationRoot(targetDir string) (portableBundledValidationRoot, error) {
	cleanTargetDir, err := filepath.Abs(filepath.Clean(targetDir))
	if err != nil {
		return portableBundledValidationRoot{}, fmt.Errorf("resolve expand target %s: %w", targetDir, err)
	}

	resolvedRoot := cleanTargetDir
	if _, err := os.Stat(cleanTargetDir); err == nil {
		resolvedRoot, err = filepath.EvalSymlinks(cleanTargetDir)
		if err != nil {
			return portableBundledValidationRoot{}, fmt.Errorf("resolve expand target %s: %w", cleanTargetDir, err)
		}
	} else if !os.IsNotExist(err) {
		return portableBundledValidationRoot{}, fmt.Errorf("stat expand target %s: %w", cleanTargetDir, err)
	}

	return portableBundledValidationRoot{
		targetDir:    cleanTargetDir,
		resolvedRoot: resolvedRoot,
	}, nil
}

func validatePortableBundledFilesystemPath(root portableBundledValidationRoot, targetLocation, targetPath string) error {
	relativePath, err := filepath.Rel(root.targetDir, targetPath)
	if err != nil {
		return fmt.Errorf("resolve bundled file path for %q: %w", targetLocation, err)
	}

	currentPath := root.targetDir
	for _, segment := range strings.Split(relativePath, string(filepath.Separator)) {
		if segment == "" || segment == "." {
			continue
		}
		currentPath = filepath.Join(currentPath, segment)
		info, err := os.Lstat(currentPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("inspect bundled file path %q: %w", targetLocation, err)
		}
		resolvedPath, isLink, err := portableBundledResolvedLinkPath(currentPath, info)
		if err != nil {
			return fmt.Errorf("resolve filesystem link for %q: %w", targetLocation, err)
		}
		if !isLink {
			continue
		}
		if !portableBundledPathWithinRoot(root.resolvedRoot, resolvedPath) {
			return fmt.Errorf("target location %q cannot escape the expand target through filesystem links", targetLocation)
		}
	}

	return nil
}

func portableBundledPathWithinRoot(rootPath, candidatePath string) bool {
	relativePath, err := filepath.Rel(rootPath, candidatePath)
	if err != nil {
		return false
	}
	return relativePath != ".." &&
		!strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relativePath)
}

func portableBundledResolvedLinkPath(path string, info os.FileInfo) (string, bool, error) {
	if info.Mode()&os.ModeSymlink == 0 {
		linkTarget, err := os.Readlink(path)
		if err != nil {
			return "", false, nil
		}
		return resolvePortableBundledLinkTarget(path, linkTarget)
	}

	linkTarget, err := os.Readlink(path)
	if err != nil {
		return "", false, err
	}
	return resolvePortableBundledLinkTarget(path, linkTarget)
}

func resolvePortableBundledLinkTarget(path, linkTarget string) (string, bool, error) {
	resolvedPath := linkTarget
	if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(filepath.Dir(path), resolvedPath)
	}
	resolvedPath, err := filepath.Abs(resolvedPath)
	if err != nil {
		return "", false, err
	}
	if evalPath, err := filepath.EvalSymlinks(resolvedPath); err == nil {
		resolvedPath = evalPath
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	return resolvedPath, true, nil
}
