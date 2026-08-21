package workstationexecution

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type (
	FactoryConfig                    = factorydefinitions.FactoryConfig
	FactoryWorkerConfig              = factorydefinitions.FactoryWorkerConfig
	FactoryWorkstationConfig         = factorydefinitions.FactoryWorkstationConfig
	ResolveExecutionCatalogRequest   = factorydefinitions.ResolveExecutionCatalogRequest
	ResolveExecutionCatalogResult    = factorydefinitions.ResolveExecutionCatalogResult
	ResolvedExecutionCatalog         = factorydefinitions.ResolvedExecutionCatalog
	ExecutionCatalogReferenceCatalog = factorydefinitions.ExecutionCatalogReferenceCatalog
	ExecutionCatalogDiagnostic       = factorydefinitions.ExecutionCatalogDiagnostic
	ExecutionCatalogDiagnosticCode   = factorydefinitions.ExecutionCatalogDiagnosticCode
	ExecutionCatalogError            = factorydefinitions.ExecutionCatalogError
	ResolvedModelOperation           = factorydefinitions.ResolvedModelOperation
	ResolvedModelOperationSlot       = factorydefinitions.ResolvedModelOperationSlot
	ResolvedWorkerDefinition         = factorydefinitions.ResolvedWorkerDefinition
	ResolvedInputGuard               = factorydefinitions.ResolvedInputGuard
	ResolvedWorkstationIO            = factorydefinitions.ResolvedWorkstationIO
	ResolvedClassificationRoute      = factorydefinitions.ResolvedClassificationRoute
	ResolvedWorkstationLimits        = factorydefinitions.ResolvedWorkstationLimits
	ResolvedResource                 = factorydefinitions.ResolvedResource
	ResolvedModelOperationBinding    = factorydefinitions.ResolvedModelOperationBinding
	ResolvedWorkstationDefinition    = factorydefinitions.ResolvedWorkstationDefinition
	ModelOperation                   = factorydefinitions.ModelOperation
	ModelOperationSlot               = factorydefinitions.ModelOperationSlot
	ModelOperationBinding            = factorydefinitions.ModelOperationBinding
	ModelOperationBindingSelector    = factorydefinitions.ModelOperationBindingSelector
	WorkstationLimits                = factorydefinitions.WorkstationLimits
	WorkPropagationConfig            = factorydefinitions.WorkPropagationConfig
	WorkPropagationMode              = factorydefinitions.WorkPropagationMode
	IOConfig                         = factorydefinitions.IOConfig
	ClassificationRouteConfig        = factorydefinitions.ClassificationRouteConfig
	ExpectedArtifactConfig           = factorydefinitions.ExpectedArtifactConfig
	ResourceConfig                   = factorydefinitions.ResourceConfig
	GuardConfig                      = factorydefinitions.GuardConfig
	WorkstationKind                  = factorydefinitions.WorkstationKind
	FileReader                       = factorydefinitions.FileReader
)

const (
	ExecutionCatalogDiagnosticInvalidDefinition    = factorydefinitions.ExecutionCatalogDiagnosticInvalidDefinition
	ExecutionCatalogDiagnosticInvalidInterpolation = factorydefinitions.ExecutionCatalogDiagnosticInvalidInterpolation
	ExecutionCatalogDiagnosticInvalidRunner        = factorydefinitions.ExecutionCatalogDiagnosticInvalidRunner
	ExecutionCatalogDiagnosticUnknownRunner        = factorydefinitions.ExecutionCatalogDiagnosticUnknownRunner
	ExecutionCatalogDiagnosticInvalidProvider      = factorydefinitions.ExecutionCatalogDiagnosticInvalidProvider
	ExecutionCatalogDiagnosticUnknownProvider      = factorydefinitions.ExecutionCatalogDiagnosticUnknownProvider
	ExecutionCatalogDiagnosticInvalidModel         = factorydefinitions.ExecutionCatalogDiagnosticInvalidModel
	ExecutionCatalogDiagnosticUnknownModel         = factorydefinitions.ExecutionCatalogDiagnosticUnknownModel
	ExecutionCatalogDiagnosticUnknownWorker        = factorydefinitions.ExecutionCatalogDiagnosticUnknownWorker
	ExecutionCatalogDiagnosticUnknownWorkstation   = factorydefinitions.ExecutionCatalogDiagnosticUnknownWorkstation
	ExecutionCatalogDiagnosticDuplicateIdentity    = factorydefinitions.ExecutionCatalogDiagnosticDuplicateIdentity
	ExecutionCatalogDiagnosticInvalidTimeout       = factorydefinitions.ExecutionCatalogDiagnosticInvalidTimeout
	WorkstationTypeLogical                         = factorydefinitions.WorkstationTypeLogical
	DecisionEnvelopeOutcomeFormat                  = factorydefinitions.DecisionEnvelopeOutcomeFormat
	PackagedTTSInvokeWorkstationName               = factorydefinitions.PackagedTTSInvokeWorkstationName
	WorkPropagationModeOutputAsPayload             = factorydefinitions.WorkPropagationModeOutputAsPayload
	ArgumentErrorCodeInvalidInterpolation          = factorydefinitions.ArgumentErrorCodeInvalidInterpolation
)

