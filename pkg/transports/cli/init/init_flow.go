package initcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func logInitRequest(cfg InitConfig, scaffoldType ScaffoldType, dirAlreadyExisted bool) {
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"init request targetDir=%s scaffoldType=%s executor=%s directoryExisted=%t",
		cfg.Dir,
		scaffoldType,
		effectiveExecutorLabel(cfg.Executor, scaffoldType),
		dirAlreadyExisted,
	)
}

func applyStarterExecutor(cfg InitConfig, scaffoldType ScaffoldType, scaffold *scaffoldDefinition) error {
	if strings.TrimSpace(cfg.Executor) == "" && scaffoldType != DefaultScaffoldType {
		return nil
	}

	executor, err := parseStarterExecutor(cfg.Executor)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "init failed targetDir=%s phase=validation", cfg.Dir)
		return err
	}
	if scaffoldType == DefaultScaffoldType {
		scaffold.files[factoryWorkersDirName+"/processor/"+factoryAgentsFileName] = defaultModelWorkerAgentsMD(executor)
	}
	return nil
}

func materializeScaffold(cfg InitConfig, scaffold scaffoldDefinition) (writtenByCategory map[string]int, defaultInputDir string, err error) {
	for _, d := range initDirs {
		path := filepath.Join(cfg.Dir, d)
		if err := os.MkdirAll(path, 0o755); err != nil {
			clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "init failed targetDir=%s phase=create-directories", cfg.Dir)
			return nil, "", fmt.Errorf("create %s: %w", path, err)
		}
	}

	writtenByCategory = map[string]int{}
	for relativePath, contents := range scaffold.files {
		written, err := writeFileIfAbsent(filepath.Join(cfg.Dir, relativePath), contents)
		if err != nil {
			clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "init failed targetDir=%s phase=write-files", cfg.Dir)
			return nil, "", err
		}
		if written {
			writtenByCategory[initPathCategory(relativePath)]++
		}
		if !cfg.JSON && relativePath == interfaces.FactoryConfigFile && written {
			if _, err := fmt.Fprintf(cfg.Output, "Created %s\n", filepath.Join(cfg.Dir, relativePath)); err != nil {
				return nil, "", err
			}
		}
	}

	factoryConfigPath := filepath.Join(cfg.Dir, interfaces.FactoryConfigFile)
	if _, err := os.Stat(factoryConfigPath); err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "init failed targetDir=%s phase=verify-factory-config", cfg.Dir)
		return nil, "", fmt.Errorf("stat %s: %w", factoryConfigPath, err)
	}

	defaultInputDir = filepath.Join(cfg.Dir, interfaces.InputsDir, scaffold.inputWorkType, interfaces.DefaultChannelName)
	if err := os.MkdirAll(defaultInputDir, 0o755); err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "init failed targetDir=%s phase=create-inputs", cfg.Dir)
		return nil, "", fmt.Errorf("create inputs/%s/default: %w", scaffold.inputWorkType, err)
	}
	return writtenByCategory, defaultInputDir, nil
}

func emitInitResult(
	cfg InitConfig,
	scaffoldType ScaffoldType,
	scaffold scaffoldDefinition,
	writtenByCategory map[string]int,
	dirAlreadyExisted bool,
	defaultInputDir string,
) error {
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(InitResult{
			ScaffoldType: string(scaffoldType),
			TargetDir:    cfg.Dir,
		})
	}

	if _, err := fmt.Fprintf(cfg.Output, "Initialized %s factory directory structure at %s/\n", scaffoldType, cfg.Dir); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cfg.Output, "  → Drop work files into %s/ to preseed on startup\n", defaultInputDir); err != nil {
		return err
	}
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
