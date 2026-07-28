package mockworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	runtimefixtures "github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

type staticRuntimeConfig = runtimefixtures.RuntimeConfigLookupFixture

func TestMockWorkerCommandRunner_DefaultAcceptIncludesConfiguredStopToken(t *testing.T) {
	runner := &MockWorkerCommandRunner{
		Config: NewEmptyMockWorkersConfig(),
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*workerconfig.FactoryWorkerConfig{
				"worker": {Type: workerconfig.WorkerTypeModel, StopToken: "COMPLETE"},
			},
		},
		Next: failCommandRunner{t: t},
	}

	result, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "worker",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if got := string(result.Stdout); got != "mock worker accepted\nCOMPLETE" {
		t.Fatalf("Stdout = %q, want default accepted output with stop token", got)
	}
}

func TestMockWorkerCommandRunner_RejectConfigPreservesObservableOutput(t *testing.T) {
	exitCode := 7
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{MockWorkers: []MockWorkerConfig{{
			WorkerName: "worker",
			RunType:    MockWorkerRunTypeReject,
			RejectConfig: &MockWorkerRejectConfig{
				Stdout:   "mock stdout",
				Stderr:   "mock stderr",
				ExitCode: &exitCode,
			},
		}}},
		Next: failCommandRunner{t: t},
	}

	result, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "worker",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", result.ExitCode)
	}
	if string(result.Stdout) != "mock stdout" || string(result.Stderr) != "mock stderr" {
		t.Fatalf("result output = stdout %q stderr %q, want configured output", result.Stdout, result.Stderr)
	}
}

func TestMockWorkerCommandRunner_RejectConfigWithZeroExitCodeStillFails(t *testing.T) {
	exitCode := 0
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{MockWorkers: []MockWorkerConfig{{
			WorkerName: "worker",
			RunType:    MockWorkerRunTypeReject,
			RejectConfig: &MockWorkerRejectConfig{
				Stdout:   "mock stdout",
				Stderr:   "mock stderr",
				ExitCode: &exitCode,
			},
		}}},
		Next: failCommandRunner{t: t},
	}

	result, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "worker",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want defensive non-zero reject exit code", result.ExitCode)
	}
	if string(result.Stdout) != "mock stdout" || string(result.Stderr) != "mock stderr" {
		t.Fatalf("result output = stdout %q stderr %q, want configured output", result.Stdout, result.Stderr)
	}
}

func TestMockWorkerCommandRunner_UnmatchedDispatchPassthroughUsesNextRunner(t *testing.T) {
	next := &recordingCommandRunner{}
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{
			UnmatchedDispatchPolicy: MockWorkerUnmatchedDispatchPolicyPassthrough,
			MockWorkers: []MockWorkerConfig{{
				WorkerName: "matched-worker",
				RunType:    MockWorkerRunTypeReject,
				RejectConfig: &MockWorkerRejectConfig{
					Stderr: "matched reject",
				},
			}},
		},
		Next: next,
	}

	req := workerprocess.CommandRequest{
		WorkerType:      "other-worker",
		WorkstationName: "process",
	}
	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(next.requests) != 1 {
		t.Fatalf("next runner call count = %d, want 1 passthrough dispatch", len(next.requests))
	}
	got := next.requests[0]
	if got.WorkerType != req.WorkerType || got.WorkstationName != req.WorkstationName {
		t.Fatalf("next runner request = %#v, want worker %q workstation %q", got, req.WorkerType, req.WorkstationName)
	}
	if result.ExitCode != 0 || string(result.Stdout) != "passthrough" {
		t.Fatalf("result = %#v, want passthrough runner output", result)
	}
}

func TestMockWorkerCommandRunner_UnmatchedDispatchDefaultAcceptSkipsNextRunner(t *testing.T) {
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{
			MockWorkers: []MockWorkerConfig{{
				WorkerName: "matched-worker",
				RunType:    MockWorkerRunTypeReject,
			}},
		},
		Next: failCommandRunner{t: t},
	}

	result, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "other-worker",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if string(result.Stdout) != defaultMockWorkerAcceptedOutput {
		t.Fatalf("Stdout = %q, want default accepted output", result.Stdout)
	}
}

