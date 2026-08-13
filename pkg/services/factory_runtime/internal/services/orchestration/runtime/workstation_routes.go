package runtime

import (
	"sort"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
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
	capabilities                *workers.Capabilities
	command                     string
	args                        []string
	factoryDirectory            string
	systemPrompt                string
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
) runtimeExecutionSelection {
	selection := initialRuntimeExecutionSelection(request.Execution)
	if lookup, ok := runtimeDefinitionLookup(cfg); ok {
		applyRuntimeDefinitionSelection(cfg, lookup, request, &selection)
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
	selection *runtimeExecutionSelection,
) {
	if selection.workerName == "" {
		selection.workerName = selection.workerType
	}
	worker, workerFound := lookup.Worker(selection.workerName)
	workstation, workstationFound := lookup.Workstation(strings.TrimSpace(request.WorkstationName))
	if !workerFound && workstationFound {
		worker, workerFound = lookup.Worker(workstation.WorkerTypeName)
	}
	if workerFound && worker != nil {
		applyRuntimeWorkerSelection(selection, request.Execution, worker)
	}
	if workstationFound && workstation != nil {
		applyRuntimeWorkstationSelection(selection, workstation)
	}
	applyRuntimeConfigSelection(cfg, selection)
}

func applyRuntimeWorkerSelection(
	selection *runtimeExecutionSelection,
	execution workers.WorkstationExecutionRequest,
	worker *interfaces.FactoryWorkerConfig,
) {
	selection.workerName = firstRuntimeValue(strings.TrimSpace(execution.WorkerName), worker.Name)
	selection.workerType = firstRuntimeValue(worker.Type, selection.workerType)
	selection.providerID = firstRuntimeValue(selection.providerID, worker.ExecutorProvider, worker.Provider)
	selection.model = firstRuntimeValue(selection.model, worker.Model)
	selection.modelProvider = firstRuntimeValue(selection.modelProvider, worker.ModelProvider)
	selection.reasoningEffort = firstRuntimeValue(selection.reasoningEffort, worker.ReasoningEffort)
	selection.modelLocality = strings.TrimSpace(worker.ModelLocality)
	selection.command = firstRuntimeValue(selection.command, worker.Command)
	if len(selection.args) == 0 {
		selection.args = append([]string(nil), worker.Args...)
	}
	selection.stopToken = firstRuntimeValue(selection.stopToken, worker.StopToken)
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
	selection *runtimeExecutionSelection,
	workstation *interfaces.FactoryWorkstationConfig,
) {
	selection.runnerID = firstRuntimeValue(selection.runnerID, workstation.Runner)
	selection.systemPrompt = firstRuntimeValue(selection.systemPrompt, workstation.Body)
	selection.outputSchema = firstRuntimeValue(selection.outputSchema, workstation.OutputSchema)
	selection.outputContract = firstRuntimeValue(selection.outputContract, workstation.OutputContract)
	selection.outputFormat = firstRuntimeValue(selection.outputFormat, workstation.OutcomeFormat)
	selection.workingDirectory = firstRuntimeValue(selection.workingDirectory, workstation.WorkingDirectory)
	selection.workingDirectoryAuthored = selection.workingDirectoryAuthored ||
		strings.TrimSpace(workstation.WorkingDirectory) != ""
	selection.worktree = firstRuntimeValue(selection.worktree, workstation.Worktree)
	selection.environment = mergeRuntimeStringMaps(workstation.Env, selection.environment)
	if selection.timeout <= 0 {
		selection.timeout = parseRuntimeDuration(workstation.Timeout)
	}
	selection.decisionEnvelope = selection.decisionEnvelope ||
		workstation.OutputContract == "decision"
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
	if selection.runnerID == "" && selection.providerID == "" && selection.model == "" {
		selection.runnerID = workers.RunnerIDCodex
	}
	if selection.userMessage == "" {
		selection.userMessage = workInputMessage(inputs)
	}
	if selection.toolExecutionMode == "" {
		selection.toolExecutionMode = workers.RunnerToolExecutionModeDisabled
	}
	selection.environment = mergeRuntimeStringMaps(nil, selection.environment)
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
