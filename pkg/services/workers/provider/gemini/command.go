package gemini

import (
	"context"
	"errors"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/commandenv"
)

// Adapter owns Gemini command construction, environment handling, and native
// failure/timeout classification for the registry-backed conductor path.
type Adapter struct{}

// NewAdapter constructs the stateless Gemini adapter.
func NewAdapter() *Adapter { return &Adapter{} }

// Identity returns Gemini's stable registry key.
func (*Adapter) Identity() adapter.Identity {
	return adapter.Identity(modelprovider.ProviderGemini)
}

// BuildCommand assembles Gemini argv and subprocess environment.
func (*Adapter) BuildCommand(_ context.Context, input adapter.CommandContext) (adapter.CommandBuildResult, error) {
	args, err := BuildArgs(input.Request, input.SkipPermissions)
	if err != nil {
		return adapter.CommandBuildResult{}, err
	}
	return adapter.CommandBuildResult{Request: BuildCommandRequest(input.Request, args)}, nil
}

// BuildArgs constructs Gemini CLI arguments and rejects unsupported optional
// capabilities with the customer-visible failure posture accepted for Gemini.
func BuildArgs(req workerexecution.ProviderInferenceRequest, skipPermissions bool) ([]string, error) {
	if err := validateOptionalCapabilities(req); err != nil {
		return nil, err
	}
	args := []string{"--prompt", req.UserMessage}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
	}
	if skipPermissions {
		args = append(args, "--approval-mode", "yolo", "--sandbox", "false")
	}
	return args, nil
}

// BuildCommandRequest owns Gemini subprocess request assembly, including
// environment merging through the shared commandenv policy.
func BuildCommandRequest(req workerexecution.ProviderInferenceRequest, args []string) workerprocess.CommandRequest {
	command := workerprocess.SubprocessRequestBase(req.Dispatch)
	command.Command = string(modelprovider.ProviderGemini)
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
		workerexecution.RunnerOptionalCapabilityImageInput:       "image input is not supported by the gemini runner in v1",
		workerexecution.RunnerOptionalCapabilityStructuredOutput: "structured output is not supported by the gemini runner in v1",
		workerexecution.RunnerOptionalCapabilitySessionResume:    "session resume is not supported by the gemini runner in v1",
		workerexecution.RunnerOptionalCapabilityWorktree:         "worktree selection is not supported by the gemini runner in v1",
	}
	for _, capability := range req.RequiredOptionalCapabilities {
		if message, blocked := unsupported[capability]; blocked {
			return errors.New(message)
		}
	}
	if req.SessionID != "" {
		return errors.New("session resume is not supported by the gemini runner in v1")
	}
	return nil
}
