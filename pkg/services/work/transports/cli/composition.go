package cli

import (
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

// ListOperation is the composition-facing Work list role injected by Wire and
// other composition roots before Cobra handlers dispatch list behavior.
type ListOperation func(ListConfig) error

// ShowOperation is the composition-facing Work show role injected by Wire and
// other composition roots before Cobra handlers dispatch show behavior.
type ShowOperation func(ShowConfig) error

// MoveOperation is the composition-facing Work move role injected by Wire and
// other composition roots before Cobra handlers dispatch move behavior.
type MoveOperation func(MoveConfig) error

// VisualizeCLIOperation is the composition-facing Work visualize role injected
// by Wire and other composition roots before Cobra handlers dispatch visualize
// behavior.
type VisualizeCLIOperation func(VisualizeConfig) error

// BindService returns the composition-facing Work CLI adapter Service
// constructed from accepted Work-root collaborators. Wire and other composition
// roots may inject the returned Service without reconstructing adapter behavior
// at the composition boundary.
func BindService(cfg Config) Service {
	return New(cfg)
}

// BindList returns the composition-facing operation closure that delegates Work
// list behavior to the owned CLI adapter Service. Wire and other composition
// roots inject the returned function without constructing the Service at the
// composition boundary.
func BindList(
	transport clihttp.Protocol,
	prepare workdomain.ListRequestPreparation,
) ListOperation {
	if prepare == nil {
		return nil
	}
	return NewList(transport, prepare)
}

// BindShow returns the composition-facing operation closure that delegates Work
// show behavior to the owned CLI adapter Service.
func BindShow(transport clihttp.Protocol) ShowOperation {
	if transport == nil {
		return nil
	}
	return NewShow(transport)
}

// BindMove returns the composition-facing operation closure that delegates Work
// move behavior to the owned CLI adapter Service.
func BindMove(transport clihttp.Protocol) MoveOperation {
	if transport == nil {
		return nil
	}
	return NewMove(transport)
}

// BindVisualize returns the composition-facing operation closure that delegates
// Work visualize behavior to the owned CLI adapter Service.
func BindVisualize(visualize workdomain.VisualizationOperation) VisualizeCLIOperation {
	if visualize == nil {
		return nil
	}
	return NewVisualize(visualize)
}