func TestMockWorkerCommandRunner_SelectsByWorkerWorkstationAndInput(t *testing.T) {
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{MockWorkers: []MockWorkerConfig{
			{
				WorkerName:      "worker",
				WorkstationName: "other",
				RunType:         MockWorkerRunTypeReject,
			},
			{
				WorkerName:      "worker",
				WorkstationName: "process",
				WorkInputs: []MockWorkInputSelector{{
					WorkID:    "work-1",
					WorkType:  "task",
					State:     "init",
					InputName: "work",
					TraceID:   "trace-1",
				}},
				RunType: MockWorkerRunTypeReject,
				RejectConfig: &MockWorkerRejectConfig{
					Stderr: "matched",
				},
			},
		}},
		Next: failCommandRunner{t: t},
	}

	result, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType:      "worker",
		WorkstationName: "process",
		InputBindings: map[string][]string{
			"work": {"token-1"},
		},
		InputTokens: inputTokens(factoryruntime.RuntimeToken{
			ID:      "token-1",
			PlaceID: "task:init",
			Color: factoryruntime.RuntimeTokenColor{
				DataType:   factoryruntime.RuntimeTokenDataTypeWork,
				WorkID:     "work-1",
				WorkTypeID: "task",
				TraceID:    "trace-1",
			},
		}),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if string(result.Stderr) != "matched" {
		t.Fatalf("Stderr = %q, want matched selector output", result.Stderr)
	}
}

func TestMockWorkerCommandRunner_NilConfigPassthroughUsesNextRunner(t *testing.T) {
	next := &recordingCommandRunner{}
	runner := &MockWorkerCommandRunner{Next: next}

	req := workerprocess.CommandRequest{WorkerType: "worker"}
	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(next.requests) != 1 {
		t.Fatalf("next runner call count = %d, want 1", len(next.requests))
	}
	if next.requests[0].WorkerType != req.WorkerType {
		t.Fatalf("next runner request worker = %q, want %q", next.requests[0].WorkerType, req.WorkerType)
	}
	if string(result.Stdout) != "passthrough" {
		t.Fatalf("Stdout = %q, want passthrough output", result.Stdout)
	}
}

func TestMockWorkerCommandRunner_ScriptConfigTransformsRequest(t *testing.T) {
	next := &inspectingCommandRunner{}
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{MockWorkers: []MockWorkerConfig{{
			WorkerName: "worker",
			RunType:    MockWorkerRunTypeScript,
			ScriptConfig: &MockWorkerScriptConfig{
				Command:          "mock-script",
				Args:             []string{"--flag", "value"},
				Env:              map[string]string{"EXTRA": "overlay", "BASE": "override"},
				WorkingDirectory: "/tmp/mock-worker",
				Stdin:            "script-stdin",
				Timeout:          "50ms",
			},
		}}},
		Next: next,
	}

	_, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "worker",
		Command:    "ignored",
		Args:       []string{"ignored"},
		Env:        []string{"BASE=original", "KEEP=value"},
		Stdin:      []byte("upstream-stdin"),
		WorkDir:    "/original",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if next.req.Command != "mock-script" {
		t.Fatalf("Command = %q, want mock-script", next.req.Command)
	}
	if !reflect.DeepEqual(next.req.Args, []string{"--flag", "value"}) {
		t.Fatalf("Args = %#v, want script args", next.req.Args)
	}
	if !reflect.DeepEqual(next.req.Env, []string{
		"BASE=override", "KEEP=value",
		"YOU_MOCK_WORKER_ARGS_JSON=[\"ignored\"]", "YOU_MOCK_WORKER_COMMAND=ignored", "YOU_MOCK_WORKER_TYPE=worker",
		"EXTRA=overlay",
	}) {
		t.Fatalf("Env = %#v, want merged env preserving order with overlays", next.req.Env)
	}
	if string(next.req.Stdin) != "script-stdin" {
		t.Fatalf("Stdin = %q, want script stdin", next.req.Stdin)
	}
	if next.req.WorkDir != "/tmp/mock-worker" {
		t.Fatalf("WorkDir = %q, want /tmp/mock-worker", next.req.WorkDir)
	}
	if !next.hasDeadline {
		t.Fatal("context deadline missing, want timeout-derived deadline")
	}
}

func TestMockWorkerCommandRunner_ScriptConfigWithoutOverridesKeepsOriginalWorkdir(t *testing.T) {
	next := &inspectingCommandRunner{}
	runner := &MockWorkerCommandRunner{
		Config: &MockWorkersConfig{MockWorkers: []MockWorkerConfig{{
			WorkerName: "worker",
			RunType:    MockWorkerRunTypeScript,
			ScriptConfig: &MockWorkerScriptConfig{
				Command: "mock-script",
				Timeout: "0s",
			},
		}}},
		Next: next,
	}

	_, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "worker",
		WorkDir:    "/original",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if next.req.WorkDir != "/original" {
		t.Fatalf("WorkDir = %q, want original workdir", next.req.WorkDir)
	}
	if next.hasDeadline {
		t.Fatal("context deadline present, want none for zero timeout")
	}
}

