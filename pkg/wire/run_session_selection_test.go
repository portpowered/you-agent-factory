package wire

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

func TestRelayRuntimeHostReadyPreservesBindingWhenRuntimeAlreadyFinished(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	source := make(chan factorysessions.RuntimeHostBinding, 1)
	target := make(chan initializer.RuntimeHostBinding, 1)
	want := factorysessions.RuntimeHostBinding{Host: "127.0.0.1", Port: 7437}
	source <- want
	close(source)
	cancel()

	done := make(chan struct{})
	go func() {
		relayRuntimeHostReady(ctx, source, target)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("readiness relay did not finish after runtime cancellation")
	}
	got, ok := <-target
	if !ok || got.Host != want.Host || got.Port != want.Port {
		t.Fatalf("relayed binding = %#v, open=%t; want %#v", got, ok, want)
	}
	if _, ok := <-target; ok {
		t.Fatal("readiness relay published more than one binding")
	}
}

func TestRunSessionStartRequestPreservesCLISelections(t *testing.T) {
	t.Parallel()
	skip := true
	mocks := workers.NewEmptyMockWorkersConfig()
	arguments := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"topic": {Values: []string{"session"}},
	}}
	request := runSessionStartRequest(runcli.RunConfig{
		Dir: "factory", FactoryConfigPath: "factory/custom.json", ExecutionBaseDir: "execution",
		FactorySessionID: "session-1", CanonicalSessionID: "canonical-1",
		HomeDir: "home", WorkFile: "work.json", ModelCacheDir: "models",
		RuntimeLogDir: "logs", RuntimeMetricsDir: "metrics",
		RunnerID: "runner", Worktree: "branch", WorkerReasoningEffort: "high",
		InvocationSkipPermissionsOverride: &skip,
		InvocationArguments:               arguments,
		Continuously:                      true, Verbose: true, BindHost: "127.0.0.1", Port: 8080,
		AutoPort: true, Pprof: true, RecordPath: "record.json", ReplayPath: "replay.json",
		ResumePath: "resume.json", Workflow: "workflow",
	}, mocks)
	assertRunSessionRequest(t, request)
	selection := request.RuntimeSelection
	assertRunRuntimeSelection(t, selection)
	assertRunDefinitionArguments(t, selection, arguments)
	assertRunHostSelection(t, selection)
	assertRunWorkerAndRecordingSelection(t, selection, mocks, &skip)
}

func assertRunSessionRequest(t *testing.T, request factorysessions.SessionStartRequest) {
	t.Helper()
	if request.Mode != factorysessions.SessionOperationModeLive || request.ActivationOnly ||
		request.Persistence != factorysessions.PersistencePolicyEnabled || request.SessionID != "session-1" ||
		request.FolderPath != "factory" {
		t.Fatalf("start request = %+v", request)
	}
}

func assertRunRuntimeSelection(t *testing.T, selection *factorysessions.SessionRuntimeSelection) {
	t.Helper()
	if selection == nil || selection.DefinitionSourcePath != "factory/custom.json" ||
		selection.ExecutionBaseDir != "execution" || selection.CanonicalSessionID != "canonical-1" ||
		selection.SystemConfigHome != "home" || selection.WorkFile != "work.json" ||
		selection.ModelCacheDirectory != "models" || selection.Mode != factorysessions.SessionRuntimeModeService ||
		!selection.Verbose || selection.LogDirectory != "logs" || selection.MetricsDirectory != "metrics" ||
		selection.LogPolicy != factorysessions.SessionArtifactPolicyEnabled ||
		selection.MetricsPolicy != factorysessions.SessionArtifactPolicyEnabled {
		t.Fatalf("runtime selection = %+v", selection)
	}
}

func assertRunDefinitionArguments(t *testing.T, selection *factorysessions.SessionRuntimeSelection, arguments *work.InvocationArguments) {
	t.Helper()
	if selection.DefinitionInvocationArguments == arguments ||
		selection.DefinitionInvocationArguments.Arguments["topic"].Values[0] != "session" {
		t.Fatalf("definition invocation arguments were not copied: %+v", selection.DefinitionInvocationArguments)
	}
}

func assertRunHostSelection(t *testing.T, selection *factorysessions.SessionRuntimeSelection) {
	t.Helper()
	if selection.Host.Directory != "factory" || selection.Host.Host != "127.0.0.1" ||
		selection.Host.Port != 8080 || !selection.Host.AutoPort || !selection.Host.Pprof || !selection.Host.MockWorkers {
		t.Fatalf("host selection = %+v", selection.Host)
	}
}

func assertRunWorkerAndRecordingSelection(t *testing.T, selection *factorysessions.SessionRuntimeSelection, mocks *workers.MockWorkersConfig, skip *bool) {
	t.Helper()
	if selection.Workers.MockWorkers != mocks || selection.Workers.RunnerID != "runner" ||
		selection.Workers.Worktree != "branch" || selection.Workers.WorkerReasoningEffort != "high" ||
		selection.Workers.InvocationSkipPermissionsOverride != skip {
		t.Fatalf("worker selection = %+v", selection.Workers)
	}
	if selection.Recording.RecordPath != "record.json" || selection.Recording.ReplayPath != "replay.json" ||
		selection.Recording.ResumePath != "resume.json" || selection.Recording.WorkflowID != "workflow" {
		t.Fatalf("recording selection = %+v", selection.Recording)
	}
}

func TestRunSessionStartRequestKeepsBatchArtifactsEnabled(t *testing.T) {
	t.Parallel()
	request := runSessionStartRequest(runcli.RunConfig{Dir: "factory"}, nil)
	selection := request.RuntimeSelection
	if selection == nil || selection.Mode != factorysessions.SessionRuntimeModeBatch ||
		selection.LogPolicy != factorysessions.SessionArtifactPolicyEnabled ||
		selection.MetricsPolicy != factorysessions.SessionArtifactPolicyEnabled ||
		selection.Host.MockWorkers {
		t.Fatalf("batch selection = %+v", selection)
	}
}
