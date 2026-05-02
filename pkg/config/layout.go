package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"gopkg.in/yaml.v3"
)

// InlineRuntimeDefinitionOptions controls how split runtime definition files
// are resolved when building a self-contained factory config.
type InlineRuntimeDefinitionOptions struct {
	RequireSplitDefinitions bool
	WorkstationLoader       WorkstationLoader
}

// FlattenFactoryConfig reads a factory directory or factory.json file and
// returns canonical JSON with worker and workstation runtime definitions inlined.
func FlattenFactoryConfig(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("factory path is required")
	}

	data, sourcePath, err := readFactoryConfigSource(path)
	if err != nil {
		return nil, err
	}

	mapper := NewFactoryConfigMapper()
	factoryCfg, err := mapper.Expand(data)
	if err != nil {
		return nil, fmt.Errorf("parse factory config %s: %w", sourcePath, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("find factory config source %s: %w", path, err)
	}
	factoryDir := filepath.Dir(sourcePath)
	requireSplitDefinitions := false
	if info.IsDir() {
		factoryDir = path
		requireSplitDefinitions = true
	}

	factoryCfg, err = InlineRuntimeDefinitions(factoryDir, factoryCfg, InlineRuntimeDefinitionOptions{
		RequireSplitDefinitions: requireSplitDefinitions,
	})
	if err != nil {
		return nil, err
	}
	if err := applySupportedPortableBundledFiles(factoryDir, factoryCfg); err != nil {
		return nil, fmt.Errorf("collect portable bundled files %s: %w", factoryDir, err)
	}
	flattened, err := mapper.Flatten(factoryCfg)
	if err != nil {
		return nil, fmt.Errorf("flatten factory config %s: %w", sourcePath, err)
	}

	return formatCanonicalFactoryJSON(flattened, sourcePath)
}

// ExpandFactoryConfigLayout writes a split factory directory layout from a
// canonical factory.json file and returns the directory that received the files.
func ExpandFactoryConfigLayout(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("factory config path is required")
	}

	data, sourcePath, targetDir, err := readFactoryConfigExpansionSource(path)
	if err != nil {
		return "", err
	}

	mapper := NewFactoryConfigMapper()
	factoryCfg, err := mapper.Expand(data)
	if err != nil {
		return "", fmt.Errorf("parse factory config %s: %w", sourcePath, err)
	}
	if err := validatePortableResourceManifestForExpand(factoryCfg); err != nil {
		return "", err
	}

	canonical, err := mapper.Flatten(factoryCfg)
	if err != nil {
		return "", fmt.Errorf("normalize factory config %s: %w", sourcePath, err)
	}

	cfgForExpandedFiles, err := InlineRuntimeDefinitions(targetDir, factoryCfg, InlineRuntimeDefinitionOptions{})
	if err != nil {
		return "", fmt.Errorf("load split runtime definitions for expand %s: %w", targetDir, err)
	}
	if cfgForExpandedFiles == nil {
		cfgForExpandedFiles = factoryCfg
	}

	if err := writeExpandedFactoryLayout(filepath.Dir(sourcePath), targetDir, cfgForExpandedFiles, canonical, sourcePath); err != nil {
		return "", err
	}
	return targetDir, nil
}

// InlineRuntimeDefinitions returns a copy of cfg with any runtime definitions
// found in workers/<name>/AGENTS.md and workstations/<name>/AGENTS.md embedded
// into the factory config.
func InlineRuntimeDefinitions(factoryDir string, cfg *interfaces.FactoryConfig, opts InlineRuntimeDefinitionOptions) (*interfaces.FactoryConfig, error) {
	if cfg == nil {
		return nil, nil
	}

	inlined, err := CloneFactoryConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("clone factory config: %w", err)
	}

	for i := range inlined.Workers {
		def, err := runtimeWorkerDefinition(factoryDir, inlined.Workers[i], opts.RequireSplitDefinitions)
		if err != nil {
			return nil, fmt.Errorf("load worker %q config: %w", inlined.Workers[i].Name, err)
		}
		if def == nil {
			continue
		}
		applyWorkerRuntimeDefinition(&inlined.Workers[i], def)
	}

	for i := range inlined.Workstations {
		def, err := runtimeWorkstationDefinition(factoryDir, inlined.Workstations[i], opts.RequireSplitDefinitions, opts.WorkstationLoader)
		if err != nil {
			return nil, fmt.Errorf("load workstation %q config: %w", inlined.Workstations[i].Name, err)
		}
		if def == nil {
			continue
		}
		if err := applyWorkstationRuntimeDefinition(&inlined.Workstations[i], def); err != nil {
			return nil, fmt.Errorf("normalize workstation %q config: %w", inlined.Workstations[i].Name, err)
		}
	}
	return inlined, nil
}

