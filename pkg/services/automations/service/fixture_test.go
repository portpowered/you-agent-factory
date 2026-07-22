package service_test

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationservice "github.com/portpowered/infinite-you/pkg/services/automations/service"
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

func newAutomationService(fixture automationFixture) *automationservice.Service {
	return automationservice.New(
		fixture.Logger,
		fixture.Clock,
		fixture.CommandRunner,
		fixture.WorkflowID,
		fixture.DefaultFactoryDir,
		fixture.HostedPollers,
		fixture.ResolveTemplates,
		automationWorkstationExecutionPolicy(),
	)
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

func automationWorkstationExecutionPolicy() factorydefinitions.WorkstationExecutionPolicyService {
	return factorydefinitionfixtures.WorkstationExecutionPolicy{
		Resolve: func(workstation *factorydefinitions.FactoryWorkstationConfig) (time.Duration, error) {
			if workstation == nil {
				return 0, nil
			}
			switch workstation.Limits.MaxExecutionTime {
			case "", "0s":
				return 0, nil
			case "1ms":
				return time.Millisecond, nil
			case "75ms":
				return 75 * time.Millisecond, nil
			case "not-a-duration":
				return 0, errors.New(`invalid workstation limits.maxExecutionTime "not-a-duration": time: invalid duration "not-a-duration"`)
			default:
				return 0, errors.New("unscripted workstation execution limit")
			}
		},
	}
}
