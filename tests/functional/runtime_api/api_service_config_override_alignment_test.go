package runtime_api

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestServiceConfigOverrideAlignment_FunctionalHTTPServerProviderCommandRunner(t *testing.T) {
	t.Parallel()
	// C06-ISOLATED CASE-21: this witness proves a shaped provider runner is
	// installed before root/service construction, which a reused process cannot
	// establish without changing the behavior under test.
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider server alignment"}`))
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "worker-b", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))

	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("step one complete. COMPLETE")},
		platformprocess.CommandResult{Stdout: []byte("step two complete. COMPLETE")},
	)
	server := startFunctionalServerWithArgs(
		t,
		dir,
		false,
		nil,
		withWorkerCommands(runner, nil),
	)

	status := waitForFunctionalServerCompletion(t, server, 10*time.Second)
	categories := functionalStateCategoriesFromStatus(status)
	if categories.Terminal != 1 {
		t.Fatalf("terminal token count = %d, want 1", categories.Terminal)
	}
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider command runner calls = %d, want 2", got)
	}
}
