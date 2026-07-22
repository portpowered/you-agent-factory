package runtimebuild

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type recordingCommandRunner struct {
	requests []workers.CommandRequest
}

type commandEdges struct {
	Provider workers.CommandRunner
	Script   workers.CommandRunner
}

func newMockCommandRunner(
	config *workers.MockWorkersConfig,
	runtimeConfig interfaces.RuntimeDefinitionLookup,
	next workers.CommandRunner,
) workers.CommandRunner {
	return &recordingMockCommandRunner{
		config:        config,
		runtimeConfig: runtimeConfig,
		next:          next,
	}
}

type recordingMockCommandRunner struct {
	config        *workers.MockWorkersConfig
	runtimeConfig interfaces.RuntimeDefinitionLookup
	next          workers.CommandRunner
}

func (*recordingMockCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func configWithCommandEdges(t *testing.T, provider, script workers.CommandRunner) commandEdges {
	t.Helper()
	return commandEdges{Provider: provider, Script: script}
}

func (r *recordingCommandRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.requests = append(r.requests, req)
	return workers.CommandResult{Stdout: []byte("service-harness-passthrough")}, nil
}

func TestCommandRunnerOverrideForMode_UnmatchedPassthroughDelegatesToNextRunner(t *testing.T) {
	t.Parallel()

	next := &recordingCommandRunner{}
	configured := configWithCommandEdges(t, nil, next)
	mockWorkersConfig := &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName: "matched-worker",
			RunType:    workers.MockWorkerRunTypeReject,
		}},
	}

	runner := commandRunnerOverrideForMode(mockWorkersConfig, configured.Script, nil, nil, newMockCommandRunner)
	if runner == nil {
		t.Fatal("expected mock-worker command runner wrapper")
	}

	wrapped, ok := runner.(*recordingMockCommandRunner)
	if !ok {
		t.Fatalf("runner type = %T, want test-owned mock runner", runner)
	}
	if wrapped.config != mockWorkersConfig {
		t.Fatalf("mock config = %#v, want caller-selected config", wrapped.config)
	}
	if wrapped.next != next {
		t.Fatalf("next runner = %#v, want composition-selected passthrough runner", wrapped.next)
	}
}

func TestRuntimeBuildDefensiveConstructionBoundaries(t *testing.T) {
	t.Parallel()

	if err := applyOperatorDefaultsToLoadedConfig("", "", nil); err != nil {
		t.Fatalf("applyOperatorDefaultsToLoadedConfig(nil) error = %v", err)
	}
	var svc *Service
	if _, err := svc.Build(context.Background(), SessionBuildSpec{}); err == nil {
		t.Fatal("nil service Build() succeeded")
	}
	if _, err := svc.BuildSpec(
		context.Background(), "", "", "", "", nil, "", nil, nil, nil, nil, false,
	); err == nil {
		t.Fatal("nil service BuildSpec() succeeded")
	}
}

func TestCommandRunnerOverrideForMode_UnmatchedDefaultAcceptSkipsNextRunner(t *testing.T) {
	t.Parallel()

	next := &recordingCommandRunner{}
	configured := configWithCommandEdges(t, nil, next)
	mockWorkersConfig := &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName: "matched-worker",
			RunType:    workers.MockWorkerRunTypeReject,
		}},
	}

	runner := commandRunnerOverrideForMode(mockWorkersConfig, configured.Script, nil, nil, newMockCommandRunner)
	if runner == nil {
		t.Fatal("expected mock-worker command runner wrapper")
	}

	wrapped, ok := runner.(*recordingMockCommandRunner)
	if !ok {
		t.Fatalf("runner type = %T, want test-owned mock runner", runner)
	}
	if wrapped.config != mockWorkersConfig || wrapped.next != next {
		t.Fatalf("wrapped inputs = %#v, want exact config and next runner", wrapped)
	}
}

func TestCommandRunnerOverrideForMode_ReplayReplacesOnlyProductionEdge(t *testing.T) {
	t.Parallel()

	sideEffects := &scriptedReplaySideEffects{}
	production := configWithCommandEdges(t, nil, nil)
	if got := commandRunnerOverrideForMode(nil, production.Script, nil, sideEffects, newMockCommandRunner); got != sideEffects {
		t.Fatalf("production replay runner = %T, want replay side effects", got)
	}

	injected := &recordingCommandRunner{}
	functional := configWithCommandEdges(t, nil, injected)
	if got := commandRunnerOverrideForMode(nil, functional.Script, nil, sideEffects, newMockCommandRunner); got != injected {
		t.Fatalf("functional replay runner = %T, want composition-selected runner", got)
	}
}

type stubProvider struct{}

func (stubProvider) Infer(context.Context, workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{}, nil
}

type scriptedReplaySideEffects struct{}

func (*scriptedReplaySideEffects) Infer(
	context.Context,
	workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{}, nil
}

func (*scriptedReplaySideEffects) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
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
		[]interfaces.PortableBundledFileReplacement{
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
	sideEffects := &scriptedReplaySideEffects{}

	if got := providerOverrideForMode(configured, sideEffects); got != configured {
		t.Fatalf("configured provider = %#v, want %#v", got, configured)
	}
	if got := providerOverrideForMode(nil, sideEffects); got != sideEffects {
		t.Fatalf("fallback provider = %#v, want replay side effects", got)
	}
}

func TestProviderCommandRunnerForMode_WrapsOverrideWhenMockWorkersEnabled(t *testing.T) {
	t.Parallel()

	next := &recordingCommandRunner{}
	configured := configWithCommandEdges(t, next, nil)

	runner := providerCommandRunnerForMode(&workers.MockWorkersConfig{}, configured.Provider, nil, newMockCommandRunner)
	wrapped, ok := runner.(*recordingMockCommandRunner)
	if !ok {
		t.Fatalf("runner type = %T, want test-owned mock runner", runner)
	}
	if wrapped.next != next {
		t.Fatalf("wrapped next = %#v, want %#v", wrapped.next, next)
	}
}
