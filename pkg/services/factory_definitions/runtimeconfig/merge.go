// Package runtimeconfig assembles the effective Factory Definition used by a
// runtime from its authored topology and separately loaded runtime definitions.
package runtimeconfig

import (
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/workstationexecution"
)

// Merge returns a detached Factory Definition with runtime Worker and
// Workstation definitions applied to the authored topology.
func Merge(
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
) (*factorydefinitions.FactoryConfig, error) {
	if factoryConfig == nil {
		return nil, nil
	}
	if runtimeDefinitions == nil {
		return nil, fmt.Errorf("runtime config is required")
	}

	effective, err := factorydefinitions.CloneFactoryConfig(factoryConfig)
	if err != nil {
		return nil, fmt.Errorf("clone factory config: %w", err)
	}
	if err := applyRuntimeDefinitions(effective, runtimeDefinitions); err != nil {
		return nil, err
	}
	return effective, nil
}

func applyRuntimeDefinitions(
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
) error {
	for index := range factoryConfig.Workers {
		definition, ok := runtimeDefinitions.Worker(factoryConfig.Workers[index].Name)
		if !ok || definition == nil {
			continue
		}
		applyWorkerRuntimeDefinition(&factoryConfig.Workers[index], definition)
	}

	for index := range factoryConfig.Workstations {
		workstation := &factoryConfig.Workstations[index]
		normalizeCanonicalWorkstationRuntime(workstation)
		definition, ok := runtimeDefinitions.Workstation(workstation.Name)
		if !ok || definition == nil {
			continue
		}
		if err := applyWorkstationRuntimeDefinition(workstation, definition); err != nil {
			return fmt.Errorf("normalize workstation %q config: %w", workstation.Name, err)
		}
	}
	return nil
}

func applyWorkerRuntimeDefinition(
	worker *factorydefinitions.FactoryWorkerConfig,
	definition *factorydefinitions.FactoryWorkerConfig,
) {
	if worker == nil || definition == nil {
		return
	}
	runtimeDefinition := factorydefinitions.CloneWorkerConfig(*definition)

	if worker.Name == "" && runtimeDefinition.Name != "" {
		worker.Name = runtimeDefinition.Name
	}
	if runtimeDefinition.Type != "" {
		worker.Type = runtimeDefinition.Type
	}
	if runtimeDefinition.Provider != "" {
		worker.Provider = runtimeDefinition.Provider
	}
	if runtimeDefinition.Model != "" {
		worker.Model = runtimeDefinition.Model
	}
	if runtimeDefinition.ModelProvider != "" {
		worker.ModelProvider = runtimeDefinition.ModelProvider
	}
	if runtimeDefinition.ModelLocality != "" {
		worker.ModelLocality = runtimeDefinition.ModelLocality
	}
	if runtimeDefinition.ExecutorProvider != "" {
		worker.ExecutorProvider = runtimeDefinition.ExecutorProvider
	}
	if len(runtimeDefinition.Operations) > 0 {
		worker.Operations = factorydefinitions.CloneModelOperations(runtimeDefinition.Operations)
	}
	if runtimeDefinition.SessionID != "" {
		worker.SessionID = runtimeDefinition.SessionID
	}
	if runtimeDefinition.Command != "" {
		worker.Command = runtimeDefinition.Command
	}
	if len(runtimeDefinition.Args) > 0 {
		worker.Args = append([]string(nil), runtimeDefinition.Args...)
	}
	if runtimeDefinition.Concurrency != 0 {
		worker.Concurrency = runtimeDefinition.Concurrency
	}
	if runtimeDefinition.Timeout != "" {
		worker.Timeout = runtimeDefinition.Timeout
	}
	if runtimeDefinition.StopToken != "" {
		worker.StopToken = runtimeDefinition.StopToken
	}
	if runtimeDefinition.SkipPermissions {
		worker.SkipPermissions = true
	}
	if runtimeDefinition.OpenCodeAgent != "" {
		worker.OpenCodeAgent = runtimeDefinition.OpenCodeAgent
	}
	if runtimeDefinition.Auth != nil {
		worker.Auth = runtimeDefinition.Auth
	}
	if runtimeDefinition.Linear != nil {
		worker.Linear = runtimeDefinition.Linear
	}
	if runtimeDefinition.Body != "" {
		worker.Body = runtimeDefinition.Body
	}
	if len(runtimeDefinition.Resources) > 0 {
		worker.Resources = append(
			[]factorydefinitions.ResourceConfig(nil),
			runtimeDefinition.Resources...,
		)
	}
}

