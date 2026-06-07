package mockworker

import (
	"context"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	runtimefixtures "github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
)

type staticRuntimeConfig = runtimefixtures.RuntimeConfigLookupFixture

func TestMockWorkerCommandRunner_DefaultAcceptIncludesConfiguredStopToken(t *testing.T) {
	runner := &MockWorkerCommandRunner{
		Config: factoryconfig.NewEmptyMockWorkersConfig(),
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker": {Type: interfaces.WorkerTypeModel, StopToken: "COMPLETE"},
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
		Config: &factoryconfig.MockWorkersConfig{MockWorkers: []factoryconfig.MockWorkerConfig{{
			WorkerName: "worker",
			RunType:    factoryconfig.MockWorkerRunTypeReject,
			RejectConfig: &factoryconfig.MockWorkerRejectConfig{
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
		Config: &factoryconfig.MockWorkersConfig{MockWorkers: []factoryconfig.MockWorkerConfig{{
			WorkerName: "worker",
			RunType:    factoryconfig.MockWorkerRunTypeReject,
			RejectConfig: &factoryconfig.MockWorkerRejectConfig{
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
		Config: &factoryconfig.MockWorkersConfig{
			UnmatchedDispatchPolicy: factoryconfig.MockWorkerUnmatchedDispatchPolicyPassthrough,
			MockWorkers: []factoryconfig.MockWorkerConfig{{
				WorkerName: "matched-worker",
				RunType:    factoryconfig.MockWorkerRunTypeReject,
				RejectConfig: &factoryconfig.MockWorkerRejectConfig{
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
		Config: &factoryconfig.MockWorkersConfig{
			MockWorkers: []factoryconfig.MockWorkerConfig{{
				WorkerName: "matched-worker",
				RunType:    factoryconfig.MockWorkerRunTypeReject,
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
		Config: &factoryconfig.MockWorkersConfig{MockWorkers: []factoryconfig.MockWorkerConfig{
			{
				WorkerName:      "worker",
				WorkstationName: "other",
				RunType:         factoryconfig.MockWorkerRunTypeReject,
			},
			{
				WorkerName:      "worker",
				WorkstationName: "process",
				WorkInputs: []factoryconfig.MockWorkInputSelector{{
					WorkID:    "work-1",
					WorkType:  "task",
					State:     "init",
					InputName: "work",
					TraceID:   "trace-1",
				}},
				RunType: factoryconfig.MockWorkerRunTypeReject,
				RejectConfig: &factoryconfig.MockWorkerRejectConfig{
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
		InputTokens: inputTokens(interfaces.Token{
			ID:      "token-1",
			PlaceID: "task:init",
			Color: interfaces.TokenColor{
				DataType:   interfaces.DataTypeWork,
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

func inputTokens(tokens ...interfaces.Token) []any {
	out := make([]any, 0, len(tokens))
	for _, token := range tokens {
		out = append(out, token)
	}
	return out
}
