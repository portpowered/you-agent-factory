package dispatch

import (
	"io"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

type runtimeDispatchPlanHTTPRequest struct {
	DispatchID      string   `json:"dispatchId"`
	CorrelationID   string   `json:"correlationId"`
	WorkIDs         []string `json:"workIds"`
	WorkstationName string   `json:"workstationName"`
	WorkerType      string   `json:"workerType"`
	ReplayKey       string   `json:"replayKey"`
}

type runtimeAcceptDispatchResultHTTPRequest struct {
	DispatchID    string `json:"dispatchId"`
	CorrelationID string `json:"correlationId"`
	WorkID        string `json:"workId"`
	ResultOutcome string `json:"resultOutcome"`
}

// RuntimeDispatchPlanHTTPResponse is the wire dispatch response retained for
// the parent adapter's compatibility surface.
type RuntimeDispatchPlanHTTPResponse struct {
	Outcome       factoryruntime.DispatchPlanOutcome `json:"outcome"`
	DispatchID    string                             `json:"dispatchId"`
	CorrelationID string                             `json:"correlationId,omitempty"`
}

func decodePlanDispatchRequest(body io.Reader) (factoryruntime.PlanDispatchRequest, error) {
	req, err := commonDecodeRequiredJSON[runtimeDispatchPlanHTTPRequest](body)
	if err != nil {
		return factoryruntime.PlanDispatchRequest{}, err
	}
	return planDispatchRequestFromHTTP(req), nil
}

func decodeAcceptDispatchResultRequest(body io.Reader) (factoryruntime.AcceptDispatchResultRequest, error) {
	req, err := commonDecodeRequiredJSON[runtimeAcceptDispatchResultHTTPRequest](body)
	if err != nil {
		return factoryruntime.AcceptDispatchResultRequest{}, err
	}
	return acceptDispatchResultRequestFromHTTP(req), nil
}

func planDispatchRequestFromHTTP(req runtimeDispatchPlanHTTPRequest) factoryruntime.PlanDispatchRequest {
	workIDs := make([]string, 0, len(req.WorkIDs))
	for _, workID := range req.WorkIDs {
		if trimmed := strings.TrimSpace(workID); trimmed != "" {
			workIDs = append(workIDs, trimmed)
		}
	}
	return factoryruntime.PlanDispatchRequest{
		DispatchID:      strings.TrimSpace(req.DispatchID),
		CorrelationID:   strings.TrimSpace(req.CorrelationID),
		WorkIDs:         workIDs,
		WorkstationName: strings.TrimSpace(req.WorkstationName),
		WorkerType:      strings.TrimSpace(req.WorkerType),
		ReplayKey:       strings.TrimSpace(req.ReplayKey),
	}
}

func acceptDispatchResultRequestFromHTTP(req runtimeAcceptDispatchResultHTTPRequest) factoryruntime.AcceptDispatchResultRequest {
	return factoryruntime.AcceptDispatchResultRequest{
		DispatchID:    strings.TrimSpace(req.DispatchID),
		CorrelationID: strings.TrimSpace(req.CorrelationID),
		WorkID:        strings.TrimSpace(req.WorkID),
		ResultOutcome: factoryruntime.DispatchResultOutcome(strings.TrimSpace(req.ResultOutcome)),
	}
}

func dispatchPlanResponseFromPlanResult(result factoryruntime.PlanDispatchResult) RuntimeDispatchPlanHTTPResponse {
	return RuntimeDispatchPlanHTTPResponse{
		Outcome:       result.Outcome,
		DispatchID:    result.DispatchID,
		CorrelationID: result.CorrelationID,
	}
}

func dispatchPlanResponseFromAcceptResult(result factoryruntime.AcceptDispatchResultResult) RuntimeDispatchPlanHTTPResponse {
	return RuntimeDispatchPlanHTTPResponse{
		Outcome:       result.Outcome,
		DispatchID:    result.DispatchID,
		CorrelationID: result.CorrelationID,
	}
}
