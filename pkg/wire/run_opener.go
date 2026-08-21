package wire

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

func provideRunOpener(
	prepareWorkTarget work.SingleWorkTargetPreparation,
	loadMockWorkers workers.MockWorkersConfigDiagnosticsLoader,
	buildRuntimeRequest runcli.RuntimeOpeningRequestFactory,
	presentations factorysessions.OpeningPresentationOwner,
	visualizations factoryvisualization.RuntimeSinkOwner,
) runcli.Opener {
	return func(
		ctx context.Context,
		cfg runcli.RunConfig,
		buildRunner runcli.RuntimeRunnerBuilder,
		invocation runcli.InvocationOperation,
		presentation factoryvisualization.ResponsePresentation,
	) (*runcli.Operation, error) {
		return runcli.OpenWithVisualizationOwnerAndDiagnostics(
			ctx, cfg, buildRunner, invocation, presentation,
			prepareWorkTarget, nil, loadMockWorkers, buildRuntimeRequest, presentations, visualizations,
		)
	}
}
