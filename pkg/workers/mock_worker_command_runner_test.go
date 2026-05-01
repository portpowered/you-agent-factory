package workers

import (
	"context"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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

	result, err := runner.Run(context.Background(), CommandRequest{
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

	result, err := runner.Run(context.Background(), CommandRequest{
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

	result, err := runner.Run(context.Background(), CommandRequest{
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

	result, err := runner.Run(context.Background(), CommandRequest{
		WorkerType:      "worker",
		WorkstationName: "process",
		InputBindings: map[string][]string{
			"work": {"token-1"},
		},
		InputTokens: InputTokens(interfaces.Token{
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

func (r failCommandRunner) Run(context.Context, CommandRequest) (CommandResult, error) {
	r.t.Fatal("next command runner should not be called")
	return CommandResult{}, nil
}