func TestMockWorkerCommandRunner_ScriptConfigMissingReturnsFailureResult(t *testing.T) {
	result, err := (&MockWorkerCommandRunner{}).runScript(context.Background(), workerprocess.CommandRequest{}, nil)
	if err != nil {
		t.Fatalf("runScript returned error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1", result.ExitCode)
	}
	if string(result.Stderr) != "mock scriptConfig is required" {
		t.Fatalf("Stderr = %q, want missing config error", result.Stderr)
	}
}

func TestMockWorkerCommandRunner_ScriptConfigInvalidTimeoutReturnsFailureResult(t *testing.T) {
	result, err := (&MockWorkerCommandRunner{}).runScript(context.Background(), workerprocess.CommandRequest{}, &MockWorkerScriptConfig{
		Command: "mock-script",
		Timeout: "definitely-not-a-duration",
	})
	if err != nil {
		t.Fatalf("runScript returned error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1", result.ExitCode)
	}
	if got := string(result.Stderr); got == "" || got == "mock scriptConfig is required" {
		t.Fatalf("Stderr = %q, want timeout validation error", got)
	}
}

func TestMockWorkerCommandRunner_RunNextFailsClosedWhenNextMissing(t *testing.T) {
	_, err := (&MockWorkerCommandRunner{}).runNext(context.Background(), workerprocess.CommandRequest{})
	if err == nil || !strings.Contains(err.Error(), "next command runner is required") {
		t.Fatalf("runNext error = %v, want required injected runner", err)
	}
}

func shellCommandForTest(scriptPath string) string {
	if runtime.GOOS == "windows" {
		return "powershell.exe"
	}
	return scriptPath
}

func shellArgsForTest(scriptPath string) []string {
	if runtime.GOOS == "windows" {
		return []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			`$value = [Console]::In.ReadToEnd(); [Console]::Out.Write("cwd:{0} stdin:{1} env:{2}", (Get-Location).Path, $value, $env:GREETING)`,
		}
	}
	return nil
}

func TestRejectResultNilConfigDefaultsToExitCodeOne(t *testing.T) {
	result := rejectResult(nil)
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1", result.ExitCode)
	}
}

func TestCommandRequestInputTokensDecodesStructuredAndSkipsInvalid(t *testing.T) {
	timestamp := time.Unix(1700000000, 0).UTC()
	tokens := commandRequestInputTokens(workerprocess.CommandRequest{
		InputTokens: []any{
			factoryruntime.RuntimeToken{
				ID:      "token-direct",
				PlaceID: "task:init",
				Color: factoryruntime.RuntimeTokenColor{
					DataType:   factoryruntime.RuntimeTokenDataTypeWork,
					WorkID:     "work-1",
					WorkTypeID: "task",
				},
			},
			map[string]any{
				"id":         "token-map",
				"place_id":   "task:ready",
				"created_at": timestamp.Format(time.RFC3339Nano),
				"entered_at": timestamp.Format(time.RFC3339Nano),
				"color": map[string]any{
					"data_type":    string(factoryruntime.RuntimeTokenDataTypeWork),
					"work_id":      "work-2",
					"work_type_id": "task",
				},
				"history": map[string]any{},
			},
			make(chan int),
		},
	})

	if len(tokens) != 2 {
		t.Fatalf("decoded token count = %d, want 2", len(tokens))
	}
	if tokens[0].ID != "token-direct" || tokens[1].ID != "token-map" {
		t.Fatalf("decoded tokens = %#v, want direct and JSON-decoded tokens", tokens)
	}
}

func TestMockWorkInputSelectorMatchesSkipsResourcesAndChecksAllFields(t *testing.T) {
	payload := []byte("payload")
	sum := sha256.Sum256(payload)
	selector := MockWorkInputSelector{
		WorkID:      "work-1",
		WorkType:    "task",
		State:       "ready",
		InputName:   "work",
		TraceID:     "trace-1",
		Channel:     "chat",
		PayloadHash: "sha256:" + hex.EncodeToString(sum[:]),
	}
	tokens := []factoryruntime.RuntimeToken{
		{
			ID:      "resource-1",
			PlaceID: "resource:ready",
			Color:   factoryruntime.RuntimeTokenColor{DataType: factoryruntime.RuntimeTokenDataTypeResource},
		},
		{
			ID:      "token-1",
			PlaceID: "task:ready",
			Color: factoryruntime.RuntimeTokenColor{
				DataType:   factoryruntime.RuntimeTokenDataTypeWork,
				WorkID:     "work-1",
				WorkTypeID: "task",
				TraceID:    "trace-1",
				Tags:       map[string]string{"channel": "chat"},
				Payload:    payload,
			},
		},
	}

	if !mockWorkInputSelectorMatches(selector, tokens, map[string][]string{"work": {"token-1"}}) {
		t.Fatal("selector should match non-resource token")
	}
	if mockWorkInputSelectorMatches(selector, tokens[:1], map[string][]string{"work": {"token-1"}}) {
		t.Fatal("selector matched resource token, want false")
	}
}

