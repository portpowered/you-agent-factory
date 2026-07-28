// Package script_pollers defines the Automations-owned script command/source
// polling capability. Trigger implementations and callers outside Automations
// consume the outer Automations service instead of this private subservice
// contract.
package script_pollers

import (
	"context"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

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
		workers.CommandRunner,
		factorydefinitions.RuntimeConfigLookup,
		factorydefinitions.FactoryWorkstationConfig,
		*factorydefinitions.FactoryWorkerConfig,
		ScriptPollerSupervision,
		automations.WorkRequestSubmitter,
	) error
}

// Dependencies supplies runtime edges for script-poller supervision. Construction
// stores these references without invoking them.
type Dependencies struct {
	Logger           func(workstationName, workerName string) *zap.Logger
	Clock            func() clockwork.Clock
	CommandRunner    func() workers.CommandRunner
	ResolveTemplates workers.TemplateFieldResolver
	ExecutionPolicy  factorydefinitions.WorkstationExecutionPolicyService
	CursorRecorder   CursorRecorder
}

// ScriptPollerCommandRequest builds the command invocation for a script poller worker.
func ScriptPollerCommandRequest(
	runtimeCfg factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	workerDef *factorydefinitions.FactoryWorkerConfig,
	resolveTemplates workers.TemplateFieldResolver,
	resume ResumeCursor,
) (workers.CommandRequest, error) {
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
