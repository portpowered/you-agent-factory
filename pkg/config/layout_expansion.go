package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/inboxgitkeep"
	"github.com/portpowered/infinite-you/pkg/config/namedfactorypath"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type splitRuntimeExpansionOptions struct {
	overwriteExisting bool
}

// FactorySplitLayoutWriteOptions controls how writeFactorySplitLayout materializes
// split factory directories for expand vs persist-from-save flows.
type FactorySplitLayoutWriteOptions struct {
	// SourceDir is the factory directory to copy portable bundled files and
	// referenced scripts from during expand. Leave empty for persist-only writes.
	SourceDir string
	// CopyReferencedScripts copies script files referenced by opted-in workstations
	// when SourceDir is set.
	CopyReferencedScripts bool
	// OverwriteExistingSplitFiles replaces existing workers/ and workstations/
	// AGENTS.md and prompt files and prunes stale entity directories.
	OverwriteExistingSplitFiles bool
}

func writeFactorySplitLayout(
	targetDir string,
	cfg *interfaces.FactoryConfig,
	canonical []byte,
	sourcePath string,
	opts FactorySplitLayoutWriteOptions,
) (LayoutExpansionReport, error) {
	if err := prepareFactorySplitLayoutWrite(targetDir, cfg, canonical, sourcePath, opts); err != nil {
		return LayoutExpansionReport{}, err
	}
	report, err := writeFactorySplitLayoutRuntimeFiles(targetDir, cfg, opts)
	if err != nil {
		return LayoutExpansionReport{}, err
	}
	if err := finalizeFactorySplitLayoutWrite(targetDir, cfg, opts, &report); err != nil {
		return LayoutExpansionReport{}, err
	}
	return report, nil
}

func prepareFactorySplitLayoutWrite(
	targetDir string,
	cfg *interfaces.FactoryConfig,
	canonical []byte,
	sourcePath string,
	opts FactorySplitLayoutWriteOptions,
) error {
	sourceDir := strings.TrimSpace(opts.SourceDir)
	if sourceDir != "" {
		if _, err := preparePortableBundledFileWrites(targetDir, cfg); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create factory directory %s: %w", targetDir, err)
	}
	formatted, err := formatCanonicalFactoryJSON(canonical, sourcePath)
	if err != nil {
		return err
	}
	factoryPath := filepath.Join(targetDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, formatted, 0o644); err != nil {
		return fmt.Errorf("write canonical factory config %s: %w", factoryPath, err)
	}
	return nil
}

func writeFactorySplitLayoutRuntimeFiles(
	targetDir string,
	cfg *interfaces.FactoryConfig,
	opts FactorySplitLayoutWriteOptions,
) (LayoutExpansionReport, error) {
	expansionOpts := splitRuntimeExpansionOptions{overwriteExisting: opts.OverwriteExistingSplitFiles}
	workerAgentPaths, err := writeExpandedWorkerFiles(targetDir, cfg.Workers, expansionOpts)
	if err != nil {
		return LayoutExpansionReport{}, err
	}
	workstationAgentPaths, promptPaths, err := writeExpandedWorkstationFiles(targetDir, cfg.Workstations, expansionOpts)
	if err != nil {
		return LayoutExpansionReport{}, err
	}
	if opts.OverwriteExistingSplitFiles {
		if err := pruneStaleSplitRuntimeDirs(targetDir, cfg); err != nil {
			return LayoutExpansionReport{}, err
		}
	}
	return LayoutExpansionReport{
		FactoryConfigPaths:    1,
		WorkerAgentPaths:      workerAgentPaths,
		WorkstationAgentPaths: workstationAgentPaths,
		PromptPaths:           promptPaths,
	}, nil
}

