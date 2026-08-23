package mockworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

const defaultMockWorkerAcceptedOutput = "mock worker accepted"

// MockWorkerCommandRunner applies mock-worker behavior at the subprocess
// boundary while preserving upstream executor setup, prompt rendering, and
// worker-pool dispatch.
type MockWorkerCommandRunner struct {
	Config        *MockWorkersConfig
	RuntimeConfig interfaces.RuntimeDefinitionLookup
	OutputPolicy  workers.OutputPolicy
	Next          workerprocess.CommandRunner
}

var _ workerprocess.CommandRunner = (*MockWorkerCommandRunner)(nil)

// Run implements CommandRunner.
func (r *MockWorkerCommandRunner) Run(ctx context.Context, req workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	if r.Config == nil {
		return r.runNext(ctx, req)
	}
	entry, matched := r.match(req)
	if !matched {
		if r.Config.UnmatchedDispatchPolicy.PassthroughUnmatched() {
			return r.runNext(ctx, req)
		}
		return r.acceptResult(req), nil
	}

	switch entry.RunType {
	case MockWorkerRunTypeAccept:
		return r.acceptResult(req), nil
	case MockWorkerRunTypeReject:
		result := mockRejectResult(req.Command, entry.RejectConfig)
		// Codex reports a structured turn.failed event on stdout. Keep the
		// subprocess result successful so the provider adapter can interpret
		// that event; CommandResultForLogging restores a configured mock exit
		// code in diagnostics when one was requested.
		if strings.TrimSpace(req.Command) == "codex" {
			result.ExitCode = 0
		}
		return result, nil
	case MockWorkerRunTypeScript:
		return r.runScript(ctx, req, entry.ScriptConfig)
	default:
		return r.acceptResult(req), nil
	}
}

// CommandResultForLogging preserves a configured mock exit code in diagnostics
// when a Codex rejection is encoded as a successful structured provider
// response for adapter parsing.
func (r *MockWorkerCommandRunner) CommandResultForLogging(
	_ context.Context,
	request workerprocess.CommandRequest,
	result workerprocess.CommandResult,
) workerprocess.CommandResult {
	if r == nil || r.Config == nil || strings.TrimSpace(request.Command) != "codex" {
		return result
	}
	entry, matched := r.match(request)
	if !matched || entry.RunType != MockWorkerRunTypeReject ||
		entry.RejectConfig == nil || entry.RejectConfig.ExitCode == nil ||
		*entry.RejectConfig.ExitCode == 0 {
		return result
	}
	result.ExitCode = *entry.RejectConfig.ExitCode
	return result
}

func (r *MockWorkerCommandRunner) runScript(ctx context.Context, req workerprocess.CommandRequest, cfg *MockWorkerScriptConfig) (workerprocess.CommandResult, error) {
	if cfg == nil {
		return workerprocess.CommandResult{Stderr: []byte("mock scriptConfig is required"), ExitCode: 1}, nil
	}
	scriptCtx := ctx
	if cfg.Timeout != "" {
		timeout, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return workerprocess.CommandResult{
				Stderr:   []byte(fmt.Sprintf("invalid mock script timeout %q: %v", cfg.Timeout, err)),
				ExitCode: 1,
			}, nil
		}
		if timeout > 0 {
			var cancel context.CancelFunc
			scriptCtx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
	}

	scriptReq := req
	scriptReq.Command = cfg.Command
	scriptReq.Args = append([]string(nil), cfg.Args...)
	scriptReq.Env = workerprocess.MergeCommandEnv(
		req.Env,
		workerprocess.CommandEnvEntriesFromMap(mockWorkerOriginalCommandEnv(req)),
		workerprocess.CommandEnvEntriesFromMap(cfg.Env),
	)
	scriptReq.Stdin = []byte(cfg.Stdin)
	if cfg.WorkingDirectory != "" {
		scriptReq.WorkDir = cfg.WorkingDirectory
	}
	return r.runNext(scriptCtx, scriptReq)
}

func mockWorkerOriginalCommandEnv(req workerprocess.CommandRequest) map[string]string {
	// CommandRequest.Args is []string, so JSON encoding cannot fail.
	args, _ := json.Marshal(req.Args)
	return map[string]string{
		"YOU_MOCK_WORKER_COMMAND":   req.Command,
		"YOU_MOCK_WORKER_ARGS_JSON": string(args),
		"YOU_MOCK_WORKER_TYPE":      req.WorkerType,
	}
}