var (
	CloneFactoryConfig          = factorydefinitions.CloneFactoryConfig
	CloneWorkstationConfig      = factorydefinitions.CloneWorkstationConfig
	CloneModelOperationBindings = factorydefinitions.CloneModelOperationBindings
	CloneIOConfigs              = factorydefinitions.CloneIOConfigs
	EffectiveAgentToolPolicy    = factorydefinitions.EffectiveAgentToolPolicy
)

// ResolveExecutionCatalog resolves one effective Factory definition without
// constructing or querying any execution service. All input and output values
// are detached; callers may mutate the result freely.
func ResolveExecutionCatalog(
	ctx context.Context,
	request ResolveExecutionCatalogRequest,
) (ResolveExecutionCatalogResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ResolveExecutionCatalogResult{}, err
	}
	if request.EffectiveDefinition == nil {
		return invalidExecutionCatalogResult()
	}
	definition, err := CloneFactoryConfig(request.EffectiveDefinition)
	if err != nil {
		return ResolveExecutionCatalogResult{}, fmt.Errorf("clone effective Factory definition: %w", err)
	}
	preserveExecutionRuntimeFields(definition, request.EffectiveDefinition)
	return resolveExecutionCatalogDefinition(
		ctx,
		definition,
		work.CloneInvocationArguments(request.Invocation.Arguments),
		request.Invocation.ReadFile,
		cloneExecutionCatalogReferences(request.References),
	)
}

func invalidExecutionCatalogResult() (ResolveExecutionCatalogResult, error) {
	diagnostics := []ExecutionCatalogDiagnostic{{
		Code:    ExecutionCatalogDiagnosticInvalidDefinition,
		Path:    "definition",
		Message: "effective Factory definition is required",
	}}
	return ResolveExecutionCatalogResult{Diagnostics: diagnostics}, &ExecutionCatalogError{
		Diagnostics: cloneExecutionCatalogDiagnostics(diagnostics),
	}
}

func resolveExecutionCatalogDefinition(
	ctx context.Context,
	definition *FactoryConfig,
	args *work.InvocationArguments,
	readFile FileReader,
	references ExecutionCatalogReferenceCatalog,
) (ResolveExecutionCatalogResult, error) {
	result := ResolveExecutionCatalogResult{
		ResolvedExecutionCatalog: ResolvedExecutionCatalog{
			DefinitionVersion: executionDefinitionVersion(definition),
			Workers:           make(map[string]ResolvedWorkerDefinition, len(definition.Workers)),
			Workstations:      make(map[string]ResolvedWorkstationDefinition, len(definition.Workstations)),
		},
	}
	factoryRunner, diagnostics := resolveExecutionFactoryRunner(definition.Runner, references.Runners)
	workers, workerDiagnostics, err := resolveExecutionWorkers(
		ctx, definition.Workers, args, readFile, references,
	)
	if err != nil {
		return ResolveExecutionCatalogResult{}, err
	}
	result.Workers = workers
	diagnostics = append(diagnostics, workerDiagnostics...)
	workstations, workstationDiagnostics, err := resolveExecutionWorkstations(
		ctx, definition.Workstations, args, readFile, factoryRunner, result.Workers, references,
	)
	if err != nil {
		return ResolveExecutionCatalogResult{}, err
	}
	result.Workstations = workstations
	diagnostics = append(diagnostics, workstationDiagnostics...)
	diagnostics = append(diagnostics, validateExecutionCatalogGuards(definition, result.Workstations, references)...)
	result.Diagnostics = cloneExecutionCatalogDiagnostics(diagnostics)
	if len(diagnostics) > 0 {
		return result, &ExecutionCatalogError{Diagnostics: cloneExecutionCatalogDiagnostics(diagnostics)}
	}
	return result, nil
}

func resolveExecutionFactoryRunner(
	authored string,
	known map[string]struct{},
) (string, []ExecutionCatalogDiagnostic) {
	value := strings.TrimSpace(authored)
	if value == "" {
		return "", nil
	}
	normalized := normalizeExecutionRunner(value)
	if normalized == "" {
		return value, []ExecutionCatalogDiagnostic{executionDiagnostic(
			ExecutionCatalogDiagnosticInvalidRunner,
			"definition.runner", value, "runner identity is invalid",
		)}
	}
	if diagnostic := validateExecutionReference(
		known, normalized, "definition.runner",
		ExecutionCatalogDiagnosticInvalidRunner, ExecutionCatalogDiagnosticUnknownRunner,
	); diagnostic != nil {
		return normalized, []ExecutionCatalogDiagnostic{*diagnostic}
	}
	return normalized, nil
}