func applyWorkstationRuntimeDefinition(
	workstation *factorydefinitions.FactoryWorkstationConfig,
	definition *factorydefinitions.FactoryWorkstationConfig,
) error {
	if workstation == nil || definition == nil {
		return nil
	}
	normalizeCanonicalWorkstationRuntime(workstation)
	baseStopWords := append([]string(nil), workstation.StopWords...)
	runtimeDefinition := factorydefinitions.CloneWorkstationConfig(*definition)
	if strings.TrimSpace(runtimeDefinition.Type) == "" &&
		strings.TrimSpace(workstation.Type) == "" {
		runtimeDefinition.Type = defaultWorkstationRuntimeType(
			firstNonEmpty(runtimeDefinition.WorkerTypeName, workstation.WorkerTypeName),
		)
	}
	normalizeCanonicalWorkstationRuntime(&runtimeDefinition)

	if runtimeDefinition.ID != "" {
		workstation.ID = runtimeDefinition.ID
	}
	if runtimeDefinition.Name != "" && workstation.Name == "" {
		workstation.Name = runtimeDefinition.Name
	}
	if runtimeDefinition.Kind != "" {
		workstation.Kind = runtimeDefinition.Kind
	}
	if runtimeDefinition.Type != "" {
		workstation.Type = runtimeDefinition.Type
	}
	if runtimeDefinition.WorkerTypeName != "" {
		workstation.WorkerTypeName = runtimeDefinition.WorkerTypeName
	}
	if runtimeDefinition.Operation != "" {
		workstation.Operation = runtimeDefinition.Operation
	}
	if runtimeDefinition.Runner != "" {
		workstation.Runner = runtimeDefinition.Runner
	}
	if runtimeDefinition.OpenCodeAgent != "" {
		workstation.OpenCodeAgent = runtimeDefinition.OpenCodeAgent
	}
	applyWorkstationRuntimeTopology(workstation, runtimeDefinition)
	applyWorkstationRuntimeTemplate(workstation, runtimeDefinition, baseStopWords)
	return nil
}

func applyWorkstationRuntimeTopology(
	workstation *factorydefinitions.FactoryWorkstationConfig,
	runtimeDefinition factorydefinitions.FactoryWorkstationConfig,
) {
	if runtimeDefinition.Cron != nil {
		cron := *runtimeDefinition.Cron
		workstation.Cron = &cron
	}
	if len(runtimeDefinition.Inputs) > 0 {
		workstation.Inputs = factorydefinitions.CloneIOConfigs(runtimeDefinition.Inputs)
	}
	if len(runtimeDefinition.Outputs) > 0 {
		workstation.Outputs = factorydefinitions.CloneIOConfigs(runtimeDefinition.Outputs)
	}
	if len(runtimeDefinition.OperationBindings) > 0 {
		workstation.OperationBindings = factorydefinitions.CloneModelOperationBindings(
			runtimeDefinition.OperationBindings,
		)
	}
	if len(runtimeDefinition.OnContinue) > 0 {
		workstation.OnContinue = factorydefinitions.CloneIOConfigs(runtimeDefinition.OnContinue)
	}
	if len(runtimeDefinition.OnRejection) > 0 {
		workstation.OnRejection = factorydefinitions.CloneIOConfigs(runtimeDefinition.OnRejection)
	}
	if len(runtimeDefinition.OnFailure) > 0 {
		workstation.OnFailure = factorydefinitions.CloneIOConfigs(runtimeDefinition.OnFailure)
	}
	if len(runtimeDefinition.Resources) > 0 {
		workstation.Resources = append(
			[]factorydefinitions.ResourceConfig(nil),
			runtimeDefinition.Resources...,
		)
	}
	if len(runtimeDefinition.Guards) > 0 {
		workstation.Guards = append(
			[]factorydefinitions.GuardConfig(nil),
			runtimeDefinition.Guards...,
		)
	}
}

