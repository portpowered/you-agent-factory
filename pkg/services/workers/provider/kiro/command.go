package kiro

import (
	"context"
	"errors"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/commandenv"
)

// Adapter owns Kiro command construction for the registry-backed conductor
// path. Response decoding and failure normalization are added by the remaining
// Kiro migration slices.
type Adapter struct{}

// NewAdapter constructs the stateless Kiro adapter.
func NewAdapter() *Adapter { return &Adapter{} }

// Identity returns Kiro's stable registry key.
func (*Adapter) Identity() adapter.Identity {
	return adapter.Identity(modelprovider.ProviderKiro)
}

// BuildCommand assembles Kiro argv and subprocess execution context.
func (*Adapter) BuildCommand(_ context.Context, input adapter.CommandContext) (adapter.CommandBuildResult, error) {
	args, err := BuildArgs(input.Request, input.SkipPermissions)
	if err != nil {
		return adapter.CommandBuildResult{}, err
	}
	return adapter.CommandBuildResult{Request: BuildCommandRequest(input.Request, args)}, nil
}

// BuildArgs constructs Kiro CLI arguments while preserving the established
// prompt, resume, and permission behavior.
func BuildArgs(req workerexecution.ProviderInferenceRequest, skipPermissions bool) ([]string, error) {
	if err := validateOptionalCapabilities(req); err != nil {
		return nil, err
	}
	args := []string{"chat", "--no-interactive"}
	if req.SessionID != "" {
		args = append(args, "--resume-id", req.SessionID)
	}
	if skipPermissions {
		args = append(args, "--trust-all-tools")
	}
	if prompt := BuildPrompt(req); prompt != "" {
		args = append(args, prompt)
	}
	return args, nil
}

// BuildPrompt combines Kiro system instructions and the user request into the
// single positional prompt accepted by kiro-cli.
func BuildPrompt(req workerexecution.ProviderInferenceRequest) string {
	systemPrompt := strings.TrimSpace(req.SystemPrompt)
	userMessage := strings.TrimSpace(req.UserMessage)
	switch {
	case systemPrompt == "":
		return userMessage
	case userMessage == "":
		return systemPrompt
	default:
		return "System instructions:\n" + systemPrompt + "\n\nUser request:\n" + userMessage
	}
}

// BuildCommandRequest owns Kiro subprocess request assembly, including the
// shared deterministic environment policy and dispatch metadata.
func BuildCommandRequest(req workerexecution.ProviderInferenceRequest, args []string) workerprocess.CommandRequest {
	command := workerprocess.SubprocessRequestBase(req.Dispatch)
	command.Command = string(modelprovider.ProviderKiro)
	command.Args = append([]string(nil), args...)
	command.Env = commandenv.Build(req.ProcessEnvironment, req.EnvVars)
	command.WorkDir = req.WorkingDirectory
	command.InputTokens = append([]any(nil), req.InputTokens...)
	if req.WorkerType != "" {
		command.WorkerType = req.WorkerType
	}
	if req.WorkstationType != "" {
		command.WorkstationName = req.WorkstationType
	}
	if req.ProjectID != "" {
		command.ProjectID = req.ProjectID
	}
	return command
}

func validateOptionalCapabilities(req workerexecution.ProviderInferenceRequest) error {
	unsupported := map[workerexecution.RunnerOptionalCapability]string{
		workerexecution.RunnerOptionalCapabilityImageInput:       "image input is not supported by the kiro runner in v1",
		workerexecution.RunnerOptionalCapabilityStructuredOutput: "structured output is not supported by the kiro runner in v1",
		workerexecution.RunnerOptionalCapabilityWorkingDirectory: "working directory is not supported by the kiro runner in v1",
		workerexecution.RunnerOptionalCapabilityWorktree:         "worktree selection is not supported by the kiro runner in v1",
	}
	for _, capability := range req.RequiredOptionalCapabilities {
		if message, blocked := unsupported[capability]; blocked {
			return errors.New(message)
		}
	}
	return nil
}
