package wire

import (
	"context"
	"net/http"
	"os"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	webhookswire "github.com/portpowered/infinite-you/pkg/services/webhooks/wire"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	"go.uber.org/zap"
)

func provideAutomationHostedClock(edges serviceedges.Edges) automations.HostedLinearClock {
	if edges.HostedClock != nil {
		return edges.HostedClock
	}
	return clockwork.NewRealClock()
}
func provideAutomationHostedHTTPClient(edges serviceedges.Edges) automations.HostedLinearHTTPDoer {
	if edges.HostedHTTPClient != nil {
		return edges.HostedHTTPClient
	}
	return &http.Client{Timeout: automations.HostedLinearDefaultRequestTimeout}
}
func provideAutomationHostedSecretResolver(edges serviceedges.Edges) automations.HostedLinearSecretResolver {
	if edges.HostedSecretResolver != nil {
		return edges.HostedSecretResolver
	}
	return automationswire.NewHostedLinearSecretResolver(os.Getenv, os.ReadFile)
}
func provideAutomationHostedCheckpointStore(edges serviceedges.Edges) (automations.HostedLinearCheckpointStore, error) {
	if edges.HostedLinearCheckpointStore != nil {
		return edges.HostedLinearCheckpointStore, nil
	}
	return automationswire.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
}
func provideAutomationsCursorFileSystem(edges serviceedges.Edges) automationswire.CursorPersistenceFileSystem {
	if edges.AutomationsCursorFileSystem != nil {
		return edges.AutomationsCursorFileSystem
	}
	return platformfilesystem.Local{}
}
func provideAutomationsHostedPollers(logger *zap.Logger, clock automations.HostedLinearClock,
	client automations.HostedLinearHTTPDoer, secrets automations.HostedLinearSecretResolver,
	checkpoints automations.HostedLinearCheckpointStore, edges serviceedges.Edges) automations.HostedPollers {
	return automationswire.NewHostedPollers(logger, clock, client, secrets, edges.HostedLinearEndpoint, checkpoints)
}

// Until T01 integration, retain the existing full-clock selection at canonical composition.
func provideAutomationsClock(clock factoryruntime.Clock) automationswire.Clock {
	if scheduler, ok := clock.(clockwork.Clock); ok {
		return scheduler
	}
	return clockwork.NewRealClock()
}
func provideAutomationsScriptPollers(logger *zap.Logger, clock automationswire.Clock,
	runner platformprocess.CommandRunner, policy factorydefinitions.WorkstationExecutionPolicyService,
	cursors automationswire.CursorScopes) automationswire.ScriptPollers {
	return automationswire.NewScriptPollers(logger, clock, runner, workerswire.ResolveTemplateFields, policy, cursors)
}
func provideAutomationsOwner(logger *zap.Logger, clock automationswire.Clock,
	lifecycle automationswire.SourceLifecycle, reconciler automationswire.Reconciliation,
	scripts automationswire.ScriptPollers, cron automationswire.Cron, watchers automationswire.FilesystemWatchers,
	hosted automations.HostedPollers, policy factorydefinitions.WorkstationExecutionPolicyService,
	cursors automationswire.CursorScopes) *automationswire.Owner {
	return automationswire.NewService(logger, clock, lifecycle, reconciler, scripts, cron, watchers,
		hosted, policy, cursors, true, "", "", "")
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
