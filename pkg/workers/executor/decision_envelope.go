package executor

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
)

func decisionEnvelopeWorkResult(
	request interfaces.WorkstationExecutionRequest,
	resp interfaces.InferenceResponse,
	diagnostics *interfaces.WorkDiagnostics,
	retryCount int,
	start time.Time,
) interfaces.WorkResult {
	result := goal.WorkResultFromDecisionEnvelopeJSONOrFailed(
		request.Dispatch.DispatchID,
		request.Dispatch.TransitionID,
		resp.Content,
	)
	result.ProviderSession = interfaces.CloneProviderSessionMetadata(resp.ProviderSession)
	result.Diagnostics = diagnostics
	result.Metrics = agentWorkMetrics(start, retryCount)
	return result
}
