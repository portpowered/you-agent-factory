// Package claude implements Claude's structured subprocess response adapter.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	provider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	claudeexitfailure "github.com/portpowered/infinite-you/pkg/services/workers/provider/claude/exitfailure"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/commandenv"
)

const (
	outputFormatStreamJSON = "stream-json"
	providerSessionKind    = "session_id"
)

// Adapter translates Claude CLI structured output into canonical drafts.
type Adapter struct{}

// NewAdapter constructs the stateless Claude adapter. Decoder state is
// allocated separately for every invocation.
func NewAdapter() *Adapter { return &Adapter{} }

func (*Adapter) Identity() adapter.Identity { return adapter.Identity(modelprovider.ProviderClaude) }

func (*Adapter) BuildCommand(_ context.Context, input adapter.CommandContext) (adapter.CommandBuildResult, error) {
	req := input.Request
	args := []string{"-p"}
	if input.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	if req.Worktree != "" {
		args = append(args, "--worktree", req.Worktree)
	}
	if req.SystemPrompt != "" {
		args = append(args, "--system-prompt", req.SystemPrompt)
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
	}
	args = append(args, "--output-format", outputFormatStreamJSON, "--include-partial-messages", req.UserMessage)

	command := workerprocess.SubprocessRequestBase(req.Dispatch)
	command.Command = string(modelprovider.ProviderClaude)
	command.Args = args
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
	return adapter.CommandBuildResult{Request: command}, nil
}

func (*Adapter) NewDecoder(_ context.Context, input adapter.DecoderContext) (adapter.Decoder, error) {
	return newDecoder(input), nil
}

func (*Adapter) ParseFinal(_ context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	if input.CommandError != nil || input.CommandResult.ExitCode != 0 {
		if failure := claudeRetryFailure(input.CommandResult.Stdout); failure != nil {
			return adapter.FinalParseResult{}, &claudeRetryError{failure: *failure}
		}
		return adapter.FinalParseResult{}, errors.New("Claude command did not complete successfully")
	}
	lines := bytes.Split(bytes.ReplaceAll(input.CommandResult.Stdout, []byte("\r\n"), []byte("\n")), []byte("\n"))
	for index := len(lines) - 1; index >= 0; index-- {
		var record struct {
			Type      string `json:"type"`
			Subtype   string `json:"subtype"`
			IsError   bool   `json:"is_error"`
			Result    string `json:"result"`
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal(bytes.TrimSpace(lines[index]), &record) != nil || record.Type != "result" {
			continue
		}
		if record.Subtype != "success" || record.IsError {
			return adapter.FinalParseResult{}, errors.New("Claude returned a terminal failure")
		}
		return adapter.FinalParseResult{Response: workerexecution.InferenceResponse{
			Content:         record.Result,
			ProviderSession: providerSession(record.SessionID),
		}}, nil
	}
	return adapter.FinalParseResult{}, errors.New("Claude stream did not contain a terminal result")
}

func (*Adapter) Capabilities(context.Context, adapter.CapabilityContext) (adapter.CapabilityResult, error) {
	return adapter.CapabilityResult{Capabilities: adapter.Capabilities{
		NativeStreaming: true, MessageDeltas: true, MessageSnapshots: true,
		ToolLifecycle: true, ToolOutputDeltas: true, StableItemIDs: true,
	}}, nil
}

func (*Adapter) ClassifyFailure(_ context.Context, input adapter.FailureContext) adapter.FailureResult {
	if input.CommandError == nil && input.CommandResult.ExitCode == 0 && input.DecodeError == nil && input.FlushError == nil && input.ParseError == nil {
		return adapter.FailureResult{}
	}
	if failure := claudeRetryFailure(input.CommandResult.Stdout); failure != nil {
		return adapter.FailureResult{Failure: failure}
	}
	var retryErr *claudeRetryError
	if errors.As(input.ParseError, &retryErr) {
		failure := retryErr.failure
		return adapter.FailureResult{Failure: &failure}
	}
	result := claudeexitfailure.ParseProviderFailure(claudeexitfailure.FailureInput{
		ExitCode: input.CommandResult.ExitCode,
		Stdout:   input.CommandResult.Stdout,
		Stderr:   input.CommandResult.Stderr,
	})
	cause := errors.Join(input.CommandError, input.DecodeError, input.FlushError, input.ParseError)
	providerErr := provider.NewProviderErrorFromResult(provider.ProviderFailureResult{
		Reason: result.Reason, Message: result.Message,
	}, cause)
	decision := provider.WorkFailureDecisionFromProviderError(providerErr)
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family:  providerErr.Family,
		Type:    providerErr.Type,
		Message: providerErr.Message,
		Retry:   adapter.RetryGuidance{Retryable: decision.Retryable},
	}}
}

func providerSession(sessionID string) *workerexecution.ProviderSessionMetadata {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	return &workerexecution.ProviderSessionMetadata{
		Provider: string(modelprovider.ProviderClaude), Kind: providerSessionKind, ID: sessionID,
	}
}

func marshalPayload(value any) (json.RawMessage, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal Claude canonical payload: %w", err)
	}
	return payload, nil
}

var _ adapter.Adapter = (*Adapter)(nil)
