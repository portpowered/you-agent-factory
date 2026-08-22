package host

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Engine is the host-private compatibility view of the concrete runtime loop.
// It is intentionally scoped to Factory Runtime's internal host package; peer
// services use factoryruntime.Service and never receive this run-loop surface.
// SubscribeFactoryEvents remains here through P5B, when the Recordings root
// becomes the sole canonical event-read owner.
type Engine interface {
	factoryruntime.APIFactory
	GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.RuntimeNet], error)
	MoveWork(context.Context, string, string, work.WorkStateChangeSource, string) (work.OperatorMoveResult, error)
	Pause(context.Context) error
	Resume(context.Context) error
	GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error)
	WaitToComplete() <-chan struct{}
	Run(context.Context) error
}
