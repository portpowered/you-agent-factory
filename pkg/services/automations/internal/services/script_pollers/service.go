// Package script_pollers defines the Automations-owned script command/source
// polling capability. Trigger implementations and callers outside Automations
// consume the outer Automations service instead of this private subservice
// contract.
package script_pollers

import (
	"context"
	"sync"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ScriptPollerRestartBackoffMin is the minimum restart delay after an unexpected
// script poller exit.
const ScriptPollerRestartBackoffMin = 25 * time.Millisecond

// Service owns script command/source polling supervision. Only explicit
// supervision operations apply injected command, clock, and admission effects.
type Service interface {
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
	StartScriptPoller(
		context.Context,
		*sync.WaitGroup,
		factorydefinitions.RuntimeConfigLookup,
		factorydefinitions.FactoryWorkstationConfig,
		*factorydefinitions.FactoryWorkerConfig,
		ScriptPollerSupervision,
		automations.WorkRequestSubmitter,
	)
	RunScriptPoller(
		context.Context,
		platformprocess.CommandRunner,
		factorydefinitions.RuntimeConfigLookup,
		factorydefinitions.FactoryWorkstationConfig,
		*factorydefinitions.FactoryWorkerConfig,
		ScriptPollerSupervision,
		automations.WorkRequestSubmitter,
	) error
}

// Scheduler supplies the selected time source for script-poller supervision.
type Scheduler interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

// ScriptPollerCommandRequest builds the command invocation for a script poller worker.
func ScriptPollerCommandRequest(
	runtimeCfg factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	workerDef *factorydefinitions.FactoryWorkerConfig,
	resolveTemplates workers.TemplateFieldResolver,
	resume ResumeCursor,
) (platformprocess.CommandRequest, error) {
	return scriptPollerCommandRequest(runtimeCfg, workstation, workerDef, resolveTemplates, resume)
}

// ParseScriptPollerOutput parses stdout from a script poller into a work request.
func ParseScriptPollerOutput(stdout []byte) (work.WorkRequest, bool, error) {
	return parseScriptPollerOutput(stdout)
}

// ParseScriptPollerStdout parses stdout from a script poller into request and
// opaque recovery facts.
func ParseScriptPollerStdout(stdout []byte) (ScriptPollerStdout, error) {
	return parseScriptPollerStdout(stdout)
}
