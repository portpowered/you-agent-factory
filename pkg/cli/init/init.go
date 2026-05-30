// Package initcmd implements the agent-factory init command behavior.
package initcmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// InitConfig holds parameters for the init command.
type InitConfig struct {
	Dir         string
	Type        string
	Executor    string
	JSON        bool
	Output      io.Writer
	Verbose     bool
	Debug       bool
	Diagnostics io.Writer
}

// InitResult reports a successful init scaffold write.
type InitResult struct {
	ScaffoldType string `json:"scaffoldType"`
	TargetDir    string `json:"targetDir"`
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
func Init(cfg InitConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	scaffoldType, scaffold, err := resolveScaffoldDefinition(cfg.Type)
	if err != nil {
		return err
	}
	dirAlreadyExisted := pathExists(cfg.Dir)
	logInitRequest(cfg, scaffoldType, dirAlreadyExisted)

	if err := applyStarterExecutor(cfg, scaffoldType, &scaffold); err != nil {
		return err
	}

	writtenByCategory, defaultInputDir, err := materializeScaffold(cfg, scaffold)
	if err != nil {
		return err
	}
	return emitInitResult(cfg, scaffoldType, scaffold, writtenByCategory, dirAlreadyExisted, defaultInputDir)
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

func pathExists(path string) bool {
	_, err := os.Stat(path)
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
