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
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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

// LoadAuthoredFactoryAPIFromPath reads the authored factory.json payload without
// flattening or usage-aware taxonomy projection so validate-only inspection
// preserves legacy aliases and explicit incompatible pairings.
func LoadAuthoredFactoryAPIFromPath(path string) (factoryapi.Factory, error) {
	data, _, err := readFactoryConfigSource(path)
	if err != nil {
		return factoryapi.Factory{}, err
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

// DefaultFactoryLayoutReplaceOptions returns persist-from-save layout options for
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

var builtInNamedFactoryCatalog = map[string][]byte{
	"@you/goal": BuiltInGoalFactoryJSON,
	"@you/tts":  BuiltInTTSFactoryJSON,
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

	if factoryDir, materialized, err := resolveBuiltInNamedFactory(globalRoot, canonicalName); err != nil {
		return nil, err
	} else if materialized {
		return namedFactoryResolution(
			canonicalName,
			factoryDir,
			NamedFactoryResolutionSourceBuiltin,
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
	segment, err := NamedFactoryNameToLayoutSegment(name)
	if err != nil {
		return "", err
	}
	return NamedFactoryLayoutSegmentToName(segment)
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

func resolveBuiltInNamedFactory(globalRoot, canonicalName string) (string, bool, error) {
	payload, ok := builtInNamedFactoryCatalog[canonicalName]
	if !ok {
		return "", false, nil
	}

	segment, err := NamedFactoryNameToLayoutSegment(canonicalName)
	if err != nil {
		return "", false, err
	}
	targetDir := filepath.Join(globalRoot, segment)
	if _, err := os.Stat(targetDir); err == nil {
		if err := requireFactoryConfig(targetDir); err != nil {
			return "", false, fmt.Errorf("materialize built-in named factory %q in global root %s: existing target invalid: %w", canonicalName, globalRoot, err)
		}
		return targetDir, true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("materialize built-in named factory %q in global root %s: check existing target: %w", canonicalName, globalRoot, err)
	}

	factoryDir, err := PersistNamedFactory(globalRoot, canonicalName, payload)
	if err != nil {
		return "", false, fmt.Errorf("materialize built-in named factory %q in global root %s: %w", canonicalName, globalRoot, err)
	}
	return factoryDir, true, nil
}

// BuiltInGoalFactoryJSON is the canonical runnable @you/goal packaged factory payload.
var BuiltInGoalFactoryJSON = []byte(`{
  "name": "@you/goal",
  "id": "builtin-goal",
  "workTypes": [
    {
      "name": "goal",
      "handlingBehavior": ["DEFAULT"],
      "states": [
        {"name": "init", "type": "INITIAL"},
        {"name": "plan", "type": "PROCESSING"},
        {"name": "execute", "type": "PROCESSING"},
        {"name": "check", "type": "PROCESSING"},
        {"name": "review", "type": "PROCESSING"},
        {"name": "summarize", "type": "PROCESSING"},
        {"name": "complete", "type": "TERMINAL"},
        {"name": "blocked", "type": "PROCESSING"},
        {"name": "needs-human", "type": "PROCESSING"},
        {"name": "interrupted", "type": "PROCESSING"},
        {"name": "failed", "type": "FAILED"}
      ]
    }
  ],
  "workers": [
    {
      "name": "goal-planner",
      "type": "AGENT_WORKER",
      "body": "Planner worker for @you/goal."
    },
    {
      "name": "goal-executor",
      "type": "AGENT_WORKER",
      "body": "Executor worker for @you/goal."
    },
    {
      "name": "goal-checker",
      "type": "SCRIPT_WORKER",
      "command": "make",
      "args": ["test"],
      "body": "Checker worker for @you/goal."
    },
    {
      "name": "goal-reviewer",
      "type": "AGENT_WORKER",
      "body": "Reviewer worker for @you/goal."
    },
    {
      "name": "goal-summarizer",
      "type": "AGENT_WORKER",
      "body": "Summarizer worker for @you/goal."
    }
  ],
  "workstations": [
    {
      "name": "plan-goal",
      "type": "AGENT_RUN",
      "worker": "goal-planner",
      "inputs": [
        {"workType": "goal", "state": "init"}
      ],
      "outputs": [
        {"workType": "goal", "state": "plan"}
      ],
      "onFailure": [
        {"workType": "goal", "state": "failed"}
      ],
      "promptFile": "prompts/planner.md",
      "body": "You are planning goal work {{ .WorkID }} for an AGENT_RUN workstation backed by an AGENT_WORKER.\n\nProduce a bounded plan the executor and reviewer can inspect quickly. Do not respond with open-ended discussion or unrestricted narrative.\n\nReturn exactly these sections:\n## Goal\nOne sentence restating the requested goal in customer-facing terms.\n## Plan\nNumbered steps specific enough for later execution. Limit to at most 8 steps.\n## Acceptance checks\nBullet list of observable outcomes the checker and reviewer should verify.\n## Risks and assumptions\nBullet list of risks, blockers, or assumptions needing review. Write \"None identified.\" if there are none."
    },
    {
      "name": "execute-goal",
      "type": "AGENT_RUN",
      "worker": "goal-executor",
      "inputs": [
        {"workType": "goal", "state": "plan"}
      ],
      "outputs": [
        {"workType": "goal", "state": "execute"}
      ],
      "onFailure": [
        {"workType": "goal", "state": "failed"}
      ],
      "promptFile": "prompts/executor.md",
      "body": "You are executing goal work {{ .WorkID }} at an AGENT_RUN workstation backed by an AGENT_WORKER.\n\nProduce a bounded execution result the checker and reviewer can inspect quickly. Do not respond with open-ended discussion or unrestricted narrative.\n\nReturn exactly these sections:\n## Completed work\nBullet list of concrete work completed in this attempt.\n## Blockers\nBullet list of blockers that stopped or slowed progress. Write \"None.\" if there are none.\n## Follow-up for review\nBullet list of remaining items, decisions, or validation the reviewer should judge before routing the goal forward.\n## Outcome\nOne of: ready_for_check, needs_replan, blocked."
    },
    {
      "name": "check-goal",
      "type": "SCRIPT_RUN",
      "worker": "goal-checker",
      "inputs": [
        {"workType": "goal", "state": "execute"}
      ],
      "outputs": [
        {"workType": "goal", "state": "check"}
      ],
      "onFailure": [
        {"workType": "goal", "state": "failed"}
      ],
      "promptFile": "prompts/checker.md",
      "body": "You are running verification for goal work {{ .WorkID }} at a SCRIPT_RUN workstation.\n\nProduce reviewable verification findings the reviewer can route on. Do not respond with open-ended discussion or unrestricted narrative.\n\nReturn exactly these sections:\n## Checks run\nBullet list of verification commands or checks executed.\n## Results\nPass/fail summary for each check.\n## Findings\nBullet list of concrete failures, warnings, or gaps. Write \"None.\" if all checks passed.\n## Recommendation\nOne of: pass, fail, needs_human."
    },
    {
      "name": "advance-goal-review",
      "type": "LOGICAL_MOVE",
      "inputs": [
        {"workType": "goal", "state": "check"}
      ],
      "outputs": [
        {"workType": "goal", "state": "review"}
      ],
      "worker": ""
    },
    {
      "name": "review-goal",
      "type": "CLASSIFIER_WORKSTATION",
      "worker": "goal-reviewer",
      "inputs": [
        {"workType": "goal", "state": "review"}
      ],
      "classificationRoutes": [
        {"label": "accepted", "outputs": [{"workType": "goal", "state": "summarize"}]},
        {"label": "needs_changes", "outputs": [{"workType": "goal", "state": "plan"}]},
        {"label": "tests_failed", "outputs": [{"workType": "goal", "state": "plan"}]},
        {"label": "needs_human", "outputs": [{"workType": "goal", "state": "needs-human"}]},
        {"label": "blocked", "outputs": [{"workType": "goal", "state": "blocked"}]},
        {"label": "interrupted", "outputs": [{"workType": "goal", "state": "interrupted"}]},
        {"label": "failed", "outputs": [{"workType": "goal", "state": "failed"}]}
      ],
      "onFailure": [
        {"workType": "goal", "state": "failed"}
      ],
      "promptFile": "prompts/reviewer.md",
      "body": "You are reviewing goal work {{ .WorkID }} backed by an AGENT_WORKER.\n\nProduce a reviewable disposition the factory can route on. Do not respond with open-ended discussion or unrestricted narrative.\n\nReturn exactly these sections:\n## Disposition\nOne of: accepted, needs_changes, tests_failed, needs_human, blocked, interrupted, failed.\n## Findings\nBullet list of concrete review findings supporting the disposition.\n## Required follow-up\nBullet list of changes, checks, or human actions needed before the goal can advance. Write \"None.\" if disposition is accepted."
    },
    {
      "name": "summarize-goal",
      "type": "AGENT_RUN",
      "worker": "goal-summarizer",
      "inputs": [
        {"workType": "goal", "state": "summarize"}
      ],
      "outputs": [
        {"workType": "goal", "state": "complete"}
      ],
      "onFailure": [
        {"workType": "goal", "state": "failed"}
      ],
      "promptFile": "prompts/summarizer.md",
      "body": "You are summarizing completed goal work {{ .WorkID }} at an AGENT_RUN workstation backed by an AGENT_WORKER.\n\nThe goal run may have passed through AGENT_RUN planning and execution steps and SCRIPT_RUN verification before review accepted the work. Produce a bounded final summary a customer can review quickly. Do not respond with open-ended discussion or unrestricted narrative.\n\nReturn exactly these sections:\n## Outcome\nOne sentence stating whether the goal succeeded and the primary deliverable.\n## What was done\nBullet list of the main completed work. Limit to at most 6 bullets.\n## Verification\nBrief pass/fail summary of SCRIPT_RUN checks and reviewer disposition.\n## Follow-up\nBullet list of open items, if any. Write \"None.\" if the goal is fully complete."
    },
    {
      "name": "goal-loop-breaker",
      "type": "LOGICAL_MOVE",
      "guards": [
        {
          "type": "VISIT_COUNT",
          "workstation": "review-goal",
          "maxVisits": 5
        }
      ],
      "inputs": [
        {"workType": "goal", "state": "plan"}
      ],
      "outputs": [
        {"workType": "goal", "state": "failed"}
      ],
      "worker": ""
    }
  ]
}`)

// BuiltInTTSFactoryJSON is the canonical runnable @you/tts packaged factory payload.
var BuiltInTTSFactoryJSON = []byte(`{
  "name": "@you/tts",
  "id": "builtin-tts",
  "workTypes": [
    {
      "name": "task",
      "handlingBehavior": ["DEFAULT"],
      "states": [
        {"name": "init", "type": "INITIAL"},
        {"name": "complete", "type": "TERMINAL"},
        {"name": "failed", "type": "FAILED"}
      ]
    }
  ],
  "resources": [
    {
      "name": "omnivoice-cache",
      "type": "MODEL",
      "capacity": 1,
      "model": "OMNIVOICE_Q4_K_M",
      "backend": "LLAMACPP",
      "loadPolicy": "ON_DEMAND"
    }
  ],
  "workers": [
    {
      "name": "tts-executor",
      "type": "MODEL_WORKER",
      "model": "OMNIVOICE_Q4_K_M",
      "modelProvider": "CODEX",
      "modelLocality": "LOCAL",
      "command": "omnivoice-llamacpp",
      "resources": [
        {"name": "omnivoice-cache", "capacity": 1}
      ],
      "operations": [
        {
          "name": "TTS",
          "inputs": [
            {"name": "text", "contentTypes": ["TEXT"], "required": true}
          ],
          "outputs": [
            {"name": "audio", "contentTypes": ["AUDIO"]}
          ]
        }
      ],
      "body": "You are the @you/tts built-in factory worker."
    }
  ],
  "workstations": [
    {
      "name": "execute-tts",
      "type": "MODEL_INVOKE",
      "worker": "tts-executor",
      "operation": "TTS",
      "operationBindings": [
        {
          "slot": "text",
          "selector": {"type": "TEXT"}
        }
      ],
      "inputs": [
        {"workType": "task", "state": "init"}
      ],
      "outputs": [
        {"workType": "task", "state": "complete"}
      ],
      "onFailure": [
        {"workType": "task", "state": "failed"}
      ],
      "body": "Convert the requested text into speech for {{ .WorkID }}."
    }
  ]
}`)