func (r *MockWorkerCommandRunner) runNext(ctx context.Context, req workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	next := r.Next
	if next == nil {
		return workerprocess.CommandResult{}, errors.New("mock worker next command runner is required")
	}
	return next.Run(ctx, req)
}

func (r *MockWorkerCommandRunner) acceptResult(req workerprocess.CommandRequest) workerprocess.CommandResult {
	output := defaultMockWorkerAcceptedOutput
	if r.OutputPolicy.GoalRoutingDecisionEnvelope {
		output = `{"decision":"accepted","output":"` + defaultMockWorkerAcceptedOutput + `"}`
	} else if r.OutputPolicy.DecisionEnvelope || strings.EqualFold(strings.TrimSpace(r.OutputPolicy.Format), "decision-envelope") {
		output = `{"decision":"ACCEPTED","output":"` + defaultMockWorkerAcceptedOutput + `"}`
	} else if stopToken := strings.TrimSpace(r.OutputPolicy.StopToken); stopToken != "" {
		output += "\n" + stopToken
	}
	if r.RuntimeConfig != nil && req.WorkstationName != "" {
		if def, ok := r.RuntimeConfig.Workstation(req.WorkstationName); ok &&
			def != nil && def.OutcomeFormat == interfaces.WorkstationOutcomeFormatDecisionEnvelope {
			output = `{"decision":"ACCEPTED","output":"` + defaultMockWorkerAcceptedOutput + `"}`
			return workerprocess.CommandResult{Stdout: []byte(mockAcceptStdout(req.Command, output))}
		}
	}
	if r.RuntimeConfig != nil && req.WorkerType != "" {
		if def, ok := r.RuntimeConfig.Worker(req.WorkerType); ok && def != nil && def.StopToken != "" {
			output += "\n" + def.StopToken
		}
	}
	return workerprocess.CommandResult{Stdout: []byte(mockAcceptStdout(req.Command, output))}
}

func rejectResult(cfg *MockWorkerRejectConfig) workerprocess.CommandResult {
	exitCode := 1
	if cfg == nil {
		return workerprocess.CommandResult{ExitCode: exitCode}
	}
	if cfg.ExitCode != nil {
		exitCode = *cfg.ExitCode
		if exitCode == 0 {
			exitCode = 1
		}
	}
	return workerprocess.CommandResult{
		Stdout:   []byte(cfg.Stdout),
		Stderr:   []byte(cfg.Stderr),
		ExitCode: exitCode,
	}
}

func (r *MockWorkerCommandRunner) match(req workerprocess.CommandRequest) (MockWorkerConfig, bool) {
	for _, candidate := range r.Config.MockWorkers {
		if mockWorkerMatches(candidate, req) {
			return candidate, true
		}
	}
	return MockWorkerConfig{}, false
}

func mockWorkerMatches(candidate MockWorkerConfig, req workerprocess.CommandRequest) bool {
	if candidate.WorkerName != "" && candidate.WorkerName != req.WorkerType {
		return false
	}
	if candidate.WorkstationName != "" && candidate.WorkstationName != req.WorkstationName {
		return false
	}
	for _, selector := range candidate.WorkInputs {
		if !mockWorkInputSelectorMatches(selector, req.Inputs, nil) {
			return false
		}
	}
	return true
}

type mockInputToken struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Color struct {
		WorkID     string            `json:"work_id"`
		WorkTypeID string            `json:"work_type_id"`
		DataType   string            `json:"data_type"`
		TraceID    string            `json:"trace_id"`
		Tags       map[string]string `json:"tags"`
		Payload    []byte            `json:"payload"`
	} `json:"color"`
}

