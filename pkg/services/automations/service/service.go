// Package service is a transitional compile shim that re-exports the composed
// Automations root from pkg/services/automations/internal. Peers should
// construct through automations/wire; baseline deletion of this path is owned
// by DEL-AUTO.
package service

import (
	automationinternal "github.com/portpowered/infinite-you/pkg/services/automations/internal"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// Clock is the automation time source needed for scheduling and supervision.
type Clock = automations.Clock

// Service supervises cron, poller, and watcher automation using injected collaborators.
type Service = automationinternal.Service

// WorkRequestSubmitter submits parsed poller or cron work requests into the runtime.
type WorkRequestSubmitter = automationinternal.WorkRequestSubmitter

// CronTriggerFailure classifies cron tick submit failures for retry policy.
type CronTriggerFailure = automationinternal.CronTriggerFailure

// ScriptPollerRestartBackoffMin is the minimum restart delay after an unexpected
// script poller exit.
const ScriptPollerRestartBackoffMin = automationinternal.ScriptPollerRestartBackoffMin

// CronMaxRetries is the number of retry attempts after the initial cron tick submit.
const CronMaxRetries = automationinternal.CronMaxRetries

// New constructs the automation service from explicit worker-sidecar dependencies.
func New(
	logger *zap.Logger,
	clock Clock,
	commandRunner workers.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) *Service {
	return automationinternal.New(
		logger,
		clock,
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedPollers,
		resolveTemplates,
		executionPolicy,
	)
}

// NewService constructs the Automations root contract for composition.
func NewService(
	logger *zap.Logger,
	clock Clock,
	commandRunner workers.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) *Service {
	return automationinternal.NewService(
		logger,
		clock,
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedPollers,
		resolveTemplates,
		executionPolicy,
	)
}

// ScriptPollerCommandRequest builds the command invocation for a script poller worker.
func ScriptPollerCommandRequest(
	runtimeCfg factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	workerDef *factorydefinitions.FactoryWorkerConfig,
	resolveTemplates workers.TemplateFieldResolver,
) (workers.CommandRequest, error) {
	return automationinternal.ScriptPollerCommandRequest(
		runtimeCfg,
		workstation,
		workerDef,
		resolveTemplates,
	)
}

// ParseScriptPollerOutput parses stdout from a script poller into a work request.
func ParseScriptPollerOutput(stdout []byte) (work.WorkRequest, bool, error) {
	return automationinternal.ParseScriptPollerOutput(stdout)
}

// ClassifyCronTriggerFailure maps cron submit errors to retry policy.
func ClassifyCronTriggerFailure(err error) CronTriggerFailure {
	return automationinternal.ClassifyCronTriggerFailure(err)
}
