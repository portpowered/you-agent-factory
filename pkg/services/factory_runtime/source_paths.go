package factory

import (
	"fmt"
	"path/filepath"
	"strings"
)

const globalWorkflowDirectoryName = "workflows"

// GlobalWorkflowRootForHome returns the Factory Runtime-owned global workflow
// source root for an explicitly resolved home directory.
func GlobalWorkflowRootForHome(homeDir string) (string, error) {
	trimmed := strings.TrimSpace(homeDir)
	if trimmed == "" {
		return "", fmt.Errorf("user home directory is required")
	}
	return filepath.Join(trimmed, ".you-agent-factory", globalWorkflowDirectoryName), nil
}
