//go:build functionallong

package smoke

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestServiceConfigOverrideAlignment_ServiceHarnessScriptCommandRunner(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness script command-runner alignment sweep")

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
