package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveFactoryRootFromConfigFile returns the factory layout root directory
// that contains the factory config file at path. The path must reference an
// existing regular file.
func ResolveFactoryRootFromConfigFile(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("factory config path is required")
	}

	resolved, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve factory config path %s: %w", trimmed, err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("factory config file not found: %s", trimmed)
		}
		return "", fmt.Errorf("find factory config file %s: %w", trimmed, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("factory config path must be a file: %s", trimmed)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("factory config path must be a file: %s", trimmed)
	}

	return filepath.Dir(resolved), nil
}
