package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func writeExpandedFactoryLayout(sourceDir, targetDir string, cfg *interfaces.FactoryConfig, canonical []byte, sourcePath string) (LayoutExpansionReport, error) {
	if _, err := preparePortableBundledFileWrites(targetDir, cfg); err != nil {
		return LayoutExpansionReport{}, err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return LayoutExpansionReport{}, fmt.Errorf("create factory directory %s: %w", targetDir, err)
	}

	formatted, err := formatCanonicalFactoryJSON(canonical, sourcePath)
	if err != nil {
		return LayoutExpansionReport{}, err
	}
	factoryPath := filepath.Join(targetDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, formatted, 0o644); err != nil {
		return LayoutExpansionReport{}, fmt.Errorf("write canonical factory config %s: %w", factoryPath, err)
	}
	report := LayoutExpansionReport{FactoryConfigPaths: 1}

	workerAgentPaths, err := writeExpandedWorkerFiles(targetDir, cfg.Workers)
	if err != nil {
		return LayoutExpansionReport{}, err
	}
	report.WorkerAgentPaths = workerAgentPaths

	workstationAgentPaths, promptPaths, err := writeExpandedWorkstationFiles(targetDir, cfg.Workstations)
	if err != nil {
		return LayoutExpansionReport{}, err
	}
	report.WorkstationAgentPaths = workstationAgentPaths
	report.PromptPaths = promptPaths

	replacements, err := materializePortableBundledFiles(targetDir, cfg)
	if err != nil {
		return LayoutExpansionReport{}, err
	}
	report.BundledReplacements = replacements
	if err := copySupportedPortableBundledFilesFromSource(sourceDir, targetDir, cfg); err != nil {
		return LayoutExpansionReport{}, err
	}
	if err := writeExpandedReferencedScripts(sourceDir, targetDir, cfg); err != nil {
		return LayoutExpansionReport{}, err
	}
	return report, nil
}

func copySupportedPortableBundledFilesFromSource(sourceDir, targetDir string, cfg *interfaces.FactoryConfig) error {
	if cfg == nil || cfg.ResourceManifest == nil || len(cfg.ResourceManifest.BundledFiles) == 0 {
		return nil
	}

	validationRoot, err := preparePortableBundledValidationRoot(targetDir)
	if err != nil {
		return err
	}

	for _, bundledFile := range cfg.ResourceManifest.BundledFiles {
		if err := copySupportedPortableBundledFileFromSource(validationRoot, sourceDir, bundledFile); err != nil {
			return err
		}
	}
	return nil
}

func copySupportedPortableBundledFileFromSource(
	validationRoot portableBundledValidationRoot,
	sourceDir string,
	bundledFile interfaces.BundledFileConfig,
) error {
	if !shouldCopySupportedPortableBundledFile(bundledFile) {
		return nil
	}
	sourcePath, ok := supportedPortableBundledSourcePath(sourceDir, bundledFile)
	if !ok {
		return nil
	}
	target, shouldCopy, err := resolvePortableBundledCopyTarget(validationRoot, bundledFile.TargetPath, sourcePath)
	if err != nil || !shouldCopy {
		return err
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read portable bundled file %s: %w", sourcePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(target.path), 0o755); err != nil {
		return fmt.Errorf("create bundled file directory for %s: %w", target.path, err)
	}
	if err := writePortableBundledFile(target.path, data, portableBundledFileMode(bundledFile)); err != nil {
		return fmt.Errorf("write bundled file %s: %w", target.path, err)
	}
	return nil
}

func shouldCopySupportedPortableBundledFile(bundledFile interfaces.BundledFileConfig) bool {
	return shouldOmitSupportedPortableBundledInline(bundledFile) && strings.TrimSpace(bundledFile.Content.Inline) == ""
}

func resolvePortableBundledCopyTarget(
	validationRoot portableBundledValidationRoot,
	targetPath string,
	sourcePath string,
) (portableBundledResolvedTarget, bool, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return portableBundledResolvedTarget{}, false, nil
		}
		return portableBundledResolvedTarget{}, false, fmt.Errorf("stat portable bundled file %s: %w", sourcePath, err)
	}
	if !info.Mode().IsRegular() {
		return portableBundledResolvedTarget{}, false, nil
	}
	target, err := portableBundledTargetPath(validationRoot.targetDir, targetPath)
	if err != nil {
		return portableBundledResolvedTarget{}, false, fmt.Errorf("resolve bundled file %q: %w", targetPath, err)
	}
	if err := validatePortableBundledFilesystemPath(validationRoot, targetPath, target); err != nil {
		return portableBundledResolvedTarget{}, false, fmt.Errorf("resolve bundled file %q: %w", targetPath, err)
	}
	return target, true, nil
}

