package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func runtimeWorkstationRouteNames(
	net *state.Net,
	executors map[string]workers.WorkerExecutor,
) []string {
	routes := make(map[string]struct{}, len(executors))
	for name := range executors {
		if name != "" {
			routes[name] = struct{}{}
		}
	}
	if net != nil {
		for id, transition := range net.Transitions {
			if id != "" {
				routes[id] = struct{}{}
			}
			if transition == nil {
				continue
			}
			if transition.Name != "" {
				routes[transition.Name] = struct{}{}
			}
			if transition.WorkerType != "" {
				routes[transition.WorkerType] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(routes))
	for name := range routes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type runtimeExecutionSelection struct {
	workerName                  string
	workerType                  string
	runnerID                    string
	providerID                  string
	model                       string
	modelProvider               string
	modelLocality               string
	reasoningEffort             string
	modelOperation              string
	scriptClassifier            bool
	capabilities                *workers.Capabilities
	command                     string
	args                        []string
	factoryDirectory            string
	systemPrompt                string
	promptTemplate              string
	userMessage                 string
	outputSchema                string
	outputContract              string
	outputFormat                string
	stopToken                   string
	decisionEnvelope            bool
	goalRoutingDecisionEnvelope bool
	toolExecutionMode           workers.RunnerToolExecutionMode
	environment                 map[string]string
	workingDirectory            string
	workingDirectoryAuthored    bool
	worktree                    string
	skipPermissions             bool
	timeout                     time.Duration
}

func resolveRuntimeExecutionSelection(
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	inputs []workers.WorkInput,
	invocation *work.InvocationArguments,
) runtimeExecutionSelection {
	selection := initialRuntimeExecutionSelection(request.Execution)
	resolveRuntimeSelectionInvocation(&selection, invocation)
	if lookup, ok := runtimeDefinitionLookup(cfg); ok {
		applyRuntimeDefinitionSelection(cfg, lookup, request, invocation, &selection)
	}
	if selection.factoryDirectory == "" && cfg != nil && cfg.workflowContext != nil {
		selection.factoryDirectory = strings.TrimSpace(cfg.workflowContext.FactoryDirectory)
	}
	finalizeRuntimeExecutionSelection(&selection, inputs)
	return selection
}

func initialRuntimeExecutionSelection(
	execution workers.WorkstationExecutionRequest,
) runtimeExecutionSelection {
	return runtimeExecutionSelection{
		workerName:                  strings.TrimSpace(execution.WorkerName),
		workerType:                  firstRuntimeValue(execution.WorkerType, execution.Dispatch.WorkerType),
		runnerID:                    strings.TrimSpace(execution.RunnerID),
		providerID:                  strings.TrimSpace(execution.ExecutorProvider),
		model:                       strings.TrimSpace(execution.Model),
		modelProvider:               strings.TrimSpace(execution.ModelProvider),
		reasoningEffort:             strings.TrimSpace(execution.ReasoningEffort),
		modelOperation:              strings.TrimSpace(execution.ModelOperation),
		capabilities:                cloneRuntimeCapabilities(execution.Capabilities),
		command:                     execution.Command,
		args:                        append([]string(nil), execution.Args...),
		factoryDirectory:            strings.TrimSpace(execution.FactoryDirectory),
		systemPrompt:                execution.SystemPrompt,
		userMessage:                 execution.UserMessage,
		outputSchema:                execution.OutputSchema,
		outputContract:              execution.OutputContract,
		outputFormat:                execution.OutputFormat,
		stopToken:                   execution.StopToken,
		decisionEnvelope:            execution.DecisionEnvelope,
		goalRoutingDecisionEnvelope: execution.GoalRoutingDecisionEnvelope,
		environment:                 cloneRuntimeStringMap(execution.EnvVars),
		workingDirectory:            strings.TrimSpace(execution.WorkingDirectory),
		workingDirectoryAuthored:    execution.WorkingDirectoryAuthored,
		worktree:                    strings.TrimSpace(execution.Worktree),
		skipPermissions:             execution.SkipPermissions,
		timeout:                     execution.Timeout,
	}
}

func applyRuntimeDefinitionSelection(
	cfg *runtimeConfig,
	lookup interfaces.RuntimeDefinitionLookup,
	request workers.WorkstationDispatchRequest,
	invocation *work.InvocationArguments,
	selection *runtimeExecutionSelection,
) {
	if selection.workerName == "" {
		selection.workerName = selection.workerType
	}
	worker, workerFound := lookup.Worker(selection.workerName)
	workstation, workstationFound := lookup.Workstation(strings.TrimSpace(request.WorkstationName))
	if !workerFound && workstationFound {
		worker, workerFound = lookup.Worker(resolveRuntimeInvocationValue(workstation.WorkerTypeName, invocation))
	}
	if workerFound && worker != nil {
		applyRuntimeWorkerSelection(cfg, selection, request.Execution, invocation, worker)
	}
	if workstationFound && workstation != nil {
		applyRuntimeWorkstationSelection(cfg, selection, invocation, workstation)
	}
	applyRuntimeConfigSelection(cfg, selection)
}

func applyRuntimeWorkerSelection(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	execution workers.WorkstationExecutionRequest,
	invocation *work.InvocationArguments,
	worker *interfaces.FactoryWorkerConfig,
) {
	selection.workerName = firstRuntimeValue(strings.TrimSpace(execution.WorkerName), worker.Name)
	selection.workerType = firstRuntimeValue(worker.Type, selection.workerType)
	if body, ok := runtimePromptSourceContent(cfg, worker.Name, true, true); ok {
		selection.systemPrompt = body
	} else {
		selection.systemPrompt = firstRuntimeValue(
			selection.systemPrompt,
			resolveRuntimeInvocationValue(worker.Body, invocation),
		)
	}
	selection.providerID = firstRuntimeValue(
		selection.providerID,
		resolveRuntimeInvocationValue(worker.ExecutorProvider, invocation),
		resolveRuntimeInvocationValue(worker.Provider, invocation),
	)
	selection.model = firstRuntimeValue(
		selection.model,
		resolveRuntimeWorkerValue(worker.Model, invocation, worker.RuntimeDefaultModel),
	)
	selection.modelProvider = firstRuntimeValue(
		selection.modelProvider,
		resolveRuntimeWorkerValue(worker.ModelProvider, invocation, worker.RuntimeDefaultModelProvider),
	)
	selection.reasoningEffort = firstRuntimeValue(
		selection.reasoningEffort,
		resolveRuntimeInvocationValue(worker.ReasoningEffort, invocation),
	)
	selection.modelLocality = strings.TrimSpace(resolveRuntimeInvocationValue(worker.ModelLocality, invocation))
	selection.command = firstRuntimeValue(
		selection.command,
		resolveRuntimeInvocationValue(worker.Command, invocation),
	)
	if len(selection.args) == 0 {
		selection.args = resolveRuntimeInvocationArgs(worker.Args, invocation)
	}
	selection.stopToken = firstRuntimeValue(
		selection.stopToken,
		resolveRuntimeInvocationValue(worker.StopToken, invocation),
	)
	selection.skipPermissions = selection.skipPermissions || worker.SkipPermissions
	if selection.timeout <= 0 {
		selection.timeout = worker.TimeoutDuration()
	}
	if worker.AgentTools != nil && strings.TrimSpace(worker.AgentTools.Policy) != "" &&
		!strings.EqualFold(worker.AgentTools.Policy, "DISABLED") {
		selection.toolExecutionMode = workers.RunnerToolExecutionModeRequired
	}
}

func applyRuntimeWorkstationSelection(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	invocation *work.InvocationArguments,
	workstation *interfaces.FactoryWorkstationConfig,
) {
	selection.runnerID = firstRuntimeValue(
		selection.runnerID,
		resolveRuntimeInvocationValue(workstation.Runner, invocation),
	)
	selection.systemPrompt = firstRuntimeValue(
		selection.systemPrompt,
		resolveRuntimeInvocationValue(workstation.Body, invocation),
	)
	if prompt, ok := runtimePromptSourceContent(cfg, workstation.Name, false, false); ok {
		if source, sourceOK := runtimePromptSource(cfg, workstation.Name, false); sourceOK && source.IsTemplate {
			selection.promptTemplate = prompt
			selection.userMessage = ""
		} else {
			selection.userMessage = prompt
		}
	} else {
		selection.promptTemplate = firstRuntimeValue(
			selection.promptTemplate,
			resolveRuntimeInvocationValue(workstation.PromptTemplate, invocation),
		)
	}
	selection.outputSchema = firstRuntimeValue(
		selection.outputSchema,
		resolveRuntimeInvocationValue(workstation.OutputSchema, invocation),
	)
	selection.outputContract = firstRuntimeValue(
		selection.outputContract,
		resolveRuntimeInvocationValue(workstation.OutputContract, invocation),
	)
	selection.modelOperation = firstRuntimeValue(
		selection.modelOperation,
		resolveRuntimeInvocationValue(workstation.Operation, invocation),
	)
	selection.scriptClassifier = workstation.Type == interfaces.WorkstationTypeClassify &&
		selection.workerType == interfaces.WorkerTypeScript
	selection.outputFormat = firstRuntimeValue(
		selection.outputFormat,
		resolveRuntimeInvocationValue(workstation.OutcomeFormat, invocation),
	)
	selection.workingDirectory = firstRuntimeValue(
		selection.workingDirectory,
		resolveRuntimeInvocationValue(workstation.WorkingDirectory, invocation),
	)
	selection.workingDirectoryAuthored = selection.workingDirectoryAuthored ||
		strings.TrimSpace(workstation.WorkingDirectory) != ""
	selection.worktree = firstRuntimeValue(
		selection.worktree,
		resolveRuntimeInvocationValue(workstation.Worktree, invocation),
	)
	selection.environment = mergeRuntimeStringMaps(
		resolveRuntimeInvocationMap(workstation.Env, invocation),
		selection.environment,
	)
	if selection.timeout <= 0 {
		selection.timeout = parseRuntimeDuration(resolveRuntimeInvocationValue(workstation.Timeout, invocation))
	}
	selection.decisionEnvelope = selection.decisionEnvelope ||
		workstation.OutputContract == "decision" ||
		strings.EqualFold(workstation.OutcomeFormat, interfaces.DecisionEnvelopeOutcomeFormat)
	selection.goalRoutingDecisionEnvelope = selection.goalRoutingDecisionEnvelope ||
		(strings.EqualFold(workstation.OutcomeFormat, interfaces.DecisionEnvelopeOutcomeFormat) &&
			len(workstation.ClassificationRoutes) > 0)
}

func runtimePromptSource(
	cfg *runtimeConfig,
	name string,
	workerSource bool,
) (interfaces.PromptSource, bool) {
	if cfg == nil || cfg.runtimeConfig == nil {
		return interfaces.PromptSource{}, false
	}
	lookup, ok := cfg.runtimeConfig.(interfaces.RuntimePromptSourceLookup)
	if !ok || lookup == nil {
		return interfaces.PromptSource{}, false
	}
	if workerSource {
		return lookup.WorkerPromptSource(name)
	}
	return lookup.WorkstationPromptSource(name)
}

func runtimePromptSourceContent(
	cfg *runtimeConfig,
	name string,
	bodySource bool,
	workerSource bool,
) (string, bool) {
	source, found := runtimePromptSource(cfg, name, workerSource)
	if !found || cfg == nil || cfg.promptSourceReader == nil || strings.TrimSpace(source.Path) == "" {
		return "", false
	}
	data, err := cfg.promptSourceReader(source.Path)
	if err != nil {
		return "", false
	}
	if !bodySource && source.IsTemplate {
		return string(data), true
	}
	return runtimeAuthoredPromptBody(string(data)), true
}

func runtimeAuthoredPromptBody(content string) string {
	if strings.HasPrefix(content, "---\r\n") {
		content = strings.Replace(content, "\r\n", "\n", -1)
	}
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	rest := content[len("---\n"):]
	if index := strings.Index(rest, "\n---\n"); index >= 0 {
		return strings.TrimSpace(rest[index+len("\n---\n"):])
	}
	if strings.HasSuffix(strings.TrimSpace(rest), "---") {
		return ""
	}
	return content
}

func applyRuntimeConfigSelection(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
) {
	configLookup, ok := cfg.runtimeConfig.(interfaces.RuntimeConfigLookup)
	if !ok || selection.factoryDirectory != "" {
		return
	}
	selection.factoryDirectory = strings.TrimSpace(configLookup.FactoryDir())
	if selection.runnerID == "" {
		if factoryConfig := configLookup.FactoryConfig(); factoryConfig != nil {
			selection.runnerID = strings.TrimSpace(factoryConfig.Runner)
		}
	}
}

func finalizeRuntimeExecutionSelection(
	selection *runtimeExecutionSelection,
	inputs []workers.WorkInput,
) {
	if selection.providerID == "" {
		selection.providerID = selection.modelProvider
	}
	if selection.modelProvider == "" {
		selection.modelProvider = selection.providerID
	}
	// A request-selected script worker has no provider/model target. A factory
	// runner default can still be present on the workstation, so resolve the
	// script route after authored worker/workstation selection but before the
	// generic provider fallback.
	if strings.TrimSpace(selection.command) != "" &&
		selection.providerID == "" && selection.model == "" {
		selection.runnerID = "script"
	}
	if selection.runnerID == "" && selection.model != "" &&
		selection.workerType != interfaces.WorkerTypeInference {
		// MODEL_WORKER is the provider-backed agent route. Resolve its runner
		// from the authored executor/model provider, preserving the shared
		// compatibility aliases and Codex default when neither is selectable.
		selection.runnerID = workers.ResolveRunnerSelection(
			"", "", firstRuntimeValue(selection.providerID, selection.modelProvider),
		).RunnerID
	}
	if selection.runnerID == "" && selection.providerID == "" && selection.model == "" {
		selection.runnerID = workers.RunnerIDCodex
	}
	if selection.workingDirectory == "" {
		// Detached execution must carry the same default workspace that the
		// legacy workstation executor derived from RuntimeConfig.
		selection.workingDirectory = selection.factoryDirectory
	}
	selection.workingDirectory = resolveRuntimePath(selection.factoryDirectory, selection.workingDirectory)
	if selection.userMessage == "" && selection.promptTemplate == "" {
		selection.userMessage = workInputMessage(inputs)
	}
	if selection.toolExecutionMode == "" {
		selection.toolExecutionMode = workers.RunnerToolExecutionModeDisabled
	}
	selection.environment = mergeRuntimeStringMaps(nil, selection.environment)
}

func resolveRuntimeSelectionInvocation(
	selection *runtimeExecutionSelection,
	invocation *work.InvocationArguments,
) {
	if selection == nil {
		return
	}
	selection.workerName = resolveRuntimeInvocationValue(selection.workerName, invocation)
	selection.workerType = resolveRuntimeInvocationValue(selection.workerType, invocation)
	selection.runnerID = resolveRuntimeInvocationValue(selection.runnerID, invocation)
	selection.providerID = resolveRuntimeInvocationValue(selection.providerID, invocation)
	selection.model = resolveRuntimeInvocationValue(selection.model, invocation)
	selection.modelProvider = resolveRuntimeInvocationValue(selection.modelProvider, invocation)
	selection.modelLocality = resolveRuntimeInvocationValue(selection.modelLocality, invocation)
	selection.reasoningEffort = resolveRuntimeInvocationValue(selection.reasoningEffort, invocation)
	selection.modelOperation = resolveRuntimeInvocationValue(selection.modelOperation, invocation)
	selection.command = resolveRuntimeInvocationValue(selection.command, invocation)
	selection.factoryDirectory = resolveRuntimeInvocationValue(selection.factoryDirectory, invocation)
	selection.systemPrompt = resolveRuntimeInvocationValue(selection.systemPrompt, invocation)
	selection.promptTemplate = resolveRuntimeInvocationValue(selection.promptTemplate, invocation)
	selection.userMessage = resolveRuntimeInvocationValue(selection.userMessage, invocation)
	selection.outputSchema = resolveRuntimeInvocationValue(selection.outputSchema, invocation)
	selection.outputContract = resolveRuntimeInvocationValue(selection.outputContract, invocation)
	selection.outputFormat = resolveRuntimeInvocationValue(selection.outputFormat, invocation)
	selection.stopToken = resolveRuntimeInvocationValue(selection.stopToken, invocation)
	selection.workingDirectory = resolveRuntimeInvocationValue(selection.workingDirectory, invocation)
	selection.worktree = resolveRuntimeInvocationValue(selection.worktree, invocation)
}

func resolveRuntimeWorkerValue(
	authored string,
	invocation *work.InvocationArguments,
	fallback string,
) string {
	value := resolveRuntimeInvocationValue(authored, invocation)
	if strings.TrimSpace(value) == "" {
		if _, exact := runtimeInvocationParameter(authored); exact {
			return fallback
		}
	}
	return value
}

func resolveRuntimeInvocationValue(
	authored string,
	invocation *work.InvocationArguments,
) string {
	name, exact := runtimeInvocationParameter(authored)
	if !exact {
		return authored
	}
	if invocation == nil || invocation.Arguments == nil {
		return ""
	}
	argument, ok := invocation.Arguments[name]
	if !ok || len(argument.Values) == 0 {
		return ""
	}
	return argument.Values[0]
}

func resolveRuntimeInvocationArgs(
	authored []string,
	invocation *work.InvocationArguments,
) []string {
	if len(authored) == 0 {
		return nil
	}
	resolved := make([]string, len(authored))
	for index, value := range authored {
		resolved[index] = resolveRuntimeInvocationValue(value, invocation)
	}
	return resolved
}

func resolveRuntimeInvocationMap(
	authored map[string]string,
	invocation *work.InvocationArguments,
) map[string]string {
	if len(authored) == 0 {
		return nil
	}
	resolved := make(map[string]string, len(authored))
	for key, value := range authored {
		resolved[key] = resolveRuntimeInvocationValue(value, invocation)
	}
	return resolved
}

func runtimeInvocationParameter(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 4 || !strings.HasPrefix(trimmed, "${") || !strings.HasSuffix(trimmed, "}") {
		return "", false
	}
	name := strings.TrimSpace(trimmed[2 : len(trimmed)-1])
	return name, name != ""
}

func renderRuntimePrompt(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	tokens []workers.Token,
	workflowContext *workers.Context,
	inputs []workers.WorkInput,
) error {
	if selection == nil || selection.userMessage != "" || selection.promptTemplate == "" {
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

func resolveRuntimePath(baseDir, value string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	normalized := filepath.FromSlash(value)
	if filepath.IsAbs(normalized) && (!portableRuntimeRootedPath(value) || pathExists(normalized)) {
		return filepath.Clean(normalized)
	}
	if portableRuntimeRootedPath(value) && strings.TrimSpace(baseDir) != "" {
		return filepath.Clean(filepath.Join(baseDir, normalized))
	}
	return filepath.Clean(normalized)
}

func portableRuntimeRootedPath(value string) bool {
	return filepath.VolumeName(value) == "" && strings.HasPrefix(value, "/")
}

func pathExists(value string) bool {
	_, err := os.Stat(value)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func runtimeDefinitionLookup(cfg *runtimeConfig) (interfaces.RuntimeDefinitionLookup, bool) {
	if cfg == nil || cfg.runtimeConfig == nil {
		return nil, false
	}
	lookup, ok := cfg.runtimeConfig.(interfaces.RuntimeDefinitionLookup)
	return lookup, ok && lookup != nil
}

func firstRuntimeValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mergeRuntimeStringMaps(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := cloneRuntimeStringMap(base)
	if merged == nil {
		merged = make(map[string]string, len(override))
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func parseRuntimeDuration(value string) time.Duration {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || duration < 0 {
		return 0
	}
	return duration
}

func workInputMessage(inputs []workers.WorkInput) string {
	for _, input := range inputs {
		for _, part := range input.Content {
			if text := strings.TrimSpace(part.Text); text != "" {
				return text
			}
		}
	}
	return ""
}

func cloneRuntimeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneRuntimeCapabilities(value *workers.Capabilities) *workers.Capabilities {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