// FactoryConfigWithRuntimeDefinitions returns a copy of cfg with runtime
// definitions from runtimeCfg embedded into the worker and workstation entries.
func FactoryConfigWithRuntimeDefinitions(cfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup) (*interfaces.FactoryConfig, error) {
	if cfg == nil {
		return nil, nil
	}
	if runtimeCfg == nil {
		return nil, fmt.Errorf("runtime config is required")
	}

	inlined, err := CloneFactoryConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("clone factory config: %w", err)
	}

	for i := range inlined.Workers {
		def, ok := runtimeCfg.Worker(inlined.Workers[i].Name)
		if !ok || def == nil {
			continue
		}
		applyWorkerRuntimeDefinition(&inlined.Workers[i], def)
	}
	for i := range inlined.Workstations {
		def, ok := runtimeCfg.Workstation(inlined.Workstations[i].Name)
		if !ok || def == nil {
			continue
		}
		if err := applyWorkstationRuntimeDefinition(&inlined.Workstations[i], def); err != nil {
			return nil, fmt.Errorf("normalize workstation %q config: %w", inlined.Workstations[i].Name, err)
		}
	}
	return inlined, nil
}

// CloneFactoryConfig deep-copies a factory config through explicit field copies.
func CloneFactoryConfig(cfg *interfaces.FactoryConfig) (*interfaces.FactoryConfig, error) {
	if cfg == nil {
		return nil, nil
	}
	cloned := &interfaces.FactoryConfig{
		Project:          cfg.Project,
		InputTypes:       cloneInputTypeConfigs(cfg.InputTypes),
		WorkTypes:        cloneWorkTypeConfigs(cfg.WorkTypes),
		Resources:        cloneResourceConfigs(cfg.Resources),
		ResourceManifest: clonePortableResourceManifestConfig(cfg.ResourceManifest),
		Workers:          cloneWorkerConfigs(cfg.Workers),
		Workstations:     cloneWorkstationConfigs(cfg.Workstations),
	}
	return cloned, nil
}

// CloneWorkerConfig returns a copy of a worker runtime definition.
func CloneWorkerConfig(def interfaces.WorkerConfig) interfaces.WorkerConfig {
	def.Args = append([]string(nil), def.Args...)
	def.Resources = append([]interfaces.ResourceConfig(nil), def.Resources...)
	return def
}

// CloneWorkstationConfig returns a copy of a workstation runtime definition.
func CloneWorkstationConfig(def interfaces.FactoryWorkstationConfig) interfaces.FactoryWorkstationConfig {
	def.Inputs = cloneIOConfigs(def.Inputs)
	def.Outputs = cloneIOConfigs(def.Outputs)
	def.Resources = append([]interfaces.ResourceConfig(nil), def.Resources...)
	def.Guards = cloneGuardConfigs(def.Guards)
	def.StopWords = append([]string(nil), def.StopWords...)
	def.RuntimeStopWords = append([]string(nil), def.RuntimeStopWords...)
	if def.Cron != nil {
		cron := *def.Cron
		def.Cron = &cron
	}
	def.OnContinue = cloneIOConfigPtr(def.OnContinue)
	def.OnRejection = cloneIOConfigPtr(def.OnRejection)
	def.OnFailure = cloneIOConfigPtr(def.OnFailure)
	if def.Env != nil {
		env := make(map[string]string, len(def.Env))
		for key, value := range def.Env {
			env[key] = value
		}
		def.Env = env
	}
	return def
}

func cloneInputTypeConfigs(configs []interfaces.InputTypeConfig) []interfaces.InputTypeConfig {
	return append([]interfaces.InputTypeConfig(nil), configs...)
}

func cloneWorkTypeConfigs(configs []interfaces.WorkTypeConfig) []interfaces.WorkTypeConfig {
	out := append([]interfaces.WorkTypeConfig(nil), configs...)
	for i := range out {
		out[i].States = append([]interfaces.StateConfig(nil), configs[i].States...)
	}
	return out
}

