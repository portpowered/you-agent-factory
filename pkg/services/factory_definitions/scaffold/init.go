// Package initcmd implements the agent-factory init command behavior.
package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
)

type InitConfig = factorydefinitions.ScaffoldConfig
type InitResult = factorydefinitions.ScaffoldResult

// Initializer owns Factory scaffold policy while delegating external effects
// to exact Factory Definitions-root ports selected by Wire.
type Initializer struct {
	files         factorydefinitions.ScaffoldFileSystem
	defaultOutput factorydefinitions.ScaffoldOutput
}

// New constructs the scaffold operation from explicit external effects.
func New(
	files factorydefinitions.ScaffoldFileSystem,
	defaultOutput factorydefinitions.ScaffoldOutput,
) (*Initializer, error) {
	if files == nil {
		return nil, fmt.Errorf("Factory Definition scaffold filesystem is required")
	}
	if defaultOutput == nil {
		return nil, fmt.Errorf("Factory Definition scaffold output is required")
	}
	return &Initializer{files: files, defaultOutput: defaultOutput}, nil
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
	DefaultStarterExecutor = factorydefinitions.DefaultStarterExecutor
)

// initDirs defines the directory structure created by Init.
var initDirs = []string{
	factoryWorkersDirName,
	factoryWorkstationsDirName,
	factoryInputsDirName,
}

func SupportedStarterExecutors() []string {
	return factorydefinitions.SupportedStarterExecutors()
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
	modelProvider := "CODEX"
	if executor == StarterExecutorClaude {
		modelProvider = "CLAUDE"
	}

	return fmt.Sprintf(`---
type: MODEL_WORKER
modelProvider: %s
executorProvider: SCRIPT_WRAP
timeout: 1h
skipPermissions: true
resources:
  - name: agent-slot
    capacity: 1
---
%s`, modelProvider, defaultProcessorSystemBody)
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
func (initializer *Initializer) Init(cfg InitConfig) error {
	if initializer == nil || initializer.files == nil {
		return fmt.Errorf("Factory Definition scaffold filesystem is required")
	}
	if initializer.defaultOutput == nil {
		return fmt.Errorf("Factory Definition scaffold output is required")
	}
	if cfg.Output == nil {
		cfg.Output = initializer.defaultOutput
	}

	scaffoldType, scaffold, err := resolveScaffoldDefinition(cfg.Type)
	if err != nil {
		return err
	}
	dirAlreadyExisted := initializer.pathExists(cfg.Dir)
	logInitRequest(cfg, scaffoldType, dirAlreadyExisted)

	if err := applyStarterExecutor(cfg, scaffoldType, &scaffold); err != nil {
		return err
	}

	writtenByCategory, defaultInputDir, err := initializer.materializeScaffold(cfg, scaffold)
	if err != nil {
		return err
	}
	return emitInitResult(cfg, scaffoldType, scaffold, writtenByCategory, dirAlreadyExisted, defaultInputDir)
}

func (initializer *Initializer) writeFileIfAbsent(path, contents string) (bool, error) {
	if _, err := initializer.files.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}

	if err := initializer.files.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := initializer.files.WriteFile(path, []byte(contents), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

func (initializer *Initializer) pathExists(path string) bool {
	_, err := initializer.files.Stat(path)
	return err == nil
}

func effectiveExecutorLabel(raw string, scaffoldType ScaffoldType) string {
	if scaffoldType != DefaultScaffoldType {
		return "not-applicable"
	}
	executor, err := parseStarterExecutor(raw)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return string(executor)
}

func initPathCategory(relativePath string) string {
	switch {
	case relativePath == interfaces.FactoryConfigFile:
		return "factory-config"
	case strings.HasPrefix(relativePath, factoryWorkersDirName+"/"):
		return "worker"
	case strings.HasPrefix(relativePath, factoryWorkstationsDirName+"/"):
		return "workstation"
	case strings.HasPrefix(relativePath, factoryInputsDirName+"/"):
		return "input"
	default:
		return "docs"
	}
}
