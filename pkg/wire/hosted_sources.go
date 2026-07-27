package wire

import (
	"net/http"
	"os"

	"github.com/jonboulle/clockwork"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func provideAutomationHostedSourcesFactory(edges serviceedges.Edges) (automations.HostedSourcesFactory, error) {
	checkpointStore := edges.HostedLinearCheckpointStore
	if checkpointStore == nil {
		var err error
		checkpointStore, err = automations.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
		if err != nil {
			return nil, err
		}
	}
	factory := automations.NewHostedSourcesFactory(checkpointStore)
	return func(
		logger *zap.Logger,
		clock workers.HostedPollerClock,
		httpClient workers.HostedPollerHTTPDoer,
		secretResolver workers.HostedPollerSecretResolver,
		linearEndpoint string,
	) automations.HostedPollers {
		if clock == nil {
			clock = clockwork.NewRealClock()
		}
		if httpClient == nil {
			httpClient = &http.Client{Timeout: automations.HostedLinearDefaultRequestTimeout}
		}
		if secretResolver == nil {
			secretResolver = automations.NewHostedLinearSecretResolver(os.Getenv, os.ReadFile)
		}
		return factory(logger, clock, httpClient, secretResolver, linearEndpoint)
	}, nil
}
