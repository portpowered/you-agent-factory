package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestConfigDriven_ResourceContention(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "resource_contention"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Work item A"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Work item B"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
	)

	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{"task:complete": 2})

	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times total, got %d", provider.CallCount())
	}
}

func assertGuardSessionPlaces(t *testing.T, session factoryapi.FactorySession, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.SessionPlaceTokenCount(session, placeID); got != want {
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
