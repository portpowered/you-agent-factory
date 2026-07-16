// Package pi implements Pi's structured subprocess response adapter.
package pi

import (
	"context"
	"errors"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/commandenv"
)

const providerSessionKind = "session_id"

// Adapter translates Pi CLI JSON-mode output into canonical drafts.
type Adapter struct{}

// NewAdapter constructs the stateless Pi adapter.
func NewAdapter() *Adapter { return &Adapter{} }

func (*Adapter) Identity() adapter.Identity { return adapter.Identity(modelprovider.Pi) }

func (*Adapter) BuildCommand(_ context.Context, input adapter.CommandContext) (adapter.CommandBuildResult, error) {
	req := input.Request
	args := []string{"--print", "--mode", "json", "--approve"}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.SessionID != "" {
		args = append(args, "--session", req.SessionID)
	}
	if req.SystemPrompt != "" {
		args = append(args, "--system-prompt", req.SystemPrompt)
	}
	args = append(args, req.UserMessage)

	command := workerprocess.SubprocessRequestBase(req.Dispatch)
	command.Command = string(modelprovider.Pi)
	command.Args = args
	command.Env = commandenv.Build(req.EnvVars)
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
	return adapter.CommandBuildResult{Request: command}, nil
}

func (*Adapter) NewDecoder(_ context.Context, input adapter.DecoderContext) (adapter.Decoder, error) {
	return newDecoder(input), nil
}

func (*Adapter) ParseFinal(_ context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	if input.CommandError != nil || input.CommandResult.ExitCode != 0 {
		if failure := piRetryFailureFromStdout(input.CommandResult.Stdout); failure != nil {
			return adapter.FinalParseResult{}, &piRetryError{failure: *failure}
		}
		if failure := parseTerminalFailure(input.CommandResult.Stdout); failure != nil {
			return adapter.FinalParseResult{}, failure
		}
		return adapter.FinalParseResult{}, errors.New("Pi command did not complete successfully")
	}
	parsed, err := parseFinalOutput(input.CommandResult.Stdout)
	if err != nil {
		return adapter.FinalParseResult{}, err
	}
	return adapter.FinalParseResult{Response: workerexecution.InferenceResponse{
		Content:         parsed.Content,
		ProviderSession: parsed.ProviderSession,
	}}, nil
}

func (*Adapter) Capabilities(context.Context, adapter.CapabilityContext) (adapter.CapabilityResult, error) {
	return adapter.CapabilityResult{Capabilities: adapter.Capabilities{
		NativeStreaming: true, MessageDeltas: true, MessageSnapshots: true,
		ReasoningSummaries: true, ToolLifecycle: true, ToolOutputDeltas: true, StableItemIDs: true,
	}}, nil
}

func (*Adapter) ClassifyFailure(_ context.Context, input adapter.FailureContext) adapter.FailureResult {
	return classifyPiFailure(input)
}

func providerSession(sessionID string) *workerexecution.ProviderSessionMetadata {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	return &workerexecution.ProviderSessionMetadata{
		Provider: string(modelprovider.Pi), Kind: providerSessionKind, ID: sessionID,
	}
}

var _ adapter.Adapter = (*Adapter)(nil)
