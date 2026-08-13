package workers_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestDetachedContextsCloneRequestScopedValues(t *testing.T) {
	t.Parallel()

	config := &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName: "worker",
			RunType:    workers.MockWorkerRunTypeScript,
			ScriptConfig: &workers.MockWorkerScriptConfig{
				Command: "original",
				Args:    []string{"--original"},
				Env:     map[string]string{"KEY": "original"},
			},
		}},
	}
	ctx := workers.WithMockWorkersConfig(context.Background(), config)
	loaded := workers.MockWorkersConfigFromContext(ctx)
	if loaded == nil {
		t.Fatal("MockWorkersConfigFromContext() = nil, want detached config")
	}
	loaded.MockWorkers[0].ScriptConfig.Args[0] = "mutated"
	loaded.MockWorkers[0].ScriptConfig.Env["KEY"] = "mutated"
	if config.MockWorkers[0].ScriptConfig.Args[0] != "--original" ||
		config.MockWorkers[0].ScriptConfig.Env["KEY"] != "original" {
		t.Fatalf("request-scoped config mutated caller input: %#v", config)
	}

	policy := workers.OutputPolicy{Format: "decision-envelope", DecisionEnvelope: true}
	policyContext := workers.WithMockWorkerOutputPolicy(ctx, policy)
	if got := workers.MockWorkerOutputPolicyFromContext(policyContext); !reflect.DeepEqual(got, policy) {
		t.Fatalf("output policy = %#v, want %#v", got, policy)
	}

	publisher := workers.ProgressPublisher(func(workers.ProgressFragment) {})
	if got := workers.ProgressPublisherFromContext(
		workers.WithProgressPublisher(context.Background(), publisher),
		nil,
	); got == nil {
		t.Fatal("ProgressPublisherFromContext() = nil, want request publisher")
	}
	fallback := workers.ProgressPublisher(func(workers.ProgressFragment) {})
	if got := workers.ProgressPublisherFromContext(context.Background(), fallback); got == nil {
		t.Fatal("ProgressPublisherFromContext() = nil, want construction fallback")
	}
	if workers.WithMockWorkersConfig(nil, config) != nil ||
		workers.WithProgressPublisher(nil, publisher) != nil {
		t.Fatal("nil context should remain nil")
	}
}