func writeExpandedWorkerFiles(targetDir string, workerConfigs []interfaces.WorkerConfig) (int, error) {
	workersDir := filepath.Join(targetDir, interfaces.WorkersDir)
	if err := os.MkdirAll(workersDir, 0o755); err != nil {
		return 0, fmt.Errorf("create workers directory %s: %w", workersDir, err)
	}

	configs := append([]interfaces.WorkerConfig(nil), workerConfigs...)
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Name < configs[j].Name
	})

	written := 0
	for _, workerCfg := range configs {
		segment, err := safeFactoryLayoutSegment("worker", workerCfg.Name)
		if err != nil {
			return 0, err
		}
		workerDir := filepath.Join(workersDir, segment)
		if workerCfg.Type == "" {
			exists, err := agentsFileExists(workerDir)
			if err != nil {
				return 0, fmt.Errorf("check worker %q AGENTS.md: %w", workerCfg.Name, err)
			}
			if exists {
				continue
			}
			def := workerDefForExpansion(workerCfg)
			agents, err := renderAgentsMarkdown(workerFrontmatterForExpansion(def), def.Body)
			if err != nil {
				return 0, fmt.Errorf("render worker %q AGENTS.md: %w", workerCfg.Name, err)
			}
			if err := writeAgentsFile(workerDir, agents); err != nil {
				return 0, fmt.Errorf("write worker %q AGENTS.md: %w", workerCfg.Name, err)
			}
			written++
			continue
		}
		agents := renderAgentsBody(workerCfg.Body)
		if err := writeAgentsFile(workerDir, agents); err != nil {
			return 0, fmt.Errorf("write worker %q AGENTS.md: %w", workerCfg.Name, err)
		}
		written++
	}
	return written, nil
}

func writeExpandedWorkstationFiles(targetDir string, workstationConfigs []interfaces.FactoryWorkstationConfig) (int, int, error) {
	workstationsDir := filepath.Join(targetDir, interfaces.WorkstationsDir)
	if err := os.MkdirAll(workstationsDir, 0o755); err != nil {
		return 0, 0, fmt.Errorf("create workstations directory %s: %w", workstationsDir, err)
	}

	configs := append([]interfaces.FactoryWorkstationConfig(nil), workstationConfigs...)
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Name < configs[j].Name
	})

	agentsWritten := 0
	promptsWritten := 0
	for _, workstationCfg := range configs {
		segment, err := safeFactoryLayoutSegment("workstation", workstationCfg.Name)
		if err != nil {
			return 0, 0, err
		}
		workstationDir := filepath.Join(workstationsDir, segment)
		if !hasInlineWorkstationRuntime(workstationCfg) {
			exists, err := agentsFileExists(workstationDir)
			if err != nil {
				return 0, 0, fmt.Errorf("check workstation %q AGENTS.md: %w", workstationCfg.Name, err)
			}
			if exists {
				continue
			}
		}
		def, promptFileContent := workstationDefForExpansion(workstationCfg)
		agents := renderAgentsBody(def.Body)
		if !hasInlineWorkstationRuntime(workstationCfg) {
			agents, err = renderAgentsMarkdown(workstationFrontmatterForExpansion(def), def.Body)
			if err != nil {
				return 0, 0, fmt.Errorf("render workstation %q AGENTS.md: %w", workstationCfg.Name, err)
			}
		}
		promptPath := ""
		if def.PromptFile != "" {
			promptPath, err = safePromptFilePath(workstationDir, def.PromptFile)
			if err != nil {
				return 0, 0, fmt.Errorf("resolve workstation %q prompt file: %w", workstationCfg.Name, err)
			}
		}
		if err := writeAgentsFile(workstationDir, agents); err != nil {
			return 0, 0, fmt.Errorf("write workstation %q AGENTS.md: %w", workstationCfg.Name, err)
		}
		agentsWritten++
		if promptPath != "" {
			if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
				return 0, 0, fmt.Errorf("create workstation %q prompt directory: %w", workstationCfg.Name, err)
			}
			if err := os.WriteFile(promptPath, []byte(promptFileContent), 0o644); err != nil {
				return 0, 0, fmt.Errorf("write workstation %q prompt file: %w", workstationCfg.Name, err)
			}
			promptsWritten++
		}
	}
	return agentsWritten, promptsWritten, nil
}

