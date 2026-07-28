package http

import (
	"encoding/json"
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

type runtimeDispatchPlanHTTPResponse struct {
	Outcome       factoryruntime.DispatchPlanOutcome `json:"outcome"`
	DispatchID    string                             `json:"dispatchId"`
	CorrelationID string                             `json:"correlationId,omitempty"`
}

func decodePlanDispatchRequest(body io.Reader) (factoryruntime.PlanDispatchRequest, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return factoryruntime.PlanDispatchRequest{}, err
	}
	if len(payload) == 0 {
		return factoryruntime.PlanDispatchRequest{}, errRequestBodyRequired
	}
	var req runtimeDispatchPlanHTTPRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return factoryruntime.PlanDispatchRequest{}, err
	}
	return planDispatchRequestFromHTTP(req), nil
}

func decodeAcceptDispatchResultRequest(body io.Reader) (factoryruntime.AcceptDispatchResultRequest, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return factoryruntime.AcceptDispatchResultRequest{}, err
	}
	if len(payload) == 0 {
		return factoryruntime.AcceptDispatchResultRequest{}, errRequestBodyRequired
	}
	var req runtimeAcceptDispatchResultHTTPRequest
	if err := json.Unmarshal(payload, &req); err != nil {
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

func acceptDispatchResultRequestFromHTTP(
	req runtimeAcceptDispatchResultHTTPRequest,
) factoryruntime.AcceptDispatchResultRequest {
	return factoryruntime.AcceptDispatchResultRequest{
		DispatchID:    strings.TrimSpace(req.DispatchID),
		CorrelationID: strings.TrimSpace(req.CorrelationID),
		WorkID:        strings.TrimSpace(req.WorkID),
		ResultOutcome: factoryruntime.DispatchResultOutcome(strings.TrimSpace(req.ResultOutcome)),
	}
}

func dispatchPlanResponseFromPlanResult(
	result factoryruntime.PlanDispatchResult,
) runtimeDispatchPlanHTTPResponse {
	return runtimeDispatchPlanHTTPResponse{
		Outcome:       result.Outcome,
		DispatchID:    result.DispatchID,
		CorrelationID: result.CorrelationID,
	}
}

func dispatchPlanResponseFromAcceptResult(
	result factoryruntime.AcceptDispatchResultResult,
) runtimeDispatchPlanHTTPResponse {
	return runtimeDispatchPlanHTTPResponse{
		Outcome:       result.Outcome,
		DispatchID:    result.DispatchID,
		CorrelationID: result.CorrelationID,
	}
}