func cloneResourceConfigs(configs []interfaces.ResourceConfig) []interfaces.ResourceConfig {
	return append([]interfaces.ResourceConfig(nil), configs...)
}

func clonePortableResourceManifestConfig(cfg *interfaces.PortableResourceManifestConfig) *interfaces.PortableResourceManifestConfig {
	if cfg == nil {
		return nil
	}

	cloned := &interfaces.PortableResourceManifestConfig{
		RequiredTools: make([]interfaces.RequiredToolConfig, len(cfg.RequiredTools)),
		BundledFiles:  make([]interfaces.BundledFileConfig, len(cfg.BundledFiles)),
	}
	for i := range cfg.RequiredTools {
		cloned.RequiredTools[i] = cfg.RequiredTools[i]
		cloned.RequiredTools[i].VersionArgs = append([]string(nil), cfg.RequiredTools[i].VersionArgs...)
	}
	for i := range cfg.BundledFiles {
		cloned.BundledFiles[i] = cfg.BundledFiles[i]
	}
	return cloned
}

func cloneWorkerConfigs(configs []interfaces.WorkerConfig) []interfaces.WorkerConfig {
	out := make([]interfaces.WorkerConfig, len(configs))
	for i := range configs {
		out[i] = CloneWorkerConfig(configs[i])
	}
	return out
}

func cloneWorkstationConfigs(configs []interfaces.FactoryWorkstationConfig) []interfaces.FactoryWorkstationConfig {
	out := make([]interfaces.FactoryWorkstationConfig, len(configs))
	for i := range configs {
		out[i] = CloneWorkstationConfig(configs[i])
	}
	return out
}

func cloneIOConfigs(configs []interfaces.IOConfig) []interfaces.IOConfig {
	out := append([]interfaces.IOConfig(nil), configs...)
	for i := range out {
		out[i] = cloneIOConfig(configs[i])
	}
	return out
}

func cloneIOConfig(cfg interfaces.IOConfig) interfaces.IOConfig {
	cloned := cfg
	cloned.Guard = cloneInputGuardConfigPtr(cfg.Guard)
	return cloned
}

func cloneIOConfigPtr(cfg *interfaces.IOConfig) *interfaces.IOConfig {
	if cfg == nil {
		return nil
	}
	cloned := cloneIOConfig(*cfg)
	return &cloned
}

func cloneInputGuardConfigPtr(cfg *interfaces.InputGuardConfig) *interfaces.InputGuardConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	return &cloned
}

func cloneGuardConfigs(configs []interfaces.GuardConfig) []interfaces.GuardConfig {
	if len(configs) == 0 {
		return nil
	}
	out := append([]interfaces.GuardConfig(nil), configs...)
	for i := range out {
		out[i].MatchConfig = cloneGuardMatchConfigPtr(configs[i].MatchConfig)
	}
	return out
}

func cloneGuardMatchConfigPtr(cfg *interfaces.GuardMatchConfig) *interfaces.GuardMatchConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	return &cloned
}

func applyWorkerRuntimeDefinition(worker *interfaces.WorkerConfig, def *interfaces.WorkerConfig) {
	if worker == nil || def == nil {
		return
	}
	runtimeDef := CloneWorkerConfig(*def)
	if worker.Name == "" && runtimeDef.Name != "" {
		worker.Name = runtimeDef.Name
	}
	if runtimeDef.Type != "" {
		worker.Type = runtimeDef.Type
	}
	if runtimeDef.Model != "" {
		worker.Model = runtimeDef.Model
	}
	if runtimeDef.ModelProvider != "" {
		worker.ModelProvider = runtimeDef.ModelProvider
	}
	if runtimeDef.ExecutorProvider != "" {
		worker.ExecutorProvider = runtimeDef.ExecutorProvider
	}
	if runtimeDef.SessionID != "" {
		worker.SessionID = runtimeDef.SessionID
	}
	if runtimeDef.Command != "" {
		worker.Command = runtimeDef.Command
	}
	if len(runtimeDef.Args) > 0 {
		worker.Args = append([]string(nil), runtimeDef.Args...)
	}
	if len(runtimeDef.Resources) > 0 {
		worker.Resources = append([]interfaces.ResourceConfig(nil), runtimeDef.Resources...)
	}
	if runtimeDef.Concurrency != 0 {
		worker.Concurrency = runtimeDef.Concurrency
	}
	if runtimeDef.Timeout != "" {
		worker.Timeout = runtimeDef.Timeout
	}
	if runtimeDef.StopToken != "" {
		worker.StopToken = runtimeDef.StopToken
	}
	if runtimeDef.SkipPermissions {
		worker.SkipPermissions = true
	}
	if runtimeDef.Body != "" {
		worker.Body = runtimeDef.Body
	}
}

