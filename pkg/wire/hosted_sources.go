package wire

import (
	"net/http"
	"os"

	"github.com/jonboulle/clockwork"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"go.uber.org/zap"
)

func provideAutomationHostedSourcesFactory(edges serviceedges.Edges) (automations.HostedSourcesFactory, error) {
	checkpointStore := edges.HostedLinearCheckpointStore
	if checkpointStore == nil {
		var err error
		checkpointStore, err = automationswire.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
		if err != nil {
			return nil, err
		}
	}
	factory := automationswire.NewHostedSourcesFactory(checkpointStore)
	clock := edges.HostedClock
	if clock == nil {
		clock = clockwork.NewRealClock()
	}
	httpClient := edges.HostedHTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: automations.HostedLinearDefaultRequestTimeout}
	}
	secretResolver := edges.HostedSecretResolver
	if secretResolver == nil {
		secretResolver = automationswire.NewHostedLinearSecretResolver(os.Getenv, os.ReadFile)
	}
	linearEndpoint := edges.HostedLinearEndpoint
	return func(
		logger *zap.Logger,
		_ automations.HostedLinearClock,
		_ automations.HostedLinearHTTPDoer,
		_ automations.HostedLinearSecretResolver,
		_ string,
	) automations.HostedPollers {
		return factory(logger, clock, httpClient, secretResolver, linearEndpoint)
	}, nil
}
