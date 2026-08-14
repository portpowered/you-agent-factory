package service

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func resolveRunnerIdentity(target workers.ExecutionTarget) string {
	runnerID := workers.NormalizeRunnerID(target.RunnerID)
	switch runnerID {
	case runners.ScriptIdentity, runners.InferenceIdentity, runners.AgentIdentity, runners.MockIdentity:
		return runnerID
	}
	if runnerID != "" {
		return runners.AgentIdentity
	}
	if strings.TrimSpace(target.Model.Name) != "" {
		return runners.InferenceIdentity
	}
	return runners.AgentIdentity
}

func adaptRunnerRequest(
	request workers.ExecuteRequest,
	identity string,
	temporaryFiles workers.TemporaryFileSystem,
) workers.RunnerExecutionRequest {
	context := adaptWorkflowContext(request)
	inputTokens := inputTokensFromWorkInputs(request.Input.Work)
	worktree := strings.TrimSpace(request.Target.Workspace.Worktree)

	return workers.RunnerExecutionRequest{
		Dispatch:                     adaptDispatch(request, inputTokens),
		Correlation:                  request.Correlation,
		WorkerName:                   request.Target.WorkerName,
		WorkerType:                   firstNonEmpty(request.Target.WorkerType, request.Target.WorkerName),
		WorkstationType:              request.Target.WorkstationName,
		RunnerID:                     runnerIDForRequest(request, identity),
		ExecutorProvider:             providerIdentity(request.Target.Provider),
		ModelOperation:               request.Input.ModelOperation,
		ModelBindings:                workers.CloneResolvedModelOperationBindings(request.Input.ModelBindings),
		InputTokens:                  workers.InputTokens(inputTokens...),
		SystemPrompt:                 request.Target.Prompt.SystemPrompt,
		UserMessage:                  request.Target.Prompt.UserMessage,
		OutputSchema:                 request.Target.Prompt.OutputSchema,
		ToolExecutionMode:            request.Target.Tools.ExecutionMode,
		RequiredOptionalCapabilities: requiredOptionalCapabilities(request, identity),
		EnvVars:                      context.envVars,
		ProcessEnvironment:           append([]string(nil), request.Target.Environment.ProcessEnvironment...),
		Worktree:                     worktree,
		WorkingDirectory:             context.workingDirectory,
		Model:                        request.Target.Model.Name,
		ModelProvider:                firstNonEmpty(request.Target.Model.Provider, providerIdentity(request.Target.Provider)),
		ReasoningEffort:              request.Target.Model.ReasoningEffort,
		ModelLocality:                request.Target.Model.Locality,
		Command:                      request.Target.Command,
		Args:                         append([]string(nil), request.Target.Args...),
		FactoryDirectory:             context.factoryDirectory,
		OutputContract:               request.Target.Output.Contract,
		OutputFormat:                 request.Target.Output.Format,
		StopToken:                    request.Target.Output.StopToken,
		DecisionEnvelope:             request.Target.Output.DecisionEnvelope,
		GoalRoutingDecisionEnvelope:  request.Target.Output.GoalRoutingDecisionEnvelope,
		PrintTimeout:                 request.Target.Timeout,
		SessionID:                    providerSessionID(request),
		ProjectID:                    context.projectID,
		WorkflowContext:              context.workflow,
		SkipPermissions:              request.Target.Permissions.SkipPermissions,
		TemporaryFiles:               temporaryFiles,
	}
}

