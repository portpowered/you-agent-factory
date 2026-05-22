package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	if err := ApplySupportedPortableBundledFiles(factoryDir, factoryCfg, true); err != nil {
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
	targetDir, _, err := ExpandFactoryConfigLayoutWithReport(path)
	return targetDir, err
}

// ExpandFactoryConfigLayoutWithReport writes a split factory directory layout
// from a canonical factory.json file and reports any differing portable
// bundled files that were overwritten during materialization.
func ExpandFactoryConfigLayoutWithReport(path string) (string, []PortableBundledFileReplacement, error) {
	if path == "" {
		return "", nil, fmt.Errorf("factory config path is required")
	}

	data, sourcePath, targetDir, err := readFactoryConfigExpansionSource(path)
	if err != nil {
		return "", nil, err
	}

	mapper := NewFactoryConfigMapper()
	factoryCfg, err := mapper.Expand(data)
	if err != nil {
		return "", nil, fmt.Errorf("parse factory config %s: %w", sourcePath, err)
	}
	if err := validatePortableBundledFilesForExpandOnPath(filepath.Dir(sourcePath), factoryCfg); err != nil {
		return "", nil, err
	}

	cfgForExpandedFiles, err := InlineRuntimeDefinitions(targetDir, factoryCfg, InlineRuntimeDefinitionOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("load split runtime definitions for expand %s: %w", targetDir, err)
	}
	if cfgForExpandedFiles == nil {
		cfgForExpandedFiles = factoryCfg
	}
	authoredFactoryCfg, err := authoredFactoryConfigForExpandedLayout(cfgForExpandedFiles)
	if err != nil {
		return "", nil, fmt.Errorf("normalize authored factory config %s: %w", sourcePath, err)
	}
	canonical, err := mapper.Flatten(authoredFactoryCfg)
	if err != nil {
		return "", nil, fmt.Errorf("normalize factory config %s: %w", sourcePath, err)
	}

	replacements, err := writeExpandedFactoryLayout(filepath.Dir(sourcePath), targetDir, cfgForExpandedFiles, canonical, sourcePath)
	if err != nil {
		return "", nil, err
	}
	return targetDir, replacements, nil
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

	runtimeDefs, err := loadRuntimeDefinitionLookupMapsFromFactoryConfig(factoryDir, cfg, opts)
	if err != nil {
		return nil, err
	}
	if err := applyRuntimeDefinitionsToClonedFactoryConfig(inlined, runtimeDefs); err != nil {
		return nil, err
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

	if err := applyRuntimeDefinitionsToClonedFactoryConfig(inlined, runtimeCfg); err != nil {
		return nil, err
	}
	return inlined, nil
}

// CloneFactoryConfig deep-copies a factory config through explicit field copies.
func CloneFactoryConfig(cfg *interfaces.FactoryConfig) (*interfaces.FactoryConfig, error) {
	if cfg == nil {
		return nil, nil
	}
	cloned := &interfaces.FactoryConfig{
		Name:             cfg.Name,
		Project:          cfg.Project,
		Guards:           cloneFactoryGuardConfigs(cfg.Guards),
		InputTypes:       cloneInputTypeConfigs(cfg.InputTypes),
		WorkTypes:        cloneWorkTypeConfigs(cfg.WorkTypes),
		Resources:        cloneResourceConfigs(cfg.Resources),
		ResourceManifest: clonePortableResourceManifestConfig(cfg.ResourceManifest),
		Workers:          cloneWorkerConfigs(cfg.Workers),
		Workstations:     cloneWorkstationConfigs(cfg.Workstations),
	}
	return cloned, nil
}

func cloneFactoryGuardConfigs(configs []interfaces.FactoryGuardConfig) []interfaces.FactoryGuardConfig {
	if len(configs) == 0 {
		return nil
	}
	cloned := make([]interfaces.FactoryGuardConfig, len(configs))
	copy(cloned, configs)
	return cloned
}

// CloneWorkerConfig returns a copy of a worker runtime definition.
func CloneWorkerConfig(def interfaces.WorkerConfig) interfaces.WorkerConfig {
	def.Args = append([]string(nil), def.Args...)
	def.Resources = append([]interfaces.ResourceConfig(nil), def.Resources...)
	if def.Auth != nil {
		auth := *def.Auth
		def.Auth = &auth
	}
	if def.Linear != nil {
		linear := *def.Linear
		linear.TeamIDs = append([]string(nil), def.Linear.TeamIDs...)
		linear.StateIDs = append([]string(nil), def.Linear.StateIDs...)
		if def.Linear.Claim != nil {
			claim := *def.Linear.Claim
			linear.Claim = &claim
		}
		def.Linear = &linear
	}
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
	def.OnContinue = cloneIOConfigs(def.OnContinue)
	def.OnRejection = cloneIOConfigs(def.OnRejection)
	def.OnFailure = cloneIOConfigs(def.OnFailure)
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
	applyWorkerRuntimeIdentity(worker, runtimeDef)
	applyWorkerRuntimeExecution(worker, runtimeDef)
	applyWorkerRuntimeResources(worker, runtimeDef)
}

func applyWorkerRuntimeIdentity(worker *interfaces.WorkerConfig, runtimeDef interfaces.WorkerConfig) {
	if worker.Name == "" && runtimeDef.Name != "" {
		worker.Name = runtimeDef.Name
	}
	if runtimeDef.Type != "" {
		worker.Type = runtimeDef.Type
	}
	if runtimeDef.Provider != "" {
		worker.Provider = runtimeDef.Provider
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
}

func applyWorkerRuntimeExecution(worker *interfaces.WorkerConfig, runtimeDef interfaces.WorkerConfig) {
	if runtimeDef.Command != "" {
		worker.Command = runtimeDef.Command
	}
	if len(runtimeDef.Args) > 0 {
		worker.Args = append([]string(nil), runtimeDef.Args...)
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
	if runtimeDef.Auth != nil {
		worker.Auth = cloneHostedWorkerAuthConfig(runtimeDef.Auth)
	}
	if runtimeDef.Linear != nil {
		worker.Linear = cloneHostedLinearWorkerConfig(runtimeDef.Linear)
	}
	if runtimeDef.Body != "" {
		worker.Body = runtimeDef.Body
	}
}

func applyWorkerRuntimeResources(worker *interfaces.WorkerConfig, runtimeDef interfaces.WorkerConfig) {
	if len(runtimeDef.Resources) > 0 {
		worker.Resources = append([]interfaces.ResourceConfig(nil), runtimeDef.Resources...)
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

func runtimeWorkerDefinition(factoryDir string, worker interfaces.WorkerConfig, requireSplitDefinition bool) (*interfaces.WorkerConfig, error) {
	inlineWorker, err := workerConfigFromInlineConfig(&worker)
	if err != nil {
		return nil, fmt.Errorf("invalid inline worker definition")
	}

	if inlineWorker != nil {
		segment, err := safeFactoryLayoutSegment("worker", worker.Name)
		if err != nil {
			return nil, err
		}
		workerDir := filepath.Join(factoryDir, interfaces.WorkersDir, segment)
		body, bodyFound, err := loadWorkerBody(workerDir)
		if err != nil {
			return nil, err
		}
		if bodyFound {
			inlineWorker.Body = body
		} else if requireSplitDefinition && strings.TrimSpace(inlineWorker.Body) == "" && splitRuntimeEntityDirExists(workerDir) {
			return nil, fmt.Errorf("worker %q is missing body-only AGENTS.md content required by the split authored layout", worker.Name)
		}
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
		segment, err := safeFactoryLayoutSegment("workstation", workstation.Name)
		if err != nil {
			return nil, err
		}
		workstationDir := filepath.Join(factoryDir, interfaces.WorkstationsDir, segment)
		splitDef, err := splitWorkstationRuntimeDefinition(factoryDir, workstation, false, loader)
		if err != nil {
			return nil, err
		}
		if splitDef == nil && requireSplitDefinition && strings.TrimSpace(inlineDef.Body) == "" && splitRuntimeEntityDirExists(workstationDir) {
			return nil, fmt.Errorf("workstation %q is missing body-only AGENTS.md content required by the split authored layout", workstation.Name)
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
	if hasInlineWorkstationRuntime(workstation) {
		def, found, err := inlineBodyOnlyWorkstationRuntimeDefinition(workstationDir, workstation)
		if err != nil {
			return nil, err
		}
		if found {
			return def, nil
		}
	}
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

func inlineBodyOnlyWorkstationRuntimeDefinition(workstationDir string, workstation interfaces.FactoryWorkstationConfig) (*interfaces.FactoryWorkstationConfig, bool, error) {
	body, found, err := loadWorkstationBody(workstationDir)
	if err != nil || !found {
		return nil, found, err
	}

	def, err := workstationRuntimeDefinitionFromInline(workstation)
	if err != nil {
		return nil, false, err
	}
	if def == nil {
		return nil, false, nil
	}

	def.Body = body
	if def.PromptFile != "" {
		promptTemplate, err := loadWorkstationPromptTemplate(workstationDir, def.PromptFile)
		if err != nil {
			return nil, false, err
		}
		def.PromptTemplate = promptTemplate
	} else {
		def.PromptTemplate = body
	}
	return def, true, nil
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
		Provider:         def.Provider,
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
		Auth:             cloneHostedWorkerAuthConfig(def.Auth),
		Linear:           cloneHostedLinearWorkerConfig(def.Linear),
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

func splitRuntimeEntityDirExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

type workerFrontmatter struct {
	Type             string                               `yaml:"type"`
	Provider         string                               `yaml:"provider,omitempty"`
	Model            string                               `yaml:"model,omitempty"`
	ModelProvider    string                               `yaml:"modelProvider,omitempty"`
	ExecutorProvider string                               `yaml:"executorProvider,omitempty"`
	Command          string                               `yaml:"command,omitempty"`
	Args             []string                             `yaml:"args,omitempty"`
	Resources        []interfaces.ResourceConfig          `yaml:"resources,omitempty"`
	Timeout          string                               `yaml:"timeout,omitempty"`
	StopToken        string                               `yaml:"stopToken,omitempty"`
	SkipPermissions  bool                                 `yaml:"skipPermissions,omitempty"`
	Auth             *interfaces.HostedWorkerAuthConfig   `yaml:"auth,omitempty"`
	Linear           *interfaces.HostedLinearWorkerConfig `yaml:"linear,omitempty"`
}

type workstationFrontmatter struct {
	ID               string                       `yaml:"id,omitempty"`
	Name             string                       `yaml:"name,omitempty"`
	Kind             interfaces.WorkstationKind   `yaml:"behavior,omitempty"`
	Type             string                       `yaml:"type,omitempty"`
	Worker           string                       `yaml:"worker,omitempty"`
	PromptFile       string                       `yaml:"promptFile,omitempty"`
	OutputSchema     string                       `yaml:"outputSchema,omitempty"`
	Limits           workstationLimitsFrontmatter `yaml:"limits,omitempty"`
	Cron             *cronFrontmatter             `yaml:"cron,omitempty"`
	Inputs           []ioFrontmatter              `yaml:"inputs,omitempty"`
	Outputs          []ioFrontmatter              `yaml:"outputs,omitempty"`
	OnContinue       []ioFrontmatter              `yaml:"onContinue,omitempty"`
	OnRejection      []ioFrontmatter              `yaml:"onRejection,omitempty"`
	OnFailure        []ioFrontmatter              `yaml:"onFailure,omitempty"`
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

func workstationFrontmatterForExpansion(def interfaces.FactoryWorkstationConfig) workstationFrontmatter {
	behavior := def.Kind
	if def.Kind != "" {
		behavior = interfaces.WorkstationKind(publicFactoryWorkstationKindFromInternal(def.Kind))
	}
	rendered := workstationFrontmatter{
		ID:               def.ID,
		Name:             def.Name,
		Kind:             behavior,
		Type:             def.Type,
		Worker:           def.WorkerTypeName,
		PromptFile:       def.PromptFile,
		OutputSchema:     def.OutputSchema,
		Limits:           workstationLimitsFrontmatter{MaxRetries: def.Limits.MaxRetries, MaxExecutionTime: def.Limits.MaxExecutionTime},
		Inputs:           ioFrontmatterSlice(def.Inputs),
		Outputs:          ioFrontmatterSlice(def.Outputs),
		OnContinue:       ioFrontmatterSlice(def.OnContinue),
		OnRejection:      ioFrontmatterSlice(def.OnRejection),
		OnFailure:        ioFrontmatterSlice(def.OnFailure),
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

func workerFrontmatterForExpansion(def interfaces.WorkerConfig) workerFrontmatter {
	modelProvider := def.ModelProvider
	if def.ModelProvider != "" {
		modelProvider = string(publicFactoryWorkerModelProviderFromInternal(def.ModelProvider))
	}
	executorProvider := def.ExecutorProvider
	if def.ExecutorProvider != "" {
		executorProvider = string(publicFactoryWorkerProviderFromInternal(def.ExecutorProvider))
	}
	return workerFrontmatter{
		Type:             def.Type,
		Provider:         publicFactoryHostedWorkerProviderFromInternal(def.Provider),
		Model:            def.Model,
		ModelProvider:    modelProvider,
		ExecutorProvider: executorProvider,
		Command:          def.Command,
		Args:             append([]string(nil), def.Args...),
		Resources:        append([]interfaces.ResourceConfig(nil), def.Resources...),
		Timeout:          def.Timeout,
		StopToken:        def.StopToken,
		SkipPermissions:  def.SkipPermissions,
		Auth:             cloneHostedWorkerAuthConfig(def.Auth),
		Linear:           cloneHostedLinearWorkerConfig(def.Linear),
	}
}
func inputGuardFrontmatterPtr(cfg *interfaces.InputGuardConfig) *inputGuardFrontmatter {
	if cfg == nil {
		return nil
	}
	return &inputGuardFrontmatter{
		Type:        interfaces.GuardType(publicFactoryGuardTypeStringFromInternal(cfg.Type)),
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
			Type:        interfaces.GuardType(publicFactoryGuardTypeStringFromInternal(configs[i].Type)),
			Workstation: configs[i].Workstation,
			MaxVisits:   configs[i].MaxVisits,
			MatchConfig: cloneGuardMatchConfigPtr(configs[i].MatchConfig),
		}
	}
	return out
}

func authoredFactoryConfigForExpandedLayout(cfg *interfaces.FactoryConfig) (*interfaces.FactoryConfig, error) {
	authored, err := CloneFactoryConfig(cfg)
	if err != nil {
		return nil, err
	}
	for i := range authored.Workers {
		authored.Workers[i].Body = ""
	}
	for i := range authored.Workstations {
		authored.Workstations[i].Body = ""
		authored.Workstations[i].PromptTemplate = ""
	}
	if authored.ResourceManifest != nil {
		for i := range authored.ResourceManifest.BundledFiles {
			if !shouldOmitSupportedPortableBundledInline(authored.ResourceManifest.BundledFiles[i]) {
				continue
			}
			authored.ResourceManifest.BundledFiles[i].Content.Inline = ""
		}
	}
	return authored, nil
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

func renderAgentsBody(body string) []byte {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return []byte{}
	}
	return []byte(trimmed + "\n")
}

func loadWorkerBody(dir string) (string, bool, error) {
	agentsPath := filepath.Join(dir, interfaces.FactoryAgentsFileName)
	body, err := loadAgentsBody(agentsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("load worker body from %s: %w", dir, err)
	}
	return body, true, nil
}

func loadWorkstationBody(dir string) (string, bool, error) {
	agentsPath := filepath.Join(dir, interfaces.FactoryAgentsFileName)
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("load workstation body from %s: %w", dir, err)
	}

	content := string(data)
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		return "", false, nil
	}

	return strings.TrimSpace(content), true, nil
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
