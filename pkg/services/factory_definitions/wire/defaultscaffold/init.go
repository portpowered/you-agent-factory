package defaultscaffold

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	scaffoldWorkersDir      = "workers"
	scaffoldWorkstationsDir = "workstations"
	scaffoldInputsDir       = "inputs"
	scaffoldAgentsFile      = "AGENTS.md"
)

//go:embed default_factory.json
var defaultFactoryJSON string

// NewScaffoldInitializer constructs the single supported default Factory
// scaffold operation from exact filesystem and output effects.
func NewScaffoldInitializer(
	files factorydefinitions.ScaffoldFileSystem,
	output factorydefinitions.ScaffoldOutput,
) (factorydefinitions.ScaffoldInitializer, error) {
	if files == nil {
		return nil, fmt.Errorf("Factory Definition scaffold filesystem is required")
	}
	if output == nil {
		return nil, fmt.Errorf("Factory Definition scaffold output is required")
	}
	return func(config factorydefinitions.ScaffoldConfig) error {
		return materializeDefaultScaffold(files, output, config.Dir)
	}, nil
}

func materializeDefaultScaffold(
	files factorydefinitions.ScaffoldFileSystem,
	output factorydefinitions.ScaffoldOutput,
	dir string,
) error {
	for _, relativeDir := range []string{
		scaffoldWorkersDir,
		scaffoldWorkstationsDir,
		scaffoldInputsDir,
	} {
		path := filepath.Join(dir, relativeDir)
		if err := files.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
	}
	for relativePath, contents := range defaultScaffoldFiles() {
		if err := writeScaffoldFileIfAbsent(files, filepath.Join(dir, relativePath), contents); err != nil {
			return err
		}
	}

	factoryConfigPath := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
	if _, err := files.Stat(factoryConfigPath); err != nil {
		return fmt.Errorf("stat %s: %w", factoryConfigPath, err)
	}
	defaultInputDir := filepath.Join(
		dir,
		factorydefinitions.InputsDir,
		factorydefinitions.DefaultFactoryInputType,
		factorydefinitions.DefaultChannelName,
	)
	if err := files.MkdirAll(defaultInputDir, 0o755); err != nil {
		return fmt.Errorf("create inputs/%s/default: %w", factorydefinitions.DefaultFactoryInputType, err)
	}
	if _, err := fmt.Fprintf(output, "Initialized default factory directory structure at %s/\n", dir); err != nil {
		return err
	}
	_, err := fmt.Fprintf(output, "  → Drop work files into %s/ to preseed on startup\n", defaultInputDir)
	return err
}

func writeScaffoldFileIfAbsent(
	files factorydefinitions.ScaffoldFileSystem,
	path string,
	contents string,
) error {
	if _, err := files.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := files.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := files.WriteFile(path, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func defaultScaffoldFiles() map[string]string {
	return map[string]string{
		factorydefinitions.FactoryConfigFile: defaultFactoryJSON,
		scaffoldWorkersDir + "/README.md": `# Workers

Worker configuration files go here.
Each subdirectory contains an AGENTS.md defining a worker type with its execution settings.
`,
		scaffoldWorkersDir + "/processor/" + scaffoldAgentsFile: `---
type: MODEL_WORKER
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
timeout: 1h
skipPermissions: true
resources:
  - name: agent-slot
    capacity: 1
---
You are the processor. Complete the task.`,
		scaffoldWorkstationsDir + "/README.md": `# Workstations

Workstation configuration files go here.
Each subdirectory contains an AGENTS.md defining the workstation prompt template.
`,
		scaffoldWorkstationsDir + "/process/" + scaffoldAgentsFile: `---
type: MODEL_WORKSTATION
---

You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.

The customer has asked you to perform the following request:

{{ (index .Inputs 0).Payload }}
`,
		scaffoldInputsDir + "/README.md": `# Inputs

Use the default starter inbox for local task submissions:
  inputs/task/default/                 - Markdown or JSON task submissions

Seed your starter work by adding files to this inbox, then run the starter to process them.
The file watcher monitors this directory tree and automatically watches new subdirectories.
`,
	}
}
