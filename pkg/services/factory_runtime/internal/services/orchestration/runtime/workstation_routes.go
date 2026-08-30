package runtime

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type expectedArtifactFileSystem interface {
	Glob(string) ([]string, error)
	Stat(string) (fs.FileInfo, error)
	EvalSymlinks(string) (string, error)
}

func expectedArtifactFileSystemFrom(value any) expectedArtifactFileSystem {
	fileSystem, _ := value.(expectedArtifactFileSystem)
	return fileSystem
}

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
	noop                        bool
	runnerID                    string
	providerID                  string
	executorProvider            string
	model                       string
	modelProvider               string
	modelLocality               string
	reasoningEffort             string
	modelOperation              string
	modelBindings               []workers.ResolvedModelOperationBinding
	classifier                  bool
	scriptClassifier            bool
	capabilities                *workers.Capabilities
	command                     string
	args                        []string
	factoryDirectory            string
	worktreeFactoryDirectory    string
	systemPrompt                string
	promptTemplate              string
	userMessage                 string
	systemPromptProvenance      runtimePromptFieldProvenance
	userPromptProvenance        runtimePromptFieldProvenance
	promptRedaction             *workers.PromptRedaction
	promptTemplateInterpolated  bool
	interpolationError          error
	outputSchema                string
	outputContract              string
	outputFormat                string
	stopToken                   string
	decisionEnvelope            bool
	goalRoutingDecisionEnvelope bool
	toolExecutionMode           workers.RunnerToolExecutionMode
	agentRunHarness             bool
	agentToolPolicy             string
	environment                 map[string]string
	workingDirectory            string
	workingDirectoryAuthored    bool
	worktree                    string
	skipPermissions             bool
	timeout                     time.Duration
}

type runtimeTemplateFieldResolver interface {
	ResolveTemplateFields(
		string,
		map[string]string,
		[]workers.Token,
		*workers.Context,
		string,
	) (*workers.ResolvedTemplateFields, error)
}

// runtimePromptTokens is the prompt-facing projection of a dispatch. Resource
// tokens reserve capacity in Runtime but are not detached Work inputs, so they
// must not change positional .Inputs references in the provider prompt.
func runtimePromptTokens(tokens []workers.Token) []workers.Token {
	if len(tokens) == 0 {
		return nil
	}
	filtered := make([]workers.Token, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == workers.DataTypeResource {
			continue
		}
		filtered = append(filtered, token)
	}
	return filtered
}

// runtimePromptContext gives the prompt renderer the same resolved execution
// fields that the detached Workers target will use. It clones the caller's
// context so prompt rendering cannot mutate request-owned state.
func runtimePromptContext(
	base *workers.Context,
	selection *runtimeExecutionSelection,
) *workers.Context {
	context := base.Clone()
	if context == nil {
		context = &workers.Context{}
	}
	if selection == nil {
		return context
	}
	if selection.workingDirectory != "" {
		context.WorkDirectory = selection.workingDirectory
	}
	if len(selection.environment) > 0 {
		if context.EnvVars == nil {
			context.EnvVars = make(map[string]string, len(selection.environment))
		}
		for key, value := range selection.environment {
			context.EnvVars[key] = value
		}
	}
	return context
}