func finalizeFactorySplitLayoutWrite(
	targetDir string,
	cfg *interfaces.FactoryConfig,
	opts FactorySplitLayoutWriteOptions,
	report *LayoutExpansionReport,
) error {
	replacements, err := materializePortableBundledFiles(targetDir, cfg)
	if err != nil {
		return err
	}
	report.BundledReplacements = replacements

	sourceDir := strings.TrimSpace(opts.SourceDir)
	if sourceDir != "" {
		if err := copySupportedPortableBundledFilesFromSource(sourceDir, targetDir, cfg); err != nil {
			return err
		}
	}
	if opts.OverwriteExistingSplitFiles {
		if err := pruneRemovedPortableBundledDocs(targetDir, cfg); err != nil {
			return err
		}
	}
	if opts.CopyReferencedScripts && sourceDir != "" {
		if err := writeExpandedReferencedScripts(sourceDir, targetDir, cfg); err != nil {
			return err
		}
	}
	if !opts.OverwriteExistingSplitFiles {
		return nil
	}
	inputsDir := filepath.Join(targetDir, interfaces.InputsDir)
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		return fmt.Errorf("create inputs directory %s: %w", inputsDir, err)
	}
	if err := ensureDefaultInputChannelDirectories(targetDir, cfg); err != nil {
		return err
	}
	return ensureCanonicalInputInboxSentinels(targetDir, cfg)
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

