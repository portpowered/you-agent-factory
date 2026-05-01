package smoke

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestServiceConfigOverrideAlignment_ServiceHarnessProviderCommandRunner(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider harness alignment"}`))
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(workers.ModelProviderCodex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "worker-b", support.BuildModelWorkerConfig(workers.ModelProviderCodex, "gpt-5-codex"))

	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("step one complete. COMPLETE")},
		workers.CommandResult{Stdout: []byte("step two complete. COMPLETE")},
	)
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProviderCommandRunner(runner),
	)

	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:failed")
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider command runner calls = %d, want 2", got)
	}
}

func TestServiceConfigOverrideAlignment_ServiceHarnessScriptCommandRunner(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("script harness alignment"))

	runner := support.NewRecordingCommandRunner("script alignment output")
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
	)

	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		HasTokenInPlace("task:done").
		HasNoTokenInPlace("task:failed")
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("script command runner calls = %d, want 1", got)
	}
}
