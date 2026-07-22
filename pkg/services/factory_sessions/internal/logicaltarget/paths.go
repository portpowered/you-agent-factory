package logicaltarget

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionvalidation"
)

// AbsolutizeFactoryDirectory resolves and cleans a factory directory path.
func AbsolutizeFactoryDirectory(dir string, resolveHome factorysessions.HomeDirectoryResolver) (string, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return "", fmt.Errorf("factory directory is required")
	}
	expanded, err := ExpandFolderHome(trimmed, resolveHome)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolve factory directory %q: %w", dir, err)
	}
	return filepath.Clean(resolved), nil
}

// ResolveSessionFolder validates and resolves a Factory Session folder path.
func ResolveSessionFolder(folderPath string, resolveHome factorysessions.HomeDirectoryResolver, directories factorysessions.DirectoryInspection) (string, error) {
	trimmed := strings.TrimSpace(folderPath)
	if trimmed == "" {
		return "", sessionvalidation.New(
			factorysessions.ValidationReasonRequired,
			"folderPath",
			fmt.Errorf("factory session folder is required"),
		)
	}
	if directories == nil {
		return "", fmt.Errorf("factory session directory inspection is required")
	}
	expanded, err := ExpandFolderHome(trimmed, resolveHome)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolve factory session folder %q: %w", folderPath, err)
	}
	info, err := directories.Stat(resolved)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return "", sessionvalidation.New(
				factorysessions.ValidationReasonMissing,
				"folderPath",
				fmt.Errorf("stat factory session folder %q: %w", resolved, err),
			)
		case errors.Is(err, fs.ErrPermission):
			return "", sessionvalidation.New(
				factorysessions.ValidationReasonUnreadable,
				"folderPath",
				fmt.Errorf("stat factory session folder %q: %w", resolved, err),
			)
		default:
			return "", fmt.Errorf("stat factory session folder %q: %w", resolved, err)
		}
	}
	if !info.IsDir() {
		return "", sessionvalidation.New(
			factorysessions.ValidationReasonNotDirectory,
			"folderPath",
			fmt.Errorf("factory session folder %q must be a directory", resolved),
		)
	}
	return resolved, nil
}

// ExpandFolderHome expands a leading tilde in folder paths.
func ExpandFolderHome(path string, resolveHome factorysessions.HomeDirectoryResolver) (string, error) {
	if resolveHome == nil {
		return "", fmt.Errorf("Factory Session home-directory resolver is required")
	}
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path, nil
	}

	homeDir, err := resolveHome()
	if err != nil {
		return "", fmt.Errorf("resolve user home for factory session folder %q: %w", path, err)
	}
	if path == "~" {
		return homeDir, nil
	}
	return filepath.Join(homeDir, path[2:]), nil
}

// SameFactoryDir reports whether two factory directory paths refer to the same location.
func SameFactoryDir(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

// SessionFactoryRootDir resolves the editable-definition root for a live session.
func SessionFactoryRootDir(serviceRootDir string, session *factorysessions.LiveSession) string {
	if session == nil {
		return ""
	}
	rootDir := session.FolderPath
	if session.FolderPath == "" || session.FactoryDir == "" || !SameFactoryDir(session.FactoryDir, session.FolderPath) {
		return rootDir
	}
	serviceRoot := filepath.Clean(serviceRootDir)
	if serviceRoot != "" && filepath.Dir(session.FactoryDir) == serviceRoot {
		return serviceRoot
	}
	return rootDir
}

// SessionFactoryPersistRoot resolves the on-disk definition persistence root.
func SessionFactoryPersistRoot(serviceRootDir string, session *factorysessions.LiveSession) string {
	if session != nil && !session.IsDefault && strings.TrimSpace(session.FolderPath) != "" {
		return session.FolderPath
	}
	return SessionFactoryRootDir(serviceRootDir, session)
}

// ValidateInitNewFactoryNestedDir rejects initialization over conflicting content.
func ValidateInitNewFactoryNestedDir(resolvedFolder string, directories factorysessions.DirectoryInspection) error {
	if directories == nil {
		return newValidationError(
			factorysessions.ValidationReasonUnreadable,
			"folderPath",
			fmt.Errorf("inspect nested factory directory: directory inspection is required"),
		)
	}
	nestedFactoryDir := filepath.Join(resolvedFolder, factorydefinitions.FactoryDir)
	info, err := directories.Stat(nestedFactoryDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return newValidationError(
			factorysessions.ValidationReasonUnreadable,
			"folderPath",
			fmt.Errorf("inspect nested factory directory %s: %w", nestedFactoryDir, err),
		)
	}
	if !info.IsDir() {
		return newValidationError(
			factorysessions.ValidationReasonConflict,
			"folderPath",
			fmt.Errorf("cannot initialize factory scaffold: %q exists and is not a directory", nestedFactoryDir),
		)
	}
	entries, err := directories.ReadDir(nestedFactoryDir)
	if err != nil {
		return newValidationError(
			factorysessions.ValidationReasonUnreadable,
			"folderPath",
			fmt.Errorf("read nested factory directory %s: %w", nestedFactoryDir, err),
		)
	}
	if len(entries) > 0 {
		return newValidationError(
			factorysessions.ValidationReasonConflict,
			"folderPath",
			fmt.Errorf("cannot initialize factory scaffold: %q already exists with conflicting content", nestedFactoryDir),
		)
	}
	return nil
}