func writeExpandedWorkerFiles(targetDir string, workerConfigs []interfaces.WorkerConfig, opts splitRuntimeExpansionOptions) (int, error) {
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
			if !opts.overwriteExisting {
				exists, err := agentsFileExists(workerDir)
				if err != nil {
					return 0, fmt.Errorf("check worker %q AGENTS.md: %w", workerCfg.Name, err)
				}
				if exists {
					continue
				}
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

func writeExpandedWorkstationFiles(targetDir string, workstationConfigs []interfaces.FactoryWorkstationConfig, opts splitRuntimeExpansionOptions) (int, int, error) {
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
		wroteAgents, wrotePrompts, err := expandSingleWorkstation(workstationsDir, workstationCfg, opts)
		if err != nil {
			return 0, 0, err
		}
		agentsWritten += wroteAgents
		promptsWritten += wrotePrompts
	}
	return agentsWritten, promptsWritten, nil
}

func expandSingleWorkstation(
	workstationsDir string,
	workstationCfg interfaces.FactoryWorkstationConfig,
	opts splitRuntimeExpansionOptions,
) (agentsWritten, promptsWritten int, err error) {
	segment, err := safeFactoryLayoutSegment("workstation", workstationCfg.Name)
	if err != nil {
		return 0, 0, err
	}
	workstationDir := filepath.Join(workstationsDir, segment)
	if skip, err := shouldSkipWorkstationExpansion(workstationDir, workstationCfg, opts); err != nil {
		return 0, 0, err
	} else if skip {
		return 0, 0, nil
	}

	def, promptFileContent := workstationDefForExpansion(workstationCfg)
	agents, err := agentsMarkdownForWorkstationExpansion(workstationCfg, def)
	if err != nil {
		return 0, 0, fmt.Errorf("render workstation %q AGENTS.md: %w", workstationCfg.Name, err)
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
	agentsWritten = 1
	if promptPath == "" {
		return agentsWritten, 0, nil
	}
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		return 0, 0, fmt.Errorf("create workstation %q prompt directory: %w", workstationCfg.Name, err)
	}
	if err := os.WriteFile(promptPath, []byte(promptFileContent), 0o644); err != nil {
		return 0, 0, fmt.Errorf("write workstation %q prompt file: %w", workstationCfg.Name, err)
	}
	promptsWritten = 1
	return agentsWritten, promptsWritten, nil
}

func shouldSkipWorkstationExpansion(
	workstationDir string,
	workstationCfg interfaces.FactoryWorkstationConfig,
	opts splitRuntimeExpansionOptions,
) (bool, error) {
	if hasInlineWorkstationRuntime(workstationCfg) || opts.overwriteExisting {
		return false, nil
	}
	exists, err := agentsFileExists(workstationDir)
	if err != nil {
		return false, fmt.Errorf("check workstation %q AGENTS.md: %w", workstationCfg.Name, err)
	}
	return exists, nil
}

func agentsMarkdownForWorkstationExpansion(
	workstationCfg interfaces.FactoryWorkstationConfig,
	def interfaces.FactoryWorkstationConfig,
) ([]byte, error) {
	if hasInlineWorkstationRuntime(workstationCfg) {
		return renderAgentsBody(def.Body), nil
	}
	return renderAgentsMarkdown(workstationFrontmatterForExpansion(def), def.Body)
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

func pruneStaleSplitRuntimeDirs(targetDir string, cfg *interfaces.FactoryConfig) error {
	if cfg == nil {
		return nil
	}
	if err := pruneStaleRuntimeEntityDirs(
		targetDir,
		interfaces.WorkersDir,
		"worker",
		len(cfg.Workers),
		func(i int) (string, error) {
			return safeFactoryLayoutSegment("worker", cfg.Workers[i].Name)
		},
	); err != nil {
		return err
	}
	return pruneStaleRuntimeEntityDirs(
		targetDir,
		interfaces.WorkstationsDir,
		"workstation",
		len(cfg.Workstations),
		func(i int) (string, error) {
			return safeFactoryLayoutSegment("workstation", cfg.Workstations[i].Name)
		},
	)
}

func pruneStaleRuntimeEntityDirs(
	targetDir string,
	parentDirName string,
	kind string,
	count int,
	segmentAt func(int) (string, error),
) error {
	parentDir := filepath.Join(targetDir, parentDirName)
	info, err := os.Stat(parentDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat %s directory: %w", parentDirName, err)
	}
	if !info.IsDir() {
		return nil
	}

	keep := make(map[string]struct{}, count)
	for i := 0; i < count; i++ {
		segment, err := segmentAt(i)
		if err != nil {
			return err
		}
		keep[segment] = struct{}{}
	}

	children, err := os.ReadDir(parentDir)
	if err != nil {
		return fmt.Errorf("read %s directory: %w", parentDirName, err)
	}
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		if _, ok := keep[child.Name()]; ok {
			continue
		}
		entityDir := filepath.Join(parentDir, child.Name())
		if err := os.RemoveAll(entityDir); err != nil {
			return fmt.Errorf("prune %s %q: %w", kind, child.Name(), err)
		}
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

// NamedFactoryListEntry describes one persisted named factory under a factory root.
type NamedFactoryListEntry struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDirectory"`
	Current    bool   `json:"current"`
}

const (
	defaultNamedFactoryHomeDir = ".you-agent-factory"
	defaultProjectNamedFactoryRoot = "factory"
	scopedNamedFactoryPrefix       = "@"
)

// NamedFactoryPathSegments returns the validated hierarchical path segments for
// a canonical named-factory display name.
func NamedFactoryPathSegments(name string) ([]string, error) {
	segments, err := namedfactorypath.PathSegments(name)
	if err != nil {
		return nil, wrapInvalidNamedFactoryName(name, err)
	}
	return segments, nil
}

// MapNamedFactoryDir maps a canonical named-factory display name to its
// hierarchical on-disk directory under factoriesRoot.
func MapNamedFactoryDir(factoriesRoot, name string) (string, error) {
	if strings.TrimSpace(factoriesRoot) == "" {
		return "", fmt.Errorf("factory root is required")
	}
	dir, err := namedfactorypath.MapDir(factoriesRoot, name)
	if err != nil {
		return "", wrapInvalidNamedFactoryName(name, err)
	}
	return dir, nil
}

// NamedFactoryNameToLayoutSegment maps a canonical named-factory display name into the single on-disk directory segment used under a factory root.
func NamedFactoryNameToLayoutSegment(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if strings.HasPrefix(trimmed, scopedNamedFactoryPrefix) {
		if err := validateScopedNamedFactoryName(trimmed); err != nil {
			return "", wrapInvalidNamedFactoryName(trimmed, err)
		}
		segment := encodeScopedNamedFactoryLayoutSegment(trimmed)
		if _, err := safeFactoryLayoutSegment("factory", segment); err != nil {
			return "", wrapInvalidNamedFactoryName(trimmed, err)
		}
		return segment, nil
	}
	if segment, err := safeFactoryLayoutSegment("factory", trimmed); err != nil {
		return "", wrapInvalidNamedFactoryName(trimmed, err)
	} else {
		return segment, nil
	}
}

func encodeScopedNamedFactoryLayoutSegment(name string) string {
	return strings.NewReplacer("%", "%25", "/", "%2F").Replace(name)
}

// NamedFactoryLayoutSegmentToName maps an on-disk named-factory directory segment back to the canonical display name shown by list and API callers.
func NamedFactoryLayoutSegmentToName(segment string) (string, error) {
	safeSegment, err := safeFactoryLayoutSegment("factory", segment)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(safeSegment, scopedNamedFactoryPrefix) {
		return safeSegment, nil
	}

	name, err := url.PathUnescape(safeSegment)
	if err != nil {
		return "", fmt.Errorf("decode factory layout segment %q: %w", segment, err)
	}
	encoded, err := NamedFactoryNameToLayoutSegment(name)
	if err != nil {
		return "", err
	}
	if encoded != safeSegment {
		return "", fmt.Errorf("factory layout segment %q is not canonical for %q", segment, name)
	}
	return name, nil
}

func validateScopedNamedFactoryName(name string) error {
	parts := strings.Split(name, "/")
	if len(parts) != 2 || parts[0] == scopedNamedFactoryPrefix || parts[1] == "" {
		return fmt.Errorf("factory name %q must be scoped as @scope/name", name)
	}
	scope := strings.TrimPrefix(parts[0], scopedNamedFactoryPrefix)
	if _, err := safeFactoryLayoutSegment("factory scope", scope); err != nil {
		return err
	}
	if _, err := safeFactoryLayoutSegment("factory", parts[1]); err != nil {
		return err
	}
	return nil
}

// GlobalNamedFactoryRootForHome builds the customer-owned global named-factory
// root for a resolved home directory.
func GlobalNamedFactoryRootForHome(homeDir string) (string, error) {
	trimmed := strings.TrimSpace(homeDir)
	if trimmed == "" {
		return "", fmt.Errorf("user home directory is required")
	}
	return defaultpaths.NamedFactoriesRoot(trimmed), nil
}

// GlobalWorkflowRootForHome builds the customer-owned global workflow lookup root
// for a resolved home directory.
func GlobalWorkflowRootForHome(homeDir string) (string, error) {
	trimmed := strings.TrimSpace(homeDir)
	if trimmed == "" {
		return "", fmt.Errorf("user home directory is required")
	}
	return filepath.Join(trimmed, defaultNamedFactoryHomeDir, "workflows"), nil
}

// DefaultGlobalNamedFactoryRoot returns the default global named-factory root
// under the current user's home directory.
func DefaultGlobalNamedFactoryRoot() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home for named factories: %w", err)
	}
	return GlobalNamedFactoryRootForHome(homeDir)
}

