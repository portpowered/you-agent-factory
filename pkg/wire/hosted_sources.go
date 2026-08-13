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
)

func provideAutomationHostedSourceInputs(
	edges serviceedges.Edges,
) (automationswire.HostedSourceInputs, error) {
	checkpointStore := edges.HostedLinearCheckpointStore
	if checkpointStore == nil {
		var err error
		checkpointStore, err = automationswire.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
		if err != nil {
			return automationswire.HostedSourceInputs{}, err
		}
	}
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
	return automationswire.HostedSourceInputs{
		Clock:           clock,
		HTTPClient:      httpClient,
		SecretResolver:  secretResolver,
		LinearEndpoint:  edges.HostedLinearEndpoint,
		CheckpointStore: checkpointStore,
	}, nil
}

func provideFactoryWebhooksService(
	edges serviceedges.Edges,
	logger logging.Logger,
) webhooks.Service {
	httpClient := edges.FactoryWebhookHTTPClient
	if httpClient == nil {
		httpClient = newFactoryWebhookHTTPClient()
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
	clockSource := edges.FactoryWebhookClock
	if clockSource == nil {
		clockSource = platformclock.Real{}
	}
	deadLetterAppender := edges.FactoryWebhookDeadLetterAppender
	if deadLetterAppender == nil {
		deadLetterAppender = platformfilesystem.Local{}.AppendDurable
	}
	return webhookswire.NewService(
		httpClient,
		secretResolver,
		clockSource,
		deadLetterAppender,
		logger,
	)
}

func newFactoryWebhookHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