func resolveExecutionWorkers(
	ctx context.Context,
	workers []FactoryWorkerConfig,
	args *work.InvocationArguments,
	readFile FileReader,
	references ExecutionCatalogReferenceCatalog,
) (map[string]ResolvedWorkerDefinition, []ExecutionCatalogDiagnostic, error) {
	resolvedWorkers := make(map[string]ResolvedWorkerDefinition, len(workers))
	var diagnostics []ExecutionCatalogDiagnostic
	for index, worker := range workers {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		name := strings.TrimSpace(worker.Name)
		if name == "" {
			diagnostics = append(diagnostics, executionDiagnostic(
				ExecutionCatalogDiagnosticInvalidDefinition,
				fmt.Sprintf("workers[%d].name", index), "", "worker name is required",
			))
			continue
		}
		if _, exists := resolvedWorkers[name]; exists {
			diagnostics = append(diagnostics, executionDiagnostic(
				ExecutionCatalogDiagnosticDuplicateIdentity,
				fmt.Sprintf("workers[%d].name", index), name, "worker name is duplicated",
			))
			continue
		}
		resolved, workerDiagnostics := resolveExecutionWorker(
			worker, name, args, readFile, references,
		)
		diagnostics = append(diagnostics, workerDiagnostics...)
		resolvedWorkers[name] = resolved
	}
	return resolvedWorkers, diagnostics, nil
}

func resolveExecutionWorkstations(
	ctx context.Context,
	workstations []FactoryWorkstationConfig,
	args *work.InvocationArguments,
	readFile FileReader,
	factoryRunner string,
	workers map[string]ResolvedWorkerDefinition,
	references ExecutionCatalogReferenceCatalog,
) (map[string]ResolvedWorkstationDefinition, []ExecutionCatalogDiagnostic, error) {
	resolvedWorkstations := make(map[string]ResolvedWorkstationDefinition, len(workstations))
	var diagnostics []ExecutionCatalogDiagnostic
	for index, workstation := range workstations {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		name := strings.TrimSpace(workstation.Name)
		if name == "" {
			name = strings.TrimSpace(workstation.ID)
		}
		if name == "" {
			diagnostics = append(diagnostics, executionDiagnostic(
				ExecutionCatalogDiagnosticInvalidDefinition,
				fmt.Sprintf("workstations[%d].name", index), "", "workstation name is required",
			))
			continue
		}
		if _, exists := resolvedWorkstations[name]; exists {
			diagnostics = append(diagnostics, executionDiagnostic(
				ExecutionCatalogDiagnosticDuplicateIdentity,
				fmt.Sprintf("workstations[%d].name", index), name, "workstation name is duplicated",
			))
			continue
		}
		resolved, workstationDiagnostics := resolveExecutionWorkstation(
			workstation, name, args, readFile, factoryRunner, workers, references,
		)
		diagnostics = append(diagnostics, workstationDiagnostics...)
		resolvedWorkstations[name] = resolved
	}
	return resolvedWorkstations, diagnostics, nil
}

func validateExecutionCatalogGuards(
	definition *FactoryConfig,
	workstations map[string]ResolvedWorkstationDefinition,
	references ExecutionCatalogReferenceCatalog,
) []ExecutionCatalogDiagnostic {
	var diagnostics []ExecutionCatalogDiagnostic
	for index, authoredWorkstation := range definition.Workstations {
		name := strings.TrimSpace(authoredWorkstation.Name)
		if name == "" {
			name = strings.TrimSpace(authoredWorkstation.ID)
		}
		workstation, ok := workstations[name]
		if !ok {
			continue
		}
		diagnostics = append(diagnostics, validateExecutionWorkstationGuards(
			name, index, workstation, workstations,
		)...)
	}
	for index, guard := range definition.Guards {
		if diagnostic := validateExecutionProvider(
			guard.ModelProvider, fmt.Sprintf("guards[%d].modelProvider", index), references.Providers,
		); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
		}
		if diagnostic := validateExecutionModel(
			guard.Model, fmt.Sprintf("guards[%d].model", index), references.Models,
		); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
		}
	}
	return diagnostics
}

func validateExecutionWorkstationGuards(
	name string,
	index int,
	workstation ResolvedWorkstationDefinition,
	workstations map[string]ResolvedWorkstationDefinition,
) []ExecutionCatalogDiagnostic {
	var diagnostics []ExecutionCatalogDiagnostic
	for guardIndex, guard := range workstation.Guards {
		reference := strings.TrimSpace(guard.Workstation)
		if reference == "" {
			continue
		}
		if _, ok := workstations[reference]; !ok {
			diagnostics = append(diagnostics, executionDiagnostic(
				ExecutionCatalogDiagnosticUnknownWorkstation,
				fmt.Sprintf("workstations[%d].guards[%d].workstation", index, guardIndex),
				reference, "guard references an unknown workstation",
			))
		}
	}
	return diagnostics
}