func hasInlineWorkstationRuntime(workstation interfaces.FactoryWorkstationConfig) bool {
	return workstationHasRuntimeFields(workstation)
}

func formatCanonicalFactoryJSON(data []byte, sourcePath string) ([]byte, error) {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		return nil, fmt.Errorf("format canonical factory config %s: %w", sourcePath, err)
	}
	formatted.WriteByte('\n')
	return formatted.Bytes(), nil
}

func readFactoryConfigSource(path string) ([]byte, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", fmt.Errorf("find factory config source %s: %w", path, err)
	}

	sourcePath := path
	if info.IsDir() {
		sourcePath = filepath.Join(path, interfaces.FactoryConfigFile)
	}

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, "", fmt.Errorf("read factory config %s: %w", sourcePath, err)
	}
	return data, sourcePath, nil
}

func readFactoryConfigExpansionSource(path string) ([]byte, string, string, error) {
	data, sourcePath, err := readFactoryConfigSource(path)
	if err != nil {
		return nil, "", "", err
	}

	targetDir := filepath.Dir(sourcePath)
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("find factory config target %s: %w", path, err)
	}
	if info.IsDir() {
		targetDir = path
	}
	return data, sourcePath, targetDir, nil
}

func writeExpandedFactoryLayout(sourceDir, targetDir string, cfg *interfaces.FactoryConfig, canonical []byte, sourcePath string) error {
	if _, err := preparePortableBundledFileWrites(targetDir, cfg); err != nil {
		return err
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

	if err := writeExpandedWorkerFiles(targetDir, cfg.Workers); err != nil {
		return err
	}
	if err := writeExpandedWorkstationFiles(targetDir, cfg.Workstations); err != nil {
		return err
	}
	if err := materializePortableBundledFiles(targetDir, cfg); err != nil {
		return err
	}
	if err := writeExpandedReferencedScripts(sourceDir, targetDir, cfg); err != nil {
		return err
	}
	return nil
}

func writeExpandedWorkerFiles(targetDir string, workerConfigs []interfaces.WorkerConfig) error {
	workersDir := filepath.Join(targetDir, interfaces.WorkersDir)
	if err := os.MkdirAll(workersDir, 0o755); err != nil {
		return fmt.Errorf("create workers directory %s: %w", workersDir, err)
	}

	configs := append([]interfaces.WorkerConfig(nil), workerConfigs...)
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Name < configs[j].Name
	})

	for _, workerCfg := range configs {
		segment, err := safeFactoryLayoutSegment("worker", workerCfg.Name)
		if err != nil {
			return err
		}
		workerDir := filepath.Join(workersDir, segment)
		if workerCfg.Type == "" {
			exists, err := agentsFileExists(workerDir)
			if err != nil {
				return fmt.Errorf("check worker %q AGENTS.md: %w", workerCfg.Name, err)
			}
			if exists {
				continue
			}
		}
		def := workerDefForExpansion(workerCfg)
		agents, err := renderAgentsMarkdown(workerFrontmatterForExpansion(def), def.Body)
		if err != nil {
			return fmt.Errorf("render worker %q AGENTS.md: %w", workerCfg.Name, err)
		}
		if err := writeAgentsFile(workerDir, agents); err != nil {
			return fmt.Errorf("write worker %q AGENTS.md: %w", workerCfg.Name, err)
		}
	}
	return nil
}