// requiredOptionalCapabilities reconstructs the runner requirements that the
// legacy workstation executor derived immediately before provider execution.
// Detached Execute callers carry the complete request instead of a prepared
// workstation request, so this normalization must happen before runner
// resolution as well as when adapting the provider request.
func requiredOptionalCapabilities(
	request workers.ExecuteRequest,
	identity string,
) []workers.RunnerOptionalCapability {
	capabilities := append(
		[]workers.RunnerOptionalCapability(nil),
		request.Target.Tools.RequiredOptionalCapabilities...,
	)
	if identity != runners.AgentIdentity {
		return capabilities
	}
	if strings.TrimSpace(request.Target.Prompt.OutputSchema) != "" {
		capabilities = appendRunnerCapabilityIfMissing(
			capabilities,
			workers.RunnerOptionalCapabilityStructuredOutput,
		)
	}
	workingDirectory := firstNonEmpty(
		request.Target.Environment.WorkingDirectory,
		request.Target.Workspace.WorkingDirectory,
	)
	if request.Target.Environment.WorkingDirectorySet && workingDirectory != "" {
		capabilities = appendRunnerCapabilityIfMissing(
			capabilities,
			workers.RunnerOptionalCapabilityWorkingDirectory,
		)
	}
	if worktreeRequiresRunnerCapability(request, workingDirectory) {
		capabilities = appendRunnerCapabilityIfMissing(
			capabilities,
			workers.RunnerOptionalCapabilityWorktree,
		)
	}
	if requestHasImageInput(request.Input.Work) {
		capabilities = appendRunnerCapabilityIfMissing(
			capabilities,
			workers.RunnerOptionalCapabilityImageInput,
		)
	}
	if request.Input.Resume != nil && firstNonEmpty(
		request.Input.Resume.ProviderSessionID,
		request.Input.Resume.ExternalRef,
	) != "" {
		capabilities = appendRunnerCapabilityIfMissing(
			capabilities,
			workers.RunnerOptionalCapabilitySessionResume,
		)
	}
	return capabilities
}

func worktreeRequiresRunnerCapability(
	request workers.ExecuteRequest,
	workingDirectory string,
) bool {
	if strings.TrimSpace(request.Target.Workspace.Worktree) == "" {
		return false
	}
	return !(workingDirectory != "" &&
		workers.NormalizeRunnerID(request.Target.RunnerID) == workers.RunnerIDCodex)
}

func requestHasImageInput(inputs []workers.WorkInput) bool {
	for _, input := range inputs {
		for _, part := range input.Content {
			if part.Type.Normalized() == work.WorkContentPartTypeImage {
				return true
			}
		}
	}
	return false
}

func appendRunnerCapabilityIfMissing(
	capabilities []workers.RunnerOptionalCapability,
	capability workers.RunnerOptionalCapability,
) []workers.RunnerOptionalCapability {
	for _, existing := range capabilities {
		if existing == capability {
			return capabilities
		}
	}
	return append(capabilities, capability)
}

type adaptedWorkflowContext struct {
	workflow         *workers.Context
	workingDirectory string
	factoryDirectory string
	projectID        string
	envVars          map[string]string
}

func runnerIDForRequest(request workers.ExecuteRequest, identity string) string {
	runnerID := workers.NormalizeRunnerID(request.Target.RunnerID)
	switch identity {
	case runners.ScriptIdentity:
		return runners.ScriptIdentity
	case runners.InferenceIdentity:
		if runnerID == "" {
			return runners.InferenceIdentity
		}
	default:
		if provider := providerIdentity(request.Target.Provider); provider != "" {
			return provider
		}
		if runnerID == "" || runnerID == runners.AgentIdentity {
			return providerRunnerID(request.Target.Provider)
		}
	}
	return runnerID
}

func adaptWorkflowContext(request workers.ExecuteRequest) adaptedWorkflowContext {
	workflow := request.Input.WorkflowContext.Clone()
	if workflow == nil {
		workflow = &workers.Context{}
	}
	workingDirectory := firstNonEmpty(
		request.Target.Environment.WorkingDirectory,
		request.Target.Workspace.WorkingDirectory,
		workflow.WorkDirectory,
	)
	factoryDirectory := firstNonEmpty(
		request.Target.FactoryDirectory,
		request.Target.Workspace.FactoryDirectory,
		workflow.FactoryDirectory,
	)
	projectID := firstNonEmpty(request.Input.Dispatch.ProjectID, workflow.ProjectID)
	envVars := mergeStringMaps(workflow.EnvVars, request.Target.Environment.Vars)
	workflow.FactoryDirectory = factoryDirectory
	workflow.WorkDirectory = workingDirectory
	workflow.ProjectID = projectID
	workflow.EnvVars = cloneStringMap(envVars)
	workflow.SessionID = firstNonEmpty(workflow.SessionID, request.Correlation.FactorySessionID)
	return adaptedWorkflowContext{
		workflow:         workflow,
		workingDirectory: workingDirectory,
		factoryDirectory: factoryDirectory,
		projectID:        projectID,
		envVars:          envVars,
	}
}