func writeExpandedReferencedScripts(sourceDir, targetDir string, cfg *interfaces.FactoryConfig) error {
	if cfg == nil {
		return nil
	}

	workersByName := make(map[string]interfaces.WorkerConfig, len(cfg.Workers))
	for _, workerCfg := range cfg.Workers {
		workersByName[workerCfg.Name] = CloneWorkerConfig(workerCfg)
	}

	copied := make(map[string]bool)
	for _, workstationCfg := range cfg.Workstations {
		if !workstationCfg.CopyReferencedScripts {
			continue
		}

		referencedPaths, err := workstationReferencedScriptPaths(workstationCfg, workersByName)
		if err != nil {
			return fmt.Errorf("copy referenced scripts for workstation %q: %w", workstationCfg.Name, err)
		}
		for _, relativePath := range referencedPaths {
			if copied[relativePath] {
				continue
			}
			if err := copyFactoryRelativeFile(sourceDir, targetDir, relativePath); err != nil {
				return fmt.Errorf("copy referenced script %q for workstation %q: %w", relativePath, workstationCfg.Name, err)
			}
			copied[relativePath] = true
		}
	}
	return nil
}

func workstationReferencedScriptPaths(
	workstation interfaces.FactoryWorkstationConfig,
	workersByName map[string]interfaces.WorkerConfig,
) ([]string, error) {
	if strings.TrimSpace(workstation.WorkerTypeName) == "" {
		return nil, nil
	}

	workerCfg, ok := workersByName[workstation.WorkerTypeName]
	if !ok {
		return nil, fmt.Errorf("worker %q not found", workstation.WorkerTypeName)
	}
	if workerCfg.Type != interfaces.WorkerTypeScript {
		return nil, nil
	}
	return supportedReferencedScriptPaths(workerCfg)
}

func supportedReferencedScriptPaths(worker interfaces.WorkerConfig) ([]string, error) {
	paths := make([]string, 0, 2)

	commandPath, err := referencedScriptPath(worker.Command)
	if err != nil {
		return nil, err
	}
	if commandPath != "" {
		paths = append(paths, commandPath)
	}

	if !isScriptInterpreterCommand(worker.Command) {
		return paths, nil
	}

	argPath, err := firstReferencedScriptArg(worker.Command, worker.Args)
	if err != nil {
		return nil, err
	}
	if argPath != "" && argPath != commandPath {
		paths = append(paths, argPath)
	}
	return paths, nil
}

func referencedScriptPath(raw string) (string, error) {
	if !looksLikeScriptPathReference(raw) {
		return "", nil
	}
	return normalizeFactoryRelativeScriptPath(raw)
}

func firstReferencedScriptArg(command string, args []string) (string, error) {
	skipNextValue := false
	nextValueIsScriptPath := false
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if nextValueIsScriptPath && shouldUseScriptArg(trimmed) {
			return normalizeFactoryRelativeScriptPath(trimmed)
		}
		nextValueIsScriptPath = false
		if skipNextValue {
			skipNextValue = false
			continue
		}
		if !shouldInspectScriptArg(trimmed) {
			continue
		}
		if trimmed == "--" {
			nextValueIsScriptPath = true
			continue
		}
		if strings.HasPrefix(trimmed, "-") {
			switch interpreterFlagModeForArg(command, trimmed) {
			case interpreterArgFlagSkipNextValue:
				skipNextValue = true
			case interpreterArgFlagScriptPathValue:
				nextValueIsScriptPath = true
			}
			continue
		}
		if !looksLikeScriptPathReference(trimmed) {
			continue
		}
		return normalizeFactoryRelativeScriptPath(trimmed)
	}
	return "", nil
}

func shouldUseScriptArg(trimmed string) bool {
	return shouldInspectScriptArg(trimmed) && trimmed != "--"
}

func shouldInspectScriptArg(trimmed string) bool {
	return trimmed != "" && !strings.Contains(trimmed, "{{") && !strings.Contains(trimmed, "}}")
}