func resolveRuntimeExecutionSelection(
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	inputTokens []workers.Token,
	inputs []workers.WorkInput,
	invocation *work.InvocationArguments,
) runtimeExecutionSelection {
	selection := initialRuntimeExecutionSelection(request.Execution)
	resolveRuntimeSelectionInvocation(&selection, invocation)
	selection.systemPromptProvenance = runtimePromptFieldProvenanceForText(
		cfg,
		request.Execution.SystemPrompt,
		invocation,
	)
	validateRuntimePromptProvenance(&selection.systemPromptProvenance, selection.systemPrompt)
	selection.userPromptProvenance = runtimePromptFieldProvenanceForText(
		cfg,
		request.Execution.UserMessage,
		invocation,
	)
	validateRuntimePromptProvenance(&selection.userPromptProvenance, selection.userMessage)
	if lookup, ok := runtimeDefinitionLookup(cfg); ok {
		applyRuntimeDefinitionSelection(cfg, lookup, request, inputTokens, invocation, &selection)
	}
	if selection.factoryDirectory == "" && cfg != nil && cfg.workflowContext != nil {
		selection.factoryDirectory = strings.TrimSpace(cfg.workflowContext.FactoryDirectory)
	}
	if cfg != nil {
		finalizeRuntimeExecutionSelection(cfg, &selection, inputs, cfg.expectedArtifactFileSystem)
	} else {
		finalizeRuntimeExecutionSelection(nil, &selection, inputs)
	}
	selection.worktreeFactoryDirectory = selection.factoryDirectory
	if cfg != nil {
		if runtimeLookup, ok := cfg.runtimeConfig.(interfaces.RuntimeConfigLookup); ok && runtimeLookup != nil {
			selection.worktreeFactoryDirectory = firstRuntimeValue(
				strings.TrimSpace(runtimeLookup.RuntimeBaseDir()),
				selection.factoryDirectory,
			)
		}
	}
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
		executorProvider:            strings.TrimSpace(execution.ExecutorProvider),
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
	inputTokens []workers.Token,
	invocation *work.InvocationArguments,
	selection *runtimeExecutionSelection,
) {
	if selection.workerName == "" {
		selection.workerName = selection.workerType
	}
	authoredWorkstation, _ := lookup.Workstation(strings.TrimSpace(request.WorkstationName))
	workstation, workstationFound, err := resolveRuntimeWorkstationDefinition(
		cfg,
		lookup,
		request,
		invocation,
	)
	if err != nil {
		selection.interpolationError = fmt.Errorf("interpolate workstation definition: %w", err)
		return
	}
	worker, workerFound := resolveRuntimeWorkerDefinition(
		lookup,
		selection,
		workstation,
		invocation,
	)
	selection.noop = runtimeSelectionIsTopologyNoop(selection, workerFound, worker)
	if workerFound && worker != nil {
		workerPromptProvenance := runtimeWorkerPromptProvenance(cfg, worker, invocation)
		interpolated, err := interpolateRuntimeWorkerConfig(cfg, worker, invocation)
		if err != nil {
			selection.interpolationError = fmt.Errorf("interpolate worker definition: %w", err)
			return
		}
		restoreRuntimeWorkerFallbacks(worker, interpolated, invocation)
		if err := applyRuntimeWorkerSelection(
			cfg,
			selection,
			request.Execution,
			invocation,
			interpolated,
			workerPromptProvenance,
		); err != nil {
			selection.interpolationError = fmt.Errorf("interpolate worker prompt: %w", err)
			return
		}
		if workstationFound && workstation != nil {
			bindings, err := resolveRuntimeModelOperationBindings(workstation, interpolated, inputTokens)
			if err != nil {
				selection.interpolationError = fmt.Errorf("resolve model operation bindings: %w", err)
				return
			}
			selection.modelBindings = bindings
		}
	}
	if workstationFound && workstation != nil {
		applyRuntimeWorkstationSelection(
			cfg,
			selection,
			invocation,
			workstation,
			runtimeWorkstationPromptProvenance(cfg, authoredWorkstation, invocation),
		)
	}
	applyRuntimeAgentRunSelection(selection, workstation, worker)
	applyRuntimeConfigSelection(cfg, selection)
}

func restoreRuntimeWorkerFallbacks(
	authored *interfaces.FactoryWorkerConfig,
	interpolated *interfaces.FactoryWorkerConfig,
	invocation *work.InvocationArguments,
) {
	if authored == nil || interpolated == nil {
		return
	}
	if strings.TrimSpace(interpolated.Model) == "" {
		interpolated.Model = resolveRuntimeWorkerValue(
			authored.Model,
			invocation,
			interpolated.RuntimeDefaultModel,
		)
	}
	if strings.TrimSpace(interpolated.ModelProvider) == "" {
		interpolated.ModelProvider = resolveRuntimeWorkerValue(
			authored.ModelProvider,
			invocation,
			interpolated.RuntimeDefaultModelProvider,
		)
	}
}

func missingRuntimeWorkerDefinition(
	workerFound bool,
	worker *interfaces.FactoryWorkerConfig,
) bool {
	return !workerFound || worker == nil || strings.TrimSpace(worker.Type) == ""
}

