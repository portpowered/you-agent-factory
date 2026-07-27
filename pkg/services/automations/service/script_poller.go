package service

import (
	"context"
	"sync"

	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ScriptPollerRestartBackoffMin is the minimum restart delay after an unexpected
// script poller exit.
const ScriptPollerRestartBackoffMin = scriptpollers.ScriptPollerRestartBackoffMin

// StartScriptPoller supervises one script poller workstation until ctx is canceled.
func (s *Service) StartScriptPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	submitter WorkRequestSubmitter,
) {
	if s == nil || s.scriptPollers == nil {
		return
	}
	s.scriptPollers.StartScriptPoller(
		ctx,
		sidecars,
		runtimeCfg,
		workstation,
		workerDef,
		scriptpollers.ScriptPollerSupervision{},
		submitter,
	)
}

// RunScriptPoller executes one script poller command cycle and submits any parsed work request.
func (s *Service) RunScriptPoller(
	ctx context.Context,
	runner workers.CommandRunner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	submitter WorkRequestSubmitter,
) error {
	if s == nil || s.scriptPollers == nil {
		return nil
	}
	return s.scriptPollers.RunScriptPoller(
		ctx,
		runner,
		runtimeCfg,
		workstation,
		workerDef,
		scriptpollers.ScriptPollerSupervision{},
		submitter,
	)
}

// ScriptPollerCommandRequest builds the command invocation for a script poller worker.
func ScriptPollerCommandRequest(
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	resolveTemplates workers.TemplateFieldResolver,
) (workers.CommandRequest, error) {
	return scriptpollers.ScriptPollerCommandRequest(
		runtimeCfg,
		workstation,
		workerDef,
		resolveTemplates,
		scriptpollers.ResumeCursor{},
	)
}

// ParseScriptPollerOutput parses stdout from a script poller into a work request.
func ParseScriptPollerOutput(stdout []byte) (work.WorkRequest, bool, error) {
	return scriptpollers.ParseScriptPollerOutput(stdout)
}
