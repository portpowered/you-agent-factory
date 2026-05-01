package runtime_api

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/service"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestServiceConfigOverrideAlignment_FunctionalHTTPServerProviderCommandRunner(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider server alignment"}`))
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(workers.ModelProviderCodex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "worker-b", support.BuildModelWorkerConfig(workers.ModelProviderCodex, "gpt-5-codex"))

	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("step one complete. COMPLETE")},
		workers.CommandResult{Stdout: []byte("step two complete. COMPLETE")},
	)
	server := startFunctionalServerWithConfig(
		t,
		dir,
		false,
		func(cfg *service.FactoryServiceConfig) {
			cfg.ProviderCommandRunnerOverride = runner
		},
	)

	snapshot := waitForFunctionalServerCompletion(t, server, 10*time.Second)
	categories := categorizeFunctionalState(snapshot)
	if categories.Terminal != 1 {
		t.Fatalf("terminal token count = %d, want 1", categories.Terminal)
	}
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider command runner calls = %d, want 2", got)
	}
}

func TestServiceConfigOverrideAlignment_FunctionalHTTPServerScriptCommandRunner(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("script server alignment"))

	runner := support.NewRecordingCommandRunner("script alignment output")
	server := startFunctionalServerWithConfig(
		t,
		dir,
		false,
		func(cfg *service.FactoryServiceConfig) {
			cfg.CommandRunnerOverride = runner
		},
	)

	snapshot := waitForFunctionalServerCompletion(t, server, 10*time.Second)
	categories := categorizeFunctionalState(snapshot)
	if categories.Terminal != 1 {
		t.Fatalf("terminal token count = %d, want 1", categories.Terminal)
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("script command runner calls = %d, want 1", got)
	}
}
