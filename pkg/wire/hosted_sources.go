package wire

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
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
	return processWallSchedulingClock{wall: edges.Clock, scheduler: edges.ProcessScheduler}
}

// processWallSchedulingClock projects the normalized process effects into
// consumers whose observations and waits may deliberately use separate sources.
type processWallSchedulingClock struct {
	wall      platformclock.Source
	scheduler platformclock.TimerSource
}

func (c processWallSchedulingClock) Now() time.Time { return c.wall.Now() }

func (c processWallSchedulingClock) After(delay time.Duration) <-chan time.Time {
	return c.scheduler.After(delay)
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

func provideFactoryWebhookHTTPClient(edges serviceedges.Edges) webhookswire.HTTPClient {
	if edges.FactoryWebhookHTTPClient != nil {
		return edges.FactoryWebhookHTTPClient
	}
	return newFactoryWebhookHTTPClient()
}

func provideFactoryWebhookSecretResolver(edges serviceedges.Edges) webhooks.SecretResolver {
	if edges.FactoryWebhookSecretResolver != nil {
		return edges.FactoryWebhookSecretResolver
	}
	resolver := automationswire.NewHostedLinearSecretResolver(os.Getenv, os.ReadFile)
	return func(ctx context.Context, source factorydefinitions.LoadedFactorySource, ref string) (string, error) {
		return resolver(ctx, source, ref)
	}
}

func provideFactoryWebhookClock(edges serviceedges.Edges) webhookswire.Clock {
	if edges.FactoryWebhookClock != nil {
		return edges.FactoryWebhookClock
	}
	return processWallSchedulingClock{wall: edges.Clock, scheduler: edges.ProcessScheduler}
}

func provideFactoryWebhookDeadLetterAppender(edges serviceedges.Edges) webhooks.DeadLetterAppender {
	if edges.FactoryWebhookDeadLetterAppender != nil {
		return edges.FactoryWebhookDeadLetterAppender
	}
	return platformfilesystem.Local{}.AppendDurable
}

func newFactoryWebhookHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
