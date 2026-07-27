//go:build functionallong

package watcher

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestWatcherSingleFileCompletesOneWork proves that dropping one watched seed
// file through the public process boundary creates and completes exactly one
// Work item in the Factory-configured success state, with no Work left in
// non-terminal states.
func TestWatcherSingleFileCompletesOneWork(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher single submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "single item"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
		},
	})

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	if provider.CallCount("processor") != 1 {
		t.Fatalf("provider call count = %d, want 1 for single watched file", provider.CallCount("processor"))
	}
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf(
			"session progress categories = %+v, want one terminal and zero failed",
			session.Runtime.Progress.Categories,
		)
	}
	if got := len(listed.Results); got != 1 {
		t.Fatalf("listed Work count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0 after completion", got)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:processing) = %d, want 0 after completion", got)
	}

	workID := support.StringPointerValue(listed.Results[0].WorkId)
	if workID == "" {
		t.Fatalf("completed Work has empty work ID: %#v", listed.Results[0])
	}
	if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "complete")) {
		t.Fatalf("HasWorkAtCustomerState(%q, task:complete) = false; listed=%#v", workID, listed)
	}
	if listed.Results[0].State == nil || listed.Results[0].State.Type != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("completed Work state type = %#v, want TERMINAL", listed.Results[0].State)
	}
}