func TestMockWorkersConfigLoaderValidatesAndReadsDetachedConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := workers.NewMockWorkersConfigLoader(nil); err == nil {
		t.Fatal("NewMockWorkersConfigLoader(nil) error = nil")
	}
	loader, err := workers.NewMockWorkersConfigLoader(mockConfigReader(func(string) ([]byte, error) {
		return []byte(`{"mockWorkers":[{"workerName":"worker","runType":"script","scriptConfig":{"command":"echo","args":["ok"]}}]}`), nil
	}))
	if err != nil {
		t.Fatalf("NewMockWorkersConfigLoader() error = %v", err)
	}
	empty, err := loader("")
	if err != nil || empty == nil || len(empty.MockWorkers) != 0 {
		t.Fatalf("loader(empty) = %#v, %v, want empty config", empty, err)
	}
	loaded, err := loader("config.json")
	if err != nil || loaded == nil || loaded.MockWorkers[0].ScriptConfig.Command != "echo" {
		t.Fatalf("loader(config) = %#v, %v, want parsed script config", loaded, err)
	}

	readErr := errors.New("read failed")
	failingLoader, err := workers.NewMockWorkersConfigLoader(mockConfigReader(func(string) ([]byte, error) {
		return nil, readErr
	}))
	if err != nil {
		t.Fatalf("NewMockWorkersConfigLoader(failing) error = %v", err)
	}
	if _, err := failingLoader("config.json"); !errors.Is(err, readErr) {
		t.Fatalf("loader(read failure) error = %v, want %v", err, readErr)
	}

	for name, data := range map[string][]byte{
		"trailing JSON":    []byte(`{"mockWorkers":[]} {}`),
		"unknown field":    []byte(`{"unexpected":true}`),
		"invalid run type": []byte(`{"mockWorkers":[{"runType":"unknown"}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := workers.ParseMockWorkersConfig(data); err == nil {
				t.Fatal("ParseMockWorkersConfig() error = nil")
			}
		})
	}
}

func TestContextualMockWorkerCommandRunnerPreservesOverrideAndStreamingBehavior(t *testing.T) {
	t.Parallel()

	var forwarded workers.CommandRequest
	next := mockCommandRunnerFunc(func(_ context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
		forwarded = request
		return workers.CommandResult{Stdout: []byte("forwarded"), Stderr: []byte("warning")}, nil
	})
	runner := workers.NewContextualMockWorkerCommandRunner(next)
	request := workers.CommandRequest{Command: "codex", WorkerType: "worker", WorkstationName: "station"}
	result, err := runner.Run(context.Background(), request)
	if err != nil || string(result.Stdout) != "forwarded" || forwarded.Command != request.Command {
		t.Fatalf("Run(without override) = %#v, %v; forwarded=%#v", result, err, forwarded)
	}

	override := &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "worker",
			WorkstationName: "station",
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	}
	ctx := workers.WithMockWorkerOutputPolicy(
		workers.WithMockWorkersConfig(context.Background(), override),
		workers.OutputPolicy{DecisionEnvelope: true},
	)
	result, err = runner.Run(ctx, request)
	if err != nil || !strings.Contains(string(result.Stdout), "ACCEPTED") {
		t.Fatalf("Run(with override) = %#v, %v, want provider decision envelope", result, err)
	}

	var streams []string
	streaming, ok := runner.(interface {
		RunStreaming(context.Context, workers.CommandRequest, workers.OutputChunkObserver) (workers.CommandResult, error)
	})
	if !ok {
		t.Fatal("contextual runner does not expose RunStreaming")
	}
	result, err = streaming.RunStreaming(ctx, request, func(stream string, chunk []byte) {
		streams = append(streams, string(chunk))
	})
	if err != nil || len(streams) != 1 || string(result.Stdout) == "" {
		t.Fatalf("RunStreaming(with override) = %#v, %v, streams=%#v", result, err, streams)
	}

	passthrough := &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
	}
	if _, err := runner.Run(workers.WithMockWorkersConfig(context.Background(), passthrough), request); err != nil {
		t.Fatalf("Run(passthrough) error = %v", err)
	}
	if _, err := (workers.ContextualMockWorkerCommandRunner{}).Run(context.Background(), request); err == nil {
		t.Fatal("Run(nil next) error = nil")
	}
}

func TestMockWorkerCommandRunnerExecutesScriptAndRejectRoutes(t *testing.T) {
	t.Parallel()

	var scriptRequest workers.CommandRequest
	next := mockCommandRunnerFunc(func(ctx context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
		scriptRequest = request
		if err := ctx.Err(); err != nil {
			return workers.CommandResult{}, err
		}
		return workers.CommandResult{Stdout: []byte("script output")}, nil
	})
	runner := &workers.MockWorkerCommandRunner{Next: next, Config: &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			RunType: workers.MockWorkerRunTypeScript,
			ScriptConfig: &workers.MockWorkerScriptConfig{
				Command:          "script-command",
				Args:             []string{"--flag"},
				Env:              map[string]string{"MOCK": "yes"},
				WorkingDirectory: "workdir",
				Stdin:            "stdin",
			},
		}},
	}}
	result, err := runner.Run(context.Background(), workers.CommandRequest{
		Command: "original", Args: []string{"arg"}, Env: []string{"BASE=1"}, WorkerType: "worker",
	})
	if err != nil || string(result.Stdout) != "script output" || scriptRequest.Command != "script-command" ||
		scriptRequest.WorkDir != "workdir" || string(scriptRequest.Stdin) != "stdin" {
		t.Fatalf("script result/request = %#v, %v / %#v", result, err, scriptRequest)
	}
	if !strings.Contains(strings.Join(scriptRequest.Env, "\n"), "YOU_MOCK_WORKER_COMMAND=original") {
		t.Fatalf("script env = %#v, want original command metadata", scriptRequest.Env)
	}

	reject := &workers.MockWorkerCommandRunner{Config: &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			RunType:      workers.MockWorkerRunTypeReject,
			RejectConfig: &workers.MockWorkerRejectConfig{Stdout: "rejected", ExitCode: func() *int { value := 7; return &value }()},
		}},
	}}
	if result, err := reject.Run(context.Background(), workers.CommandRequest{Command: "other"}); err != nil || result.ExitCode != 7 || string(result.Stdout) != "rejected" {
		t.Fatalf("reject result = %#v, %v", result, err)
	}
	if result, err := reject.Run(context.Background(), workers.CommandRequest{Command: "codex"}); err != nil || result.ExitCode != 0 || !strings.Contains(string(result.Stdout), "turn.failed") {
		t.Fatalf("codex reject result = %#v, %v", result, err)
	}
}

type mockConfigReader func(string) ([]byte, error)

func (reader mockConfigReader) ReadFile(path string) ([]byte, error) { return reader(path) }

type mockCommandRunnerFunc func(context.Context, workers.CommandRequest) (workers.CommandResult, error)

func (runner mockCommandRunnerFunc) Run(ctx context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
	return runner(ctx, request)
}
