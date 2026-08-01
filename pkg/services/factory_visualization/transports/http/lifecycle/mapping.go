package lifecycle

import factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"

func lifecycleHTTPResponseFromActivateResult(
	result factoryvisualization.ActivateResult,
) LifecycleHTTPResponse {
	return lifecycleHTTPResponseFromState(result.State)
}

func lifecycleHTTPResponseFromJoinResult(
	result factoryvisualization.JoinResult,
) LifecycleHTTPResponse {
	return lifecycleHTTPResponseFromState(result.State)
}

func lifecycleHTTPResponseFromStopDrainResult(
	result factoryvisualization.StopDrainResult,
) LifecycleHTTPResponse {
	return lifecycleHTTPResponseFromState(result.State)
}

func lifecycleHTTPResponseFromState(
	state factoryvisualization.LifecycleState,
) LifecycleHTTPResponse {
	return LifecycleHTTPResponse{State: string(state)}
}