func writeExpandedWorkstationFiles(targetDir string, workstationConfigs []interfaces.FactoryWorkstationConfig) error {
	workstationsDir := filepath.Join(targetDir, interfaces.WorkstationsDir)
	if err := os.MkdirAll(workstationsDir, 0o755); err != nil {
		return fmt.Errorf("create workstations directory %s: %w", workstationsDir, err)
	}

	configs := append([]interfaces.FactoryWorkstationConfig(nil), workstationConfigs...)
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Name < configs[j].Name
	})

	for _, workstationCfg := range configs {
		segment, err := safeFactoryLayoutSegment("workstation", workstationCfg.Name)
		if err != nil {
			return err
		}
		workstationDir := filepath.Join(workstationsDir, segment)
		if !hasInlineWorkstationRuntime(workstationCfg) {
			exists, err := agentsFileExists(workstationDir)
			if err != nil {
				return fmt.Errorf("check workstation %q AGENTS.md: %w", workstationCfg.Name, err)
			}
			if exists {
				continue
			}
		}
		def, promptFileContent := workstationDefForExpansion(workstationCfg)
		agents, err := renderAgentsMarkdown(workstationFrontmatterForExpansion(def), def.Body)
		if err != nil {
			return fmt.Errorf("render workstation %q AGENTS.md: %w", workstationCfg.Name, err)
		}
		promptPath := ""
		if def.PromptFile != "" {
			promptPath, err = safePromptFilePath(workstationDir, def.PromptFile)
			if err != nil {
				return fmt.Errorf("resolve workstation %q prompt file: %w", workstationCfg.Name, err)
			}
		}
		if err := writeAgentsFile(workstationDir, agents); err != nil {
			return fmt.Errorf("write workstation %q AGENTS.md: %w", workstationCfg.Name, err)
		}
		if promptPath != "" {
			if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
				return fmt.Errorf("create workstation %q prompt directory: %w", workstationCfg.Name, err)
			}
			if err := os.WriteFile(promptPath, []byte(promptFileContent), 0o644); err != nil {
				return fmt.Errorf("write workstation %q prompt file: %w", workstationCfg.Name, err)
			}
		}
	}
	return nil
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
		if nextValueIsScriptPath {
			nextValueIsScriptPath = false
			if trimmed == "" || trimmed == "--" || strings.Contains(trimmed, "{{") || strings.Contains(trimmed, "}}") {
				continue
			}
			return normalizeFactoryRelativeScriptPath(trimmed)
		}
		if skipNextValue {
			skipNextValue = false
			continue
		}
		if trimmed == "" || strings.Contains(trimmed, "{{") || strings.Contains(trimmed, "}}") {
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

type interpreterArgFlagMode int

const (
	interpreterArgFlagIgnore interpreterArgFlagMode = iota
	interpreterArgFlagSkipNextValue
	interpreterArgFlagScriptPathValue
)

func interpreterFlagModeForArg(command, arg string) interpreterArgFlagMode {
	normalized := strings.ToLower(strings.TrimSpace(arg))
	if normalized == "" || !strings.HasPrefix(normalized, "-") || strings.Contains(normalized, "=") {
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

func runtimeWorkerDefinition(factoryDir string, worker interfaces.WorkerConfig, requireSplitDefinition bool) (*interfaces.WorkerConfig, error) {
	inlineWorker, err := workerConfigFromInlineConfig(&worker)
	if err != nil {
		return nil, fmt.Errorf("invalid inline worker definition")
	}

	if inlineWorker != nil {
		return inlineWorker, nil
	}

	segment, err := safeFactoryLayoutSegment("worker", worker.Name)
	if err != nil {
		return nil, err
	}
	workerDir := filepath.Join(factoryDir, interfaces.WorkersDir, segment)
	def, err := LoadWorkerConfig(workerDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if requireSplitDefinition {
				return nil, fmt.Errorf("inline factory definition is incomplete: worker %q is missing definition and no AGENTS.md was found", worker.Name)
			}
			return nil, nil
		}
		return nil, err
	}
	if def.Name == "" {
		def.Name = worker.Name
	}
	return def, nil
}

func runtimeWorkstationDefinition(factoryDir string, workstation interfaces.FactoryWorkstationConfig, requireSplitDefinition bool, loader WorkstationLoader) (*interfaces.FactoryWorkstationConfig, error) {
	if hasInlineWorkstationRuntime(workstation) {
		inlineDef, err := workstationRuntimeDefinitionFromInline(workstation)
		if err != nil {
			return nil, err
		}
		splitDef, err := splitWorkstationRuntimeDefinition(factoryDir, workstation, false, loader)
		if err != nil {
			return nil, err
		}
		return mergeRuntimeWorkstationDefinitions(inlineDef, splitDef)
	}

	return splitWorkstationRuntimeDefinition(factoryDir, workstation, requireSplitDefinition, loader)
}

func splitWorkstationRuntimeDefinition(factoryDir string, workstation interfaces.FactoryWorkstationConfig, requireSplitDefinition bool, loader WorkstationLoader) (*interfaces.FactoryWorkstationConfig, error) {
	if loader != nil {
		def, err := loader.Load(workstation.Name)
		if err != nil {
			return nil, err
		}
		if def != nil {
			return def, nil
		}
	}

	segment, err := safeFactoryLayoutSegment("workstation", workstation.Name)
	if err != nil {
		return nil, err
	}
	workstationDir := filepath.Join(factoryDir, interfaces.WorkstationsDir, segment)
	def, err := LoadWorkstationConfig(workstationDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if requireSplitDefinition {
				return nil, fmt.Errorf("inline factory definition is incomplete: workstation %q is missing definition and no AGENTS.md was found", workstation.Name)
			}
			return nil, nil
		}
		return nil, err
	}
	return def, nil
}

func mergeRuntimeWorkstationDefinitions(inlineDef, splitDef *interfaces.FactoryWorkstationConfig) (*interfaces.FactoryWorkstationConfig, error) {
	if inlineDef == nil {
		return splitDef, nil
	}
	if splitDef == nil {
		return inlineDef, nil
	}

	merged := CloneWorkstationConfig(*inlineDef)
	if err := applyWorkstationRuntimeDefinition(&merged, splitDef); err != nil {
		return nil, err
	}
	if inlineDef.Body == "" && splitDef.Body == inlineDef.PromptTemplate && merged.PromptTemplate == inlineDef.PromptTemplate {
		merged.Body = ""
	}
	return &merged, nil
}

func workerDefForExpansion(def interfaces.WorkerConfig) interfaces.WorkerConfig {
	if def.Type == "" {
		return interfaces.WorkerConfig{Type: interfaces.WorkerTypeModel}
	}

	return interfaces.WorkerConfig{
		Type:             def.Type,
		Model:            def.Model,
		ModelProvider:    def.ModelProvider,
		ExecutorProvider: def.ExecutorProvider,
		SessionID:        def.SessionID,
		Command:          def.Command,
		Args:             append([]string(nil), def.Args...),
		Resources:        append([]interfaces.ResourceConfig(nil), def.Resources...),
		Concurrency:      def.Concurrency,
		Timeout:          def.Timeout,
		StopToken:        def.StopToken,
		SkipPermissions:  def.SkipPermissions,
		Body:             def.Body,
	}
}

func workstationDefForExpansion(workstationCfg interfaces.FactoryWorkstationConfig) (interfaces.FactoryWorkstationConfig, string) {
	if !hasInlineWorkstationRuntime(workstationCfg) {
		def := interfaces.FactoryWorkstationConfig{
			Type:           interfaces.WorkstationTypeModel,
			WorkerTypeName: workstationCfg.WorkerTypeName,
			StopWords:      append([]string(nil), workstationCfg.StopWords...),
		}
		if workstationCfg.WorkerTypeName == "" {
			def.Type = interfaces.WorkstationTypeLogical
		}
		return def, ""
	}

	def := CloneWorkstationConfig(workstationCfg)
	if def.WorkerTypeName == "" {
		def.WorkerTypeName = workstationCfg.WorkerTypeName
	}
	normalizeCanonicalWorkstationRuntime(&def)

	promptFileContent := ""
	if def.PromptFile != "" {
		promptFileContent = def.PromptTemplate
		if promptFileContent == "" {
			promptFileContent = def.Body
		}
	} else if def.Body == "" {
		def.Body = def.PromptTemplate
	}
	return def, promptFileContent
}

func safeFactoryLayoutSegment(kind, name string) (string, error) {
	segment := strings.TrimSpace(name)
	if segment == "" {
		return "", fmt.Errorf("%s name is required for factory config layout", kind)
	}
	if filepath.IsAbs(segment) || filepath.VolumeName(segment) != "" || strings.ContainsAny(segment, `/\`) {
		return "", fmt.Errorf("%s name %q cannot contain path separators", kind, name)
	}
	if segment == "." || segment == ".." {
		return "", fmt.Errorf("%s name %q is not a valid directory name", kind, name)
	}
	return segment, nil
}

func safePromptFilePath(workstationDir, promptFile string) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(promptFile))
	if cleaned == "" || cleaned == "." {
		return "", fmt.Errorf("prompt file path is required")
	}
	if filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" {
		return "", fmt.Errorf("prompt file %q must be relative to the workstation directory", promptFile)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("prompt file %q cannot escape the workstation directory", promptFile)
	}
	return filepath.Join(workstationDir, cleaned), nil
}

type workerFrontmatter struct {
	Type             string                      `yaml:"type"`
	Model            string                      `yaml:"model,omitempty"`
	ModelProvider    string                      `yaml:"modelProvider,omitempty"`
	ExecutorProvider string                      `yaml:"executorProvider,omitempty"`
	Command          string                      `yaml:"command,omitempty"`
	Args             []string                    `yaml:"args,omitempty"`
	Resources        []interfaces.ResourceConfig `yaml:"resources,omitempty"`
	Timeout          string                      `yaml:"timeout,omitempty"`
	StopToken        string                      `yaml:"stopToken,omitempty"`
	SkipPermissions  bool                        `yaml:"skipPermissions,omitempty"`
}

type workstationFrontmatter struct {
	ID               string                       `yaml:"id,omitempty"`
	Name             string                       `yaml:"name,omitempty"`
	Kind             interfaces.WorkstationKind   `yaml:"kind,omitempty"`
	Type             string                       `yaml:"type,omitempty"`
	Worker           string                       `yaml:"worker,omitempty"`
	PromptFile       string                       `yaml:"promptFile,omitempty"`
	OutputSchema     string                       `yaml:"outputSchema,omitempty"`
	Limits           workstationLimitsFrontmatter `yaml:"limits,omitempty"`
	Cron             *cronFrontmatter             `yaml:"cron,omitempty"`
	Inputs           []ioFrontmatter              `yaml:"inputs,omitempty"`
	Outputs          []ioFrontmatter              `yaml:"outputs,omitempty"`
	OnContinue       *ioFrontmatter               `yaml:"onContinue,omitempty"`
	OnRejection      *ioFrontmatter               `yaml:"onRejection,omitempty"`
	OnFailure        *ioFrontmatter               `yaml:"onFailure,omitempty"`
	Resources        []interfaces.ResourceConfig  `yaml:"resources,omitempty"`
	Guards           []guardFrontmatter           `yaml:"guards,omitempty"`
	StopWords        []string                     `yaml:"stopWords,omitempty"`
	WorkingDirectory string                       `yaml:"workingDirectory,omitempty"`
	Worktree         string                       `yaml:"worktree,omitempty"`
	Env              map[string]string            `yaml:"env,omitempty"`
}

type workstationLimitsFrontmatter struct {
	MaxRetries       int    `yaml:"maxRetries,omitempty"`
	MaxExecutionTime string `yaml:"maxExecutionTime,omitempty"`
}

type cronFrontmatter struct {
	Schedule       string `yaml:"schedule,omitempty"`
	TriggerAtStart bool   `yaml:"triggerAtStart,omitempty"`
	Jitter         string `yaml:"jitter,omitempty"`
	ExpiryWindow   string `yaml:"expiryWindow,omitempty"`
}

type ioFrontmatter struct {
	WorkType string                 `yaml:"workType"`
	State    string                 `yaml:"state"`
	Guard    *inputGuardFrontmatter `yaml:"guard,omitempty"`
}

type inputGuardFrontmatter struct {
	Type        interfaces.GuardType `yaml:"type"`
	MatchInput  string               `yaml:"matchInput,omitempty"`
	ParentInput string               `yaml:"parentInput,omitempty"`
	SpawnedBy   string               `yaml:"spawnedBy,omitempty"`
}

type guardFrontmatter struct {
	Type        interfaces.GuardType         `yaml:"type"`
	Workstation string                       `yaml:"workstation,omitempty"`
	MaxVisits   int                          `yaml:"maxVisits,omitempty"`
	MatchConfig *interfaces.GuardMatchConfig `yaml:"matchConfig,omitempty"`
}

func workerFrontmatterForExpansion(def interfaces.WorkerConfig) workerFrontmatter {
	return workerFrontmatter{
		Type:             def.Type,
		Model:            def.Model,
		ModelProvider:    def.ModelProvider,
		ExecutorProvider: def.ExecutorProvider,
		Command:          def.Command,
		Args:             append([]string(nil), def.Args...),
		Resources:        append([]interfaces.ResourceConfig(nil), def.Resources...),
		Timeout:          def.Timeout,
		StopToken:        def.StopToken,
		SkipPermissions:  def.SkipPermissions,
	}
}

func workstationFrontmatterForExpansion(def interfaces.FactoryWorkstationConfig) workstationFrontmatter {
	rendered := workstationFrontmatter{
		ID:               def.ID,
		Name:             def.Name,
		Kind:             def.Kind,
		Type:             def.Type,
		Worker:           def.WorkerTypeName,
		PromptFile:       def.PromptFile,
		OutputSchema:     def.OutputSchema,
		Limits:           workstationLimitsFrontmatter{MaxRetries: def.Limits.MaxRetries, MaxExecutionTime: def.Limits.MaxExecutionTime},
		Inputs:           ioFrontmatterSlice(def.Inputs),
		Outputs:          ioFrontmatterSlice(def.Outputs),
		OnContinue:       ioFrontmatterPtr(def.OnContinue),
		OnRejection:      ioFrontmatterPtr(def.OnRejection),
		OnFailure:        ioFrontmatterPtr(def.OnFailure),
		Resources:        append([]interfaces.ResourceConfig(nil), def.Resources...),
		Guards:           guardFrontmatterSlice(def.Guards),
		StopWords:        append([]string(nil), def.StopWords...),
		WorkingDirectory: def.WorkingDirectory,
		Worktree:         def.Worktree,
		Env:              cloneStringMap(def.Env),
	}
	if def.Cron != nil {
		rendered.Cron = &cronFrontmatter{
			Schedule:       def.Cron.Schedule,
			TriggerAtStart: def.Cron.TriggerAtStart,
			Jitter:         def.Cron.Jitter,
			ExpiryWindow:   def.Cron.ExpiryWindow,
		}
	}
	return rendered
}

func ioFrontmatterSlice(configs []interfaces.IOConfig) []ioFrontmatter {
	if len(configs) == 0 {
		return nil
	}
	out := make([]ioFrontmatter, len(configs))
	for i := range configs {
		out[i] = ioFrontmatter{
			WorkType: configs[i].WorkTypeName,
			State:    configs[i].StateName,
			Guard:    inputGuardFrontmatterPtr(configs[i].Guard),
		}
	}
	return out
}

func ioFrontmatterPtr(cfg *interfaces.IOConfig) *ioFrontmatter {
	if cfg == nil {
		return nil
	}
	return &ioFrontmatter{
		WorkType: cfg.WorkTypeName,
		State:    cfg.StateName,
		Guard:    inputGuardFrontmatterPtr(cfg.Guard),
	}
}

func inputGuardFrontmatterPtr(cfg *interfaces.InputGuardConfig) *inputGuardFrontmatter {
	if cfg == nil {
		return nil
	}
	return &inputGuardFrontmatter{
		Type:        cfg.Type,
		MatchInput:  cfg.MatchInput,
		ParentInput: cfg.ParentInput,
		SpawnedBy:   cfg.SpawnedBy,
	}
}

func guardFrontmatterSlice(configs []interfaces.GuardConfig) []guardFrontmatter {
	if len(configs) == 0 {
		return nil
	}
	out := make([]guardFrontmatter, len(configs))
	for i := range configs {
		out[i] = guardFrontmatter{
			Type:        configs[i].Type,
			Workstation: configs[i].Workstation,
			MaxVisits:   configs[i].MaxVisits,
			MatchConfig: cloneGuardMatchConfigPtr(configs[i].MatchConfig),
		}
	}
	return out
}

func renderAgentsMarkdown(frontmatter any, body string) ([]byte, error) {
	frontmatterBytes, err := yaml.Marshal(frontmatter)
	if err != nil {
		return nil, err
	}

	var rendered strings.Builder
	rendered.WriteString("---\n")
	rendered.Write(frontmatterBytes)
	rendered.WriteString("---\n")
	if body != "" {
		rendered.WriteString("\n")
		rendered.WriteString(strings.TrimSpace(body))
		rendered.WriteString("\n")
	}
	return []byte(rendered.String()), nil
}

func writeAgentsFile(dir string, content []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	path := filepath.Join(dir, interfaces.FactoryAgentsFileName)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
