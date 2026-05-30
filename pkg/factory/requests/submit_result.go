package requests

import "github.com/portpowered/infinite-you/pkg/interfaces"

// WorkRequestSubmitResultFromNormalized builds accepted-request metadata from
// normalized submit requests, including per-work identifiers for batch upserts
// and primary-work fields for unary submit responses.
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
	works := make([]interfaces.WorkRequestSubmittedWork, 0, len(work))
	for _, req := range work {
		works = append(works, interfaces.WorkRequestSubmittedWork{
			Name:         SubmitWorkName(req),
			WorkTypeName: req.WorkTypeID,
			WorkID:       req.WorkID,
		})
	}
	result.Works = works
	return result
}