func TestSelectorHelpersCoverMismatchBranches(t *testing.T) {
	token := factoryruntime.RuntimeToken{
		ID:      "token-1",
		PlaceID: "task:ready",
		Color: factoryruntime.RuntimeTokenColor{
			DataType:   factoryruntime.RuntimeTokenDataTypeWork,
			WorkID:     "work-1",
			WorkTypeID: "task",
			TraceID:    "trace-1",
			Tags:       map[string]string{"channel": "chat"},
			Payload:    []byte("payload"),
		},
	}

	if selectorMatchesToken(MockWorkInputSelector{WorkID: "other"}, token, nil) {
		t.Fatal("selector unexpectedly matched wrong work id")
	}
	if selectorMatchesToken(MockWorkInputSelector{WorkType: "other"}, token, nil) {
		t.Fatal("selector unexpectedly matched wrong work type")
	}
	if selectorMatchesToken(MockWorkInputSelector{State: "other"}, token, nil) {
		t.Fatal("selector unexpectedly matched wrong state")
	}
	if selectorMatchesToken(MockWorkInputSelector{InputName: "work"}, token, map[string][]string{"work": {"other-token"}}) {
		t.Fatal("selector unexpectedly matched wrong input binding")
	}
	if selectorMatchesToken(MockWorkInputSelector{TraceID: "other"}, token, nil) {
		t.Fatal("selector unexpectedly matched wrong trace id")
	}
	if selectorMatchesToken(MockWorkInputSelector{Channel: "other"}, token, nil) {
		t.Fatal("selector unexpectedly matched wrong channel")
	}
	if selectorMatchesToken(MockWorkInputSelector{PayloadHash: "sha256:other"}, token, nil) {
		t.Fatal("selector unexpectedly matched wrong payload hash")
	}
}

func TestHelperFunctions(t *testing.T) {
	if bindingContainsToken(nil, "", "token-1") {
		t.Fatal("bindingContainsToken matched empty name")
	}
	if bindingContainsToken(nil, "work", "") {
		t.Fatal("bindingContainsToken matched empty token id")
	}
	if bindingContainsToken(map[string][]string{"work": {"other"}}, "work", "token-1") {
		t.Fatal("bindingContainsToken matched missing token")
	}
	if tokenState(factoryruntime.RuntimeToken{PlaceID: "no-prefix", Color: factoryruntime.RuntimeTokenColor{WorkTypeID: "task"}}) != "" {
		t.Fatal("tokenState returned non-empty state for unrelated place")
	}
	if payloadHash(nil) != "" {
		t.Fatal("payloadHash returned non-empty hash for empty payload")
	}
	if got := payloadHash([]byte("payload")); got == "" || got == "payload" {
		t.Fatalf("payloadHash = %q, want sha256-prefixed digest", got)
	}
}

type failCommandRunner struct {
	t *testing.T
}

func (r failCommandRunner) Run(context.Context, workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.t.Fatal("next command runner should not be called")
	return workerprocess.CommandResult{}, nil
}

type recordingCommandRunner struct {
	requests []workerprocess.CommandRequest
}

func (r *recordingCommandRunner) Run(_ context.Context, req workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.requests = append(r.requests, req)
	return workerprocess.CommandResult{Stdout: []byte("passthrough")}, nil
}

type inspectingCommandRunner struct {
	req         workerprocess.CommandRequest
	hasDeadline bool
}

func (r *inspectingCommandRunner) Run(ctx context.Context, req workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.req = req
	_, r.hasDeadline = ctx.Deadline()
	return workerprocess.CommandResult{}, nil
}

func inputTokens(tokens ...factoryruntime.RuntimeToken) []any {
	out := make([]any, 0, len(tokens))
	for _, token := range tokens {
		out = append(out, token)
	}
	return out
}
