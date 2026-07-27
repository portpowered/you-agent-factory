package activationlifecycle_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
)

func TestActivationLifecycleContractDoesNotImportFactoryRuntime(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}

	forbidden := "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	for _, dep := range strings.Fields(string(output)) {
		if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
			t.Fatalf("activation_lifecycle must not import Factory Runtime; found dependency %s", dep)
		}
	}
}

func TestActivationLifecycleLifecycleSurfaceUsesVisualizationOwnedObservation(t *testing.T) {
	t.Parallel()

	var source activationlifecycle.EventSource = observationSourceProbe{}
	var sink activationlifecycle.ViewSink = observationSinkProbe{}
	_ = source
	_ = sink

	var _ activationlifecycle.EngineObservation
	var _ activationlifecycle.ActivateRequest
	var _ activationlifecycle.ActivateResult
	var _ activationlifecycle.JoinRequest
	var _ activationlifecycle.JoinResult
	var _ activationlifecycle.StopDrainRequest
	var _ activationlifecycle.StopDrainResult
	var _ activationlifecycle.LifecycleError
}

type observationSourceProbe struct{}

func (observationSourceProbe) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	return nil, nil
}

func (observationSourceProbe) GetEngineObservation(
	context.Context,
) (*activationlifecycle.EngineObservation, error) {
	return &activationlifecycle.EngineObservation{}, nil
}

type observationSinkProbe struct{}

func (observationSinkProbe) PresentFactoryView(view activationlifecycle.View) {
	_ = view.EngineObservation
}
