package wire

import (
	"context"
	"net/http"
	"os"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	webhookswire "github.com/portpowered/infinite-you/pkg/services/webhooks/wire"
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

func provideFactoryWebhooksService(
	edges serviceedges.Edges,
	logger logging.Logger,
) webhooks.Service {
	httpClient := edges.FactoryWebhookHTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	secretResolver := edges.FactoryWebhookSecretResolver
	if secretResolver == nil {
		hostedResolver := automationswire.NewHostedLinearSecretResolver(os.Getenv, os.ReadFile)
		secretResolver = func(
			ctx context.Context,
			source factorydefinitions.LoadedFactorySource,
			secretRef string,
		) (string, error) {
			return hostedResolver(ctx, source, secretRef)
		}
	}
	clockSource := edges.Clock
	if clockSource == nil {
		clockSource = platformclock.Real{}
	}
	return webhookswire.NewService(httpClient, secretResolver, clockSource, logger)
}
