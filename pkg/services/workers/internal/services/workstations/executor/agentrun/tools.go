package agentrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"
)

const (
	ToolNameReadFile      = "read_file"
	ToolNameListDirectory = "list_directory"
	ToolNameWriteFile     = "write_file"

	DiagnosticToolPolicy      = "tool_policy"
	DiagnosticToolCallCount   = "tool_call_count"
	DiagnosticToolDiagnostics = "tool_diagnostics"

	toolDiagnosticMaxLen = 240
)

var (
	ErrToolPolicyDenied         = errors.New("agent tool execution denied by policy")
	ErrToolNotSupported         = errors.New("agent tool is not supported")
	ErrToolFileSystemRequired   = errors.New("agent tool filesystem is required")
	ErrAgentRunToolsUnsupported = errors.New("agent-run tools are unsupported by the text-only Workers Runner")
)

const agentRunToolsUnsupportedRecoveryAction = "set agentTools.policy to DISABLED or use a Runner/provider contract with structured tool calls"

func agentRunToolsUnsupportedError(policy string) error {
	return fmt.Errorf(
		"%w: the model requested a structured tool call under policy %q, but the text-only Workers Runner cannot represent it; %s",
		ErrAgentRunToolsUnsupported,
		workerconfig.NormalizeAgentToolPolicy(policy),
		agentRunToolsUnsupportedRecoveryAction,
	)
}

// unsupportedToolExecutor keeps permissive tool policies compatible with
// text-only runners while rejecting the first structured call that reaches the
// harness. The Runner contract does not carry ToolCalls, so a normal AGENT_RUN
// completion never invokes this boundary; a future structured Runner can use
// the same diagnostic until the contract is explicitly upgraded.
type unsupportedToolExecutor struct {
	policy   string
	recorder *ToolDiagnosticRecorder
}

func newUnsupportedToolExecutor(policy string, recorder *ToolDiagnosticRecorder) *unsupportedToolExecutor {
	return &unsupportedToolExecutor{
		policy:   workerconfig.NormalizeAgentToolPolicy(policy),
		recorder: recorder,
	}
}

func (executor *unsupportedToolExecutor) Execute(_ context.Context, call messages.ToolCall) (messages.ToolCallResponse, error) {
	if executor != nil && executor.recorder != nil {
		toolName := strings.TrimSpace(call.Name)
		executor.recorder.Record(toolName, "start", "")
		executor.recorder.Record(toolName, "unsupported", "structured_tool_calls_unavailable")
	}
	policy := ""
	if executor != nil {
		policy = executor.policy
	}
	return messages.ToolCallResponse{}, agentRunToolsUnsupportedError(policy)
}

// ToolDiagnostic records a safe summary for one tool lifecycle event.
type ToolDiagnostic struct {
	ToolName string
	Phase    string
	Detail   string
}

// ToolDiagnosticRecorder collects bounded tool diagnostics for work results.
type ToolDiagnosticRecorder struct {
	events []ToolDiagnostic
}

func NewToolDiagnosticRecorder() *ToolDiagnosticRecorder {
	return &ToolDiagnosticRecorder{}
}

func (recorder *ToolDiagnosticRecorder) Record(toolName, phase, detail string) {
	if recorder == nil {
		return
	}
	recorder.events = append(recorder.events, ToolDiagnostic{
		ToolName: strings.TrimSpace(toolName),
		Phase:    strings.TrimSpace(phase),
		Detail:   sanitizeToolDiagnosticDetail(detail),
	})
}

func (recorder *ToolDiagnosticRecorder) Events() []ToolDiagnostic {
	if recorder == nil {
		return nil
	}
	out := make([]ToolDiagnostic, len(recorder.events))
	copy(out, recorder.events)
	return out
}

func (recorder *ToolDiagnosticRecorder) hasPhaseSince(phase string, start int) bool {
	if recorder == nil {
		return false
	}
	if start < 0 {
		start = 0
	}
	if start >= len(recorder.events) {
		return false
	}
	for _, event := range recorder.events[start:] {
		if event.Phase == phase {
			return true
		}
	}
	return false
}

func sanitizeToolDiagnosticDetail(detail string) string {
	trimmed := strings.TrimSpace(detail)
	if len(trimmed) <= toolDiagnosticMaxLen {
		return trimmed
	}
	return trimmed[:toolDiagnosticMaxLen] + "..."
}

func toolFailureDetailFromReason(toolName, arguments, reason string) string {
	relativePath := toolRelativePathFromArguments(arguments)
	if relativePath != "" {
		return fmt.Sprintf("path=%s reason=%s", relativePath, reason)
	}
	return fmt.Sprintf("reason=%s", reason)
}

// toolRuntimeError is the bounded error returned from filesystem tool execution.
// Its message is safe for dispatch failureMessage and dashboard inspection surfaces.
type toolRuntimeError struct {
	toolName  string
	arguments string
	reason    string
	cause     error
}

