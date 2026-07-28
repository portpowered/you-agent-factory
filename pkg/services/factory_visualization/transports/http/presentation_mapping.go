package http

import factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"

func openPresentationHTTPResponseFromResult(
	result factoryvisualization.OpenPresentationResult,
) OpenPresentationHTTPResponse {
	return OpenPresentationHTTPResponse{
		SessionID: string(result.SessionID),
		Mode:      string(result.Mode),
	}
}

func presentProgressHTTPResponseFromResult(
	result factoryvisualization.PresentProgressResult,
) PresentProgressHTTPResponse {
	return PresentProgressHTTPResponse{AcceptedCount: result.AcceptedCount}
}

func finalizePresentationHTTPResponseFromResult(
	result factoryvisualization.FinalizePresentationResult,
) FinalizePresentationHTTPResponse {
	return FinalizePresentationHTTPResponse{
		Finalized:    result.Finalized,
		ProgressSeen: result.ProgressSeen,
	}
}

func closePresentationHTTPResponseFromResult(
	result factoryvisualization.ClosePresentationResult,
) ClosePresentationHTTPResponse {
	return ClosePresentationHTTPResponse{DroppedCount: result.DroppedCount}
}