type interpreterArgFlagMode int

const (
	interpreterArgFlagIgnore interpreterArgFlagMode = iota
	interpreterArgFlagSkipNextValue
	interpreterArgFlagScriptPathValue
)

func interpreterFlagModeForArg(command, arg string) interpreterArgFlagMode {
	normalized := strings.ToLower(strings.TrimSpace(arg))
	if !shouldInspectInterpreterFlag(normalized) {
		return interpreterArgFlagIgnore
	}

	switch interpreterCommandKey(command) {
	case "node", "bun":
		switch normalized {
		case "-e", "--eval", "-p", "--print", "-r", "--require", "--import", "--loader", "--experimental-loader", "--conditions", "--input-type", "--env-file", "--env-file-if-exists", "--inspect-port", "--openssl-config", "--redirect-warnings", "--trace-event-categories", "--title", "--watch-path":
			return interpreterArgFlagSkipNextValue
		}
	case "python", "python3":
		switch normalized {
		case "-c", "-m", "-w", "-x":
			return interpreterArgFlagSkipNextValue
		}
	case "powershell", "pwsh":
		switch normalized {
		case "-file", "-f":
			return interpreterArgFlagScriptPathValue
		case "-command", "-c", "-configurationname", "-custompipename", "-encodedcommand", "-ec", "-executionpolicy", "-inputformat", "-outputformat", "-settingsfile", "-workingdirectory":
			return interpreterArgFlagSkipNextValue
		}
	case "bash", "sh":
		switch normalized {
		case "-c", "-o":
			return interpreterArgFlagSkipNextValue
		}
	case "ruby":
		switch normalized {
		case "-c", "-e", "-i", "-r":
			return interpreterArgFlagSkipNextValue
		}
	case "perl":
		switch normalized {
		case "-e", "-i", "-m", "-x":
			return interpreterArgFlagSkipNextValue
		}
	}

	return interpreterArgFlagIgnore
}

func shouldInspectInterpreterFlag(normalized string) bool {
	return normalized != "" && strings.HasPrefix(normalized, "-") && !strings.Contains(normalized, "=")
}

func interpreterCommandKey(command string) string {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func looksLikeScriptPathReference(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	if filepath.IsAbs(trimmed) || filepath.VolumeName(trimmed) != "" {
		return true
	}
	if strings.Contains(trimmed, "{{") || strings.Contains(trimmed, "}}") {
		return false
	}
	if strings.ContainsAny(trimmed, `/\`) || strings.HasPrefix(trimmed, ".") {
		return true
	}
	return isScriptLikeExtension(filepath.Ext(trimmed))
}

func isScriptLikeExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".py", ".ps1", ".psm1", ".sh", ".bash", ".zsh", ".js", ".mjs", ".cjs", ".ts", ".rb", ".pl", ".cmd", ".bat":
		return true
	default:
		return false
	}
}

func isScriptInterpreterCommand(command string) bool {
	switch interpreterCommandKey(command) {
	case "python", "python3", "bash", "sh", "powershell", "pwsh", "node", "bun", "ruby", "perl":
		return true
	default:
		return false
	}
}

func normalizeFactoryRelativeScriptPath(raw string) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(raw))
	if cleaned == "" || cleaned == "." {
		return "", fmt.Errorf("script path is required")
	}
	if filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" {
		return "", fmt.Errorf("script path %q must be relative to the factory directory", raw)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("script path %q cannot escape the factory directory", raw)
	}
	return cleaned, nil
}

func copyFactoryRelativeFile(sourceDir, targetDir, relativePath string) error {
	sourcePath := filepath.Join(sourceDir, relativePath)
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("read source file %s: %w", sourcePath, err)
	}
	if sourceInfo.IsDir() {
		return fmt.Errorf("source file %s is a directory", sourcePath)
	}

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read source file %s: %w", sourcePath, err)
	}

	targetPath := filepath.Join(targetDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create target directory for %s: %w", targetPath, err)
	}

	mode := sourceInfo.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	if err := os.WriteFile(targetPath, data, mode); err != nil {
		return fmt.Errorf("write target file %s: %w", targetPath, err)
	}
	return nil
}

func agentsFileExists(dir string) (bool, error) {
	path := filepath.Join(dir, interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
