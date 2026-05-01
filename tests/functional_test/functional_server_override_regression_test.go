package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestFunctionalServerOverrideCompatibilityRegression_MockWorkersAndProviderOverride(t *testing.T) {
	t.Run("StartFunctionalServerMockWorkersCompletes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"mock worker compatibility"}`))

		server := StartFunctionalServer(t, dir, true)
		stateResp := server.WaitForCompleted(t, 10*time.Second)

		if stateResp.Categories.Terminal != 1 {
			t.Fatalf("terminal token count = %d, want 1", stateResp.Categories.Terminal)
		}
		if stateResp.Categories.Failed != 0 {
			t.Fatalf("failed token count = %d, want 0", stateResp.Categories.Failed)
		}
	})

	t.Run("ProviderOverrideIsAppliedBeforeServiceBuildForHTTPRuntime", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))
		writeAgentConfig(t, dir, "worker-a", buildModelWorkerConfig(workers.ModelProviderCodex, "gpt-5-codex"))
		writeAgentConfig(t, dir, "worker-b", buildModelWorkerConfig(workers.ModelProviderCodex, "gpt-5-codex"))

		runner := testutil.NewProviderCommandRunner(
			workers.CommandResult{Stdout: []byte("first runtime step complete. COMPLETE")},
			workers.CommandResult{Stdout: []byte("second runtime step complete. COMPLETE")},
		)
		server := StartFunctionalServerWithConfig(
			t,
			dir,
			false,
			func(cfg *service.FactoryServiceConfig) {
				cfg.ProviderCommandRunnerOverride = runner
			},
			factory.WithServiceMode(),
		)

		traceID := server.SubmitWork(t, "task", []byte(`{"title":"provider override regression"}`))
		if traceID == "" {
			t.Fatal("expected POST /work to return a trace ID")
		}

		state := waitForStateSnapshot(
			t,
			10*time.Second,
			func() (StateResponse, bool) {
				stateResp := server.GetState(t)
				return stateResp, stateResp.FactoryState == "RUNNING" &&
					stateResp.RuntimeStatus == string(interfaces.RuntimeStatusIdle) &&
					stateResp.Categories.Terminal == 1
			},
		)
		if state.Categories.Failed != 0 {
			t.Fatalf("failed token count = %d, want 0", state.Categories.Failed)
		}

		if got := runner.CallCount(); got != 2 {
			t.Fatalf("provider command runner calls = %d, want 2", got)
		}
		for i, req := range runner.Requests() {
			if req.Command != string(workers.ModelProviderCodex) {
				t.Fatalf("provider request %d command = %q, want %q", i, req.Command, workers.ModelProviderCodex)
			}
			if req.Execution.TraceID != traceID {
				t.Fatalf("provider request %d trace ID = %q, want %q", i, req.Execution.TraceID, traceID)
			}
		}
	})
}
