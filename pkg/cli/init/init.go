// Package initcmd implements the agent-factory init command behavior.
package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// InitConfig holds parameters for the init command.
type InitConfig struct {
	Dir      string
	Type     string
	Executor string
}

const (
	factoryWorkersDirName      = "workers"
	factoryWorkstationsDirName = "workstations"
	factoryInputsDirName       = "inputs"
	factoryAgentsFileName      = "AGENTS.md"
	defaultProcessorSystemBody = "You are the processor. Complete the task."
)

type starterExecutor string

const (
	StarterExecutorCodex  starterExecutor = "codex"
	StarterExecutorClaude starterExecutor = "claude"

	// DefaultStarterExecutor is the executor/provider scaffolded when --executor is omitted.
	DefaultStarterExecutor = string(StarterExecutorCodex)
)

// initDirs defines the directory structure created by Init.
var initDirs = []string{
	factoryWorkersDirName,
	factoryWorkstationsDirName,
	factoryInputsDirName,
}

func SupportedStarterExecutors() []string {
	return []string{string(StarterExecutorCodex), string(StarterExecutorClaude)}
}

func parseStarterExecutor(raw string) (starterExecutor, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		value = DefaultStarterExecutor
	}

	executor := starterExecutor(value)
	switch executor {
	case StarterExecutorCodex, StarterExecutorClaude:
		return executor, nil
	default:
		return "", fmt.Errorf(
			"unsupported init executor %q: supported values are %s",
			raw,
			strings.Join(SupportedStarterExecutors(), ", "),
		)
	}
}

func defaultModelWorkerAgentsMD(executor starterExecutor) string {
	model := "gpt-5-codex"
	modelProvider := "CODEX"
	if executor == StarterExecutorClaude {
		model = "claude-sonnet-4-20250514"
		modelProvider = "CLAUDE"
	}

	return fmt.Sprintf(`---
type: MODEL_WORKER
model: %s
modelProvider: %s
executorProvider: SCRIPT_WRAP
resources:
  - name: agent-slot
    capacity: 1
timeout: 1h
skipPermissions: true
---
%s`, model, modelProvider, defaultProcessorSystemBody)
}

// Init creates the factory directory structure.
//
// Created files and directories:
//
//	<dir>/factory.json                — scaffold-specific workflow definition
//	<dir>/workers/                    — worker configuration files
//	<dir>/workstations/               — workstation configuration files
//	<dir>/inputs/                     — multi-channel input directory
//	<dir>/inputs/<work-type>/default/ — scaffold-specific preseed directory
//
// After running init, start the factory with:
//
//	agent-factory run --dir <dir>
//
// Submit work via the API (POST /work) or by placing files in the scaffold's
// default inputs/<work-type>/default/ directory.
func Init(cfg InitConfig) error {
	scaffoldType, scaffold, err := resolveScaffoldDefinition(cfg.Type)
	if err != nil {
		return err
	}

	if strings.TrimSpace(cfg.Executor) != "" || scaffoldType == DefaultScaffoldType {
		executor, err := parseStarterExecutor(cfg.Executor)
		if err != nil {
			return err
		}
		if scaffoldType == DefaultScaffoldType {
			scaffold.files[factoryWorkersDirName+"/processor/"+factoryAgentsFileName] = defaultModelWorkerAgentsMD(executor)
		}
	}

	for _, d := range initDirs {
		path := filepath.Join(cfg.Dir, d)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
	}

	for relativePath, contents := range scaffold.files {
		written, err := writeFileIfAbsent(filepath.Join(cfg.Dir, relativePath), contents)
		if err != nil {
			return err
		}
		if relativePath == interfaces.FactoryConfigFile && written {
			fmt.Printf("Created %s\n", filepath.Join(cfg.Dir, relativePath))
		}
	}

	factoryConfigPath := filepath.Join(cfg.Dir, interfaces.FactoryConfigFile)
	if _, err := os.Stat(factoryConfigPath); err != nil {
		return fmt.Errorf("stat %s: %w", factoryConfigPath, err)
	}

	defaultInputDir := filepath.Join(cfg.Dir, interfaces.InputsDir, scaffold.inputWorkType, interfaces.DefaultChannelName)
	if err := os.MkdirAll(defaultInputDir, 0o755); err != nil {
		return fmt.Errorf("create inputs/%s/default: %w", scaffold.inputWorkType, err)
	}

	fmt.Printf("Initialized %s factory directory structure at %s/\n", scaffoldType, cfg.Dir)
	fmt.Printf("  → Drop work files into %s/ to preseed on startup\n", defaultInputDir)
	return nil
}

func writeFileIfAbsent(path, contents string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}
