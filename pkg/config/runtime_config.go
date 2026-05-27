package config

import (
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LoadedFactoryConfig is the effective runtime configuration assembled from
// factory.json plus any worker/workstation AGENTS.md definitions available on disk.
type LoadedFactoryConfig struct {
	factoryDir                  string
	runtimeBaseDir              string
	factory                     *interfaces.FactoryConfig
	lookup                      *runtimeDefinitionLookupMaps
	portableBundledReplacements []PortableBundledFileReplacement
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
	if err := applyRuntimeDefinitionsToClonedFactoryConfig(effectiveFactory, runtimeCfg); err != nil {
		return nil, err
	}

	loaded := &LoadedFactoryConfig{
		factoryDir: factoryDir,
		factory:    effectiveFactory,
		lookup:     newRuntimeDefinitionLookupMaps(len(effectiveFactory.Workers), len(effectiveFactory.Workstations)),
	}

	for i := range effectiveFactory.Workers {
		workerCopy := CloneWorkerConfig(effectiveFactory.Workers[i])
		loaded.lookup.workers[workerCopy.Name] = &workerCopy
	}

	for i := range effectiveFactory.Workstations {
		workstationCopy := CloneWorkstationConfig(effectiveFactory.Workstations[i])
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
	return LoadRuntimeConfigFromFactoryDir(resolvedFactoryDir, workstationLoader)
}

// LoadRuntimeConfigFromFactoryDir reads one concrete factory directory without
// following the current-factory pointer indirection used by workspace roots.
func LoadRuntimeConfigFromFactoryDir(factoryDir string, workstationLoader WorkstationLoader) (*LoadedFactoryConfig, error) {
	factoryCfg, err := loadFactoryConfig(factoryDir)
	if err != nil {
		return nil, err
	}
	replacements, err := materializePortableBundledFiles(factoryDir, factoryCfg)
	if err != nil {
		return nil, fmt.Errorf("materialize portable bundled files: %w", err)
	}
	if err := ApplySupportedPortableBundledFiles(factoryDir, factoryCfg, false); err != nil {
		return nil, fmt.Errorf("collect portable bundled files: %w", err)
	}
	inlineDefinitionsRequired := hasInlineRuntimeDefinitions(factoryCfg)
	runtimeDefs, err := loadRuntimeDefinitionLookupMapsFromFactoryConfig(factoryDir, factoryCfg, InlineRuntimeDefinitionOptions{
		RequireSplitDefinitions: inlineDefinitionsRequired,
		WorkstationLoader:       workstationLoader,
	})
	if err != nil {
		return nil, err
	}

	loaded, err := NewLoadedFactoryConfig(factoryDir, factoryCfg, runtimeDefs)
	if err != nil {
		return nil, err
	}
	loaded.portableBundledReplacements = clonePortableBundledFileReplacements(replacements)
	return loaded, nil
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

// PortableBundledFileReplacements returns bundled file target paths that were
// overwritten while materializing inline portable content during runtime load.
func (c *LoadedFactoryConfig) PortableBundledFileReplacements() []PortableBundledFileReplacement {
	if c == nil {
		return nil
	}
	return clonePortableBundledFileReplacements(c.portableBundledReplacements)
}

// WorkstationConfigs returns the effective workstation definitions by name.
func (c *LoadedFactoryConfig) WorkstationConfigs() map[string]*interfaces.FactoryWorkstationConfig {
	if c == nil || c.lookup == nil {
		return nil
	}
	return c.lookup.workstations
}

// Worker returns the loaded worker definition for the given configured worker name.
func (c *LoadedFactoryConfig) Worker(name string) (*interfaces.WorkerConfig, bool) {
	if c == nil {
		return nil, false
	}
	return c.lookup.Worker(name)
}

// Workstation returns the canonical loaded workstation entry for the given configured workstation name.
func (c *LoadedFactoryConfig) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	if c == nil {
		return nil, false
	}
	return c.lookup.Workstation(name)
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

var _ interfaces.RuntimeDefinitionLookup = (*runtimeDefinitionLookupMaps)(nil)

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
	if err := validatePortableResourceManifestOnPath(factoryDir, cfg); err != nil {
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
	if hasNonEmptyWorkerRuntimeStrings(worker) {
		return true
	}
	return len(worker.Args) > 0 ||
		len(worker.Resources) > 0 ||
		worker.Concurrency != 0 ||
		worker.SkipPermissions ||
		worker.Auth != nil ||
		worker.Linear != nil
}

func hasNonEmptyWorkerRuntimeStrings(worker interfaces.WorkerConfig) bool {
	for _, value := range []string{
		string(worker.Type),
		worker.Provider,
		worker.Model,
		worker.ModelProvider,
		worker.ExecutorProvider,
		worker.SessionID,
		worker.Command,
		worker.Timeout,
		worker.StopToken,
		worker.Body,
	} {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func workerConfigFromInlineConfig(def *interfaces.WorkerConfig) (*interfaces.WorkerConfig, error) {
	if def == nil {
		return nil, nil
	}
	if strings.TrimSpace(def.Type) == "" {
		return nil, nil
	}
	cloned := CloneWorkerConfig(*def)
	return &cloned, nil
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
		workstation.Runner != "" ||
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
		workstation.Runner != "" ||
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
		workstation.Runner == "" &&
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
	if runtimeDef.Operation != "" {
		workstation.Operation = runtimeDef.Operation
	}
	if runtimeDef.Runner != "" {
		workstation.Runner = runtimeDef.Runner
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
	if len(runtimeDef.OperationBindings) > 0 {
		workstation.OperationBindings = cloneModelOperationBindings(runtimeDef.OperationBindings)
	}
	if len(runtimeDef.OnContinue) > 0 {
		workstation.OnContinue = cloneIOConfigs(runtimeDef.OnContinue)
	}
	if len(runtimeDef.OnRejection) > 0 {
		workstation.OnRejection = cloneIOConfigs(runtimeDef.OnRejection)
	}
	if len(runtimeDef.OnFailure) > 0 {
		workstation.OnFailure = cloneIOConfigs(runtimeDef.OnFailure)
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

func loadRuntimeDefinitionLookupMapsFromFactoryConfig(factoryDir string, cfg *interfaces.FactoryConfig, opts InlineRuntimeDefinitionOptions) (*runtimeDefinitionLookupMaps, error) {
	if cfg == nil {
		return newRuntimeDefinitionLookupMaps(0, 0), nil
	}

	runtimeDefs := newRuntimeDefinitionLookupMaps(len(cfg.Workers), len(cfg.Workstations))

	for _, workstation := range cfg.Workstations {
		def, err := runtimeWorkstationDefinition(factoryDir, workstation, opts.RequireSplitDefinitions, opts.WorkstationLoader)
		if err != nil {
			return nil, fmt.Errorf("load workstation %q config: %w", workstation.Name, err)
		}
		if def != nil {
			runtimeDefs.workstations[workstation.Name] = def
		}
	}

	for _, worker := range cfg.Workers {
		def, err := runtimeWorkerDefinition(factoryDir, worker, opts.RequireSplitDefinitions)
		if err != nil {
			return nil, fmt.Errorf("load worker %q config: %w", worker.Name, err)
		}
		if def != nil {
			runtimeDefs.workers[worker.Name] = def
		}
	}

	return runtimeDefs, nil
}

// applyRuntimeDefinitionsToClonedFactoryConfig mutates a cloned factory config
// in place so runtime worker and workstation definitions follow one merge path.
func applyRuntimeDefinitionsToClonedFactoryConfig(cfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup) error {
	if cfg == nil {
		return nil
	}

	for i := range cfg.Workers {
		if runtimeCfg == nil {
			continue
		}
		def, ok := runtimeCfg.Worker(cfg.Workers[i].Name)
		if !ok || def == nil {
			continue
		}
		applyWorkerRuntimeDefinition(&cfg.Workers[i], def)
	}

	for i := range cfg.Workstations {
		normalizeCanonicalWorkstationRuntime(&cfg.Workstations[i])
		if runtimeCfg == nil {
			continue
		}
		def, ok := runtimeCfg.Workstation(cfg.Workstations[i].Name)
		if !ok || def == nil {
			continue
		}
		if err := applyWorkstationRuntimeDefinition(&cfg.Workstations[i], def); err != nil {
			return fmt.Errorf("normalize workstation %q config: %w", cfg.Workstations[i].Name, err)
		}
	}

	return nil
}

// NormalizeWorkstationExecutionLimit rewrites legacy workstation timeout
// authoring into the canonical limits.maxExecutionTime field and clears the
// retired top-level timeout field.
func NormalizeWorkstationExecutionLimit(cfg *interfaces.FactoryWorkstationConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.Limits.MaxExecutionTime) == "" && strings.TrimSpace(cfg.Timeout) != "" {
		cfg.Limits.MaxExecutionTime = cfg.Timeout
	}
	cfg.Timeout = ""
}

// WorkstationExecutionTimeout resolves the configured workstation execution
// timeout from the canonical execution limits field.
func WorkstationExecutionTimeout(cfg *interfaces.FactoryWorkstationConfig) (time.Duration, error) {
	if cfg == nil {
		return 0, nil
	}

	if strings.TrimSpace(cfg.Limits.MaxExecutionTime) != "" {
		timeout, err := time.ParseDuration(cfg.Limits.MaxExecutionTime)
		if err != nil {
			return 0, fmt.Errorf("invalid workstation limits.maxExecutionTime %q: %w", cfg.Limits.MaxExecutionTime, err)
		}
		if timeout > 0 {
			return timeout, nil
		}
	}

	return 0, nil
}

// ErrFactoryLayoutNotFound reports that a directory does not contain either a
// legacy single-factory layout or a named-factory current-pointer layout.
var ErrFactoryLayoutNotFound = errors.New("factory layout not found")

// ErrNamedFactoryAlreadyExists reports that the requested named-factory target
// already exists on disk.
var ErrNamedFactoryAlreadyExists = errors.New("named factory already exists")

// ErrInvalidNamedFactory reports that the submitted named-factory payload could
// not be normalized into a runnable named-factory layout.
var ErrInvalidNamedFactory = errors.New("invalid named factory")

// ValidateNamedFactoryName applies the canonical safe directory-segment rules
// used by the named-factory on-disk layout.
func ValidateNamedFactoryName(name string) error {
	_, err := safeFactoryLayoutSegment("factory", name)
	return err
}

// PersistNamedFactory materializes a compact canonical factory payload under a
// named subdirectory rooted at rootDir.
func PersistNamedFactory(rootDir, name string, canonicalFactoryJSON []byte) (string, error) {
	result, err := PersistNamedFactoryWithReport(rootDir, name, canonicalFactoryJSON)
	if err != nil {
		return "", err
	}
	return result.FactoryDir, nil
}

// NamedFactoryPersistResult reports the staged named-factory directory together
// with any bundled files that were overwritten while restoring inline portable
// content into the thin persisted layout.
type NamedFactoryPersistResult struct {
	FactoryDir                      string
	PortableBundledFileReplacements []PortableBundledFileReplacement
}

// PersistNamedFactoryWithReport materializes a compact canonical factory
// payload under a named subdirectory rooted at rootDir and reports any
// differing portable bundled files that were replaced on disk.
func PersistNamedFactoryWithReport(rootDir, name string, canonicalFactoryJSON []byte) (*NamedFactoryPersistResult, error) {
	return persistNamedFactory(rootDir, name, canonicalFactoryJSON, namedFactoryPersistOptions{}, namedFactoryPersistHooks{})
}

// ReplaceNamedFactory materializes a compact canonical factory payload and
// atomically replaces an existing named factory directory rooted at rootDir.
func ReplaceNamedFactory(rootDir, name string, canonicalFactoryJSON []byte) (string, error) {
	result, err := ReplaceNamedFactoryWithReport(rootDir, name, canonicalFactoryJSON)
	if err != nil {
		return "", err
	}
	return result.FactoryDir, nil
}

// ReplaceNamedFactoryWithReport is the replacement equivalent of
// PersistNamedFactoryWithReport. It uses the same staging and validation path
// as create, then swaps the staged layout into the existing named-factory slot.
func ReplaceNamedFactoryWithReport(rootDir, name string, canonicalFactoryJSON []byte) (*NamedFactoryPersistResult, error) {
	return persistNamedFactory(rootDir, name, canonicalFactoryJSON, namedFactoryPersistOptions{replaceExisting: true}, namedFactoryPersistHooks{})
}

type namedFactoryPersistOptions struct {
	replaceExisting bool
}

type namedFactoryPersistHooks struct {
	afterWrite        func(stagingDir string) error
	loadRuntimeConfig func(factoryDir string, workstationLoader WorkstationLoader) (*LoadedFactoryConfig, error)
}

func persistNamedFactory(rootDir, name string, canonicalFactoryJSON []byte, options namedFactoryPersistOptions, hooks namedFactoryPersistHooks) (*NamedFactoryPersistResult, error) {
	if strings.TrimSpace(rootDir) == "" {
		return nil, fmt.Errorf("factory root is required")
	}

	segment, err := safeFactoryLayoutSegment("factory", name)
	if err != nil {
		return nil, err
	}
	targetDir := filepath.Join(rootDir, segment)
	if err := validateNamedFactoryTarget(targetDir, segment, options); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("create factory root %s: %w", rootDir, err)
	}
	factoryCfg, canonical, err := normalizeNamedFactoryPayload(segment, canonicalFactoryJSON)
	if err != nil {
		return nil, err
	}

	sourcePath := filepath.Join(targetDir, interfaces.FactoryConfigFile)
	stagingDir, err := os.MkdirTemp(rootDir, "."+segment+".staging-")
	if err != nil {
		return nil, fmt.Errorf("create staging directory for factory %q: %w", segment, err)
	}
	keepStaging := false
	defer func() {
		if !keepStaging {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	replacements, err := writeNamedFactoryLayout(stagingDir, factoryCfg, canonical, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidNamedFactory, err)
	}
	if hooks.afterWrite != nil {
		if err := hooks.afterWrite(stagingDir); err != nil {
			return nil, fmt.Errorf("prepare staged factory %q: %w", segment, err)
		}
	}
	loadRuntimeConfig := hooks.loadRuntimeConfig
	if loadRuntimeConfig == nil {
		loadRuntimeConfig = LoadRuntimeConfig
	}
	if err := commitNamedFactoryLayout(rootDir, segment, stagingDir, targetDir, options, loadRuntimeConfig); err != nil {
		return nil, err
	}
	keepStaging = true
	return &NamedFactoryPersistResult{
		FactoryDir:                      targetDir,
		PortableBundledFileReplacements: clonePortableBundledFileReplacements(replacements),
	}, nil
}

func validateNamedFactoryTarget(targetDir, segment string, options namedFactoryPersistOptions) error {
	if _, err := os.Stat(targetDir); err == nil {
		if options.replaceExisting {
			return nil
		}
		return fmt.Errorf("%w: factory %q already exists", ErrNamedFactoryAlreadyExists, segment)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check existing factory %q: %w", segment, err)
	}
	if options.replaceExisting {
		return fmt.Errorf("replace factory %q: %w", segment, os.ErrNotExist)
	}
	return nil
}

func normalizeNamedFactoryPayload(segment string, canonicalFactoryJSON []byte) (*interfaces.FactoryConfig, []byte, error) {
	mapper := NewFactoryConfigMapper()
	factoryCfg, err := mapper.Expand(canonicalFactoryJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: parse factory %q config: %w", ErrInvalidNamedFactory, segment, err)
	}
	authoredFactoryCfg, err := authoredFactoryConfigForExpandedLayout(factoryCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: normalize authored factory %q config: %w", ErrInvalidNamedFactory, segment, err)
	}
	canonical, err := mapper.Flatten(authoredFactoryCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: normalize factory %q config: %w", ErrInvalidNamedFactory, segment, err)
	}
	return factoryCfg, canonical, nil
}

func commitNamedFactoryLayout(
	rootDir string,
	segment string,
	stagingDir string,
	targetDir string,
	options namedFactoryPersistOptions,
	loadRuntimeConfig func(factoryDir string, workstationLoader WorkstationLoader) (*LoadedFactoryConfig, error),
) error {
	if _, err := loadRuntimeConfig(stagingDir, nil); err != nil {
		return fmt.Errorf("%w: validate factory %q config: %w", ErrInvalidNamedFactory, segment, err)
	}
	if options.replaceExisting {
		return replaceNamedFactoryDir(rootDir, segment, stagingDir, targetDir)
	}
	if err := os.Rename(stagingDir, targetDir); err != nil {
		return fmt.Errorf("commit factory %q: %w", segment, err)
	}
	return nil
}

func replaceNamedFactoryDir(rootDir, segment, stagingDir, targetDir string) error {
	backupDir, err := os.MkdirTemp(rootDir, "."+segment+".previous-")
	if err != nil {
		return fmt.Errorf("prepare replacement backup for factory %q: %w", segment, err)
	}
	if err := os.Remove(backupDir); err != nil {
		return fmt.Errorf("prepare replacement backup for factory %q: %w", segment, err)
	}

	if err := os.Rename(targetDir, backupDir); err != nil {
		return fmt.Errorf("backup existing factory %q: %w", segment, err)
	}
	committed := false
	defer func() {
		if committed {
			_ = os.RemoveAll(backupDir)
		}
	}()

	if err := os.Rename(stagingDir, targetDir); err != nil {
		if restoreErr := os.Rename(backupDir, targetDir); restoreErr != nil {
			return fmt.Errorf("commit replacement factory %q: %w; restore failed: %w", segment, err, restoreErr)
		}
		return fmt.Errorf("commit replacement factory %q: %w", segment, err)
	}
	committed = true
	return nil
}

// ReadCurrentFactoryPointer returns the current named factory selected for the
// root directory's named-factory layout.
func ReadCurrentFactoryPointer(rootDir string) (string, error) {
	path := filepath.Join(rootDir, interfaces.CurrentFactoryPointerFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	segment, err := safeFactoryLayoutSegment("factory", string(data))
	if err != nil {
		return "", fmt.Errorf("read current factory pointer %s: %w", path, err)
	}
	return segment, nil
}

// WriteCurrentFactoryPointer persists the selected named factory for later
// restart-time resolution.
func WriteCurrentFactoryPointer(rootDir, name string) error {
	if strings.TrimSpace(rootDir) == "" {
		return fmt.Errorf("factory root is required")
	}

	segment, err := safeFactoryLayoutSegment("factory", name)
	if err != nil {
		return err
	}
	if err := requireFactoryConfig(filepath.Join(rootDir, segment)); err != nil {
		return fmt.Errorf("set current factory %q: %w", segment, err)
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return fmt.Errorf("create factory root %s: %w", rootDir, err)
	}

	path := filepath.Join(rootDir, interfaces.CurrentFactoryPointerFile)
	if err := os.WriteFile(path, []byte(segment+"\n"), 0o644); err != nil {
		return fmt.Errorf("write current factory pointer %s: %w", path, err)
	}
	return nil
}

// ResolveNamedFactoryDir returns the canonical on-disk directory for a
// persisted named factory rooted under rootDir.
func ResolveNamedFactoryDir(rootDir, name string) (string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return "", fmt.Errorf("factory root is required")
	}

	segment, err := safeFactoryLayoutSegment("factory", name)
	if err != nil {
		return "", err
	}

	factoryDir := filepath.Join(rootDir, segment)
	if err := requireFactoryConfig(factoryDir); err != nil {
		return "", fmt.Errorf("resolve factory %q: %w", segment, err)
	}
	return factoryDir, nil
}

// ResolveCurrentFactoryDir returns the directory that should be treated as the
// active runtime root. A persisted current-pointer layout takes precedence over
// a legacy single-factory root.
func ResolveCurrentFactoryDir(rootDir string) (string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return "", fmt.Errorf("factory root is required")
	}

	if name, err := ReadCurrentFactoryPointer(rootDir); err == nil {
		return ResolveNamedFactoryDir(rootDir, name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if err := requireFactoryConfig(rootDir); err == nil {
		return rootDir, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	return "", fmt.Errorf("resolve current factory in %s: %w", rootDir, ErrFactoryLayoutNotFound)
}

func writeNamedFactoryLayout(targetDir string, cfg *interfaces.FactoryConfig, canonical []byte, sourcePath string) ([]PortableBundledFileReplacement, error) {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, fmt.Errorf("create factory directory %s: %w", targetDir, err)
	}

	formatted, err := formatCanonicalFactoryJSON(canonical, sourcePath)
	if err != nil {
		return nil, err
	}
	factoryPath := filepath.Join(targetDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, formatted, 0o644); err != nil {
		return nil, fmt.Errorf("write canonical factory config %s: %w", factoryPath, err)
	}
	if err := writeExpandedWorkerFiles(targetDir, cfg.Workers); err != nil {
		return nil, err
	}
	if err := writeExpandedWorkstationFiles(targetDir, cfg.Workstations); err != nil {
		return nil, err
	}
	replacements, err := materializePortableBundledFiles(targetDir, cfg)
	if err != nil {
		return nil, err
	}
	inputsDir := filepath.Join(targetDir, interfaces.InputsDir)
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create inputs directory %s: %w", inputsDir, err)
	}
	if err := ensureDefaultInputChannelDirectories(targetDir, cfg); err != nil {
		return nil, err
	}
	return replacements, nil
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

func requireFactoryConfig(factoryDir string) error {
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	info, err := os.Stat(factoryPath)
	if err != nil {
		return fmt.Errorf("find factory config %s: %w", factoryPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("factory config %s is a directory", factoryPath)
	}
	return nil
}
