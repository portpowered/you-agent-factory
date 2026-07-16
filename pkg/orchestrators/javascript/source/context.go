package workflowsource

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// DefaultContext builds lookup roots from a project working directory.
func DefaultContext(projectRoot string) (Context, error) {
	projectRoot = filepath.Clean(strings.TrimSpace(projectRoot))
	if projectRoot == "" {
		return Context{}, fmt.Errorf("project root is required")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Context{}, fmt.Errorf("resolve user home for workflow lookup: %w", err)
	}

	globalFactoryRoot, err := factoryconfig.GlobalNamedFactoryRootForHome(homeDir)
	if err != nil {
		return Context{}, err
	}
	globalWorkflowRoot, err := factoryconfig.GlobalWorkflowRootForHome(homeDir)
	if err != nil {
		return Context{}, err
	}

	projectFactoryRoot, err := factoryconfig.DefaultProjectNamedFactoryRoot(projectRoot)
	if err != nil {
		return Context{}, err
	}

	packageRoot := filepath.Join(projectRoot, interfaces.FactoryDir)
	return Context{
		ProjectRoot:         projectRoot,
		PackageRoot:         packageRoot,
		ProjectWorkflowRoot: filepath.Join(projectRoot, ProjectClaudeWorkflowsDir),
		GlobalWorkflowRoot:  globalWorkflowRoot,
		ProjectFactoryRoot:  projectFactoryRoot,
		GlobalFactoryRoot:   globalFactoryRoot,
	}, nil
}