func preserveExecutionRuntimeFields(clone, source *FactoryConfig) {
	if clone == nil || source == nil {
		return
	}
	for index := range clone.Workers {
		if index >= len(source.Workers) {
			break
		}
		clone.Workers[index].Body = source.Workers[index].Body
		clone.Workers[index].PromptSourcePath = source.Workers[index].PromptSourcePath
		clone.Workers[index].SessionID = source.Workers[index].SessionID
		clone.Workers[index].Concurrency = source.Workers[index].Concurrency
		clone.Workers[index].RuntimeDefaultModelProvider = source.Workers[index].RuntimeDefaultModelProvider
		clone.Workers[index].RuntimeDefaultModel = source.Workers[index].RuntimeDefaultModel
	}
	for index := range clone.Workstations {
		if index >= len(source.Workstations) {
			break
		}
		clone.Workstations[index].Body = source.Workstations[index].Body
		clone.Workstations[index].PromptTemplate = source.Workstations[index].PromptTemplate
		clone.Workstations[index].RuntimeStopWords = append([]string(nil), source.Workstations[index].RuntimeStopWords...)
		clone.Workstations[index].PromptSourcePath = source.Workstations[index].PromptSourcePath
		clone.Workstations[index].PromptSourceIsTemplate = source.Workstations[index].PromptSourceIsTemplate
		clone.Workstations[index].CopyReferencedScripts = source.Workstations[index].CopyReferencedScripts
	}
}

func resolveExecutionWorker(
	worker FactoryWorkerConfig,
	name string,
	args *work.InvocationArguments,
	readFile FileReader,
	references ExecutionCatalogReferenceCatalog,
) (ResolvedWorkerDefinition, []ExecutionCatalogDiagnostic) {
	resolved, err := interpolateExecutionWorker(worker, args, readFile)
	if err != nil {
		return ResolvedWorkerDefinition{Name: name, ID: worker.ID}, []ExecutionCatalogDiagnostic{
			executionInterpolationDiagnostic("workers."+name, err),
		}
	}
	if strings.TrimSpace(resolved.ModelProvider) == "" {
		resolved.ModelProvider = resolved.RuntimeDefaultModelProvider
	}
	if strings.TrimSpace(resolved.Model) == "" {
		resolved.Model = resolved.RuntimeDefaultModel
	}
	result := resolvedExecutionWorkerValue(resolved, name)
	diagnostics := append([]ExecutionCatalogDiagnostic(nil),
		executionWorkerTimeoutDiagnostic(&result, resolved.Timeout, name)...)
	diagnostics = append(diagnostics, validateExecutionWorkerPolicy(result, name, references)...)
	return result, diagnostics
}

func resolvedExecutionWorkerValue(
	worker FactoryWorkerConfig,
	name string,
) ResolvedWorkerDefinition {
	return ResolvedWorkerDefinition{
		ID:               worker.ID,
		Name:             name,
		Type:             strings.TrimSpace(worker.Type),
		Provider:         strings.TrimSpace(worker.Provider),
		Model:            strings.TrimSpace(worker.Model),
		ModelProvider:    strings.TrimSpace(worker.ModelProvider),
		ReasoningEffort:  strings.TrimSpace(worker.ReasoningEffort),
		ModelLocality:    strings.TrimSpace(worker.ModelLocality),
		ExecutorProvider: strings.TrimSpace(worker.ExecutorProvider),
		Command:          worker.Command,
		Args:             append([]string(nil), worker.Args...),
		Body:             worker.Body,
		PromptSourcePath: worker.PromptSourcePath,
		StopToken:        worker.StopToken,
		SkipPermissions:  worker.SkipPermissions,
		AgentToolPolicy:  EffectiveAgentToolPolicy(worker.AgentTools),
		Operations:       resolvedExecutionOperations(worker.Operations),
		Resources:        resolvedResources(worker.Resources),
	}
}

func executionWorkerTimeoutDiagnostic(
	result *ResolvedWorkerDefinition,
	authored string,
	name string,
) []ExecutionCatalogDiagnostic {
	if authored == "" {
		return nil
	}
	timeout, err := time.ParseDuration(strings.TrimSpace(authored))
	if err != nil {
		return []ExecutionCatalogDiagnostic{executionDiagnostic(
			ExecutionCatalogDiagnosticInvalidTimeout,
			"workers."+name+".timeout", authored, "worker timeout is invalid",
		)}
	}
	if timeout > 0 {
		result.Timeout = timeout
	}
	return nil
}

