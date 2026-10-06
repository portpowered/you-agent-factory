package cli_rest_journeys_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var namedLifecycleProcess support.ApplicationProcess

// The parent owns one process until all independently scoped CLI journeys join.
func TestFactoryAndSessionCLIJourneys(t *testing.T) {
	t.Parallel()
	yamlParityCommands = &sourceCommandObserver{calls: make(map[string]int), sessions: make(map[string]platformprocess.CommandRunner)}
	yamlParityAPI = support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: yamlParityCommands,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			if _, ok := ctx.Value(validationObservationKey{}).(*validationObservation); ok {
				return rejectValidationHostStart(ctx, request)
			}
			return yamlParityAPI.Start(ctx, request)
		},
		FactorySessionIDGenerator: uuid.NewString,
		WorkRequestIDGenerator:    uuid.NewString,
	})
	namedLifecycleProcess = process
	resolvedSessionCLIProcess = process
	yamlParityCLIProcess = process
	validatePersistCLIProcess = process
	t.Run("NamedFactories", runNamedFactoryCLIJourneys)
	t.Run("SessionCommands", runSessionCLIJourneys)
	t.Run("AuthoredFormats", runFactoryYAMLParityJourneys)
	t.Run("ValidationAndPersistence", runFactoryValidationPersistenceJourneys)
}
