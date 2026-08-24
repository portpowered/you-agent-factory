// Package wire is the Automations service composition boundary.
//
// Wire performs construction only, returns the singular automations.Root, and
// starts no lifecycle components. Parent-private
// reconciliation/cron/script-pollers/filesystem-watchers assembly and the
// accepted hosted-sources construction port stay inside the owner boundary;
// peers depend on Service rather than owner internals or construction ports.
package wire

import (
	"context"
	"fmt"
	"sync"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	automationinternal "github.com/portpowered/infinite-you/pkg/services/automations/internal"
	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	hostedsourceswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/wire"
	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// HostedSourceInputs is the cohesive set of external effects needed to
// compose Automations-owned hosted-source and cursor-persistence
// implementations. The application graph selects these effects once; it
// never constructs or passes a hosted-source service into Runtime opening.
type HostedSourceInputs struct {
	Clock            automations.HostedLinearClock
	HTTPClient       automations.HostedLinearHTTPDoer
	SecretResolver   automations.HostedLinearSecretResolver
	LinearEndpoint   string
	CheckpointStore  automations.HostedLinearCheckpointStore
	CursorFileSystem scriptpollerswire.CursorPersistenceFileSystem
}

// NewRoot constructs the singular Automations root. Hosted-source mechanics
// are composed here, behind the owning service boundary, before the root is
// published to peer services.
func NewRoot(
	logger *zap.Logger,
	clock automations.Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hosted HostedSourceInputs,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) (automations.Root, error) {
	if hosted.CursorFileSystem == nil {
		return automations.Root{}, fmt.Errorf("construct Automations: script poller cursor filesystem is required")
	}
	service, err := newService(
		logger,
		clock,
		commandRunner,
		workflowID,
		defaultFactoryDir,
		composeHostedPollers(logger, hosted),
		resolveTemplates,
		executionPolicy,
		hosted.CursorFileSystem,
	)
	if err != nil {
		return automations.Root{}, err
	}
	return service.Root(), nil
}

// NewService constructs an inert owner from explicit construction ports. It is
// retained for owner-local composition tests and focused callers; canonical
// process composition uses NewRoot so hosted-source construction and runtime
// capabilities are published together.
func NewService(
	logger *zap.Logger,
	clock automations.Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) (automations.Service, error) {
	service, err := newService(
		logger,
		clock,
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedPollers,
		resolveTemplates,
		executionPolicy,
		nil,
	)
	if err != nil {
		return nil, err
	}
	return service, nil
}

func newService(
	logger *zap.Logger,
	clock automations.Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cursorFileSystem scriptpollerswire.CursorPersistenceFileSystem,
) (*automationinternal.Service, error) {
	if err := validateDirectDependencies(
		logger,
		clock,
		commandRunner,
		hostedPollers,
		resolveTemplates,
		executionPolicy,
	); err != nil {
		return nil, err
	}

	service := automationinternal.NewWithCursorFileSystem(
		logger,
		clock,
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedPollers,
		resolveTemplates,
		executionPolicy,
		cursorFileSystem,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Automations: implementation rejected its dependencies")
	}
	return service, nil
}

type hostedPollersRootAdapter struct {
	inner hostedsources.HostedPollers
}

func (h hostedPollersRootAdapter) StartLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeConfig factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	worker *factorydefinitions.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	return h.inner.StartLinearPoller(
		ctx,
		sidecars,
		runtimeConfig,
		workstation,
		worker,
		hostedsources.WorkSubmitter(submitter),
	)
}

func (h hostedPollersRootAdapter) ValidateLinearPoller(
	runtimeConfig factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	worker *factorydefinitions.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	return h.inner.ValidateLinearPoller(
		runtimeConfig,
		workstation,
		worker,
		hostedsources.WorkSubmitter(submitter),
	)
}

func composeHostedPollers(
	logger *zap.Logger,
	inputs HostedSourceInputs,
) automations.HostedPollers {
	return hostedPollersRootAdapter{inner: hostedsourceswire.NewHostedPollers(
		logger,
		inputs.Clock,
		inputs.HTTPClient,
		adaptSecretResolver(inputs.SecretResolver),
		inputs.LinearEndpoint,
		inputs.CheckpointStore,
	)}
}

func adaptSecretResolver(
	resolver automations.HostedLinearSecretResolver,
) hostedsources.SecretResolver {
	if resolver == nil {
		return nil
	}
	return func(
		ctx context.Context,
		runtimePaths hostedsources.HostedRuntimePaths,
		secretRef string,
	) (string, error) {
		return resolver(ctx, runtimePaths, secretRef)
	}
}

func validateDirectDependencies(
	logger *zap.Logger,
	clock automations.Clock,
	commandRunner platformprocess.CommandRunner,
	hostedPollers automations.HostedPollers,
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
	if hostedPollers == nil {
		return fmt.Errorf("construct Automations: hosted pollers are required")
	}
	if resolveTemplates == nil {
		return fmt.Errorf("construct Automations: template field resolver is required")
	}
	if executionPolicy == nil {
		return fmt.Errorf("construct Automations: workstation execution policy is required")
	}
	return nil
}
