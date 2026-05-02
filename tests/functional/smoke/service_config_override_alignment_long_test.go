//go:build functionallong

package smoke

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestServiceConfigOverrideAlignment_ServiceHarnessProviderCommandRunner(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness provider override alignment smoke")

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
