package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// LoadedFactoryConfig is the effective runtime configuration assembled from
// factory.json plus any worker/workstation AGENTS.md definitions available on disk.
type LoadedFactoryConfig struct {
	factoryDir     string
	runtimeBaseDir string
	factory        *interfaces.FactoryConfig
	lookup         *runtimeDefinitionLookupMaps
}

var _ interfaces.RuntimeConfigLookup = (*LoadedFactoryConfig)(nil)

// NewLoadedFactoryConfig builds the effective runtime configuration from a
// canonical factory config plus optional runtime-loaded definitions.
func NewLoadedFactoryConfig(factoryDir string, factoryCfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup) (*LoadedFactoryConfig, error) {
	if factoryCfg == nil {
		return &LoadedFactoryConfig{factoryDir: factoryDir}, nil
	}

	effectiveFactory, err := CloneFactoryConfig(factoryCfg)
	if err != nil {
		return nil, fmt.Errorf("clone factory config: %w", err)
	}

	loaded := &LoadedFactoryConfig{
		factoryDir: factoryDir,
		factory:    effectiveFactory,
		lookup:     newRuntimeDefinitionLookupMaps(len(effectiveFactory.Workers), len(effectiveFactory.Workstations)),
	}

	for i := range effectiveFactory.Workers {
		worker := CloneWorkerConfig(effectiveFactory.Workers[i])
		if runtimeCfg != nil {
			if def, ok := runtimeCfg.Worker(worker.Name); ok && def != nil {
				applyWorkerRuntimeDefinition(&worker, def)
			}
		}
		effectiveFactory.Workers[i] = worker
		workerCopy := CloneWorkerConfig(worker)
		loaded.lookup.workers[workerCopy.Name] = &workerCopy
	}

	for i := range effectiveFactory.Workstations {
		workstation := CloneWorkstationConfig(effectiveFactory.Workstations[i])
		normalizeCanonicalWorkstationRuntime(&workstation)
		if runtimeCfg != nil {
			if def, ok := runtimeCfg.Workstation(workstation.Name); ok && def != nil {
				if err := applyWorkstationRuntimeDefinition(&workstation, def); err != nil {
					return nil, fmt.Errorf("normalize workstation %q config: %w", workstation.Name, err)
				}
			}
		}
		effectiveFactory.Workstations[i] = workstation
		workstationCopy := CloneWorkstationConfig(workstation)
		loaded.lookup.workstations[workstationCopy.Name] = &workstationCopy
	}

	return loaded, nil
}

// LoadRuntimeConfig reads factory.json plus worker/workstation AGENTS.md files
// into a single runtime configuration object with stable lookup maps.
func LoadRuntimeConfig(factoryDir string, workstationLoader WorkstationLoader) (*LoadedFactoryConfig, error) {
	resolvedFactoryDir, err := ResolveCurrentFactoryDir(factoryDir)
	if err != nil {
		return nil, err
	}

	factoryCfg, err := loadFactoryConfig(resolvedFactoryDir)
	if err != nil {
		return nil, err
	}
	if err := materializePortableBundledFiles(resolvedFactoryDir, factoryCfg); err != nil {
		return nil, fmt.Errorf("materialize portable bundled files: %w", err)
	}
	runtimeDefs := newRuntimeDefinitionConfig(len(factoryCfg.Workers), len(factoryCfg.Workstations))

	inlineDefinitionsRequired := hasInlineRuntimeDefinitions(factoryCfg)
	for _, workstation := range factoryCfg.Workstations {
		def, err := runtimeWorkstationDefinition(resolvedFactoryDir, workstation, inlineDefinitionsRequired, workstationLoader)
		if err != nil {
			return nil, fmt.Errorf("load workstation %q config: %w", workstation.Name, err)
		}
		if def != nil {
			runtimeDefs.workstations[workstation.Name] = def
		}
	}

	for _, worker := range factoryCfg.Workers {
		def, err := runtimeWorkerDefinition(resolvedFactoryDir, worker, inlineDefinitionsRequired)
		if err != nil {
			return nil, fmt.Errorf("load worker %q config: %w", worker.Name, err)
		}
		if def == nil {
			continue
		}
		runtimeDefs.workers[worker.Name] = def
	}

	return NewLoadedFactoryConfig(resolvedFactoryDir, factoryCfg, runtimeDefs)
}

// FactoryDir returns the source directory used to load the factory config.
func (c *LoadedFactoryConfig) FactoryDir() string {
	if c == nil {
		return ""
	}
	return c.factoryDir
}

