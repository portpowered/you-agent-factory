package agent_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestBuildProcessExecutesProviderAttemptThroughRuntimeRoot proves the
// application process still composes the canonical Workers runtime around the
// Providers root and reaches the injected provider command edge.
func TestBuildProcessExecutesProviderAttemptThroughRuntimeRoot(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"runtime provider root"}`))
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("functional-runtime-provider-output COMPLETE"),
	})

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want one", runner.CallCount())
	}
}
