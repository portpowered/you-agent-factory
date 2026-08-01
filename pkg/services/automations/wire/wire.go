// Package wire is the Automations service composition boundary.
//
// Wire performs construction only, returns the singular automations.Service
// root interface, and starts no lifecycle components. Parent-private
// reconciliation/cron/script-pollers/filesystem-watchers assembly and the
// accepted hosted-sources construction port stay inside the owner boundary;
// peers depend on Service rather than owner internals or construction ports.
package wire

import (
	"context"
	"fmt"

	"github.com/jonboulle/clockwork"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	automationinternal "github.com/portpowered/infinite-you/pkg/services/automations/internal"
	cronwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron/wire"
	filesystemwatcherswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers/wire"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// NewService constructs an inert Automations root from construction and
// process-edge ports. It composes the accepted root through parent-private
// reconciliation/cron/script-pollers/filesystem-watchers owner assembly and
// the accepted hosted-sources construction port without publishing owner types
// on the returned peer surface. Missing required construction ports fail with a
// deterministic construction error and a nil service.
func NewService(
	logger *zap.Logger,
	clock automations.Clock,
	commandRunner workers.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedSources automations.HostedSourcesFactory,
	hostedSourcesLogger *zap.Logger,
	hostedClock automations.HostedLinearClock,
	hostedHTTP automations.HostedLinearHTTPDoer,
	hostedSecrets automations.HostedLinearSecretResolver,
	linearEndpoint string,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) (automations.Service, error) {
	if err := validateDependencies(
		logger,
		clock,
		commandRunner,
		hostedSources,
		hostedClock,
		resolveTemplates,
		executionPolicy,
	); err != nil {
		return nil, err
	}

	hostedLogger := hostedSourcesLogger
	if hostedLogger == nil {
		hostedLogger = logger
	}
	hostedPollers := hostedSources(
		hostedLogger,
		hostedClock,
		hostedHTTP,
		hostedSecrets,
		linearEndpoint,
	)
	if hostedPollers == nil {
		return nil, fmt.Errorf("construct Automations: hosted-sources factory returned nil HostedPollers")
	}

	var service *automationinternal.Service
	reconciler := reconciliationwire.NewService(reconciliation.Effects{
		Start: func(ctx context.Context, effect reconciliation.StartEffect) error {
			if service == nil {
				return fmt.Errorf("Automations scheduler service is not initialized")
			}
			return service.StartSchedulerSourceEffect(ctx, effect)
		},
		Stop: func(ctx context.Context, effect reconciliation.StopEffect) error {
			if service == nil {
				return fmt.Errorf("Automations scheduler service is not initialized")
			}
			return service.StopSchedulerSourceEffect(ctx, effect)
		},
		Wait: func(ctx context.Context, effect reconciliation.WaitEffect) (automations.SourceObservation, error) {
			if service == nil {
				return automations.SourceObservation{}, fmt.Errorf("Automations scheduler service is not initialized")
			}
			return service.WaitSchedulerSourceEffect(ctx, effect)
		},
	})
	childScriptPollers := scriptpollerswire.NewService(
		pollerLogger(logger),
		pollerClock(clock),
		pollerCommandRunner(commandRunner),
		resolveTemplates,
		executionPolicy,
	)
	childCron := cronwire.NewService()
	childFilesystemWatchers := filesystemwatcherswire.NewService(pollerClock(clock))
	service = automationinternal.New(
		logger,
		clock,
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedPollers,
		resolveTemplates,
		executionPolicy,
		reconciler,
		childScriptPollers,
		childCron,
		childFilesystemWatchers,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Automations: implementation rejected its dependencies")
	}
	return service, nil
}

func pollerLogger(logger *zap.Logger) func(string, string) *zap.Logger {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(workstationName, workerName string) *zap.Logger {
		return logger.With(
			zap.String("workstation", workstationName),
			zap.String("worker", workerName),
		)
	}
}

func pollerClock(clock automations.Clock) func() clockwork.Clock {
	return func() clockwork.Clock {
		if typed, ok := clock.(clockwork.Clock); ok && typed != nil {
			return typed
		}
		return clockwork.NewRealClock()
	}
}

func pollerCommandRunner(runner workers.CommandRunner) func() workers.CommandRunner {
	return func() workers.CommandRunner { return runner }
}

func validateDependencies(
	logger *zap.Logger,
	clock automations.Clock,
	commandRunner workers.CommandRunner,
	hostedSources automations.HostedSourcesFactory,
	hostedClock automations.HostedLinearClock,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) error {
	if logger == nil {
		return fmt.Errorf("construct Automations: logger is required")
	}
	if clock == nil {
		return fmt.Errorf("construct Automations: clock is required")
	}
	if commandRunner == nil {
		return fmt.Errorf("construct Automations: command runner is required")
	}
	if hostedSources == nil {
		return fmt.Errorf("construct Automations: hosted-sources factory is required")
	}
	if hostedClock == nil {
		return fmt.Errorf("construct Automations: hosted poller clock is required")
	}
	if resolveTemplates == nil {
		return fmt.Errorf("construct Automations: template field resolver is required")
	}
	if executionPolicy == nil {
		return fmt.Errorf("construct Automations: workstation execution policy is required")
	}
	return nil
}
