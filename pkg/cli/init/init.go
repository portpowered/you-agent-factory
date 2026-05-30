// Package initcmd implements the agent-factory init command behavior.
package initcmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// InitConfig holds parameters for the init command.
type InitConfig struct {
	Dir         string
	Type        string
	Executor    string
	Verbose     bool
	Debug       bool
	Diagnostics io.Writer
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
	scaffoldType, scaffold, err := resolveScaffoldDefinition(cfg.Type)
	if err != nil {
		return err
	}
	dirAlreadyExisted := pathExists(cfg.Dir)
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"init request targetDir=%s scaffoldType=%s executor=%s directoryExisted=%t",
		cfg.Dir,
		scaffoldType,
		effectiveExecutorLabel(cfg.Executor, scaffoldType),
		dirAlreadyExisted,
	)

	if strings.TrimSpace(cfg.Executor) != "" || scaffoldType == DefaultScaffoldType {
		executor, err := parseStarterExecutor(cfg.Executor)
		if err != nil {
			clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "init failed targetDir=%s phase=validation", cfg.Dir)
			return err
		}
		if scaffoldType == DefaultScaffoldType {
			scaffold.files[factoryWorkersDirName+"/processor/"+factoryAgentsFileName] = defaultModelWorkerAgentsMD(executor)
		}
	}

	for _, d := range initDirs {
		path := filepath.Join(cfg.Dir, d)
		if err := os.MkdirAll(path, 0o755); err != nil {
			clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "init failed targetDir=%s phase=create-directories", cfg.Dir)
			return fmt.Errorf("create %s: %w", path, err)
		}
	}

	writtenByCategory := map[string]int{}
	for relativePath, contents := range scaffold.files {
		written, err := writeFileIfAbsent(filepath.Join(cfg.Dir, relativePath), contents)
		if err != nil {
			clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "init failed targetDir=%s phase=write-files", cfg.Dir)
			return err
		}
		if written {
			writtenByCategory[initPathCategory(relativePath)]++
		}
		if relativePath == interfaces.FactoryConfigFile && written {
			fmt.Printf("Created %s\n", filepath.Join(cfg.Dir, relativePath))
		}
	}

	factoryConfigPath := filepath.Join(cfg.Dir, interfaces.FactoryConfigFile)
	if _, err := os.Stat(factoryConfigPath); err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "init failed targetDir=%s phase=verify-factory-config", cfg.Dir)
		return fmt.Errorf("stat %s: %w", factoryConfigPath, err)
	}

	defaultInputDir := filepath.Join(cfg.Dir, interfaces.InputsDir, scaffold.inputWorkType, interfaces.DefaultChannelName)
	if err := os.MkdirAll(defaultInputDir, 0o755); err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "init failed targetDir=%s phase=create-inputs", cfg.Dir)
		return fmt.Errorf("create inputs/%s/default: %w", scaffold.inputWorkType, err)
	}

	fmt.Printf("Initialized %s factory directory structure at %s/\n", scaffoldType, cfg.Dir)
	fmt.Printf("  → Drop work files into %s/ to preseed on startup\n", defaultInputDir)
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"init complete targetDir=%s scaffoldType=%s executor=%s directoryExisted=%t generatedFactoryConfigs=%d generatedWorkerFiles=%d generatedWorkstationFiles=%d generatedInputFiles=%d generatedDocs=%d inputDir=%s",
		cfg.Dir,
		scaffoldType,
		effectiveExecutorLabel(cfg.Executor, scaffoldType),
		dirAlreadyExisted,
		writtenByCategory["factory-config"],
		writtenByCategory["worker"],
		writtenByCategory["workstation"],
		writtenByCategory["input"],
		writtenByCategory["docs"],
		defaultInputDir,
	)
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
