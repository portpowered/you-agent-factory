package workflowsource

import (
	"fmt"
	"path/filepath"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// DefaultContext builds lookup roots from a project working directory.
func DefaultContext(
	projectRoot string,
	homeDir string,
	files fileSystem,
	resolveSymlinks func(string) (string, error),
) (Context, error) {
	projectRoot = filepath.Clean(strings.TrimSpace(projectRoot))
	if projectRoot == "" {
		return Context{}, fmt.Errorf("project root is required")
	}

	homeDir = filepath.Clean(strings.TrimSpace(homeDir))
	if homeDir == "" {
		return Context{}, fmt.Errorf("workflow source home is required")
	}
	if files == nil {
		return Context{}, fmt.Errorf("workflow source filesystem is required")
	}
	if resolveSymlinks == nil {
		return Context{}, fmt.Errorf("workflow source symlink resolver is required")
	}

	packageRoot := filepath.Join(projectRoot, interfaces.FactoryDir)
	return Context{
		ProjectRoot:         projectRoot,
		PackageRoot:         packageRoot,
		ProjectWorkflowRoot: filepath.Join(projectRoot, ProjectClaudeWorkflowsDir),
		GlobalWorkflowRoot:  filepath.Join(homeDir, ".you-agent-factory", GlobalWorkflowsDirName),
		ProjectFactoryRoot:  filepath.Join(projectRoot, interfaces.FactoryDir),
		GlobalFactoryRoot:   interfaces.NamedFactoriesRoot(homeDir),
		files:               files,
		resolveSymlinks:     resolveSymlinks,
	}, nil
}
