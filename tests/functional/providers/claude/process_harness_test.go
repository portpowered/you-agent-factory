package claude

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const claudeFunctionalModel = "claude-sonnet-5"

// TestClaudeStreamJSONCommandThroughRootBuildProcess proves the customer
// process dispatches the complete Claude streaming CLI contract and consumes
// its native terminal result successfully.
func TestClaudeStreamJSONCommandThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderClaude,
		claudeFunctionalModel,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"claude stream-json command"}`))

	runner := support.NewRecordingCommandRunner("Done. COMPLETE")
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
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("Claude command calls = %d, want 1", runner.CallCount())
	}

	request := runner.LastRequest()
	if request.Command != string(modelprovider.ProviderClaude) {
		t.Fatalf("command = %q, want %q", request.Command, modelprovider.ProviderClaude)
	}
	support.AssertArgsContainSequence(t, request.Args, []string{
		"--model", claudeFunctionalModel,
		"--verbose",
		"--output-format", "stream-json",
		"--include-partial-messages",
	})
}
