package workers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

const defaultMockWorkerAcceptedOutput = "mock worker accepted"

// MockWorkerCommandRunner is the root-contract test seam used to prove that
// provider command adapters preserve Workers dispatch correlation. Full mock
// worker runtime policy remains private to Workers.
type MockWorkerCommandRunner struct {
	Config *MockWorkersConfig
	Next   CommandRunner
}

func (runner *MockWorkerCommandRunner) Run(
	ctx context.Context,
	request CommandRequest,
) (CommandResult, error) {
	if runner.Config == nil {
		return runner.runNext(ctx, request)
	}
	for _, candidate := range runner.Config.MockWorkers {
		if candidate.WorkerName != "" && candidate.WorkerName != request.WorkerType {
			continue
		}
		if candidate.WorkstationName != "" && candidate.WorkstationName != request.WorkstationName {
			continue
		}
		switch candidate.RunType {
		case MockWorkerRunTypeReject:
			return mockWorkerRejectResult(request.Command, candidate.RejectConfig), nil
		case MockWorkerRunTypeScript:
			return CommandResult{Stderr: []byte("root mock worker does not execute scripts"), ExitCode: 1}, nil
		default:
			return CommandResult{Stdout: []byte(mockWorkerAcceptOutput(request.Command))}, nil
		}
	}
	if runner.Config.UnmatchedDispatchPolicy.PassthroughUnmatched() {
		return runner.runNext(ctx, request)
	}
	return CommandResult{Stdout: []byte(mockWorkerAcceptOutput(request.Command))}, nil
}

func (runner *MockWorkerCommandRunner) runNext(
	ctx context.Context,
	request CommandRequest,
) (CommandResult, error) {
	if runner.Next == nil {
		return CommandResult{}, errors.New("mock worker next command runner is required")
	}
	return runner.Next.Run(ctx, request)
}

func mockWorkerAcceptOutput(command string) string {
	switch strings.TrimSpace(command) {
	case "codex":
		return marshalMockWorkerJSON(map[string]any{
			"type": "item.completed",
			"item": map[string]any{"id": "message-final", "type": "agent_message", "text": defaultMockWorkerAcceptedOutput},
		}) + "\n"
	case "claude":
		return marshalMockWorkerJSON(map[string]any{"type": "system", "subtype": "init", "session_id": "mock-claude-session"}) + "\n" +
			marshalMockWorkerJSON(map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": defaultMockWorkerAcceptedOutput, "session_id": "mock-claude-session"}) + "\n"
	default:
		return defaultMockWorkerAcceptedOutput
	}
}

func mockWorkerRejectResult(command string, config *MockWorkerRejectConfig) CommandResult {
	exitCode := 1
	result := CommandResult{ExitCode: exitCode}
	if config != nil {
		result.Stdout = []byte(config.Stdout)
		result.Stderr = []byte(config.Stderr)
		if config.ExitCode != nil && *config.ExitCode != 0 {
			result.ExitCode = *config.ExitCode
		}
	}
	if strings.TrimSpace(command) == "codex" {
		result.Stdout = []byte(marshalMockWorkerJSON(map[string]any{
			"type":  "turn.failed",
			"error": map[string]any{"message": "mock worker rejected the dispatch"},
		}) + "\n")
		result.Stderr = nil
	}
	return result
}

func marshalMockWorkerJSON(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return defaultMockWorkerAcceptedOutput
	}
	return string(payload)
}