func applyRuntimeWorkerSelection(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	execution workers.WorkstationExecutionRequest,
	invocation *work.InvocationArguments,
	worker *interfaces.FactoryWorkerConfig,
	workerPromptProvenance ...runtimePromptFieldProvenance,
) error {
	var workerPrompt runtimePromptFieldProvenance
	if len(workerPromptProvenance) > 0 {
		workerPrompt = workerPromptProvenance[0]
	}
	selection.workerName = firstRuntimeValue(strings.TrimSpace(execution.WorkerName), worker.Name)
	selection.workerType = firstRuntimeValue(worker.Type, selection.workerType)
	if body, ok := runtimePromptSourceContent(cfg, worker.Name, true, true); ok {
		interpolated, err := interpolateRuntimeWorkerPrompt(cfg, body, invocation)
		if err != nil {
			return err
		}
		selection.systemPrompt = interpolated
		selection.systemPromptProvenance = runtimePromptFieldProvenanceForText(cfg, body, invocation)
		validateRuntimePromptProvenance(&selection.systemPromptProvenance, interpolated)
	} else {
		resolvedPrompt := firstRuntimeValue(
			selection.systemPrompt,
			resolveRuntimeInvocationValue(worker.Body, invocation),
		)
		if selection.systemPrompt == "" {
			selection.systemPromptProvenance = workerPrompt.clone()
			validateRuntimePromptProvenance(&selection.systemPromptProvenance, resolvedPrompt)
		}
		selection.systemPrompt = resolvedPrompt
	}
	executorProvider := resolveRuntimeInvocationValue(worker.ExecutorProvider, invocation)
	selection.executorProvider = firstRuntimeValue(selection.executorProvider, executorProvider)
	selection.providerID = firstRuntimeValue(
		selection.providerID,
		executorProvider,
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
	return nil
}

func applyRuntimeWorkstationSelection(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	invocation *work.InvocationArguments,
	workstation *interfaces.FactoryWorkstationConfig,
	workstationPromptProvenance ...runtimeWorkstationPromptProvenanceSet,
) {
	var workstationPrompt runtimeWorkstationPromptProvenanceSet
	if len(workstationPromptProvenance) > 0 {
		workstationPrompt = workstationPromptProvenance[0]
	}
	applyRuntimeWorkstationPromptSelection(cfg, selection, invocation, workstation, workstationPrompt)
	applyRuntimeWorkstationOutputSelection(selection, invocation, workstation)
}

func applyRuntimeWorkstationPromptSelection(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	invocation *work.InvocationArguments,
	workstation *interfaces.FactoryWorkstationConfig,
	workstationPrompt runtimeWorkstationPromptProvenanceSet,
) {
	selection.runnerID = firstRuntimeValue(
		selection.runnerID,
		resolveRuntimeInvocationValue(workstation.Runner, invocation),
	)
	resolvedSystemPrompt := firstRuntimeValue(
		selection.systemPrompt,
		resolveRuntimeInvocationValue(workstation.Body, invocation),
	)
	if selection.systemPrompt == "" ||
		(workstationPrompt.body.available && workstationPrompt.body.resolved == resolvedSystemPrompt) {
		selection.systemPromptProvenance = workstationPrompt.body.clone()
		validateRuntimePromptProvenance(&selection.systemPromptProvenance, resolvedSystemPrompt)
	}
	selection.systemPrompt = resolvedSystemPrompt
	if prompt, ok := runtimePromptSourceContent(cfg, workstation.Name, false, false); ok {
		selection.promptTemplate = prompt
		selection.userMessage = ""
		selection.promptTemplateInterpolated = false
	} else {
		usedExistingPrompt := strings.TrimSpace(selection.promptTemplate) != ""
		selection.promptTemplate = firstRuntimeValue(
			selection.promptTemplate,
			resolveRuntimeInvocationValue(workstation.PromptTemplate, invocation),
		)
		selection.promptTemplateInterpolated = !usedExistingPrompt &&
			cfg != nil && cfg.invocationInterpolation != nil &&
			strings.TrimSpace(workstation.PromptTemplate) != ""
		if !usedExistingPrompt && selection.userMessage == "" {
			selection.userPromptProvenance = workstationPrompt.promptTemplate.clone()
			validateRuntimePromptProvenance(&selection.userPromptProvenance, selection.promptTemplate)
		}
	}
}

func applyRuntimeWorkstationOutputSelection(
	selection *runtimeExecutionSelection,
	invocation *work.InvocationArguments,
	workstation *interfaces.FactoryWorkstationConfig,
) {
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
	selection.classifier = workstation.Type == interfaces.WorkstationTypeClassify
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
		selection.timeout = parseRuntimeDuration(
			resolveRuntimeInvocationValue(workstation.Limits.MaxExecutionTime, invocation),
		)
	}
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
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	inputs []workers.WorkInput,
	fileSystems ...expectedArtifactFileSystem,
) {
	fileSystem := firstExpectedArtifactFileSystem(fileSystems)
	// SCRIPT_WRAP is the legacy command-wrapper spelling for a model-backed
	// worker. Detached Workers execution needs the canonical Providers identity
	// carried by modelProvider; retaining SCRIPT_WRAP here makes the stateless
	// agent boundary reject an otherwise valid authored worker after a runtime
	// replacement.
	if strings.EqualFold(strings.TrimSpace(selection.providerID), "script_wrap") {
		selection.providerID = strings.TrimSpace(selection.modelProvider)
	}
	// ACP is an authored execution mechanism, not a Providers catalog ID.
	// Detached Workers authorizes Target.Provider.ID before it reaches the
	// agent runner, so carry the concrete model-provider identity across this
	// boundary and use it for runner selection as well.
	if isACPProviderMarker(selection.executorProvider) ||
		isACPProviderMarker(selection.providerID) {
		selection.executorProvider = workers.ExecutorProviderACP
		if modelProvider := strings.TrimSpace(selection.modelProvider); modelProvider != "" {
			selection.providerID = modelProvider
			selection.runnerID = modelProvider
		} else {
			// Keep the authored ACP marker as an invalid provider target so the
			// Workers/Providers boundary returns its typed unknown-provider
			// rejection instead of silently falling back to Codex.
			selection.providerID = workers.ExecutorProviderACP
			selection.runnerID = workers.ExecutorProviderACP
		}
	}
	if selection.providerID == "" {
		selection.providerID = selection.modelProvider
	}
	if selection.modelProvider == "" {
		selection.modelProvider = selection.providerID
	}
	finalizeRuntimeRunnerSelection(selection)
	finalizeRuntimeWorkspaceSelection(cfg, selection, fileSystem)
	if selection.userMessage == "" && selection.promptTemplate == "" {
		selection.userMessage = workInputMessage(inputs)
	}
	if selection.toolExecutionMode == "" {
		selection.toolExecutionMode = workers.RunnerToolExecutionModeDisabled
	}
	selection.environment = mergeRuntimeStringMaps(nil, selection.environment)
}

