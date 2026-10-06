package cli_rest_journeys_test

import (
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var namedLifecycleProcess support.ApplicationProcess

// The parent owns one process until all independently scoped CLI journeys join.
func TestFactoryAndSessionCLIJourneys(t *testing.T) {
	t.Parallel()
	process := support.BuildProcess(t, serviceedges.Edges{})
	namedLifecycleProcess = process
	resolvedSessionCLIProcess = process
	t.Run("NamedFactories", runNamedFactoryCLIJourneys)
	t.Run("SessionCommands", runSessionCLIJourneys)
}
