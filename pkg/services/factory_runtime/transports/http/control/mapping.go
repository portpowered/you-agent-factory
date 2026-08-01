package control

import (
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// RuntimeControlHTTPResponse is the wire lifecycle response retained for the
// parent adapter's compatibility surface.
type RuntimeControlHTTPResponse struct {
	Outcome factoryruntime.ControlOutcome `json:"outcome"`
}

func controlResponseFromPauseResult(result factoryruntime.PauseResult) RuntimeControlHTTPResponse {
	return RuntimeControlHTTPResponse{Outcome: result.Outcome}
}

func controlResponseFromResumeResult(result factoryruntime.ResumeResult) RuntimeControlHTTPResponse {
	return RuntimeControlHTTPResponse{Outcome: result.Outcome}
}

func controlResponseFromTerminateResult(result factoryruntime.TerminateResult) RuntimeControlHTTPResponse {
	return RuntimeControlHTTPResponse{Outcome: result.Outcome}
}

func terminateRequestFromAPI(req factoryapi.FactorySessionLifecycleControlRequest) factoryruntime.TerminateRequest {
	return factoryruntime.TerminateRequest{Reason: stringValue(req.Reason)}
}

func moveWorkRequestFromAPI(workID string, req factoryapi.MoveWorkRequest) factoryruntime.MoveWorkRequest {
	return factoryruntime.MoveWorkRequest{
		WorkID:    workID,
		StateName: strings.TrimSpace(req.StateName),
		Source:    factoryruntime.WorkMoveSourceAPI,
		RequestID: strings.TrimSpace(stringValue(req.RequestId)),
	}
}

func workResponseFromMoveResult(result factoryruntime.MoveWorkResult) factoryapi.Work {
	work := factoryapi.Work{
		WorkId:       stringPtrIfNotEmpty(result.WorkID),
		WorkTypeName: stringPtrIfNotEmpty(result.WorkTypeID),
	}
	if result.ToState != "" {
		work.State = &factoryapi.WorkState{Name: result.ToState}
	}
	return work
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
