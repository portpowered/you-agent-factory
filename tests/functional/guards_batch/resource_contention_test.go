package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestConfigDriven_ResourceContention(t *testing.T) {
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "resource_contention")

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Work item A"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Work item B"}`))

	_, listed := runSharedGuardsFactoryToCompletionWithRouteAndWork(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(
			sharedGuardsProviderOutput("Done. COMPLETE"),
			sharedGuardsProviderOutput("Done. COMPLETE"),
		),
	}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:complete": 2})
	assertSharedGuardsProviderCalls(t, dir, 2)
}

func assertGuardSessionPlaces(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func assertGuardResourceAvailability(t *testing.T, session factoryapi.FactorySession, name string, want int) {
	t.Helper()
	for _, usage := range session.Runtime.Usage.Resources {
		if usage.Name == name {
			if usage.Available != want || usage.Total != want {
				t.Errorf("%s resource usage = %#v, want %d available and total", name, usage, want)
			}
			return
		}
	}
	t.Errorf("session resource usage missing %q", name)
}
