package workers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const defaultMockWorkerAcceptedOutput = "mock worker accepted"

// MockWorkerCommandRunner applies mock-worker behavior at the subprocess
// boundary while preserving upstream executor setup, prompt rendering, and
// worker-pool dispatch.
type MockWorkerCommandRunner struct {
	Config        *factoryconfig.MockWorkersConfig
	RuntimeConfig interfaces.RuntimeDefinitionLookup
	Next          CommandRunner
}

var _ CommandRunner = (*MockWorkerCommandRunner)(nil)

// Run implements CommandRunner.
func (r *MockWorkerCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if r.Config == nil {
		return r.runNext(ctx, req)
	}
	entry, matched := r.match(req)
	if !matched {
		return r.acceptResult(req), nil
	}

	switch entry.RunType {
	case factoryconfig.MockWorkerRunTypeAccept:
		return r.acceptResult(req), nil
	case factoryconfig.MockWorkerRunTypeReject:
		return rejectResult(entry.RejectConfig), nil
	case factoryconfig.MockWorkerRunTypeScript:
		return r.runScript(ctx, req, entry.ScriptConfig)
	default:
		return r.acceptResult(req), nil
	}
}

func (r *MockWorkerCommandRunner) runScript(ctx context.Context, req CommandRequest, cfg *factoryconfig.MockWorkerScriptConfig) (CommandResult, error) {
	if cfg == nil {
		return CommandResult{Stderr: []byte("mock scriptConfig is required"), ExitCode: 1}, nil
	}
	scriptCtx := ctx
	if cfg.Timeout != "" {
		timeout, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return CommandResult{
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
	scriptReq.Env = mergeCommandEnv(req.Env, commandEnvEntriesFromMap(cfg.Env))
	scriptReq.Stdin = []byte(cfg.Stdin)
	if cfg.WorkingDirectory != "" {
		scriptReq.WorkDir = cfg.WorkingDirectory
	}
	return r.runNext(scriptCtx, scriptReq)
}

func (r *MockWorkerCommandRunner) runNext(ctx context.Context, req CommandRequest) (CommandResult, error) {
	next := r.Next
	if next == nil {
		next = ExecCommandRunner{}
	}
	return next.Run(ctx, req)
}

func (r *MockWorkerCommandRunner) acceptResult(req CommandRequest) CommandResult {
	output := defaultMockWorkerAcceptedOutput
	if r.RuntimeConfig != nil && req.WorkerType != "" {
		if def, ok := r.RuntimeConfig.Worker(req.WorkerType); ok && def != nil && def.StopToken != "" {
			output += "\n" + def.StopToken
		}
	}
	return CommandResult{Stdout: []byte(output)}
}

func rejectResult(cfg *factoryconfig.MockWorkerRejectConfig) CommandResult {
	exitCode := 1
	if cfg == nil {
		return CommandResult{ExitCode: exitCode}
	}
	if cfg.ExitCode != nil {
		exitCode = *cfg.ExitCode
		if exitCode == 0 {
			exitCode = 1
		}
	}
	return CommandResult{
		Stdout:   []byte(cfg.Stdout),
		Stderr:   []byte(cfg.Stderr),
		ExitCode: exitCode,
	}
}

func (r *MockWorkerCommandRunner) match(req CommandRequest) (factoryconfig.MockWorkerConfig, bool) {
	for _, candidate := range r.Config.MockWorkers {
		if mockWorkerMatches(candidate, req) {
			return candidate, true
		}
	}
	return factoryconfig.MockWorkerConfig{}, false
}

func mockWorkerMatches(candidate factoryconfig.MockWorkerConfig, req CommandRequest) bool {
	if candidate.WorkerName != "" && candidate.WorkerName != req.WorkerType {
		return false
	}
	if candidate.WorkstationName != "" && candidate.WorkstationName != req.WorkstationName {
		return false
	}
	for _, selector := range candidate.WorkInputs {
		if !mockWorkInputSelectorMatches(selector, CommandRequestInputTokens(req), req.InputBindings) {
			return false
		}
	}
	return true
}

func mockWorkInputSelectorMatches(selector factoryconfig.MockWorkInputSelector, tokens []interfaces.Token, bindings map[string][]string) bool {
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		if selectorMatchesToken(selector, token, bindings) {
			return true
		}
	}
	return false
}

func selectorMatchesToken(selector factoryconfig.MockWorkInputSelector, token interfaces.Token, bindings map[string][]string) bool {
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

func tokenState(token interfaces.Token) string {
	prefix := token.Color.WorkTypeID + ":"
	if strings.HasPrefix(token.PlaceID, prefix) {
		return strings.TrimPrefix(token.PlaceID, prefix)
	}
	return ""
}

func payloadHash(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
