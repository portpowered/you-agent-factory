// backendsizecheck:ignore-file built-in catalog layout and packaged factory JSON remain co-located until dedicated config seams split.
// pkgmaintcheck:ignore-file-lines built-in catalog layout and packaged factory JSON remain co-located until dedicated config seams split.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
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
	if err := ApplySupportedPortableBundledFiles(factoryDir, factoryCfg, true, true); err != nil {
		return nil, fmt.Errorf("collect portable bundled files %s: %w", factoryDir, err)
	}
	if err := ApplySharedFactoryStarterWork(factoryDir, factoryCfg); err != nil {
		return nil, fmt.Errorf("collect shared factory starter work %s: %w", factoryDir, err)
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

// LayoutExpansionReport describes filesystem paths managed by a split-layout
// expansion without exposing file contents.
type LayoutExpansionReport struct {
	FactoryConfigPaths    int
	WorkerAgentPaths      int
	WorkstationAgentPaths int
	PromptPaths           int
	BundledReplacements   []PortableBundledFileReplacement
}

// ExpandFactoryConfigLayoutWithReport writes a split factory directory layout
// from a canonical factory.json file and reports any differing portable
// bundled files that were overwritten during materialization.
func ExpandFactoryConfigLayoutWithReport(path string) (string, []PortableBundledFileReplacement, error) {
	targetDir, report, err := ExpandFactoryConfigLayoutWithExpansionReport(path)
	return targetDir, report.BundledReplacements, err
}

// ExpandFactoryConfigLayoutWithExpansionReport writes a split factory directory
// layout and reports path counts by category for CLI diagnostics.
func ExpandFactoryConfigLayoutWithExpansionReport(path string) (string, LayoutExpansionReport, error) {
	if path == "" {
		return "", LayoutExpansionReport{}, fmt.Errorf("factory config path is required")
	}

	data, sourcePath, targetDir, err := readFactoryConfigExpansionSource(path)
	if err != nil {
		return "", LayoutExpansionReport{}, err
	}

	mapper := NewFactoryConfigMapper()
	factoryCfg, err := mapper.Expand(data)
	if err != nil {
		return "", LayoutExpansionReport{}, fmt.Errorf("parse factory config %s: %w", sourcePath, err)
	}
	if err := validatePortableBundledFilesForExpandOnPath(filepath.Dir(sourcePath), factoryCfg); err != nil {
		return "", LayoutExpansionReport{}, err
	}

	cfgForExpandedFiles, err := InlineRuntimeDefinitions(targetDir, factoryCfg, InlineRuntimeDefinitionOptions{})
	if err != nil {
		return "", LayoutExpansionReport{}, fmt.Errorf("load split runtime definitions for expand %s: %w", targetDir, err)
	}
	if cfgForExpandedFiles == nil {
		cfgForExpandedFiles = factoryCfg
	}
	authoredFactoryCfg, err := authoredFactoryConfigForExpandedLayout(cfgForExpandedFiles)
	if err != nil {
		return "", LayoutExpansionReport{}, fmt.Errorf("normalize authored factory config %s: %w", sourcePath, err)
	}
	canonical, err := mapper.Flatten(authoredFactoryCfg)
	if err != nil {
		return "", LayoutExpansionReport{}, fmt.Errorf("normalize factory config %s: %w", sourcePath, err)
	}

	report, err := writeFactorySplitLayout(targetDir, cfgForExpandedFiles, canonical, sourcePath, FactorySplitLayoutWriteOptions{
		SourceDir:                   filepath.Dir(sourcePath),
		CopyReferencedScripts:       true,
		OverwriteExistingSplitFiles: false,
	})
	if err != nil {
		return "", LayoutExpansionReport{}, err
	}
	return targetDir, report, nil
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

func applyWorkerRuntimeDefinition(worker *workerconfig.Config, def *workerconfig.Config) {
	if worker == nil || def == nil {
		return
	}
	runtimeDef := CloneWorkerConfig(*def)
	applyWorkerRuntimeIdentity(worker, runtimeDef)
	applyWorkerRuntimeExecution(worker, runtimeDef)
	applyWorkerRuntimeResources(worker, runtimeDef)
}

func applyWorkerRuntimeIdentity(worker *workerconfig.Config, runtimeDef workerconfig.Config) {
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
	if runtimeDef.ModelLocality != "" {
		worker.ModelLocality = runtimeDef.ModelLocality
	}
	if runtimeDef.ExecutorProvider != "" {
		worker.ExecutorProvider = runtimeDef.ExecutorProvider
	}
	if len(runtimeDef.Operations) > 0 {
		worker.Operations = cloneModelOperations(runtimeDef.Operations)
	}
	if runtimeDef.SessionID != "" {
		worker.SessionID = runtimeDef.SessionID
	}
}

func applyWorkerRuntimeExecution(worker *workerconfig.Config, runtimeDef workerconfig.Config) {
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
	if runtimeDef.OpenCodeAgent != "" {
		worker.OpenCodeAgent = runtimeDef.OpenCodeAgent
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

func applyWorkerRuntimeResources(worker *workerconfig.Config, runtimeDef workerconfig.Config) {
	if len(runtimeDef.Resources) > 0 {
		worker.Resources = append([]factoryresource.Config(nil), runtimeDef.Resources...)
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

// LoadAuthoredFactoryAPIFromPath reads the authored factory.json payload without
// flattening or usage-aware taxonomy projection so validate-only inspection
// preserves legacy aliases and explicit incompatible pairings.
func LoadAuthoredFactoryAPIFromPath(path string) (factoryapi.Factory, error) {
	data, sourcePath, err := readFactoryConfigSource(path)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	if err := ValidatePortableLayoutBoundaryJSON(data); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("validate authored factory config %s: %w", sourcePath, err)
	}
	var factory factoryapi.Factory
	if err := json.Unmarshal(data, &factory); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("parse factory config: %w", err)
	}
	return factory, nil
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

func runtimeWorkerDefinition(factoryDir string, worker workerconfig.Config, requireSplitDefinition bool) (*workerconfig.Config, error) {
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

func workerDefForExpansion(def workerconfig.Config) workerconfig.Config {
	if def.Type == "" {
		return workerconfig.Config{Type: interfaces.WorkerTypeModel}
	}

	return workerconfig.Config{
		Type:             def.Type,
		Provider:         def.Provider,
		Model:            def.Model,
		ModelProvider:    def.ModelProvider,
		ExecutorProvider: def.ExecutorProvider,
		SessionID:        def.SessionID,
		Command:          def.Command,
		Args:             append([]string(nil), def.Args...),
		Resources:        append([]factoryresource.Config(nil), def.Resources...),
		Concurrency:      def.Concurrency,
		Timeout:          def.Timeout,
		StopToken:        def.StopToken,
		SkipPermissions:  def.SkipPermissions,
		OpenCodeAgent:    def.OpenCodeAgent,
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

type factorySplitLayoutReplaceHooks struct {
	afterStageWrite func(stagingDir string) error
}

// PreparedFactoryLayoutPayload holds normalized factory state produced once from
// submitted JSON for split-layout validation and persist.
type PreparedFactoryLayoutPayload struct {
	Config    *interfaces.FactoryConfig
	Canonical []byte
}

// PrepareFactoryLayoutPayload normalizes factory JSON into expanded config (with
// inline runtime bodies for split writes), prunes stale layout references for
// save, and returns thin canonical factory.json bytes.
func PrepareFactoryLayoutPayload(segment string, payload []byte) (*PreparedFactoryLayoutPayload, error) {
	cfg, _, err := normalizeNamedFactoryPayload(segment, payload)
	if err != nil {
		return nil, err
	}
	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	factoryvalidation.PruneLayout(cfg, topology)
	authoredFactoryCfg, err := authoredFactoryConfigForExpandedLayout(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: normalize authored factory %q config: %w", ErrInvalidNamedFactory, segment, err)
	}
	mapper := NewFactoryConfigMapper()
	canonical, err := mapper.Flatten(authoredFactoryCfg)
	if err != nil {
		return nil, fmt.Errorf("%w: normalize factory %q config: %w", ErrInvalidNamedFactory, segment, err)
	}
	return &PreparedFactoryLayoutPayload{
		Config:    cfg,
		Canonical: canonical,
	}, nil
}

// FactoryLayoutReplaceOptions configures ReplaceFactoryLayoutAtDir.
type FactoryLayoutReplaceOptions struct {
	LayoutWrite FactorySplitLayoutWriteOptions
}

// DefaultFactoryLayoutReplaceOptions returns split-layout replace options for
// an existing factory directory at targetDir.
func DefaultFactoryLayoutReplaceOptions(targetDir string) FactoryLayoutReplaceOptions {
	return FactoryLayoutReplaceOptions{
		LayoutWrite: FactorySplitLayoutWriteOptions{
			SourceDir:                   strings.TrimSpace(targetDir),
			OverwriteExistingSplitFiles: true,
		},
	}
}

// FactorySplitLayoutReplaceResult holds rollback and backup-discard callbacks
// returned by ReplaceFactorySplitLayout after a successful commit.
type FactorySplitLayoutReplaceResult struct {
	Restore       func()
	DiscardBackup func()
}

// ReplaceFactoryLayoutAtDir atomically replaces targetDir from payload using
// normalizeNamedFactoryPayload, staging, writeFactorySplitLayout, LoadRuntimeConfig
// validation, and an atomic swap. restore reverts to the pre-replace directory
// tree when downstream activation fails.
// ReplaceFactoryLayoutAtDirWithResult runs the same pipeline as
// ReplaceFactoryLayoutAtDir and also returns DiscardBackup for callers that
// must remove the on-disk backup after successful downstream activation.
func ReplaceFactoryLayoutAtDirWithResult(targetDir string, payload []byte, opts FactoryLayoutReplaceOptions) (*FactorySplitLayoutReplaceResult, error) {
	return replaceFactoryLayoutAtDir(targetDir, payload, nil, opts, factorySplitLayoutReplaceHooks{})
}

// ReplaceFactoryLayoutAtDirWithPreparedWithResult atomically replaces targetDir
// using a pre-normalized payload so validation and split layout writes share the
// same expanded FactoryConfig and canonical factory.json bytes.
func ReplaceFactoryLayoutAtDirWithPreparedWithResult(
	targetDir string,
	prepared *PreparedFactoryLayoutPayload,
	opts FactoryLayoutReplaceOptions,
) (*FactorySplitLayoutReplaceResult, error) {
	if prepared == nil {
		return nil, fmt.Errorf("prepared factory layout payload is required")
	}
	return replaceFactoryLayoutAtDir(targetDir, nil, prepared, opts, factorySplitLayoutReplaceHooks{})
}

func ReplaceFactoryLayoutAtDir(targetDir string, payload []byte, opts FactoryLayoutReplaceOptions) (restore func(), err error) {
	result, err := ReplaceFactoryLayoutAtDirWithResult(targetDir, payload, opts)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Restore == nil {
		return func() {}, nil
	}
	return result.Restore, nil
}

// ReplaceFactoryLayoutAtDirWithAfterStageHook runs ReplaceFactoryLayoutAtDir and
// invokes afterStageWrite on the staged directory before validation. It exists for
// tests that assert validation failures leave the committed target unchanged.
func ReplaceFactoryLayoutAtDirWithAfterStageHook(
	targetDir string,
	payload []byte,
	opts FactoryLayoutReplaceOptions,
	afterStageWrite func(stagingDir string) error,
) (restore func(), err error) {
	hooks := factorySplitLayoutReplaceHooks{}
	if afterStageWrite != nil {
		hooks.afterStageWrite = afterStageWrite
	}
	result, err := replaceFactoryLayoutAtDir(targetDir, payload, nil, opts, hooks)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Restore == nil {
		return func() {}, nil
	}
	return result.Restore, nil
}

// ReplaceFactorySplitLayout atomically replaces an existing factory directory at
// targetDir with a split-layout materialization of canonicalFactoryJSON. Call
// Restore to reinstate the pre-replace tree when downstream steps fail; call
// DiscardBackup after successful activation to remove the on-disk backup.
func ReplaceFactorySplitLayout(targetDir string, canonicalFactoryJSON []byte) (*FactorySplitLayoutReplaceResult, error) {
	return ReplaceFactoryLayoutAtDirWithResult(targetDir, canonicalFactoryJSON, DefaultFactoryLayoutReplaceOptions(targetDir))
}

// ReplaceFactorySplitLayoutWithAfterStageHook runs split-layout replace and invokes
// afterStageWrite on the staged directory before validation. It exists for tests
// in pkg/config/splitreplacetests that assert validation failures leave disk unchanged.
func ReplaceFactorySplitLayoutWithAfterStageHook(
	targetDir string,
	canonicalFactoryJSON []byte,
	afterStageWrite func(stagingDir string) error,
) (*FactorySplitLayoutReplaceResult, error) {
	hooks := factorySplitLayoutReplaceHooks{}
	if afterStageWrite != nil {
		hooks.afterStageWrite = afterStageWrite
	}
	return replaceFactoryLayoutAtDir(targetDir, canonicalFactoryJSON, nil, DefaultFactoryLayoutReplaceOptions(targetDir), hooks)
}

func replaceFactoryLayoutAtDir(
	targetDir string,
	payload []byte,
	prepared *PreparedFactoryLayoutPayload,
	opts FactoryLayoutReplaceOptions,
	hooks factorySplitLayoutReplaceHooks,
) (*FactorySplitLayoutReplaceResult, error) {
	if strings.TrimSpace(targetDir) == "" {
		return nil, fmt.Errorf("factory directory is required")
	}
	if err := requireFactoryConfig(targetDir); err != nil {
		return nil, fmt.Errorf("replace factory layout at dir: %w", err)
	}

	segment := filepath.Base(targetDir)
	parentDir := filepath.Dir(targetDir)
	stagingDir, cleanupStaging, err := stageFactorySplitLayoutReplace(targetDir, segment, parentDir, payload, prepared, opts, hooks)
	if err != nil {
		return nil, err
	}
	defer cleanupStaging()

	backupDir, err := commitFactorySplitLayoutReplace(parentDir, targetDir, stagingDir, segment)
	if err != nil {
		return nil, err
	}

	return &FactorySplitLayoutReplaceResult{
		Restore: func() {
			restoreFactorySplitLayoutReplace(targetDir, backupDir)
		},
		DiscardBackup: func() {
			_ = os.RemoveAll(backupDir)
		},
	}, nil
}

func stageFactorySplitLayoutReplace(
	targetDir, segment, parentDir string,
	payload []byte,
	prepared *PreparedFactoryLayoutPayload,
	opts FactoryLayoutReplaceOptions,
	hooks factorySplitLayoutReplaceHooks,
) (stagingDir string, cleanup func(), err error) {
	var factoryCfg *interfaces.FactoryConfig
	var canonical []byte
	switch {
	case prepared != nil:
		factoryCfg = prepared.Config
		canonical = prepared.Canonical
	case len(payload) > 0:
		preparedPayload, err := PrepareFactoryLayoutPayload(segment, payload)
		if err != nil {
			return "", func() {}, err
		}
		factoryCfg = preparedPayload.Config
		canonical = preparedPayload.Canonical
	default:
		return "", func() {}, fmt.Errorf("factory layout payload is required")
	}

	sourcePath := filepath.Join(targetDir, interfaces.FactoryConfigFile)
	stagingDir, err = os.MkdirTemp(parentDir, "."+segment+".staging-")
	if err != nil {
		return "", func() {}, fmt.Errorf("create staging directory for factory %q: %w", segment, err)
	}
	cleanup = func() {
		_ = os.RemoveAll(stagingDir)
	}

	layoutWrite := opts.LayoutWrite
	if strings.TrimSpace(layoutWrite.SourceDir) == "" {
		layoutWrite.SourceDir = targetDir
	}
	if _, err := writeFactorySplitLayout(stagingDir, factoryCfg, canonical, sourcePath, layoutWrite); err != nil {
		return "", cleanup, fmt.Errorf("%w: %w", ErrInvalidNamedFactory, err)
	}
	if hooks.afterStageWrite != nil {
		if err := hooks.afterStageWrite(stagingDir); err != nil {
			return "", cleanup, fmt.Errorf("prepare staged factory %q: %w", segment, err)
		}
	}
	if _, err := LoadRuntimeConfig(stagingDir, nil); err != nil {
		return "", cleanup, fmt.Errorf("%w: validate factory %q config: %w", ErrInvalidNamedFactory, segment, err)
	}

	return stagingDir, cleanup, nil
}

func commitFactorySplitLayoutReplace(parentDir, targetDir, stagingDir, segment string) (backupDir string, err error) {
	backupDir, err = os.MkdirTemp(parentDir, "."+segment+".previous-")
	if err != nil {
		return "", fmt.Errorf("prepare replacement backup for factory %q: %w", segment, err)
	}
	if err := os.Remove(backupDir); err != nil {
		return "", fmt.Errorf("prepare replacement backup for factory %q: %w", segment, err)
	}

	if err := os.Rename(targetDir, backupDir); err != nil {
		if runtime.GOOS == "windows" {
			if replaceErr := replaceWatchedDirectoryContents(targetDir, stagingDir, backupDir); replaceErr == nil {
				return "", nil
			} else {
				return "", fmt.Errorf("backup existing factory %q: %w; Windows in-place replacement failed: %v", segment, err, replaceErr)
			}
		}
		return "", fmt.Errorf("backup existing factory %q: %w", segment, err)
	}
	committed := false
	defer func() {
		if !committed {
			if restoreErr := os.Rename(backupDir, targetDir); restoreErr != nil {
				return
			}
			_ = os.RemoveAll(backupDir)
		}
	}()

	if err := os.Rename(stagingDir, targetDir); err != nil {
		return "", fmt.Errorf("commit factory %q: %w", segment, err)
	}
	committed = true
	return backupDir, nil
}

func replaceWatchedDirectoryContents(targetDir, stagingDir, backupDir string) error {
	if err := os.CopyFS(backupDir, os.DirFS(targetDir)); err != nil {
		return fmt.Errorf("snapshot existing factory: %w", err)
	}
	if err := clearDirectoryContents(targetDir); err != nil {
		return err
	}
	if err := os.CopyFS(targetDir, os.DirFS(stagingDir)); err != nil {
		_ = clearDirectoryContents(targetDir)
		_ = os.CopyFS(targetDir, os.DirFS(backupDir))
		return fmt.Errorf("copy staged factory: %w", err)
	}
	if err := os.RemoveAll(stagingDir); err != nil {
		return fmt.Errorf("remove staged factory after commit: %w", err)
	}
	if err := os.RemoveAll(backupDir); err != nil {
		return fmt.Errorf("remove factory backup after commit: %w", err)
	}
	return nil
}

func clearDirectoryContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read directory for replacement: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return fmt.Errorf("remove existing factory entry %q: %w", entry.Name(), err)
		}
	}
	return nil
}

func restoreFactorySplitLayoutReplace(targetDir, backupDir string) {
	if strings.TrimSpace(targetDir) == "" || strings.TrimSpace(backupDir) == "" {
		return
	}
	if _, err := os.Stat(backupDir); err != nil {
		return
	}

	parentDir := filepath.Dir(targetDir)
	segment := filepath.Base(targetDir)
	trashDir, err := os.MkdirTemp(parentDir, "."+segment+".rollback-trash-")
	if err != nil {
		return
	}
	if err := os.Remove(trashDir); err != nil {
		return
	}

	if err := os.Rename(targetDir, trashDir); err != nil {
		_ = os.RemoveAll(trashDir)
		return
	}
	if err := os.Rename(backupDir, targetDir); err != nil {
		_ = os.Rename(trashDir, targetDir)
		return
	}
	_ = os.RemoveAll(trashDir)
}

// ResolveNamedFactoryDirAcrossRoots returns the runnable factory directory for
// name, checking the project-local root before the global root.
func ResolveNamedFactoryDirAcrossRoots(projectRoot, globalRoot, name string) (string, error) {
	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, name)
	if err != nil {
		return "", err
	}
	return resolution.FactoryDir, nil
}

// ResolveNamedFactoryAcrossRoots resolves name from projectRoot first and
// globalRoot second. It selects exactly one persisted factory directory and
// never merges definitions across roots.
func ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, name string) (*NamedFactoryResolution, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	globalRoot = strings.TrimSpace(globalRoot)
	if projectRoot == "" {
		return nil, fmt.Errorf("project factory root is required")
	}
	if globalRoot == "" {
		return nil, fmt.Errorf("global factory root is required")
	}

	canonicalName, err := canonicalNamedFactoryName(name)
	if err != nil {
		return nil, err
	}

	if factoryDir, found, err := resolveNamedFactoryCandidate(projectRoot, canonicalName); err != nil {
		return nil, err
	} else if found {
		return namedFactoryResolution(
			canonicalName,
			factoryDir,
			NamedFactoryResolutionSourceProjectLocal,
			projectRoot,
			globalRoot,
			projectLocalPrecedenceDecision(globalRoot, canonicalName),
		), nil
	}

	if factoryDir, found, err := resolveNamedFactoryCandidate(globalRoot, canonicalName); err != nil {
		return nil, err
	} else if found {
		return namedFactoryResolution(
			canonicalName,
			factoryDir,
			NamedFactoryResolutionSourceGlobal,
			projectRoot,
			globalRoot,
			NamedFactoryPrecedenceDecisionNone,
		), nil
	}

	return nil, fmt.Errorf(
		"resolve named factory %q in project root %s or global root %s: %w",
		canonicalName,
		projectRoot,
		globalRoot,
		newNamedFactoryNotFoundError(canonicalName),
	)
}

func canonicalNamedFactoryName(name string) (string, error) {
	segments, err := NamedFactoryPathSegments(name)
	if err != nil {
		return "", err
	}
	return NamedFactoryNameFromPathSegments(segments)
}

func resolveNamedFactoryCandidate(rootDir, name string) (string, bool, error) {
	factoryDir, err := ResolveNamedFactoryDir(rootDir, name)
	if err == nil {
		return factoryDir, true, nil
	}
	if errors.Is(err, ErrNamedFactoryNotFound) {
		return "", false, nil
	}
	return "", false, err
}