func isACPProviderMarker(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), workers.ExecutorProviderACP)
}

func firstExpectedArtifactFileSystem(values []expectedArtifactFileSystem) expectedArtifactFileSystem {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

func finalizeRuntimeRunnerSelection(selection *runtimeExecutionSelection) {
	// A request-selected script worker has no provider/model target. A factory
	// runner default can still be present on the workstation, so resolve the
	// script route after authored worker/workstation selection but before the
	// generic provider fallback.
	if strings.TrimSpace(selection.command) != "" &&
		selection.providerID == "" && selection.model == "" {
		selection.runnerID = "script"
	}
	if strings.EqualFold(strings.TrimSpace(selection.workerType), interfaces.WorkerTypeScript) &&
		strings.TrimSpace(selection.command) != "" &&
		selection.providerID == "" && selection.model == "" {
		// SCRIPT_WORKER is an execution contract, not a model-provider alias.
		// A factory-level runner default must not redirect its command through
		// the agent/provider path.
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
	selection.executorProvider = resolveRuntimeInvocationValue(selection.executorProvider, invocation)
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

func resolveRuntimePath(baseDir, value string, fileSystem expectedArtifactFileSystem) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	normalized := filepath.FromSlash(value)
	if filepath.IsAbs(normalized) && (!portableRuntimeRootedPath(value) || pathExists(normalized, fileSystem)) {
		return filepath.Clean(normalized)
	}
	if portableRuntimeRootedPath(value) && strings.TrimSpace(baseDir) != "" {
		return filepath.Clean(filepath.Join(baseDir, normalized))
	}
	if fileSystem != nil && strings.TrimSpace(baseDir) != "" && !filepath.IsAbs(normalized) {
		return filepath.Clean(filepath.Join(baseDir, normalized))
	}
	return filepath.Clean(normalized)
}

func portableRuntimeRootedPath(value string) bool {
	return filepath.VolumeName(value) == "" && strings.HasPrefix(value, "/")
}

func pathExists(value string, fileSystem expectedArtifactFileSystem) bool {
	if fileSystem == nil {
		return false
	}
	_, err := fileSystem.Stat(value)
	return err == nil || !errors.Is(err, fs.ErrNotExist)
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

func workerTokenPlaceKey(token workers.Token) string {
	prefix := strings.TrimSpace(token.Color.WorkTypeID)
	if prefix == "" {
		prefix = strings.TrimSpace(token.Color.Name)
	}
	stateName := strings.TrimSpace(token.State)
	if prefix == "" {
		return stateName
	}
	if stateName == "" {
		return prefix
	}
	return prefix + ":" + stateName
}
