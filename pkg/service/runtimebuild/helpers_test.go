package runtimebuild

import (
	"context"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/factory/replay"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type recordingCommandRunner struct {
	requests []workerprocess.CommandRequest
}

func configWithCommandEdges(t *testing.T, provider, script workers.CommandRunner) Config {
	t.Helper()
	components, err := workerapplication.New(zap.NewNop(), workerapplication.Edges{
		ProviderCommandRunner: provider,
		ScriptCommandRunner:   script,
	})
	if err != nil {
		t.Fatalf("construct worker application: %v", err)
	}
	return Config{WorkerApplication: components}
}

func (r *recordingCommandRunner) Run(_ context.Context, req workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.requests = append(r.requests, req)
	return workerprocess.CommandResult{Stdout: []byte("service-harness-passthrough")}, nil
}

func TestCommandRunnerOverrideForMode_UnmatchedPassthroughDelegatesToNextRunner(t *testing.T) {
	t.Parallel()

	next := &recordingCommandRunner{}
	configured := configWithCommandEdges(t, nil, next)
	cfg := &Config{
		MockWorkersConfig: &factoryconfig.MockWorkersConfig{
			UnmatchedDispatchPolicy: factoryconfig.MockWorkerUnmatchedDispatchPolicyPassthrough,
			MockWorkers: []factoryconfig.MockWorkerConfig{{
				WorkerName: "matched-worker",
				RunType:    factoryconfig.MockWorkerRunTypeReject,
			}},
		},
		WorkerApplication: configured.WorkerApplication,
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

func TestRuntimeBuildDefensiveConstructionBoundaries(t *testing.T) {
	t.Parallel()

	if err := applyOperatorDefaultsToLoadedConfig(operatorconfig.ResolvedDefaults{}, nil); err != nil {
		t.Fatalf("applyOperatorDefaultsToLoadedConfig(nil) error = %v", err)
	}
	var svc *Service
	if configured, err := svc.WithPetriMutationRecorder(nil); configured != nil || err == nil {
		t.Fatalf("nil service WithPetriMutationRecorder() = (%v, %v), want construction error", configured, err)
	}
	if _, err := svc.Build(context.Background(), SessionBuildSpec{}); err == nil {
		t.Fatal("nil service Build() succeeded")
	}
	if _, err := svc.BuildSpec(context.Background(), SessionSpecInput{}); err == nil {
		t.Fatal("nil service BuildSpec() succeeded")
	}
}

func TestCommandRunnerOverrideForMode_UnmatchedDefaultAcceptSkipsNextRunner(t *testing.T) {
	t.Parallel()

	next := &recordingCommandRunner{}
	configured := configWithCommandEdges(t, nil, next)
	cfg := &Config{
		MockWorkersConfig: &factoryconfig.MockWorkersConfig{
			MockWorkers: []factoryconfig.MockWorkerConfig{{
				WorkerName: "matched-worker",
				RunType:    factoryconfig.MockWorkerRunTypeReject,
			}},
		},
		WorkerApplication: configured.WorkerApplication,
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

func TestCommandRunnerOverrideForMode_ReplayReplacesOnlyProductionEdge(t *testing.T) {
	t.Parallel()

	sideEffects := &replay.SideEffects{}
	production := configWithCommandEdges(t, nil, nil)
	if got := commandRunnerOverrideForMode(&production, nil, sideEffects); got != sideEffects {
		t.Fatalf("production replay runner = %T, want replay side effects", got)
	}

	injected := &recordingCommandRunner{}
	functional := configWithCommandEdges(t, nil, injected)
	if got := commandRunnerOverrideForMode(&functional, nil, sideEffects); got != injected {
		t.Fatalf("functional replay runner = %T, want composition-selected runner", got)
	}
}

type stubProvider struct{}

func (stubProvider) Infer(context.Context, workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{}, nil
}

func TestNewSessionLogger_AnnotatesSessionFields(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	logger := NewSessionLogger(zap.New(core), "session-123", "/tmp/folder", "/tmp/factory")
	logger.Info("hello")

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("entry count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["session_id"] != "session-123" {
		t.Fatalf("session_id = %#v, want session-123", fields["session_id"])
	}
	if fields["folder_path"] != "/tmp/folder" {
		t.Fatalf("folder_path = %#v, want /tmp/folder", fields["folder_path"])
	}
	if fields["factory_dir"] != "/tmp/factory" {
		t.Fatalf("factory_dir = %#v, want /tmp/factory", fields["factory_dir"])
	}
}

func TestWarnPortableBundledReplacementReport_LogsTargets(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.WarnLevel)
	WarnPortableBundledReplacementReport(
		zap.New(core),
		"portable replacements updated",
		[]factoryconfig.PortableBundledFileReplacement{
			{TargetPath: "factory/docs/README.md"},
			{TargetPath: "factory/scripts/run.ps1"},
		},
	)

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("entry count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	paths, ok := fields["target_paths"].([]interface{})
	if !ok {
		t.Fatalf("target_paths type = %T, want []interface{}", fields["target_paths"])
	}
	if len(paths) != 2 || paths[0] != "factory/docs/README.md" || paths[1] != "factory/scripts/run.ps1" {
		t.Fatalf("target_paths = %#v, want ordered replacement targets", paths)
	}
}

func TestProviderOverrideForMode_PrefersConfiguredProviderAndFallsBackToReplay(t *testing.T) {
	t.Parallel()

	configured := stubProvider{}
	sideEffects := &replay.SideEffects{}

	if got := providerOverrideForMode(&Config{ProviderOverride: configured}, sideEffects); got != configured {
		t.Fatalf("configured provider = %#v, want %#v", got, configured)
	}
	if got := providerOverrideForMode(&Config{}, sideEffects); got != sideEffects {
		t.Fatalf("fallback provider = %#v, want replay side effects", got)
	}
}

func TestProviderCommandRunnerForMode_WrapsOverrideWhenMockWorkersEnabled(t *testing.T) {
	t.Parallel()

	next := &recordingCommandRunner{}
	configured := configWithCommandEdges(t, next, nil)
	cfg := &Config{MockWorkersConfig: &factoryconfig.MockWorkersConfig{}, WorkerApplication: configured.WorkerApplication}

	runner := providerCommandRunnerForMode(cfg, nil)
	wrapped, ok := runner.(*workers.MockWorkerCommandRunner)
	if !ok {
		t.Fatalf("runner type = %T, want *workers.MockWorkerCommandRunner", runner)
	}
	if wrapped.Next != next {
		t.Fatalf("wrapped.Next = %#v, want %#v", wrapped.Next, next)
	}
}