func validateExecutionWorkerPolicy(
	worker ResolvedWorkerDefinition,
	name string,
	references ExecutionCatalogReferenceCatalog,
) []ExecutionCatalogDiagnostic {
	var diagnostics []ExecutionCatalogDiagnostic
	diagnostics = append(diagnostics, validateExecutionProviderDiagnostic(
		worker.Provider, "workers."+name+".provider", references.Providers,
	)...)
	diagnostics = append(diagnostics, validateExecutionProviderDiagnostic(
		worker.ModelProvider, "workers."+name+".modelProvider", references.Providers,
	)...)
	if worker.Model != "" {
		if diagnostic := validateExecutionModel(
			worker.Model, "workers."+name+".model", references.Models,
		); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
		}
	}
	for resourceIndex, resource := range worker.Resources {
		diagnostics = append(diagnostics, validateExecutionProviderDiagnostic(
			resource.Provider,
			fmt.Sprintf("workers.%s.resources[%d].provider", name, resourceIndex),
			references.Providers,
		)...)
		if diagnostic := validateExecutionModel(
			resource.Model,
			fmt.Sprintf("workers.%s.resources[%d].model", name, resourceIndex),
			references.Models,
		); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
		}
	}
	if strings.EqualFold(worker.ExecutorProvider, "ACP") && worker.ModelProvider == "" {
		diagnostics = append(diagnostics, executionDiagnostic(
			ExecutionCatalogDiagnosticInvalidProvider,
			"workers."+name+".modelProvider", "",
			"ACP executor requires a model provider",
		))
	}
	if worker.ExecutorProvider != "" &&
		!validExecutionIdentity(worker.ExecutorProvider) &&
		!strings.EqualFold(worker.ExecutorProvider, "SCRIPT_WRAP") {
		diagnostics = append(diagnostics, executionDiagnostic(
			ExecutionCatalogDiagnosticInvalidProvider,
			"workers."+name+".executorProvider",
			worker.ExecutorProvider, "executor provider identity is invalid",
		))
	}
	return diagnostics
}

func validateExecutionProviderDiagnostic(
	value string,
	path string,
	known map[string]struct{},
) []ExecutionCatalogDiagnostic {
	if diagnostic := validateExecutionProvider(value, path, known); diagnostic != nil {
		return []ExecutionCatalogDiagnostic{*diagnostic}
	}
	return nil
}

func resolveExecutionWorkstation(
	workstation FactoryWorkstationConfig,
	name string,
	args *work.InvocationArguments,
	readFile FileReader,
	factoryRunner string,
	workers map[string]ResolvedWorkerDefinition,
	references ExecutionCatalogReferenceCatalog,
) (ResolvedWorkstationDefinition, []ExecutionCatalogDiagnostic) {
	resolved, err := interpolateExecutionWorkstation(workstation, args, readFile)
	if err != nil {
		return ResolvedWorkstationDefinition{Name: name, ID: workstation.ID}, []ExecutionCatalogDiagnostic{
			executionInterpolationDiagnostic("workstations."+name, err),
		}
	}
	result := resolvedExecutionWorkstationValue(resolved, name)
	diagnostics := executionWorkstationTimeoutDiagnostics(&result, resolved, name)
	diagnostics = append(diagnostics, validateExecutionWorkstationPolicy(
		&result, resolved, factoryRunner, workers, references,
	)...)
	return result, diagnostics
}

func resolvedExecutionWorkstationValue(
	workstation FactoryWorkstationConfig,
	name string,
) ResolvedWorkstationDefinition {
	outcomeFormat := strings.TrimSpace(workstation.OutcomeFormat)
	return ResolvedWorkstationDefinition{
		ID:               workstation.ID,
		Name:             name,
		Kind:             workstation.Kind,
		Type:             strings.TrimSpace(workstation.Type),
		Operation:        strings.TrimSpace(workstation.Operation),
		WorkerName:       strings.TrimSpace(workstation.WorkerTypeName),
		Runner:           normalizeExecutionRunner(workstation.Runner),
		PromptFile:       workstation.PromptFile,
		OutputSchema:     workstation.OutputSchema,
		OutputContract:   workstation.OutputContract,
		Body:             workstation.Body,
		PromptTemplate:   workstation.PromptTemplate,
		WorkingDirectory: workstation.WorkingDirectory,
		Worktree:         workstation.Worktree,
		Environment:      cloneStringMap(workstation.Env),
		OutputFormat:     outcomeFormat,
		DecisionEnvelope: outcomeFormat == DecisionEnvelopeOutcomeFormat,
		GoalRoutingDecisionEnvelope: outcomeFormat == DecisionEnvelopeOutcomeFormat &&
			len(workstation.ClassificationRoutes) > 0,
		FormatInvocationSummary:  strings.EqualFold(name, "execute-goal"),
		FormatInvocationResponse: strings.EqualFold(name, "run-subagent"),
		FormatTTSMetadata: strings.EqualFold(name, PackagedTTSInvokeWorkstationName) &&
			strings.EqualFold(strings.TrimSpace(workstation.Operation), "TTS"),
		WorkPropagation:      effectiveWorkPropagation(workstation.WorkPropagation),
		Limits:               resolvedWorkstationLimits(workstation.Limits),
		StopWords:            append([]string(nil), workstation.StopWords...),
		RuntimeStopWords:     append([]string(nil), workstation.RuntimeStopWords...),
		OperationBindings:    resolvedOperationBindings(workstation.OperationBindings),
		Inputs:               resolvedWorkstationIO(workstation.Inputs),
		Outputs:              resolvedWorkstationIO(workstation.Outputs),
		OnContinue:           resolvedWorkstationIO(workstation.OnContinue),
		OnRejection:          resolvedWorkstationIO(workstation.OnRejection),
		OnFailure:            resolvedWorkstationIO(workstation.OnFailure),
		ClassificationRoutes: resolvedClassificationRoutes(workstation.ClassificationRoutes),
		ExpectedArtifacts:    append([]ExpectedArtifactConfig(nil), workstation.ExpectedArtifacts...),
		Resources:            resolvedResources(workstation.Resources),
		Guards:               append([]GuardConfig(nil), workstation.Guards...),
	}
}

