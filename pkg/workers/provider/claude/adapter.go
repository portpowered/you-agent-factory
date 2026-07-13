// Package claude implements Claude's structured subprocess response adapter.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
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

func (*Adapter) Identity() adapter.Identity { return adapter.Identity(interfaces.ModelProviderClaude) }

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
	command.Command = string(interfaces.ModelProviderClaude)
	command.Args = args
	command.Env = workerprocess.MergeCommandEnv(os.Environ(), workerprocess.CommandEnvEntriesFromMap(req.EnvVars))
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
		return adapter.FinalParseResult{Response: interfaces.InferenceResponse{
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
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family:  interfaces.WorkFailureFamilyTerminal,
		Type:    interfaces.WorkFailureTypeUnknown,
		Message: "Claude invocation failed.",
	}}
}

func providerSession(sessionID string) *interfaces.ProviderSessionMetadata {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	return &interfaces.ProviderSessionMetadata{
		Provider: string(interfaces.ModelProviderClaude), Kind: providerSessionKind, ID: sessionID,
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
