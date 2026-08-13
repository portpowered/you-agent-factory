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
	runnerID := workers.NormalizeRunnerID(request.Target.RunnerID)
	switch identity {
	case runners.ScriptIdentity:
		runnerID = runners.ScriptIdentity
	case runners.InferenceIdentity:
		if runnerID == "" {
			runnerID = runners.InferenceIdentity
		}
	default:
		if runnerID == "" || runnerID == runners.AgentIdentity {
			runnerID = providerRunnerID(request.Target.Provider)
		}
	}

	workingDirectory := strings.TrimSpace(request.Target.Environment.WorkingDirectory)
	if workingDirectory == "" {
		workingDirectory = strings.TrimSpace(request.Target.Workspace.WorkingDirectory)
	}
	workflowContext := request.Input.WorkflowContext.Clone()
	if workflowContext == nil {
		workflowContext = &workers.Context{}
	}
	workingDirectory = firstNonEmpty(workingDirectory, workflowContext.WorkDirectory)
	factoryDirectory := firstNonEmpty(
		request.Target.FactoryDirectory,
		request.Target.Workspace.FactoryDirectory,
		workflowContext.FactoryDirectory,
	)
	projectID := firstNonEmpty(request.Input.Dispatch.ProjectID, workflowContext.ProjectID)
	envVars := mergeStringMaps(workflowContext.EnvVars, request.Target.Environment.Vars)
	workflowContext.FactoryDirectory = factoryDirectory
	workflowContext.WorkDirectory = workingDirectory
	workflowContext.ProjectID = projectID
	workflowContext.EnvVars = cloneStringMap(envVars)
	workflowContext.SessionID = firstNonEmpty(
		workflowContext.SessionID,
		request.Correlation.FactorySessionID,
	)
	worktree := strings.TrimSpace(request.Target.Workspace.Worktree)
	inputTokens := inputTokensFromWorkInputs(request.Input.Work)
	sessionID := ""
	if request.Input.Resume != nil {
		sessionID = strings.TrimSpace(request.Input.Resume.ProviderSessionID)
		if sessionID == "" {
			sessionID = strings.TrimSpace(request.Input.Resume.ExternalRef)
		}
	}

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

	return workers.RunnerExecutionRequest{
		Dispatch:                     dispatch,
		Correlation:                  request.Correlation,
		WorkerName:                   request.Target.WorkerName,
		WorkerType:                   firstNonEmpty(request.Target.WorkerType, request.Target.WorkerName),
		WorkstationType:              request.Target.WorkstationName,
		RunnerID:                     runnerID,
		ExecutorProvider:             providerIdentity(request.Target.Provider),
		ModelOperation:               request.Input.ModelOperation,
		ModelBindings:                workers.CloneResolvedModelOperationBindings(request.Input.ModelBindings),
		InputTokens:                  workers.InputTokens(inputTokens...),
		SystemPrompt:                 request.Target.Prompt.SystemPrompt,
		UserMessage:                  request.Target.Prompt.UserMessage,
		OutputSchema:                 request.Target.Prompt.OutputSchema,
		ToolExecutionMode:            request.Target.Tools.ExecutionMode,
		RequiredOptionalCapabilities: append([]workers.RunnerOptionalCapability(nil), request.Target.Tools.RequiredOptionalCapabilities...),
		EnvVars:                      envVars,
		ProcessEnvironment:           append([]string(nil), request.Target.Environment.ProcessEnvironment...),
		Worktree:                     worktree,
		WorkingDirectory:             workingDirectory,
		Model:                        request.Target.Model.Name,
		ModelProvider:                firstNonEmpty(request.Target.Model.Provider, providerIdentity(request.Target.Provider)),
		ReasoningEffort:              request.Target.Model.ReasoningEffort,
		ModelLocality:                request.Target.Model.Locality,
		Command:                      request.Target.Command,
		Args:                         append([]string(nil), request.Target.Args...),
		FactoryDirectory:             factoryDirectory,
		OutputContract:               request.Target.Output.Contract,
		OutputFormat:                 request.Target.Output.Format,
		StopToken:                    request.Target.Output.StopToken,
		DecisionEnvelope:             request.Target.Output.DecisionEnvelope,
		GoalRoutingDecisionEnvelope:  request.Target.Output.GoalRoutingDecisionEnvelope,
		SessionID:                    sessionID,
		ProjectID:                    projectID,
		WorkflowContext:              workflowContext,
		SkipPermissions:              request.Target.Permissions.SkipPermissions,
		TemporaryFiles:               temporaryFiles,
	}
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
		}
		tokens = append(tokens, workers.Token{Color: color})
	}
	return tokens
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
