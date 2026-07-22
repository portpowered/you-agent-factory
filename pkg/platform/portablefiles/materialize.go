// Package portablefiles provides contract-free filesystem primitives for
// safely materializing portable files below a caller-owned directory.
package portablefiles

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

// ValidationRoot is an absolute materialization root and its symlink-resolved
// equivalent. Callers should create it once and reuse it for related writes.
type ValidationRoot struct {
	targetDir    string
	resolvedRoot string
}

// Target is a relative location resolved beneath a ValidationRoot.
type Target struct {
	path     string
	segments []string
}

// FileSystem is the complete host-filesystem effect used by portable-file
// discovery, containment validation, materialization, and pruning. Domain
// policy remains with the caller; this contract exposes mechanics only.
type FileSystem interface {
	Stat(string) (fs.FileInfo, error)
	Lstat(string) (fs.FileInfo, error)
	Readlink(string) (string, error)
	EvalSymlinks(string) (string, error)
	WalkDir(string, fs.WalkDirFunc) error
	ReadFile(string) ([]byte, error)
	WriteFile(string, []byte, fs.FileMode) error
	Chmod(string, fs.FileMode) error
	MkdirAll(string, fs.FileMode) error
	Remove(string) error
}

// PrepareValidationRoot resolves targetDir for containment checks.
func PrepareValidationRoot(fileSystem FileSystem, targetDir string) (ValidationRoot, error) {
	if fileSystem == nil {
		return ValidationRoot{}, fmt.Errorf("portable filesystem is required")
	}
	cleanTargetDir, err := filepath.Abs(filepath.Clean(targetDir))
	if err != nil {
		return ValidationRoot{}, fmt.Errorf("resolve materialization target %s: %w", targetDir, err)
	}

	resolvedRoot := cleanTargetDir
	if _, err := fileSystem.Stat(cleanTargetDir); err == nil {
		resolvedRoot, err = fileSystem.EvalSymlinks(cleanTargetDir)
		if err != nil {
			return ValidationRoot{}, fmt.Errorf("resolve materialization target %s: %w", cleanTargetDir, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return ValidationRoot{}, fmt.Errorf("stat materialization target %s: %w", cleanTargetDir, err)
	}

	return ValidationRoot{targetDir: cleanTargetDir, resolvedRoot: resolvedRoot}, nil
}

// TargetDir returns the absolute directory beneath which files are resolved.
func (r ValidationRoot) TargetDir() string {
	return r.targetDir
}

// ResolveTarget validates a portable relative location and resolves it below
// root. stripPrefix may remove a domain container such as "factory/" before
// the location is mapped onto disk.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func ResolveTarget(root ValidationRoot, targetLocation, stripPrefix string) (Target, error) {
	trimmed := strings.TrimSpace(targetLocation)
	if trimmed == "" {
		return Target{}, fmt.Errorf("target location is required")
	}

	normalized := strings.ReplaceAll(trimmed, `\`, "/")
	cleaned := path.Clean(normalized)
	if cleaned == "" || cleaned == "." {
		return Target{}, fmt.Errorf("target location is required")
	}
	if strings.HasPrefix(normalized, "/") || strings.HasPrefix(normalized, `\`) ||
		filepath.IsAbs(trimmed) || filepath.VolumeName(trimmed) != "" {
		return Target{}, fmt.Errorf("target location %q must be relative to the materialization target", targetLocation)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return Target{}, fmt.Errorf("target location %q cannot escape the materialization target", targetLocation)
	}

	materializedPath := cleaned
	normalizedPrefix := strings.Trim(strings.ReplaceAll(stripPrefix, `\`, "/"), "/")
	if normalizedPrefix != "" && strings.HasPrefix(materializedPath, normalizedPrefix+"/") {
		materializedPath = strings.TrimPrefix(materializedPath, normalizedPrefix+"/")
	}

	filesystemPath := filepath.FromSlash(materializedPath)
	targetPath := filepath.Join(root.targetDir, filesystemPath)
	relativePath, err := filepath.Rel(root.targetDir, targetPath)
	if err != nil {
		return Target{}, fmt.Errorf("resolve materialized file path for %q: %w", targetLocation, err)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relativePath) {
		return Target{}, fmt.Errorf("target location %q cannot escape the materialization target", targetLocation)
	}
	return Target{path: targetPath, segments: strings.Split(filesystemPath, string(filepath.Separator))}, nil
}

// Path returns the resolved filesystem path.
func (t Target) Path() string {
	return t.path
}

// ValidateFilesystemPath rejects an existing symlink in target's path when it
// resolves outside root.
func ValidateFilesystemPath(
	fileSystem FileSystem,
	root ValidationRoot,
	targetLocation string,
	target Target,
) error {
	if fileSystem == nil {
		return fmt.Errorf("portable filesystem is required")
	}
	currentPath := root.targetDir
	for _, segment := range target.segments {
		if segment == "" || segment == "." {
			continue
		}
		currentPath = filepath.Join(currentPath, segment)
		info, err := fileSystem.Lstat(currentPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("inspect materialized file path %q: %w", targetLocation, err)
		}
		resolvedPath, isLink, err := resolvedLinkPath(fileSystem, currentPath, info)
		if err != nil {
			return fmt.Errorf("resolve filesystem link for %q: %w", targetLocation, err)
		}
		if isLink && !pathWithinRoot(root.resolvedRoot, resolvedPath) {
			return fmt.Errorf("target location %q cannot escape the materialization target through filesystem links", targetLocation)
		}
	}
	return nil
}

// WriteFile writes data and normalizes the resulting file mode.
func WriteFile(fileSystem FileSystem, filePath string, data []byte, mode fs.FileMode) error {
	if fileSystem == nil {
		return fmt.Errorf("portable filesystem is required")
	}
	if err := fileSystem.WriteFile(filePath, data, mode); err != nil {
		return err
	}
	return fileSystem.Chmod(filePath, mode)
}

// ReplacementNeeded reports whether writing data would replace different
// existing content. A new file is not a replacement.
func ReplacementNeeded(fileSystem FileSystem, filePath string, data []byte) (bool, error) {
	if fileSystem == nil {
		return false, fmt.Errorf("portable filesystem is required")
	}
	current, err := fileSystem.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return !bytes.Equal(current, data), nil
}

func pathWithinRoot(rootPath, candidatePath string) bool {
	relativePath, err := filepath.Rel(rootPath, candidatePath)
	if err != nil {
		return false
	}
	return relativePath != ".." &&
		!strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relativePath)
}

func resolvedLinkPath(
	fileSystem FileSystem,
	filePath string,
	info fs.FileInfo,
) (string, bool, error) {
	if info.Mode()&fs.ModeSymlink == 0 {
		return "", false, nil
	}
	linkTarget, err := fileSystem.Readlink(filePath)
	if err != nil {
		return "", false, err
	}
	resolvedPath := linkTarget
	if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(filepath.Dir(filePath), resolvedPath)
	}
	resolvedPath, err = filepath.Abs(resolvedPath)
	if err != nil {
		return "", false, err
	}
	if evalPath, evalErr := fileSystem.EvalSymlinks(resolvedPath); evalErr == nil {
		resolvedPath = evalPath
	} else if !errors.Is(evalErr, fs.ErrNotExist) {
		return "", false, evalErr
	}
	return resolvedPath, true, nil
}
