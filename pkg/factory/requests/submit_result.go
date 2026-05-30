package requests

import "github.com/portpowered/infinite-you/pkg/interfaces"

// WorkRequestSubmitResultFromNormalized builds accepted-request metadata from the
// primary normalized work item in a batch (index 0).
func WorkRequestSubmitResultFromNormalized(requestID string, work []interfaces.SubmitRequest, accepted bool) interfaces.WorkRequestSubmitResult {
	result := interfaces.WorkRequestSubmitResult{
		RequestID: requestID,
		Accepted:  accepted,
	}
	if len(work) == 0 {
		return result
	}
	primary := work[0]
	result.TraceID = primary.TraceID
	result.WorkID = primary.WorkID
	result.Name = primary.Name
	result.WorkTypeName = primary.WorkTypeID
	return result
}
