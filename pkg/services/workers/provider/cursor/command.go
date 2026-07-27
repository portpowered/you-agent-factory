package cursor

import (
	"context"
	"errors"
	"strings"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/commandenv"
)

const cursorAgentCommand = "agent"

// AdapterDependencies are the platform facts and effects used by Cursor
// command construction.
type AdapterDependencies struct {
	OperatingSystem string
	TemporaryDir    string
	TemporaryFiles  platformfilesystem.TemporaryFileSystem
}

// Adapter owns Cursor command construction for the registry-backed conductor
// path, including Windows long-prompt materialization and cleanup.
type Adapter struct {
	operatingSystem  string
	temporaryDir     string
	temporaryFiles   platformfilesystem.TemporaryFileSystem
	requestedSession *workerexecution.ProviderSessionMetadata
}

// NewAdapter constructs a Cursor adapter. Omitted dependencies are valid for
// direct prompts; oversized Windows prompts fail closed without their required
// temporary-file effect.
func NewAdapter(dependencies ...AdapterDependencies) *Adapter {
	result := &Adapter{}
	if len(dependencies) == 0 {
		return result
	}
	result.operatingSystem = strings.TrimSpace(dependencies[0].OperatingSystem)
	result.temporaryDir = dependencies[0].TemporaryDir
	result.temporaryFiles = dependencies[0].TemporaryFiles
	return result
}

// Identity returns Cursor's stable registry key.
func (*Adapter) Identity() adapter.Identity {
	return adapter.Identity("cursor")
}

// BuildCommand assembles Cursor argv and subprocess execution context.
func (a *Adapter) BuildCommand(ctx context.Context, input adapter.CommandContext) (adapter.CommandBuildResult, error) {
	args, err := BuildArgs(input.Request, input.SkipPermissions)
	if err != nil {
		return adapter.CommandBuildResult{}, err
	}
	args, cleanup, err := a.materializePrompt(ctx, input.Request, args)
	if err != nil {
		return adapter.CommandBuildResult{}, err
	}
	return adapter.CommandBuildResult{
		Request: BuildCommandRequest(input.Request, args),
		Cleanup: cleanup,
	}, nil
}

// BuildArgs constructs Cursor Agent CLI arguments while preserving the
// established prompt, resume, permission, workspace, and streaming behavior.
func BuildArgs(req workerexecution.ProviderInferenceRequest, skipPermissions bool) ([]string, error) {
	if err := validateOptionalCapabilities(req); err != nil {
		return nil, err
	}
	args := make([]string, 0, 14)
	if skipPermissions {
		args = append(args, "-f")
	}
	args = append(args, "-p")
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
	}
	if req.WorkingDirectory != "" {
		args = append(args, "--workspace", req.WorkingDirectory)
	}
	if req.Worktree != "" {
		args = append(args, "--worktree", req.Worktree)
	}
	args = append(args,
		"--output-format", CursorOutputFormatStreamJSON,
		"--stream-partial-output",
		BuildPrompt(req),
	)
	return args, nil
}

// BuildPrompt combines Cursor system instructions and the user request into
// the single positional prompt accepted by the Agent CLI.
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

// BuildCommandRequest owns Cursor subprocess request assembly, including the
// shared deterministic environment policy and dispatch metadata.
func BuildCommandRequest(req workerexecution.ProviderInferenceRequest, args []string) workerprocess.CommandRequest {
	command := workerprocess.SubprocessRequestBase(req.Dispatch)
	command.Command = cursorAgentCommand
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
		workerexecution.RunnerOptionalCapabilityImageInput:       "image input is not supported by the cursor-cli runner in v1",
		workerexecution.RunnerOptionalCapabilityStructuredOutput: "structured output is not supported by the cursor-cli runner in v1",
		workerexecution.RunnerOptionalCapabilityWorktree:         "worktree selection is not supported by the cursor-cli runner in v1",
	}
	for _, capability := range req.RequiredOptionalCapabilities {
		if message, blocked := unsupported[capability]; blocked {
			return errors.New(message)
		}
	}
	return nil
}
