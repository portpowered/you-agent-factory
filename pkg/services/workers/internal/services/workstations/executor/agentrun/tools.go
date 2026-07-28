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
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

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
	ErrToolPolicyDenied       = errors.New("agent tool execution denied by policy")
	ErrToolNotSupported       = errors.New("agent tool is not supported")
	ErrToolFileSystemRequired = errors.New("agent tool filesystem is required")
)

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

func sanitizeToolDiagnosticDetail(detail string) string {
	trimmed := strings.TrimSpace(detail)
	if len(trimmed) <= toolDiagnosticMaxLen {
		return trimmed
	}
	return trimmed[:toolDiagnosticMaxLen] + "..."
}

func toolFailureDetail(toolName, arguments string, err error) string {
	if err == nil {
		return ""
	}
	return toolFailureDetailFromReason(toolName, arguments, toolFailureReason(err))
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

func mergeToolDiagnostics(base map[string]string, policy string, recorder *ToolDiagnosticRecorder) map[string]string {
	merged := make(map[string]string, len(base)+3)
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range toolDiagnosticsMetadata(policy, recorder) {
		merged[key] = value
	}
	return merged
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

// PolicyToolExecutor enforces agent tool policy and records safe diagnostics.
type PolicyToolExecutor struct {
	policy     string
	workingDir string
	recorder   *ToolDiagnosticRecorder
	fileSystem workerexecution.AgentToolFileSystem
}

func NewPolicyToolExecutor(
	fileSystem workerexecution.AgentToolFileSystem,
	policy string,
	workingDir string,
	recorder *ToolDiagnosticRecorder,
) *PolicyToolExecutor {
	return &PolicyToolExecutor{
		policy:     workerconfig.NormalizeAgentToolPolicy(policy),
		workingDir: strings.TrimSpace(workingDir),
		recorder:   recorder,
		fileSystem: fileSystem,
	}
}

func (executor *PolicyToolExecutor) Execute(ctx context.Context, call messages.ToolCall) (messages.ToolCallResponse, error) {
	_ = ctx
	toolName := strings.TrimSpace(call.Name)
	executor.recorder.Record(toolName, "start", "")

	switch executor.policy {
	case workerconfig.AgentToolPolicyDisabled:
		executor.recorder.Record(toolName, "denied", "policy=disabled")
		return messages.ToolCallResponse{}, fmt.Errorf("%w: tools are disabled for this agent worker", ErrToolPolicyDenied)
	case workerconfig.AgentToolPolicyReadOnly:
		if toolName == ToolNameWriteFile {
			executor.recorder.Record(toolName, "denied", "policy=read_only")
			return messages.ToolCallResponse{}, fmt.Errorf("%w: write tools are not allowed in read-only mode", ErrToolPolicyDenied)
		}
	case workerconfig.AgentToolPolicyEnabled:
	default:
		executor.recorder.Record(toolName, "denied", "policy=invalid")
		return messages.ToolCallResponse{}, fmt.Errorf("%w: unsupported tool policy %q", ErrToolPolicyDenied, executor.policy)
	}

	content, err := executor.executeBoundedTool(toolName, call.Arguments)
	if err != nil {
		safeErr := newToolRuntimeError(toolName, call.Arguments, err)
		executor.recorder.Record(toolName, "failure", toolFailureDetailFromReason(toolName, call.Arguments, safeErr.reason))
		return messages.ToolCallResponse{}, safeErr
	}
	executor.recorder.Record(toolName, "success", contentSummary(content))
	return messages.ToolCallResponse{
		ToolCallID: call.ID,
		Name:       toolName,
		Content:    content,
	}, nil
}

func (executor *PolicyToolExecutor) executeBoundedTool(toolName, arguments string) (string, error) {
	switch toolName {
	case ToolNameReadFile:
		return executor.readFile(arguments)
	case ToolNameListDirectory:
		return executor.listDirectory(arguments)
	case ToolNameWriteFile:
		return executor.writeFile(arguments)
	default:
		return "", fmt.Errorf("%w: %s", ErrToolNotSupported, toolName)
	}
}

type pathArgument struct {
	Path string `json:"path"`
}

type writeFileArgument struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (executor *PolicyToolExecutor) readFile(arguments string) (string, error) {
	var args pathArgument
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("read_file arguments must be JSON with path: %w", err)
	}
	path, err := executor.resolveBoundedPath(args.Path)
	if err != nil {
		return "", err
	}
	if executor.fileSystem == nil {
		return "", ErrToolFileSystemRequired
	}
	data, err := executor.fileSystem.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read_file failed: %w", err)
	}
	return string(data), nil
}

func (executor *PolicyToolExecutor) listDirectory(arguments string) (string, error) {
	var args pathArgument
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("list_directory arguments must be JSON with path: %w", err)
	}
	path, err := executor.resolveBoundedPath(args.Path)
	if err != nil {
		return "", err
	}
	if executor.fileSystem == nil {
		return "", ErrToolFileSystemRequired
	}
	entries, err := executor.fileSystem.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("list_directory failed: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name()+"/")
			continue
		}
		names = append(names, entry.Name())
	}
	return strings.Join(names, "\n"), nil
}

func (executor *PolicyToolExecutor) writeFile(arguments string) (string, error) {
	var args writeFileArgument
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("write_file arguments must be JSON with path and content: %w", err)
	}
	path, err := executor.resolveBoundedPath(args.Path)
	if err != nil {
		return "", err
	}
	if executor.fileSystem == nil {
		return "", ErrToolFileSystemRequired
	}
	if err := executor.fileSystem.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("write_file failed creating parent directories: %w", err)
	}
	if err := executor.fileSystem.WriteFile(path, []byte(args.Content), 0o644); err != nil {
		return "", fmt.Errorf("write_file failed: %w", err)
	}
	return fmt.Sprintf("wrote %d bytes", len(args.Content)), nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func (executor *PolicyToolExecutor) resolveBoundedPath(relativePath string) (string, error) {
	if executor == nil || executor.fileSystem == nil {
		return "", ErrToolFileSystemRequired
	}
	trimmed := strings.TrimSpace(relativePath)
	if trimmed == "" {
		return "", errors.New("tool path is required")
	}
	if isAnyPlatformAbsolutePath(trimmed) {
		return "", errors.New("tool path must be relative to the agent working directory")
	}
	base := executor.workingDir
	if base == "" {
		base = "."
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", errors.New("tool path cannot escape the agent working directory")
	}
	absBase, err := executor.fileSystem.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve agent working directory: %w", err)
	}
	absPath := filepath.Join(absBase, cleaned)
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return "", fmt.Errorf("resolve tool path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("tool path cannot escape the agent working directory")
	}
	info, err := executor.fileSystem.Stat(absBase)
	if err != nil {
		return "", fmt.Errorf("agent working directory is unavailable: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("agent working directory must be a directory")
	}
	if _, err := executor.fileSystem.Stat(absPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("resolve tool path: %w", err)
	}
	return absPath, nil
}

func contentSummary(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "empty"
	}
	return fmt.Sprintf("bytes=%d", len(content))
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
