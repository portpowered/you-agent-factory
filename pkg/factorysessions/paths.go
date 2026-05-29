package factorysessions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AbsolutizeFactoryDirectory resolves and cleans a factory directory path.
func AbsolutizeFactoryDirectory(dir string) (string, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return "", fmt.Errorf("factory directory is required")
	}
	expanded, err := ExpandFolderHome(trimmed)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolve factory directory %q: %w", dir, err)
	}
	return filepath.Clean(resolved), nil
}

// ResolveSessionFolder validates and resolves a factory session folder path.
func ResolveSessionFolder(folderPath string) (string, error) {
	trimmed := strings.TrimSpace(folderPath)
	if trimmed == "" {
		return "", NewValidationError(
			validationReasonRequired,
			"folderPath",
			fmt.Errorf("factory session folder is required"),
		)
	}
	expanded, err := ExpandFolderHome(trimmed)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolve factory session folder %q: %w", folderPath, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		switch {
		case os.IsNotExist(err):
			return "", NewValidationError(
				validationReasonMissing,
				"folderPath",
				fmt.Errorf("stat factory session folder %q: %w", resolved, err),
			)
		case os.IsPermission(err):
			return "", NewValidationError(
				validationReasonUnreadable,
				"folderPath",
				fmt.Errorf("stat factory session folder %q: %w", resolved, err),
			)
		default:
			return "", fmt.Errorf("stat factory session folder %q: %w", resolved, err)
		}
	}
	if !info.IsDir() {
		return "", NewValidationError(
			validationReasonNotDirectory,
			"folderPath",
			fmt.Errorf("factory session folder %q must be a directory", resolved),
		)
	}
	return resolved, nil
}

// ExpandFolderHome expands a leading tilde in folder paths.
func ExpandFolderHome(path string) (string, error) {
	if path != "~" &&
		!strings.HasPrefix(path, "~/") &&
		!strings.HasPrefix(path, `~\`) {
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
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
