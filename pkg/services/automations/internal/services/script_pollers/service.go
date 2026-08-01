// Package script_pollers owns Automations script-command polling and recovery.
// Its cursor, identity, parsing, and execution collaborators remain private to
// the implementation service; callers outside Automations consume the outer
// Automations service root.
package script_pollers

import (
	"context"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Service is the singular script-poller authority. It owns command execution,
// restart supervision, cursor recovery, and Work Request admission for its
// configured poller instances.
type Service interface {
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
	StartScriptPoller(
		context.Context,
		*sync.WaitGroup,
		factorydefinitions.RuntimeConfigLookup,
		factorydefinitions.FactoryWorkstationConfig,
		*factorydefinitions.FactoryWorkerConfig,
		string,
		automations.WorkRequestSubmitter,
	)
	RunScriptPoller(
		context.Context,
		workers.CommandRunner,
		factorydefinitions.RuntimeConfigLookup,
		factorydefinitions.FactoryWorkstationConfig,
		*factorydefinitions.FactoryWorkerConfig,
		string,
		automations.WorkRequestSubmitter,
	) error
}