func executionWorkstationTimeoutDiagnostics(
	result *ResolvedWorkstationDefinition,
	workstation FactoryWorkstationConfig,
	name string,
) []ExecutionCatalogDiagnostic {
	timeout, err := executionWorkstationTimeout(workstation)
	if err != nil {
		return []ExecutionCatalogDiagnostic{executionDiagnostic(
			ExecutionCatalogDiagnosticInvalidTimeout,
			"workstations."+name+".limits.maxExecutionTime",
			workstation.Limits.MaxExecutionTime, "workstation timeout is invalid",
		)}
	}
	result.Timeout = timeout
	return nil
}

func validateExecutionWorkstationPolicy(
	result *ResolvedWorkstationDefinition,
	workstation FactoryWorkstationConfig,
	factoryRunner string,
	workers map[string]ResolvedWorkerDefinition,
	references ExecutionCatalogReferenceCatalog,
) []ExecutionCatalogDiagnostic {
	var diagnostics []ExecutionCatalogDiagnostic
	workerName := result.WorkerName
	if workerName != "" {
		if _, ok := workers[workerName]; !ok {
			diagnostics = append(diagnostics, executionDiagnostic(
				ExecutionCatalogDiagnosticUnknownWorker,
				"workstations."+result.Name+".worker", workerName,
				"workstation references an unknown worker",
			))
		}
	} else if !isLogicalExecutionWorkstation(workstation) {
		diagnostics = append(diagnostics, executionDiagnostic(
			ExecutionCatalogDiagnosticUnknownWorker,
			"workstations."+result.Name+".worker", "",
			"executable workstation must reference a worker",
		))
	}
	result.Runner, result.RunnerSelectionSource = selectExecutionRunner(
		result.Runner, factoryRunner, workerName, workers,
	)
	if diagnostic := validateExecutionReference(
		references.Runners, result.Runner, "workstations."+result.Name+".runner",
		ExecutionCatalogDiagnosticInvalidRunner, ExecutionCatalogDiagnosticUnknownRunner,
	); diagnostic != nil {
		diagnostics = append(diagnostics, *diagnostic)
	}
	diagnostics = append(diagnostics, validateExecutionWorkstationResources(
		result.Resources, result.Name, references,
	)...)
	return diagnostics
}

func selectExecutionRunner(
	workstationRunner string,
	factoryRunner string,
	workerName string,
	workers map[string]ResolvedWorkerDefinition,
) (string, string) {
	if workstationRunner != "" {
		return workstationRunner, "workstation"
	}
	if factoryRunner != "" {
		return factoryRunner, "factory"
	}
	if workerName != "" {
		if worker, ok := workers[workerName]; ok && worker.ModelProvider != "" {
			return normalizeExecutionRunner(worker.ModelProvider), "legacy_provider"
		}
	}
	return "codex", "default"
}

func validateExecutionWorkstationResources(
	resources []ResolvedResource,
	name string,
	references ExecutionCatalogReferenceCatalog,
) []ExecutionCatalogDiagnostic {
	var diagnostics []ExecutionCatalogDiagnostic
	for resourceIndex, resource := range resources {
		diagnostics = append(diagnostics, validateExecutionProviderDiagnostic(
			resource.Provider,
			fmt.Sprintf("workstations.%s.resources[%d].provider", name, resourceIndex),
			references.Providers,
		)...)
		if diagnostic := validateExecutionModel(
			resource.Model,
			fmt.Sprintf("workstations.%s.resources[%d].model", name, resourceIndex),
			references.Models,
		); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
		}
	}
	return diagnostics
}