func newToolRuntimeError(toolName, arguments string, err error) *toolRuntimeError {
	return &toolRuntimeError{
		toolName:  strings.TrimSpace(toolName),
		arguments: arguments,
		reason:    toolFailureReason(err),
		cause:     err,
	}
}

func (err *toolRuntimeError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *toolRuntimeError) Error() string {
	if err == nil {
		return ""
	}
	return err.toolName + ": " + toolFailureDetailFromReason(err.toolName, err.arguments, err.reason)
}

func toolRelativePathFromArguments(arguments string) string {
	rawPath := extractPathFromArguments(arguments)
	if !isSafeRelativePathForDiagnostics(rawPath) {
		return ""
	}
	return rawPath
}

func extractPathFromArguments(arguments string) string {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return ""
	}
	var pathArgs pathArgument
	if err := json.Unmarshal([]byte(trimmed), &pathArgs); err == nil {
		return strings.TrimSpace(pathArgs.Path)
	}
	var writeArgs writeFileArgument
	if err := json.Unmarshal([]byte(trimmed), &writeArgs); err == nil {
		return strings.TrimSpace(writeArgs.Path)
	}
	return ""
}

func isSafeRelativePathForDiagnostics(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	if isAnyPlatformAbsolutePath(trimmed) {
		return false
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func isAnyPlatformAbsolutePath(path string) bool {
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\\`) {
		return true
	}
	return len(path) >= 3 && path[1] == ':' && (path[2] == '/' || path[2] == '\\') &&
		((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z'))
}

func toolFailureReason(err error) string {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "write_file failed creating parent directories") {
		return "operation_failed"
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "not_found"
	case errors.Is(err, fs.ErrPermission):
		return "permission_denied"
	}
	switch {
	case strings.Contains(message, "tool path is required"):
		return "path_required"
	case strings.Contains(message, "tool path must be relative"):
		return "path_must_be_relative"
	case strings.Contains(message, "tool path cannot escape"):
		return "path_escape_denied"
	case strings.Contains(message, "arguments must be json"):
		return "invalid_arguments"
	case strings.Contains(message, "agent working directory"):
		return "working_directory_unavailable"
	case strings.Contains(message, "not supported"):
		return "tool_not_supported"
	case strings.Contains(message, "read_file failed"),
		strings.Contains(message, "list_directory failed"),
		strings.Contains(message, "write_file failed"):
		return "operation_failed"
	default:
		return "operation_failed"
	}
}

func toolDiagnosticsMetadata(policy string, recorder *ToolDiagnosticRecorder) map[string]string {
	metadata := map[string]string{
		DiagnosticToolPolicy: workerconfig.NormalizeAgentToolPolicy(policy),
	}
	if recorder == nil {
		return metadata
	}
	events := recorder.Events()
	if len(events) == 0 {
		return metadata
	}
	metadata[DiagnosticToolCallCount] = fmt.Sprintf("%d", len(events))
	summaries := make([]string, 0, len(events))
	for _, event := range events {
		summary := event.ToolName + ":" + event.Phase
		if event.Detail != "" {
			summary += ":" + event.Detail
		}
		summaries = append(summaries, summary)
	}
	metadata[DiagnosticToolDiagnostics] = strings.Join(summaries, ",")
	return metadata
}

func toolDefinitionsForPolicy(policy string) []messages.ToolDefinition {
	switch workerconfig.NormalizeAgentToolPolicy(policy) {
	case workerconfig.AgentToolPolicyReadOnly:
		return []messages.ToolDefinition{
			{Name: ToolNameReadFile, Description: "Read a UTF-8 text file relative to the agent working directory."},
			{Name: ToolNameListDirectory, Description: "List entries in a directory relative to the agent working directory."},
		}
	case workerconfig.AgentToolPolicyEnabled:
		return []messages.ToolDefinition{
			{Name: ToolNameReadFile, Description: "Read a UTF-8 text file relative to the agent working directory."},
			{Name: ToolNameListDirectory, Description: "List entries in a directory relative to the agent working directory."},
			{Name: ToolNameWriteFile, Description: "Write UTF-8 text to a file relative to the agent working directory."},
		}
	default:
		return nil
	}
}

type pathArgument struct {
	Path string `json:"path"`
}

type writeFileArgument struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func isToolPolicyError(err error) bool {
	return errors.Is(err, ErrToolPolicyDenied) || errors.Is(err, ErrToolNotSupported)
}

func isToolRuntimeError(err error) bool {
	if err == nil || isToolPolicyError(err) {
		return false
	}
	var toolErr *toolRuntimeError
	if errors.As(err, &toolErr) {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "read_file failed") ||
		strings.Contains(message, "write_file failed") ||
		strings.Contains(message, "list_directory failed")
}