// RuntimeBaseDir returns the directory used to resolve relative runtime paths
// such as workstation workingDirectory values. It defaults to the loaded
// factory directory when no explicit runtime override is set.
func (c *LoadedFactoryConfig) RuntimeBaseDir() string {
	if c == nil {
		return ""
	}
	if c.runtimeBaseDir != "" {
		return c.runtimeBaseDir
	}
	return c.factoryDir
}

// SetRuntimeBaseDir overrides the directory used to resolve relative runtime
// execution paths without changing the authored factory source directory.
func (c *LoadedFactoryConfig) SetRuntimeBaseDir(dir string) {
	if c == nil {
		return
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		c.runtimeBaseDir = ""
		return
	}
	c.runtimeBaseDir = filepath.Clean(dir)
}

// FactoryConfig returns the effective factory config after runtime definitions
// have been merged onto the canonical topology.
func (c *LoadedFactoryConfig) FactoryConfig() *interfaces.FactoryConfig {
	if c == nil {
		return nil
	}
	return c.factory
}

// WorkstationConfigs returns the effective workstation definitions by name.
func (c *LoadedFactoryConfig) WorkstationConfigs() map[string]*interfaces.FactoryWorkstationConfig {
	lookup := c.runtimeDefinitionLookup()
	if lookup == nil {
		return nil
	}
	return lookup.workstations
}

// Worker returns the loaded worker definition for the given configured worker name.
func (c *LoadedFactoryConfig) Worker(name string) (*interfaces.WorkerConfig, bool) {
	return c.runtimeDefinitionLookup().Worker(name)
}

// Workstation returns the canonical loaded workstation entry for the given configured workstation name.
func (c *LoadedFactoryConfig) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	return c.runtimeDefinitionLookup().Workstation(name)
}

func (c *LoadedFactoryConfig) runtimeDefinitionLookup() *runtimeDefinitionLookupMaps {
	if c == nil {
		return nil
	}
	return c.lookup
}

type runtimeDefinitionLookupMaps struct {
	workers      map[string]*interfaces.WorkerConfig
	workstations map[string]*interfaces.FactoryWorkstationConfig
}

func (c *runtimeDefinitionLookupMaps) Worker(name string) (*interfaces.WorkerConfig, bool) {
	if c == nil {
		return nil, false
	}
	def, ok := c.workers[name]
	return def, ok
}

func (c *runtimeDefinitionLookupMaps) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	if c == nil {
		return nil, false
	}
	def, ok := c.workstations[name]
	return def, ok
}

type runtimeDefinitionConfig = runtimeDefinitionLookupMaps

var _ interfaces.RuntimeDefinitionLookup = (*runtimeDefinitionConfig)(nil)

func newRuntimeDefinitionConfig(workerCount, workstationCount int) *runtimeDefinitionConfig {
	return newRuntimeDefinitionLookupMaps(workerCount, workstationCount)
}

func newRuntimeDefinitionLookupMaps(workerCount, workstationCount int) *runtimeDefinitionLookupMaps {
	return &runtimeDefinitionLookupMaps{
		workers:      make(map[string]*interfaces.WorkerConfig, workerCount),
		workstations: make(map[string]*interfaces.FactoryWorkstationConfig, workstationCount),
	}
}

