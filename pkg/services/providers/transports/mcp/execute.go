package providersmcp

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// ExecuteInput is the MCP request shape for you.provider.execute.
type ExecuteInput struct {
	Provider           string            `json:"provider"`
	AttemptID          string            `json:"attemptId"`
	WorkerType         string            `json:"workerType,omitempty"`
	WorkstationName    string            `json:"workstationName,omitempty"`
	Model              string            `json:"model,omitempty"`
	ReasoningEffort    string            `json:"reasoningEffort,omitempty"`
	SkipPermissions    bool              `json:"skipPermissions,omitempty"`
	SystemPrompt       string            `json:"systemPrompt,omitempty"`
	UserMessage        string            `json:"userMessage,omitempty"`
	InputTokens        []any             `json:"inputTokens,omitempty"`
	OutputSchema       string            `json:"outputSchema,omitempty"`
	WorkingDirectory   string            `json:"workingDirectory,omitempty"`
	Worktree           string            `json:"worktree,omitempty"`
	EnvVars            map[string]string `json:"envVars,omitempty"`
	ProcessEnvironment []string          `json:"processEnvironment,omitempty"`
}

func (input ExecuteInput) executeRequest() providers.ExecuteRequest {
	request := providers.ExecuteRequest{
		Provider:           providers.ID(input.Provider),
		AttemptID:          input.AttemptID,
		WorkerType:         input.WorkerType,
		WorkstationName:    input.WorkstationName,
		Model:              input.Model,
		ReasoningEffort:    input.ReasoningEffort,
		SkipPermissions:    input.SkipPermissions,
		SystemPrompt:       input.SystemPrompt,
		UserMessage:        input.UserMessage,
		InputTokens:        input.InputTokens,
		OutputSchema:       input.OutputSchema,
		WorkingDirectory:   input.WorkingDirectory,
		Worktree:           input.Worktree,
		EnvVars:            input.EnvVars,
		ProcessEnvironment: input.ProcessEnvironment,
	}
	return request
}

// Execute performs one normalized provider attempt through the you.provider.execute
// MCP tool.
func Execute(
	ctx context.Context,
	service providers.Service,
	input ExecuteInput,
) ToolResponse[providers.ExecuteResult] {
	if response, done := requestContextErrorResponse[providers.ExecuteResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[providers.ExecuteResult]{Error: &envelope}
	}
	result, err := service.Execute(ctx, input.executeRequest())
	if err != nil {
		envelope := executeErrorEnvelope(err)
		return ToolResponse[providers.ExecuteResult]{Error: &envelope}
	}
	return ToolResponse[providers.ExecuteResult]{Result: &result}
}
