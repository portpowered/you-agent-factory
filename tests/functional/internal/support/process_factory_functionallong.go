//go:build functionallong

package support

import (
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// RunFactoryToCompletionWithEdgesAndObservationsStable is the retained
// watcher/repeater fallback for scenarios whose Work set can grow after a
// transient idle projection.
func RunFactoryToCompletionWithEdgesAndObservationsStable(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	session, work, events, _ := runFactoryToCompletionWithMode(
		t,
		dir,
		overrides,
		timeout,
		false,
		terminalObservationStableWindow,
	)
	return session, work, events
}