func commandRequestInputTokens(request workerprocess.CommandRequest) []mockInputToken {
	if len(request.Inputs) == 0 {
		return nil
	}

	out := make([]mockInputToken, 0, len(request.Inputs))
	for index, input := range request.Inputs {
		out = append(out, mockInputToken{
			ID:    fmt.Sprintf("input-%d", index),
			State: input.State,
			Color: struct {
				WorkID     string            `json:"work_id"`
				WorkTypeID string            `json:"work_type_id"`
				DataType   string            `json:"data_type"`
				TraceID    string            `json:"trace_id"`
				Tags       map[string]string `json:"tags"`
				Payload    []byte            `json:"payload"`
			}{
				WorkID: input.WorkID, WorkTypeID: input.WorkTypeID,
				DataType: input.Kind, TraceID: input.Lineage.TraceID,
				Tags: input.Tags, Payload: contentPayload(input.Content),
			},
		})
	}
	return out
}

func contentPayload(content []work.WorkContentPart) []byte {
	for _, part := range content {
		if part.Type.Normalized() == work.WorkContentPartTypeText {
			return []byte(part.Text)
		}
	}
	return nil
}

func decodeToken(raw any) (mockInputToken, bool) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return mockInputToken{}, false
	}
	var token mockInputToken
	if err := json.Unmarshal(encoded, &token); err != nil {
		return mockInputToken{}, false
	}
	return token, true
}

func mockWorkInputSelectorMatches(selector MockWorkInputSelector, rawTokens any, bindings map[string][]string) bool {
	if inputs, ok := rawTokens.([]workers.WorkInput); ok {
		for _, input := range inputs {
			if input.Kind == string(workers.DataTypeResource) {
				continue
			}
			if selectorMatchesWorkInput(selector, input) {
				return true
			}
		}
		return false
	}
	tokens, ok := decodeTokens(rawTokens)
	if !ok {
		return false
	}
	for _, token := range tokens {
		if token.Color.DataType == "resource" {
			continue
		}
		if selectorMatchesDecodedToken(selector, token, bindings) {
			return true
		}
	}
	return false
}

func selectorMatchesWorkInput(selector MockWorkInputSelector, input workers.WorkInput) bool {
	if selector.WorkID != "" && selector.WorkID != input.WorkID {
		return false
	}
	if selector.WorkType != "" && selector.WorkType != input.WorkTypeID {
		return false
	}
	if selector.State != "" && selector.State != input.State {
		return false
	}
	if selector.InputName != "" && !containsString(input.InputNames, selector.InputName) {
		return false
	}
	if selector.TraceID != "" && selector.TraceID != input.Lineage.TraceID {
		return false
	}
	if selector.Channel != "" && selector.Channel != input.Tags["channel"] {
		return false
	}
	if selector.PayloadHash != "" && selector.PayloadHash != payloadHash(contentPayload(input.Content)) {
		return false
	}
	return true
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func decodeTokens(raw any) ([]mockInputToken, bool) {
	if tokens, ok := raw.([]mockInputToken); ok {
		return tokens, true
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var tokens []mockInputToken
	if err := json.Unmarshal(encoded, &tokens); err != nil {
		return nil, false
	}
	return tokens, true
}

func selectorMatchesToken(selector MockWorkInputSelector, rawToken any, bindings map[string][]string) bool {
	token, ok := decodeToken(rawToken)
	return ok && selectorMatchesDecodedToken(selector, token, bindings)
}

func selectorMatchesDecodedToken(selector MockWorkInputSelector, token mockInputToken, bindings map[string][]string) bool {
	if selector.WorkID != "" && selector.WorkID != token.Color.WorkID {
		return false
	}
	if selector.WorkType != "" && selector.WorkType != token.Color.WorkTypeID {
		return false
	}
	if selector.State != "" && selector.State != tokenState(token) {
		return false
	}
	if selector.InputName != "" && !bindingContainsToken(bindings, selector.InputName, token.ID) {
		return false
	}
	if selector.TraceID != "" && selector.TraceID != token.Color.TraceID {
		return false
	}
	if selector.Channel != "" && selector.Channel != token.Color.Tags["channel"] {
		return false
	}
	if selector.PayloadHash != "" && selector.PayloadHash != payloadHash(token.Color.Payload) {
		return false
	}
	return true
}

func bindingContainsToken(bindings map[string][]string, name string, tokenID string) bool {
	if name == "" || tokenID == "" {
		return false
	}
	for _, candidate := range bindings[name] {
		if candidate == tokenID {
			return true
		}
	}
	return false
}

func tokenState(rawToken any) string {
	token, ok := decodeToken(rawToken)
	if !ok {
		return ""
	}
	return token.State
}

func payloadHash(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