func resolvedExecutionOperations(operations []ModelOperation) []ResolvedModelOperation {
	if len(operations) == 0 {
		return nil
	}
	resolved := make([]ResolvedModelOperation, len(operations))
	for index, operation := range operations {
		resolved[index] = ResolvedModelOperation{Name: operation.Name}
		resolved[index].Inputs = resolvedExecutionOperationSlots(operation.Inputs)
		resolved[index].Outputs = resolvedExecutionOperationSlots(operation.Outputs)
	}
	return resolved
}

func resolvedExecutionOperationSlots(slots []ModelOperationSlot) []ResolvedModelOperationSlot {
	if len(slots) == 0 {
		return nil
	}
	resolved := make([]ResolvedModelOperationSlot, len(slots))
	for index, slot := range slots {
		resolved[index] = ResolvedModelOperationSlot{
			Name:         slot.Name,
			ContentTypes: append([]string(nil), slot.ContentTypes...),
			Modality:     slot.Modality,
			Required:     slot.Required,
			Repeatable:   cloneBoolPointer(slot.Repeatable),
			MediaTypes:   append([]string(nil), slot.MediaTypes...),
		}
	}
	return resolved
}

func cloneBoolPointer(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func resolvedWorkstationLimits(limits WorkstationLimits) ResolvedWorkstationLimits {
	return ResolvedWorkstationLimits{
		MaxRetries:                    limits.MaxRetries,
		MaxExecutionTime:              limits.MaxExecutionTime,
		MaxGeneratedWorkItems:         limits.MaxGeneratedWorkItems,
		MaxGeneratedWorkItemsArgument: limits.MaxGeneratedWorkItemsArgument,
		MaxGeneratedWorkItemsOffset:   limits.MaxGeneratedWorkItemsArgumentOffset,
	}
}

func effectiveWorkPropagation(config *WorkPropagationConfig) WorkPropagationMode {
	if config == nil || strings.TrimSpace(string(config.Mode)) == "" {
		return WorkPropagationModeOutputAsPayload
	}
	return WorkPropagationMode(strings.TrimSpace(string(config.Mode)))
}

func resolvedOperationBindings(bindings []ModelOperationBinding) []ResolvedModelOperationBinding {
	if len(bindings) == 0 {
		return nil
	}
	resolved := make([]ResolvedModelOperationBinding, len(bindings))
	for index, binding := range bindings {
		resolved[index] = ResolvedModelOperationBinding{
			Slot:           binding.Slot,
			Config:         work.CloneWorkContentParts(binding.Config),
			DefaultContent: work.CloneWorkContentParts(binding.DefaultContent),
		}
		if binding.Selector != nil {
			selector := *binding.Selector
			resolved[index].Selector = &selector
		}
	}
	return resolved
}

func resolvedWorkstationIO(values []IOConfig) []ResolvedWorkstationIO {
	if len(values) == 0 {
		return nil
	}
	resolved := make([]ResolvedWorkstationIO, len(values))
	for index, value := range values {
		resolved[index] = ResolvedWorkstationIO{
			WorkTypeName: value.WorkTypeName,
			StateName:    value.StateName,
		}
		if value.Guard != nil {
			guard := &ResolvedInputGuard{
				Type:        value.Guard.Type,
				MatchInput:  value.Guard.MatchInput,
				ParentInput: value.Guard.ParentInput,
				SpawnedBy:   value.Guard.SpawnedBy,
			}
			resolved[index].Guard = guard
		}
	}
	return resolved
}

func resolvedClassificationRoutes(routes []ClassificationRouteConfig) []ResolvedClassificationRoute {
	if len(routes) == 0 {
		return nil
	}
	resolved := make([]ResolvedClassificationRoute, len(routes))
	for index, route := range routes {
		resolved[index] = ResolvedClassificationRoute{
			Label:   route.Label,
			Outputs: resolvedWorkstationIO(route.Outputs),
		}
	}
	return resolved
}

func resolvedResources(resources []ResourceConfig) []ResolvedResource {
	if len(resources) == 0 {
		return nil
	}
	resolved := make([]ResolvedResource, len(resources))
	for index, resource := range resources {
		resolved[index] = ResolvedResource{
			ID:         resource.ID,
			Name:       resource.Name,
			Type:       resource.Type,
			Capacity:   resource.Capacity,
			Model:      resource.Model,
			Backend:    resource.Backend,
			LoadPolicy: resource.LoadPolicy,
			Provider:   resource.Provider,
		}
	}
	return resolved
}

func cloneExecutionCatalogDiagnostics(values []ExecutionCatalogDiagnostic) []ExecutionCatalogDiagnostic {
	if len(values) == 0 {
		return nil
	}
	return append([]ExecutionCatalogDiagnostic(nil), values...)
}
func executionWorkstationTimeout(workstation FactoryWorkstationConfig) (time.Duration, error) {
	value := strings.TrimSpace(workstation.Limits.MaxExecutionTime)
	if value == "" {
		value = strings.TrimSpace(workstation.Timeout)
	}
	if value == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if timeout <= 0 {
		return 0, nil
	}
	return timeout, nil
}

func isLogicalExecutionWorkstation(workstation FactoryWorkstationConfig) bool {
	typeName := strings.TrimSpace(workstation.Type)
	return strings.EqualFold(typeName, WorkstationTypeLogical) ||
		strings.EqualFold(typeName, "LOGICAL_MOVE")
}

func validateExecutionProvider(value, path string, known map[string]struct{}) *ExecutionCatalogDiagnostic {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if !validExecutionIdentity(value) {
		return pointerExecutionDiagnostic(executionDiagnostic(
			ExecutionCatalogDiagnosticInvalidProvider, path, value, "provider identity is invalid"))
	}
	return validateExecutionReference(known, value, path,
		ExecutionCatalogDiagnosticInvalidProvider, ExecutionCatalogDiagnosticUnknownProvider)
}

func validateExecutionModel(value, path string, known map[string]struct{}) *ExecutionCatalogDiagnostic {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if strings.ContainsAny(value, "\r\n") {
		return pointerExecutionDiagnostic(executionDiagnostic(
			ExecutionCatalogDiagnosticInvalidModel, path, value, "model identity is invalid"))
	}
	return validateExecutionReference(known, value, path,
		ExecutionCatalogDiagnosticInvalidModel, ExecutionCatalogDiagnosticUnknownModel)
}

func validateExecutionReference(
	known map[string]struct{}, value, path string,
	invalidCode, unknownCode ExecutionCatalogDiagnosticCode,
) *ExecutionCatalogDiagnostic {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !validExecutionIdentity(trimmed) {
		return pointerExecutionDiagnostic(executionDiagnostic(invalidCode, path, value, "reference identity is invalid"))
	}
	if len(known) == 0 {
		if invalidCode == ExecutionCatalogDiagnosticInvalidRunner &&
			!knownExecutionRunner(trimmed) {
			return pointerExecutionDiagnostic(executionDiagnostic(
				unknownCode, path, trimmed, "runner is not a supported built-in identity"))
		}
		return nil
	}
	if _, ok := known[trimmed]; ok {
		return nil
	}
	for candidate := range known {
		if strings.EqualFold(candidate, trimmed) {
			return nil
		}
	}
	return pointerExecutionDiagnostic(executionDiagnostic(
		unknownCode, path, trimmed, "reference is not present in the detached catalog"))
}

func knownExecutionRunner(value string) bool {
	switch normalizeExecutionRunner(value) {
	case "codex", "claude", "antigravity":
		return true
	default:
		return false
	}
}

var executionIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func validExecutionIdentity(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == value && trimmed != "" && executionIdentityPattern.MatchString(trimmed) &&
		!strings.Contains(trimmed, "..") && !strings.Contains(trimmed, "--")
}

func normalizeExecutionRunner(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "agy" {
		return "antigravity"
	}
	return trimmed
}

func executionDiagnostic(code ExecutionCatalogDiagnosticCode, path, reference, message string) ExecutionCatalogDiagnostic {
	return ExecutionCatalogDiagnostic{Code: code, Path: path, Reference: reference, Message: message}
}

func pointerExecutionDiagnostic(diagnostic ExecutionCatalogDiagnostic) *ExecutionCatalogDiagnostic {
	return &diagnostic
}

func executionInterpolationDiagnostic(path string, err error) ExecutionCatalogDiagnostic {
	message := "invocation interpolation could not be resolved"
	if argumentError, ok := err.(*work.ArgumentError); ok && argumentError.Parameter != "" {
		message = fmt.Sprintf("invocation parameter %q could not be interpolated", argumentError.Parameter)
	}
	return executionDiagnostic(ExecutionCatalogDiagnosticInvalidInterpolation, path, "", message)
}

func executionDefinitionVersion(definition *FactoryConfig) string {
	if definition == nil || definition.Version == nil {
		return ""
	}
	return fmt.Sprintf("%d@%s", definition.Version.Logical, definition.Version.Physical.UTC().Format(time.RFC3339Nano))
}

func cloneExecutionCatalogReferences(value ExecutionCatalogReferenceCatalog) ExecutionCatalogReferenceCatalog {
	return ExecutionCatalogReferenceCatalog{
		Runners:   cloneIdentitySet(value.Runners),
		Providers: cloneIdentitySet(value.Providers),
		Models:    cloneIdentitySet(value.Models),
	}
}

func cloneIdentitySet(values map[string]struct{}) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]struct{}, len(values))
	for key := range values {
		cloned[normalizeExecutionRunner(key)] = struct{}{}
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	cloned := make(map[string]string, len(values))
	for _, key := range keys {
		cloned[key] = values[key]
	}
	return cloned
}
