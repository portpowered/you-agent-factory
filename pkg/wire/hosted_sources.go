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
	return func(
		logger *zap.Logger,
		clock automations.HostedLinearClock,
		httpClient automations.HostedLinearHTTPDoer,
		secretResolver automations.HostedLinearSecretResolver,
		linearEndpoint string,
	) automations.HostedPollers {
		if clock == nil {
			clock = clockwork.NewRealClock()
		}
		if httpClient == nil {
			httpClient = &http.Client{Timeout: automations.HostedLinearDefaultRequestTimeout}
		}
		if secretResolver == nil {
			secretResolver = automationswire.NewHostedLinearSecretResolver(os.Getenv, os.ReadFile)
		}
		return factory(logger, clock, httpClient, secretResolver, linearEndpoint)
	}, nil
}