func adaptDispatch(request workers.ExecuteRequest, inputTokens []workers.Token) work.WorkDispatch {
	dispatch := work.CloneWorkDispatch(request.Input.Dispatch)
	if dispatch.DispatchID == "" {
		dispatch = work.WorkDispatch{
			DispatchID:      request.Correlation.DispatchID,
			WorkstationName: request.Target.WorkstationName,
			WorkerType:      firstNonEmpty(request.Target.WorkerType, request.Target.WorkerName),
			Execution: work.ExecutionMetadata{
				RequestID: request.Correlation.RequestID,
				TraceID:   request.Correlation.TraceID,
				WorkIDs:   workIDs(request.Input.Work),
			},
		}
	}
	dispatch.DispatchID = request.Correlation.DispatchID
	dispatch.WorkstationName = request.Target.WorkstationName
	dispatch.WorkerType = firstNonEmpty(request.Target.WorkerType, request.Target.WorkerName)
	dispatch.InputTokens = workers.InputTokens(inputTokens...)
	dispatch.Execution.RequestID = request.Correlation.RequestID
	dispatch.Execution.TraceID = request.Correlation.TraceID
	dispatch.Execution.WorkIDs = workIDs(request.Input.Work)
	return dispatch
}

func providerSessionID(request workers.ExecuteRequest) string {
	if request.Input.Resume == nil {
		return ""
	}
	return firstNonEmpty(
		request.Input.Resume.ProviderSessionID,
		request.Input.Resume.ExternalRef,
	)
}

func inputTokensFromWorkInputs(inputs []workers.WorkInput) []workers.Token {
	if len(inputs) == 0 {
		return nil
	}
	tokens := make([]workers.Token, 0, len(inputs))
	for _, input := range inputs {
		color := workers.Color{
			Name:       input.WorkID,
			RequestID:  input.RequestID,
			WorkID:     input.WorkID,
			WorkTypeID: input.WorkTypeID,
			DataType:   workers.DataTypeWork,
			TraceID:    input.Lineage.TraceID,
			ParentID:   input.Lineage.ParentWorkID,
			Tags:       cloneStringMap(input.Tags),
			Relations:  append([]work.Relation(nil), input.Relations...),
			Content:    work.CloneWorkContentParts(input.Content),
			Payload:    payloadFromContent(input.Content),
		}
		tokens = append(tokens, workers.Token{Color: color})
	}
	return tokens
}

func payloadFromContent(content []work.WorkContentPart) []byte {
	if len(content) == 0 {
		return nil
	}
	var builder strings.Builder
	for _, part := range content {
		if part.Type.Normalized() != work.WorkContentPartTypeText {
			continue
		}
		builder.WriteString(part.Text)
	}
	if builder.Len() == 0 {
		return nil
	}
	return []byte(builder.String())
}

func providerRunnerID(provider workers.ProviderReference) string {
	if id := workers.NormalizeRunnerID(provider.ID); id != "" {
		return id
	}
	if alias := workers.NormalizeRunnerID(provider.Alias); alias != "" {
		return alias
	}
	return workers.RunnerIDCodex
}

func providerIdentity(provider workers.ProviderReference) string {
	if id := strings.TrimSpace(provider.ID); id != "" {
		return id
	}
	return strings.TrimSpace(provider.Alias)
}

func workIDs(inputs []workers.WorkInput) []string {
	if len(inputs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if id := strings.TrimSpace(input.WorkID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func mergeStringMaps(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := cloneStringMap(base)
	if merged == nil {
		merged = make(map[string]string, len(override))
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}
