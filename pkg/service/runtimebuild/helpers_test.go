package runtimebuild

import (
	"context"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
)

type recordingCommandRunner struct {
	requests []workerprocess.CommandRequest
}

func (r *recordingCommandRunner) Run(_ context.Context, req workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.requests = append(r.requests, req)
	return workerprocess.CommandResult{Stdout: []byte("service-harness-passthrough")}, nil
}

func TestCommandRunnerOverrideForMode_UnmatchedPassthroughDelegatesToNextRunner(t *testing.T) {
	t.Parallel()

	next := &recordingCommandRunner{}
	cfg := &Config{
		MockWorkersConfig: &factoryconfig.MockWorkersConfig{
			UnmatchedDispatchPolicy: factoryconfig.MockWorkerUnmatchedDispatchPolicyPassthrough,
			MockWorkers: []factoryconfig.MockWorkerConfig{{
				WorkerName: "matched-worker",
				RunType:    factoryconfig.MockWorkerRunTypeReject,
			}},
		},
		CommandRunnerOverride: next,
	}

	runner := commandRunnerOverrideForMode(cfg, nil, nil)
	if runner == nil {
		t.Fatal("expected mock-worker command runner wrapper")
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
	if result.ExitCode != 0 || string(result.Stdout) != "service-harness-passthrough" {
		t.Fatalf("result = %#v, want service harness passthrough output", result)
	}
}

func TestCommandRunnerOverrideForMode_UnmatchedDefaultAcceptSkipsNextRunner(t *testing.T) {
	t.Parallel()

	next := &recordingCommandRunner{}
	cfg := &Config{
		MockWorkersConfig: &factoryconfig.MockWorkersConfig{
			MockWorkers: []factoryconfig.MockWorkerConfig{{
				WorkerName: "matched-worker",
				RunType:    factoryconfig.MockWorkerRunTypeReject,
			}},
		},
		CommandRunnerOverride: next,
	}

	runner := commandRunnerOverrideForMode(cfg, nil, nil)
	if runner == nil {
		t.Fatal("expected mock-worker command runner wrapper")
	}

	result, err := runner.Run(context.Background(), workerprocess.CommandRequest{
		WorkerType: "other-worker",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(next.requests) != 0 {
		t.Fatalf("next runner call count = %d, want default accept without passthrough", len(next.requests))
	}
	if string(result.Stdout) != "mock worker accepted" {
		t.Fatalf("Stdout = %q, want default accepted mock output", result.Stdout)
	}
}
