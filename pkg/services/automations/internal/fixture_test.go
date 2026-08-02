package internal_test

import (
	"context"
	"sync"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationinternal "github.com/portpowered/infinite-you/pkg/services/automations/internal"
	cronwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron/wire"
	filesystemwatcherswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers/wire"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type automationFixture struct {
	Logger            *zap.Logger
	Clock             factory.Clock
	CommandRunner     workers.CommandRunner
	WorkflowID        string
	DefaultFactoryDir string
	ResolveTemplates  workers.TemplateFieldResolver
	HostedPollers     automations.HostedPollers
}

func newAutomationService(fixture automationFixture) *automationinternal.Service {
	var service *automationinternal.Service
	reconciler := reconciliationwire.NewService(reconciliation.Effects{
		Start: func(ctx context.Context, effect reconciliation.StartEffect) error {
			return service.StartSchedulerSourceEffect(ctx, effect)
		},
		Stop: func(ctx context.Context, effect reconciliation.StopEffect) error {
			return service.StopSchedulerSourceEffect(ctx, effect)
		},
		Wait: func(ctx context.Context, effect reconciliation.WaitEffect) (automations.SourceObservation, error) {
			return service.WaitSchedulerSourceEffect(ctx, effect)
		},
	})
	logger := fixture.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	service = automationinternal.New(
		fixture.Logger,
		fixture.Clock,
		fixture.CommandRunner,
		fixture.WorkflowID,
		fixture.DefaultFactoryDir,
		fixture.HostedPollers,
		fixture.ResolveTemplates,
		reconciler,
		scriptpollerswire.NewService(
			logger,
			fixturePollerClock(fixture.Clock),
			fixture.CommandRunner,
			fixture.ResolveTemplates,
		),
		cronwire.NewService(),
		filesystemwatcherswire.NewService(fixturePollerClock(fixture.Clock)),
	)
	return service
}

func fixturePollerClock(clock factory.Clock) clockwork.Clock {
	if typed, ok := clock.(clockwork.Clock); ok && typed != nil {
		return typed
	}
	return clockwork.NewRealClock()
}

type programmableHostedPollers struct {
	Start func(
		context.Context,
		*sync.WaitGroup,
		factorydefinitions.RuntimeConfigLookup,
		factorydefinitions.FactoryWorkstationConfig,
		*factorydefinitions.FactoryWorkerConfig,
		automations.HostedWorkSubmitter,
	) error
	Validate func(
		factorydefinitions.RuntimeConfigLookup,
		factorydefinitions.FactoryWorkstationConfig,
		*factorydefinitions.FactoryWorkerConfig,
		automations.HostedWorkSubmitter,
	) error
}

func (p programmableHostedPollers) StartLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeConfig factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	worker *factorydefinitions.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	if p.Start == nil {
		return nil
	}
	return p.Start(ctx, sidecars, runtimeConfig, workstation, worker, submitter)
}

func (p programmableHostedPollers) ValidateLinearPoller(
	runtimeConfig factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	worker *factorydefinitions.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	if p.Validate == nil {
		return nil
	}
	return p.Validate(runtimeConfig, workstation, worker, submitter)
}
