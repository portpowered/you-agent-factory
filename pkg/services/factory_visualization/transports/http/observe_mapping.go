package http

import factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"

func observeHTTPResponseFromResult(
	result factoryvisualization.ObserveResult,
) ObserveHTTPResponse {
	return ObserveHTTPResponse{
		View: ObserveHTTPProjectedView{
			TickCount:          result.View.TickCount,
			RetainedEventCount: result.View.RetainedEventCount,
			ObservedAt:         result.View.ObservedAt,
		},
	}
}
