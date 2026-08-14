package runtime

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func resolveRuntimeWorkstationDefinition(
	cfg *runtimeConfig,
	lookup interfaces.RuntimeDefinitionLookup,
	request workers.WorkstationDispatchRequest,
	invocation *work.InvocationArguments,
) (*interfaces.FactoryWorkstationConfig, bool, error) {
	workstation, found := lookup.Workstation(strings.TrimSpace(request.WorkstationName))
	if !found || workstation == nil {
		return workstation, found, nil
	}
	interpolated, err := interpolateRuntimeWorkstationConfig(cfg, workstation, invocation)
	return interpolated, found, err
}

func resolveRuntimeWorkerDefinition(
	lookup interfaces.RuntimeDefinitionLookup,
	selection *runtimeExecutionSelection,
	workstation *interfaces.FactoryWorkstationConfig,
	invocation *work.InvocationArguments,
) (*interfaces.FactoryWorkerConfig, bool) {
	worker, found := lookup.Worker(selection.workerName)
	if found || workstation == nil {
		return worker, found
	}
	workstationWorkerName := resolveRuntimeInvocationValue(workstation.WorkerTypeName, invocation)
	if selection.workerName == "" {
		selection.workerName = strings.TrimSpace(workstationWorkerName)
	}
	return lookup.Worker(workstationWorkerName)
}

func runtimeSelectionIsTopologyNoop(
	selection *runtimeExecutionSelection,
	workerFound bool,
	worker *interfaces.FactoryWorkerConfig,
) bool {
	if !missingRuntimeWorkerDefinition(workerFound, worker) || selection.workerName == "" {
		return false
	}
	return selection.providerID == "" && selection.model == "" &&
		selection.modelProvider == "" && selection.command == ""
}

func interpolateRuntimeWorkerConfig(
	cfg *runtimeConfig,
	worker *interfaces.FactoryWorkerConfig,
	invocation *work.InvocationArguments,
) (*interfaces.FactoryWorkerConfig, error) {
	if worker == nil || cfg == nil || cfg.invocationInterpolation == nil {
		return worker, nil
	}
	interpolated, err := cfg.invocationInterpolation.InterpolateWorkerConfig(
		*worker,
		invocation,
		cfg.invocationFileReader,
	)
	if err != nil {
		return nil, err
	}
	return &interpolated, nil
}

func interpolateRuntimeWorkstationConfig(
	cfg *runtimeConfig,
	workstation *interfaces.FactoryWorkstationConfig,
	invocation *work.InvocationArguments,
) (*interfaces.FactoryWorkstationConfig, error) {
	if workstation == nil || cfg == nil || cfg.invocationInterpolation == nil {
		return workstation, nil
	}
	interpolated, err := cfg.invocationInterpolation.InterpolateWorkstationConfig(
		*workstation,
		invocation,
		cfg.invocationFileReader,
	)
	if err != nil {
		return nil, err
	}
	return &interpolated, nil
}

func renderRuntimePrompt(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	tokens []workers.Token,
	workflowContext *workers.Context,
	inputs []workers.WorkInput,
	invocation *work.InvocationArguments,
) error {
	if selection == nil {
		return nil
	}
	if selection.interpolationError != nil {
		return selection.interpolationError
	}
	if err := interpolateRuntimePromptTemplate(cfg, selection, invocation); err != nil {
		return err
	}
	if err := renderRuntimePromptMessage(cfg, selection, tokens, workflowContext, inputs); err != nil {
		return err
	}
	return resolveRuntimeTemplateFields(cfg, selection, tokens, workflowContext)
}

func interpolateRuntimePromptTemplate(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	invocation *work.InvocationArguments,
) error {
	if cfg == nil || cfg.invocationInterpolation == nil ||
		selection.promptTemplate == "" || selection.promptTemplateInterpolated {
		return nil
	}
	interpolated, err := cfg.invocationInterpolation.InterpolateWorkstationConfig(
		interfaces.FactoryWorkstationConfig{PromptTemplate: selection.promptTemplate},
		invocation,
		cfg.invocationFileReader,
	)
	if err != nil {
		return fmt.Errorf("interpolate workstation prompt: %w", err)
	}
	selection.promptTemplate = interpolated.PromptTemplate
	selection.promptTemplateInterpolated = true
	return nil
}

func renderRuntimePromptMessage(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	tokens []workers.Token,
	workflowContext *workers.Context,
	inputs []workers.WorkInput,
) error {
	if selection.userMessage != "" || selection.promptTemplate == "" {
		return nil
	}
	if cfg == nil || cfg.promptRenderer == nil {
		// Legacy test and adapter callers may not provide the optional renderer.
		// Preserve their detached execution behavior by using the same payload
		// fallback as an empty authored prompt.
		selection.userMessage = workInputMessage(inputs)
		return nil
	}
	rendered, err := cfg.promptRenderer.RenderPrompt(
		selection.promptTemplate,
		tokens,
		workflowContext,
	)
	if err != nil {
		return fmt.Errorf("render workstation prompt: %w", err)
	}
	selection.userMessage = rendered
	return nil
}

func resolveRuntimeTemplateFields(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	tokens []workers.Token,
	workflowContext *workers.Context,
) error {
	if cfg == nil || cfg.templateFieldResolver == nil {
		return nil
	}
	resolved, err := cfg.templateFieldResolver.ResolveTemplateFields(
		selection.workingDirectory,
		selection.environment,
		tokens,
		workflowContext,
		selection.worktree,
	)
	if err != nil {
		return fmt.Errorf("resolve workstation execution fields: %w", err)
	}
	if resolved != nil {
		selection.workingDirectory = resolved.WorkingDirectory
		selection.worktree = resolved.Worktree
		selection.environment = cloneRuntimeStringMap(resolved.Env)
	}
	return nil
}
