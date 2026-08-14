package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const defaultMockWorkerAcceptedOutput = "mock worker accepted"

// MockWorkerCommandRunner is the root-contract test seam used to prove that
// provider command adapters preserve Workers dispatch correlation. Full mock
// worker runtime policy remains private to Workers.
type MockWorkerCommandRunner struct {
	Config       *MockWorkersConfig
	Next         CommandRunner
	OutputPolicy OutputPolicy
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
			return runner.runScript(ctx, request, candidate.ScriptConfig)
		default:
			return CommandResult{Stdout: []byte(mockWorkerAcceptOutput(
				request.Command,
				runner.OutputPolicy,
			))}, nil
		}
	}
	if runner.Config.UnmatchedDispatchPolicy.PassthroughUnmatched() {
		return runner.runNext(ctx, request)
	}
	return CommandResult{Stdout: []byte(mockWorkerAcceptOutput(
		request.Command,
		runner.OutputPolicy,
	))}, nil
}

// CommandResultForLogging preserves the configured process exit code in
// diagnostics when a Codex mock encodes its terminal failure in turn.failed
// output and therefore returns a successful process result to the adapter.
func (runner *MockWorkerCommandRunner) CommandResultForLogging(
	_ context.Context,
	request CommandRequest,
	result CommandResult,
) CommandResult {
	if runner == nil || runner.Config == nil {
		return result
	}
	for _, candidate := range runner.Config.MockWorkers {
		if candidate.WorkerName != "" && candidate.WorkerName != request.WorkerType {
			continue
		}
		if candidate.WorkstationName != "" && candidate.WorkstationName != request.WorkstationName {
			continue
		}
		if candidate.RunType != MockWorkerRunTypeReject ||
			strings.TrimSpace(request.Command) != "codex" ||
			candidate.RejectConfig == nil || candidate.RejectConfig.ExitCode == nil ||
			*candidate.RejectConfig.ExitCode == 0 {
			return result
		}
		result.ExitCode = *candidate.RejectConfig.ExitCode
		return result
	}
	return result
}

func (runner *MockWorkerCommandRunner) runScript(
	ctx context.Context,
	request CommandRequest,
	config *MockWorkerScriptConfig,
) (CommandResult, error) {
	if config == nil {
		return CommandResult{Stderr: []byte("mock scriptConfig is required"), ExitCode: 1}, nil
	}
	scriptContext := ctx
	if config.Timeout != "" {
		timeout, err := time.ParseDuration(config.Timeout)
		if err != nil {
			return CommandResult{
				Stderr:   []byte(fmt.Sprintf("invalid mock script timeout %q: %v", config.Timeout, err)),
				ExitCode: 1,
			}, nil
		}
		if timeout > 0 {
			var cancel context.CancelFunc
			scriptContext, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
	}

	scriptRequest := request
	scriptRequest.Command = config.Command
	scriptRequest.Args = append([]string(nil), config.Args...)
	scriptRequest.Env = MergeCommandEnv(
		request.Env,
		CommandEnvEntriesFromMap(mockWorkerOriginalCommandEnv(request)),
		CommandEnvEntriesFromMap(config.Env),
	)
	scriptRequest.Stdin = []byte(config.Stdin)
	if config.WorkingDirectory != "" {
		scriptRequest.WorkDir = config.WorkingDirectory
	}
	return runner.runNext(scriptContext, scriptRequest)
}

func mockWorkerOriginalCommandEnv(request CommandRequest) map[string]string {
	args, _ := json.Marshal(request.Args)
	return map[string]string{
		"YOU_MOCK_WORKER_COMMAND":   request.Command,
		"YOU_MOCK_WORKER_ARGS_JSON": string(args),
		"YOU_MOCK_WORKER_TYPE":      request.WorkerType,
	}
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

func mockWorkerAcceptOutput(command string, policy OutputPolicy) string {
	acceptedOutput := defaultMockWorkerAcceptedOutput
	if policy.GoalRoutingDecisionEnvelope {
		acceptedOutput = marshalMockWorkerJSON(map[string]any{
			"decision": "accepted",
			"output":   defaultMockWorkerAcceptedOutput,
		})
	} else if policy.DecisionEnvelope ||
		strings.EqualFold(strings.TrimSpace(policy.Format), "decision-envelope") {
		acceptedOutput = marshalMockWorkerJSON(map[string]any{
			"decision": "ACCEPTED",
			"output":   defaultMockWorkerAcceptedOutput,
		})
	} else if stopToken := strings.TrimSpace(policy.StopToken); stopToken != "" {
		acceptedOutput = defaultMockWorkerAcceptedOutput + "\n" + stopToken
	}
	switch strings.TrimSpace(command) {
	case "codex":
		return strings.Join([]string{
			marshalMockWorkerJSON(map[string]any{"type": "turn.started"}),
			marshalMockWorkerJSON(map[string]any{
				"type": "item.completed",
				"item": map[string]any{"id": "message-final", "type": "agent_message", "text": acceptedOutput},
			}),
			marshalMockWorkerJSON(map[string]any{
				"type":  "turn.completed",
				"usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
			}),
		}, "\n") + "\n"
	case "claude":
		return marshalMockWorkerJSON(map[string]any{"type": "system", "subtype": "init", "session_id": "mock-claude-session"}) + "\n" +
			marshalMockWorkerJSON(map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": acceptedOutput, "session_id": "mock-claude-session"}) + "\n"
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
		result.ExitCode = 0
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