func loadFactoryConfig(factoryDir string) (*interfaces.FactoryConfig, error) {
	data, err := os.ReadFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		return nil, err
	}

	cfg, err := NewFactoryConfigMapper().Expand(data)
	if err != nil {
		return nil, err
	}
	if err := validatePortableResourceManifestOnPath(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func hasInlineRuntimeDefinitions(cfg *interfaces.FactoryConfig) bool {
	if cfg == nil {
		return false
	}

	for _, worker := range cfg.Workers {
		if workerHasInlineRuntimeDefinitionFields(worker) {
			return true
		}
	}
	for _, workstation := range cfg.Workstations {
		if workstationHasInlineRuntimeDefinitionFields(workstation) {
			return true
		}
	}
	return false
}

func workerHasInlineRuntimeDefinitionFields(worker interfaces.WorkerConfig) bool {
	return strings.TrimSpace(worker.Type) != "" ||
		worker.Model != "" ||
		worker.ModelProvider != "" ||
		worker.ExecutorProvider != "" ||
		worker.SessionID != "" ||
		worker.Command != "" ||
		len(worker.Args) > 0 ||
		len(worker.Resources) > 0 ||
		worker.Concurrency != 0 ||
		worker.Timeout != "" ||
		worker.StopToken != "" ||
		worker.SkipPermissions ||
		worker.Body != ""
}

func workerConfigFromInlineConfig(def *interfaces.WorkerConfig) (*interfaces.WorkerConfig, error) {
	if def == nil {
		return nil, nil
	}
	if strings.TrimSpace(def.Type) == "" {
		return nil, nil
	}
	return &interfaces.WorkerConfig{
		Name:             def.Name,
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
	}, nil
}

func workstationRuntimeDefinitionFromInline(workstation interfaces.FactoryWorkstationConfig) (*interfaces.FactoryWorkstationConfig, error) {
	if !workstationHasRuntimeFields(workstation) {
		return nil, nil
	}
	def := CloneWorkstationConfig(workstation)
	if strings.TrimSpace(def.Type) == "" {
		def.Type = defaultWorkstationRuntimeType(def.WorkerTypeName)
	}
	normalizeCanonicalWorkstationRuntime(&def)
	return &def, nil
}

func workstationHasRuntimeFields(workstation interfaces.FactoryWorkstationConfig) bool {
	return strings.TrimSpace(workstation.Type) != "" ||
		workstation.PromptFile != "" ||
		workstation.OutputSchema != "" ||
		workstation.Timeout != "" ||
		workstation.Limits.MaxRetries != 0 ||
		workstation.Limits.MaxExecutionTime != "" ||
		workstation.Body != "" ||
		workstation.PromptTemplate != "" ||
		workstation.WorkingDirectory != "" ||
		workstation.Worktree != "" ||
		len(workstation.Env) > 0
}

func workstationHasInlineRuntimeDefinitionFields(workstation interfaces.FactoryWorkstationConfig) bool {
	if isTopologyOnlyLogicalMoveLoopBreaker(workstation) {
		return false
	}
	return strings.TrimSpace(workstation.Type) != "" ||
		workstation.PromptFile != "" ||
		workstation.OutputSchema != "" ||
		workstation.Timeout != "" ||
		workstation.Limits.MaxRetries != 0 ||
		workstation.Limits.MaxExecutionTime != "" ||
		workstation.Body != "" ||
		workstation.PromptTemplate != "" ||
		workstation.WorkingDirectory != "" ||
		workstation.Worktree != "" ||
		len(workstation.Env) > 0
}

func isTopologyOnlyLogicalMoveLoopBreaker(workstation interfaces.FactoryWorkstationConfig) bool {
	return strings.TrimSpace(workstation.Type) == interfaces.WorkstationTypeLogical &&
		workstation.PromptFile == "" &&
		workstation.OutputSchema == "" &&
		workstation.Timeout == "" &&
		workstation.Limits.MaxRetries == 0 &&
		workstation.Limits.MaxExecutionTime == "" &&
		workstation.Body == "" &&
		workstation.PromptTemplate == "" &&
		workstation.WorkingDirectory == "" &&
		workstation.Worktree == "" &&
		len(workstation.Env) == 0
}

func applyWorkstationRuntimeDefinition(workstation *interfaces.FactoryWorkstationConfig, def *interfaces.FactoryWorkstationConfig) error {
	if workstation == nil || def == nil {
		return nil
	}
	normalizeCanonicalWorkstationRuntime(workstation)
	baseStopWords := append([]string(nil), workstation.StopWords...)
	runtimeDef := CloneWorkstationConfig(*def)
	if strings.TrimSpace(runtimeDef.Type) == "" && strings.TrimSpace(workstation.Type) == "" {
		runtimeDef.Type = defaultWorkstationRuntimeType(firstNonEmpty(runtimeDef.WorkerTypeName, workstation.WorkerTypeName))
	}
	normalizeCanonicalWorkstationRuntime(&runtimeDef)

	applyWorkstationRuntimeIdentity(workstation, runtimeDef)
	applyWorkstationRuntimeTopology(workstation, runtimeDef)
	applyWorkstationRuntimeTemplate(workstation, runtimeDef, baseStopWords)
	return nil
}

func applyWorkstationRuntimeIdentity(workstation *interfaces.FactoryWorkstationConfig, runtimeDef interfaces.FactoryWorkstationConfig) {
	if runtimeDef.ID != "" {
		workstation.ID = runtimeDef.ID
	}
	if runtimeDef.Name != "" && workstation.Name == "" {
		workstation.Name = runtimeDef.Name
	}
	if runtimeDef.Kind != "" {
		workstation.Kind = runtimeDef.Kind
	}
	if runtimeDef.Type != "" {
		workstation.Type = runtimeDef.Type
	}
	if runtimeDef.WorkerTypeName != "" {
		workstation.WorkerTypeName = runtimeDef.WorkerTypeName
	}
}

func applyWorkstationRuntimeTopology(workstation *interfaces.FactoryWorkstationConfig, runtimeDef interfaces.FactoryWorkstationConfig) {
	if runtimeDef.Cron != nil {
		cron := *runtimeDef.Cron
		workstation.Cron = &cron
	}
	if len(runtimeDef.Inputs) > 0 {
		workstation.Inputs = cloneIOConfigs(runtimeDef.Inputs)
	}
	if len(runtimeDef.Outputs) > 0 {
		workstation.Outputs = cloneIOConfigs(runtimeDef.Outputs)
	}
	if runtimeDef.OnContinue != nil {
		workstation.OnContinue = cloneIOConfigPtr(runtimeDef.OnContinue)
	}
	if runtimeDef.OnRejection != nil {
		workstation.OnRejection = cloneIOConfigPtr(runtimeDef.OnRejection)
	}
	if runtimeDef.OnFailure != nil {
		workstation.OnFailure = cloneIOConfigPtr(runtimeDef.OnFailure)
	}
	if len(runtimeDef.Resources) > 0 {
		workstation.Resources = append([]interfaces.ResourceConfig(nil), runtimeDef.Resources...)
	}
	if len(runtimeDef.Guards) > 0 {
		workstation.Guards = append([]interfaces.GuardConfig(nil), runtimeDef.Guards...)
	}
}

func applyWorkstationRuntimeTemplate(
	workstation *interfaces.FactoryWorkstationConfig,
	runtimeDef interfaces.FactoryWorkstationConfig,
	baseStopWords []string,
) {
	if runtimeDef.PromptFile != "" {
		workstation.PromptFile = runtimeDef.PromptFile
	}
	if runtimeDef.OutputSchema != "" {
		workstation.OutputSchema = runtimeDef.OutputSchema
	}
	workstation.Limits = mergeWorkstationLimits(workstation.Limits, runtimeDef.Limits)
	NormalizeWorkstationExecutionLimit(workstation)
	workstation.StopWords = mergeStopWords(baseStopWords, mergeStopWords(runtimeDef.StopWords, runtimeDef.RuntimeStopWords))
	if runtimeDef.Body != "" {
		workstation.Body = runtimeDef.Body
	}
	if runtimeDef.PromptTemplate != "" {
		workstation.PromptTemplate = runtimeDef.PromptTemplate
	}
	if runtimeDef.WorkingDirectory != "" {
		workstation.WorkingDirectory = runtimeDef.WorkingDirectory
	}
	if runtimeDef.Worktree != "" {
		workstation.Worktree = runtimeDef.Worktree
	}
	workstation.Env = mergeStringMap(workstation.Env, runtimeDef.Env)
}

func mergeStopWords(base []string, extra []string) []string {
	if len(base) == 0 {
		return append([]string(nil), extra...)
	}
	out := append([]string(nil), base...)
	seen := make(map[string]bool, len(base)+len(extra))
	for _, stopWord := range base {
		seen[stopWord] = true
	}
	for _, stopWord := range extra {
		if seen[stopWord] {
			continue
		}
		out = append(out, stopWord)
		seen[stopWord] = true
	}
	return out
}

func normalizeCanonicalWorkstationRuntime(workstation *interfaces.FactoryWorkstationConfig) {
	if workstation == nil {
		return
	}
	if workstation.PromptTemplate == "" {
		workstation.PromptTemplate = workstation.Body
	}
	NormalizeWorkstationExecutionLimit(workstation)
}

func defaultWorkstationRuntimeType(workerName string) string {
	if strings.TrimSpace(workerName) == "" {
		return interfaces.WorkstationTypeLogical
	}
	return interfaces.WorkstationTypeModel
}

func mergeWorkstationLimits(base, runtime interfaces.WorkstationLimits) interfaces.WorkstationLimits {
	merged := base
	if runtime.MaxRetries != 0 {
		merged.MaxRetries = runtime.MaxRetries
	}
	if runtime.MaxExecutionTime != "" {
		merged.MaxExecutionTime = runtime.MaxExecutionTime
	}
	return merged
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func mergeStringMap(base, runtime map[string]string) map[string]string {
	if len(base) == 0 {
		return cloneStringMap(runtime)
	}
	merged := cloneStringMap(base)
	for key, value := range runtime {
		merged[key] = value
	}
	return merged
}