// DefaultProjectNamedFactoryRoot returns the default project-local named
// factory root for a caller working directory.
func DefaultProjectNamedFactoryRoot(cwd string) (string, error) {
	trimmed := strings.TrimSpace(cwd)
	if trimmed == "" {
		return "", fmt.Errorf("working directory is required")
	}
	return filepath.Join(trimmed, defaultProjectNamedFactoryRoot), nil
}

// ListNamedFactories discovers persisted named factories by scanning rootDir for
// subdirectories that contain a valid factory.json layout.
func ListNamedFactories(rootDir string) ([]NamedFactoryListEntry, error) {
	if strings.TrimSpace(rootDir) == "" {
		return nil, fmt.Errorf("factory root is required")
	}

	info, err := os.Stat(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("factory root %s does not exist: %w", rootDir, err)
		}
		return nil, fmt.Errorf("stat factory root %s: %w", rootDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("factory root %s is not a directory", rootDir)
	}

	currentName, err := readCurrentFactoryPointerForList(rootDir)
	if err != nil {
		return nil, err
	}

	children, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("read factory root %s: %w", rootDir, err)
	}

	entries := make([]NamedFactoryListEntry, 0, len(children))
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		name := child.Name()
		if name == interfaces.InputsDir || name == interfaces.WorkersDir || name == interfaces.WorkstationsDir {
			continue
		}
		factoryDir := filepath.Join(rootDir, name)
		if err := requireFactoryConfig(factoryDir); err != nil {
			continue
		}
		displayName, err := NamedFactoryLayoutSegmentToName(name)
		if err != nil {
			continue
		}
		entries = append(entries, NamedFactoryListEntry{
			Name:       displayName,
			FactoryDir: factoryDir,
			Current:    currentName != "" && displayName == currentName,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func readCurrentFactoryPointerForList(rootDir string) (string, error) {
	name, err := ReadCurrentFactoryPointer(rootDir)
	if err == nil {
		return name, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return "", fmt.Errorf("read current factory pointer: %w", err)
}

// ErrNamedFactoryIsCurrent reports that a named factory cannot be deleted
// because it is selected by .current-factory.
var ErrNamedFactoryIsCurrent = errors.New("cannot delete current factory")

// DeleteNamedFactory removes a persisted named factory directory under rootDir.
// It refuses to delete the factory referenced by .current-factory.
func DeleteNamedFactory(rootDir, name string) error {
	if strings.TrimSpace(rootDir) == "" {
		return fmt.Errorf("factory root is required")
	}

	segment, err := NamedFactoryNameToLayoutSegment(name)
	if err != nil {
		return err
	}
	canonicalName, err := NamedFactoryLayoutSegmentToName(segment)
	if err != nil {
		return err
	}

	factoryDir, err := ResolveNamedFactoryDir(rootDir, name)
	if err != nil {
		return err
	}

	current, err := ReadCurrentFactoryPointer(rootDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read current factory pointer: %w", err)
	}
	if current == canonicalName {
		return fmt.Errorf(
			"delete factory %q: %w: switch .current-factory to another factory first",
			segment,
			ErrNamedFactoryIsCurrent,
		)
	}

	if err := os.RemoveAll(factoryDir); err != nil {
		return fmt.Errorf("delete factory %q: %w", segment, err)
	}
	return nil
}

func ensureDefaultInputChannelDirectories(targetDir string, cfg *interfaces.FactoryConfig) error {
	if cfg == nil {
		return nil
	}

	for _, workType := range cfg.WorkTypes {
		workTypeName := strings.TrimSpace(workType.Name)
		if workTypeName == "" {
			continue
		}

		channelDir := filepath.Join(targetDir, interfaces.InputsDir, workTypeName, interfaces.DefaultChannelName)
		if err := os.MkdirAll(channelDir, 0o755); err != nil {
			return fmt.Errorf("create inputs/%s/%s directory: %w", workTypeName, interfaces.DefaultChannelName, err)
		}
	}
	return nil
}

const batchInputInboxChannelName = "BATCH"

func ensureCanonicalInputInboxSentinels(targetDir string, cfg *interfaces.FactoryConfig) error {
	if err := inboxgitkeep.EnsureInputInboxGitkeep(
		targetDir,
		filepath.Join(interfaces.InputsDir, batchInputInboxChannelName, interfaces.DefaultChannelName, ".gitkeep"),
	); err != nil {
		return fmt.Errorf("ensure batch inbox sentinel: %w", err)
	}
	if cfg == nil {
		return nil
	}
	for _, workType := range cfg.WorkTypes {
		workTypeName := strings.TrimSpace(workType.Name)
		if workTypeName == "" {
			continue
		}
		relativePath := filepath.Join(
			interfaces.InputsDir,
			workTypeName,
			interfaces.DefaultChannelName,
			".gitkeep",
		)
		if err := inboxgitkeep.EnsureInputInboxGitkeep(targetDir, relativePath); err != nil {
			return fmt.Errorf("ensure inputs/%s/%s .gitkeep: %w", workTypeName, interfaces.DefaultChannelName, err)
		}
	}
	return nil
}
