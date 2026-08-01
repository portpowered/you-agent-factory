// Package wire is the Automations service composition boundary.
//
// Wire performs construction only, returns the singular automations.Service
// root interface, and starts no lifecycle components. Parent-private
// reconciliation/cron/script-pollers/filesystem-watchers assembly and the
// accepted hosted-sources construction port stay inside the owner boundary;
// peers depend on Service rather than owner internals or construction ports.
package wire

import (
	"fmt"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	automationinternal "github.com/portpowered/infinite-you/pkg/services/automations/internal"
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
	executionPolicy factorydefinitionswire.WorkstationExecutionPolicyService,
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

	service := automationinternal.NewService(
		logger,
		clock,
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedPollers,
		resolveTemplates,
		executionPolicy,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Automations: implementation rejected its dependencies")
	}
	return service, nil
}

func validateDependencies(
	logger *zap.Logger,
	clock automations.Clock,
	commandRunner workers.CommandRunner,
	hostedSources automations.HostedSourcesFactory,
	hostedClock automations.HostedLinearClock,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitionswire.WorkstationExecutionPolicyService,
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