func applyWorkstationRuntimeTemplate(
	workstation *factorydefinitions.FactoryWorkstationConfig,
	runtimeDefinition factorydefinitions.FactoryWorkstationConfig,
	baseStopWords []string,
) {
	if runtimeDefinition.PromptFile != "" {
		workstation.PromptFile = runtimeDefinition.PromptFile
	}
	if runtimeDefinition.OutputSchema != "" {
		workstation.OutputSchema = runtimeDefinition.OutputSchema
	}
	workstation.Limits = mergeWorkstationLimits(workstation.Limits, runtimeDefinition.Limits)
	workstationexecution.NormalizeExecutionLimit(workstation)
	workstation.StopWords = mergeStopWords(
		baseStopWords,
		mergeStopWords(runtimeDefinition.StopWords, runtimeDefinition.RuntimeStopWords),
	)
	if runtimeDefinition.Body != "" {
		workstation.Body = runtimeDefinition.Body
	}
	if runtimeDefinition.PromptTemplate != "" {
		workstation.PromptTemplate = runtimeDefinition.PromptTemplate
	}
	if runtimeDefinition.WorkingDirectory != "" {
		workstation.WorkingDirectory = runtimeDefinition.WorkingDirectory
	}
	if runtimeDefinition.Worktree != "" {
		workstation.Worktree = runtimeDefinition.Worktree
	}
	workstation.Env = mergeStringMap(workstation.Env, runtimeDefinition.Env)
}

func normalizeCanonicalWorkstationRuntime(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) {
	if workstation == nil {
		return
	}
	if factorydefinitions.StrictPublicFactoryWorkstationType(workstation.Type) ==
		factorydefinitions.WorkstationTypePoller {
		switch workstation.Kind {
		case "", factorydefinitions.WorkstationKindStandard:
			workstation.Kind = factorydefinitions.WorkstationKindPoller
		}
	}
	if workstation.PromptTemplate == "" {
		workstation.PromptTemplate = workstation.Body
	}
	workstationexecution.NormalizeExecutionLimit(workstation)
}

// NormalizeCanonicalWorkstationRuntime applies the shared authored/runtime
// normalization used when a Workstation definition enters effective runtime
// state or is written back to an authored layout.
func NormalizeCanonicalWorkstationRuntime(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) {
	normalizeCanonicalWorkstationRuntime(workstation)
}

func defaultWorkstationRuntimeType(workerName string) string {
	if strings.TrimSpace(workerName) == "" {
		return factorydefinitions.WorkstationTypeLogical
	}
	return factorydefinitions.WorkstationTypeModel
}

func mergeWorkstationLimits(
	base factorydefinitions.WorkstationLimits,
	runtime factorydefinitions.WorkstationLimits,
) factorydefinitions.WorkstationLimits {
	merged := base
	if runtime.MaxRetries != 0 {
		merged.MaxRetries = runtime.MaxRetries
	}
	if runtime.MaxExecutionTime != "" {
		merged.MaxExecutionTime = runtime.MaxExecutionTime
	}
	return merged
}

func mergeStopWords(base, extra []string) []string {
	if len(base) == 0 {
		return append([]string(nil), extra...)
	}
	merged := append([]string(nil), base...)
	seen := make(map[string]bool, len(base)+len(extra))
	for _, stopWord := range base {
		seen[stopWord] = true
	}
	for _, stopWord := range extra {
		if seen[stopWord] {
			continue
		}
		merged = append(merged, stopWord)
		seen[stopWord] = true
	}
	return merged
}

func mergeStringMap(base, runtime map[string]string) map[string]string {
	if len(base) == 0 && len(runtime) == 0 {
		return nil
	}
	merged := make(map[string]string, len(base)+len(runtime))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range runtime {
		merged[key] = value
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
