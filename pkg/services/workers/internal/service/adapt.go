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
	worktree := strings.TrimSpace(request.Target.Workspace.Worktree)
	sessionID := ""
	if request.Input.Resume != nil {
		sessionID = strings.TrimSpace(request.Input.Resume.ProviderSessionID)
		if sessionID == "" {
			sessionID = strings.TrimSpace(request.Input.Resume.ExternalRef)
		}
	}
	// Resume carries only an opaque continuation identity. Providers/Provider
	// Sessions validate and resume the underlying conversation; Workers must
	// not inspect or retain mutable session objects.

	adapted := workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      request.Correlation.DispatchID,
			WorkstationName: request.Target.WorkstationName,
			WorkerType:      request.Target.WorkerName,
			Execution: work.ExecutionMetadata{
				RequestID: request.Correlation.RequestID,
				TraceID:   request.Correlation.TraceID,
				WorkIDs:   workIDs(request.Input.Work),
			},
		},
		WorkerType:                   request.Target.WorkerName,
		WorkstationType:              request.Target.WorkstationName,
		RunnerID:                     runnerID,
		ExecutorProvider:             providerIdentity(request.Target.Provider),
		ModelOperation:               request.Input.ModelOperation,
		ModelBindings:                workers.CloneResolvedModelOperationBindings(request.Input.ModelBindings),
		SystemPrompt:                 request.Target.Prompt.SystemPrompt,
		UserMessage:                  request.Target.Prompt.UserMessage,
		OutputSchema:                 request.Target.Prompt.OutputSchema,
		ToolExecutionMode:            request.Target.Tools.ExecutionMode,
		RequiredOptionalCapabilities: append([]workers.RunnerOptionalCapability(nil), request.Target.Tools.RequiredOptionalCapabilities...),
		EnvVars:                      cloneStringMap(request.Target.Environment.Vars),
		ProcessEnvironment:           append([]string(nil), request.Target.Environment.ProcessEnvironment...),
		Worktree:                     worktree,
		WorkingDirectory:             workingDirectory,
		Model:                        request.Target.Model.Name,
		ModelProvider:                firstNonEmpty(request.Target.Model.Provider, providerIdentity(request.Target.Provider)),
		ReasoningEffort:              request.Target.Model.ReasoningEffort,
		ModelLocality:                request.Target.Model.Locality,
		SessionID:                    sessionID,
		SkipPermissions:              request.Target.Permissions.SkipPermissions,
	}
	if request.Attempt.Number > 0 {
		// Prior attempts remain Runtime-owned; runners currently receive only
		// the active request. Correlation.AttemptID is preserved on the root
		// result and observations.
		_ = request.Attempt.Number
	}
	return adapted
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
