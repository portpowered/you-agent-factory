package repeater

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRepeater_YieldsBetweenIterations proves that concurrent Work items seeded
// for a repeater workstation interleave iterations so each item progresses
// without one monopolizing the repeater across reject-then-accept cycles.
func TestRepeater_YieldsBetweenIterations(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-A"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-B"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"exec-worker":   {{Content: "retry"}, {Content: "done COMPLETE"}, {Content: "done COMPLETE"}},
		"finish-worker": {{Content: "done COMPLETE"}, {Content: "done COMPLETE"}},
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)

	if provider.CallCount("exec-worker") < 3 {
		t.Errorf("exec-worker call count = %d, want at least 3 interleaved iterations", provider.CallCount("exec-worker"))
	}
	if provider.CallCount("finish-worker") < 2 {
		t.Errorf("finish-worker call count = %d, want at least 2 completions", provider.CallCount("finish-worker"))
	}
	assertRepeaterWorkStates(t, listed, map[string]int{"task:complete": 2})
}

// TestRepeater_ResourceReleaseBetweenIterations proves that a repeater releases
// held resources between non-accepting iterations so a later accepting output
// can proceed to completion after earlier rejections.
func TestRepeater_ResourceReleaseBetweenIterations(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_resource"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "resource repeater test"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"exec-worker":   {{Content: "retry"}, {Content: "retry"}, {Content: "done COMPLETE"}},
		"finish-worker": {{Content: "done COMPLETE"}},
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)

	if provider.CallCount("exec-worker") != 3 {
		t.Errorf("exec-worker call count = %d, want 3 reject-then-accept iterations", provider.CallCount("exec-worker"))
	}
	assertRepeaterWorkStates(t, listed, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})
}

func assertRepeaterWorkStates(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}
